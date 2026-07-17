package info_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/Apexllcc/hypersdk-go/info"
	"github.com/Apexllcc/hypersdk-go/transport"
	"github.com/Apexllcc/hypersdk-go/types"
)

func TestOrderStatusByCloidUsesOfficialStringOIDWire(t *testing.T) {
	t.Parallel()
	cloid, err := types.ParseCloid("0x1234567890abcdef1234567890abcdef")
	if err != nil {
		t.Fatal(err)
	}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var request struct {
			Type string `json:"type"`
			User string `json:"user"`
			OID  string `json:"oid"`
		}
		if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
			t.Fatal(err)
		}
		if request.Type != "orderStatus" || request.User != "0xabc" || request.OID != cloid.String() {
			t.Fatalf("request = %#v", request)
		}
		_, _ = w.Write([]byte(`{"status":"order","order":{"order":{"order":{"coin":"BTC","limitPx":"1","oid":1,"side":"B","sz":"1","timestamp":1,"cloid":"0x1234567890abcdef1234567890abcdef"},"isPositionTpsl":false,"isTrigger":false,"orderType":"Market","origSz":"1","reduceOnly":false,"triggerCondition":"N/A","triggerPx":"0","tif":"FrontendMarket"},"status":"filled","statusTimestamp":1}}`))
	}))
	defer server.Close()

	client := info.NewClient(server.URL, transport.NewDefaultHTTPTransport(nil), time.Second, "test")
	status, err := client.OrderStatusByCloid(context.Background(), "0xabc", cloid)
	if err != nil {
		t.Fatal(err)
	}
	if status.Status != "filled" || status.StatusTimestamp != 1 || status.Order == nil || status.Order.OID != 1 || status.Order.Cloid == nil || *status.Order.Cloid != cloid.String() {
		t.Fatalf("status = %#v", status)
	}
}

func TestOrderStatusByCloidRejectsEmptyUserBeforeRequest(t *testing.T) {
	t.Parallel()
	requests := 0
	server := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
		requests++
	}))
	defer server.Close()

	client := info.NewClient(server.URL, transport.NewDefaultHTTPTransport(nil), time.Second, "test")
	if _, err := client.OrderStatusByCloid(context.Background(), "", types.Cloid{}); err == nil {
		t.Fatal("OrderStatusByCloid accepted an empty user")
	}
	if requests != 0 {
		t.Fatalf("invalid request made %d HTTP requests", requests)
	}
}
