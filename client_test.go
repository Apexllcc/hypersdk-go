package hyperliquid_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	hyperliquid "github.com/Apexllcc/hyperliquid-go-sdk"
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
