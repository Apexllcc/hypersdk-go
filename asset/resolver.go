// Package asset resolves Hyperliquid symbols into unambiguous asset metadata.
package asset

import (
	"context"
	"fmt"
	"strings"
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

// IDResolver resolves a protocol asset ID back to its market metadata. Asset
// IDs are the canonical representation used by Exchange actions, while a
// symbol can be ambiguous across spot, perpetual, and HIP-3 namespaces.
type IDResolver interface {
	ResolveID(context.Context, int) (Asset, error)
}

// ErrNotFound indicates an asset is absent from a resolver.
var ErrNotFound = fmt.Errorf("asset not found")

// StaticResolver is a concurrency-safe, deterministic resolver useful in tests and custom applications.
type StaticResolver struct {
	assets map[string][]Asset
	byID   map[int][]Asset
}

func NewStaticResolver(assets []Asset) *StaticResolver {
	m := make(map[string][]Asset, len(assets))
	byID := make(map[int][]Asset, len(assets))
	for _, a := range assets {
		m[a.Symbol] = append(m[a.Symbol], a)
		byID[a.ID] = append(byID[a.ID], a)
	}
	return &StaticResolver{assets: m, byID: byID}
}

// ResolveID resolves a unique protocol asset ID. Duplicate IDs are rejected
// rather than silently choosing an arbitrary market.
func (r *StaticResolver) ResolveID(ctx context.Context, id int) (Asset, error) {
	if err := ctx.Err(); err != nil {
		return Asset{}, err
	}
	assets := r.byID[id]
	if len(assets) == 0 {
		return Asset{}, fmt.Errorf("%w: %d", ErrNotFound, id)
	}
	if len(assets) != 1 {
		return Asset{}, fmt.Errorf("ambiguous asset ID: %d", id)
	}
	return assets[0], nil
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
	if err := validateMarketRef(ref); err != nil {
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
	if err := ctx.Err(); err != nil {
		return Asset{}, err
	}
	key := symbolCacheKey(symbol)
	r.mu.Lock()
	e, ok := r.entries[key]
	r.mu.Unlock()
	if ok && r.now().Before(e.expires) {
		return e.asset, nil
	}
	a, err := r.source.Resolve(ctx, symbol)
	if err != nil {
		return Asset{}, err
	}
	r.mu.Lock()
	r.entries[key] = cacheEntry{a, r.now().Add(r.ttl)}
	r.mu.Unlock()
	return a, nil
}

// ResolveMarket caches an unambiguous resolver lookup when the source supports it.
func (r *CachedResolver) ResolveMarket(ctx context.Context, ref types.MarketRef) (Asset, error) {
	if err := ctx.Err(); err != nil {
		return Asset{}, err
	}
	if err := validateMarketRef(ref); err != nil {
		return Asset{}, err
	}
	source, ok := r.source.(MarketResolver)
	if !ok {
		return Asset{}, fmt.Errorf("source does not support market references")
	}
	key := marketCacheKey(ref)
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
	r.entries[symbolCacheKey(symbol)] = cacheEntry{a, r.now().Add(r.ttl)}
	r.mu.Unlock()
	return a, nil
}

// RefreshMarket refreshes a market-reference cache entry immediately.
func (r *CachedResolver) RefreshMarket(ctx context.Context, ref types.MarketRef) (Asset, error) {
	if err := ctx.Err(); err != nil {
		return Asset{}, err
	}
	if err := validateMarketRef(ref); err != nil {
		return Asset{}, err
	}
	source, ok := r.source.(MarketResolver)
	if !ok {
		return Asset{}, fmt.Errorf("source does not support market references")
	}
	a, err := source.ResolveMarket(ctx, ref)
	if err != nil {
		return Asset{}, err
	}
	r.mu.Lock()
	r.entries[marketCacheKey(ref)] = cacheEntry{a, r.now().Add(r.ttl)}
	r.mu.Unlock()
	return a, nil
}

// ResolveID delegates to a source that supports reverse asset lookup. Reverse
// lookups are deliberately not cached separately: each cached Asset already
// has a unique ID and the source owns the canonical ID namespace.
func (r *CachedResolver) ResolveID(ctx context.Context, id int) (Asset, error) {
	source, ok := r.source.(IDResolver)
	if !ok {
		return Asset{}, fmt.Errorf("source does not support asset IDs")
	}
	return source.ResolveID(ctx, id)
}

func symbolCacheKey(symbol string) string { return "symbol:" + symbol }
func marketCacheKey(ref types.MarketRef) string {
	return "market:" + string(ref.Kind) + ":" + ref.DEX + ":" + ref.Symbol
}

func validateMarketRef(ref types.MarketRef) error {
	if ref.Symbol == "" {
		return fmt.Errorf("market symbol is required")
	}
	switch ref.Kind {
	case Perp, Spot:
		if ref.DEX != "" {
			return fmt.Errorf("%s market reference must not specify a DEX", ref.Kind)
		}
	case HIP3:
		if ref.DEX == "" {
			return fmt.Errorf("HIP-3 market reference requires a DEX")
		}
		prefix := ref.DEX + ":"
		if !strings.HasPrefix(ref.Symbol, prefix) || strings.TrimPrefix(ref.Symbol, prefix) == "" {
			return fmt.Errorf("HIP-3 market symbol must use %q namespace", ref.DEX+":")
		}
	default:
		return fmt.Errorf("unsupported market kind: %q", ref.Kind)
	}
	return nil
}
