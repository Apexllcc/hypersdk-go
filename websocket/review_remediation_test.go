package websocket_test

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/Apexllcc/hyperliquid-go-sdk/transport"
	"github.com/Apexllcc/hyperliquid-go-sdk/websocket"
	gws "github.com/gorilla/websocket"
)

func waitForSubscribed(t *testing.T, states <-chan websocket.SubscriptionStateEvent) {
	t.Helper()
	deadline := time.After(time.Second)
	for {
		select {
		case event, ok := <-states:
			if !ok {
				t.Fatal("state stream closed before subscribed")
			}
			if event.State == websocket.SubscriptionStateError {
				t.Fatalf("subscription failed before acknowledgement: %v", event.Error)
			}
			if event.State == websocket.SubscriptionStateSubscribed {
				return
			}
		case <-deadline:
			t.Fatal("timed out waiting for subscribed")
		}
	}
}

func waitForStateError(t *testing.T, states <-chan websocket.SubscriptionStateEvent, target error) {
	t.Helper()
	deadline := time.After(time.Second)
	for {
		select {
		case event, ok := <-states:
			if !ok {
				t.Fatal("state stream closed before error")
			}
			if event.State == websocket.SubscriptionStateError {
				if !errors.Is(event.Error, target) {
					t.Fatalf("state error = %v, want %v", event.Error, target)
				}
				return
			}
		case <-deadline:
			t.Fatal("timed out waiting for error state")
		}
	}
}

type secondCallBlockingLimiter struct {
	calls    atomic.Int32
	entered  chan struct{}
	release  chan struct{}
	canceled chan struct{}
	once     sync.Once
}

func newSecondCallBlockingLimiter() *secondCallBlockingLimiter {
	return &secondCallBlockingLimiter{entered: make(chan struct{}), release: make(chan struct{}), canceled: make(chan struct{})}
}

func (l *secondCallBlockingLimiter) Wait(ctx context.Context) error {
	if l.calls.Add(1) == 1 {
		return nil
	}
	l.once.Do(func() { close(l.entered) })
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

type reconnectLifecycleLimiter struct {
	calls    atomic.Int32
	blocked  chan struct{}
	canceled chan struct{}
	block    sync.Once
	cancel   sync.Once
}

func newReconnectLifecycleLimiter() *reconnectLifecycleLimiter {
	return &reconnectLifecycleLimiter{blocked: make(chan struct{}), canceled: make(chan struct{})}
}

func (l *reconnectLifecycleLimiter) Wait(ctx context.Context) error {
	switch l.calls.Add(1) {
	case 1:
		return nil
	case 2:
		l.block.Do(func() { close(l.blocked) })
		<-ctx.Done()
		l.cancel.Do(func() { close(l.canceled) })
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

type blockingPostGate struct {
	entered  chan struct{}
	canceled chan struct{}
	once     sync.Once
}

func (g *blockingPostGate) Acquire(ctx context.Context) (func(), error) {
	g.once.Do(func() { close(g.entered) })
	<-ctx.Done()
	close(g.canceled)
	return nil, ctx.Err()
}

type blockingMessageLimiter struct {
	entered  chan struct{}
	canceled chan struct{}
	once     sync.Once
}

func (l *blockingMessageLimiter) Wait(ctx context.Context) error {
	l.once.Do(func() { close(l.entered) })
	<-ctx.Done()
	close(l.canceled)
	return ctx.Err()
}

func TestNormalizedL2AcknowledgementSharesUnderlyingWireSubscription(t *testing.T) {
	upgrader := gws.Upgrader{}
	afterSubscribe := make(chan string, 1)
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
			t.Errorf("subscribe: %v", err)
			return
		}
		if request.Method != "subscribe" || request.Subscription["type"] != "l2Book" || request.Subscription["coin"] != "BTC" {
			t.Errorf("subscribe request = %#v", request)
			return
		}
		normalized := map[string]any{"type": "l2Book", "coin": "BTC", "nSigFigs": nil, "mantissa": nil, "fast": false}
		if err := connection.WriteJSON(subscriptionResponse("subscribe", normalized)); err != nil {
			t.Errorf("subscribe ack: %v", err)
			return
		}
		if err := connection.ReadJSON(&request); err != nil {
			t.Errorf("request after subscribe: %v", err)
			return
		}
		afterSubscribe <- request.Method
		if request.Method != "unsubscribe" {
			return
		}
		_ = connection.WriteJSON(subscriptionResponse("unsubscribe", normalized))
	}))
	defer server.Close()

	client := websocket.NewClient("ws"+strings.TrimPrefix(server.URL, "http"), websocket.Config{SubscriptionAckTimeout: 200 * time.Millisecond})
	defer func() { _ = client.Close() }()
	omitted, err := client.SubscribeL2Book(context.Background(), websocket.L2BookRequest{Coin: "BTC"})
	if err != nil {
		t.Fatal(err)
	}
	explicitFalse := false
	falseSubscription, err := client.SubscribeL2Book(context.Background(), websocket.L2BookRequest{Coin: "BTC", Fast: &explicitFalse})
	if err != nil {
		t.Fatal(err)
	}
	if omitted == falseSubscription {
		t.Fatal("logical omitted and explicit false identities were collapsed")
	}
	waitForSubscribed(t, omitted.States())
	waitForSubscribed(t, falseSubscription.States())
	if err := omitted.Close(); err != nil {
		t.Fatal(err)
	}
	select {
	case method := <-afterSubscribe:
		t.Fatalf("closing one logical reference sent %q", method)
	case <-time.After(30 * time.Millisecond):
	}
	if err := falseSubscription.Close(); err != nil {
		t.Fatal(err)
	}
	select {
	case method := <-afterSubscribe:
		if method != "unsubscribe" {
			t.Fatalf("last logical reference sent %q", method)
		}
	case <-time.After(time.Second):
		t.Fatal("last logical reference did not unsubscribe")
	}
}

