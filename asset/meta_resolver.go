package asset

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/Apexllcc/hypersdk-go/info"
	"github.com/Apexllcc/hypersdk-go/types"
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

// WithOutcomeMetadata includes Testnet outcome markets in the metadata
// snapshot. The official outcomeMeta endpoint is Testnet-only, so callers on
// other networks must not enable this option.
func WithOutcomeMetadata() MetaResolverOption {
	return func(r *MetaResolver) error {
		r.outcomes = true
		return nil
	}
}

// MetaResolver loads official Perp, Spot, and HIP-3 metadata into a coherent
// immutable snapshot. Concurrent callers coalesce into one network refresh.
type MetaResolver struct {
	info     *info.Client
	ttl      time.Duration
	now      func() time.Time
	outcomes bool

	mu       sync.Mutex
	assets   []Asset
	bySymbol map[string][]Asset
	byID     map[int][]Asset
	expires  time.Time
	loading  *metaLoad
}

// metaLoad carries one coalesced metadata refresh result to every caller that
// joined it. Keeping the result alongside the completion signal is important:
// an explicit Refresh must not turn another caller's failed refresh into a
// silent success by falling back to an older snapshot.
type metaLoad struct {
	done     chan struct{}
	snapshot metaSnapshot
	err      error
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
	if err := ctx.Err(); err != nil {
		return Asset{}, err
	}
	if err := validateMarketRef(ref); err != nil {
		return Asset{}, err
	}
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
	if err := ctx.Err(); err != nil {
		return Asset{}, err
	}
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
	if err := ctx.Err(); err != nil {
		return metaSnapshot{}, err
	}
	r.mu.Lock()
	if !force && r.snapshotValidLocked() {
		s := r.currentLocked()
		r.mu.Unlock()
		return s, nil
	}
	if load := r.loading; load != nil {
		r.mu.Unlock()
		select {
		case <-ctx.Done():
			return metaSnapshot{}, ctx.Err()
		case <-load.done:
			if load.err == nil {
				return load.snapshot, nil
			}
			// An ordinary read may continue using an unexpired snapshot after
			// another caller's explicit refresh failed. Explicit refresh callers,
			// and callers with no valid snapshot, must receive the shared error.
			if !force {
				r.mu.Lock()
				if r.snapshotValidLocked() {
					s := r.currentLocked()
					r.mu.Unlock()
					return s, nil
				}
				r.mu.Unlock()
			}
			return metaSnapshot{}, load.err
		}
	}
	load := &metaLoad{done: make(chan struct{})}
	r.loading = load
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
		s = r.currentLocked()
	}
	load.snapshot, load.err = s, err
	r.loading = nil
	close(load.done)
	r.mu.Unlock()
	return s, err
}

func (r *MetaResolver) snapshotValidLocked() bool {
	return len(r.assets) != 0 && (r.ttl == 0 || r.now().Before(r.expires))
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
	var outcomeMeta info.OutcomeMetaResponse
	if r.outcomes {
		outcomeMeta, err = r.info.OutcomeMeta(ctx)
		if err != nil {
			return metaSnapshot{}, fmt.Errorf("load outcome metadata: %w", err)
		}
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
	if r.outcomes {
		assets = appendOutcomes(assets, outcomeMeta)
	}
	s := indexAssets(assets)
	for alias, aliasAssets := range aliases {
		s.bySymbol[alias] = append(s.bySymbol[alias], aliasAssets...)
	}
	return s, nil
}

func appendOutcomes(assets []Asset, meta info.OutcomeMetaResponse) []Asset {
	for _, outcome := range meta.Outcomes {
		if outcome.Outcome < 0 || len(outcome.SideSpecs) != 2 {
			continue
		}
		for side := 0; side < 2; side++ {
			encoding := outcome.Outcome*10 + side
			assets = append(assets, Asset{
				ID: 100000000 + encoding, Symbol: fmt.Sprintf("#%d", encoding), Name: fmt.Sprintf("+%d", encoding),
				Kind: Outcome, SzDecimals: 0,
			})
		}
	}
	return assets
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
