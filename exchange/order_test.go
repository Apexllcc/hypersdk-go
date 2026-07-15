package exchange_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/Apexllcc/hyperliquid-go-sdk/asset"
	"github.com/Apexllcc/hyperliquid-go-sdk/exchange"
	"github.com/Apexllcc/hyperliquid-go-sdk/nonce"
	"github.com/Apexllcc/hyperliquid-go-sdk/signer"
	"github.com/ethereum/go-ethereum/common"
	"github.com/shopspring/decimal"
)

func TestPlaceOrderSignsFinalDigestAndUsesDecimalWireValues(t *testing.T) {
	t.Parallel()
	local, err := signer.NewLocalPrivateKeySignerFromHex("0123456789012345678901234567890123456789012345678901234567890123")
	if err != nil {
		t.Fatal(err)
	}
	var signed bool
	mock := mockSigner{address: local.Address(), sign: func(ctx context.Context, d signer.Digest) (signer.Signature, error) {
		signed = true
		return local.SignDigest(ctx, d)
	}}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !signed {
			t.Fatal("request submitted before signing")
		}
		if r.URL.Path != "/exchange" {
			t.Fatalf("path=%s", r.URL.Path)
		}
		_, _ = w.Write([]byte(`{"status":"ok","response":{"type":"order","data":{"statuses":[{"resting":{"oid":1}}]}}}`))
	}))
	defer server.Close()
	resolver := asset.NewStaticResolver([]asset.Asset{{ID: 0, Symbol: "BTC", Kind: asset.Perp, SzDecimals: 5}})
	c := exchange.NewClient(server.URL+"/exchange", "mainnet", nil, 0, mock, nonce.NewMonotonicManager(nil), resolver, "test")
	result, err := c.PlaceOrder(context.Background(), exchange.OrderRequest{Coin: "BTC", IsBuy: true, Price: decimal.RequireFromString("60000.000"), Size: decimal.RequireFromString("0.00100"), Type: exchange.LimitOrder{TimeInForce: exchange.TIFGTC}})
	if err != nil {
		t.Fatal(err)
	}
	if result.Status != "ok" {
		t.Fatalf("status=%s", result.Status)
	}
}

type mockSigner struct {
	address common.Address
	sign    func(context.Context, signer.Digest) (signer.Signature, error)
}

func (m mockSigner) Address() common.Address { return m.address }
func (m mockSigner) SignDigest(ctx context.Context, d signer.Digest) (signer.Signature, error) {
	return m.sign(ctx, d)
}
