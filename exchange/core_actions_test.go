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
	"github.com/ethereum/go-ethereum/common"
	"github.com/shopspring/decimal"
)

func TestCoreExchangeActionsUseTheirDocumentedSigningPaths(t *testing.T) {
	local, err := signer.NewLocalPrivateKeySignerFromHex("0123456789012345678901234567890123456789012345678901234567890123")
	if err != nil {
		t.Fatal(err)
	}
	vault := common.HexToAddress("0x1111111111111111111111111111111111111111")
	seen := make(map[string]struct{})
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var payload struct {
			Action json.RawMessage `json:"action"`
			Nonce  uint64          `json:"nonce"`
			Vault  *common.Address `json:"vaultAddress"`
			Expiry *uint64         `json:"expiresAfter"`
		}
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			t.Fatal(err)
		}
		var action map[string]any
		if err := json.Unmarshal(payload.Action, &action); err != nil {
			t.Fatal(err)
		}
		kind, _ := action["type"].(string)
		seen[kind] = struct{}{}
		switch kind {
		case "updateLeverage", "updateIsolatedMargin", "topUpIsolatedOnlyMargin":
			if payload.Vault == nil || *payload.Vault != vault || payload.Expiry == nil {
				t.Fatalf("%s must use configured L1 vault/expiry: vault=%v expiry=%v", kind, payload.Vault, payload.Expiry)
			}
		case "reserveRequestWeight", "noop":
			if payload.Vault != nil || payload.Expiry == nil {
				t.Fatalf("%s must omit vault while retaining L1 expiry: vault=%v expiry=%v", kind, payload.Vault, payload.Expiry)
			}
		case "sendToEvmWithData", "cDeposit", "cWithdraw", "tokenDelegate":
			if payload.Vault != nil || payload.Expiry != nil {
				t.Fatalf("%s must not send L1 vault/expiry: vault=%v expiry=%v", kind, payload.Vault, payload.Expiry)
			}
			if action["signatureChainId"] != "0x66eee" || action["hyperliquidChain"] != "Mainnet" || action["nonce"] != float64(payload.Nonce) {
				t.Fatalf("%s user action fields=%#v nonce=%d", kind, action, payload.Nonce)
			}
		}
		_, _ = w.Write([]byte(`{"status":"ok","response":{"type":"default","data":{}}}`))
	}))
	defer server.Close()
	expires := uint64(1_700_001_234_567)
	client, err := exchange.NewClientWithOptions(server.URL, "mainnet", nil, 0, local, nonce.NewMonotonicManager(nil), asset.NewStaticResolver([]asset.Asset{{ID: 0, Symbol: "BTC", Kind: asset.Perp}}), "test", exchange.WithVaultAddress(vault), exchange.WithExpiresAfter(expires))
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	if _, err := client.UpdateLeverage(ctx, exchange.UpdateLeverageRequest{Coin: "BTC", IsCross: true, Leverage: 5}); err != nil {
		t.Fatal(err)
	}
	if _, err := client.UpdateIsolatedMargin(ctx, exchange.UpdateIsolatedMarginRequest{Coin: "BTC", IsBuy: true, Amount: decimal.RequireFromString("1.25")}); err != nil {
		t.Fatal(err)
	}
	if _, err := client.TopUpIsolatedOnlyMargin(ctx, exchange.TopUpIsolatedOnlyMarginRequest{Coin: "BTC", Leverage: decimal.RequireFromString("2.5")}); err != nil {
		t.Fatal(err)
	}
	if _, err := client.ReserveRequestWeight(ctx, 10); err != nil {
		t.Fatal(err)
	}
	if _, err := client.Noop(ctx, 1_700_000_000_000); err != nil {
		t.Fatal(err)
	}
	if _, err := client.SendToEVMWithData(ctx, exchange.SendToEVMWithDataRequest{Token: "USDC", Amount: decimal.RequireFromString("1.25"), SourceDEX: "", DestinationRecipient: "0x2222222222222222222222222222222222222222", AddressEncoding: exchange.AddressEncodingHex, DestinationChainID: 42161, GasLimit: 200000, Data: []byte{1, 2}}); err != nil {
		t.Fatal(err)
	}
	if _, err := client.CDeposit(ctx, exchange.StakingTransferRequest{Wei: 100_000_000}); err != nil {
		t.Fatal(err)
	}
	if _, err := client.CWithdraw(ctx, exchange.StakingTransferRequest{Wei: 100_000_000}); err != nil {
		t.Fatal(err)
	}
	if _, err := client.TokenDelegate(ctx, exchange.TokenDelegateRequest{Validator: common.HexToAddress("0x3333333333333333333333333333333333333333"), Wei: 100_000_000}); err != nil {
		t.Fatal(err)
	}
	for _, kind := range []string{"updateLeverage", "updateIsolatedMargin", "topUpIsolatedOnlyMargin", "reserveRequestWeight", "noop", "sendToEvmWithData", "cDeposit", "cWithdraw", "tokenDelegate"} {
		if _, ok := seen[kind]; !ok {
			t.Errorf("did not submit %s", kind)
		}
	}
}

func TestCoreExchangeActionsRejectInvalidValuesBeforeSubmission(t *testing.T) {
	local, err := signer.NewLocalPrivateKeySignerFromHex("0123456789012345678901234567890123456789012345678901234567890123")
	if err != nil {
		t.Fatal(err)
	}
	requests := 0
	server := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) { requests++ }))
	defer server.Close()
	client := exchange.NewClient(server.URL, "mainnet", nil, 0, local, nonce.NewMonotonicManager(nil), asset.NewStaticResolver([]asset.Asset{{ID: 0, Symbol: "BTC", Kind: asset.Perp}}), "test")
	cases := []func() error{
		func() error {
			_, err := client.UpdateLeverage(context.Background(), exchange.UpdateLeverageRequest{Coin: "BTC"})
			return err
		},
		func() error {
			_, err := client.UpdateIsolatedMargin(context.Background(), exchange.UpdateIsolatedMarginRequest{Coin: "BTC"})
			return err
		},
		func() error {
			_, err := client.TopUpIsolatedOnlyMargin(context.Background(), exchange.TopUpIsolatedOnlyMarginRequest{Coin: "BTC"})
			return err
		},
		func() error {
			_, err := client.CDeposit(context.Background(), exchange.StakingTransferRequest{})
			return err
		},
		func() error {
			_, err := client.TokenDelegate(context.Background(), exchange.TokenDelegateRequest{Wei: 1})
			return err
		},
		func() error {
			_, err := client.SendToEVMWithData(context.Background(), exchange.SendToEVMWithDataRequest{Token: "USDC", Amount: decimal.NewFromInt(1), AddressEncoding: exchange.AddressEncodingHex})
			return err
		},
	}
	for _, call := range cases {
		if err := call(); err == nil {
			t.Error("expected invalid request error")
		}
	}
	if requests != 0 {
		t.Fatalf("invalid requests submitted=%d", requests)
	}
}
