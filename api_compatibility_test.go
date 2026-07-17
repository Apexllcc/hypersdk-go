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
	exchangeText := string(exchange)
	rows := compatibilityMatrixRows(exchangeText)

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

	for action, status := range map[string]string{
		"borrowLend":                    "DOCUMENT_UNSUPPORTED",
		"registerReferrer":              "DOCUMENT_UNSUPPORTED",
		"spotUser":                      "DOCUMENT_UNSUPPORTED",
		"linkStakingUser":               "BLOCKED",
		"stakingLinkDisableTradingUser": "BLOCKED",
		"userPortfolioMargin":           "SUPERSEDED",
	} {
		got, ok := rows[action]
		if !ok {
			t.Errorf("compatibility matrix is missing %q", action)
			continue
		}
		if got != status {
			t.Errorf("compatibility matrix status for %q = %q, want %q", action, got, status)
		}
	}

	for _, want := range []string{
		"BorrowLendUserState",
		"BorrowLendReserveState",
		"AllBorrowLendReserveStates",
		"SetReferrer",
		"does not create a referral code",
		"complete Exchange envelope",
		"action-specific signing contract",
		"UserSetAbstraction",
		"`unifiedAccount`",
		"`disabled`",
		"MessagePack action bytes",
		"big-endian u64 nonce",
		"vault-presence marker",
		"vault address bytes when present",
		"expiry marker",
		"`expiresAfter` as a big-endian u64 value when present",
		"low-S canonical form",
		"recovered signer address",
	} {
		if !strings.Contains(exchangeText, want) {
			t.Errorf("docs/api/exchange.md is missing compatibility safeguard %q", want)
		}
	}
}

func TestCompatibilityMatrixRowsUsesDecisionCellNotOtherStatusText(t *testing.T) {
	t.Parallel()

	rows := compatibilityMatrixRows(strings.Join([]string{
		"## Reference compatibility matrix",
		"",
		"| Candidate action | Decision | Evidence |",
		"| --- | --- | --- |",
		"| `borrowLend` | `BLOCKED` | Previous decision was `DOCUMENT_UNSUPPORTED`. |",
	}, "\n"))
	if got := rows["borrowLend"]; got != "BLOCKED" {
		t.Fatalf("decision for borrowLend = %q, want BLOCKED", got)
	}
}

func compatibilityMatrixRows(document string) map[string]string {
	rows := make(map[string]string)
	inMatrix := false
	for _, line := range strings.Split(document, "\n") {
		if line == "## Reference compatibility matrix" {
			inMatrix = true
			continue
		}
		if !inMatrix || !strings.HasPrefix(line, "| `") {
			continue
		}
		fields := strings.Split(line, "|")
		if len(fields) < 3 {
			continue
		}
		action := strings.Trim(strings.TrimSpace(fields[1]), "`")
		decision := strings.Trim(strings.TrimSpace(fields[2]), "`")
		rows[action] = decision
	}
	return rows
}
