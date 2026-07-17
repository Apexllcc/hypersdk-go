package hyperliquid_test

import (
	"os"
	"strings"
	"testing"
)

func TestReferenceCompatibilityMatrixPreservesProtocolDecisions(t *testing.T) {
	t.Parallel()

	api, err := os.ReadFile("API.md")
	if err != nil {
		t.Fatalf("read API.md: %v", err)
	}
	exchange, err := os.ReadFile("docs/api/exchange.md")
	if err != nil {
		t.Fatalf("read docs/api/exchange.md: %v", err)
	}
	exchangeText := strings.Join(strings.Fields(string(exchange)), " ")

	if !strings.Contains(string(api), "[reference compatibility matrix](docs/api/exchange.md#reference-compatibility-matrix)") {
		t.Error("API.md must link to the Exchange compatibility matrix")
	}
	for _, want := range []string{
		"Reference compatibility matrix",
		"DOCUMENT_UNSUPPORTED",
		"BLOCKED",
		"SUPERSEDED",
		"https://hyperliquid.gitbook.io/hyperliquid-docs/for-developers/api/exchange-endpoint",
		"https://github.com/hyperliquid-dex/hyperliquid-python-sdk/commit/2fdb18f9517675ea03695a0962bd19eece9c83f0",
	} {
		if !strings.Contains(exchangeText, want) {
			t.Errorf("docs/api/exchange.md is missing compatibility evidence %q", want)
		}
	}

	for _, decision := range []struct {
		action string
		status string
		reason string
		remedy string
	}{
		{
			action: "borrowLend",
			status: "DOCUMENT_UNSUPPORTED",
			reason: "No complete official Exchange action wire schema or signing contract is published.",
			remedy: "`BorrowLendUserState`",
		},
		{
			action: "registerReferrer",
			status: "DOCUMENT_UNSUPPORTED",
			reason: "No complete official Exchange action wire schema or signing contract is published.",
			remedy: "`SetReferrer`",
		},
		{
			action: "spotUser",
			status: "DOCUMENT_UNSUPPORTED",
			reason: "No complete official Exchange action wire schema or signing contract is published.",
			remedy: "No current Go SDK replacement",
		},
		{
			action: "linkStakingUser",
			status: "BLOCKED",
			reason: "Official product documentation omits the complete exchange envelope and action-specific signing contract.",
			remedy: "No current Go SDK replacement",
		},
		{
			action: "stakingLinkDisableTradingUser",
			status: "BLOCKED",
			reason: "Official product documentation omits the complete exchange envelope and action-specific signing contract.",
			remedy: "No current Go SDK replacement",
		},
		{
			action: "userPortfolioMargin",
			status: "SUPERSEDED",
			reason: "The current official action is userSetAbstraction.",
			remedy: "`UserSetAbstraction`",
		},
	} {
		row := "`" + decision.action + "` | `" + decision.status + "` | " + decision.reason + " | " + decision.remedy
		if !strings.Contains(exchangeText, row) {
			t.Errorf("compatibility row missing or altered: %s", row)
		}
	}

	for _, want := range []string{
		"The SDK implements none of the six candidate action names above.",
		"L1 actions use the phantom-Agent EIP-712 domain `Exchange` / `1` / chain ID `1337` / zero verifying contract",
		"User-signed actions use `HyperliquidSignTransaction` / `1` / `0x66eee` / zero verifying contract",
		"MessagePack action bytes, big-endian u64 nonce, vault-presence marker, and expiry marker",
		"low-S canonical form and recovered signer address",
		"`portfolioMargin` is not an `enabled: false` translation",
	} {
		if !strings.Contains(exchangeText, want) {
			t.Errorf("docs/api/exchange.md is missing compatibility safeguard %q", want)
		}
	}
}
