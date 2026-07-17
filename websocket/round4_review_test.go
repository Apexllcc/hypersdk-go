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

type canonicalRebuildLimiter struct {
	calls    atomic.Int32
	entered  chan struct{}
	canceled chan struct{}
	release  chan struct{}
}

func (l *canonicalRebuildLimiter) Wait(ctx context.Context) error {
	if l.calls.Add(1) == 1 {
		close(l.entered)
		select {
		case <-ctx.Done():
			close(l.canceled)
			return ctx.Err()
		case <-l.release:
			return nil
		}
	}
	select {
	case <-l.release:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

func TestEquivalentRegistryAddRebuildsBlockedSubscribeWithCanonicalWire(t *testing.T) {
	upgrader := gws.Upgrader{}
	subscribeWires := make(chan map[string]any, 2)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		connection, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			t.Errorf("upgrade: %v", err)
			return
		}
		defer func() { _ = connection.Close() }()
		for {
			var request struct {
				Method       string         `json:"method"`
				Subscription map[string]any `json:"subscription"`
			}
			if err := connection.ReadJSON(&request); err != nil {
				return
			}
			if request.Method == "subscribe" {
				subscribeWires <- request.Subscription
			}
			normalized := map[string]any{"type": "spotState", "user": "0xabcdef", "ignorePortfolioMargin": false}
			if err := connection.WriteJSON(subscriptionResponse(request.Method, normalized)); err != nil {
				return
			}
		}
	}))
	defer server.Close()

	limiter := &canonicalRebuildLimiter{entered: make(chan struct{}), canceled: make(chan struct{}), release: make(chan struct{})}
	client := websocket.NewClient("ws"+strings.TrimPrefix(server.URL, "http"), websocket.Config{MessageAdmission: limiter, PingInterval: time.Hour})
	defer func() { _ = client.Close() }()
	explicitFalse := false
	first, err := client.SubscribeSpotState(context.Background(), websocket.SpotStateRequest{User: "0xABCDEF", IsPortfolioMargin: &explicitFalse})
	if err != nil {
		t.Fatal(err)
	}
	select {
	case <-limiter.entered:
	case <-time.After(time.Second):
		t.Fatal("explicit-false uppercase subscribe did not enter admission")
	}
	second, err := client.SubscribeSpotState(context.Background(), websocket.SpotStateRequest{User: "0xabcdef"})
	if err != nil {
		t.Fatal(err)
	}
	select {
	case <-limiter.canceled:
	case <-time.After(time.Second):
		t.Fatal("equivalent registry add did not cancel noncanonical subscribe admission")
	}
	close(limiter.release)
	waitForSubscribed(t, first.States())
	waitForSubscribed(t, second.States())
	select {
	case wire := <-subscribeWires:
		if wire["user"] != "0xabcdef" {
			t.Fatalf("canonical subscribe user = %#v", wire["user"])
		}
		if _, present := wire["isPortfolioMargin"]; present {
			t.Fatalf("canonical subscribe retained default field: %#v", wire)
		}
	case <-time.After(time.Second):
		t.Fatal("canonical subscribe was not sent")
	}
	select {
	case wire := <-subscribeWires:
		t.Fatalf("equivalent group sent duplicate subscribe: %#v", wire)
	case <-time.After(30 * time.Millisecond):
	}
}