func TestNormalizedUserFillsAcknowledgementMatchesOmittedFalseAndAddressCase(t *testing.T) {
	upgrader := gws.Upgrader{}
	unsubscribed := make(chan struct{})
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
			t.Errorf("subscribe: %v", err)
			return
		}
		normalized := map[string]any{
			"type": "userFills", "user": "0xabcdef", "aggregateByTime": false,
		}
		if err := connection.WriteJSON(subscriptionResponse("subscribe", normalized)); err != nil {
			return
		}
		if err := connection.ReadJSON(&request); err != nil {
			return
		}
		if request.Method != "unsubscribe" {
			t.Errorf("request after acknowledgement = %q, want unsubscribe", request.Method)
			return
		}
		_ = connection.WriteJSON(subscriptionResponse("unsubscribe", normalized))
		close(unsubscribed)
	}))
	defer server.Close()

	client := websocket.NewClient("ws"+strings.TrimPrefix(server.URL, "http"), websocket.Config{SubscriptionAckTimeout: 100 * time.Millisecond})
	defer func() { _ = client.Close() }()
	subscription, err := client.SubscribeUserFills(context.Background(), websocket.UserFillsRequest{User: "0xABCDEF"})
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
		t.Fatal("normalized userFills unsubscribe was not matched")
	}
}

