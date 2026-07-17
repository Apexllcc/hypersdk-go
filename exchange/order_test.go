package exchange_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/Apexllcc/hypersdk-go/asset"
	"github.com/Apexllcc/hypersdk-go/exchange"
	"github.com/Apexllcc/hypersdk-go/nonce"
	"github.com/Apexllcc/hypersdk-go/signer"
	"github.com/Apexllcc/hypersdk-go/types"
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

func TestPlaceOrderAcceptsHIP3MarketRef(t *testing.T) {
	t.Parallel()
	local, err := signer.NewLocalPrivateKeySignerFromHex("0123456789012345678901234567890123456789012345678901234567890123")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = local.Close() }()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer func() { _ = r.Body.Close() }()
		var request struct {
			Action struct {
				Orders []struct {
					Asset int `json:"a"`
				} `json:"orders"`
			} `json:"action"`
		}
		if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
			t.Fatalf("decode signed HIP-3 order: %v", err)
		}
		if len(request.Action.Orders) != 1 || request.Action.Orders[0].Asset != 140001 {
			t.Fatalf("HIP-3 order asset = %#v, want asset 140001", request.Action.Orders)
		}
		_, _ = w.Write([]byte(`{"status":"ok","response":{"type":"order","data":{"statuses":[{"resting":{"oid":1}}]}}}`))
	}))
	defer server.Close()

	resolver := asset.NewStaticResolver([]asset.Asset{{
		ID: 140001, Symbol: "felix:TSLA", Name: "felix:TSLA", Kind: asset.HIP3, SzDecimals: 2, DEX: "felix",
	}})
	client := exchange.NewClient(server.URL+"/exchange", "testnet", nil, 0, local, nonce.NewMonotonicManager(nil), resolver, "test")
	market := types.MarketRef{Symbol: "felix:TSLA", Kind: types.HIP3, DEX: "felix"}
	response, err := client.PlaceOrder(context.Background(), exchange.OrderRequest{
		Market: &market, IsBuy: true,
		Price: decimal.RequireFromString("132.60"), Size: decimal.RequireFromString("0.08"),
		Type: exchange.LimitOrder{TimeInForce: exchange.TIFGTC},
	})
	if err != nil {
		t.Fatalf("place HIP-3 order: %v", err)
	}
	if response.Status != "ok" {
		t.Fatalf("HIP-3 order status = %q, want ok", response.Status)
	}
}

func TestPlaceOrderAcceptsOutcomeMarketRef(t *testing.T) {
	t.Parallel()
	local, err := signer.NewLocalPrivateKeySignerFromHex("0123456789012345678901234567890123456789012345678901234567890123")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = local.Close() }()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer func() { _ = r.Body.Close() }()
		var request struct {
			Action struct {
				Orders []struct {
					Asset int `json:"a"`
				} `json:"orders"`
			} `json:"action"`
		}
		if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
			t.Fatalf("decode signed outcome order: %v", err)
		}
		if len(request.Action.Orders) != 1 || request.Action.Orders[0].Asset != 100103950 {
			t.Fatalf("outcome order asset = %#v, want asset 100103950", request.Action.Orders)
		}
		_, _ = w.Write([]byte(`{"status":"ok","response":{"type":"order","data":{"statuses":[{"resting":{"oid":1}}]}}}`))
	}))
	defer server.Close()
	const outcomeKind = types.MarketKind("outcome")
	resolver := asset.NewStaticResolver([]asset.Asset{{ID: 100103950, Symbol: "#103950", Name: "+103950", Kind: outcomeKind, SzDecimals: 0}})
	client := exchange.NewClient(server.URL+"/exchange", "testnet", nil, 0, local, nonce.NewMonotonicManager(nil), resolver, "test")
	market := types.MarketRef{Symbol: "#103950", Kind: outcomeKind}
	response, err := client.PlaceOrder(context.Background(), exchange.OrderRequest{
		Market: &market, IsBuy: true, Price: decimal.RequireFromString("0.485"), Size: decimal.NewFromInt(21),
		Type: exchange.LimitOrder{TimeInForce: exchange.TIFGTC},
	})
	if err != nil {
		t.Fatalf("place outcome order: %v", err)
	}
	if response.Status != "ok" {
		t.Fatalf("outcome order status = %q, want ok", response.Status)
	}
}

func TestOutcomeUsesSpotPricePrecisionAndBuilderLimit(t *testing.T) {
	t.Parallel()
	local, err := signer.NewLocalPrivateKeySignerFromHex("0123456789012345678901234567890123456789012345678901234567890123")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = local.Close() }()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{"status":"ok","response":{"type":"order","data":{"statuses":[{"resting":{"oid":1}}]}}}`))
	}))
	defer server.Close()
	resolver := asset.NewStaticResolver([]asset.Asset{{ID: 100000010, Symbol: "#10", Kind: asset.Outcome, SzDecimals: 0}})
	client := exchange.NewClient(server.URL+"/exchange", "testnet", nil, 0, local, nonce.NewMonotonicManager(nil), resolver, "test")
	market := types.MarketRef{Symbol: "#10", Kind: types.Outcome}
	builder := common.HexToAddress("0x00000000000000000000000000000000000000b0")
	request := exchange.OrderRequest{
		Market: &market, IsBuy: true, Price: decimal.RequireFromString("0.00000001"), Size: decimal.NewFromInt(21),
		Type: exchange.LimitOrder{TimeInForce: exchange.TIFGTC}, Builder: &exchange.Builder{Address: builder, Fee: 1000},
	}
	if _, err := client.PlaceOrder(context.Background(), request); err != nil {
		t.Fatalf("place outcome order with spot precision/builder limit: %v", err)
	}
	request.Builder.Fee = 1001
	if _, err := client.PlaceOrder(context.Background(), request); err == nil {
		t.Fatal("outcome builder fee above 1000 accepted")
	}
}

func TestPlaceOrderAcceptsLargeIntegerPrice(t *testing.T) {
	t.Parallel()
	local, err := signer.NewLocalPrivateKeySignerFromHex("0123456789012345678901234567890123456789012345678901234567890123")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = local.Close() }()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{"status":"ok","response":{"type":"order","data":{"statuses":[{"resting":{"oid":1}}]}}}`))
	}))
	defer server.Close()
	client := exchange.NewClient(server.URL+"/exchange", "testnet", nil, 0, local, nonce.NewMonotonicManager(nil), asset.NewStaticResolver([]asset.Asset{{ID: 0, Symbol: "BTC", Kind: asset.Perp, SzDecimals: 5}}), "test")
	if _, err := client.PlaceOrder(context.Background(), exchange.OrderRequest{
		Coin: "BTC", IsBuy: true, Price: decimal.NewFromInt(123456), Size: decimal.RequireFromString("0.001"),
		Type: exchange.LimitOrder{TimeInForce: exchange.TIFGTC},
	}); err != nil {
		t.Fatalf("place large integer-priced order: %v", err)
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
