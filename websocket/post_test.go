package websocket_test

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/Apexllcc/hyperliquid-go-sdk/transport"
	ws "github.com/Apexllcc/hyperliquid-go-sdk/websocket"
	"github.com/gorilla/websocket"
)

func postTestServer(t *testing.T, handler func(*websocket.Conn)) string {
	t.Helper()
	upgrader := websocket.Upgrader{}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		connection, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			t.Errorf("upgrade: %v", err)
			return
		}
		defer func() { _ = connection.Close() }()
		handler(connection)
	}))
	t.Cleanup(server.Close)
	return "ws" + strings.TrimPrefix(server.URL, "http")
}

func TestPostInfoCorrelatesConcurrentResponses(t *testing.T) {
	url := postTestServer(t, func(connection *websocket.Conn) {
		var first, second struct {
			Method  string `json:"method"`
			ID      uint64 `json:"id"`
			Request struct {
				Type    string         `json:"type"`
				Payload map[string]any `json:"payload"`
			} `json:"request"`
		}
		if err := connection.ReadJSON(&first); err != nil {
			t.Errorf("first request: %v", err)
			return
		}
		if err := connection.ReadJSON(&second); err != nil {
			t.Errorf("second request: %v", err)
			return
		}
		if first.Method != "post" || second.Method != "post" || first.Request.Type != "info" || second.Request.Type != "info" || first.ID == second.ID {
			t.Errorf("bad requests: %#v %#v", first, second)
			return
		}
		for _, request := range []struct {
			ID      uint64
			Payload map[string]any
		}{{second.ID, second.Request.Payload}, {first.ID, first.Request.Payload}} {
			if err := connection.WriteJSON(map[string]any{"channel": "post", "data": map[string]any{"id": request.ID, "response": map[string]any{"type": "info", "payload": map[string]any{"type": "test", "data": request.Payload}}}}); err != nil {
				t.Errorf("response: %v", err)
				return
			}
		}
	})
	client := ws.NewClient(url)
	defer func() { _ = client.Close() }()
	var gotOne, gotTwo struct {
		Name string `json:"name"`
	}
	var group sync.WaitGroup
	group.Add(2)
	go func() {
		defer group.Done()
		if err := client.Request(context.Background(), transport.RequestInfo, map[string]string{"name": "one"}, &gotOne); err != nil {
			t.Errorf("first: %v", err)
		}
	}()
	go func() {
		defer group.Done()
		if err := client.Request(context.Background(), transport.RequestInfo, map[string]string{"name": "two"}, &gotTwo); err != nil {
			t.Errorf("second: %v", err)
		}
	}()
	group.Wait()
	if gotOne.Name != "one" || gotTwo.Name != "two" {
		t.Fatalf("responses were not correlated: %#v %#v", gotOne, gotTwo)
	}
}

func TestPostCancellationRemovesPendingRequest(t *testing.T) {
	requestSeen := make(chan uint64, 1)
	url := postTestServer(t, func(connection *websocket.Conn) {
		var request struct {
			ID uint64 `json:"id"`
		}
		if err := connection.ReadJSON(&request); err != nil {
			t.Errorf("request: %v", err)
			return
		}
		requestSeen <- request.ID
		<-time.After(100 * time.Millisecond)
		_ = connection.WriteJSON(map[string]any{"channel": "post", "data": map[string]any{"id": request.ID, "response": map[string]any{"type": "info", "payload": map[string]any{"type": "test", "data": map[string]any{"late": true}}}}})
	})
	client := ws.NewClient(url)
	defer func() { _ = client.Close() }()
	ctx, cancel := context.WithCancel(context.Background())
	result := make(chan error, 1)
	go func() {
		result <- client.Request(ctx, transport.RequestInfo, map[string]string{"type": "allMids"}, &map[string]any{})
	}()
	select {
	case <-requestSeen:
		cancel()
	case <-time.After(time.Second):
		t.Fatal("request not observed")
	}
	select {
	case err := <-result:
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("error=%v, want canceled", err)
		}
	case <-time.After(time.Second):
		t.Fatal("canceled request did not return")
	}
}

