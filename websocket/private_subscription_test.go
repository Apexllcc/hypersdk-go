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

func TestPrivateSubscriptionsDecodeOfficialPayloads(t *testing.T) {
	upgrader := gws.Upgrader{}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		connection, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			t.Error(err)
			return
		}
		defer func() { _ = connection.Close() }()
		for i := 0; i < 5; i++ {
			var request struct {
				Subscription struct {
					Type string `json:"type"`
				} `json:"subscription"`
			}
			if err := connection.ReadJSON(&request); err != nil {
				t.Error(err)
				return
			}
			switch request.Subscription.Type {
			case "userEvents":
				_ = connection.WriteJSON(map[string]any{"channel": "userEvents", "data": map[string]any{"funding": map[string]any{"time": 1, "coin": "BTC", "usdc": "2.3", "szi": "4.5", "fundingRate": "0.001", "nSamples": 2}}})
			case "orderUpdates":
				_ = connection.WriteJSON(map[string]any{"channel": "orderUpdates", "data": []map[string]any{{"order": map[string]any{"coin": "BTC", "side": "B", "limitPx": "10.1", "sz": "2.3", "oid": 9, "timestamp": 1, "origSz": "3.0", "cloid": "0x1"}, "status": "open", "statusTimestamp": 2}}})
			case "userFills":
				_ = connection.WriteJSON(map[string]any{"channel": "userFills", "data": map[string]any{"user": "0xabc", "isSnapshot": true, "fills": []map[string]any{{"coin": "BTC", "px": "1.2", "sz": "3.4", "side": "B", "time": 1, "startPosition": "0", "dir": "Open Long", "closedPnl": "0", "hash": "0xabc", "oid": 2, "crossed": true, "fee": "0.01", "tid": 3, "feeToken": "USDC"}}}})
			case "userFundings":
				_ = connection.WriteJSON(map[string]any{"channel": "userFundings", "data": map[string]any{"user": "0xabc", "isSnapshot": true, "fundings": []map[string]any{{"time": 1, "coin": "BTC", "usdc": "2.3", "szi": "4.5", "fundingRate": "0.001", "nSamples": 2}}}})
			case "userNonFundingLedgerUpdates":
				_ = connection.WriteJSON(map[string]any{"channel": "userNonFundingLedgerUpdates", "data": map[string]any{"user": "0xabc", "isSnapshot": true, "nonFundingLedgerUpdates": []map[string]any{{"time": 1, "hash": "0xledger", "delta": map[string]any{"type": "withdraw", "usdc": "2.3", "nonce": 7, "fee": "0.1"}}}}})
			default:
				t.Errorf("unexpected subscription %q", request.Subscription.Type)
			}
		}
		<-time.After(time.Second)
	}))
	defer server.Close()

	client := websocket.NewClient("ws" + strings.TrimPrefix(server.URL, "http"))
	defer func() { _ = client.Close() }()
	userEvents, err := client.SubscribeUserEvents(context.Background(), "0xabc")
	if err != nil {
		t.Fatal(err)
	}
	orderUpdates, err := client.SubscribeOrderUpdates(context.Background(), "0xabc")
	if err != nil {
		t.Fatal(err)
	}
	userFills, err := client.SubscribeUserFills(context.Background(), websocket.UserFillsRequest{User: "0xabc"})
	if err != nil {
		t.Fatal(err)
	}
	userFundings, err := client.SubscribeUserFundings(context.Background(), "0xabc")
	if err != nil {
		t.Fatal(err)
	}
	ledger, err := client.SubscribeUserNonFundingLedgerUpdates(context.Background(), "0xabc")
	if err != nil {
		t.Fatal(err)
	}

	select {
	case event := <-userEvents.Events():
		if event.Funding == nil || event.Funding.USDC.String() != "2.3" || event.Funding.Samples == nil || *event.Funding.Samples != 2 {
			t.Fatalf("user event = %+v", event)
		}
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for user event")
	}
	select {
	case events := <-orderUpdates.Events():
		if len(events) != 1 || events[0].Order.LimitPrice.String() != "10.1" || events[0].Order.Cloid != "0x1" {
			t.Fatalf("orders = %+v", events)
		}
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for order update")
	}
	select {
	case event := <-userFills.Events():
		if !event.IsSnapshot || event.User != "0xabc" || len(event.Fills) != 1 || event.Fills[0].Price.String() != "1.2" {
			t.Fatalf("fills = %+v", event)
		}
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for fills")
	}
	select {
	case event := <-userFundings.Events():
		if !event.IsSnapshot || len(event.Fundings) != 1 || event.Fundings[0].Samples == nil || *event.Fundings[0].Samples != 2 {
			t.Fatalf("fundings = %+v", event)
		}
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for fundings")
	}
	select {
	case event := <-ledger.Events():
		if !event.IsSnapshot || len(event.Updates) != 1 || event.Updates[0].Delta.Withdraw == nil || event.Updates[0].Delta.Withdraw.Fee.String() != "0.1" {
			t.Fatalf("ledger = %+v", event)
		}
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for ledger")
	}
}

