// Package config reads operator settings from the environment.
package config

import (
	"os"
	"strings"
)

type Config struct {
	// Namespace where Channel/Group CRDs and provider Secrets live.
	OperatorNamespace string
	// URL tenants use to reach the proxy (written into tenant Secrets).
	ProxyURL string
	// Tenant-facing listen address: only /v1/* is served there.
	ProxyAddr string
	// Internal listen address: health, metrics, admin API and dashboard.
	AdminAddr string
	// Admin key for the management API and dashboard. Empty = admin API disabled.
	AdminKey string
	// Number of trusted reverse proxies in front of llmproxy, or "" to use the
	// connection's address. With N, the client IP is taken from X-Forwarded-For.
	TrustProxy string
}

func FromEnv() Config {
	return Config{
		OperatorNamespace: env("OPERATOR_NAMESPACE", "llmproxy-system"),
		ProxyURL:          env("PROXY_URL", "http://llmproxy.llmproxy-system.svc.cluster.local:8000"),
		ProxyAddr:         ":" + env("PROXY_PORT", "8000"),
		AdminAddr:         ":" + env("ADMIN_PORT", env("HEALTH_PORT", "8081")),
		// Trimmed: Secrets created with --from-file usually end in a newline.
		AdminKey:   strings.TrimSpace(os.Getenv("ADMIN_KEY")),
		TrustProxy: os.Getenv("TRUST_PROXY"),
	}
}

func env(name, fallback string) string {
	if v := os.Getenv(name); v != "" {
		return v
	}
	return fallback
}
