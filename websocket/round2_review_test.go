package websocket_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/Apexllcc/hypersdk-go/websocket"
	gws "github.com/gorilla/websocket"
)

type cancelLastWireAdmissionLimiter struct {
	calls    atomic.Int32
	entered  chan struct{}
	canceled chan struct{}
}

func (l *cancelLastWireAdmissionLimiter) Wait(ctx context.Context) error {
	switch l.calls.Add(1) {
	case 1:
		return nil
	case 2:
		close(l.entered)
		<-ctx.Done()
		close(l.canceled)
		return ctx.Err()
	default:
		select {
		case <-l.canceled:
			return nil
		case <-ctx.Done():
			return ctx.Err()
		}
	}
}

func TestStringErrorBeforeGoodAckRejectsOnlyEmbeddedBadRequest(t *testing.T) {
	upgrader := gws.Upgrader{}
	keepOpen := make(chan struct{})
	var connections atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		connection, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			t.Errorf("upgrade: %v", err)
			return
		}
		defer func() { _ = connection.Close() }()
		connections.Add(1)
		byCoin := make(map[string]map[string]any)
		for range 2 {
			var request struct {
				Subscription map[string]any `json:"subscription"`
			}
			if err := connection.ReadJSON(&request); err != nil {
				return
			}
			coin, _ := request.Subscription["coin"].(string)
			byCoin[coin] = request.Subscription
		}
		echoedRequest, _ := json.Marshal(map[string]any{"method": "subscribe", "subscription": byCoin["BAD"]})
		if err := connection.WriteJSON(map[string]any{
			"channel": "error",
			"data":    "Invalid subscription: " + string(echoedRequest),
		}); err != nil {
			return
		}
		if err := connection.WriteJSON(subscriptionResponse("subscribe", byCoin["GOOD"])); err != nil {
			return
		}
		if err := connection.WriteJSON(map[string]any{
			"channel": "trades",
			"data": []map[string]any{{
				"coin": "GOOD", "side": "B", "px": "1", "sz": "2", "hash": "0x1", "time": 1, "tid": 2, "users": []string{"0xa", "0xb"},
			}},
		}); err != nil {
			return
		}
		<-keepOpen
	}))
	defer func() {
		close(keepOpen)
		server.Close()
	}()

	client := websocket.NewClient("ws"+strings.TrimPrefix(server.URL, "http"), websocket.Config{ReconnectDelay: time.Millisecond})
	defer func() { _ = client.Close() }()
	bad, err := client.SubscribeTrades(context.Background(), websocket.TradesRequest{Coin: "BAD"})
	if err != nil {
		t.Fatal(err)
	}
	good, err := client.SubscribeTrades(context.Background(), websocket.TradesRequest{Coin: "GOOD"})
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = good.Close() }()
	waitForStateError(t, bad.States(), websocket.ErrSubscriptionRejected)
	waitForSubscribed(t, good.States())
	select {
	case events, ok := <-good.Events():
		if !ok || len(events) != 1 || events[0].Coin != "GOOD" {
			t.Fatalf("good events = %#v, open=%t", events, ok)
		}
	case <-time.After(time.Second):
		t.Fatal("good subscription stopped receiving after BAD string rejection")
	}
	if got := connections.Load(); got != 1 {
		t.Fatalf("string rejection disrupted shared connection; connections=%d", got)
	}
}

