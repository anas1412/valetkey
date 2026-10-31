package registry

import (
	"testing"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/anas1412/valetkey/internal/types"
)

// A rejected key keeps its channel out of routing until the key or provider changes.
func TestRejectedKey(t *testing.T) {
	r := New()
	ch := func(baseURL string, priority int) *types.Channel {
		return &types.Channel{ObjectMeta: metav1.ObjectMeta{Name: "openai"},
			Spec: types.ChannelSpec{Type: "openai", BaseURL: baseURL, Priority: priority}}
	}
	r.UpsertChannel(ch("https://api.openai.com", 1), "sk-old")
	r.UpsertGroup(&types.TenantGroup{ObjectMeta: metav1.ObjectMeta{Name: "billing"}, Spec: types.GroupSpec{ChannelRefs: []string{"openai"}}})

	if !r.SetRejected("openai", "provider rejected the API key (HTTP 401)") || r.SetRejected("openai", "provider rejected the API key (HTTP 401)") {
		t.Fatal("SetRejected should report a change once")
	}
	if len(r.GroupChannels("billing")) != 0 {
		t.Fatal("a rejected channel must not be routed to")
	}

	r.UpsertChannel(ch("https://api.openai.com", 5), "sk-old") // other spec change, same key
	if e, _ := r.Channel("openai"); e.Rejected == "" {
		t.Fatal("the rejection must survive a change that keeps the key")
	}
	r.UpsertChannel(ch("https://api.openai.com", 5), "sk-new")
	if e, _ := r.Channel("openai"); e.Rejected != "" || !e.Usable() {
		t.Fatal("a new key must clear the rejection")
	}

	r.SetRejected("openai", "rejected")
	r.UpsertChannel(ch("https://openrouter.ai/api", 5), "sk-new")
	if e, _ := r.Channel("openai"); e.Rejected != "" {
		t.Fatal("a new provider URL must clear the rejection")
	}
}
