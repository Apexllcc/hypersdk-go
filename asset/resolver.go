// Package asset resolves Hyperliquid symbols into unambiguous asset metadata.
package asset

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/Apexllcc/hyperliquid-go-sdk/types"
)

// Kind differentiates perpetual, spot, and HIP-3 assets.
type Kind = types.MarketKind

const (
	Perp = types.Perpetual
	Spot = types.Spot
	HIP3 = types.HIP3
)

// Asset is the metadata required to construct an order wire object.
type Asset = types.Asset

// Resolver resolves an exact, caller-supplied asset symbol.
type Resolver interface {
	Resolve(context.Context, string) (Asset, error)
}

// MarketResolver resolves an explicit market namespace.
type MarketResolver interface {
	ResolveMarket(context.Context, types.MarketRef) (Asset, error)
}

// ErrNotFound indicates an asset is absent from a resolver.
var ErrNotFound = fmt.Errorf("asset not found")

// StaticResolver is a concurrency-safe, deterministic resolver useful in tests and custom applications.
type StaticResolver struct{ assets map[string][]Asset }

func NewStaticResolver(assets []Asset) *StaticResolver {
	m := make(map[string][]Asset, len(assets))
	for _, a := range assets {
		m[a.Symbol] = append(m[a.Symbol], a)
	}
	return &StaticResolver{m}
}
func (r *StaticResolver) Resolve(ctx context.Context, symbol string) (Asset, error) {
	if err := ctx.Err(); err != nil {
		return Asset{}, err
	}
	assets, ok := r.assets[symbol]
	if !ok {
		return Asset{}, fmt.Errorf("%w: %s", ErrNotFound, symbol)
	}
	if len(assets) != 1 {
		return Asset{}, fmt.Errorf("ambiguous asset symbol: %s", symbol)
	}
	return assets[0], nil
}

func (r *StaticResolver) ResolveMarket(ctx context.Context, ref types.MarketRef) (Asset, error) {
	if err := ctx.Err(); err != nil {
		return Asset{}, err
	}
	for _, a := range r.assets[ref.Symbol] {
		if a.Kind == ref.Kind && a.DEX == ref.DEX {
			return a, nil
		}
	}
	return Asset{}, fmt.Errorf("%w: %s", ErrNotFound, ref.Symbol)
}

// CachedResolver caches successful resolutions for a bounded TTL.
type CachedResolver struct {
	source  Resolver
	ttl     time.Duration
	now     func() time.Time
	mu      sync.Mutex
	entries map[string]cacheEntry
}
type cacheEntry struct {
	asset   Asset
	expires time.Time
}

func NewCachedResolver(source Resolver, ttl time.Duration) (*CachedResolver, error) {
	if source == nil {
		return nil, fmt.Errorf("nil resolver")
	}
	if ttl <= 0 {
		return nil, fmt.Errorf("invalid cache TTL")
	}
	return &CachedResolver{source: source, ttl: ttl, now: time.Now, entries: make(map[string]cacheEntry)}, nil
}
func (r *CachedResolver) Resolve(ctx context.Context, symbol string) (Asset, error) {
	r.mu.Lock()
	e, ok := r.entries[symbol]
	r.mu.Unlock()
	if ok && r.now().Before(e.expires) {
		return e.asset, nil
	}
	a, err := r.source.Resolve(ctx, symbol)
	if err != nil {
		return Asset{}, err
	}
	r.mu.Lock()
	r.entries[symbol] = cacheEntry{a, r.now().Add(r.ttl)}
	r.mu.Unlock()
	return a, nil
}

// ResolveMarket caches an unambiguous resolver lookup when the source supports it.
func (r *CachedResolver) ResolveMarket(ctx context.Context, ref types.MarketRef) (Asset, error) {
	source, ok := r.source.(MarketResolver)
	if !ok {
		return Asset{}, fmt.Errorf("source does not support market references")
	}
	key := string(ref.Kind) + ":" + ref.DEX + ":" + ref.Symbol
	r.mu.Lock()
	entry, found := r.entries[key]
	r.mu.Unlock()
	if found && r.now().Before(entry.expires) {
		return entry.asset, nil
	}
	a, err := source.ResolveMarket(ctx, ref)
	if err != nil {
		return Asset{}, err
	}
	r.mu.Lock()
	r.entries[key] = cacheEntry{a, r.now().Add(r.ttl)}
	r.mu.Unlock()
	return a, nil
}

// Refresh refreshes a cached asset immediately.
func (r *CachedResolver) Refresh(ctx context.Context, symbol string) (Asset, error) {
	a, err := r.source.Resolve(ctx, symbol)
	if err != nil {
		return Asset{}, err
	}
	r.mu.Lock()
	r.entries[symbol] = cacheEntry{a, r.now().Add(r.ttl)}
	r.mu.Unlock()
	return a, nil
}
