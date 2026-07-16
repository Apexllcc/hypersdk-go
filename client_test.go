package hyperliquid_test

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	hyperliquid "github.com/Apexllcc/hyperliquid-go-sdk"
	"github.com/Apexllcc/hyperliquid-go-sdk/websocket"
	gws "github.com/gorilla/websocket"
)

func TestInfoOnlyClientCallsAllMidsAtConfiguredEndpoint(t *testing.T) {
	t.Parallel()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/info" || r.Method != http.MethodPost {
			t.Fatalf("unexpected request: %s %s", r.Method, r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"BTC":"100000.25"}`))
	}))
	defer server.Close()

	client, err := hyperliquid.NewClient(hyperliquid.WithInfoBaseURL(server.URL + "/info"))
	if err != nil {
		t.Fatal(err)
	}
	mids, err := client.Info.AllMids(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if got := mids["BTC"].String(); got != "100000.25" {
		t.Fatalf("mid = %q", got)
	}
}

func TestRootClientCloseClosesOwnedWebSocketAndIsIdempotent(t *testing.T) {
	t.Parallel()
	client, err := hyperliquid.NewClient()
	if err != nil {
		t.Fatal(err)
	}
	if err := client.Close(); err != nil {
		t.Fatalf("first close: %v", err)
	}
	if err := client.Close(); err != nil {
		t.Fatalf("second close: %v", err)
	}
	var response map[string]any
	err = client.WebSocket.PostInfo(context.Background(), map[string]any{"type": "allMids"}, &response)
	if !errors.Is(err, websocket.ErrWebSocketClosed) {
		t.Fatalf("post after close error = %v, want ErrWebSocketClosed", err)
	}
}

func TestRootClientCloseReleasesLazyExplorerSubscription(t *testing.T) {
	t.Parallel()
	connected := make(chan struct{})
	disconnected := make(chan struct{})
	upgrader := gws.Upgrader{}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		connection, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			t.Errorf("upgrade: %v", err)
			return
		}
		defer func() { _ = connection.Close() }()
		defer close(disconnected)
		var request struct {
			Method       string         `json:"method"`
			Subscription map[string]any `json:"subscription"`
		}
		if err := connection.ReadJSON(&request); err != nil {
			t.Errorf("read subscription: %v", err)
			return
		}
		if request.Method != "subscribe" || request.Subscription["type"] != "explorerBlock" {
			t.Errorf("request=%+v", request)
			return
		}
		close(connected)
		_, _, _ = connection.ReadMessage()
	}))
	defer server.Close()

	client, err := hyperliquid.NewClient(hyperliquid.WithExplorerWebSocketURL("ws" + strings.TrimPrefix(server.URL, "http")))
	if err != nil {
		t.Fatal(err)
	}
	subscription, err := client.Explorer.ExplorerBlock(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	select {
	case <-connected:
	case <-time.After(time.Second):
		t.Fatal("lazy Explorer connection was not established")
	}
	if err := client.Close(); err != nil {
		t.Fatal(err)
	}
	select {
	case <-disconnected:
	case <-time.After(time.Second):
		t.Fatal("lazy Explorer connection was not released")
	}
	if _, ok := <-subscription.Events(); ok {
		t.Fatal("Explorer subscription events remain open after root close")
	}
}

func TestRootClientIntegratesExplorerAtConfiguredRPCEndpoint(t *testing.T) {
	t.Parallel()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/explorer" || r.Method != http.MethodPost {
			t.Fatalf("unexpected request: %s %s", r.Method, r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"type":"blockDetails","blockDetails":{"blockTime":1,"hash":"0xhash","height":2,"numTxs":0,"proposer":"0xproposer","txs":[]}}`))
	}))
	defer server.Close()

	client, err := hyperliquid.NewClient(hyperliquid.WithExplorerBaseURL(server.URL + "/explorer"))
	if err != nil {
		t.Fatal(err)
	}
	if client.Explorer == nil {
		t.Fatal("root Explorer client is nil")
	}
	block, err := client.Explorer.BlockDetails(context.Background(), 2)
	if err != nil || block.BlockDetails.Height != 2 {
		t.Fatalf("block=%+v err=%v", block, err)
	}
}
