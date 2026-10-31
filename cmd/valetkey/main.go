// valetkey: Kubernetes-native LLM proxy and operator.
package main

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"
	_ "time/tzdata" // budget time zones work in the distroless image

	apierrors "k8s.io/apimachinery/pkg/api/errors"

	"github.com/anas1412/valetkey/internal/admin"
	"github.com/anas1412/valetkey/internal/budget"
	"github.com/anas1412/valetkey/internal/config"
	"github.com/anas1412/valetkey/internal/kube"
	"github.com/anas1412/valetkey/internal/metrics"
	"github.com/anas1412/valetkey/internal/pricing"
	"github.com/anas1412/valetkey/internal/promql"
	"github.com/anas1412/valetkey/internal/reconciler"
	"github.com/anas1412/valetkey/internal/registry"
	"github.com/anas1412/valetkey/internal/relay"
	"github.com/anas1412/valetkey/internal/server"
	"github.com/anas1412/valetkey/internal/stats"
	"github.com/anas1412/valetkey/internal/types"
	"github.com/anas1412/valetkey/web"
)

const (
	// In-flight streams get this long to finish on shutdown.
	drainTimeout = 50 * time.Second
	// Spend is saved to CRD status this often; a crash loses at most this much.
	spendSaveInterval = 5 * time.Second
)

func main() {
	slog.SetDefault(slog.New(slog.NewJSONHandler(os.Stdout, nil)))
	if err := run(); err != nil {
		slog.Error("fatal", "err", err)
		os.Exit(1)
	}
}

func run() error {
	cfg := config.FromEnv()
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGTERM, os.Interrupt)
	defer stop()

	kc, err := kube.New()
	if err != nil {
		return err
	}
	reg, m, st := registry.New(), metrics.New(), stats.New()
	budgets := budget.NewTracker()
	prices := &pricing.Store{Interval: cfg.PricingRefresh,
		Persist: kube.ConfigMapStore{Client: kc, Namespace: cfg.OperatorNamespace, Name: "valetkey-pricing", Key: "catalog.json.gz"}}
	prices.Start(ctx)
	ctrl := reconciler.New(cfg, kc, reg, m, st, budgets)
	rl := &relay.Relay{Registry: reg, Limiter: relay.NewRateLimiter(), Metrics: m, Stats: st, Transports: relay.NewTransports(),
		Prices: prices, Budgets: budgets}
	api := &admin.API{Config: cfg, Kube: kc, Registry: reg, Controller: ctrl, Stats: st, Prices: prices, Budgets: budgets}
	if cfg.PrometheusURL != "" {
		api.Prom = promql.New(cfg.PrometheusURL)
		slog.Info("usage history from Prometheus", "url", cfg.PrometheusURL)
	}

	servers := []*http.Server{
		{Addr: cfg.ProxyAddr, Handler: server.Proxy(rl, reg, cfg.TrustProxy), ReadHeaderTimeout: 10 * time.Second},
		{Addr: cfg.MetricsAddr, Handler: server.Metrics(m, reg, ctrl.Ready), ReadHeaderTimeout: 10 * time.Second},
		{Addr: cfg.AdminAddr, Handler: server.Admin(api, web.Dist()), ReadHeaderTimeout: 10 * time.Second},
	}
	errc := make(chan error, len(servers)+1)
	for _, s := range servers {
		go func() {
			slog.Info("listening", "addr", s.Addr)
			if err := s.ListenAndServe(); !errors.Is(err, http.ErrServerClosed) {
				errc <- err
			}
		}()
	}
	go func() { errc <- ctrl.Run(ctx) }()
	// Spend saving outlives the request drain, so the last requests are saved too.
	saveCtx, stopSaving := context.WithCancel(context.Background())
	defer stopSaving()
	saved := make(chan struct{})
	go func() {
		budgets.Run(saveCtx, spendSaveInterval, func(ctx context.Context, s budget.Scope, sp types.Spend) error {
			gvr, ns := types.ProxyKeyGVR, s.Namespace
			if s.Kind == "tenant" {
				gvr, ns = types.GroupGVR, cfg.OperatorNamespace
			}
			err := kc.PatchSpend(ctx, gvr, ns, s.Name, sp)
			if apierrors.IsNotFound(err) {
				budgets.Forget(s)
				return nil
			}
			return err
		})
		close(saved)
	}()

	select {
	case <-ctx.Done():
	case err := <-errc:
		if err != nil {
			return err
		}
	}
	slog.Info("shutting down")
	drain, cancel := context.WithTimeout(context.Background(), drainTimeout)
	defer cancel()
	for _, s := range servers {
		_ = s.Shutdown(drain)
	}
	stopSaving()
	<-saved // last spend written to status
	return nil
}