func TestPostActionErrorIsProtocolError(t *testing.T) {
	url := postTestServer(t, func(connection *websocket.Conn) {
		var request struct {
			ID      uint64 `json:"id"`
			Request struct {
				Type string `json:"type"`
			} `json:"request"`
		}
		if err := connection.ReadJSON(&request); err != nil {
			t.Errorf("request: %v", err)
			return
		}
		if request.Request.Type != "action" {
			t.Errorf("request type=%q", request.Request.Type)
		}
		_ = connection.WriteJSON(map[string]any{"channel": "post", "data": map[string]any{"id": request.ID, "response": map[string]any{"type": "error", "payload": "400 Bad Request"}}})
	})
	client := ws.NewClient(url)
	defer func() { _ = client.Close() }()
	err := client.Request(context.Background(), transport.RequestAction, map[string]string{"action": "noop"}, &map[string]any{})
	var postErr *ws.PostError
	if !errors.As(err, &postErr) || postErr.Message != "400 Bad Request" {
		t.Fatalf("error=%T %[1]v, want PostError", err)
	}
}

func TestPostRejectsResponseKindMismatch(t *testing.T) {
	url := postTestServer(t, func(connection *websocket.Conn) {
		var request struct {
			ID uint64 `json:"id"`
		}
		if err := connection.ReadJSON(&request); err != nil {
			t.Errorf("request: %v", err)
			return
		}
		_ = connection.WriteJSON(map[string]any{"channel": "post", "data": map[string]any{"id": request.ID, "response": map[string]any{"type": "action", "payload": map[string]any{"status": "ok"}}}})
	})
	client := ws.NewClient(url)
	defer func() { _ = client.Close() }()
	err := client.PostInfo(context.Background(), map[string]string{"type": "allMids"}, &map[string]any{})
	if !errors.Is(err, ws.ErrUnexpectedPostResponse) {
		t.Fatalf("error=%v, want unexpected response", err)
	}
}

func TestPostDisconnectFailsActionWithoutReplay(t *testing.T) {
	requests := make(chan struct{}, 2)
	url := postTestServer(t, func(connection *websocket.Conn) {
		var request struct {
			Method string `json:"method"`
		}
		if err := connection.ReadJSON(&request); err == nil && request.Method == "post" {
			requests <- struct{}{}
		}
		// Returning closes the connection before a response. The client must not
		// replay a signed action on another socket.
	})
	client := ws.NewClient(url)
	defer func() { _ = client.Close() }()
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	err := client.PostAction(ctx, map[string]string{"action": "noop"}, &map[string]any{})
	if err == nil {
		t.Fatal("disconnect returned nil error")
	}
	select {
	case <-requests:
	case <-time.After(time.Second):
		t.Fatal("action was not written")
	}
	select {
	case <-requests:
		t.Fatal("action was replayed after disconnect")
	case <-time.After(100 * time.Millisecond):
	}
}

func TestPostCloseUnblocksPendingRequest(t *testing.T) {
	seen := make(chan struct{}, 1)
	url := postTestServer(t, func(connection *websocket.Conn) {
		var request struct {
			ID uint64 `json:"id"`
		}
		if err := connection.ReadJSON(&request); err == nil {
			seen <- struct{}{}
		}
		<-time.After(time.Second)
	})
	client := ws.NewClient(url)
	result := make(chan error, 1)
	go func() {
		result <- client.PostInfo(context.Background(), map[string]string{"type": "allMids"}, &map[string]any{})
	}()
	select {
	case <-seen:
	case <-time.After(time.Second):
		t.Fatal("request was not written")
	}
	if err := client.Close(); err != nil {
		t.Fatal(err)
	}
	select {
	case err := <-result:
		if !errors.Is(err, ws.ErrWebSocketClosed) {
			t.Fatalf("error=%v, want websocket closed", err)
		}
	case <-time.After(time.Second):
		t.Fatal("Close did not unblock request")
	}
}
