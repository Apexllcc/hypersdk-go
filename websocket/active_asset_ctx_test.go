package websocket_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/Apexllcc/hyperliquid-go-sdk/websocket"
	gws "github.com/gorilla/websocket"
)

func TestActiveAssetCtxSubscriptionDecodesPerpAndSpotContexts(t *testing.T) {
	t.Parallel()
	upgrader := gws.Upgrader{}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		conn, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			t.Error(err)
			return
		}
		defer func() { _ = conn.Close() }()
		var request struct {
			Method       string `json:"method"`
			Subscription struct {
				Type string `json:"type"`
				Coin string `json:"coin"`
			} `json:"subscription"`
		}
		if err := conn.ReadJSON(&request); err != nil {
			t.Error(err)
			return
		}
		if request.Method != "subscribe" || request.Subscription.Type != "activeAssetCtx" || request.Subscription.Coin != "xyz:BTC" {
			t.Errorf("subscription=%+v", request)
			return
		}
		_ = conn.WriteJSON(map[string]any{"channel": "activeAssetCtx", "data": map[string]any{
			"coin": "xyz:BTC", "ctx": map[string]string{
				"dayNtlVlm": "12.3", "prevDayPx": "9", "markPx": "10", "midPx": "10.1",
				"funding": "0.0001", "openInterest": "11", "oraclePx": "10.2",
			},
		}})
		_ = conn.WriteJSON(map[string]any{"channel": "activeAssetCtx", "data": map[string]any{
			"coin": "xyz:BTC", "ctx": map[string]string{
				"dayNtlVlm": "12.3", "prevDayPx": "9", "markPx": "10", "circulatingSupply": "21000000",
			},
		}})
		<-time.After(time.Second)
	}))
	defer server.Close()

	client := websocket.NewClient("ws" + strings.TrimPrefix(server.URL, "http"))
	defer func() { _ = client.Close() }()
	subscription, err := client.SubscribeActiveAssetCtx(context.Background(), websocket.ActiveAssetCtxRequest{Coin: "xyz:BTC"})
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = subscription.Close() }()

	for wantPerp := true; ; wantPerp = false {
		select {
		case event := <-subscription.Events():
			if event.Coin != "xyz:BTC" {
				t.Fatalf("coin=%q", event.Coin)
			}
			if wantPerp {
				if event.Perp == nil || event.Spot != nil || event.Perp.MarkPrice.String() != "10" || event.Perp.Funding.String() != "0.0001" {
					t.Fatalf("perp event=%+v", event)
				}
				continue
			}
			if event.Spot == nil || event.Perp != nil || event.Spot.CirculatingSupply.String() != "21000000" {
				t.Fatalf("spot event=%+v", event)
			}
			return
		case <-time.After(time.Second):
			t.Fatal("timed out waiting for active asset context")
		}
	}
}

func TestL2BookLevelsRetainExactDecimalValues(t *testing.T) {
	var connections atomic.Int32
	upgrader := gws.Upgrader{}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		conn, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			t.Error(err)
			return
		}
		defer func() { _ = conn.Close() }()
		if _, _, err := conn.ReadMessage(); err != nil {
			t.Error(err)
			return
		}
		connections.Add(1)
		_ = conn.WriteJSON(map[string]any{"channel": "l2Book", "data": map[string]any{
			"coin": "BTC", "time": 1, "spread": "0.123456789012345678",
			"levels": []any{
				[]map[string]any{{"px": "1.234567890123456789", "sz": "2.34567890123456789", "n": 2}},
				[]map[string]any{},
			},
		}})
		<-time.After(time.Second)
	}))
	defer server.Close()

	client := websocket.NewClient("ws" + strings.TrimPrefix(server.URL, "http"))
	defer func() { _ = client.Close() }()
	subscription, err := client.SubscribeL2Book(context.Background(), websocket.L2BookRequest{Coin: "BTC"})
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = subscription.Close() }()
	select {
	case event := <-subscription.Events():
		if len(event.Levels[0]) != 1 || event.Levels[0][0].Price.String() != "1.234567890123456789" || event.Levels[0][0].Size.String() != "2.34567890123456789" || event.Spread == nil || event.Spread.String() != "0.123456789012345678" {
			t.Fatalf("levels=%+v", event.Levels)
		}
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for L2 book")
	}
}

