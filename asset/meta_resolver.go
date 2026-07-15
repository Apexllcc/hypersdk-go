package asset

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/Apexllcc/hyperliquid-go-sdk/info"
	"github.com/Apexllcc/hyperliquid-go-sdk/types"
)

const defaultMetaRefreshTTL = 5 * time.Minute

// MetaResolverOption configures a metadata-backed asset resolver.
type MetaResolverOption func(*MetaResolver) error

// WithMetaRefreshTTL controls how long a successful metadata snapshot remains
// valid. A zero TTL disables automatic expiry; callers can still call Refresh.
func WithMetaRefreshTTL(ttl time.Duration) MetaResolverOption {
	return func(r *MetaResolver) error {
		if ttl < 0 {
			return fmt.Errorf("invalid metadata refresh TTL")
		}
		r.ttl = ttl
		return nil
	}
}

// MetaResolver loads official Perp, Spot, and HIP-3 metadata into a coherent
// immutable snapshot. Concurrent callers coalesce into one network refresh.
type MetaResolver struct {
	info *info.Client
	ttl  time.Duration
	now  func() time.Time

	mu       sync.Mutex
	assets   []Asset
	bySymbol map[string][]Asset
	byID     map[int][]Asset
	expires  time.Time
	loading  chan struct{}
}

func NewMetaResolver(client *info.Client, options ...MetaResolverOption) (*MetaResolver, error) {
	if client == nil {
		return nil, fmt.Errorf("nil info client")
	}
	r := &MetaResolver{info: client, ttl: defaultMetaRefreshTTL, now: time.Now}
	for _, option := range options {
		if option == nil {
			return nil, fmt.Errorf("nil metadata resolver option")
		}
		if err := option(r); err != nil {
			return nil, err
		}
	}
	return r, nil
}

// Refresh atomically replaces the cached metadata snapshot. If a refresh
// fails, the last successful snapshot remains intact and a later call retries.
func (r *MetaResolver) Refresh(ctx context.Context) error {
	_, err := r.snapshot(ctx, true)
	return err
}

func (r *MetaResolver) Resolve(ctx context.Context, symbol string) (Asset, error) {
	s, err := r.snapshot(ctx, false)
	if err != nil {
		return Asset{}, err
	}
	assets := s.bySymbol[symbol]
	if len(assets) == 0 {
		return Asset{}, fmt.Errorf("%w: %s", ErrNotFound, symbol)
	}
	if len(assets) != 1 {
		return Asset{}, fmt.Errorf("ambiguous asset symbol: %s", symbol)
	}
	return assets[0], nil
}

func (r *MetaResolver) ResolveMarket(ctx context.Context, ref types.MarketRef) (Asset, error) {
	s, err := r.snapshot(ctx, false)
	if err != nil {
		return Asset{}, err
	}
	for _, a := range s.bySymbol[ref.Symbol] {
		if a.Kind == ref.Kind && a.DEX == ref.DEX {
			return a, nil
		}
	}
	return Asset{}, fmt.Errorf("%w: %s", ErrNotFound, ref.Symbol)
}

// ResolveID returns the unique market associated with a protocol asset ID.
func (r *MetaResolver) ResolveID(ctx context.Context, id int) (Asset, error) {
	s, err := r.snapshot(ctx, false)
	if err != nil {
		return Asset{}, err
	}
	assets := s.byID[id]
	if len(assets) == 0 {
		return Asset{}, fmt.Errorf("%w: %d", ErrNotFound, id)
	}
	if len(assets) != 1 {
		return Asset{}, fmt.Errorf("ambiguous asset ID: %d", id)
	}
	return assets[0], nil
}

type metaSnapshot struct {
	assets   []Asset
	bySymbol map[string][]Asset
	byID     map[int][]Asset
}

