package asset_test

import (
	"context"
	"testing"

	"github.com/Apexllcc/hyperliquid-go-sdk/asset"
	"github.com/Apexllcc/hyperliquid-go-sdk/types"
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
