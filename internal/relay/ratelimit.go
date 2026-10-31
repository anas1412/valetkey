package relay

import (
	"math"
	"sync"
	"time"
)

const window = time.Minute

type counter struct {
	count   int
	resetAt time.Time
}

// RateLimiter is an in-memory fixed-window (1 minute) limiter per ProxyKey.
// No Redis, so limits apply per replica.
type RateLimiter struct {
	mu  sync.Mutex
	rpm map[string]*counter
	tpm map[string]*counter
	now func() time.Time
}

func NewRateLimiter() *RateLimiter {
	return &RateLimiter{rpm: map[string]*counter{}, tpm: map[string]*counter{}, now: time.Now}
}

// Acquire checks the key's RPM and remaining TPM budget and, if allowed, counts the request.
// Limits ≤ 0 mean unlimited; ratios are the group multipliers (0 = 1).
// It returns the seconds to wait and which limit was hit, or 0 when allowed.
func (l *RateLimiter) Acquire(key string, rpm, tpm int, rpmRatio, tpmRatio float64) (retryAfter int, tier string) {
	l.mu.Lock()
	defer l.mu.Unlock()
	now := l.now()
	rpm, tpm = effective(rpm, rpmRatio), effective(tpm, tpmRatio)

	if t := current(l.tpm, key, now); tpm > 0 && t != nil && t.count >= tpm {
		return secondsUntil(t.resetAt, now), "tpm"
	}
	if r := current(l.rpm, key, now); rpm > 0 && r != nil && r.count >= rpm {
		return secondsUntil(r.resetAt, now), "rpm"
	}
	if rpm > 0 {
		add(l.rpm, key, 1, now)
	}
	return 0, ""
}

// RecordTokens counts tokens used by a finished request against the key's TPM window.
func (l *RateLimiter) RecordTokens(key string, tokens int) {
	if tokens <= 0 {
		return
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	add(l.tpm, key, tokens, l.now())
}

func current(m map[string]*counter, key string, now time.Time) *counter {
	if c := m[key]; c != nil && now.Before(c.resetAt) {
		return c
	}
	return nil
}

func add(m map[string]*counter, key string, n int, now time.Time) {
	if c := current(m, key, now); c != nil {
		c.count += n
		return
	}
	m[key] = &counter{count: n, resetAt: now.Add(window)}
}

func effective(limit int, ratio float64) int {
	if limit <= 0 {
		return 0
	}
	if ratio <= 0 {
		ratio = 1
	}
	return max(1, int(math.Round(float64(limit)*ratio)))
}

func secondsUntil(t, now time.Time) int {
	return max(1, int(math.Ceil(t.Sub(now).Seconds())))
}
