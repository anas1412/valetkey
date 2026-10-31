// Package kube wraps the dynamic Kubernetes client: CRD and Secret reads/writes
// and status patches. Only the dynamic client is used, to keep the binary small.
package kube

import (
	"context"
	"encoding/base64"
	"encoding/json"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	ktypes "k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"

	"github.com/anas1412/valetkey/internal/types"
)

const ManagedBy = "valetkey"

type Client struct {
	Dynamic dynamic.Interface
}

// New uses the in-cluster service account, or $KUBECONFIG / ~/.kube/config for local runs.
func New() (*Client, error) {
	cfg, err := rest.InClusterConfig()
	if err != nil {
		rules := clientcmd.NewDefaultClientConfigLoadingRules()
		cfg, err = clientcmd.NewNonInteractiveDeferredLoadingClientConfig(rules, nil).ClientConfig()
		if err != nil {
			return nil, err
		}
	}
	dyn, err := dynamic.NewForConfig(cfg)
	if err != nil {
		return nil, err
	}
	return &Client{Dynamic: dyn}, nil
}

// Secret is the part of a Kubernetes Secret valetkey uses.
type Secret struct {
	Name, Namespace string
	Labels          map[string]string
	Data            map[string]string // decoded
}

// ReadSecret returns nil (no error) when the Secret does not exist.
func (c *Client) ReadSecret(ctx context.Context, ns, name string) (*Secret, error) {
	u, err := c.Dynamic.Resource(types.SecretGVR).Namespace(ns).Get(ctx, name, metav1.GetOptions{})
	if apierrors.IsNotFound(err) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	s := &Secret{Name: name, Namespace: ns, Labels: u.GetLabels(), Data: map[string]string{}}
	data, _, _ := unstructured.NestedStringMap(u.Object, "data")
	for k, v := range data {
		if b, err := base64.StdEncoding.DecodeString(v); err == nil {
			s.Data[k] = string(b)
		}
	}
	return s, nil
}

// SecretValue returns "" when the Secret or the key is missing.
func (c *Client) SecretValue(ctx context.Context, ns, name, key string) (string, error) {
	s, err := c.ReadSecret(ctx, ns, name)
	if err != nil || s == nil {
		return "", err
	}
	return s.Data[key], nil
}

// ApplySecret creates or replaces an Opaque Secret with the given string data.
func (c *Client) ApplySecret(ctx context.Context, ns, name string, labels map[string]string, owner *metav1.OwnerReference, data map[string]string) error {
	meta := map[string]any{"name": name, "namespace": ns}
	if labels != nil {
		meta["labels"] = toAny(labels)
	}
	if owner != nil {
		raw, err := toMap(owner)
		if err != nil {
			return err
		}
		meta["ownerReferences"] = []any{raw}
	}
	obj := &unstructured.Unstructured{Object: map[string]any{
		"apiVersion": "v1", "kind": "Secret", "metadata": meta, "type": "Opaque", "stringData": toAny(data),
	}}
	res := c.Dynamic.Resource(types.SecretGVR).Namespace(ns)
	existing, err := res.Get(ctx, name, metav1.GetOptions{})
	if apierrors.IsNotFound(err) {
		_, err = res.Create(ctx, obj, metav1.CreateOptions{})
		return err
	}
	if err != nil {
		return err
	}
	obj.SetResourceVersion(existing.GetResourceVersion())
	_, err = res.Update(ctx, obj, metav1.UpdateOptions{})
	return err
}

// DeleteSecret ignores a missing Secret.
func (c *Client) DeleteSecret(ctx context.Context, ns, name string) error {
	err := c.Dynamic.Resource(types.SecretGVR).Namespace(ns).Delete(ctx, name, metav1.DeleteOptions{})
	if apierrors.IsNotFound(err) {
		return nil
	}
	return err
}

// PatchStatus merge-patches the status subresource of a CRD object.
func (c *Client) PatchStatus(ctx context.Context, gvr schema.GroupVersionResource, ns, name string, status types.Status) error {
	body, err := json.Marshal(map[string]any{"status": status})
	if err != nil {
		return err
	}
	_, err = c.Dynamic.Resource(gvr).Namespace(ns).Patch(ctx, name, ktypes.MergePatchType, body, metav1.PatchOptions{}, "status")
	return err
}

// Create creates a CRD object from a typed value.
func (c *Client) Create(ctx context.Context, gvr schema.GroupVersionResource, ns string, obj any) (*unstructured.Unstructured, error) {
	raw, err := toMap(obj)
	if err != nil {
		return nil, err
	}
	return c.Dynamic.Resource(gvr).Namespace(ns).Create(ctx, &unstructured.Unstructured{Object: raw}, metav1.CreateOptions{})
}

// Get reads one CRD object.
func (c *Client) Get(ctx context.Context, gvr schema.GroupVersionResource, ns, name string) (*unstructured.Unstructured, error) {
	return c.Dynamic.Resource(gvr).Namespace(ns).Get(ctx, name, metav1.GetOptions{})
}

// ReplaceSpec replaces only `spec` of the live object, keeping metadata and status.
func (c *Client) ReplaceSpec(ctx context.Context, gvr schema.GroupVersionResource, ns, name string, spec any) (*unstructured.Unstructured, error) {
	res := c.Dynamic.Resource(gvr).Namespace(ns)
	cur, err := res.Get(ctx, name, metav1.GetOptions{})
	if err != nil {
		return nil, err
	}
	raw, err := toMap(spec)
	if err != nil {
		return nil, err
	}
	cur.Object["spec"] = raw
	return res.Update(ctx, cur, metav1.UpdateOptions{})
}

func (c *Client) Delete(ctx context.Context, gvr schema.GroupVersionResource, ns, name string) error {
	return c.Dynamic.Resource(gvr).Namespace(ns).Delete(ctx, name, metav1.DeleteOptions{})
}

func (c *Client) Namespaces(ctx context.Context) ([]string, error) {
	list, err := c.Dynamic.Resource(types.NamespaceGVR).List(ctx, metav1.ListOptions{})
	if err != nil {
		return nil, err
	}
	names := make([]string, 0, len(list.Items))
	for _, n := range list.Items {
		names = append(names, n.GetName())
	}
	return names, nil
}

// toMap converts a typed value (or pointer) into an unstructured map via JSON.
func toMap(v any) (map[string]any, error) {
	b, err := json.Marshal(v)
	if err != nil {
		return nil, err
	}
	var out map[string]any
	return out, json.Unmarshal(b, &out)
}

func toAny(m map[string]string) map[string]any {
	out := make(map[string]any, len(m))
	for k, v := range m {
		out[k] = v
	}
	return out
}
