package relay

import (
	"sort"
	"testing"
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/anas1412/llmp/internal/registry"
	"github.com/anas1412/llmp/internal/types"
)

func TestUsageBufferedAndStreamed(t *testing.T) {
	var u Usage
	u.FeedJSON([]byte(`{"usage":{"prompt_tokens":12,"completion_tokens":7}}`))
	if u.Input != 12 || u.Output != 7 {
		t.Fatalf("openai usage = %d/%d", u.Input, u.Output)
	}

	var a Usage
	for _, c := range []string{
		"event: message_start\ndata: {\"type\":\"message_start\",\"message\":{\"usage\":{\"input_tokens\":25,\"output_tokens\":1}}}\n\n",
		"event: message_delta\ndata: {\"type\":\"message_delta\",\"usa",
		"ge\":{\"output_tokens\":42}}\n\ndata: [DONE]",
	} {
		a.FeedSSE(c)
	}
	a.EndSSE()
	if a.Input != 25 || a.Output != 42 {
		t.Fatalf("anthropic stream usage = %d/%d", a.Input, a.Output)
	}
}

func TestJoinURL(t *testing.T) {
	for base, want := range map[string]string{
		"https://api.openai.com":         "https://api.openai.com/v1/chat/completions",
		"https://openrouter.ai/api/":     "https://openrouter.ai/api/v1/chat/completions",
		"https://api.groq.com/openai/v1": "https://api.groq.com/openai/v1/chat/completions",
	} {
		if got, _ := JoinURL(base, "/v1/chat/completions"); got != want {
			t.Errorf("JoinURL(%q) = %q, want %q", base, got, want)
		}
	}
}

func TestRateLimiter(t *testing.T) {
	l := NewRateLimiter()
	now := time.Unix(1000, 0)
	l.now = func() time.Time { return now }

	// rpm 2 × ratio 1.5 = 3 per minute
	for i := 0; i < 3; i++ {
		if wait, _ := l.Acquire("k", 2, 0, 1.5, 0); wait != 0 {
			t.Fatalf("request %d limited", i)
		}
	}
	if wait, tier := l.Acquire("k", 2, 0, 1.5, 0); tier != "rpm" || wait < 1 || wait > 60 {
		t.Fatalf("expected rpm limit, got %d %q", wait, tier)
	}
	now = now.Add(61 * time.Second)
	if wait, _ := l.Acquire("k", 2, 0, 1.5, 0); wait != 0 {
		t.Fatal("window did not reset")
	}

	l.RecordTokens("t", 150)
	if _, tier := l.Acquire("t", 0, 100, 0, 0); tier != "tpm" {
		t.Fatal("expected tpm limit")
	}
	if wait, _ := l.Acquire("free", 0, 0, 0, 0); wait != 0 {
		t.Fatal("unlimited key was limited")
	}
}

func TestCandidates(t *testing.T) {
	reg := registry.New()
	reg.UpsertChannel(channel("oa", "openai", 10, []string{"gpt-4o"}, nil, ""), "k1")
	reg.UpsertChannel(channel("an", "anthropic", 10, []string{"claude-x"}, map[string]string{"gpt-4o": "claude-x"}, ""), "k2")
	reg.UpsertChannel(channel("off", "openai", 10, nil, nil, types.Disabled), "k3")
	reg.UpsertChannel(channel("nokey", "openai", 10, nil, nil, ""), "")
	reg.UpsertGroup(&types.TenantGroup{ObjectMeta: metav1.ObjectMeta{Name: "team"}, Spec: types.GroupSpec{ChannelRefs: []string{"oa", "an", "off", "nokey"}}})

	check := func(model string, api API, want ...string) {
		t.Helper()
		var got []string
		for _, e := range Candidates(reg, "team", model, api) {
			got = append(got, e.Name())
		}
		sort.Strings(got)
		sort.Strings(want)
		if len(got) != len(want) || (len(got) > 0 && got[0] != want[0]) || (len(got) > 1 && got[1] != want[1]) {
			t.Errorf("Candidates(%q) = %v, want %v", model, got, want)
		}
	}
	check("gpt-4o", OpenAI, "oa", "an")
	check("claude-x", OpenAI, "an")
	check("gpt-4o", Anthropic, "an")
	check("unknown", OpenAI)
	if len(Candidates(reg, "other", "gpt-4o", OpenAI)) != 0 {
		t.Error("unknown group must have no candidates")
	}
}

func channel(name, typ string, prio int, models []string, mapping map[string]string, status string) *types.Channel {
	return &types.Channel{
		ObjectMeta: metav1.ObjectMeta{Name: name},
		Spec: types.ChannelSpec{Type: typ, BaseURL: "http://x", Priority: prio, Models: models, ModelMapping: mapping, Status: status,
			KeySecretRef: types.SecretRef{Name: "s"}},
	}
}