func TestNormalizedSpotStateAcknowledgementSharesOmittedAndFalseWire(t *testing.T) {
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
		requests <- request.Method
		normalized := map[string]any{
			"type": "spotState", "user": "0xabcdef", "isPortfolioMargin": false,
		}
		_ = connection.WriteJSON(subscriptionResponse("subscribe", normalized))
		if err := connection.ReadJSON(&request); err == nil {
			requests <- request.Method
			_ = connection.WriteJSON(subscriptionResponse("unsubscribe", normalized))
		}
	}))
	defer server.Close()

	client := websocket.NewClient("ws"+strings.TrimPrefix(server.URL, "http"), websocket.Config{SubscriptionAckTimeout: 100 * time.Millisecond})
	defer func() { _ = client.Close() }()
	omitted, err := client.SubscribeSpotState(context.Background(), websocket.SpotStateRequest{User: "0xABCDEF"})
	if err != nil {
		t.Fatal(err)
	}
	explicitFalse := false
	falseSubscription, err := client.SubscribeSpotState(context.Background(), websocket.SpotStateRequest{User: "0xabcdef", IsPortfolioMargin: &explicitFalse})
	if err != nil {
		t.Fatal(err)
	}
	if omitted == falseSubscription {
		t.Fatal("logical omitted and explicit false spotState identities were collapsed")
	}
	waitForSubscribed(t, omitted.States())
	waitForSubscribed(t, falseSubscription.States())
	select {
	case method := <-requests:
		if method != "subscribe" {
			t.Fatalf("first method = %q", method)
		}
	case <-time.After(time.Second):
		t.Fatal("missing subscribe")
	}
	select {
	case method := <-requests:
		t.Fatalf("duplicate underlying spotState request = %q", method)
	case <-time.After(30 * time.Millisecond):
	}
	if err := omitted.Close(); err != nil {
		t.Fatal(err)
	}
	select {
	case method := <-requests:
		t.Fatalf("closing one spotState reference sent %q", method)
	case <-time.After(30 * time.Millisecond):
	}
	if err := falseSubscription.Close(); err != nil {
		t.Fatal(err)
	}
	select {
	case method := <-requests:
		if method != "unsubscribe" {
			t.Fatalf("last spotState reference sent %q", method)
		}
	case <-time.After(time.Second):
		t.Fatal("normalized spotState unsubscribe was not matched")
	}
}

func TestCorrelatedRejectionTerminatesOnlyBadSubscriptionAndDoesNotReplay(t *testing.T) {
	upgrader := gws.Upgrader{}
	var connections atomic.Int32
	var countsMu sync.Mutex
	counts := map[string]int{}
	forceReconnect := make(chan struct{})
	restoredGood := make(chan struct{})
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		connection, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			t.Errorf("upgrade: %v", err)
			return
		}
		defer func() { _ = connection.Close() }()
		generation := connections.Add(1)
		readSubscription := func() map[string]any {
			var request struct {
				Subscription map[string]any `json:"subscription"`
			}
			if err := connection.ReadJSON(&request); err != nil {
				t.Errorf("generation %d read: %v", generation, err)
				return nil
			}
			coin, _ := request.Subscription["coin"].(string)
			countsMu.Lock()
			counts[coin]++
			countsMu.Unlock()
			return request.Subscription
		}
		if generation == 1 {
			first, second := readSubscription(), readSubscription()
			if first == nil || second == nil {
				return
			}
			byCoin := map[string]map[string]any{first["coin"].(string): first, second["coin"].(string): second}
			if err := connection.WriteJSON(subscriptionResponse("subscribe", byCoin["GOOD"])); err != nil {
				return
			}
			if err := connection.WriteJSON(map[string]any{
				"channel": "error",
				"data": map[string]any{
					"error":   "invalid subscription",
					"request": map[string]any{"method": "subscribe", "subscription": byCoin["BAD"]},
				},
			}); err != nil {
				return
			}
			<-forceReconnect
			return
		}
		restored := readSubscription()
		if restored == nil {
			return
		}
		if restored["coin"] != "GOOD" {
			t.Errorf("replayed rejected subscription: %#v", restored)
			return
		}
		_ = connection.WriteJSON(subscriptionResponse("subscribe", restored))
		close(restoredGood)
		<-time.After(time.Second)
	}))
	defer server.Close()

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
	waitForSubscribed(t, good.States())
	waitForStateError(t, bad.States(), websocket.ErrSubscriptionRejected)
	<-time.After(30 * time.Millisecond)
	if got := connections.Load(); got != 1 {
		t.Fatalf("correlated rejection disrupted acknowledged subscription; connections=%d", got)
	}
	close(forceReconnect)
	select {
	case <-restoredGood:
	case <-time.After(time.Second):
		t.Fatal("good subscription was not restored")
	}
	countsMu.Lock()
	badCount, goodCount := counts["BAD"], counts["GOOD"]
	countsMu.Unlock()
	if badCount != 1 || goodCount != 2 {
		t.Fatalf("wire counts BAD=%d GOOD=%d, want 1 and 2", badCount, goodCount)
	}
}

