package exchange_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/Apexllcc/hyperliquid-go-sdk/asset"
	"github.com/Apexllcc/hyperliquid-go-sdk/exchange"
	"github.com/Apexllcc/hyperliquid-go-sdk/nonce"
	"github.com/Apexllcc/hyperliquid-go-sdk/signer"
	"github.com/Apexllcc/hyperliquid-go-sdk/types"
	"github.com/ethereum/go-ethereum/common"
	"github.com/shopspring/decimal"
)

func TestPlaceTriggerOrderSendsCanonicalTriggerVaultExpiryAndBuilder(t *testing.T) {
	local, err := signer.NewLocalPrivateKeySignerFromHex("0123456789012345678901234567890123456789012345678901234567890123")
	if err != nil {
		t.Fatal(err)
	}
	vault := common.HexToAddress("0x1111111111111111111111111111111111111111")
	expiresAfter := uint64(1700001234567)
	builder := common.HexToAddress("0x2222222222222222222222222222222222222222")
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var raw json.RawMessage
		if err := json.NewDecoder(r.Body).Decode(&raw); err != nil {
			t.Fatal(err)
		}
		var got struct {
			Action struct {
				Orders []struct {
					Type map[string]struct {
						IsMarket  bool   `json:"isMarket"`
						TriggerPx string `json:"triggerPx"`
						TPSL      string `json:"tpsl"`
					} `json:"t"`
				} `json:"orders"`
				Builder *struct {
					Address common.Address `json:"b"`
					Fee     uint64         `json:"f"`
				} `json:"builder"`
			} `json:"action"`
			VaultAddress common.Address `json:"vaultAddress"`
			ExpiresAfter uint64         `json:"expiresAfter"`
		}
		if err := json.Unmarshal(raw, &got); err != nil {
			t.Fatal(err)
		}
		trigger := got.Action.Orders[0].Type["trigger"]
		if !trigger.IsMarket || trigger.TriggerPx != "59000" || trigger.TPSL != "sl" {
			t.Fatalf("trigger = %#v", trigger)
		}
		if got.VaultAddress != vault || got.ExpiresAfter != expiresAfter {
			t.Fatalf("submission parameters = vault %s, expiry %d", got.VaultAddress, got.ExpiresAfter)
		}
		if got.Action.Builder == nil || got.Action.Builder.Address != builder || got.Action.Builder.Fee != 10 {
			t.Fatalf("builder = %#v", got.Action.Builder)
		}
		_, _ = w.Write([]byte(`{"status":"ok","response":{"type":"order","data":{}}}`))
	}))
	defer server.Close()
	c, err := exchange.NewClientWithOptions(server.URL, "testnet", nil, 0, local, nonce.NewMonotonicManager(nil), triggerResolver(), "test", exchange.WithVaultAddress(vault), exchange.WithExpiresAfter(expiresAfter))
	if err != nil {
		t.Fatal(err)
	}
	_, err = c.PlaceOrder(context.Background(), exchange.OrderRequest{
		Coin: "BTC", IsBuy: false, Price: decimal.RequireFromString("58000"), Size: decimal.RequireFromString("0.1"),
		Type:    exchange.TriggerOrder{IsMarket: true, TriggerPrice: decimal.RequireFromString("59000"), TPSL: exchange.TPSLStopLoss},
		Builder: &exchange.Builder{Address: builder, Fee: 10},
	})
	if err != nil {
		t.Fatal(err)
	}
}

func TestPlaceOrderRejectsUnsafeSignerOutputBeforeSubmission(t *testing.T) {
	local, err := signer.NewLocalPrivateKeySignerFromHex("0123456789012345678901234567890123456789012345678901234567890123")
	if err != nil {
		t.Fatal(err)
	}
	wrong, err := signer.NewLocalPrivateKeySignerFromHex("1123456789012345678901234567890123456789012345678901234567890123")
	if err != nil {
		t.Fatal(err)
	}
	highS := signer.Signature{R: [32]byte{1}, S: [32]byte{}}
	copy(highS.S[:], common.Hex2Bytes("fffffffffffffffffffffffffffffffebaaedce6af48a03bbfd25e8cd0364141"))
	cases := []struct {
		name string
		sig  func(context.Context, signer.Digest) (signer.Signature, error)
	}{
		{name: "high S", sig: func(context.Context, signer.Digest) (signer.Signature, error) { return highS, nil }},
		{name: "wrong recovered address", sig: wrong.SignDigest},
		{name: "invalid scalar", sig: func(context.Context, signer.Digest) (signer.Signature, error) { return signer.Signature{}, nil }},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			requests := 0
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { requests++ }))
			defer server.Close()
			unsafe := mockSigner{address: local.Address(), sign: tc.sig}
			c := exchange.NewClient(server.URL, "mainnet", nil, 0, unsafe, nonce.NewMonotonicManager(nil), triggerResolver(), "test")
			_, err := c.PlaceOrder(context.Background(), validLimitOrder())
			if err == nil {
				t.Fatal("expected unsafe signature to be rejected")
			}
			if requests != 0 {
				t.Fatalf("submitted %d unsafe requests", requests)
			}
		})
	}
}

