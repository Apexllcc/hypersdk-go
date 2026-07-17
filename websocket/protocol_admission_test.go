package websocket_test

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/Apexllcc/hyperliquid-go-sdk/websocket"
	gws "github.com/gorilla/websocket"
)

func subscriptionResponse(method string, subscription map[string]any) map[string]any {
	return map[string]any{
		"channel": "subscriptionResponse",
		"data":    map[string]any{"method": method, "subscription": subscription},
	}
}

func nextState(t *testing.T, states <-chan websocket.SubscriptionStateEvent) websocket.SubscriptionStateEvent {
	t.Helper()
	select {
	case event := <-states:
		return event
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for subscription state")
		return websocket.SubscriptionStateEvent{}
	}
}

func TestL2BookFastIsEncodedAndPartOfIdentity(t *testing.T) {
	upgrader := gws.Upgrader{}
	requestSeen := make(chan map[string]any, 3)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		connection, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			t.Errorf("upgrade: %v", err)
			return
		}
		defer func() { _ = connection.Close() }()
		for range 3 {
			var request struct {
				Subscription map[string]any `json:"subscription"`
			}
			if err := connection.ReadJSON(&request); err != nil {
				t.Errorf("read subscription: %v", err)
				return
			}
			requestSeen <- request.Subscription
			if err := connection.WriteJSON(subscriptionResponse("subscribe", request.Subscription)); err != nil {
				t.Errorf("ack subscription: %v", err)
				return
			}
		}
	}))
	defer server.Close()

	client := websocket.NewClient("ws" + strings.TrimPrefix(server.URL, "http"))
	defer func() { _ = client.Close() }()
	fast, slow := true, false
	fastSubscription, err := client.SubscribeL2Book(context.Background(), websocket.L2BookRequest{Coin: "BTC", Fast: &fast})
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = fastSubscription.Close() }()
	alsoFast := true
	duplicateFast, err := client.SubscribeL2Book(context.Background(), websocket.L2BookRequest{Coin: "BTC", Fast: &alsoFast})
	if err != nil {
		t.Fatal(err)
	}
	if duplicateFast != fastSubscription {
		t.Fatal("equal fast values from distinct pointers did not reuse one logical subscription")
	}
	slowSubscription, err := client.SubscribeL2Book(context.Background(), websocket.L2BookRequest{Coin: "BTC", Fast: &slow})
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = slowSubscription.Close() }()
	if fastSubscription == slowSubscription {
		t.Fatal("fast=true and fast=false reused one logical subscription")
	}
	omittedSubscription, err := client.SubscribeL2Book(context.Background(), websocket.L2BookRequest{Coin: "BTC"})
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = omittedSubscription.Close() }()
	if omittedSubscription == fastSubscription || omittedSubscription == slowSubscription {
		t.Fatal("omitted fast value reused an explicit true or false subscription")
	}

	values := map[bool]bool{}
	omitted := false
	for range 3 {
		select {
		case request := <-requestSeen:
			raw, present := request["fast"]
			if !present {
				omitted = true
				continue
			}
			value, ok := raw.(bool)
			if !ok {
				t.Fatalf("fast field is non-boolean: %#v", request)
			}
			values[value] = true
		case <-time.After(time.Second):
			t.Fatal("timed out waiting for L2 book request")
		}
	}
	if !omitted || !values[true] || !values[false] {
		t.Fatalf("fast values = %#v, omitted=%t; want omitted, true, and false", values, omitted)
	}
	_ = nextState(t, fastSubscription.States())
	_ = nextState(t, fastSubscription.States())
	if event := nextState(t, fastSubscription.States()); event.State != websocket.SubscriptionStateSubscribed {
		t.Fatalf("state after acknowledgement = %q", event.State)
	}
}