func TestUncorrelatedRejectionTerminatesPendingWithoutReconnectLoop(t *testing.T) {
	upgrader := gws.Upgrader{}
	var connections atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		connection, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			t.Errorf("upgrade: %v", err)
			return
		}
		defer func() { _ = connection.Close() }()
		connections.Add(1)
		if _, _, err := connection.ReadMessage(); err != nil {
			return
		}
		_ = connection.WriteJSON(map[string]any{"channel": "error", "data": "invalid subscription"})
		<-time.After(100 * time.Millisecond)
	}))
	defer server.Close()

	client := websocket.NewClient("ws"+strings.TrimPrefix(server.URL, "http"), websocket.Config{ReconnectDelay: time.Millisecond})
	defer func() { _ = client.Close() }()
	bad, err := client.SubscribeTrades(context.Background(), websocket.TradesRequest{Coin: "BAD"})
	if err != nil {
		t.Fatal(err)
	}
	waitForStateError(t, bad.States(), websocket.ErrSubscriptionRejected)
	time.Sleep(50 * time.Millisecond)
	if got := connections.Load(); got != 1 {
		t.Fatalf("uncorrelated permanent rejection reconnected %d times", got)
	}
}

func TestSubscriptionAndPostShareOneMessageAdmissionBudget(t *testing.T) {
	upgrader := gws.Upgrader{}
	wireRequests := make(chan string, 2)
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
		wireRequests <- request.Method
		if request.Method == "subscribe" {
			_ = connection.WriteJSON(subscriptionResponse("subscribe", request.Subscription))
		}
		<-time.After(100 * time.Millisecond)
	}))
	defer server.Close()

	limiter := websocket.NewMessageAdmissionLimiter(1, time.Hour)
	client := websocket.NewClient("ws"+strings.TrimPrefix(server.URL, "http"), websocket.Config{MessageAdmission: limiter})
	defer func() { _ = client.Close() }()
	subscription, err := client.SubscribeTrades(context.Background(), websocket.TradesRequest{Coin: "BTC"})
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = subscription.Close() }()
	waitForSubscribed(t, subscription.States())
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Millisecond)
	defer cancel()
	if err := client.PostInfo(ctx, map[string]string{"type": "allMids"}, &map[string]any{}); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("post after subscription admission = %v", err)
	}
	select {
	case method := <-wireRequests:
		if method != "subscribe" {
			t.Fatalf("first method = %q", method)
		}
	case <-time.After(time.Second):
		t.Fatal("subscription was not sent")
	}
	select {
	case method := <-wireRequests:
		t.Fatalf("separate budget allowed extra %q", method)
	case <-time.After(30 * time.Millisecond):
	}
}

func TestInjectedMessageAdmissionIsSharedAcrossClients(t *testing.T) {
	url := postTestServer(t, func(connection *gws.Conn) {
		var request struct {
			ID uint64 `json:"id"`
		}
		if err := connection.ReadJSON(&request); err != nil {
			return
		}
		_ = connection.WriteJSON(map[string]any{"channel": "post", "data": map[string]any{"id": request.ID, "response": map[string]any{"type": "info", "payload": map[string]any{"type": "test", "data": map[string]any{"ok": true}}}}})
		_ = connection.SetReadDeadline(time.Now().Add(100 * time.Millisecond))
		_, _, _ = connection.ReadMessage()
	})
	limiter := websocket.NewMessageAdmissionLimiter(1, time.Hour)
	first := websocket.NewClient(url, websocket.Config{MessageAdmission: limiter})
	defer func() { _ = first.Close() }()
	second := websocket.NewClient(url, websocket.Config{MessageAdmission: limiter})
	defer func() { _ = second.Close() }()
	if err := first.Request(context.Background(), transport.RequestInfo, map[string]string{"type": "first"}, &map[string]any{}); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Millisecond)
	defer cancel()
	if err := second.PostInfo(ctx, map[string]string{"type": "second"}, &map[string]any{}); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("second client shared-limit error = %v", err)
	}
}

