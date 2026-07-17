package explorer_test

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/Apexllcc/hypersdk-go/explorer"
	"github.com/Apexllcc/hypersdk-go/internal/hlerr"
	"github.com/Apexllcc/hypersdk-go/transport"
	"github.com/Apexllcc/hypersdk-go/websocket"
)

func TestExplorerHTTPMethodsUseTypedRPCPayloads(t *testing.T) {
	t.Parallel()
	requests := make(chan map[string]any, 3)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer func() { _ = r.Body.Close() }()
		if r.Method != http.MethodPost || r.URL.Path != "/explorer" {
			t.Errorf("request=%s %s", r.Method, r.URL.Path)
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		body, err := io.ReadAll(r.Body)
		if err != nil {
			t.Error(err)
			return
		}
		request := map[string]any{}
		if err := json.Unmarshal(body, &request); err != nil {
			t.Error(err)
			return
		}
		requests <- request
		w.Header().Set("Content-Type", "application/json")
		switch request["type"] {
		case "blockDetails":
			_, _ = w.Write([]byte(`{"type":"blockDetails","blockDetails":{"blockTime":1,"hash":"0xaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa","height":2,"numTxs":1,"proposer":"0xbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb","txs":[{"action":{"type":"cancel","cancels":[{"a":1,"o":2}]},"block":2,"error":null,"hash":"0xcccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccc","time":1,"user":"0xdddddddddddddddddddddddddddddddddddddddd"}]}}`))
		case "txDetails":
			_, _ = w.Write([]byte(`{"type":"txDetails","tx":{"action":{"type":"order","orders":[]},"block":2,"error":"rejected","hash":"0xcccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccc","time":1,"user":"0xdddddddddddddddddddddddddddddddddddddddd"}}`))
		case "userDetails":
			_, _ = w.Write([]byte(`{"type":"userDetails","txs":[{"action":["legacy",1],"block":2,"error":null,"hash":"0xcccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccc","time":1,"user":"0xdddddddddddddddddddddddddddddddddddddddd"}]}`))
		default:
			w.WriteHeader(http.StatusBadRequest)
		}
	}))
	defer server.Close()

	client := explorer.NewClient(server.URL+"/explorer", transport.NewDefaultHTTPTransport(nil), time.Second, "test")
	block, err := client.BlockDetails(context.Background(), 2)
	if err != nil {
		t.Fatal(err)
	}
	if block.BlockDetails.Height != 2 || block.BlockDetails.Transactions[0].Action.Type != "cancel" {
		t.Fatalf("block=%+v", block)
	}
	transaction, err := client.TxDetails(context.Background(), "0x"+strings.Repeat("c", 64))
	if err != nil {
		t.Fatal(err)
	}
	if transaction.Transaction.Error == nil || *transaction.Transaction.Error != "rejected" || transaction.Transaction.Action.Type != "order" {
		t.Fatalf("transaction=%+v", transaction)
	}
	user, err := client.UserDetails(context.Background(), "0x"+strings.Repeat("d", 40))
	if err != nil {
		t.Fatal(err)
	}
	if len(user.Transactions) != 1 || len(user.Transactions[0].Action.Tuple) != 2 || user.Transactions[0].Action.Object != nil {
		t.Fatalf("user=%+v", user)
	}
	for _, want := range []string{"blockDetails", "txDetails", "userDetails"} {
		select {
		case request := <-requests:
			if request["type"] != want {
				t.Fatalf("type=%v, want %s", request["type"], want)
			}
		case <-time.After(time.Second):
			t.Fatalf("missing %s request", want)
		}
	}
}

func TestNewClientNilTransportUsesDefaultHTTPTransport(t *testing.T) {
	t.Parallel()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Fatalf("method=%s, want POST", r.Method)
		}
		_, _ = w.Write([]byte(`{"type":"blockDetails","blockDetails":{"height":2}}`))
	}))
	defer server.Close()

	client := explorer.NewClient(server.URL, nil, time.Second, "test")
	result, err := client.BlockDetails(context.Background(), 2)
	if err != nil {
		t.Fatal(err)
	}
	if result.BlockDetails.Height != 2 {
		t.Fatalf("height=%d, want 2", result.BlockDetails.Height)
	}
}

