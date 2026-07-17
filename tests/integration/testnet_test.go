//go:build integration && testnet

package integration

import (
	"context"
	"errors"
	"os"
	"testing"
	"time"

	hyperliquid "github.com/Apexllcc/hypersdk-go"
	"github.com/Apexllcc/hypersdk-go/websocket"
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

// TestTestnetAccountInfoReadSurface exercises every account-address Info
// endpoint that needs no user-created Vault, subaccount, agent, builder, or
// staking/borrow-lend position. Empty business arrays are valid; a successful
// request still verifies the typed request/response path against Testnet.
func TestTestnetAccountInfoReadSurface(t *testing.T) {
	address := requireReadOnlyTestnet(t)
	client := newReadOnlyTestnetClient(t)
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()
	startTime := time.Now().Add(-30 * 24 * time.Hour).UnixMilli()

	probes := []struct {
		name string
		run  func() error
	}{
		{"clearinghouseState", func() error { _, err := client.Info.ClearinghouseState(ctx, address); return err }},
		{"spotClearinghouseState", func() error { _, err := client.Info.SpotClearinghouseState(ctx, address); return err }},
		{"activeAssetData", func() error { _, err := client.Info.ActiveAssetData(ctx, address, "BTC"); return err }},
		{"openOrders", func() error { _, err := client.Info.OpenOrders(ctx, address); return err }},
		{"frontendOpenOrders", func() error { _, err := client.Info.FrontendOpenOrders(ctx, address); return err }},
		{"userFills", func() error { _, err := client.Info.UserFills(ctx, address, false); return err }},
		{"userFillsByTime", func() error { _, err := client.Info.UserFillsByTime(ctx, address, startTime, nil, false); return err }},
		{"historicalOrders", func() error { _, err := client.Info.HistoricalOrders(ctx, address); return err }},
		{"portfolio", func() error { _, err := client.Info.Portfolio(ctx, address); return err }},
		{"userFunding", func() error { _, err := client.Info.UserFunding(ctx, address, nil, nil); return err }},
		{"userFees", func() error { _, err := client.Info.UserFees(ctx, address); return err }},
		{"userRateLimit", func() error { _, err := client.Info.UserRateLimit(ctx, address); return err }},
		{"userNonFundingLedgerUpdates", func() error {
			_, err := client.Info.UserNonFundingLedgerUpdates(ctx, address, startTime, nil)
			return err
		}},
		{"userTwapSliceFills", func() error { _, err := client.Info.UserTwapSliceFills(ctx, address); return err }},
		{"userTwapSliceFillsByTime", func() error { _, err := client.Info.UserTwapSliceFillsByTime(ctx, address, startTime, nil); return err }},
		{"twapHistory", func() error { _, err := client.Info.TWAPHistory(ctx, address); return err }},
		{"subaccounts", func() error { _, err := client.Info.Subaccounts(ctx, address); return err }},
		{"subaccounts2", func() error { _, err := client.Info.Subaccounts2(ctx, address); return err }},
		{"userVaultEquities", func() error { _, err := client.Info.UserVaultEquities(ctx, address); return err }},
		{"leadingVaults", func() error { _, err := client.Info.LeadingVaults(ctx, address); return err }},
		{"userRole", func() error { _, err := client.Info.UserRole(ctx, address); return err }},
		{"userAbstraction", func() error { _, err := client.Info.UserAbstraction(ctx, address); return err }},
		{"userDEXAbstraction", func() error { _, err := client.Info.UserDEXAbstraction(ctx, address); return err }},
		{"extraAgents", func() error { _, err := client.Info.ExtraAgents(ctx, address); return err }},
		{"approvedBuilders", func() error { _, err := client.Info.ApprovedBuilders(ctx, address); return err }},
		{"maxBuilderFee/self", func() error { _, err := client.Info.MaxBuilderFee(ctx, address, address); return err }},
		{"isVIP", func() error { _, err := client.Info.IsVIP(ctx, address); return err }},
		{"legalCheck", func() error { _, err := client.Info.LegalCheck(ctx, address); return err }},
		{"referral", func() error { _, err := client.Info.Referral(ctx, address); return err }},
		{"delegatorSummary", func() error { _, err := client.Info.DelegatorSummary(ctx, address); return err }},
		{"delegations", func() error { _, err := client.Info.Delegations(ctx, address); return err }},
		{"delegatorHistory", func() error { _, err := client.Info.DelegatorHistory(ctx, address); return err }},
		{"delegatorRewards", func() error { _, err := client.Info.DelegatorRewards(ctx, address); return err }},
		{"borrowLendUserState", func() error { _, err := client.Info.BorrowLendUserState(ctx, address); return err }},
		{"userBorrowLendInterest", func() error { _, err := client.Info.UserBorrowLendInterest(ctx, address, startTime, nil); return err }},
		{"spotDeployState", func() error { _, err := client.Info.SpotDeployState(ctx, address); return err }},
		{"userToMultiSigSigners", func() error { _, err := client.Info.UserToMultiSigSigners(ctx, address); return err }},
	}

	for _, probe := range probes {
		t.Run(probe.name, func(t *testing.T) {
			if err := probe.run(); err != nil {
				t.Fatal(err)
			}
		})
		select {
		case <-ctx.Done():
			t.Fatal(ctx.Err())
		case <-time.After(100 * time.Millisecond):
		}
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
