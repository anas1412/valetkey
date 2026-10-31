package keys

import (
	"regexp"
	"testing"
)

func TestGenerateAndHash(t *testing.T) {
	a := Generate()
	if !regexp.MustCompile(`^sk-proxy-[A-Za-z0-9_-]{32}$`).MatchString(a) {
		t.Fatalf("bad key format %q", a)
	}
	if a == Generate() {
		t.Fatal("keys must be unique")
	}
	if Hash(a) != Hash(a) {
		t.Fatal("hash must be deterministic")
	}
	if !Equal("abc", "abc") || Equal("abc", "abcd") {
		t.Fatal("Equal is wrong")
	}
}

func TestAllowList(t *testing.T) {
	l, err := ParseAllowList([]string{"10.0.0.0/8", "2001:db8::/32", "192.168.1.5"})
	if err != nil {
		t.Fatal(err)
	}
	for ip, want := range map[string]bool{
		"10.1.2.3": true, "::ffff:10.1.2.3": true, "2001:db8::1": true, "192.168.1.5": true,
		"192.168.1.6": false, "11.0.0.1": false, "garbage": false,
	} {
		if got := l.Allows(ip); got != want {
			t.Errorf("Allows(%q) = %v, want %v", ip, got, want)
		}
	}
	for _, bad := range []string{"10.0.0.0/33", "not-an-ip/8"} {
		if _, err := ParseAllowList([]string{bad}); err == nil {
			t.Errorf("expected error for %q", bad)
		}
	}
}
