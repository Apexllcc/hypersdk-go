package info_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/Apexllcc/hyperliquid-go-sdk/info"
	"github.com/Apexllcc/hyperliquid-go-sdk/transport"
)

func TestCandleSnapshotUsesOfficialNestedRequestWire(t *testing.T) {
	t.Parallel()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var got map[string]any
		if err := json.NewDecoder(r.Body).Decode(&got); err != nil {
			t.Fatal(err)
		}
		if got["type"] != "candleSnapshot" {
			t.Fatalf("type: %#v", got["type"])
		}
		req, ok := got["req"].(map[string]any)
		if !ok {
			t.Fatalf("missing nested req: %#v", got)
		}
		if req["coin"] != "BTC" || req["interval"] != "1h" {
			t.Fatalf("req: %#v", req)
		}
		if _, ok := req["endTime"]; ok {
			t.Fatalf("unexpected empty endTime: %#v", req)
		}
		_, _ = w.Write([]byte(`[]`))
	}))
	defer server.Close()
	c := info.NewClient(server.URL, transport.NewDefaultHTTPTransport(nil), time.Second, "test")
	_, err := c.CandleSnapshot(context.Background(), info.CandleRequest{Coin: "BTC", Interval: "1h", StartTime: 1})
	if err != nil {
		t.Fatal(err)
	}
}

func TestNewClientNilTransportUsesDefaultHTTPTransport(t *testing.T) {
	t.Parallel()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Fatalf("method=%s, want POST", r.Method)
		}
		_, _ = w.Write([]byte(`{"BTC":"100000"}`))
	}))
	defer server.Close()

	client := info.NewClient(server.URL, nil, time.Second, "test")
	result, err := client.AllMids(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if got := result["BTC"].String(); got != "100000" {
		t.Fatalf("BTC mid=%s", got)
	}
}

func TestL2BookWithOptionsUsesOfficialAggregationWire(t *testing.T) {
	t.Parallel()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var got map[string]any
		if err := json.NewDecoder(r.Body).Decode(&got); err != nil {
			t.Fatal(err)
		}
		if got["type"] != "l2Book" || got["coin"] != "BTC" || got["nSigFigs"] != float64(5) || got["mantissa"] != float64(2) {
			t.Fatalf("l2Book request = %#v", got)
		}
		_, _ = w.Write([]byte(`{"coin":"BTC","time":1,"spread":"0.123456789012345678","levels":[[],[]]}`))
	}))
	defer server.Close()
	nSigFigs, mantissa := 5, 2
	client := info.NewClient(server.URL, transport.NewDefaultHTTPTransport(nil), time.Second, "test")
	book, err := client.L2BookWithOptions(context.Background(), info.L2BookRequest{Coin: "BTC", NSigFigs: &nSigFigs, Mantissa: &mantissa})
	if err != nil {
		t.Fatal(err)
	}
	if book.Spread == nil || book.Spread.String() != "0.123456789012345678" {
		t.Fatalf("spread=%v", book.Spread)
	}
}

func TestL2BookAggregationRejectsInvalidOfficialCombinations(t *testing.T) {
	t.Parallel()
	client := info.NewClient("http://127.0.0.1:1", transport.NewDefaultHTTPTransport(nil), time.Second, "test")
	for _, request := range []info.L2BookRequest{
		{Coin: "BTC", NSigFigs: intPtr(1)},
		{Coin: "BTC", Mantissa: intPtr(1)},
		{Coin: "BTC", NSigFigs: intPtr(4), Mantissa: intPtr(1)},
		{Coin: "BTC", NSigFigs: intPtr(5), Mantissa: intPtr(3)},
	} {
		if _, err := client.L2BookWithOptions(context.Background(), request); err == nil {
			t.Fatalf("expected validation failure for %+v", request)
		}
	}
}

func TestCandleSnapshotRejectsUnsupportedOfficialInterval(t *testing.T) {
	t.Parallel()
	requests := 0
	server := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) { requests++ }))
	defer server.Close()
	client := info.NewClient(server.URL, transport.NewDefaultHTTPTransport(nil), time.Second, "test")
	if _, err := client.CandleSnapshot(context.Background(), info.CandleRequest{Coin: "BTC", Interval: "10m", StartTime: 1}); err == nil {
		t.Fatal("expected unsupported candle interval to fail before HTTP")
	}
	if requests != 0 {
		t.Fatalf("unsupported interval made %d HTTP requests", requests)
	}
}

func intPtr(value int) *int { return &value }

func TestMetaAndClearinghouseFixturesUseOfficialShapes(t *testing.T) {
	t.Parallel()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var request struct {
			Type string `json:"type"`
		}
		_ = json.NewDecoder(r.Body).Decode(&request)
		switch request.Type {
		case "meta":
			_, _ = w.Write([]byte(`{"universe":[{"name":"BTC","szDecimals":5,"maxLeverage":50,"marginTableId":50}],"marginTables":[[50,{"description":"tiered","marginTiers":[{"lowerBound":"0.0","maxLeverage":50}]}]],"collateralToken":0}`))
		case "clearinghouseState":
			_, _ = w.Write([]byte(`{"assetPositions":[{"type":"oneWay","position":{"coin":"BTC","cumFunding":{"allTime":"1.0","sinceOpen":"0.5","sinceChange":"0.1"},"leverage":{"type":"cross","value":5},"marginUsed":"1","maxLeverage":50,"positionValue":"2","returnOnEquity":"0","szi":"0.1","unrealizedPnl":"0"}}],"crossMaintenanceMarginUsed":"0","crossMarginSummary":{"accountValue":"1","totalMarginUsed":"0","totalNtlPos":"0","totalRawUsd":"0"},"marginSummary":{"accountValue":"1","totalMarginUsed":"0","totalNtlPos":"0","totalRawUsd":"0"},"time":1,"withdrawable":"1"}`))
		default:
			t.Fatalf("unexpected type %q", request.Type)
		}
	}))
	defer server.Close()
	c := info.NewClient(server.URL, transport.NewDefaultHTTPTransport(nil), time.Second, "test")
	meta, err := c.Meta(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if meta.MarginTables[0].ID != 50 || meta.Universe[0].MarginTableID != 50 {
		t.Fatalf("meta=%+v", meta)
	}
	state, err := c.ClearinghouseState(context.Background(), "0x1")
	if err != nil {
		t.Fatal(err)
	}
	if got := state.AssetPositions[0].Position.CumFunding.AllTime.String(); got != "1" {
		t.Fatalf("funding=%s", got)
	}
}
