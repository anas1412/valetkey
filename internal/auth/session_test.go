package auth

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestSessionLoginLogoutAndLockout(t *testing.T) {
	s := NewSessions("secret")

	// Wrong keys lock the IP out after maxFailures attempts; other IPs are unaffected.
	for i := 0; i < maxFailures; i++ {
		if s.Login(httptest.NewRecorder(), httptest.NewRequest("POST", "/", nil), "wrong", "1.1.1.1") {
			t.Fatal("wrong key accepted")
		}
	}
	if s.LockedOut("1.1.1.1") <= 0 {
		t.Fatal("IP not locked out after repeated failures")
	}
	if s.LockedOut("2.2.2.2") != 0 {
		t.Fatal("unrelated IP locked out")
	}

	// A good login sets a cookie that authorizes requests…
	rec := httptest.NewRecorder()
	if !s.Login(rec, httptest.NewRequest("POST", "/", nil), "secret", "2.2.2.2") {
		t.Fatal("right key rejected")
	}
	cookie := rec.Result().Cookies()[0]
	req := httptest.NewRequest("GET", "/", nil)
	req.AddCookie(cookie)
	if ok, via := s.Authorized(req); !ok || via != "dashboard" {
		t.Fatalf("cookie not accepted: %v %q", ok, via)
	}

	// …until it is signed out, even though the browser could replay it.
	s.Logout(httptest.NewRecorder(), req)
	if ok, _ := s.Authorized(req); ok {
		t.Fatal("cookie still valid after sign-out")
	}

	// Tampered cookies and the bearer key.
	bad := httptest.NewRequest("GET", "/", nil)
	bad.AddCookie(&http.Cookie{Name: cookieName, Value: "9999999999.deadbeef"})
	if ok, _ := s.Authorized(bad); ok {
		t.Fatal("forged cookie accepted")
	}
	api := httptest.NewRequest("GET", "/", nil)
	api.Header.Set("Authorization", "Bearer secret")
	if ok, via := s.Authorized(api); !ok || via != "api" {
		t.Fatal("bearer admin key rejected")
	}
}
