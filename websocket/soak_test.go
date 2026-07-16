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

	"github.com/Apexllcc/hyperliquid-go-sdk/websocket"
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
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		connection, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			t.Error(err)
			return
		}
		defer func() { _ = connection.Close() }()
		connections.Add(1)
		for {
			if _, _, err := connection.ReadMessage(); err != nil {
				return
			}
		}
	}))
	defer server.Close()

	client := websocket.NewClient("ws"+strings.TrimPrefix(server.URL, "http"), websocket.Config{
		ReconnectDelay: 2 * time.Millisecond,
		PingInterval:   5 * time.Millisecond,
		PongWait:       20 * time.Millisecond,
	})
	defer func() { _ = client.Close() }()
	subscription, err := client.SubscribeAllMids(context.Background(), websocket.AllMidsRequest{})
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = subscription.Close() }()

	deadline := time.Now().Add(duration)
	for time.Now().Before(deadline) {
		if err := context.Background().Err(); err != nil {
			t.Fatal(err)
		}
		time.Sleep(100 * time.Millisecond)
	}
	if connections.Load() < 2 {
		t.Fatalf("soak did not exercise recovery; connections = %d", connections.Load())
	}
}
