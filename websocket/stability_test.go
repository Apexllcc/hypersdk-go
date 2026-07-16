package websocket_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/Apexllcc/hyperliquid-go-sdk/websocket"
	gws "github.com/gorilla/websocket"
)

func TestConcurrentSubscriptionChurnLeavesClientClosable(t *testing.T) {
	upgrader := gws.Upgrader{}
	anchorSubscribed := make(chan struct{})
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		connection, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			t.Error(err)
			return
		}
		defer func() { _ = connection.Close() }()
		for {
			var request struct {
				Method       string `json:"method"`
				Subscription struct {
					Type string `json:"type"`
				} `json:"subscription"`
			}
			if err := connection.ReadJSON(&request); err != nil {
				return
			}
			if request.Method == "subscribe" && request.Subscription.Type == "allMids" {
				select {
				case anchorSubscribed <- struct{}{}:
				default:
				}
			}
		}
	}))
	defer server.Close()

	client := websocket.NewClient("ws"+strings.TrimPrefix(server.URL, "http"), websocket.Config{ReconnectDelay: time.Millisecond})
	t.Cleanup(func() { _ = client.Close() })
	anchor, err := client.SubscribeAllMids(context.Background(), websocket.AllMidsRequest{})
	if err != nil {
		t.Fatal(err)
	}
	select {
	case <-anchorSubscribed:
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for anchored subscription")
	}
	var workers sync.WaitGroup
	for worker := range 12 {
		workers.Add(1)
		go func(worker int) {
			defer workers.Done()
			for iteration := range 30 {
				subscription, err := client.SubscribeTrades(context.Background(), websocket.TradesRequest{Coin: "COIN" + strconv.Itoa(worker) + "_" + strconv.Itoa(iteration)})
				if err != nil {
					t.Errorf("SubscribeTrades(%d, %d): %v", worker, iteration, err)
					return
				}
				if err := subscription.Close(); err != nil {
					t.Errorf("Close(%d, %d): %v", worker, iteration, err)
					return
				}
			}
		}(worker)
	}
	workers.Wait()
	if err := anchor.Close(); err != nil {
		t.Fatal(err)
	}

	if err := client.Close(); err != nil {
		t.Fatal(err)
	}
}

func TestHalfOpenConnectionReachesReadDeadlineAndReconnects(t *testing.T) {
	upgrader := gws.Upgrader{}
	var connections atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		connection, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			t.Error(err)
			return
		}
		connections.Add(1)
		defer func() { _ = connection.Close() }()
		// Drain subscriptions and application heartbeat messages but never send a
		// response. The client must treat the silent peer as half-open once its
		// configured read deadline passes and then restore its subscription.
		for {
			if _, _, err := connection.ReadMessage(); err != nil {
				return
			}
		}
	}))
	defer server.Close()

	client := websocket.NewClient("ws"+strings.TrimPrefix(server.URL, "http"), websocket.Config{
		ReconnectDelay: time.Millisecond,
		PingInterval:   5 * time.Millisecond,
		PongWait:       25 * time.Millisecond,
	})
	defer func() { _ = client.Close() }()
	subscription, err := client.SubscribeAllMids(context.Background(), websocket.AllMidsRequest{})
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = subscription.Close() }()

	deadline := time.Now().Add(time.Second)
	for connections.Load() < 2 && time.Now().Before(deadline) {
		time.Sleep(time.Millisecond)
	}
	if got := connections.Load(); got < 2 {
		t.Fatalf("connections after half-open read deadline = %d, want at least 2", got)
	}
}