func TestSubscribedStateWaitsForMatchingServerAcknowledgement(t *testing.T) {
	upgrader := gws.Upgrader{}
	releaseAck := make(chan struct{})
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
			t.Errorf("read subscription: %v", err)
			return
		}
		<-releaseAck
		_ = connection.WriteJSON(subscriptionResponse("subscribe", request.Subscription))
		<-time.After(time.Second)
	}))
	defer server.Close()

	client := websocket.NewClient("ws"+strings.TrimPrefix(server.URL, "http"), websocket.Config{SubscriptionAckTimeout: time.Second})
	defer func() { _ = client.Close() }()
	subscription, err := client.SubscribeTrades(context.Background(), websocket.TradesRequest{Coin: "BTC"})
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = subscription.Close() }()
	if state := nextState(t, subscription.States()).State; state != websocket.SubscriptionStateConnecting {
		t.Fatalf("first state = %q", state)
	}
	if state := nextState(t, subscription.States()).State; state != websocket.SubscriptionStateConnected {
		t.Fatalf("second state = %q", state)
	}
	select {
	case state := <-subscription.States():
		t.Fatalf("state before acknowledgement = %q", state.State)
	case <-time.After(30 * time.Millisecond):
	}
	close(releaseAck)
	if state := nextState(t, subscription.States()).State; state != websocket.SubscriptionStateSubscribed {
		t.Fatalf("state after acknowledgement = %q", state)
	}
}

func TestSubscriptionNoAcknowledgementTimesOut(t *testing.T) {
	upgrader := gws.Upgrader{}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		connection, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			t.Errorf("upgrade: %v", err)
			return
		}
		defer func() { _ = connection.Close() }()
		_, _, _ = connection.ReadMessage()
		<-time.After(time.Second)
	}))
	defer server.Close()

	client := websocket.NewClient("ws"+strings.TrimPrefix(server.URL, "http"), websocket.Config{
		SubscriptionAckTimeout: 25 * time.Millisecond,
		ReconnectDelay:         time.Hour,
	})
	defer func() { _ = client.Close() }()
	subscription, err := client.SubscribeTrades(context.Background(), websocket.TradesRequest{Coin: "BTC"})
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = subscription.Close() }()
	deadline := time.After(time.Second)
	for {
		select {
		case event := <-subscription.States():
			if event.State == websocket.SubscriptionStateError {
				if !errors.Is(event.Error, websocket.ErrSubscriptionAckTimeout) {
					t.Fatalf("timeout error = %v", event.Error)
				}
				return
			}
		case <-deadline:
			t.Fatal("timed out waiting for acknowledgement error")
		}
	}
}

func TestSubscriptionRejectionIsReported(t *testing.T) {
	upgrader := gws.Upgrader{}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		connection, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			t.Errorf("upgrade: %v", err)
			return
		}
		defer func() { _ = connection.Close() }()
		if _, _, err := connection.ReadMessage(); err != nil {
			return
		}
		_ = connection.WriteJSON(map[string]any{"channel": "error", "data": "subscription denied"})
	}))
	defer server.Close()

	client := websocket.NewClient("ws"+strings.TrimPrefix(server.URL, "http"), websocket.Config{ReconnectDelay: time.Hour})
	defer func() { _ = client.Close() }()
	subscription, err := client.SubscribeTrades(context.Background(), websocket.TradesRequest{Coin: "BTC"})
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = subscription.Close() }()
	deadline := time.After(time.Second)
	for {
		select {
		case event := <-subscription.States():
			if event.State == websocket.SubscriptionStateError {
				if !errors.Is(event.Error, websocket.ErrSubscriptionRejected) {
					t.Fatalf("rejection error = %v", event.Error)
				}
				return
			}
		case <-deadline:
			t.Fatal("timed out waiting for rejection")
		}
	}
}

