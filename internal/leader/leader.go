// Package leader picks one Valetkey pod to do the writes that must happen once:
// minting keys and updating CRD status. It uses a coordination.k8s.io Lease through
// the dynamic client, so no extra client libraries end up in the binary.
package leader

import (
	"context"
	"log/slog"
	"sync/atomic"
	"time"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/dynamic"
)

var leaseGVR = schema.GroupVersionResource{Group: "coordination.k8s.io", Version: "v1", Resource: "leases"}

const timeFormat = "2006-01-02T15:04:05.000000Z07:00" // metav1.MicroTime

// Elector holds or waits for the Lease. Every pod runs one; one of them leads.
type Elector struct {
	Client    dynamic.Interface
	Namespace string
	Name      string
	Identity  string        // this pod
	Duration  time.Duration // a leader that stops renewing loses the Lease after this
	Renew     time.Duration // how often the Lease is renewed or tried

	leading atomic.Bool
}

// Leading reports whether this pod currently holds the Lease.
func (e *Elector) Leading() bool { return e.leading.Load() }

// Run renews or tries to take the Lease until ctx ends, then gives it up so the
// next pod takes over at once. onLead runs each time this pod becomes leader.
func (e *Elector) Run(ctx context.Context, onLead func()) {
	tick := time.NewTicker(e.Renew)
	defer tick.Stop()
	lastRenew := time.Time{}
	for {
		ok, err := e.tryAcquireOrRenew(ctx)
		switch {
		case ok:
			lastRenew = time.Now()
			if !e.leading.Swap(true) {
				slog.Info("leading: this pod mints keys and writes status", "pod", e.Identity)
				if onLead != nil {
					onLead()
				}
			}
		case e.leading.Load() && time.Since(lastRenew) > e.Duration:
			// Couldn't renew for a whole lease term: another pod may lead now.
			e.leading.Store(false)
			slog.Warn("lost leadership", "pod", e.Identity, "err", err)
		}
		select {
		case <-ctx.Done():
			if e.leading.Load() {
				e.release()
			}
			return
		case <-tick.C:
		}
	}
}

func (e *Elector) tryAcquireOrRenew(ctx context.Context) (bool, error) {
	res := e.Client.Resource(leaseGVR).Namespace(e.Namespace)
	now := time.Now().UTC()
	lease, err := res.Get(ctx, e.Name, metav1.GetOptions{})
	if apierrors.IsNotFound(err) {
		_, err = res.Create(ctx, e.lease(nil, now, now, 0), metav1.CreateOptions{})
		return err == nil, err
	}
	if err != nil {
		return false, err
	}
	holder, _, _ := unstructured.NestedString(lease.Object, "spec", "holderIdentity")
	renewed, _, _ := unstructured.NestedString(lease.Object, "spec", "renewTime")
	acquired, _, _ := unstructured.NestedString(lease.Object, "spec", "acquireTime")
	transitions, _, _ := unstructured.NestedInt64(lease.Object, "spec", "leaseTransitions")
	renewTime, _ := time.Parse(timeFormat, renewed)
	if holder != "" && holder != e.Identity && now.Sub(renewTime) < e.Duration {
		return false, nil // someone else leads and is alive
	}
	acquire, _ := time.Parse(timeFormat, acquired)
	if holder != e.Identity {
		acquire, transitions = now, transitions+1
	}
	// Update with the read resourceVersion: if another pod got there first, this fails.
	_, err = res.Update(ctx, e.lease(lease, acquire, now, transitions), metav1.UpdateOptions{})
	return err == nil, err
}

func (e *Elector) lease(cur *unstructured.Unstructured, acquire, renew time.Time, transitions int64) *unstructured.Unstructured {
	obj := &unstructured.Unstructured{Object: map[string]any{
		"apiVersion": "coordination.k8s.io/v1", "kind": "Lease",
		"metadata": map[string]any{"name": e.Name, "namespace": e.Namespace},
	}}
	if cur != nil {
		obj.SetResourceVersion(cur.GetResourceVersion())
	}
	obj.Object["spec"] = map[string]any{
		"holderIdentity":       e.Identity,
		"leaseDurationSeconds": int64(e.Duration.Seconds()),
		"acquireTime":          acquire.Format(timeFormat),
		"renewTime":            renew.Format(timeFormat),
		"leaseTransitions":     transitions,
	}
	return obj
}

// release clears the holder so another pod can lead right away.
func (e *Elector) release() {
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	res := e.Client.Resource(leaseGVR).Namespace(e.Namespace)
	lease, err := res.Get(ctx, e.Name, metav1.GetOptions{})
	if err != nil {
		return
	}
	if holder, _, _ := unstructured.NestedString(lease.Object, "spec", "holderIdentity"); holder != e.Identity {
		return
	}
	_ = unstructured.SetNestedField(lease.Object, "", "spec", "holderIdentity")
	_, _ = res.Update(ctx, lease, metav1.UpdateOptions{})
	e.leading.Store(false)
}