func TestInjectedPostAdmissionGateIsSharedAcrossClients(t *testing.T) {
	upgrader := gws.Upgrader{}
	requests := make(chan struct{}, 2)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		connection, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			t.Errorf("upgrade: %v", err)
			return
		}
		defer func() { _ = connection.Close() }()
		if _, _, err := connection.ReadMessage(); err == nil {
			requests <- struct{}{}
		}
		<-time.After(time.Second)
	}))
	defer server.Close()

	gate := websocket.NewPostAdmissionGate(1)
	url := "ws" + strings.TrimPrefix(server.URL, "http")
	first := websocket.NewClient(url, websocket.Config{PostAdmission: gate})
	defer func() { _ = first.Close() }()
	second := websocket.NewClient(url, websocket.Config{PostAdmission: gate})
	defer func() { _ = second.Close() }()
	firstCtx, firstCancel := context.WithCancel(context.Background())
	defer firstCancel()
	go func() { _ = first.PostInfo(firstCtx, map[string]string{"type": "held"}, &map[string]any{}) }()
	select {
	case <-requests:
	case <-time.After(time.Second):
		t.Fatal("first client did not acquire shared POST gate")
	}
	secondCtx, secondCancel := context.WithTimeout(context.Background(), 30*time.Millisecond)
	defer secondCancel()
	if err := second.PostInfo(secondCtx, map[string]string{"type": "blocked"}, &map[string]any{}); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("second client POST gate error = %v", err)
	}
	select {
	case <-requests:
		t.Fatal("second client bypassed shared POST gate")
	case <-time.After(30 * time.Millisecond):
	}
}

func TestCanceledSubscriptionWaitingForMessageAdmissionDoesNotReachWire(t *testing.T) {
	upgrader := gws.Upgrader{}
	wires := make(chan string, 2)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		connection, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			t.Errorf("upgrade: %v", err)
			return
		}
		defer func() { _ = connection.Close() }()
		for {
			var request struct {
				Subscription map[string]any `json:"subscription"`
			}
			if err := connection.ReadJSON(&request); err != nil {
				return
			}
			coin, _ := request.Subscription["coin"].(string)
			wires <- coin
			_ = connection.WriteJSON(subscriptionResponse("subscribe", request.Subscription))
		}
	}))
	defer server.Close()

	limiter := newSecondCallBlockingLimiter()
	client := websocket.NewClient("ws"+strings.TrimPrefix(server.URL, "http"), websocket.Config{MessageAdmission: limiter, PingInterval: time.Hour})
	defer func() { _ = client.Close() }()
	first, err := client.SubscribeTrades(context.Background(), websocket.TradesRequest{Coin: "FIRST"})
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = first.Close() }()
	waitForSubscribed(t, first.States())
	second, err := client.SubscribeTrades(context.Background(), websocket.TradesRequest{Coin: "CANCELED"})
	if err != nil {
		t.Fatal(err)
	}
	select {
	case <-limiter.entered:
	case <-time.After(time.Second):
		t.Fatal("second subscription did not reach message admission")
	}
	if err := second.Close(); err != nil {
		t.Fatal(err)
	}
	close(limiter.release)
	select {
	case coin := <-wires:
		if coin != "FIRST" {
			t.Fatalf("first wire = %q", coin)
		}
	case <-time.After(time.Second):
		t.Fatal("missing first wire")
	}
	select {
	case coin := <-wires:
		t.Fatalf("canceled subscription reached wire as %q", coin)
	case <-time.After(50 * time.Millisecond):
	}
}

func TestHeartbeatStopCancelsAndJoinsMessageAdmissionWait(t *testing.T) {
	upgrader := gws.Upgrader{}
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
		_ = connection.WriteJSON(subscriptionResponse("subscribe", request.Subscription))
		<-time.After(time.Second)
	}))
	defer server.Close()

	limiter := newSecondCallBlockingLimiter()
	defer func() {
		select {
		case <-limiter.release:
		default:
			close(limiter.release)
		}
	}()
	client := websocket.NewClient("ws"+strings.TrimPrefix(server.URL, "http"), websocket.Config{MessageAdmission: limiter, PingInterval: time.Millisecond})
	subscription, err := client.SubscribeTrades(context.Background(), websocket.TradesRequest{Coin: "BTC"})
	if err != nil {
		t.Fatal(err)
	}
	waitForSubscribed(t, subscription.States())
	select {
	case <-limiter.entered:
	case <-time.After(time.Second):
		t.Fatal("heartbeat did not wait for message admission")
	}
	closed := make(chan struct{})
	go func() {
		_ = client.Close()
		close(closed)
	}()
	select {
	case <-closed:
	case <-time.After(time.Second):
		t.Fatal("Client.Close blocked behind heartbeat admission")
	}
	select {
	case <-limiter.canceled:
	case <-time.After(50 * time.Millisecond):
		t.Fatal("heartbeat admission was not canceled and joined before Close returned")
	}
}

