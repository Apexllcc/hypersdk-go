package exchange_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"

	"github.com/Apexllcc/hyperliquid-go-sdk/asset"
	"github.com/Apexllcc/hyperliquid-go-sdk/exchange"
	"github.com/Apexllcc/hyperliquid-go-sdk/nonce"
	"github.com/Apexllcc/hyperliquid-go-sdk/signer"
	"github.com/Apexllcc/hyperliquid-go-sdk/signing"
	"github.com/ethereum/go-ethereum/common"
)

func TestDeployActionsUseL1MasterSigningAndOuterVaultExpiry(t *testing.T) {
	local, err := signer.NewLocalPrivateKeySignerFromHex("0123456789012345678901234567890123456789012345678901234567890123")
	if err != nil {
		t.Fatal(err)
	}
	vault := common.HexToAddress("0x1111111111111111111111111111111111111111")
	expires := uint64(1_700_010_000_000)
	var got []struct {
		Action    map[string]json.RawMessage `json:"action"`
		Vault     *common.Address            `json:"vaultAddress"`
		Expiry    *uint64                    `json:"expiresAfter"`
		Signature map[string]any             `json:"signature"`
	}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var payload struct {
			Action    map[string]json.RawMessage `json:"action"`
			Vault     *common.Address            `json:"vaultAddress"`
			Expiry    *uint64                    `json:"expiresAfter"`
			Signature map[string]any             `json:"signature"`
		}
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			t.Fatal(err)
		}
		got = append(got, payload)
		_, _ = w.Write([]byte(`{"status":"ok","response":{"type":"default","data":{}}}`))
	}))
	defer server.Close()
	c, err := exchange.NewClientWithOptions(server.URL, "mainnet", nil, 0, local, nonce.NewMonotonicManager(nil), asset.NewStaticResolver(nil), "test", exchange.WithVaultAddress(vault), exchange.WithExpiresAfter(expires))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := c.SubmitPerpDeploy(context.Background(), signing.SetFeeScale{DEX: "abc", Scale: "1"}); err != nil {
		t.Fatal(err)
	}
	if _, err := c.SubmitSpotDeploy(context.Background(), signing.RegisterSpot{Tokens: [2]uint64{1, 0}}); err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 {
		t.Fatalf("requests = %d", len(got))
	}
	for _, payload := range got {
		if payload.Vault == nil || *payload.Vault != vault || payload.Expiry == nil || *payload.Expiry != expires {
			t.Fatalf("outer vault/expiry = %+v", payload)
		}
		if _, ok := payload.Signature["r"]; !ok {
			t.Fatalf("missing L1 signature: %+v", payload.Signature)
		}
		if _, ok := payload.Action["type"]; !ok {
			t.Fatalf("missing action type")
		}
	}
}

func TestDeployActionsRejectInvalidInputBeforeHTTPAndNeverRetry(t *testing.T) {
	local, err := signer.NewLocalPrivateKeySignerFromHex("0123456789012345678901234567890123456789012345678901234567890123")
	if err != nil {
		t.Fatal(err)
	}
	var requests atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests.Add(1)
		// A terminated connection is ambiguous after a signed action. Exchange
		// must not replay it.
		hj := w.(http.Hijacker)
		conn, _, err := hj.Hijack()
		if err == nil {
			_ = conn.Close()
		}
	}))
	defer server.Close()
	c := exchange.NewClient(server.URL, "mainnet", nil, 0, local, nonce.NewMonotonicManager(nil), asset.NewStaticResolver(nil), "test")
	if _, err := c.SubmitPerpDeploy(context.Background(), signing.SetFeeRecipient{DEX: "abc", FeeRecipient: "invalid"}); err == nil {
		t.Fatal("expected validation error")
	}
	if requests.Load() != 0 {
		t.Fatalf("invalid deployment made %d HTTP requests", requests.Load())
	}
	if _, err := c.SubmitSpotDeploy(context.Background(), signing.RegisterSpot{Tokens: [2]uint64{1, 0}}); err == nil {
		t.Fatal("expected network error")
	}
	if requests.Load() != 1 {
		t.Fatalf("exchange replayed deployment: %d requests", requests.Load())
	}
}
