//go:build integration && testnet

package integration

import (
	"context"
	"testing"
	"time"

	"github.com/Apexllcc/hypersdk-go/info"
	"github.com/Apexllcc/hypersdk-go/transport"
	"github.com/Apexllcc/hypersdk-go/websocket"
)

// testnetWSSubscription is the lifecycle shared by every market and account
// subscription handle. The server acknowledgement is the meaningful protocol
// compatibility assertion: private feeds such as notification and TWAP may
// legitimately have no event while the subscribed account is idle.
type testnetWSSubscription interface {
	Close() error
	Errors() <-chan error
	States() <-chan websocket.SubscriptionStateEvent
}

// TestTestnetWebSocketSubscriptionSurface confirms Testnet accepts every
// market and account subscription exposed by the SDK. It is read-only: no
// signing key is created, and the sole user input is the account address used
// for public account snapshots. Each subscription is closed before the next
// one so this test remains well below the platform's active-subscription cap.
func TestTestnetWebSocketSubscriptionSurface(t *testing.T) {
	address := requireReadOnlyTestnet(t)
	client := newReadOnlyTestnetClient(t)
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()

	const (
		perpCoin = "BTC"
		spotCoin = "PURR/USDC"
	)
	probes := []struct {
		name      string
		subscribe func() (testnetWSSubscription, error)
	}{
		{"l2Book", func() (testnetWSSubscription, error) {
			return client.WebSocket.SubscribeL2Book(ctx, websocket.L2BookRequest{Coin: perpCoin})
		}},
		{"allMids", func() (testnetWSSubscription, error) {
			return client.WebSocket.SubscribeAllMids(ctx, websocket.AllMidsRequest{})
		}},
		{"trades", func() (testnetWSSubscription, error) {
			return client.WebSocket.SubscribeTrades(ctx, websocket.TradesRequest{Coin: perpCoin})
		}},
		{"candle", func() (testnetWSSubscription, error) {
			return client.WebSocket.SubscribeCandle(ctx, websocket.CandleRequest{Coin: perpCoin, Interval: "1m"})
		}},
		{"bbo", func() (testnetWSSubscription, error) {
			return client.WebSocket.SubscribeBBO(ctx, websocket.BBORequest{Coin: perpCoin})
		}},
		{"activeAssetCtx/perp", func() (testnetWSSubscription, error) {
			return client.WebSocket.SubscribeActiveAssetCtx(ctx, websocket.ActiveAssetCtxRequest{Coin: perpCoin})
		}},
		{"activeAssetCtx/spot", func() (testnetWSSubscription, error) {
			return client.WebSocket.SubscribeActiveSpotAssetCtx(ctx, spotCoin)
		}},
		{"assetCtxs", func() (testnetWSSubscription, error) {
			return client.WebSocket.SubscribeAssetCtxs(ctx, websocket.DEXRequest{})
		}},
		{"fastAssetCtxs", func() (testnetWSSubscription, error) { return client.WebSocket.SubscribeFastAssetCtxs(ctx) }},
		{"spotAssetCtxs", func() (testnetWSSubscription, error) { return client.WebSocket.SubscribeSpotAssetCtxs(ctx) }},
		{"outcomeMetaUpdates", func() (testnetWSSubscription, error) { return client.WebSocket.SubscribeOutcomeMetaUpdates(ctx) }},
		{"userEvents", func() (testnetWSSubscription, error) { return client.WebSocket.SubscribeUserEvents(ctx, address) }},
		{"orderUpdates", func() (testnetWSSubscription, error) { return client.WebSocket.SubscribeOrderUpdates(ctx, address) }},
		{"userFills", func() (testnetWSSubscription, error) {
			return client.WebSocket.SubscribeUserFills(ctx, websocket.UserFillsRequest{User: address})
		}},
		{"userFundings", func() (testnetWSSubscription, error) { return client.WebSocket.SubscribeUserFundings(ctx, address) }},
		{"userNonFundingLedgerUpdates", func() (testnetWSSubscription, error) {
			return client.WebSocket.SubscribeUserNonFundingLedgerUpdates(ctx, address)
		}},
		{"notification", func() (testnetWSSubscription, error) { return client.WebSocket.SubscribeNotification(ctx, address) }},
		{"webData3", func() (testnetWSSubscription, error) { return client.WebSocket.SubscribeWebData3(ctx, address) }},
		{"openOrders", func() (testnetWSSubscription, error) {
			return client.WebSocket.SubscribeOpenOrders(ctx, websocket.UserDEXRequest{User: address})
		}},
		{"clearinghouseState", func() (testnetWSSubscription, error) {
			return client.WebSocket.SubscribeClearinghouseState(ctx, websocket.UserDEXRequest{User: address})
		}},
		{"activeAssetData", func() (testnetWSSubscription, error) {
			return client.WebSocket.SubscribeActiveAssetData(ctx, websocket.ActiveAssetDataRequest{Coin: perpCoin, User: address})
		}},
		{"twapStates", func() (testnetWSSubscription, error) {
			return client.WebSocket.SubscribeTWAPStates(ctx, websocket.UserDEXRequest{User: address})
		}},
		{"userTwapSliceFills", func() (testnetWSSubscription, error) {
			return client.WebSocket.SubscribeUserTWAPSliceFills(ctx, address)
		}},
		{"userTwapHistory", func() (testnetWSSubscription, error) { return client.WebSocket.SubscribeUserTWAPHistory(ctx, address) }},
		{"spotState", func() (testnetWSSubscription, error) {
			return client.WebSocket.SubscribeSpotState(ctx, websocket.SpotStateRequest{User: address})
		}},
		{"allDexsClearinghouseState", func() (testnetWSSubscription, error) {
			return client.WebSocket.SubscribeAllDEXsClearinghouseState(ctx, address)
		}},
		{"allDexsAssetCtxs", func() (testnetWSSubscription, error) { return client.WebSocket.SubscribeAllDEXsAssetCtxs(ctx) }},
		{"userHistoricalOrders", func() (testnetWSSubscription, error) {
			return client.WebSocket.SubscribeUserHistoricalOrders(ctx, address)
		}},
	}

	for _, probe := range probes {
		t.Run(probe.name, func(t *testing.T) {
			subscription, err := probe.subscribe()
			if err != nil {
				t.Fatalf("subscribe %s: %v", probe.name, err)
			}
			t.Cleanup(func() { _ = subscription.Close() })
			waitForTestnetWSSubscribed(t, ctx, probe.name, subscription)
		})
	}
}

