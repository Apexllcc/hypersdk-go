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

func TestL2BookSubscriptionReconnectsAndRestoresSubscription(t *testing.T) {
	t.Parallel()
	var connections atomic.Int32
	upgrader := gws.Upgrader{}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		conn, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			t.Fatal(err)
		}
		defer func() { _ = conn.Close() }()
		var sub map[string]any
		if err := conn.ReadJSON(&sub); err != nil {
			t.Error(err)
			return
		}
		if sub["method"] != "subscribe" {
			t.Errorf("request=%v", sub)
		}
		n := connections.Add(1)
		_ = conn.WriteJSON(map[string]any{"channel": "l2Book", "data": map[string]any{"coin": "BTC", "time": n, "levels": [][]any{[]any{}, []any{}}}})
		if n == 1 {
			return
		}
		<-time.After(time.Second)
	}))
	defer server.Close()
	url := "ws" + strings.TrimPrefix(server.URL, "http") + "/ws"
	c := websocket.NewClient(url, websocket.Config{ReconnectDelay: 10 * time.Millisecond})
	sub, err := c.SubscribeL2Book(context.Background(), websocket.L2BookRequest{Coin: "BTC"})
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = sub.Close() }()
	for i := 0; i < 2; i++ {
		select {
		case event := <-sub.Events():
			if event.Coin != "BTC" {
				t.Fatalf("event=%+v", event)
			}
		case <-time.After(time.Second):
			t.Fatal("timed out waiting for restored subscription")
		}
	}
	if connections.Load() < 2 {
		t.Fatalf("connections=%d", connections.Load())
	}
}

func TestDuplicateL2BookSubscriptionReusesOneHandle(t *testing.T) {
	t.Parallel()
	c := websocket.NewClient("ws://example.invalid")
	first, err := c.SubscribeL2Book(context.Background(), websocket.L2BookRequest{Coin: "BTC"})
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = first.Close() }()
	second, err := c.SubscribeL2Book(context.Background(), websocket.L2BookRequest{Coin: "BTC"})
	if err != nil {
		t.Fatal(err)
	}
	if first != second {
		t.Fatal("duplicate subscription created a second handle")
	}
}
