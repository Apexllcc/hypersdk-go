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

type gatedWebSocketDialer struct {
	release chan struct{}
}

func (d *gatedWebSocketDialer) DialContext(ctx context.Context, url string) (*gws.Conn, error) {
	select {
	case <-d.release:
		connection, _, err := gws.DefaultDialer.DialContext(ctx, url, nil)
		return connection, err
	case <-ctx.Done():
		return nil, ctx.Err()
	}
}

func TestSpotStateFalseMatchesRealIgnorePortfolioMarginAck(t *testing.T) {
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
		if request.Method != "subscribe" {
			t.Errorf("spotState false subscribe = %#v", request)
			return
		}
		if value, present := request.Subscription["isPortfolioMargin"]; present && value != false {
			t.Errorf("spotState false subscribe changed semantics: %#v", request.Subscription)
			return
		}
		normalized := map[string]any{"type": "spotState", "user": "0xabcdef", "ignorePortfolioMargin": false}
		if err := connection.WriteJSON(subscriptionResponse("subscribe", normalized)); err != nil {
			return
		}
		if err := connection.ReadJSON(&request); err != nil {
			return
		}
		if request.Method != "unsubscribe" {
			t.Errorf("spotState false unsubscribe = %#v", request)
			return
		}
		if value, present := request.Subscription["isPortfolioMargin"]; present && value != false {
			t.Errorf("spotState false unsubscribe changed semantics: %#v", request.Subscription)
			return
		}
		if err := connection.WriteJSON(subscriptionResponse("unsubscribe", normalized)); err != nil {
			return
		}
		unsubscribed <- struct{}{}
	}))
	defer server.Close()

	explicitFalse := false
	client := websocket.NewClient("ws"+strings.TrimPrefix(server.URL, "http"), websocket.Config{SubscriptionAckTimeout: 100 * time.Millisecond})
	defer func() { _ = client.Close() }()
	subscription, err := client.SubscribeSpotState(context.Background(), websocket.SpotStateRequest{User: "0xABCDEF", IsPortfolioMargin: &explicitFalse})
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
		t.Fatal("real spotState false unsubscribe was not acknowledged")
	}
}

func TestSpotStateOmittedAndFalseUseDeterministicDefaultWire(t *testing.T) {
	upgrader := gws.Upgrader{}
	requests := make(chan string, 2)
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
		if _, present := request.Subscription["isPortfolioMargin"]; present {
			t.Errorf("default spotState wire retained optional false: %#v", request.Subscription)
			return
		}
		if request.Subscription["user"] != "0xabcdef" {
			t.Errorf("default spotState wire user was nondeterministic: %#v", request.Subscription)
			return
		}
		requests <- request.Method
		normalized := map[string]any{"type": "spotState", "user": "0xabcdef", "ignorePortfolioMargin": false}
		if err := connection.WriteJSON(subscriptionResponse("subscribe", normalized)); err != nil {
			return
		}
		if err := connection.ReadJSON(&request); err != nil {
			return
		}
		if _, present := request.Subscription["isPortfolioMargin"]; present {
			t.Errorf("default spotState unsubscribe retained optional false: %#v", request.Subscription)
			return
		}
		if request.Subscription["user"] != "0xabcdef" {
			t.Errorf("default spotState unsubscribe user was nondeterministic: %#v", request.Subscription)
			return
		}
		requests <- request.Method
		_ = connection.WriteJSON(subscriptionResponse("unsubscribe", normalized))
	}))
	defer server.Close()

	dialer := &gatedWebSocketDialer{release: make(chan struct{})}
	client := websocket.NewClient("ws"+strings.TrimPrefix(server.URL, "http"), websocket.Config{Dialer: dialer, SubscriptionAckTimeout: 100 * time.Millisecond})
	defer func() { _ = client.Close() }()
	explicitFalse := false
	falseSubscription, err := client.SubscribeSpotState(context.Background(), websocket.SpotStateRequest{User: "0xABCDEF", IsPortfolioMargin: &explicitFalse})
	if err != nil {
		t.Fatal(err)
	}
	omitted, err := client.SubscribeSpotState(context.Background(), websocket.SpotStateRequest{User: "0xabcdef"})
	if err != nil {
		t.Fatal(err)
	}
	close(dialer.release)
	waitForSubscribed(t, falseSubscription.States())
	waitForSubscribed(t, omitted.States())
	if err := falseSubscription.Close(); err != nil {
		t.Fatal(err)
	}
	select {
	case method := <-requests:
		if method != "subscribe" {
			t.Fatalf("first request = %q", method)
		}
	case <-time.After(time.Second):
		t.Fatal("missing grouped subscribe")
	}
	select {
	case method := <-requests:
		t.Fatalf("closing one grouped handle sent %q", method)
	case <-time.After(30 * time.Millisecond):
	}
	if err := omitted.Close(); err != nil {
		t.Fatal(err)
	}
	select {
	case method := <-requests:
		if method != "unsubscribe" {
			t.Fatalf("last grouped request = %q", method)
		}
	case <-time.After(time.Second):
		t.Fatal("missing grouped unsubscribe")
	}
}