func TestReconnectRequiresFreshAcknowledgement(t *testing.T) {
	upgrader := gws.Upgrader{}
	var connections atomic.Int32
	secondRequest := make(chan struct{})
	releaseSecondAck := make(chan struct{})
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
		if connections.Add(1) == 1 {
			_ = connection.WriteJSON(subscriptionResponse("subscribe", request.Subscription))
			return
		}
		close(secondRequest)
		<-releaseSecondAck
		_ = connection.WriteJSON(subscriptionResponse("subscribe", request.Subscription))
		<-time.After(time.Second)
	}))
	defer server.Close()

	client := websocket.NewClient("ws"+strings.TrimPrefix(server.URL, "http"), websocket.Config{
		ReconnectDelay:         time.Millisecond,
		SubscriptionAckTimeout: time.Second,
	})
	defer func() { _ = client.Close() }()
	subscription, err := client.SubscribeTrades(context.Background(), websocket.TradesRequest{Coin: "BTC"})
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = subscription.Close() }()
	for {
		if state := nextState(t, subscription.States()).State; state == websocket.SubscriptionStateSubscribed {
			break
		}
	}
	select {
	case <-secondRequest:
	case <-time.After(time.Second):
		t.Fatal("reconnect did not resend subscription")
	}
	for {
		state := nextState(t, subscription.States()).State
		if state == websocket.SubscriptionStateConnected {
			break
		}
	}
	select {
	case state := <-subscription.States():
		if state.State == websocket.SubscriptionStateSubscribed {
			t.Fatal("reconnect published subscribed before fresh acknowledgement")
		}
	case <-time.After(30 * time.Millisecond):
	}
	close(releaseSecondAck)
	if state := nextState(t, subscription.States()).State; state != websocket.SubscriptionStateSubscribed {
		t.Fatalf("state after reconnect acknowledgement = %q", state)
	}
}

func TestUnsubscribeRequestIsSentAndCorrelated(t *testing.T) {
	upgrader := gws.Upgrader{}
	unsubscribed := make(chan map[string]any, 1)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		connection, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			t.Errorf("upgrade: %v", err)
			return
		}
		defer func() { _ = connection.Close() }()
		for _, method := range []string{"subscribe", "unsubscribe"} {
			var request struct {
				Method       string         `json:"method"`
				Subscription map[string]any `json:"subscription"`
			}
			if err := connection.ReadJSON(&request); err != nil {
				t.Errorf("read %s: %v", method, err)
				return
			}
			if request.Method != method {
				t.Errorf("method = %q, want %q", request.Method, method)
				return
			}
			if err := connection.WriteJSON(subscriptionResponse(method, request.Subscription)); err != nil {
				t.Errorf("ack %s: %v", method, err)
				return
			}
			if method == "unsubscribe" {
				unsubscribed <- request.Subscription
			}
		}
	}))
	defer server.Close()

	client := websocket.NewClient("ws" + strings.TrimPrefix(server.URL, "http"))
	defer func() { _ = client.Close() }()
	subscription, err := client.SubscribeTrades(context.Background(), websocket.TradesRequest{Coin: "BTC"})
	if err != nil {
		t.Fatal(err)
	}
	for {
		if state := nextState(t, subscription.States()).State; state == websocket.SubscriptionStateSubscribed {
			break
		}
	}
	if err := subscription.Close(); err != nil {
		t.Fatal(err)
	}
	select {
	case request := <-unsubscribed:
		if request["type"] != "trades" || request["coin"] != "BTC" {
			t.Fatalf("unsubscribe = %#v", request)
		}
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for unsubscribe")
	}
}

func TestActiveSubscriptionAndUniqueUserLimits(t *testing.T) {
	t.Run("active subscriptions", func(t *testing.T) {
		client := websocket.NewClient("ws://example.invalid", websocket.Config{MaxActiveSubscriptions: 2, ReconnectDelay: time.Hour})
		defer func() { _ = client.Close() }()
		for i := range 2 {
			subscription, err := client.SubscribeTrades(context.Background(), websocket.TradesRequest{Coin: fmt.Sprintf("COIN-%d", i)})
			if err != nil {
				t.Fatal(err)
			}
			defer func() { _ = subscription.Close() }()
		}
		if _, err := client.SubscribeTrades(context.Background(), websocket.TradesRequest{Coin: "OVER"}); !errors.Is(err, websocket.ErrActiveSubscriptionLimit) {
			t.Fatalf("third subscription error = %v", err)
		}
	})

	t.Run("unique users", func(t *testing.T) {
		client := websocket.NewClient("ws://example.invalid", websocket.Config{MaxUniqueUsers: 2, ReconnectDelay: time.Hour})
		defer func() { _ = client.Close() }()
		for _, user := range []string{"0xA", "0xB"} {
			subscription, err := client.SubscribeUserFundings(context.Background(), user)
			if err != nil {
				t.Fatal(err)
			}
			defer func() { _ = subscription.Close() }()
		}
		if _, err := client.SubscribeUserFundings(context.Background(), "0xC"); !errors.Is(err, websocket.ErrUniqueUserLimit) {
			t.Fatalf("third unique user error = %v", err)
		}
	})
}

