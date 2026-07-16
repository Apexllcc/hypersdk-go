package hyperliquid_test

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestExchangeExampleIsSafeAndBuildable(t *testing.T) {
	t.Parallel()

	path := filepath.Join("examples", "exchange_limit_order", "main.go")
	source, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read exchange example: %v", err)
	}
	if !strings.Contains(string(source), "HL_TESTNET_TRADE") {
		t.Fatal("exchange example must require an explicit testnet trade opt-in")
	}
	if !strings.Contains(string(source), "exchange.OrderRequest") {
		t.Fatal("exchange example must demonstrate a typed order request")
	}
	if strings.Contains(string(source), ".PlaceOrder(") {
		t.Fatal("exchange example must remain a dry-run and never submit an order")
	}
	if strings.Contains(string(source), "293af1852d0c9d4d67ae0b340bbcc896138ae6e55b4592a705f7326bc966e6f9") {
		t.Fatal("exchange example must not contain a private key")
	}

	command := exec.Command("go", "build", "-o", filepath.Join(t.TempDir(), "exchange_limit_order"), "./examples/exchange_limit_order")
	if output, err := command.CombinedOutput(); err != nil {
		t.Fatalf("build exchange example: %v\n%s", err, output)
	}
}
