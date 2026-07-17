package hyperliquid_test

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	hyperliquid "github.com/Apexllcc/hypersdk-go"
	"github.com/Apexllcc/hypersdk-go/exchange"
	"github.com/Apexllcc/hypersdk-go/signer"
	"github.com/Apexllcc/hypersdk-go/transport"
	"github.com/Apexllcc/hypersdk-go/types"
	"github.com/Apexllcc/hypersdk-go/websocket"
	gws "github.com/gorilla/websocket"
	"github.com/shopspring/decimal"
)

type rootPolicyRecorder struct {
	requests chan transport.RequestKind
}

func (p rootPolicyRecorder) RequestWeight(kind transport.RequestKind, _ any) uint64 {
	p.requests <- kind
	return 1
}

func (rootPolicyRecorder) ResponseWeight(transport.RequestKind, any, any) uint64 { return 0 }

type rootFixedPolicy struct{ weight uint64 }

func (p rootFixedPolicy) RequestWeight(transport.RequestKind, any) uint64 { return p.weight }
func (rootFixedPolicy) ResponseWeight(transport.RequestKind, any, any) uint64 {
	return 0
}

func TestInfoOnlyClientCallsAllMidsAtConfiguredEndpoint(t *testing.T) {
	t.Parallel()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/info" || r.Method != http.MethodPost {
			t.Fatalf("unexpected request: %s %s", r.Method, r.URL.Path)
		}
		if got := r.Header.Get("User-Agent"); got != "hypersdk-go" {
			t.Fatalf("default User-Agent = %q, want hypersdk-go", got)
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

func TestTestnetDefaultResolverLoadsOutcomeMetadata(t *testing.T) {
	local, err := signer.NewLocalPrivateKeySignerFromHex("0123456789012345678901234567890123456789012345678901234567890123")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = local.Close() }()

	var outcomeMetadataCalls, outcomeOrders int
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer func() { _ = r.Body.Close() }()
		var request struct {
			Type   string `json:"type"`
			Action struct {
				Orders []struct {
					Asset int `json:"a"`
				} `json:"orders"`
			} `json:"action"`
		}
		if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
			t.Fatalf("decode root client request: %v", err)
		}
		switch request.Type {
		case "meta":
			_, _ = w.Write([]byte(`{"universe":[{"name":"BTC","szDecimals":5,"maxLeverage":50}]}`))
		case "spotMeta":
			_, _ = w.Write([]byte(`{"tokens":[{"name":"USDC","szDecimals":6,"index":0}],"universe":[]}`))
		case "perpDexs":
			_, _ = w.Write([]byte(`[null]`))
		case "outcomeMeta":
			outcomeMetadataCalls++
			_, _ = w.Write([]byte(`{"outcomes":[{"outcome":10,"name":"outcome","description":"test","sideSpecs":[{"name":"yes"},{"name":"no"}],"quoteToken":"USDC"}],"questions":[]}`))
		case "":
			outcomeOrders++
			if len(request.Action.Orders) != 1 || request.Action.Orders[0].Asset != 100000100 {
				t.Fatalf("root testnet outcome asset = %#v, want 100000100", request.Action.Orders)
			}
			_, _ = w.Write([]byte(`{"status":"ok","response":{"type":"order","data":{"statuses":[{"resting":{"oid":1}}]}}}`))
		default:
			t.Fatalf("unexpected root client request type %q", request.Type)
		}
	}))
	defer server.Close()

	client, err := hyperliquid.NewClient(
		hyperliquid.WithTestnet(),
		hyperliquid.WithInfoBaseURL(server.URL),
		hyperliquid.WithExchangeBaseURL(server.URL),
		hyperliquid.WithDigestSigner(local),
	)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = client.Close() }()
	market := types.MarketRef{Symbol: "#100", Kind: types.Outcome}
	if _, err := client.Exchange.PlaceOrder(context.Background(), exchange.OrderRequest{
		Market: &market, IsBuy: true, Price: decimal.RequireFromString("0.5"), Size: decimal.NewFromInt(20),
		Type: exchange.LimitOrder{TimeInForce: exchange.TIFALO},
	}); err != nil {
		t.Fatalf("place outcome through root Testnet client: %v", err)
	}
	if outcomeMetadataCalls != 1 || outcomeOrders != 1 {
		t.Fatalf("outcome metadata calls=%d orders=%d, want 1 each", outcomeMetadataCalls, outcomeOrders)
	}
}

func TestWithRateLimitPolicyAppliesWeightsToRootHTTPClients(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{"BTC":"1"}`))
	}))
	defer server.Close()
	policy := rootPolicyRecorder{requests: make(chan transport.RequestKind, 1)}
	client, err := hyperliquid.NewClient(
		hyperliquid.WithInfoBaseURL(server.URL),
		hyperliquid.WithRateLimitPolicy(policy),
	)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := client.Info.AllMids(context.Background()); err != nil {
		t.Fatal(err)
	}
	select {
	case kind := <-policy.requests:
		if kind != transport.RequestInfo {
			t.Fatalf("request kind = %q, want info", kind)
		}
	case <-time.After(time.Second):
		t.Fatal("rate-limit policy was not invoked")
	}
}

func TestWithRateLimitPolicyAndLimiterSharesAdmissionAcrossRootClients(t *testing.T) {
	requests := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		requests++
		_, _ = w.Write([]byte(`{"BTC":"1"}`))
	}))
	defer server.Close()
	limiter := transport.NewWeightLimiter(1, time.Hour)
	options := []hyperliquid.Option{
		hyperliquid.WithInfoBaseURL(server.URL),
		hyperliquid.WithRateLimitPolicyAndLimiter(rootFixedPolicy{weight: 1}, limiter),
	}
	first, err := hyperliquid.NewClient(options...)
	if err != nil {
		t.Fatal(err)
	}
	second, err := hyperliquid.NewClient(options...)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := first.Info.AllMids(context.Background()); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer cancel()
	if _, err := second.Info.AllMids(ctx); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("second request error = %v, want context deadline exceeded", err)
	}
	if requests != 1 {
		t.Fatalf("HTTP requests = %d, want 1", requests)
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
