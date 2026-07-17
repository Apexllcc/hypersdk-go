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
	"github.com/ethereum/go-ethereum/common"
	"github.com/shopspring/decimal"
)

func TestPlaceAndCancelTWAPUseSignedL1PathAndTypedResponses(t *testing.T) {
	local, err := signer.NewLocalPrivateKeySignerFromHex("0123456789012345678901234567890123456789012345678901234567890123")
	if err != nil {
		t.Fatal(err)
	}
	requests := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests++
		var payload struct {
			Action json.RawMessage `json:"action"`
		}
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			t.Fatal(err)
		}
		var kind struct {
			Type string `json:"type"`
		}
		if err := json.Unmarshal(payload.Action, &kind); err != nil {
			t.Fatal(err)
		}
		switch kind.Type {
		case "twapOrder":
			_, _ = w.Write([]byte(`{"status":"ok","response":{"type":"twapOrder","data":{"status":{"running":{"twapId":77738308}}}}}`))
		case "twapCancel":
			_, _ = w.Write([]byte(`{"status":"ok","response":{"type":"twapCancel","data":{"status":"success"}}}`))
		default:
			t.Fatalf("unexpected action %q", kind.Type)
		}
	}))
	defer server.Close()
	c := exchange.NewClient(server.URL, "mainnet", nil, 0, local, nonce.NewMonotonicManager(nil), asset.NewStaticResolver([]asset.Asset{{ID: 0, Symbol: "BTC", Kind: asset.Perp, SzDecimals: 5}}), "test")

	placed, err := c.PlaceTWAP(context.Background(), exchange.TWAPOrderRequest{Coin: "BTC", IsBuy: true, Size: decimal.RequireFromString("1.2"), Minutes: 30, Randomize: true})
	if err != nil {
		t.Fatal(err)
	}
	orderData, ok := placed.Response.Data.(exchange.TWAPOrderResponseData)
	if !ok || orderData.Status.Running == nil || orderData.Status.Running.TWAPID != 77738308 {
		t.Fatalf("unexpected TWAP order response: %#v", placed.Response.Data)
	}
	canceled, err := c.CancelTWAP(context.Background(), exchange.TWAPCancelRequest{Coin: "BTC", TWAPID: 77738308})
	if err != nil {
		t.Fatal(err)
	}
	cancelData, ok := canceled.Response.Data.(exchange.TWAPCancelResponseData)
	if !ok || cancelData.Status.Success == nil || *cancelData.Status.Success != "success" {
		t.Fatalf("unexpected TWAP cancel response: %#v", canceled.Response.Data)
	}
	if requests != 2 {
		t.Fatalf("requests = %d, want 2", requests)
	}
}

func TestActionResponsePreservesUnknownVariant(t *testing.T) {
	var response exchange.ActionResponse
	if err := json.Unmarshal([]byte(`{"status":"ok","response":{"type":"futureAction","data":{"new":"field"}}}`), &response); err != nil {
		t.Fatal(err)
	}
	unknown, ok := response.Response.Data.(exchange.UnknownActionResponseData)
	if !ok {
		t.Fatalf("data type = %T", response.Response.Data)
	}
	if got := string(unknown.RawJSON()); got != `{"new":"field"}` {
		t.Fatalf("raw unknown data = %s", got)
	}
}

func TestTWAPDoesNotRetryExchangeSubmission(t *testing.T) {
	local, err := signer.NewLocalPrivateKeySignerFromHex("0123456789012345678901234567890123456789012345678901234567890123")
	if err != nil {
		t.Fatal(err)
	}
	requests := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		requests++
		w.WriteHeader(http.StatusServiceUnavailable)
		_, _ = w.Write([]byte("temporarily unavailable"))
	}))
	defer server.Close()
	c := exchange.NewClient(server.URL, "mainnet", nil, 0, local, nonce.NewMonotonicManager(nil), asset.NewStaticResolver([]asset.Asset{{ID: 0, Symbol: "BTC", Kind: asset.Perp, SzDecimals: 5}}), "test")
	_, err = c.PlaceTWAP(context.Background(), exchange.TWAPOrderRequest{Coin: "BTC", IsBuy: true, Size: decimal.RequireFromString("1"), Minutes: 30})
	if err == nil {
		t.Fatal("expected HTTP failure")
	}
	if requests != 1 {
		t.Fatalf("requests = %d, want exactly one Exchange submission", requests)
	}
}

func TestTWAPUsesConfiguredVaultAndExpiry(t *testing.T) {
	local, err := signer.NewLocalPrivateKeySignerFromHex("0123456789012345678901234567890123456789012345678901234567890123")
	if err != nil {
		t.Fatal(err)
	}
	vault := common.HexToAddress("0x1111111111111111111111111111111111111111")
	expiresAfter := uint64(1_700_001_234_567)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var payload struct {
			VaultAddress common.Address `json:"vaultAddress"`
			ExpiresAfter uint64         `json:"expiresAfter"`
		}
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			t.Fatal(err)
		}
		if payload.VaultAddress != vault || payload.ExpiresAfter != expiresAfter {
			t.Fatalf("submission options = vault %s, expiry %d", payload.VaultAddress, payload.ExpiresAfter)
		}
		_, _ = w.Write([]byte(`{"status":"ok","response":{"type":"twapOrder","data":{"status":{"running":{"twapId":1}}}}}`))
	}))
	defer server.Close()
	c, err := exchange.NewClientWithOptions(server.URL, "mainnet", nil, 0, local, nonce.NewMonotonicManager(nil), asset.NewStaticResolver([]asset.Asset{{ID: 0, Symbol: "BTC", Kind: asset.Perp, SzDecimals: 5}}), "test", exchange.WithVaultAddress(vault), exchange.WithExpiresAfter(expiresAfter))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := c.PlaceTWAP(context.Background(), exchange.TWAPOrderRequest{Coin: "BTC", IsBuy: true, Size: decimal.RequireFromString("1"), Minutes: 30}); err != nil {
		t.Fatal(err)
	}
}

func TestTWAPReturnsHTTP200ProtocolRejectionAsError(t *testing.T) {
	local, err := signer.NewLocalPrivateKeySignerFromHex("0123456789012345678901234567890123456789012345678901234567890123")
	if err != nil {
		t.Fatal(err)
	}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{"status":"err","response":"Invalid TWAP duration"}`))
	}))
	defer server.Close()
	c := exchange.NewClient(server.URL, "mainnet", nil, 0, local, nonce.NewMonotonicManager(nil), asset.NewStaticResolver([]asset.Asset{{ID: 0, Symbol: "BTC", Kind: asset.Perp, SzDecimals: 5}}), "test")
	response, err := c.PlaceTWAP(context.Background(), exchange.TWAPOrderRequest{Coin: "BTC", IsBuy: true, Size: decimal.RequireFromString("1"), Minutes: 30})
	if err == nil {
		t.Fatal("expected protocol rejection")
	}
	if response.Error == nil || response.Error.Message != "Invalid TWAP duration" {
		t.Fatalf("response error = %#v", response.Error)
	}
}
