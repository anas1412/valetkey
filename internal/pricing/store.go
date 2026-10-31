package pricing

import (
	"bytes"
	"compress/gzip"
	"context"
	_ "embed"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"sync"
	"time"

	"github.com/anas1412/valetkey/internal/types"
)

// snapshot is the catalogue as fetched when this binary was built; it is the
// fallback for clusters without internet access (regenerate: go run ./hack/pricing-snapshot).
//
//go:embed snapshot.json.gz
var snapshot []byte

// Persister keeps the last good catalogue across restarts (a ConfigMap in-cluster).
type Persister interface {
	Load(ctx context.Context) ([]byte, error)
	Save(ctx context.Context, data []byte) error
}

// Store serves the current catalogue and refreshes it in the background.
type Store struct {
	Fetcher  Fetcher
	Interval time.Duration  // 0 = never fetch: snapshot (or saved copy) plus overrides only
	Persist  Persister      // optional
	OnChange func(*Catalog) // optional, called with every new catalogue

	mu  sync.RWMutex
	cat *Catalog
}

// Start loads the saved or built-in catalogue right away, then keeps it fresh.
func (s *Store) Start(ctx context.Context) {
	s.set(s.initial(ctx))
	if s.Interval > 0 {
		go s.loop(ctx)
	}
}

func (s *Store) initial(ctx context.Context) *Catalog {
	if s.Persist != nil {
		if data, err := s.Persist.Load(ctx); err == nil && len(data) > 0 {
			if cat, err := Decode(data); err == nil {
				return cat
			}
		}
	}
	cat, err := Decode(snapshot)
	if err != nil {
		slog.Error("built-in price snapshot is unreadable", "err", err)
		return &Catalog{}
	}
	return cat
}

func (s *Store) loop(ctx context.Context) {
	for {
		s.Refresh(ctx)
		select {
		case <-ctx.Done():
			return
		case <-time.After(s.Interval):
		}
	}
}

// Refresh fetches every source now. On failure the current catalogue stays.
func (s *Store) Refresh(ctx context.Context) {
	cat, err := s.Fetcher.Fetch(ctx, time.Now())
	if err != nil {
		slog.Warn("price refresh failed, keeping the current prices", "err", err)
		return
	}
	s.set(cat)
	slog.Info("prices refreshed", "providers", len(cat.Providers))
	if s.Persist != nil {
		if data, err := Encode(cat); err == nil {
			if err := s.Persist.Save(ctx, data); err != nil {
				slog.Warn("could not save prices", "err", err)
			}
		}
	}
}

func (s *Store) set(c *Catalog) {
	s.mu.Lock()
	s.cat = c
	s.mu.Unlock()
	if s.OnChange != nil {
		s.OnChange(c)
	}
}

// Catalog is the current catalogue (never nil after Start).
func (s *Store) Catalog() *Catalog {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if s.cat == nil {
		return &Catalog{}
	}
	return s.cat
}

// Resolve prices a model on a channel: the channel's own override first, then the
// catalogue entry for the channel's provider. model is the upstream model name.
func (s *Store) Resolve(spec types.ChannelSpec, model string) (Price, bool) {
	if spec.Pricing != nil {
		if mp, ok := spec.Pricing.Models[model]; ok {
			p, err := overridePrice(mp)
			if err != nil {
				return Price{}, false
			}
			return p, true
		}
	}
	cat := s.Catalog()
	provider := ""
	if spec.Pricing != nil {
		provider = spec.Pricing.Provider
	}
	if provider == "" {
		provider = cat.ProviderFor(spec.BaseURL)
	}
	if provider == "" {
		return Price{}, false
	}
	return cat.Lookup(provider, model)
}

func overridePrice(mp types.ModelPrice) (Price, error) {
	in, err := PerMillionString(mp.Input)
	if err != nil {
		return Price{}, err
	}
	out, err := PerMillionString(mp.Output)
	if err != nil {
		return Price{}, err
	}
	r := Rates{Input: in, Output: out, CacheRead: in, CacheWrite5m: in, CacheWrite1h: in}
	for _, f := range []struct {
		s   string
		dst *USD
	}{{mp.CacheRead, &r.CacheRead}, {mp.CacheWrite, &r.CacheWrite5m}, {mp.CacheWrite1h, &r.CacheWrite1h}} {
		if f.s == "" {
			continue
		}
		if *f.dst, err = PerMillionString(f.s); err != nil {
			return Price{}, err
		}
	}
	if mp.CacheWrite != "" && mp.CacheWrite1h == "" {
		r.CacheWrite1h = r.CacheWrite5m
	}
	return Price{Rates: r, Source: "channel override"}, nil
}

// ValidateOverrides reports the first unreadable price in a channel's overrides.
func ValidateOverrides(p *types.ChannelPricing) error {
	if p == nil {
		return nil
	}
	for model, mp := range p.Models {
		if _, err := overridePrice(mp); err != nil {
			return fmt.Errorf("pricing for %q: %w", model, err)
		}
	}
	return nil
}

// Encode and Decode store a catalogue as gzipped JSON.
func Encode(c *Catalog) ([]byte, error) {
	var buf bytes.Buffer
	zw, _ := gzip.NewWriterLevel(&buf, gzip.BestCompression)
	if err := json.NewEncoder(zw).Encode(c); err != nil {
		return nil, err
	}
	if err := zw.Close(); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

func Decode(data []byte) (*Catalog, error) {
	zr, err := gzip.NewReader(bytes.NewReader(data))
	if err != nil {
		return nil, err
	}
	raw, err := io.ReadAll(zr)
	if err != nil {
		return nil, err
	}
	var c Catalog
	return &c, json.Unmarshal(raw, &c)
}
