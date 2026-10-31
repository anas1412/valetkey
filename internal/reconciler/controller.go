// Package reconciler watches Channels, Groups, provider Secrets and ProxyKeys,
// keeps the registry current, mints virtual keys and writes CRD status.
package reconciler

import (
	"context"
	"github.com/anas1412/valetkey/internal/budget"
	"log/slog"
	"reflect"
	"sync"
	"sync/atomic"
	"time"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/client-go/dynamic/dynamicinformer"
	"k8s.io/client-go/tools/cache"
	"k8s.io/client-go/util/workqueue"

	"github.com/anas1412/valetkey/internal/config"
	"github.com/anas1412/valetkey/internal/kube"
	"github.com/anas1412/valetkey/internal/metrics"
	"github.com/anas1412/valetkey/internal/registry"
	"github.com/anas1412/valetkey/internal/stats"
	"github.com/anas1412/valetkey/internal/types"
)

// Periodic resync re-delivers every object as an update, catching anything missed
// (e.g. a tenant Secret deleted by hand).
const resync = 10 * time.Minute

type Controller struct {
	cfg      config.Config
	kube     *kube.Client
	registry *registry.Registry
	metrics  *metrics.Metrics
	stats    *stats.Stats
	budgets  *budget.Tracker // optional

	queue     workqueue.TypedRateLimitingInterface[string]
	proxyKeys cache.GenericLister
	secrets   cache.GenericLister
	ready     atomic.Bool
	chanMu    sync.Mutex
	problems  map[string]string // channel → last reported key problem; guarded by chanMu
}

func New(cfg config.Config, k *kube.Client, reg *registry.Registry, m *metrics.Metrics, st *stats.Stats, b *budget.Tracker) *Controller {
	return &Controller{
		cfg: cfg, kube: k, registry: reg, metrics: m, stats: st, budgets: b,
		queue:    workqueue.NewTypedRateLimitingQueue(workqueue.DefaultTypedControllerRateLimiter[string]()),
		problems: map[string]string{},
	}
}

// Ready is true once every informer has loaded its initial list.
func (c *Controller) Ready() bool { return c.ready.Load() }

// Run starts the informers and the reconcile worker; it blocks until ctx ends.
func (c *Controller) Run(ctx context.Context) error {
	ns := c.cfg.OperatorNamespace
	local := dynamicinformer.NewFilteredDynamicSharedInformerFactory(c.kube.Dynamic, resync, ns, nil)
	cluster := dynamicinformer.NewDynamicSharedInformerFactory(c.kube.Dynamic, resync)

	// Provider Secrets load first, so channels resolve their keys on the first pass
	// instead of briefly reporting "secret not found" on every restart.
	secrets := local.ForResource(types.SecretGVR)
	c.secrets = secrets.Lister()
	local.Start(ctx.Done())
	if !cache.WaitForCacheSync(ctx.Done(), secrets.Informer().HasSynced) {
		return ctx.Err()
	}

	channels := local.ForResource(types.ChannelGVR)
	groups := local.ForResource(types.GroupGVR)
	proxyKeys := cluster.ForResource(types.ProxyKeyGVR)
	c.proxyKeys = proxyKeys.Lister()

	channels.Informer().AddEventHandler(handler(c.onChannel, c.onChannelDelete))
	groups.Informer().AddEventHandler(handler(c.onGroup, c.onGroupDelete))
	secrets.Informer().AddEventHandler(handler(c.onSecret, c.onSecret))
	proxyKeys.Informer().AddEventHandler(handler(c.enqueue, c.onProxyKeyDelete))

	local.Start(ctx.Done()) // starts the informers added since the first Start
	cluster.Start(ctx.Done())
	for _, inf := range []cache.SharedIndexInformer{channels.Informer(), groups.Informer(), proxyKeys.Informer()} {
		if !cache.WaitForCacheSync(ctx.Done(), inf.HasSynced) {
			return ctx.Err()
		}
	}
	c.ready.Store(true)
	slog.Info("informers synced", "operatorNamespace", ns)

	go c.worker(ctx)
	<-ctx.Done()
	c.queue.ShutDown()
	return nil
}

// RequeueAll re-reconciles every ProxyKey (after a Group or Channel change).
func (c *Controller) RequeueAll() {
	if c.proxyKeys == nil {
		return
	}
	objs, _ := c.proxyKeys.List(everything())
	for _, o := range objs {
		if u, ok := o.(*unstructured.Unstructured); ok {
			c.queue.Add(u.GetNamespace() + "/" + u.GetName())
		}
	}
}

// Enqueue schedules one ProxyKey for reconcile.
func (c *Controller) Enqueue(namespace, name string) { c.queue.Add(namespace + "/" + name) }

// ProxyKeys returns every known ProxyKey, including not-ready and disabled ones.
func (c *Controller) ProxyKeys() []*types.ProxyKey {
	if c.proxyKeys == nil {
		return nil
	}
	objs, _ := c.proxyKeys.List(everything())
	out := make([]*types.ProxyKey, 0, len(objs))
	for _, o := range objs {
		if pk, ok := types.FromUnstructured[types.ProxyKey](o); ok {
			out = append(out, pk)
		}
	}
	return out
}

// ProxyKey returns one ProxyKey from the informer cache.
func (c *Controller) ProxyKey(namespace, name string) (*types.ProxyKey, bool) {
	if c.proxyKeys == nil {
		return nil, false
	}
	o, err := c.proxyKeys.ByNamespace(namespace).Get(name)
	if err != nil {
		return nil, false
	}
	return types.FromUnstructured[types.ProxyKey](o)
}

func (c *Controller) enqueue(obj any) {
	if u, ok := obj.(*unstructured.Unstructured); ok {
		c.queue.Add(u.GetNamespace() + "/" + u.GetName())
	}
}

func (c *Controller) worker(ctx context.Context) {
	for {
		key, shutdown := c.queue.Get()
		if shutdown {
			return
		}
		if err := c.reconcile(ctx, key); err != nil {
			slog.Error("reconcile failed", "proxykey", key, "err", err)
			c.stats.Event("error", "reconcile", key+": "+err.Error())
			c.queue.AddRateLimited(key)
		} else {
			c.queue.Forget(key)
		}
		c.queue.Done(key)
	}
}

// handler builds informer callbacks; deletes may arrive as tombstones.
func handler(onChange, onDelete func(any)) cache.ResourceEventHandlerFuncs {
	return cache.ResourceEventHandlerFuncs{
		AddFunc: onChange,
		UpdateFunc: func(old, obj any) {
			if spendOnly(old, obj) {
				return // our own budget bookkeeping, nothing to reconcile
			}
			onChange(obj)
		},
		DeleteFunc: func(obj any) {
			if t, ok := obj.(cache.DeletedFinalStateUnknown); ok {
				obj = t.Obj
			}
			onDelete(obj)
		},
	}
}

// spendOnly reports an update that changed nothing but status.spend.
func spendOnly(old, obj any) bool {
	o, ok1 := old.(*unstructured.Unstructured)
	n, ok2 := obj.(*unstructured.Unstructured)
	if !ok1 || !ok2 || o.GetGeneration() != n.GetGeneration() {
		return false
	}
	os, _, _ := unstructured.NestedMap(o.Object, "status")
	ns, _, _ := unstructured.NestedMap(n.Object, "status")
	if reflect.DeepEqual(os["spend"], ns["spend"]) {
		return false
	}
	delete(os, "spend")
	delete(ns, "spend")
	return reflect.DeepEqual(os, ns) && reflect.DeepEqual(o.Object["spec"], n.Object["spec"])
}
