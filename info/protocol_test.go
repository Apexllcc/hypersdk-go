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
