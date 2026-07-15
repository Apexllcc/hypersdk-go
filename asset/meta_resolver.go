package asset

import (
	"context"
	"fmt"
	"github.com/Apexllcc/hyperliquid-go-sdk/info"
	"github.com/Apexllcc/hyperliquid-go-sdk/types"
	"sync"
)

// MetaResolver loads official Perp and Spot metadata on first use.
type MetaResolver struct {
	info    *info.Client
	once    sync.Once
	loadErr error
	assets  []Asset
}

func NewMetaResolver(client *info.Client) (*MetaResolver, error) {
	if client == nil {
		return nil, fmt.Errorf("nil info client")
	}
	return &MetaResolver{info: client}, nil
}
func (r *MetaResolver) load(ctx context.Context) {
	r.once.Do(func() {
		meta, err := r.info.Meta(ctx)
		if err != nil {
			r.loadErr = err
			return
		}
		for id, item := range meta.Universe {
			r.assets = append(r.assets, Asset{ID: id, Symbol: item.Name, Name: item.Name, Kind: Perp, SzDecimals: item.SzDecimals})
		}
		spot, err := r.info.SpotMeta(ctx)
		if err != nil {
			r.loadErr = err
			return
		}
		for _, pair := range spot.Universe {
			if len(pair.Tokens) != 2 {
				continue
			}
			base := spot.Tokens[pair.Tokens[0]]
			r.assets = append(r.assets, Asset{ID: 10000 + pair.Index, Symbol: pair.Name, Name: pair.Name, Kind: Spot, SzDecimals: base.SzDecimals})
		}
	})
}
func (r *MetaResolver) Resolve(ctx context.Context, symbol string) (Asset, error) {
	r.load(ctx)
	if r.loadErr != nil {
		return Asset{}, r.loadErr
	}
	var found []Asset
	for _, a := range r.assets {
		if a.Symbol == symbol {
			found = append(found, a)
		}
	}
	if len(found) == 0 {
		return Asset{}, fmt.Errorf("%w: %s", ErrNotFound, symbol)
	}
	if len(found) != 1 {
		return Asset{}, fmt.Errorf("ambiguous asset symbol: %s", symbol)
	}
	return found[0], nil
}
func (r *MetaResolver) ResolveMarket(ctx context.Context, ref types.MarketRef) (Asset, error) {
	r.load(ctx)
	if r.loadErr != nil {
		return Asset{}, r.loadErr
	}
	for _, a := range r.assets {
		if a.Symbol == ref.Symbol && a.Kind == ref.Kind && a.DEX == ref.DEX {
			return a, nil
		}
	}
	return Asset{}, fmt.Errorf("%w: %s", ErrNotFound, ref.Symbol)
}