func TestSpotStateTrueMatchesIgnorePortfolioMarginFalseForSubscribeAndUnsubscribe(t *testing.T) {
	upgrader := gws.Upgrader{}
	unsubscribed := make(chan struct{}, 1)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		connection, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			t.Errorf("upgrade: %v", err)
			return
		}
		defer func() { _ = connection.Close() }()
		var request struct {
			Method       string         `json:"method"`
			Subscription map[string]any `json:"subscription"`
		}
		if err := connection.ReadJSON(&request); err != nil {
			return
		}
		if request.Method != "subscribe" || request.Subscription["isPortfolioMargin"] != true {
			t.Errorf("spotState subscribe = %#v", request)
			return
		}
		if _, invented := request.Subscription["ignorePortfolioMargin"]; invented {
			t.Errorf("spotState request invented ignorePortfolioMargin: %#v", request.Subscription)
			return
		}
		normalized := map[string]any{"type": "spotState", "user": "0xabcdef", "ignorePortfolioMargin": false}
		if err := connection.WriteJSON(subscriptionResponse("subscribe", normalized)); err != nil {
			return
		}
		if err := connection.ReadJSON(&request); err != nil {
			return
		}
		if request.Method != "unsubscribe" || request.Subscription["isPortfolioMargin"] != true {
			t.Errorf("spotState unsubscribe = %#v", request)
			return
		}
		if _, invented := request.Subscription["ignorePortfolioMargin"]; invented {
			t.Errorf("spotState unsubscribe invented ignorePortfolioMargin: %#v", request.Subscription)
			return
		}
		if err := connection.WriteJSON(subscriptionResponse("unsubscribe", normalized)); err != nil {
			return
		}
		unsubscribed <- struct{}{}
	}))
	defer server.Close()

	explicitTrue := true
	client := websocket.NewClient("ws"+strings.TrimPrefix(server.URL, "http"), websocket.Config{SubscriptionAckTimeout: 100 * time.Millisecond})
	defer func() { _ = client.Close() }()
	subscription, err := client.SubscribeSpotState(context.Background(), websocket.SpotStateRequest{User: "0xABCDEF", IsPortfolioMargin: &explicitTrue})
	if err != nil {
		t.Fatal(err)
	}
	waitForSubscribed(t, subscription.States())
	if err := subscription.Close(); err != nil {
		t.Fatal(err)
	}
	select {
	case <-unsubscribed:
	case <-time.After(time.Second):
		t.Fatal("normalized spotState unsubscribe was not acknowledged")
	}
}

func TestClosingLastWireReferenceCancelsAdmissionAndContinuesUnrelatedWork(t *testing.T) {
	upgrader := gws.Upgrader{}
	wires := make(chan string, 3)
	var connections atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		connection, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			t.Errorf("upgrade: %v", err)
			return
		}
		defer func() { _ = connection.Close() }()
		connections.Add(1)
		for {
			var request struct {
				Method       string         `json:"method"`
				Subscription map[string]any `json:"subscription"`
			}
			if err := connection.ReadJSON(&request); err != nil {
				return
			}
			coin, _ := request.Subscription["coin"].(string)
			if request.Method == "subscribe" {
				wires <- coin
			}
			if err := connection.WriteJSON(subscriptionResponse(request.Method, request.Subscription)); err != nil {
				return
			}
		}
	}))
	defer server.Close()

	limiter := &cancelLastWireAdmissionLimiter{entered: make(chan struct{}), canceled: make(chan struct{})}
	client := websocket.NewClient("ws"+strings.TrimPrefix(server.URL, "http"), websocket.Config{MessageAdmission: limiter, PingInterval: time.Hour})
	defer func() { _ = client.Close() }()
	first, err := client.SubscribeTrades(context.Background(), websocket.TradesRequest{Coin: "FIRST"})
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = first.Close() }()
	waitForSubscribed(t, first.States())
	canceled, err := client.SubscribeTrades(context.Background(), websocket.TradesRequest{Coin: "CANCELED"})
	if err != nil {
		t.Fatal(err)
	}
	select {
	case <-limiter.entered:
	case <-time.After(time.Second):
		t.Fatal("second wire did not enter message admission")
	}
	if err := canceled.Close(); err != nil {
		t.Fatal(err)
	}
	select {
	case <-limiter.canceled:
	case <-time.After(time.Second):
		t.Fatal("closing last wire reference did not cancel message admission")
	}
	third, err := client.SubscribeTrades(context.Background(), websocket.TradesRequest{Coin: "THIRD"})
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = third.Close() }()
	waitForSubscribed(t, third.States())
	want := []string{"FIRST", "THIRD"}
	for index, expected := range want {
		select {
		case coin := <-wires:
			if coin != expected {
				t.Fatalf("wire %d = %q, want %q", index, coin, expected)
			}
		case <-time.After(time.Second):
			t.Fatalf("missing wire %d (%s)", index, expected)
		}
	}
	select {
	case coin := <-wires:
		t.Fatalf("stale subscription reached wire as %q", coin)
	case <-time.After(30 * time.Millisecond):
	}
	if got := connections.Load(); got != 1 {
		t.Fatalf("unrelated work required reconnect; connections=%d", got)
	}
}

