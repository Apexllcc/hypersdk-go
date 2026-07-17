package exchange_test

import (
	"context"
	"errors"
	"net/http"
	"sync/atomic"
	"testing"

	"github.com/Apexllcc/hypersdk-go/asset"
	"github.com/Apexllcc/hypersdk-go/exchange"
	"github.com/Apexllcc/hypersdk-go/nonce"
	"github.com/Apexllcc/hypersdk-go/signer"
	"github.com/Apexllcc/hypersdk-go/transport"
	"github.com/shopspring/decimal"
)

type failingExchangeTransport struct{ calls atomic.Int32 }

func (t *failingExchangeTransport) Do(context.Context, *http.Request) (*http.Response, error) {
	t.calls.Add(1)
	return nil, errors.New("network unavailable")
}

func TestPlaceOrderMakesExactlyOneAttemptOnNetworkFailure(t *testing.T) {
	local, err := signer.NewLocalPrivateKeySignerFromHex("0123456789012345678901234567890123456789012345678901234567890123")
	if err != nil {
		t.Fatal(err)
	}
	failed := &failingExchangeTransport{}
	var httpTransport transport.HTTPTransport = failed
	client := exchange.NewClient("https://exchange.invalid", "mainnet", httpTransport, 0, local, nonce.NewMonotonicManager(nil), asset.NewStaticResolver([]asset.Asset{{ID: 0, Symbol: "BTC", Kind: asset.Perp, SzDecimals: 5}}), "test")
	_, err = client.PlaceOrder(context.Background(), exchange.OrderRequest{Coin: "BTC", IsBuy: true, Price: decimal.RequireFromString("100"), Size: decimal.RequireFromString("1"), Type: exchange.LimitOrder{TimeInForce: exchange.TIFGTC}})
	if err == nil {
		t.Fatal("expected network failure")
	}
	if got := failed.calls.Load(); got != 1 {
		t.Fatalf("Exchange HTTP attempts = %d, want 1", got)
	}
}