func TestActiveSubscriptionLimitIsConcurrentSafe(t *testing.T) {
	const limit = 5
	client := websocket.NewClient("ws://example.invalid", websocket.Config{MaxActiveSubscriptions: limit, ReconnectDelay: time.Hour})
	defer func() { _ = client.Close() }()
	var admitted atomic.Int32
	var group sync.WaitGroup
	for i := range 50 {
		group.Add(1)
		go func(i int) {
			defer group.Done()
			_, err := client.SubscribeTrades(context.Background(), websocket.TradesRequest{Coin: fmt.Sprintf("COIN-%d", i)})
			if err == nil {
				admitted.Add(1)
				return
			}
			if !errors.Is(err, websocket.ErrActiveSubscriptionLimit) {
				t.Errorf("subscription %d: %v", i, err)
			}
		}(i)
	}
	group.Wait()
	if got := admitted.Load(); got != limit {
		t.Fatalf("admitted = %d, want %d", got, limit)
	}
}

func TestOutgoingMessageRateWaitHonorsCancellation(t *testing.T) {
	url := postTestServer(t, func(connection *gws.Conn) {
		var request struct {
			ID uint64 `json:"id"`
		}
		if err := connection.ReadJSON(&request); err != nil {
			return
		}
		_ = connection.WriteJSON(map[string]any{"channel": "post", "data": map[string]any{"id": request.ID, "response": map[string]any{"type": "info", "payload": map[string]any{"type": "test", "data": map[string]any{"ok": true}}}}})
		_ = connection.SetReadDeadline(time.Now().Add(100 * time.Millisecond))
		if _, _, err := connection.ReadMessage(); err == nil {
			t.Error("rate-limited request reached the wire")
		}
	})
	client := websocket.NewClient(url, websocket.Config{MaxOutgoingMessagesPerMinute: 1})
	defer func() { _ = client.Close() }()
	if err := client.PostInfo(context.Background(), map[string]string{"type": "first"}, &map[string]any{}); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Millisecond)
	defer cancel()
	err := client.PostInfo(ctx, map[string]string{"type": "second"}, &map[string]any{})
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("rate-limited request error = %v, want deadline exceeded", err)
	}
}

func TestHundredAndFirstPostWaitsAndHonorsCancellation(t *testing.T) {
	upgrader := gws.Upgrader{}
	requests := make(chan struct{}, 101)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		connection, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			t.Errorf("upgrade: %v", err)
			return
		}
		defer func() { _ = connection.Close() }()
		for {
			if _, _, err := connection.ReadMessage(); err != nil {
				return
			}
			requests <- struct{}{}
		}
	}))
	defer server.Close()

	client := websocket.NewClient("ws"+strings.TrimPrefix(server.URL, "http"), websocket.Config{MaxConcurrentPosts: 100})
	defer func() { _ = client.Close() }()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	for range 100 {
		go func() { _ = client.PostInfo(ctx, map[string]string{"type": "held"}, &map[string]any{}) }()
	}
	for range 100 {
		select {
		case <-requests:
		case <-time.After(2 * time.Second):
			t.Fatal("timed out waiting for first 100 POST requests")
		}
	}
	blockedCtx, blockedCancel := context.WithCancel(context.Background())
	result := make(chan error, 1)
	go func() { result <- client.PostInfo(blockedCtx, map[string]string{"type": "blocked"}, &map[string]any{}) }()
	select {
	case <-requests:
		t.Fatal("101st POST reached the wire")
	case <-time.After(30 * time.Millisecond):
	}
	blockedCancel()
	select {
	case err := <-result:
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("101st POST error = %v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("101st POST did not honor cancellation")
	}
}
