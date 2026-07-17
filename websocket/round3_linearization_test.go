package websocket

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/gorilla/websocket"
)

func TestRejectionLinearizesAgainstLateEquivalentRegistryHandle(t *testing.T) {
	upgrader := websocket.Upgrader{}
	firstWire := make(chan map[string]any, 1)
	sendError := make(chan struct{})
	var wireCount atomic.Int32
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
		wireCount.Add(1)
		firstWire <- request.Subscription
		<-sendError
		echoed, _ := json.Marshal(request)
		if err := connection.WriteJSON(map[string]any{"channel": "error", "data": "Invalid subscription: " + string(echoed)}); err != nil {
			return
		}
		_ = connection.SetReadDeadline(time.Now().Add(300 * time.Millisecond))
		if err := connection.ReadJSON(&request); err == nil {
			wireCount.Add(1)
		}
	}))
	defer server.Close()

	client := NewClient("ws"+strings.TrimPrefix(server.URL, "http"), Config{PingInterval: time.Hour, ReconnectDelay: time.Millisecond})
	defer func() { _ = client.Close() }()
	initial, err := client.SubscribeSpotState(context.Background(), SpotStateRequest{User: "0xABCDEF"})
	if err != nil {
		t.Fatal(err)
	}
	select {
	case <-firstWire:
	case <-time.After(time.Second):
		t.Fatal("initial spotState request did not reach wire")
	}

	explicitFalse := false
	lateWire := newSubscriptionWire("spotState", map[string]any{"user": "0xabcdef", "isPortfolioMargin": explicitFalse})
	late := newStreamSubscription(context.Background(), client, "spotState:0xabcdef:false", "spotState", lateWire, decodeJSON[SpotStateEvent], func(event SpotStateEvent) bool {
		return strings.EqualFold(event.User, "0xabcdef")
	})
	client.mu.Lock()
	client.subs[late.key] = late
	client.handles[late.key] = &SpotStateSubscription{late}
	late.stateChange(SubscriptionStateConnecting, nil)
	client.mu.Unlock()

	close(sendError)
	select {
	case <-initial.done:
	case <-time.After(time.Second):
		t.Fatal("initial rejected handle did not terminate")
	}
	client.manager.notify()
	select {
	case <-late.done:
	case <-time.After(300 * time.Millisecond):
		t.Fatal("late server-equivalent handle survived rejection boundary")
	}
	time.Sleep(50 * time.Millisecond)
	if got := wireCount.Load(); got != 1 {
		t.Fatalf("rejected server-equivalent group reached wire %d times, want 1", got)
	}
	client.mu.Lock()
	_, retained := client.subs[late.key]
	client.mu.Unlock()
	if retained {
		t.Fatal("late rejected handle remained in resubscribe registry")
	}
}
