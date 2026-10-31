// Package config reads operator settings from the environment.
package config

import "os"

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
	// Number of trusted reverse proxies in front of llmproxy, or "" to use the
	// connection's address. With N, the client IP is taken from X-Forwarded-For.
	TrustProxy string
}

func FromEnv() Config {
	return Config{
		OperatorNamespace: env("OPERATOR_NAMESPACE", "llmproxy-system"),
		ProxyURL:          env("PROXY_URL", "http://llmproxy.llmproxy-system.svc.cluster.local:8000"),
		ProxyAddr:         ":" + env("PROXY_PORT", "8000"),
		MetricsAddr:       ":" + env("METRICS_PORT", "8081"),
		AdminAddr:         env("ADMIN_ADDR", "127.0.0.1:8082"),
		TrustProxy:        os.Getenv("TRUST_PROXY"),
	}
}

func env(name, fallback string) string {
	if v := os.Getenv(name); v != "" {
		return v
	}
	return fallback
}
