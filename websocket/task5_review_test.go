package websocket_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/Apexllcc/hypersdk-go/websocket"
	gws "github.com/gorilla/websocket"
)

func TestSpotStatePortfolioVariantsShareRealServerIdentity(t *testing.T) {
	upgrader := gws.Upgrader{}
	wires := make(chan map[string]any, 2)
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
		wires <- request.Subscription
		normalized := map[string]any{"type": "spotState", "user": "0xabcdef", "ignorePortfolioMargin": false}
		if err := connection.WriteJSON(subscriptionResponse("subscribe", normalized)); err != nil {
			return
		}
		if err := connection.ReadJSON(&request); err == nil {
			wires <- request.Subscription
			echo := map[string]any{"method": request.Method, "subscription": request.Subscription}
			_ = connection.WriteJSON(map[string]any{"channel": "error", "data": map[string]any{"error": "Already subscribed", "request": echo}})
		}
	}))
	defer server.Close()

	client := websocket.NewClient("ws"+strings.TrimPrefix(server.URL, "http"), websocket.Config{PingInterval: time.Hour})
	defer func() { _ = client.Close() }()
	explicitFalse, explicitTrue := false, true
	falseSubscription, err := client.SubscribeSpotState(context.Background(), websocket.SpotStateRequest{User: "0xABCDEF", IsPortfolioMargin: &explicitFalse})
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = falseSubscription.Close() }()
	waitForSubscribed(t, falseSubscription.States())
	trueSubscription, err := client.SubscribeSpotState(context.Background(), websocket.SpotStateRequest{User: "0xabcdef", IsPortfolioMargin: &explicitTrue})
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = trueSubscription.Close() }()
	waitForSubscribed(t, trueSubscription.States())

	select {
	case <-wires:
	case <-time.After(time.Second):
		t.Fatal("missing initial spotState wire")
	}
	select {
	case duplicate := <-wires:
		t.Fatalf("server-equivalent portfolio variant sent duplicate wire: %#v", duplicate)
	case event := <-falseSubscription.States():
		if event.State == websocket.SubscriptionStateError {
			t.Fatalf("false handle rejected after duplicate wire: %v", event.Error)
		}
	case event := <-trueSubscription.States():
		if event.State == websocket.SubscriptionStateError {
			t.Fatalf("true handle rejected after duplicate wire: %v", event.Error)
		}
	case <-time.After(50 * time.Millisecond):
	}
}

type allowFirstBlockSecondLimiter struct {
	calls    atomic.Int32
	blocked  chan struct{}
	release  chan struct{}
	canceled chan struct{}
	once     sync.Once
}

func newAllowFirstBlockSecondLimiter() *allowFirstBlockSecondLimiter {
	return &allowFirstBlockSecondLimiter{blocked: make(chan struct{}), release: make(chan struct{}), canceled: make(chan struct{})}
}

func (l *allowFirstBlockSecondLimiter) Wait(ctx context.Context) error {
	if l.calls.Add(1) == 1 {
		return nil
	}
	l.once.Do(func() { close(l.blocked) })
	select {
	case <-l.release:
		return nil
	case <-ctx.Done():
		select {
		case <-l.canceled:
		default:
			close(l.canceled)
		}
		return ctx.Err()
	}
}

func TestBlockedSecondSubscriptionAdmissionDoesNotStallFirstAcknowledgement(t *testing.T) {
	upgrader := gws.Upgrader{}
	firstWire := make(chan map[string]any, 1)
	sendFirstAck := make(chan struct{})
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		connection, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			t.Errorf("upgrade: %v", err)
			return
		}
		defer func() { _ = connection.Close() }()
		var request struct {
			Subscription map[string]any `json:"subscription"`
		}
		if err := connection.ReadJSON(&request); err != nil {
			return
		}
		firstWire <- request.Subscription
		<-sendFirstAck
		_ = connection.WriteJSON(subscriptionResponse("subscribe", request.Subscription))
		<-time.After(time.Second)
	}))
	defer server.Close()

	limiter := newAllowFirstBlockSecondLimiter()
	client := websocket.NewClient("ws"+strings.TrimPrefix(server.URL, "http"), websocket.Config{MessageAdmission: limiter, PingInterval: time.Hour})
	defer func() { _ = client.Close() }()
	first, err := client.SubscribeTrades(context.Background(), websocket.TradesRequest{Coin: "FIRST"})
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = first.Close() }()
	select {
	case <-firstWire:
	case <-time.After(time.Second):
		t.Fatal("first subscription did not reach wire")
	}
	second, err := client.SubscribeTrades(context.Background(), websocket.TradesRequest{Coin: "SECOND"})
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = second.Close() }()
	select {
	case <-limiter.blocked:
	case <-time.After(time.Second):
		t.Fatal("second subscription did not block in message admission")
	}
	close(sendFirstAck)
	waitForSubscribed(t, first.States())
}

func TestBlockedSecondSubscriptionAdmissionDoesNotStallEstablishedEvent(t *testing.T) {
	upgrader := gws.Upgrader{}
	sendEvent := make(chan struct{})
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		connection, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			t.Errorf("upgrade: %v", err)
			return
		}
		defer func() { _ = connection.Close() }()
		var request struct {
			Subscription map[string]any `json:"subscription"`
		}
		if err := connection.ReadJSON(&request); err != nil {
			return
		}
		if err := connection.WriteJSON(subscriptionResponse("subscribe", request.Subscription)); err != nil {
			return
		}
		<-sendEvent
		_ = connection.WriteJSON(map[string]any{
			"channel": "trades",
			"data":    []map[string]any{{"coin": "FIRST", "side": "B", "px": "1", "sz": "2", "hash": "0x1", "time": 1, "tid": 2, "users": []string{"0xa", "0xb"}}},
		})
		<-time.After(time.Second)
	}))
	defer server.Close()

	limiter := newAllowFirstBlockSecondLimiter()
	client := websocket.NewClient("ws"+strings.TrimPrefix(server.URL, "http"), websocket.Config{MessageAdmission: limiter, PingInterval: time.Hour})
	defer func() { _ = client.Close() }()
	first, err := client.SubscribeTrades(context.Background(), websocket.TradesRequest{Coin: "FIRST"})
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = first.Close() }()
	waitForSubscribed(t, first.States())
	second, err := client.SubscribeTrades(context.Background(), websocket.TradesRequest{Coin: "SECOND"})
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = second.Close() }()
	select {
	case <-limiter.blocked:
	case <-time.After(time.Second):
		t.Fatal("second subscription did not block in message admission")
	}
	close(sendEvent)
	select {
	case events := <-first.Events():
		if len(events) != 1 || events[0].Coin != "FIRST" {
			t.Fatalf("established events = %#v", events)
		}
	case <-time.After(time.Second):
		t.Fatal("blocked outbound admission stalled established inbound event")
	}
}
