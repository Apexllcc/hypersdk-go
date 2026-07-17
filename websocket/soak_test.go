//go:build soak

package websocket_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/Apexllcc/hypersdk-go/websocket"
	gws "github.com/gorilla/websocket"
)

// TestWebSocketSoak repeatedly recovers a subscription from a local flaky
// peer. It is intentionally opt-in: run with `-tags=soak` and set
// HL_SOAK_DURATION (for example, `2h`) for a long-duration leak/recovery run.
func TestWebSocketSoak(t *testing.T) {
	duration, err := time.ParseDuration(os.Getenv("HL_SOAK_DURATION"))
	if err != nil || duration <= 0 {
		t.Skip("set HL_SOAK_DURATION and run with -tags=soak to enable the soak test")
	}
	upgrader := gws.Upgrader{}
	var connections atomic.Int32
	var subscriptions atomic.Int32
	var subscribedConnections atomic.Int32
	var activeConnections atomic.Int32
	var activeHandlers atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		connection, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			t.Error(err)
			return
		}
		connections.Add(1)
		activeConnections.Add(1)
		activeHandlers.Add(1)
		defer func() {
			activeHandlers.Add(-1)
			activeConnections.Add(-1)
			_ = connection.Close()
		}()
		sawSubscribe := false
		for {
			var request struct {
				Method string `json:"method"`
			}
			if err := connection.ReadJSON(&request); err != nil {
				return
			}
			if request.Method == "subscribe" {
				subscriptions.Add(1)
				if !sawSubscribe {
					sawSubscribe = true
					subscribedConnections.Add(1)
				}
			}
		}
	}))
	defer server.Close()

	client := websocket.NewClient("ws"+strings.TrimPrefix(server.URL, "http"), websocket.Config{
		ReconnectDelay: 2 * time.Millisecond,
		PingInterval:   5 * time.Millisecond,
		PongWait:       20 * time.Millisecond,
	})
	t.Cleanup(func() { _ = client.Close() })
	if _, err := client.SubscribeAllMids(context.Background(), websocket.AllMidsRequest{}); err != nil {
		t.Fatal(err)
	}

	deadline := time.Now().Add(duration)
	for time.Now().Before(deadline) {
		time.Sleep(100 * time.Millisecond)
	}
	if connections.Load() < 2 {
		t.Fatalf("soak did not exercise recovery; connections = %d", connections.Load())
	}
	if subscriptions.Load() < 2 {
		t.Fatalf("soak did not observe restored subscriptions; subscriptions = %d", subscriptions.Load())
	}
	if subscribedConnections.Load() < 2 {
		t.Fatalf("soak did not observe subscriptions on separate recovered connections; subscribed connections = %d", subscribedConnections.Load())
	}
	if err := client.Close(); err != nil {
		t.Fatal(err)
	}
	deadline = time.Now().Add(time.Second)
	for (activeHandlers.Load() != 0 || activeConnections.Load() != 0) && time.Now().Before(deadline) {
		time.Sleep(time.Millisecond)
	}
	if handlers, active := activeHandlers.Load(), activeConnections.Load(); handlers != 0 || active != 0 {
		t.Fatalf("server handlers/connections leaked after client close: handlers=%d connections=%d", handlers, active)
	}
}
