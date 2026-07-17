package websocket

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/gorilla/websocket"
)

type postWaitMutationLimiter struct {
	calls    atomic.Int32
	returned chan struct{}
}

func TestSubscriptionRegistryMutationCannotLinearizeInsideFinalWriteCommit(t *testing.T) {
	upgrader := websocket.Upgrader{}
	wireSeen := make(chan struct{}, 1)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		connection, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			t.Errorf("upgrade: %v", err)
			return
		}
		defer func() { _ = connection.Close() }()
		var request subscriptionWire
		if err := connection.ReadJSON(&request); err != nil {
			return
		}
		wireSeen <- struct{}{}
		normalized := map[string]any{"type": "spotState", "user": "0xabcdef", "ignorePortfolioMargin": false}
		_ = connection.WriteJSON(map[string]any{"channel": "subscriptionResponse", "data": map[string]any{"method": "subscribe", "subscription": normalized}})
		<-time.After(time.Second)
	}))
	defer server.Close()

	client := NewClient("ws"+strings.TrimPrefix(server.URL, "http"), Config{PingInterval: time.Hour})
	defer func() { _ = client.Close() }()
	commitEntered := make(chan struct{})
	allowWrite := make(chan struct{})
	var hookOnce sync.Once
	var releaseOnce sync.Once
	releaseWrite := func() { releaseOnce.Do(func() { close(allowWrite) }) }
	defer releaseWrite()
	client.manager.beforeSubscriptionWrite = func() {
		hookOnce.Do(func() {
			close(commitEntered)
			<-allowWrite
		})
	}
	explicitFalse := false
	first, err := client.SubscribeSpotState(context.Background(), SpotStateRequest{User: "0xABCDEF", IsPortfolioMargin: &explicitFalse})
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = first.Close() }()
	select {
	case <-commitEntered:
	case <-time.After(time.Second):
		t.Fatal("subscription write did not enter final commit boundary")
	}

	type subscribeResult struct {
		subscription *SpotStateSubscription
		err          error
	}
	mutation := make(chan subscribeResult, 1)
	go func() {
		subscription, err := client.SubscribeSpotState(context.Background(), SpotStateRequest{User: "0xabcdef"})
		mutation <- subscribeResult{subscription: subscription, err: err}
	}()
	select {
	case result := <-mutation:
		if result.subscription != nil {
			_ = result.subscription.Close()
		}
		t.Fatalf("registry mutation linearized inside final write commit: %v", result.err)
	case <-time.After(50 * time.Millisecond):
	}
	releaseWrite()
	var second *SpotStateSubscription
	select {
	case result := <-mutation:
		if result.err != nil {
			t.Fatal(result.err)
		}
		second = result.subscription
	case <-time.After(time.Second):
		t.Fatal("registry mutation did not resume after write commit")
	}
	defer func() { _ = second.Close() }()
	select {
	case <-wireSeen:
	case <-time.After(time.Second):
		t.Fatal("subscription wire was not committed")
	}
}

func (l *postWaitMutationLimiter) Wait(context.Context) error {
	if l.calls.Add(1) == 1 {
		close(l.returned)
	}
	return nil
}

