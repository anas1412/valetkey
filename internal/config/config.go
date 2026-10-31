// Package config reads operator settings from the environment.
package config

import (
	"os"
	"time"
)

type Config struct {
	// Namespace where Channel/Group CRDs and provider Secrets live.
	OperatorNamespace string
	// URL tenants use to reach the proxy (written into tenant Secrets).
	ProxyURL string
	// Tenant-facing listen address: only /v1/* is served there.
	ProxyAddr string
	// Health and Prometheus metrics, reachable by the kubelet and Prometheus.
	MetricsAddr string
	// Dashboard and admin API. Bound to the pod's loopback by default, so the only
	// way in is `kubectl port-forward`, which Kubernetes RBAC controls.
	AdminAddr string
	// Optional Prometheus base URL; when set, the dashboard reads usage history from it.
	PrometheusURL string
	// Number of trusted reverse proxies in front of valetkey, or "" to use the
	// connection's address. With N, the client IP is taken from X-Forwarded-For.
	TrustProxy string
	// How often live prices are fetched; 0 turns fetching off (built-in prices and
	// channel overrides only, for clusters without internet access).
	PricingRefresh time.Duration
}

func FromEnv() Config {
	return Config{
		OperatorNamespace: env("OPERATOR_NAMESPACE", "valetkey-system"),
		ProxyURL:          env("PROXY_URL", "http://valetkey.valetkey-system.svc.cluster.local:8000"),
		ProxyAddr:         ":" + env("PROXY_PORT", "8000"),
		MetricsAddr:       ":" + env("METRICS_PORT", "8081"),
		AdminAddr:         env("ADMIN_ADDR", "127.0.0.1:8082"),
		PrometheusURL:     os.Getenv("PROMETHEUS_URL"),
		TrustProxy:        os.Getenv("TRUST_PROXY"),
		PricingRefresh:    duration("PRICING_REFRESH", 6*time.Hour),
	}
}

func env(name, fallback string) string {
	if v := os.Getenv(name); v != "" {
		return v
	}
	return fallback
}

// duration reads a Go duration ("6h"); "0" or "off" disables.
func duration(name string, fallback time.Duration) time.Duration {
	v := os.Getenv(name)
	if v == "" {
		return fallback
	}
	if v == "off" {
		return 0
	}
	d, err := time.ParseDuration(v)
	if err != nil {
		return fallback
	}
	return d
}
