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

func TestSendUSDUsesUserActionWithoutVaultOrExpiry(t *testing.T) {
	local, err := signer.NewLocalPrivateKeySignerFromHex("0123456789012345678901234567890123456789012345678901234567890123")
	if err != nil {
		t.Fatal(err)
	}
	vault := common.HexToAddress("0x1111111111111111111111111111111111111111")
	expires := uint64(1700001234567)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var got struct {
			Action struct {
				Type             string `json:"type"`
				HyperliquidChain string `json:"hyperliquidChain"`
				Amount           string `json:"amount"`
				Time             uint64 `json:"time"`
			} `json:"action"`
			Nonce  uint64         `json:"nonce"`
			Vault  common.Address `json:"vaultAddress"`
			Expiry *uint64        `json:"expiresAfter"`
		}
		if err := json.NewDecoder(r.Body).Decode(&got); err != nil {
			t.Fatal(err)
		}
		if got.Action.Type != "usdSend" || got.Action.HyperliquidChain != "Mainnet" || got.Action.Amount != "12.34" {
			t.Fatalf("action = %#v", got.Action)
		}
		if got.Action.Time != got.Nonce {
			t.Fatalf("action time %d does not match nonce %d", got.Action.Time, got.Nonce)
		}
		if got.Vault != vault || got.Expiry != nil {
			t.Fatalf("user-signed action outer fields vault=%v expiry=%v", got.Vault, got.Expiry)
		}
		_, _ = w.Write([]byte(`{"status":"ok","response":{"type":"usdSend","data":{}}}`))
	}))
	defer server.Close()
	c, err := exchange.NewClientWithOptions(server.URL, "mainnet", nil, 0, local, nonce.NewMonotonicManager(nil), asset.NewStaticResolver(nil), "test", exchange.WithVaultAddress(vault), exchange.WithExpiresAfter(expires))
	if err != nil {
		t.Fatal(err)
	}
	_, err = c.SendUSD(context.Background(), exchange.USDSendRequest{Destination: common.HexToAddress("0x2222222222222222222222222222222222222222"), Amount: decimal.RequireFromString("12.34")})
	if err != nil {
		t.Fatal(err)
	}
}

func TestVaultTransferDoesNotSignAsConfiguredTradingVault(t *testing.T) {
	local, err := signer.NewLocalPrivateKeySignerFromHex("0123456789012345678901234567890123456789012345678901234567890123")
	if err != nil {
		t.Fatal(err)
	}
	configuredVault := common.HexToAddress("0x1111111111111111111111111111111111111111")
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var got struct {
			Action struct {
				Type         string `json:"type"`
				VaultAddress string `json:"vaultAddress"`
			} `json:"action"`
			Vault common.Address `json:"vaultAddress"`
		}
		if err := json.NewDecoder(r.Body).Decode(&got); err != nil {
			t.Fatal(err)
		}
		if got.Action.Type != "vaultTransfer" || got.Action.VaultAddress != "0x2222222222222222222222222222222222222222" {
			t.Fatalf("action=%#v", got.Action)
		}
		if got.Vault != configuredVault {
			t.Fatalf("outer vault must retain configured routing address, got %v", got.Vault)
		}
		_, _ = w.Write([]byte(`{"status":"ok","response":{"type":"vaultTransfer","data":{}}}`))
	}))
	defer server.Close()
	c, err := exchange.NewClientWithOptions(server.URL, "mainnet", nil, 0, local, nonce.NewMonotonicManager(nil), asset.NewStaticResolver(nil), "test", exchange.WithVaultAddress(configuredVault))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := c.TransferVaultUSD(context.Background(), exchange.VaultTransferRequest{VaultAddress: common.HexToAddress("0x2222222222222222222222222222222222222222"), IsDeposit: true, USD: 456}); err != nil {
		t.Fatal(err)
	}
}