func TestCorrelatedUnsubscribeRejectionClearsExactPendingWithoutReconnect(t *testing.T) {
	upgrader := gws.Upgrader{}
	keepOpen := make(chan struct{})
	var connections atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		connection, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			t.Errorf("upgrade: %v", err)
			return
		}
		defer func() { _ = connection.Close() }()
		generation := connections.Add(1)
		if generation != 1 {
			<-keepOpen
			return
		}
		byCoin := make(map[string]map[string]any)
		for range 2 {
			var request struct {
				Subscription map[string]any `json:"subscription"`
			}
			if err := connection.ReadJSON(&request); err != nil {
				return
			}
			coin, _ := request.Subscription["coin"].(string)
			byCoin[coin] = request.Subscription
		}
		if err := connection.WriteJSON(subscriptionResponse("subscribe", byCoin["DROP"])); err != nil {
			return
		}
		if err := connection.WriteJSON(subscriptionResponse("subscribe", byCoin["GOOD"])); err != nil {
			return
		}
		var unsubscribe struct {
			Method       string         `json:"method"`
			Subscription map[string]any `json:"subscription"`
		}
		if err := connection.ReadJSON(&unsubscribe); err != nil {
			return
		}
		if unsubscribe.Method != "unsubscribe" || unsubscribe.Subscription["coin"] != "DROP" {
			t.Errorf("unsubscribe request = %#v", unsubscribe)
			return
		}
		if err := connection.WriteJSON(map[string]any{
			"channel": "error",
			"data": map[string]any{
				"error":   "subscription was already removed",
				"request": map[string]any{"method": "unsubscribe", "subscription": unsubscribe.Subscription},
			},
		}); err != nil {
			return
		}
		time.Sleep(120 * time.Millisecond)
		if err := connection.WriteJSON(map[string]any{
			"channel": "trades",
			"data": []map[string]any{{
				"coin": "GOOD", "side": "B", "px": "1", "sz": "2", "hash": "0x2", "time": 2, "tid": 3, "users": []string{"0xa", "0xb"},
			}},
		}); err != nil {
			return
		}
		<-keepOpen
	}))
	defer func() {
		close(keepOpen)
		server.Close()
	}()

	client := websocket.NewClient("ws"+strings.TrimPrefix(server.URL, "http"), websocket.Config{
		SubscriptionAckTimeout: 40 * time.Millisecond,
		ReconnectDelay:         time.Millisecond,
		PingInterval:           time.Hour,
	})
	defer func() { _ = client.Close() }()
	drop, err := client.SubscribeTrades(context.Background(), websocket.TradesRequest{Coin: "DROP"})
	if err != nil {
		t.Fatal(err)
	}
	good, err := client.SubscribeTrades(context.Background(), websocket.TradesRequest{Coin: "GOOD"})
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = good.Close() }()
	waitForSubscribed(t, drop.States())
	waitForSubscribed(t, good.States())
	if err := drop.Close(); err != nil {
		t.Fatal(err)
	}
	select {
	case events, ok := <-good.Events():
		if !ok || len(events) != 1 || events[0].Coin != "GOOD" {
			t.Fatalf("good events = %#v, open=%t", events, ok)
		}
	case <-time.After(time.Second):
		t.Fatal("GOOD stopped after correlated unsubscribe rejection")
	}
	if got := connections.Load(); got != 1 {
		t.Fatalf("unsubscribe rejection triggered reconnect; connections=%d", got)
	}
}