func TestParsedStaleBadErrorDoesNotRejectCurrentGoodPending(t *testing.T) {
	upgrader := gws.Upgrader{}
	wiresRead := make(chan struct{})
	keepOpen := make(chan struct{})
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		connection, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			t.Errorf("upgrade: %v", err)
			return
		}
		defer func() { _ = connection.Close() }()
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
		close(wiresRead)
		var unsubscribe struct {
			Method       string         `json:"method"`
			Subscription map[string]any `json:"subscription"`
		}
		if err := connection.ReadJSON(&unsubscribe); err != nil {
			return
		}
		if unsubscribe.Method != "unsubscribe" || unsubscribe.Subscription["coin"] != "BAD" {
			t.Errorf("BAD unsubscribe = %#v", unsubscribe)
			return
		}
		if err := connection.WriteJSON(subscriptionResponse("unsubscribe", unsubscribe.Subscription)); err != nil {
			return
		}
		echoed, _ := json.Marshal(map[string]any{"method": "subscribe", "subscription": byCoin["BAD"]})
		if err := connection.WriteJSON(map[string]any{"channel": "error", "data": "Invalid subscription: " + string(echoed)}); err != nil {
			return
		}
		if err := connection.WriteJSON(subscriptionResponse("subscribe", byCoin["GOOD"])); err != nil {
			return
		}
		<-keepOpen
	}))
	defer func() {
		close(keepOpen)
		server.Close()
	}()

	client := websocket.NewClient("ws"+strings.TrimPrefix(server.URL, "http"), websocket.Config{SubscriptionAckTimeout: 200 * time.Millisecond, PingInterval: time.Hour})
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
	select {
	case <-wiresRead:
	case <-time.After(time.Second):
		t.Fatal("BAD and GOOD did not reach wire")
	}
	if err := bad.Close(); err != nil {
		t.Fatal(err)
	}
	waitForSubscribed(t, good.States())
}

func TestAlreadyUnsubscribedSubscriptionOnlyStringClearsUniquePending(t *testing.T) {
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
		if connections.Add(1) != 1 {
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
		echoed, _ := json.Marshal(unsubscribe.Subscription)
		if err := connection.WriteJSON(map[string]any{"channel": "error", "data": "Already unsubscribed: " + string(echoed)}); err != nil {
			return
		}
		time.Sleep(120 * time.Millisecond)
		if err := connection.WriteJSON(map[string]any{
			"channel": "trades",
			"data": []map[string]any{{
				"coin": "GOOD", "side": "B", "px": "1", "sz": "2", "hash": "0x3", "time": 3, "tid": 4, "users": []string{"0xa", "0xb"},
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
		t.Fatal("GOOD stopped after Already unsubscribed string")
	}
	if got := connections.Load(); got != 1 {
		t.Fatalf("subscription-only unsubscribe error reconnected %d times", got)
	}
}

func TestRegistryAddCancelsBlockedUnsubscribeAndContinuesUnrelatedWork(t *testing.T) {
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
			wires <- request.Method + ":" + coin
			if err := connection.WriteJSON(subscriptionResponse(request.Method, request.Subscription)); err != nil {
				return
			}
		}
	}))
	defer server.Close()

	limiter := &cancelLastWireAdmissionLimiter{entered: make(chan struct{}), canceled: make(chan struct{})}
	client := websocket.NewClient("ws"+strings.TrimPrefix(server.URL, "http"), websocket.Config{MessageAdmission: limiter, PingInterval: time.Hour})
	defer func() { _ = client.Close() }()
	first, err := client.SubscribeTrades(context.Background(), websocket.TradesRequest{Coin: "SAME"})
	if err != nil {
		t.Fatal(err)
	}
	waitForSubscribed(t, first.States())
	if err := first.Close(); err != nil {
		t.Fatal(err)
	}
	select {
	case <-limiter.entered:
	case <-time.After(time.Second):
		t.Fatal("unsubscribe did not enter message admission")
	}
	readded, err := client.SubscribeTrades(context.Background(), websocket.TradesRequest{Coin: "SAME"})
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = readded.Close() }()
	select {
	case <-limiter.canceled:
	case <-time.After(time.Second):
		t.Fatal("registry add did not cancel stale unsubscribe admission")
	}
	third, err := client.SubscribeTrades(context.Background(), websocket.TradesRequest{Coin: "THIRD"})
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = third.Close() }()
	waitForSubscribed(t, readded.States())
	waitForSubscribed(t, third.States())
	want := []string{"subscribe:SAME", "subscribe:THIRD"}
	for index, expected := range want {
		select {
		case wire := <-wires:
			if wire != expected {
				t.Fatalf("wire %d = %q, want %q", index, wire, expected)
			}
		case <-time.After(time.Second):
			t.Fatalf("missing wire %d (%s)", index, expected)
		}
	}
	select {
	case wire := <-wires:
		t.Fatalf("stale or duplicate wire sent: %s", wire)
	case <-time.After(30 * time.Millisecond):
	}
	if got := connections.Load(); got != 1 {
		t.Fatalf("registry re-add required reconnect; connections=%d", got)
	}
}