func TestSendUSDRejectsInvalidSignatureBeforeSubmission(t *testing.T) {
	local, err := signer.NewLocalPrivateKeySignerFromHex("0123456789012345678901234567890123456789012345678901234567890123")
	if err != nil {
		t.Fatal(err)
	}
	requests := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { requests++ }))
	defer server.Close()
	unsafe := mockSigner{address: local.Address(), sign: func(context.Context, signer.Digest) (signer.Signature, error) { return signer.Signature{}, nil }}
	c := exchange.NewClient(server.URL, "mainnet", nil, 0, unsafe, nonce.NewMonotonicManager(nil), asset.NewStaticResolver(nil), "test")
	_, err = c.SendUSD(context.Background(), exchange.USDSendRequest{Destination: common.HexToAddress("0x2222222222222222222222222222222222222222"), Amount: decimal.RequireFromString("1")})
	if err == nil {
		t.Fatal("expected invalid signature to be rejected")
	}
	if requests != 0 {
		t.Fatalf("submitted %d unsafe requests", requests)
	}
}

func TestUserSignedSubaccountTransfersFollowConfiguredVaultSemantics(t *testing.T) {
	local, err := signer.NewLocalPrivateKeySignerFromHex("0123456789012345678901234567890123456789012345678901234567890123")
	if err != nil {
		t.Fatal(err)
	}
	vault := common.HexToAddress("0x1111111111111111111111111111111111111111")
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var got struct {
			Action struct {
				Type           string `json:"type"`
				Amount         string `json:"amount"`
				FromSubAccount string `json:"fromSubAccount"`
			} `json:"action"`
			Vault *common.Address `json:"vaultAddress"`
		}
		if err := json.NewDecoder(r.Body).Decode(&got); err != nil {
			t.Fatal(err)
		}
		switch got.Action.Type {
		case "usdClassTransfer":
			if got.Action.Amount != "2 subaccount:"+vault.Hex() || got.Vault != nil {
				t.Fatalf("class transfer action=%#v vault=%v", got.Action, got.Vault)
			}
		case "sendAsset":
			if got.Action.FromSubAccount != vault.Hex() || got.Vault != nil {
				t.Fatalf("send asset action=%#v vault=%v", got.Action, got.Vault)
			}
		default:
			t.Fatalf("unexpected action %q", got.Action.Type)
		}
		_, _ = w.Write([]byte(`{"status":"ok","response":{"type":"ok","data":{}}}`))
	}))
	defer server.Close()
	c, err := exchange.NewClientWithOptions(server.URL, "mainnet", nil, 0, local, nonce.NewMonotonicManager(nil), asset.NewStaticResolver(nil), "test", exchange.WithVaultAddress(vault))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := c.TransferUSDClass(context.Background(), exchange.USDClassTransferRequest{Amount: decimal.RequireFromString("2"), ToPerp: true}); err != nil {
		t.Fatal(err)
	}
	if _, err := c.SendAsset(context.Background(), exchange.SendAssetRequest{Destination: common.HexToAddress("0x2222222222222222222222222222222222222222"), Token: "USDC", Amount: decimal.RequireFromString("3")}); err != nil {
		t.Fatal(err)
	}
}

func TestVaultTransferWithoutNonceManagerReturnsError(t *testing.T) {
	local, err := signer.NewLocalPrivateKeySignerFromHex("0123456789012345678901234567890123456789012345678901234567890123")
	if err != nil {
		t.Fatal(err)
	}
	c := exchange.NewClient("http://127.0.0.1:1", "mainnet", nil, 0, local, nil, asset.NewStaticResolver(nil), "test")
	if _, err := c.TransferVaultUSD(context.Background(), exchange.VaultTransferRequest{VaultAddress: common.HexToAddress("0x2222222222222222222222222222222222222222"), IsDeposit: true, USD: 1}); err == nil {
		t.Fatal("expected nonce manager error")
	}
}
