package auth

import (
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/anas1412/llmproxy-k8s/internal/keys"
)

const (
	cookieName = "llmproxy_session"
	sessionTTL = 8 * time.Hour

	// Failed sign-ins allowed per client IP per window before it is locked out.
	maxFailures   = 5
	failureWindow = 15 * time.Minute
)

// Sessions are signed cookies: "<expiry>.<hmac(adminKey, expiry)>", so changing
// ADMIN_KEY invalidates every session. Signed-out cookies are remembered until
// they expire, and repeated wrong keys lock the client IP out for a while.
type Sessions struct {
	AdminKey string

	mu       sync.Mutex
	revoked  map[string]time.Time // cookie value → expiry
	failures map[string][]time.Time
}

func NewSessions(adminKey string) *Sessions {
	return &Sessions{AdminKey: adminKey, revoked: map[string]time.Time{}, failures: map[string][]time.Time{}}
}

// LockedOut returns how long ip must wait before trying again (0 = allowed).
func (s *Sessions) LockedOut(ip string) time.Duration {
	s.mu.Lock()
	defer s.mu.Unlock()
	recent := s.recentFailures(ip)
	if len(recent) < maxFailures {
		return 0
	}
	return time.Until(recent[0].Add(failureWindow))
}

// Login checks the admin key and, if correct, sets the session cookie.
func (s *Sessions) Login(w http.ResponseWriter, r *http.Request, key, ip string) bool {
	if s.AdminKey == "" || !keys.Equal(key, s.AdminKey) {
		s.mu.Lock()
		s.failures[ip] = append(s.recentFailures(ip), time.Now())
		s.mu.Unlock()
		return false
	}
	s.mu.Lock()
	delete(s.failures, ip)
	s.mu.Unlock()

	exp := time.Now().Add(sessionTTL)
	expiry := strconv.FormatInt(exp.Unix(), 10)
	http.SetCookie(w, &http.Cookie{
		Name: cookieName, Value: expiry + "." + keys.Sign(s.AdminKey, expiry), Path: "/",
		HttpOnly: true, SameSite: http.SameSiteStrictMode, Secure: r.TLS != nil, MaxAge: int(sessionTTL.Seconds()),
	})
	return true
}

// Logout revokes the current cookie server-side and clears it in the browser.
func (s *Sessions) Logout(w http.ResponseWriter, r *http.Request) {
	if c, err := r.Cookie(cookieName); err == nil {
		if exp, ok := s.validCookie(c.Value); ok {
			s.mu.Lock()
			s.revoked[c.Value] = exp
			for v, e := range s.revoked {
				if time.Now().After(e) {
					delete(s.revoked, v)
				}
			}
			s.mu.Unlock()
		}
	}
	http.SetCookie(w, &http.Cookie{Name: cookieName, Value: "", Path: "/", MaxAge: -1, HttpOnly: true, SameSite: http.SameSiteStrictMode})
}

// Authorized accepts a valid session cookie or "Authorization: Bearer <ADMIN_KEY>".
// It also reports which one was used, for the audit trail.
func (s *Sessions) Authorized(r *http.Request) (ok bool, via string) {
	if s.AdminKey == "" {
		return false, ""
	}
	if v, found := strings.CutPrefix(r.Header.Get("Authorization"), "Bearer "); found && keys.Equal(v, s.AdminKey) {
		return true, "api"
	}
	c, err := r.Cookie(cookieName)
	if err != nil {
		return false, ""
	}
	if _, valid := s.validCookie(c.Value); !valid {
		return false, ""
	}
	s.mu.Lock()
	_, revoked := s.revoked[c.Value]
	s.mu.Unlock()
	return !revoked, "dashboard"
}

func (s *Sessions) validCookie(value string) (time.Time, bool) {
	expiry, sig, ok := strings.Cut(value, ".")
	unix, err := strconv.ParseInt(expiry, 10, 64)
	if !ok || err != nil {
		return time.Time{}, false
	}
	exp := time.Unix(unix, 0)
	if time.Now().After(exp) || !keys.Equal(sig, keys.Sign(s.AdminKey, expiry)) {
		return time.Time{}, false
	}
	return exp, true
}

// recentFailures drops failures older than the window. Caller holds mu.
func (s *Sessions) recentFailures(ip string) []time.Time {
	var out []time.Time
	for _, t := range s.failures[ip] {
		if time.Since(t) < failureWindow {
			out = append(out, t)
		}
	}
	if len(out) == 0 {
		delete(s.failures, ip)
	} else {
		s.failures[ip] = out
	}
	return out
}