// TestTestnetWebSocketReadOnlyRequestSurface verifies both public request
// spellings over the official WebSocket post protocol. PostAction is excluded:
// it is deliberately an Exchange mutation path and this build-tagged suite is
// read-only.
func TestTestnetWebSocketReadOnlyRequestSurface(t *testing.T) {
	requireReadOnlyTestnet(t)
	client := newReadOnlyTestnetClient(t)
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()

	request := info.AllMidsRequest{Type: "allMids"}
	var postInfoResponse info.AllMidsResponse
	if err := client.WebSocket.PostInfo(ctx, request, &postInfoResponse); err != nil {
		t.Fatalf("PostInfo allMids: %v", err)
	}
	if len(postInfoResponse) == 0 {
		t.Fatal("PostInfo allMids returned no markets")
	}

	var requestResponse info.AllMidsResponse
	if err := client.WebSocket.Request(ctx, transport.RequestInfo, request, &requestResponse); err != nil {
		t.Fatalf("Request info allMids: %v", err)
	}
	if len(requestResponse) == 0 {
		t.Fatal("Request info allMids returned no markets")
	}
}

func waitForTestnetWSSubscribed(t *testing.T, ctx context.Context, name string, subscription testnetWSSubscription) {
	t.Helper()
	for {
		select {
		case state, ok := <-subscription.States():
			if !ok {
				t.Fatalf("%s states closed before subscribed", name)
			}
			switch state.State {
			case websocket.SubscriptionStateSubscribed:
				return
			case websocket.SubscriptionStateError:
				t.Fatalf("%s subscription rejected: %v", name, state.Error)
			}
		case err, ok := <-subscription.Errors():
			if !ok {
				continue
			}
			if err != nil {
				t.Fatalf("%s subscription error: %v", name, err)
			}
		case <-ctx.Done():
			t.Fatalf("%s subscription acknowledgement: %v", name, ctx.Err())
		}
	}
}

// TestTestnetExplorerWebSocketSubscriptionSurface validates the two Explorer
// RPC streams separately because they use the dedicated Explorer WebSocket
// endpoint, not the market API endpoint. Explorer produces block batches
// continuously; an event proves both subscription acknowledgement and the
// raw-array decoder path.
func TestTestnetExplorerWebSocketSubscriptionSurface(t *testing.T) {
	requireReadOnlyTestnet(t)
	client := newReadOnlyTestnetClient(t)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	blockSubscription, err := client.Explorer.ExplorerBlock(ctx)
	if err != nil {
		t.Fatalf("subscribe explorerBlock: %v", err)
	}
	defer func() { _ = blockSubscription.Close() }()
	waitForTestnetWSSubscribed(t, ctx, "explorerBlock", blockSubscription)
	waitForTestnetExplorerEvent(t, ctx, "explorerBlock", blockSubscription.Events(), blockSubscription.Errors())

	txSubscription, err := client.Explorer.ExplorerTxs(ctx)
	if err != nil {
		t.Fatalf("subscribe explorerTxs: %v", err)
	}
	defer func() { _ = txSubscription.Close() }()
	waitForTestnetWSSubscribed(t, ctx, "explorerTxs", txSubscription)
	waitForTestnetExplorerEvent(t, ctx, "explorerTxs", txSubscription.Events(), txSubscription.Errors())
}

func waitForTestnetExplorerEvent[T any](t *testing.T, ctx context.Context, name string, events <-chan T, errors <-chan error) {
	t.Helper()
	select {
	case _, ok := <-events:
		if !ok {
			t.Fatalf("%s events closed before an event", name)
		}
	case err, ok := <-errors:
		if ok && err != nil {
			t.Fatalf("%s subscription error: %v", name, err)
		}
		t.Fatalf("%s errors closed before an event", name)
	case <-ctx.Done():
		t.Fatalf("%s event: %v", name, ctx.Err())
	}
}
