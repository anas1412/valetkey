// Package stats keeps in-memory usage history for the dashboard. No database:
// everything resets when the pod restarts. Prompt and response content is never stored.
package stats

import (
	"sync"
	"time"
)

type RequestEntry struct {
	Time         int64  `json:"time"` // epoch ms
	Namespace    string `json:"namespace"`
	Key          string `json:"key"`
	Group        string `json:"group"`
	Channel      string `json:"channel"`
	Model        string `json:"model"`
	Status       int    `json:"status"`
	InputTokens  int    `json:"inputTokens"`
	OutputTokens int    `json:"outputTokens"`
	LatencyMs    int64  `json:"latencyMs"`
	Error        string `json:"error,omitempty"`
	// CostUSD is the request's cost; CostNote is "estimated" (no usage reported,
	// worst case charged) or "unpriced" (no known price).
	CostUSD  float64 `json:"costUSD"`
	CostNote string  `json:"costNote,omitempty"`
}

type Event struct {
	Time    int64  `json:"time"`
	Level   string `json:"level"` // info | warn | error
	Kind    string `json:"kind"`
	Message string `json:"message"`
}

type Bucket struct {
	Minute       int64   `json:"minute"` // epoch minutes
	Requests     int     `json:"requests"`
	Errors       int     `json:"errors"`
	InputTokens  int     `json:"inputTokens"`
	OutputTokens int     `json:"outputTokens"`
	Cost         float64 `json:"cost"` // US dollars
}

const (
	maxRequests = 500
	maxEvents   = 200
	minutes     = 24 * 60
)

type KeyUsage struct {
	Requests int    `json:"requests"`
	LastUsed *int64 `json:"lastUsed,omitempty"`
}

type Stats struct {
	StartedAt int64

	mu       sync.Mutex
	requests []RequestEntry
	events   []Event
	buckets  map[int64]*Bucket
	keys     map[string]*KeyUsage // "ns/name"
}

func New() *Stats {
	return &Stats{StartedAt: time.Now().UnixMilli(), buckets: map[int64]*Bucket{}, keys: map[string]*KeyUsage{}}
}

func (s *Stats) RecordRequest(e RequestEntry) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.requests = push(s.requests, e, maxRequests)

	id := e.Namespace + "/" + e.Key
	u := s.keys[id]
	if u == nil {
		u = &KeyUsage{}
		s.keys[id] = u
	}
	u.Requests++
	t := e.Time
	u.LastUsed = &t

	minute := e.Time / 60_000
	b := s.buckets[minute]
	if b == nil {
		b = &Bucket{Minute: minute}
		s.buckets[minute] = b
		for m := range s.buckets {
			if m <= minute-minutes {
				delete(s.buckets, m)
			}
		}
	}
	b.Requests++
	if e.Status >= 400 {
		b.Errors++
	}
	b.InputTokens += e.InputTokens
	b.OutputTokens += e.OutputTokens
	b.Cost += e.CostUSD
}

func (s *Stats) Event(level, kind, message string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.events = push(s.events, Event{Time: time.Now().UnixMilli(), Level: level, Kind: kind, Message: message}, maxEvents)
}

// Requests returns the request log, newest first.
func (s *Stats) Requests() []RequestEntry {
	s.mu.Lock()
	defer s.mu.Unlock()
	return reversed(s.requests)
}

// Events returns operator events, newest first.
func (s *Stats) Events() []Event {
	s.mu.Lock()
	defer s.mu.Unlock()
	return reversed(s.events)
}

// Series returns per-step buckets for the last 24 h, oldest first, empty steps filled in.
func (s *Stats) Series(step int) []Bucket {
	s.mu.Lock()
	defer s.mu.Unlock()
	now := time.Now().UnixMilli() / 60_000
	out := make([]Bucket, 0, minutes/step)
	for start := now - minutes + 1; start <= now; start += int64(step) {
		agg := Bucket{Minute: start}
		for m := start; m < start+int64(step) && m <= now; m++ {
			if b := s.buckets[m]; b != nil {
				agg.Requests += b.Requests
				agg.Errors += b.Errors
				agg.InputTokens += b.InputTokens
				agg.OutputTokens += b.OutputTokens
				agg.Cost += b.Cost
			}
		}
		out = append(out, agg)
	}
	return out
}

func (s *Stats) KeyUsage(namespace, name string) KeyUsage {
	s.mu.Lock()
	defer s.mu.Unlock()
	if u := s.keys[namespace+"/"+name]; u != nil {
		return *u
	}
	return KeyUsage{}
}

// ChannelShare counts requests per channel over the kept request log.
func (s *Stats) ChannelShare() map[string]int {
	s.mu.Lock()
	defer s.mu.Unlock()
	share := map[string]int{}
	for _, r := range s.requests {
		if r.Channel != "none" {
			share[r.Channel]++
		}
	}
	return share
}

func push[T any](list []T, item T, max int) []T {
	list = append(list, item)
	if len(list) > max {
		list = list[len(list)-max:]
	}
	return list
}

func reversed[T any](list []T) []T {
	out := make([]T, len(list))
	for i, v := range list {
		out[len(list)-1-i] = v
	}
	return out
}