func TestPrivateSubscriptionReconnectsAndResubscribes(t *testing.T) {
	upgrader := gws.Upgrader{}
	connections := atomic.Int32{}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		connection, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			t.Error(err)
			return
		}
		defer func() { _ = connection.Close() }()
		if err := connection.SetReadDeadline(time.Now().Add(time.Second)); err != nil {
			t.Error(err)
			return
		}
		var request struct {
			Subscription struct{ Type, User string } `json:"subscription"`
		}
		if err := connection.ReadJSON(&request); err != nil {
			t.Error(err)
			return
		}
		if request.Subscription.Type != "userFills" || request.Subscription.User != "0xabc" {
			t.Errorf("request=%+v", request)
		}
		if connections.Add(1) == 2 {
			_ = connection.WriteJSON(map[string]any{"channel": "userFills", "data": map[string]any{"user": "0xabc", "fills": []map[string]any{}}})
			<-time.After(time.Second)
		}
	}))
	defer server.Close()
	client := websocket.NewClient("ws"+strings.TrimPrefix(server.URL, "http"), websocket.Config{ReconnectDelay: time.Millisecond})
	defer func() { _ = client.Close() }()
	subscription, err := client.SubscribeUserFills(context.Background(), websocket.UserFillsRequest{User: "0xabc"})
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = subscription.Close() }()
	select {
	case event := <-subscription.Events():
		if event.User != "0xabc" {
			t.Fatalf("event=%+v", event)
		}
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for resubscribed private event")
	}
	if connections.Load() < 2 {
		t.Fatalf("connections=%d", connections.Load())
	}
}

func TestPrivateUserEventsAndOrderUpdatesCannotBeAmbiguouslyMultiplexed(t *testing.T) {
	client := websocket.NewClient("ws://unused")
	defer func() { _ = client.Close() }()
	userEvents, err := client.SubscribeUserEvents(context.Background(), "0xaaa")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = userEvents.Close() }()
	if _, err := client.SubscribeUserEvents(context.Background(), "0xbbb"); err == nil {
		t.Fatal("expected user event multiplexing rejection")
	}
	orderUpdates, err := client.SubscribeOrderUpdates(context.Background(), "0xaaa")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = orderUpdates.Close() }()
	if _, err := client.SubscribeOrderUpdates(context.Background(), "0xbbb"); err == nil {
		t.Fatal("expected order update multiplexing rejection")
	}
	userFills, err := client.SubscribeUserFills(context.Background(), websocket.UserFillsRequest{User: "0xaaa"})
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = userFills.Close() }()
	if _, err := client.SubscribeUserFills(context.Background(), websocket.UserFillsRequest{User: "0xaaa", AggregateByTime: true}); err == nil {
		t.Fatal("expected conflicting user fills aggregation rejection")
	}
}