func TestPlaceOrdersRejectsMixedBuilderSettings(t *testing.T) {
	local, err := signer.NewLocalPrivateKeySignerFromHex("0123456789012345678901234567890123456789012345678901234567890123")
	if err != nil {
		t.Fatal(err)
	}
	requests := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { requests++ }))
	defer server.Close()
	c := exchange.NewClient(server.URL, "mainnet", nil, 0, local, nonce.NewMonotonicManager(nil), triggerResolver(), "test")
	plain := validLimitOrder()
	withBuilder := validLimitOrder()
	withBuilder.Builder = &exchange.Builder{Address: common.HexToAddress("0x2222222222222222222222222222222222222222"), Fee: 10}
	_, err = c.PlaceOrders(context.Background(), []exchange.OrderRequest{plain, withBuilder})
	if err == nil {
		t.Fatal("expected mixed builder settings to be rejected")
	}
	if requests != 0 {
		t.Fatalf("submitted %d requests", requests)
	}
}

func TestPlaceOrderRejectsBuilderFeeAboveMarketLimit(t *testing.T) {
	local, err := signer.NewLocalPrivateKeySignerFromHex("0123456789012345678901234567890123456789012345678901234567890123")
	if err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		name  string
		asset asset.Asset
		fee   uint64
	}{
		{name: "perpetual", asset: asset.Asset{ID: 0, Symbol: "BTC", Kind: asset.Perp, SzDecimals: 5}, fee: 101},
		{name: "spot", asset: asset.Asset{ID: 10000, Symbol: "PURR/USDC", Kind: asset.Spot, SzDecimals: 5}, fee: 1001},
	} {
		t.Run(tc.name, func(t *testing.T) {
			requests := 0
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { requests++ }))
			defer server.Close()
			resolver := asset.NewStaticResolver([]asset.Asset{tc.asset})
			c := exchange.NewClient(server.URL, "mainnet", nil, 0, local, nonce.NewMonotonicManager(nil), resolver, "test")
			request := validLimitOrder()
			request.Coin = tc.asset.Symbol
			request.Builder = &exchange.Builder{Address: common.HexToAddress("0x2222222222222222222222222222222222222222"), Fee: tc.fee}
			_, err := c.PlaceOrder(context.Background(), request)
			if err == nil {
				t.Fatal("expected invalid builder fee to be rejected")
			}
			if requests != 0 {
				t.Fatalf("submitted %d requests", requests)
			}
		})
	}
}

func TestModifyOrderTargetsCloid(t *testing.T) {
	local, err := signer.NewLocalPrivateKeySignerFromHex("0123456789012345678901234567890123456789012345678901234567890123")
	if err != nil {
		t.Fatal(err)
	}
	cloid := mustCloid(t, "0x1234567890abcdef1234567890abcdef")
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var payload struct {
			Action struct {
				Modifies []struct {
					OID string `json:"oid"`
				} `json:"modifies"`
			} `json:"action"`
		}
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			t.Fatal(err)
		}
		if got := payload.Action.Modifies[0].OID; got != cloid.String() {
			t.Fatalf("modify target = %q", got)
		}
		_, _ = w.Write([]byte(`{"status":"ok","response":{"type":"default","data":{}}}`))
	}))
	defer server.Close()
	c := exchange.NewClient(server.URL, "mainnet", nil, 0, local, nonce.NewMonotonicManager(nil), triggerResolver(), "test")
	_, err = c.ModifyOrder(context.Background(), exchange.ModifyRequest{Cloid: &cloid, Order: validLimitOrder()})
	if err != nil {
		t.Fatal(err)
	}
}

func mustCloid(t *testing.T, raw string) types.Cloid {
	t.Helper()
	cloid, err := types.ParseCloid(raw)
	if err != nil {
		t.Fatal(err)
	}
	return cloid
}

func triggerResolver() asset.Resolver {
	return asset.NewStaticResolver([]asset.Asset{{ID: 0, Symbol: "BTC", Kind: asset.Perp, SzDecimals: 5}})
}

func validLimitOrder() exchange.OrderRequest {
	return exchange.OrderRequest{Coin: "BTC", IsBuy: true, Price: decimal.RequireFromString("60000"), Size: decimal.RequireFromString("0.1"), Type: exchange.LimitOrder{TimeInForce: exchange.TIFGTC}}
}