func TestSubscriptionWriteRechecksCanonicalFingerprintAfterWriteLock(t *testing.T) {
	upgrader := websocket.Upgrader{}
	wireSeen := make(chan map[string]any, 1)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		connection, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			t.Errorf("upgrade: %v", err)
			return
		}
		defer func() { _ = connection.Close() }()
		var request subscriptionWire
		if err := connection.ReadJSON(&request); err != nil {
			return
		}
		wireSeen <- request.Subscription
		normalized := map[string]any{"type": "spotState", "user": "0xabcdef", "ignorePortfolioMargin": false}
		_ = connection.WriteJSON(map[string]any{"channel": "subscriptionResponse", "data": map[string]any{"method": "subscribe", "subscription": normalized}})
		<-time.After(time.Second)
	}))
	defer server.Close()

	limiter := &postWaitMutationLimiter{returned: make(chan struct{})}
	client := NewClient("ws"+strings.TrimPrefix(server.URL, "http"), Config{MessageAdmission: limiter, PingInterval: time.Hour})
	defer func() { _ = client.Close() }()
	client.manager.write.Lock()
	locked := true
	defer func() {
		if locked {
			client.manager.write.Unlock()
		}
	}()
	explicitFalse := false
	first, err := client.SubscribeSpotState(context.Background(), SpotStateRequest{User: "0xABCDEF", IsPortfolioMargin: &explicitFalse})
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = first.Close() }()
	select {
	case <-limiter.returned:
	case <-time.After(time.Second):
		t.Fatal("subscription admission did not return")
	}
	second, err := client.SubscribeSpotState(context.Background(), SpotStateRequest{User: "0xabcdef"})
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = second.Close() }()
	client.manager.write.Unlock()
	locked = false

	select {
	case wire := <-wireSeen:
		if wire["user"] != "0xabcdef" {
			t.Fatalf("post-Wait wire retained stale user: %#v", wire)
		}
		if _, present := wire["isPortfolioMargin"]; present {
			t.Fatalf("post-Wait wire retained stale default field: %#v", wire)
		}
	case <-time.After(time.Second):
		t.Fatal("canonical rebuilt wire was not sent")
	}
}

func TestClientCloseUnblocksBlockingBackpressureDelivery(t *testing.T) {
	upgrader := websocket.Upgrader{}
	sendEvents := make(chan struct{})
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		connection, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			t.Errorf("upgrade: %v", err)
			return
		}
		defer func() { _ = connection.Close() }()
		var request subscriptionWire
		if err := connection.ReadJSON(&request); err != nil {
			return
		}
		if err := connection.WriteJSON(map[string]any{"channel": "subscriptionResponse", "data": map[string]any{"method": "subscribe", "subscription": request.Subscription}}); err != nil {
			return
		}
		<-sendEvents
		if err := connection.WriteJSON(map[string]any{"channel": "closeBackpressure", "data": 1}); err != nil {
			return
		}
		_ = connection.WriteJSON(map[string]any{"channel": "closeBackpressure", "data": 2})
		<-time.After(time.Second)
	}))
	defer server.Close()

	client := NewClient("ws"+strings.TrimPrefix(server.URL, "http"), Config{EventBuffer: 1, Backpressure: BackpressureBlock, PingInterval: time.Hour})
	secondDelivery := make(chan struct{})
	var decodes atomic.Int32
	subscription, err := subscribeStream(context.Background(), client, "closeBackpressure", "closeBackpressure", newSubscriptionWire("closeBackpressure", map[string]any{}), func(data json.RawMessage) (int, error) {
		var event int
		if err := json.Unmarshal(data, &event); err != nil {
			return 0, err
		}
		if decodes.Add(1) == 2 {
			close(secondDelivery)
		}
		return event, nil
	}, func(int) bool { return true }, nil)
	if err != nil {
		t.Fatal(err)
	}
	close(sendEvents)
	select {
	case <-secondDelivery:
	case <-time.After(time.Second):
		t.Fatal("second delivery did not reach blocking backpressure")
	}
	closed := make(chan error, 1)
	go func() { closed <- client.Close() }()
	select {
	case err := <-closed:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(time.Second):
		t.Fatal("Client.Close deadlocked behind blocking event delivery")
	}
	select {
	case _, ok := <-subscription.Events():
		if !ok {
			return
		}
	case <-time.After(time.Second):
		t.Fatal("subscription events were not finalized after Client.Close")
	}
	select {
	case _, ok := <-subscription.Events():
		if ok {
			t.Fatal("subscription events remained open after Client.Close")
		}
	case <-time.After(time.Second):
		t.Fatal("subscription events did not close after draining buffered event")
	}
}