func TestExplorerValidatesInputsBeforeSending(t *testing.T) {
	t.Parallel()
	client := explorer.NewClient("http://127.0.0.1:1/explorer", transport.NewDefaultHTTPTransport(nil), time.Second, "test")
	if _, err := client.TxDetails(context.Background(), "0xnot-a-hash"); err == nil {
		t.Fatal("invalid hash was accepted")
	}
	if _, err := client.UserDetails(context.Background(), "not-an-address"); err == nil {
		t.Fatal("invalid user was accepted")
	}
	called := false
	client.SetRequestTransport(requestTransportFunc(func(context.Context, transport.RequestKind, any, any) error {
		called = true
		return nil
	}))
	if _, err := client.BlockDetails(context.Background(), 0); err == nil {
		t.Fatal("zero block height was accepted")
	}
	if called {
		t.Fatal("zero block height reached the request transport")
	}
}

func TestExplorerCloseDoesNotCloseCallerOwnedSubscriptionClient(t *testing.T) {
	t.Parallel()
	subscriptions := websocket.NewClient("ws://127.0.0.1:1")
	defer func() { _ = subscriptions.Close() }()
	client := explorer.NewClient("http://127.0.0.1:1/explorer", transport.NewDefaultHTTPTransport(nil), time.Second, "test", subscriptions)
	if err := client.Close(); err != nil {
		t.Fatal(err)
	}
	if _, err := subscriptions.SubscribeAllMids(context.Background(), websocket.AllMidsRequest{}); err != nil {
		t.Fatalf("caller-owned websocket was closed: %v", err)
	}
}

func TestExplorerSurfacesProtocolErrorsAndCancellation(t *testing.T) {
	t.Parallel()
	started := make(chan struct{}, 2)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("slow") == "true" {
			started <- struct{}{}
			select {
			case <-r.Context().Done():
			case <-time.After(100 * time.Millisecond):
			}
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"type":"error","message":"not found"}`))
	}))
	defer server.Close()
	client := explorer.NewClient(server.URL, transport.NewDefaultHTTPTransport(nil), time.Second, "test")
	_, err := client.BlockDetails(context.Background(), 2)
	var apiErr *hlerr.APIError
	if !errors.As(err, &apiErr) || apiErr.Message != "not found" {
		t.Fatalf("error=%T %[1]v", err)
	}
	slowClient := explorer.NewClient(server.URL+"?slow=true", transport.NewDefaultHTTPTransport(nil), time.Second, "test")
	ctx, cancel := context.WithCancel(context.Background())
	result := make(chan error, 1)
	go func() {
		_, callErr := slowClient.BlockDetails(ctx, 2)
		result <- callErr
	}()
	select {
	case <-started:
		cancel()
	case <-time.After(time.Second):
		t.Fatal("slow request was not sent")
	}
	err = <-result
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("error=%v, want canceled", err)
	}
	timeoutClient := explorer.NewClient(server.URL+"?slow=true", transport.NewDefaultHTTPTransport(nil), 10*time.Millisecond, "test")
	_, err = timeoutClient.BlockDetails(context.Background(), 2)
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("error=%v, want deadline exceeded", err)
	}
}

type requestTransportFunc func(context.Context, transport.RequestKind, any, any) error

func (f requestTransportFunc) Request(ctx context.Context, kind transport.RequestKind, payload any, response any) error {
	return f(ctx, kind, payload, response)
}

func TestExplorerUsesInjectedRequestTransport(t *testing.T) {
	t.Parallel()
	client := explorer.NewClient("http://unused", transport.NewDefaultHTTPTransport(nil), time.Second, "test")
	client.SetRequestTransport(requestTransportFunc(func(_ context.Context, kind transport.RequestKind, payload any, response any) error {
		if kind != transport.RequestExplorer {
			t.Fatalf("kind=%q", kind)
		}
		request := payload.(map[string]any)
		if request["type"] != "blockDetails" || request["height"] != uint64(7) {
			t.Fatalf("payload=%#v", request)
		}
		response.(*explorer.BlockDetailsResponse).BlockDetails.Height = 7
		return nil
	}))
	result, err := client.BlockDetails(context.Background(), 7)
	if err != nil || result.BlockDetails.Height != 7 {
		t.Fatalf("result=%+v err=%v", result, err)
	}
}
