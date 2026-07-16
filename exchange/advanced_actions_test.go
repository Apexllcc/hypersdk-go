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

func TestAdvancedExchangeActionsUseOfficialSigningPaths(t *testing.T) {
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
		case "userDexAbstraction", "userSetAbstraction":
			if payload.Expiry != nil || payload.Vault == nil || *payload.Vault != vault {
				t.Fatalf("%s must be user-signed with outer vault but no expiry: vault=%v expiry=%v", kind, payload.Vault, payload.Expiry)
			}
			if action["signatureChainId"] != "0x66eee" || action["hyperliquidChain"] != "Mainnet" || action["nonce"] != float64(payload.Nonce) {
				t.Fatalf("%s action=%#v nonce=%d", kind, action, payload.Nonce)
			}
		case "agentEnableDexAbstraction", "agentSetAbstraction", "validatorL1Stream", "claimRewards", "setReferrer", "createSubAccount":
			if payload.Vault == nil || *payload.Vault != vault || payload.Expiry == nil || *payload.Expiry != expires {
				t.Fatalf("%s must be L1 with configured vault/expiry: vault=%v expiry=%v", kind, payload.Vault, payload.Expiry)
			}
		case "userOutcome":
			if payload.Vault == nil || *payload.Vault != vault || payload.Expiry == nil || *payload.Expiry != expires {
				t.Fatalf("%s must use ordinary L1 vault/expiry: vault=%v expiry=%v", kind, payload.Vault, payload.Expiry)
			}
			variant, ok := action["splitOutcome"].(map[string]any)
			if !ok || variant["outcome"] != float64(7) || variant["amount"] != "1.25" {
				t.Fatalf("%s action variant missing: %#v", kind, action)
			}
		case "agentSendAsset":
			if payload.Vault != nil || payload.Expiry == nil || *payload.Expiry != expires {
				t.Fatalf("%s must omit outer vault but include expiry: vault=%v expiry=%v", kind, payload.Vault, payload.Expiry)
			}
			if action["nonce"] != float64(payload.Nonce) || action["fromSubAccount"] != "" {
				t.Fatalf("%s must carry source account and match outer nonce: action=%#v outer nonce=%d", kind, action, payload.Nonce)
			}
		case "authorizeAqav2Role", "hip3LiquidatorTransfer":
			if payload.Vault != nil || payload.Expiry == nil || *payload.Expiry != expires {
				t.Fatalf("%s must omit outer vault but include expiry: vault=%v expiry=%v", kind, payload.Vault, payload.Expiry)
			}
		}
		_, _ = w.Write([]byte(`{"status":"ok","response":{"type":"default","data":{}}}`))
	}))
	defer server.Close()
	c, err := exchange.NewClientWithOptions(server.URL, "mainnet", nil, 0, local, nonce.NewMonotonicManager(nil), asset.NewStaticResolver(nil), "test", exchange.WithVaultAddress(vault), exchange.WithExpiresAfter(expires))
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	if _, err := c.UserDexAbstraction(ctx, exchange.UserDexAbstractionRequest{User: common.HexToAddress("0x2222222222222222222222222222222222222222"), Enabled: true}); err != nil {
		t.Fatal(err)
	}
	if _, err := c.UserSetAbstraction(ctx, exchange.UserSetAbstractionRequest{User: common.HexToAddress("0x2222222222222222222222222222222222222222"), Abstraction: exchange.UserAbstractionUnifiedAccount}); err != nil {
		t.Fatal(err)
	}
	if _, err := c.AgentEnableDexAbstraction(ctx); err != nil {
		t.Fatal(err)
	}
	if _, err := c.AgentSetAbstraction(ctx, exchange.AgentAbstractionPortfolioMargin); err != nil {
		t.Fatal(err)
	}
	if _, err := c.ValidatorL1Stream(ctx, "0.04"); err != nil {
		t.Fatal(err)
	}
	if _, err := c.ClaimRewards(ctx); err != nil {
		t.Fatal(err)
	}
	if _, err := c.SetReferrer(ctx, "code"); err != nil {
		t.Fatal(err)
	}
	if _, err := c.CreateSubAccount(ctx, "desk-1"); err != nil {
		t.Fatal(err)
	}
	if _, err := c.AgentSendAsset(ctx, exchange.AgentSendAssetRequest{Destination: common.HexToAddress("0x2222222222222222222222222222222222222222"), SourceDEX: "", DestinationDEX: "spot", Token: "USDC:0x1", Amount: decimal.RequireFromString("1.25")}); err != nil {
		t.Fatal(err)
	}
	if _, err := c.AuthorizeAQAV2Role(ctx, exchange.AuthorizeAQAV2RoleRequest{Token: 0, Role: exchange.AQAV2RoleTechnical}); err != nil {
		t.Fatal(err)
	}
	if _, err := c.HIP3LiquidatorTransfer(ctx, exchange.HIP3LiquidatorTransferRequest{DEX: "xyz", NTL: 1_000_000_000, IsDeposit: true}); err != nil {
		t.Fatal(err)
	}
	if _, err := c.UserOutcome(ctx, exchange.UserOutcomeRequest{SplitOutcome: &exchange.SplitOutcomeRequest{Outcome: 7, Amount: decimal.RequireFromString("1.25")}}); err != nil {
		t.Fatal(err)
	}
	for _, kind := range []string{"userDexAbstraction", "userSetAbstraction", "agentEnableDexAbstraction", "agentSetAbstraction", "validatorL1Stream", "claimRewards", "setReferrer", "createSubAccount", "agentSendAsset", "authorizeAqav2Role", "hip3LiquidatorTransfer", "userOutcome"} {
		if _, ok := seen[kind]; !ok {
			t.Errorf("did not submit %s", kind)
		}
	}
}

