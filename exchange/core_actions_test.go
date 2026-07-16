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
	"github.com/Apexllcc/hyperliquid-go-sdk/signing"
	"github.com/ethereum/go-ethereum/common"
	"github.com/shopspring/decimal"
)

type fixedNonceManager struct{ next uint64 }

func (m *fixedNonceManager) Next(context.Context, common.Address) (uint64, error) {
	n := m.next
	m.next++
	return n, nil
}

type digestRecordingSigner struct {
	inner   signer.DigestSigner
	digests []signer.Digest
}

func (s *digestRecordingSigner) Address() common.Address { return s.inner.Address() }
func (s *digestRecordingSigner) SignDigest(ctx context.Context, digest signer.Digest) (signer.Signature, error) {
	s.digests = append(s.digests, digest)
	return s.inner.SignDigest(ctx, digest)
}

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

func TestCompatibilityExchangeActionsUseVerifiedL1Paths(t *testing.T) {
	local, err := signer.NewLocalPrivateKeySignerFromHex("0123456789012345678901234567890123456789012345678901234567890123")
	if err != nil {
		t.Fatal(err)
	}
	vault := common.HexToAddress("0x1111111111111111111111111111111111111111")
	expires := uint64(1_700_001_234_567)
	seen := map[string]struct{}{}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var payload struct {
			Action json.RawMessage `json:"action"`
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
		case "evmUserModify", "CValidatorAction", "CSignerAction":
			if payload.Vault == nil || *payload.Vault != vault || payload.Expiry == nil {
				t.Fatalf("%s must sign outside but route through configured vault", kind)
			}
		case "finalizeEvmContract":
			if payload.Vault != nil || payload.Expiry == nil {
				t.Fatalf("%s must omit its outer vault while retaining expiry", kind)
			}
		case "gossipPriorityBid":
			if payload.Vault == nil || *payload.Vault != vault || payload.Expiry == nil {
				t.Fatalf("%s must use configured L1 vault/expiry", kind)
			}
		}
		_, _ = w.Write([]byte(`{"status":"ok","response":{"type":"default","data":{}}}`))
	}))
	defer server.Close()
	client, err := exchange.NewClientWithOptions(server.URL, "mainnet", nil, 0, local, nonce.NewMonotonicManager(nil), asset.NewStaticResolver(nil), "test", exchange.WithVaultAddress(vault), exchange.WithExpiresAfter(expires))
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	if _, err := client.UseBigEVMBlocks(ctx, true); err != nil {
		t.Fatal(err)
	}
	if _, err := client.SubmitGossipPriorityBid(ctx, 0, "1.2.3.4", 100_000_000); err != nil {
		t.Fatal(err)
	}
	if _, err := client.SubmitCValidatorAction(ctx, signing.CValidatorUnregister{}); err != nil {
		t.Fatal(err)
	}
	if _, err := client.CSignerAction(ctx, signing.CSignerJailSelf{}); err != nil {
		t.Fatal(err)
	}
	if _, err := client.FinalizeEVMContract(ctx, 200, signing.FinalizeEVMCreate{Nonce: 0}); err != nil {
		t.Fatal(err)
	}
	for _, kind := range []string{"evmUserModify", "gossipPriorityBid", "CValidatorAction", "CSignerAction", "finalizeEvmContract"} {
		if _, ok := seen[kind]; !ok {
			t.Errorf("did not submit %s", kind)
		}
	}
}

func TestCompatibilityExchangeActionsHashWithOfficialVaultRules(t *testing.T) {
	local, err := signer.NewLocalPrivateKeySignerFromHex("0123456789012345678901234567890123456789012345678901234567890123")
	if err != nil {
		t.Fatal(err)
	}
	recording := &digestRecordingSigner{inner: local}
	vault := common.HexToAddress("0x1111111111111111111111111111111111111111")
	expires := uint64(1_700_001_234_567)
	nonces := &fixedNonceManager{next: 1_700_000_000_000}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{"status":"ok","response":{"type":"default","data":{}}}`))
	}))
	defer server.Close()
	client, err := exchange.NewClientWithOptions(server.URL, "mainnet", nil, 0, recording, nonces, asset.NewStaticResolver(nil), "test", exchange.WithVaultAddress(vault), exchange.WithExpiresAfter(expires))
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	if _, err := client.EVMUserModify(ctx, true); err != nil {
		t.Fatal(err)
	}
	if _, err := client.GossipPriorityBid(ctx, 0, "1.2.3.4", 100_000_000); err != nil {
		t.Fatal(err)
	}
	if _, err := client.CValidatorAction(ctx, signing.CValidatorUnregister{}); err != nil {
		t.Fatal(err)
	}
	if _, err := client.FinalizeEVMContract(ctx, 200, signing.FinalizeEVMCreate{Nonce: 0}); err != nil {
		t.Fatal(err)
	}
	want := []struct {
		action any
		vault  *common.Address
	}{
		{signing.EVMUserModifyAction{UsingBigBlocks: true}, nil},
		{signing.GossipPriorityBidAction{SlotID: 0, IP: "1.2.3.4", MaxGas: 100_000_000}, &vault},
		{signing.CValidatorAction{Variant: signing.CValidatorUnregister{}}, nil},
		{signing.FinalizeEVMContractAction{Token: 200, Input: signing.FinalizeEVMCreate{Nonce: 0}}, nil},
	}
	if len(recording.digests) != len(want) {
		t.Fatalf("recorded digests=%d", len(recording.digests))
	}
	for i, tc := range want {
		digest, err := signing.ComputeL1ActionDigest(tc.action, 1_700_000_000_000+uint64(i), tc.vault, &expires, true)
		if err != nil {
			t.Fatal(err)
		}
		if recording.digests[i] != digest {
			t.Fatalf("action %d signed wrong vault digest", i)
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
		func() error {
			_, err := client.SubmitGossipPriorityBid(context.Background(), 2, "not-an-ip", 0)
			return err
		},
		func() error {
			_, err := client.SubmitCValidatorAction(context.Background(), nil)
			return err
		},
		func() error {
			_, err := client.FinalizeEVMContract(context.Background(), 200, nil)
			return err
		},
		func() error {
			var variant *signing.CValidatorUnregister
			_, err := client.SubmitCValidatorAction(context.Background(), variant)
			return err
		},
		func() error {
			var input *signing.FinalizeEVMCreate
			_, err := client.FinalizeEVMContract(context.Background(), 200, input)
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
