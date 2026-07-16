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
	"github.com/shopspring/decimal"
)

func TestPredictedFundingsDecodesTypedNullableVenueData(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer func() { _ = r.Body.Close() }()
		var request struct {
			Type string `json:"type"`
		}
		if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
			t.Fatal(err)
		}
		if request.Type != "predictedFundings" {
			t.Fatalf("type = %q, want predictedFundings", request.Type)
		}
		_, _ = w.Write([]byte(`[["BTC",[["Hyperliquid",{"fundingRate":"0.0000125","nextFundingTime":1700000000000,"fundingIntervalHours":1,"futureField":"ignored"}],["Other",null]]],["ETH",[]]]`))
	}))
	defer server.Close()

	client := info.NewClient(server.URL, transport.NewDefaultHTTPTransport(nil), time.Second, "test")
	got, err := client.PredictedFundings(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 || got[0].Asset != "BTC" || len(got[0].Exchanges) != 2 {
		t.Fatalf("predicted fundings = %#v", got)
	}
	venue := got[0].Exchanges[0]
	if venue.Exchange != "Hyperliquid" || venue.Data == nil {
		t.Fatalf("venue = %#v", venue)
	}
	if !venue.Data.FundingRate.Equal(decimal.RequireFromString("0.0000125")) || venue.Data.NextFundingTime != 1_700_000_000_000 {
		t.Fatalf("venue data = %#v", venue.Data)
	}
	if venue.Data.FundingIntervalHours == nil || *venue.Data.FundingIntervalHours != 1 {
		t.Fatalf("funding interval = %#v", venue.Data.FundingIntervalHours)
	}
	if got[0].Exchanges[1].Data != nil {
		t.Fatalf("nullable venue data = %#v, want nil", got[0].Exchanges[1].Data)
	}
}

func TestPredictedFundingsAcceptsFutureTupleElements(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`[["BTC",[["Hyperliquid",{"fundingRate":"0","nextFundingTime":1},"future-venue-value"]],"future-asset-value"]]`))
	}))
	defer server.Close()

	client := info.NewClient(server.URL, transport.NewDefaultHTTPTransport(nil), time.Second, "test")
	got, err := client.PredictedFundings(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 || got[0].Asset != "BTC" || len(got[0].Exchanges) != 1 || got[0].Exchanges[0].Data == nil {
		t.Fatalf("predicted fundings = %#v", got)
	}
}