func TestAdvancedExchangeActionsRejectInvalidValuesBeforeSubmission(t *testing.T) {
	local, err := signer.NewLocalPrivateKeySignerFromHex("0123456789012345678901234567890123456789012345678901234567890123")
	if err != nil {
		t.Fatal(err)
	}
	requests := 0
	server := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) { requests++ }))
	defer server.Close()
	c := exchange.NewClient(server.URL, "mainnet", nil, 0, local, nonce.NewMonotonicManager(nil), asset.NewStaticResolver(nil), "test")
	cases := []func() error{
		func() error {
			_, err := c.UserDexAbstraction(context.Background(), exchange.UserDexAbstractionRequest{})
			return err
		},
		func() error {
			_, err := c.UserSetAbstraction(context.Background(), exchange.UserSetAbstractionRequest{})
			return err
		},
		func() error { _, err := c.AgentSetAbstraction(context.Background(), "invalid"); return err },
		func() error { _, err := c.ValidatorL1Stream(context.Background(), "not-a-rate"); return err },
		func() error { _, err := c.SetReferrer(context.Background(), ""); return err },
		func() error { _, err := c.CreateSubAccount(context.Background(), ""); return err },
		func() error {
			_, err := c.AgentSendAsset(context.Background(), exchange.AgentSendAssetRequest{})
			return err
		},
		func() error {
			_, err := c.AuthorizeAQAV2Role(context.Background(), exchange.AuthorizeAQAV2RoleRequest{Role: "invalid"})
			return err
		},
		func() error {
			_, err := c.HIP3LiquidatorTransfer(context.Background(), exchange.HIP3LiquidatorTransferRequest{DEX: "xyz", NTL: 1})
			return err
		},
		func() error {
			_, err := c.UserOutcome(context.Background(), exchange.UserOutcomeRequest{})
			return err
		},
		func() error {
			_, err := c.UserOutcome(context.Background(), exchange.UserOutcomeRequest{SplitOutcome: &exchange.SplitOutcomeRequest{Amount: decimal.Zero}})
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
