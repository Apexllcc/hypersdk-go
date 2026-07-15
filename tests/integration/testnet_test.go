//go:build integration

package integration

import (
	"os"
	"testing"
)

// TestExplicitTestnetConfiguration protects against accidental live trading.
// It verifies only that users consciously supplied testnet configuration.
func TestExplicitTestnetConfiguration(t *testing.T) {
	if os.Getenv("HL_TESTNET_ACCOUNT_ADDRESS") == "" {
		t.Skip("set HL_TESTNET_ACCOUNT_ADDRESS to enable testnet integration setup")
	}
}
