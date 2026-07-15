//go:build integration && testnet

package integration

import (
	"context"
	"errors"
	"os"
	"testing"
	"time"

	hyperliquid "github.com/Apexllcc/hyperliquid-go-sdk"
	"github.com/Apexllcc/hyperliquid-go-sdk/websocket"
	"github.com/ethereum/go-ethereum/common"
)

const (
	testnetEnableEnv  = "HL_TESTNET_READONLY"
	testnetAccountEnv = "HL_TESTNET_ACCOUNT_ADDRESS"
)

// requireReadOnlyTestnet makes a network test opt-in twice: it is compiled
// only with -tags='integration testnet' and it runs only with an explicit
// read-only acknowledgement and a valid testnet account address. These tests
// never create an Exchange client action or submit a transaction.
func requireReadOnlyTestnet(t *testing.T) string {
	t.Helper()
	if os.Getenv(testnetEnableEnv) != "1" {
		t.Skip("set HL_TESTNET_READONLY=1 to enable read-only testnet checks")
	}
	address := os.Getenv(testnetAccountEnv)
	if !common.IsHexAddress(address) {
		t.Skip("set HL_TESTNET_ACCOUNT_ADDRESS to a valid hexadecimal address")
	}
	return address
}

func newReadOnlyTestnetClient(t *testing.T) *hyperliquid.Client {
	t.Helper()
	client, err := hyperliquid.NewClient(
		hyperliquid.WithTestnet(),
		hyperliquid.WithHTTPTimeout(10*time.Second),
		hyperliquid.WithWebSocketConfig(websocket.Config{
			ReconnectDelay: 200 * time.Millisecond,
			EventBuffer:    1,
		}),
	)
	if err != nil {
		t.Fatalf("new testnet client: %v", err)
	}
	t.Cleanup(func() { _ = client.WebSocket.Close() })
	return client
}

// TestTestnetInfoReadOnly calls only unsigned Info endpoints. No signing key,
// funding operation, order, transfer, or Exchange endpoint is involved.
func TestTestnetInfoReadOnly(t *testing.T) {
	address := requireReadOnlyTestnet(t)
	client := newReadOnlyTestnetClient(t)
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	mids, err := client.Info.AllMids(ctx)
	if err != nil {
		t.Fatalf("testnet all mids: %v", err)
	}
	if len(mids) == 0 {
		t.Fatal("testnet all mids returned no markets")
	}
	if _, err := client.Info.ClearinghouseState(ctx, address); err != nil {
		t.Fatalf("testnet clearinghouse state: %v", err)
	}
}

// TestTestnetWebSocketReadOnly subscribes to a public market stream only.
// It closes the shared connection during cleanup and does not authenticate.
func TestTestnetWebSocketReadOnly(t *testing.T) {
	requireReadOnlyTestnet(t)
	client := newReadOnlyTestnetClient(t)
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	subscription, err := client.WebSocket.SubscribeAllMids(ctx, websocket.AllMidsRequest{})
	if err != nil {
		t.Fatalf("subscribe testnet all mids: %v", err)
	}
	defer func() { _ = subscription.Close() }()

	select {
	case event, ok := <-subscription.Events():
		if !ok {
			t.Fatal("testnet all mids subscription closed before an event")
		}
		if len(event.Mids) == 0 {
			t.Fatal("testnet all mids event contains no markets")
		}
	case err := <-subscription.Errors():
		if err == nil {
			t.Fatal("testnet websocket reported a nil error")
		}
		t.Fatalf("testnet all mids subscription: %v", err)
	case <-ctx.Done():
		if errors.Is(ctx.Err(), context.DeadlineExceeded) {
			t.Fatal("timed out waiting for a public testnet websocket event")
		}
		t.Fatalf("testnet websocket context: %v", ctx.Err())
	}
}
