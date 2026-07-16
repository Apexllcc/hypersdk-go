package websocket_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/Apexllcc/hyperliquid-go-sdk/websocket"
	gws "github.com/gorilla/websocket"
)

func TestExplorerSubscriptionsHandleRawRPCArrays(t *testing.T) {
	upgrader := gws.Upgrader{}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		connection, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			t.Error(err)
			return
		}
		defer func() { _ = connection.Close() }()
		for i := 0; i < 2; i++ {
			var request struct {
				Method       string         `json:"method"`
				Subscription map[string]any `json:"subscription"`
			}
			if err := connection.ReadJSON(&request); err != nil {
				t.Error(err)
				return
			}
			if request.Method != "subscribe" {
				t.Errorf("method=%q", request.Method)
				return
			}
			switch request.Subscription["type"] {
			case "explorerBlock":
				if err := connection.WriteJSON([]map[string]any{{"blockTime": 1, "hash": "0xblock", "height": 2, "numTxs": 3, "proposer": "0xproposer"}}); err != nil {
					t.Error(err)
				}
			case "explorerTxs":
				if err := connection.WriteJSON([]map[string]any{{"action": map[string]any{"type": "cancel"}, "block": 2, "error": nil, "hash": "0xtx", "time": 1, "user": "0xuser"}}); err != nil {
					t.Error(err)
				}
			default:
				t.Errorf("type=%q", request.Subscription["type"])
			}
		}
		<-time.After(time.Second)
	}))
	defer server.Close()

	client := websocket.NewClient("ws" + strings.TrimPrefix(server.URL, "http"))
	defer func() { _ = client.Close() }()
	blocks, err := client.SubscribeExplorerBlock(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	txs, err := client.SubscribeExplorerTxs(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	select {
	case event := <-blocks.Events():
		if len(event) != 1 || event[0].Height != 2 || event[0].NumTransactions != 3 {
			t.Fatalf("blocks=%+v", event)
		}
	case <-time.After(time.Second):
		t.Fatal("block timeout")
	}
	select {
	case event := <-txs.Events():
		if len(event) != 1 || event[0].Action.Type != "cancel" || event[0].Block != 2 {
			t.Fatalf("txs=%+v", event)
		}
	case <-time.After(time.Second):
		t.Fatal("transaction timeout")
	}
}
