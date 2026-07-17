package asset_test

import (
	"context"
	"testing"

	"github.com/Apexllcc/hypersdk-go/asset"
	"github.com/Apexllcc/hypersdk-go/types"
)

func TestStaticResolverRejectsAmbiguousSymbolAndResolvesMarketRef(t *testing.T) {
	t.Parallel()
	r := asset.NewStaticResolver([]asset.Asset{{ID: 0, Symbol: "BTC", Kind: asset.Perp}, {ID: 10000, Symbol: "BTC", Kind: asset.Spot}})
	if _, err := r.Resolve(context.Background(), "BTC"); err == nil {
		t.Fatal("ambiguous symbol must not resolve")
	}
	got, err := r.ResolveMarket(context.Background(), types.MarketRef{Symbol: "BTC", Kind: types.Spot})
	if err != nil {
		t.Fatal(err)
	}
	if got.ID != 10000 {
		t.Fatalf("asset id = %d", got.ID)
	}
}

func TestStaticResolverRejectsUnqualifiedHIP3MarketRef(t *testing.T) {
	t.Parallel()
	r := asset.NewStaticResolver([]asset.Asset{{ID: 110000, Symbol: "ABC", Kind: asset.HIP3, DEX: "test"}})
	if _, err := r.ResolveMarket(context.Background(), types.MarketRef{Symbol: "ABC", Kind: types.HIP3, DEX: "test"}); err == nil {
		t.Fatal("unqualified HIP-3 symbol resolved")
	}
}

func TestStaticResolverRejectsHIP3MarketRefWithoutCoin(t *testing.T) {
	t.Parallel()
	r := asset.NewStaticResolver([]asset.Asset{{ID: 110000, Symbol: "test:", Kind: asset.HIP3, DEX: "test"}})
	if _, err := r.ResolveMarket(context.Background(), types.MarketRef{Symbol: "test:", Kind: types.HIP3, DEX: "test"}); err == nil {
		t.Fatal("HIP-3 market reference without a coin resolved")
	}
}
