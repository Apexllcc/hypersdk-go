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

	"github.com/Apexllcc/hypersdk-go/websocket"
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

func TestHeartbeatRenewalReconnectsAndRestoresSubscription(t *testing.T) {
	const pongWait = 240 * time.Millisecond

	upgrader := gws.Upgrader{}
	var connections atomic.Int32
	initialAcknowledged := make(chan time.Time, 1)
	pingSeen := make(chan struct{}, 1)
	pongSent := make(chan time.Time, 1)
	replayed := make(chan map[string]any, 1)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		connection, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			t.Error(err)
			return
		}
		generation := connections.Add(1)
		defer func() { _ = connection.Close() }()

		for {
			var request struct {
				Method       string         `json:"method"`
				Subscription map[string]any `json:"subscription"`
			}
			if err := connection.ReadJSON(&request); err != nil {
				return
			}
			if request.Method != "subscribe" {
				continue
			}

			if generation == 1 {
				if err := connection.WriteJSON(subscriptionResponse("subscribe", request.Subscription)); err != nil {
					t.Error(err)
					return
				}
				acknowledgedAt := time.Now()
				initialAcknowledged <- acknowledgedAt

				for {
					var heartbeat struct {
						Method string `json:"method"`
					}
					if err := connection.ReadJSON(&heartbeat); err != nil {
						return
					}
					if heartbeat.Method == "ping" {
						pingSeen <- struct{}{}
						break
					}
				}

				// Arrive near the original deadline, then remain silent. This
				// inbound application frame must renew the client's read deadline.
				time.Sleep(time.Until(acknowledgedAt.Add(150 * time.Millisecond)))
				if err := connection.WriteJSON(map[string]any{"channel": "pong"}); err != nil {
					t.Error(err)
					return
				}
				pongSent <- time.Now()
				continue
			}

			replayed <- request.Subscription
			if err := connection.WriteJSON(subscriptionResponse("subscribe", request.Subscription)); err != nil {
				t.Error(err)
			}
			for {
				if _, _, err := connection.ReadMessage(); err != nil {
					return
				}
			}
		}
	}))
	defer server.Close()

	client := websocket.NewClient("ws"+strings.TrimPrefix(server.URL, "http"), websocket.Config{
		ReconnectDelay: time.Millisecond,
		PingInterval:   20 * time.Millisecond,
		PongWait:       pongWait,
	})
	defer func() { _ = client.Close() }()
	subscription, err := client.SubscribeAllMids(context.Background(), websocket.AllMidsRequest{})
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = subscription.Close() }()
	waitForSubscribed(t, subscription.States())
	// The acknowledgement deadline was renewed before subscribed was emitted,
	// so this is a conservative upper bound on that original read deadline.
	originalDeadline := time.Now().Add(pongWait)

	select {
	case <-initialAcknowledged:
	case <-time.After(time.Second):
		t.Fatal("initial subscription was not acknowledged")
	}
	select {
	case <-pingSeen:
	case <-time.After(time.Second):
		t.Fatal("server did not receive application ping")
	}
	select {
	case sentAt := <-pongSent:
		if !sentAt.Before(originalDeadline) {
			t.Fatalf("pong sent at %s, not before original deadline %s", sentAt, originalDeadline)
		}
	case <-time.After(time.Second):
		t.Fatal("server did not send application pong")
	}

	// Cross the original deadline with margin. A reconnect here would prove the
	// late inbound pong failed to renew the read deadline.
	time.Sleep(time.Until(originalDeadline.Add(40 * time.Millisecond)))
	if got := connections.Load(); got != 1 {
		t.Fatalf("connections after original deadline = %d, want 1", got)
	}

	var restored map[string]any
	select {
	case restored = <-replayed:
	case <-time.After(time.Second):
		t.Fatal("active subscription was not replayed after renewed deadline expired")
	}
	if got := restored["type"]; got != "allMids" {
		t.Fatalf("replayed subscription type = %v, want allMids", got)
	}
	for {
		select {
		case state, ok := <-subscription.States():
			if !ok {
				t.Fatal("state stream closed before restored subscribed state")
			}
			if state.State == websocket.SubscriptionStateSubscribed {
				return
			}
		case <-time.After(time.Second):
			t.Fatal("replayed subscription acknowledgement did not restore subscribed state")
		}
	}
}