func TestReconnectCancelsOldHeartbeatAdmissionBeforeResubscribe(t *testing.T) {
	upgrader := gws.Upgrader{}
	limiter := newReconnectLifecycleLimiter()
	var connections atomic.Int32
	restored := make(chan struct{})
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
		_ = connection.WriteJSON(subscriptionResponse("subscribe", request.Subscription))
		if connections.Add(1) == 1 {
			<-limiter.blocked
			return
		}
		close(restored)
		<-time.After(time.Second)
	}))
	defer server.Close()

	client := websocket.NewClient("ws"+strings.TrimPrefix(server.URL, "http"), websocket.Config{
		MessageAdmission: limiter,
		PingInterval:     time.Millisecond,
		ReconnectDelay:   time.Millisecond,
	})
	defer func() { _ = client.Close() }()
	subscription, err := client.SubscribeTrades(context.Background(), websocket.TradesRequest{Coin: "BTC"})
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = subscription.Close() }()
	waitForSubscribed(t, subscription.States())
	select {
	case <-restored:
	case <-time.After(time.Second):
		t.Fatal("reconnect could not pass admission after old heartbeat stopped")
	}
}

func TestClientCloseCancelsBlockedPostGate(t *testing.T) {
	gate := &blockingPostGate{entered: make(chan struct{}), canceled: make(chan struct{})}
	client := websocket.NewClient("ws://unused", websocket.Config{PostAdmission: gate})
	result := make(chan error, 1)
	go func() {
		result <- client.PostInfo(context.Background(), map[string]string{"type": "blocked"}, &map[string]any{})
	}()
	select {
	case <-gate.entered:
	case <-time.After(time.Second):
		t.Fatal("POST did not wait on gate")
	}
	if err := client.Close(); err != nil {
		t.Fatal(err)
	}
	select {
	case <-gate.canceled:
	case <-time.After(time.Second):
		t.Fatal("Client.Close did not cancel POST gate")
	}
	select {
	case err := <-result:
		if !errors.Is(err, websocket.ErrWebSocketClosed) {
			t.Fatalf("blocked POST error = %v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("blocked POST did not return")
	}
}

func TestClientCloseCancelsBlockedPostMessageAdmission(t *testing.T) {
	upgrader := gws.Upgrader{}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		connection, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			t.Errorf("upgrade: %v", err)
			return
		}
		defer func() { _ = connection.Close() }()
		<-time.After(time.Second)
	}))
	defer server.Close()
	limiter := &blockingMessageLimiter{entered: make(chan struct{}), canceled: make(chan struct{})}
	client := websocket.NewClient("ws"+strings.TrimPrefix(server.URL, "http"), websocket.Config{MessageAdmission: limiter})
	result := make(chan error, 1)
	go func() {
		result <- client.PostInfo(context.Background(), map[string]string{"type": "blocked"}, &map[string]any{})
	}()
	select {
	case <-limiter.entered:
	case <-time.After(time.Second):
		t.Fatal("POST did not wait on message admission")
	}
	if err := client.Close(); err != nil {
		t.Fatal(err)
	}
	select {
	case <-limiter.canceled:
	case <-time.After(time.Second):
		t.Fatal("Client.Close did not cancel POST message admission")
	}
	select {
	case err := <-result:
		if !errors.Is(err, websocket.ErrWebSocketClosed) {
			t.Fatalf("blocked POST error = %v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("blocked POST did not return")
	}
}