func TestSubscriptionStateEventsTrackConnectionAndClose(t *testing.T) {
	upgrader := gws.Upgrader{}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		conn, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			t.Error(err)
			return
		}
		defer func() { _ = conn.Close() }()
		if _, _, err := conn.ReadMessage(); err != nil {
			t.Error(err)
			return
		}
		<-time.After(time.Second)
	}))
	defer server.Close()

	client := websocket.NewClient("ws"+strings.TrimPrefix(server.URL, "http"), websocket.Config{ReconnectDelay: time.Hour})
	defer func() { _ = client.Close() }()
	subscription, err := client.SubscribeTrades(context.Background(), websocket.TradesRequest{Coin: "BTC"})
	if err != nil {
		t.Fatal(err)
	}

	want := []websocket.SubscriptionState{websocket.SubscriptionStateConnecting, websocket.SubscriptionStateConnected, websocket.SubscriptionStateSubscribed}
	for _, state := range want {
		select {
		case event := <-subscription.States():
			if event.State != state {
				t.Fatalf("state=%q, want %q", event.State, state)
			}
		case <-time.After(time.Second):
			t.Fatalf("timed out waiting for %q", state)
		}
	}
	if err := subscription.Close(); err != nil {
		t.Fatal(err)
	}
	select {
	case event, ok := <-subscription.States():
		if !ok || event.State != websocket.SubscriptionStateUnsubscribed {
			t.Fatalf("close state=%+v, open=%t", event, ok)
		}
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for unsubscribe state")
	}
}

func TestSubscriptionStateEventsTrackReconnectAndRestore(t *testing.T) {
	upgrader := gws.Upgrader{}
	var connections atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		conn, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			t.Error(err)
			return
		}
		defer func() { _ = conn.Close() }()
		if _, _, err := conn.ReadMessage(); err != nil {
			t.Error(err)
			return
		}
		if connections.Add(1) == 1 {
			return // force a server-side disconnect after the initial subscription
		}
		<-time.After(time.Second)
	}))
	defer server.Close()

	client := websocket.NewClient("ws"+strings.TrimPrefix(server.URL, "http"), websocket.Config{ReconnectDelay: time.Millisecond})
	defer func() { _ = client.Close() }()
	subscription, err := client.SubscribeTrades(context.Background(), websocket.TradesRequest{Coin: "BTC"})
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = subscription.Close() }()

	for _, state := range []websocket.SubscriptionState{websocket.SubscriptionStateConnecting, websocket.SubscriptionStateConnected, websocket.SubscriptionStateSubscribed} {
		select {
		case event := <-subscription.States():
			if event.State != state {
				t.Fatalf("initial state=%q, want %q", event.State, state)
			}
		case <-time.After(time.Second):
			t.Fatalf("timed out waiting for initial %q", state)
		}
	}
	seen := map[websocket.SubscriptionState]bool{}
	deadline := time.After(time.Second)
	for !seen[websocket.SubscriptionStateError] || !seen[websocket.SubscriptionStateReconnecting] || !seen[websocket.SubscriptionStateConnected] || !seen[websocket.SubscriptionStateSubscribed] {
		select {
		case event := <-subscription.States():
			seen[event.State] = true
		case <-deadline:
			t.Fatalf("missing reconnect lifecycle states: %+v", seen)
		}
	}
	for connections.Load() < 2 {
		select {
		case <-deadline:
			t.Fatalf("connections=%d", connections.Load())
		case <-time.After(time.Millisecond):
		}
	}
}

func TestUnconsumedStateEventsCannotStopReconnect(t *testing.T) {
	upgrader := gws.Upgrader{}
	var connections atomic.Int32
	const disconnects = 32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		conn, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			t.Error(err)
			return
		}
		defer func() { _ = conn.Close() }()
		if _, _, err := conn.ReadMessage(); err != nil {
			t.Error(err)
			return
		}
		if connections.Add(1) <= disconnects {
			return
		}
		if err := conn.WriteJSON(map[string]any{"channel": "trades", "data": []map[string]any{{"coin": "BTC", "side": "B", "px": "1", "sz": "2", "hash": "0x1", "time": 1, "tid": 2, "users": []string{"0xa", "0xb"}}}}); err != nil {
			t.Error(err)
		}
		<-time.After(time.Second)
	}))
	defer server.Close()

	client := websocket.NewClient("ws"+strings.TrimPrefix(server.URL, "http"), websocket.Config{ReconnectDelay: time.Millisecond, StateBuffer: 2})
	defer func() { _ = client.Close() }()
	subscription, err := client.SubscribeTrades(context.Background(), websocket.TradesRequest{Coin: "BTC"})
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = subscription.Close() }()

	// Intentionally do not read subscription.States(). Lifecycle observability
	// must not be a precondition for a shared connection to recover.
	select {
	case events := <-subscription.Events():
		if len(events) != 1 || events[0].Price.String() != "1" {
			t.Fatalf("events=%+v", events)
		}
	case <-time.After(2 * time.Second):
		t.Fatalf("reconnect stalled after %d connections", connections.Load())
	}
	if connections.Load() <= disconnects {
		t.Fatalf("connections=%d", connections.Load())
	}
}
