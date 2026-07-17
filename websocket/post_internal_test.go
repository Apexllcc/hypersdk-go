package websocket

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/Apexllcc/hypersdk-go/transport"
	"github.com/gorilla/websocket"
)

func TestPostWriteTokenHonorsEarlyCancellationWithDeadline(t *testing.T) {
	manager := newPostManager(&Client{})
	<-manager.write // model another request currently owning the write token
	defer manager.unlockWrite()
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	cancel() // cancellation must win even though a deadline is also present
	err := manager.lockWrite(ctx)
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("lock error=%v, want context canceled", err)
	}
}

func TestCanceledQueuedPostDoesNotWriteOrDisconnectActiveConnection(t *testing.T) {
	upgrader := websocket.Upgrader{}
	requests := make(chan string, 3)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		connection, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			t.Errorf("upgrade: %v", err)
			return
		}
		defer func() { _ = connection.Close() }()
		for {
			var request struct {
				ID      uint64 `json:"id"`
				Request struct {
					Type    string          `json:"type"`
					Payload json.RawMessage `json:"payload"`
				} `json:"request"`
			}
			if err := connection.ReadJSON(&request); err != nil {
				return
			}
			requests <- request.Request.Type
			if request.Request.Type == "action" {
				t.Errorf("canceled queued action was written")
				return
			}
			if err := connection.WriteJSON(map[string]any{"channel": "post", "data": map[string]any{"id": request.ID, "response": map[string]any{"type": "info", "payload": map[string]any{"type": "allMids", "data": map[string]string{"BTC": "1"}}}}}); err != nil {
				return
			}
		}
	}))
	defer server.Close()
	client := NewClient("ws" + strings.TrimPrefix(server.URL, "http"))
	defer func() { _ = client.Close() }()
	if err := client.PostInfo(context.Background(), map[string]string{"type": "allMids"}, &map[string]string{}); err != nil {
		t.Fatal(err)
	}
	select {
	case kind := <-requests:
		if kind != "info" {
			t.Fatalf("preflight kind=%q", kind)
		}
	case <-time.After(time.Second):
		t.Fatal("preflight was not received")
	}
	<-client.posts.write // hold an active writer token without touching the socket
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	result := make(chan error, 1)
	go func() { result <- client.PostAction(ctx, map[string]string{"action": "noop"}, &map[string]any{}) }()
	cancel()
	select {
	case err := <-result:
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("queued error=%v, want canceled", err)
		}
	case <-time.After(time.Second):
		t.Fatal("queued request did not cancel")
	}
	client.posts.unlockWrite()
	if err := client.Request(context.Background(), transport.RequestInfo, map[string]string{"type": "allMids"}, &map[string]string{}); err != nil {
		t.Fatalf("connection was affected by queued cancellation: %v", err)
	}
	select {
	case kind := <-requests:
		if kind != "info" {
			t.Fatalf("second wire request=%q, want info", kind)
		}
	case <-time.After(time.Second):
		t.Fatal("follow-up request was not received")
	}
}
