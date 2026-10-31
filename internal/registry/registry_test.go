package registry

import (
	"testing"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/anas1412/valetkey/internal/types"
)

// A rejected key keeps its provider out of routing until the key or provider changes.
func TestRejectedKey(t *testing.T) {
	r := New()
	ch := func(baseURL string, priority int) *types.Provider {
		return &types.Provider{ObjectMeta: metav1.ObjectMeta{Name: "openai"},
			Spec: types.ProviderSpec{Type: "openai", BaseURL: baseURL, Priority: priority}}
	}
	r.UpsertProvider(ch("https://api.openai.com", 1), "sk-old")
	r.UpsertTenant(&types.Tenant{ObjectMeta: metav1.ObjectMeta{Name: "billing"}, Spec: types.TenantSpec{Providers: []string{"openai"}}})

	if !r.SetRejected("openai", "provider rejected the API key (HTTP 401)") || r.SetRejected("openai", "provider rejected the API key (HTTP 401)") {
		t.Fatal("SetRejected should report a change once")
	}
	if len(r.TenantProviders("billing")) != 0 {
		t.Fatal("a rejected provider must not be routed to")
	}

	r.UpsertProvider(ch("https://api.openai.com", 5), "sk-old") // other spec change, same key
	if e, _ := r.Provider("openai"); e.Rejected == "" {
		t.Fatal("the rejection must survive a change that keeps the key")
	}
	r.UpsertProvider(ch("https://api.openai.com", 5), "sk-new")
	if e, _ := r.Provider("openai"); e.Rejected != "" || !e.Usable() {
		t.Fatal("a new key must clear the rejection")
	}

	r.SetRejected("openai", "rejected")
	r.UpsertProvider(ch("https://openrouter.ai/api", 5), "sk-new")
	if e, _ := r.Provider("openai"); e.Rejected != "" {
		t.Fatal("a new provider URL must clear the rejection")
	}
}