func (r *MetaResolver) snapshot(ctx context.Context, force bool) (metaSnapshot, error) {
	for {
		r.mu.Lock()
		if len(r.assets) != 0 && !force && (r.ttl == 0 || r.now().Before(r.expires)) {
			s := r.currentLocked()
			r.mu.Unlock()
			return s, nil
		}
		if r.loading != nil {
			wait := r.loading
			r.mu.Unlock()
			select {
			case <-ctx.Done():
				return metaSnapshot{}, ctx.Err()
			case <-wait:
				// A completed refresh may satisfy this call; loop to re-check TTL.
				force = false
				continue
			}
		}
		wait := make(chan struct{})
		r.loading = wait
		r.mu.Unlock()

		s, err := r.fetch(ctx)

		r.mu.Lock()
		if err == nil {
			r.assets, r.bySymbol, r.byID = s.assets, s.bySymbol, s.byID
			if r.ttl > 0 {
				r.expires = r.now().Add(r.ttl)
			} else {
				r.expires = time.Time{}
			}
		}
		r.loading = nil
		close(wait)
		if err == nil {
			s = r.currentLocked()
		}
		r.mu.Unlock()
		return s, err
	}
}

func (r *MetaResolver) currentLocked() metaSnapshot {
	return metaSnapshot{assets: r.assets, bySymbol: r.bySymbol, byID: r.byID}
}

func (r *MetaResolver) fetch(ctx context.Context) (metaSnapshot, error) {
	baseMeta, err := r.info.Meta(ctx)
	if err != nil {
		return metaSnapshot{}, fmt.Errorf("load perpetual metadata: %w", err)
	}
	spot, err := r.info.SpotMeta(ctx)
	if err != nil {
		return metaSnapshot{}, fmt.Errorf("load spot metadata: %w", err)
	}
	dexes, err := r.info.PerpDEXs(ctx)
	if err != nil {
		return metaSnapshot{}, fmt.Errorf("load perpetual DEX metadata: %w", err)
	}

	assets := appendPerps(nil, baseMeta, "", 0, Perp)
	for index, dex := range dexes {
		if dex == nil || dex.Name == "" {
			continue
		}
		meta, err := r.info.MetaForDEX(ctx, dex.Name)
		if err != nil {
			return metaSnapshot{}, fmt.Errorf("load HIP-3 metadata for %q: %w", dex.Name, err)
		}
		// Official IDs use 100000 + perp_dex_index*10000 + index_in_meta.
		assets = appendPerps(assets, meta, dex.Name, 100000+index*10000, HIP3)
	}

	tokens := make(map[int]info.SpotToken, len(spot.Tokens))
	for _, token := range spot.Tokens {
		tokens[token.Index] = token
	}
	aliases := make(map[string][]Asset)
	for _, pair := range spot.Universe {
		base, ok := tokens[pair.Tokens[0]]
		if !ok {
			return metaSnapshot{}, fmt.Errorf("spot pair %q has unknown base token index %d", pair.Name, pair.Tokens[0])
		}
		quote, ok := tokens[pair.Tokens[1]]
		if !ok {
			return metaSnapshot{}, fmt.Errorf("spot pair %q has unknown quote token index %d", pair.Name, pair.Tokens[1])
		}
		asset := Asset{ID: 10000 + pair.Index, Symbol: pair.Name, Name: pair.Name, Kind: Spot, SzDecimals: base.SzDecimals}
		assets = append(assets, asset)
		// spotMeta can use an internal coin name (for example "@107"),
		// whereas callers commonly use the human-readable BASE/QUOTE name.
		alias := base.Name + "/" + quote.Name
		if alias != pair.Name {
			aliases[alias] = append(aliases[alias], asset)
		}
	}
	s := indexAssets(assets)
	for alias, aliasAssets := range aliases {
		s.bySymbol[alias] = append(s.bySymbol[alias], aliasAssets...)
	}
	return s, nil
}

func appendPerps(assets []Asset, meta info.MetaResponse, dex string, offset int, kind Kind) []Asset {
	for index, item := range meta.Universe {
		assets = append(assets, Asset{ID: offset + index, Symbol: item.Name, Name: item.Name, Kind: kind, SzDecimals: item.SzDecimals, DEX: dex})
	}
	return assets
}

func indexAssets(assets []Asset) metaSnapshot {
	bySymbol := make(map[string][]Asset, len(assets))
	byID := make(map[int][]Asset, len(assets))
	for _, a := range assets {
		bySymbol[a.Symbol] = append(bySymbol[a.Symbol], a)
		byID[a.ID] = append(byID[a.ID], a)
	}
	return metaSnapshot{assets: assets, bySymbol: bySymbol, byID: byID}
}
