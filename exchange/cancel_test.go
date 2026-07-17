package exchange_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/Apexllcc/hypersdk-go/asset"
	"github.com/Apexllcc/hypersdk-go/exchange"
	"github.com/Apexllcc/hypersdk-go/nonce"
	"github.com/Apexllcc/hypersdk-go/signer"
)

func TestCancelOrderUsesSharedSignedSubmission(t *testing.T) {
	t.Parallel()
	local, err := signer.NewLocalPrivateKeySignerFromHex("0123456789012345678901234567890123456789012345678901234567890123")
	if err != nil {
		t.Fatal(err)
	}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"status":"ok","response":{"type":"cancel","data":{"statuses":["success"]}}}`))
	}))
	defer server.Close()
	c := exchange.NewClient(server.URL, "mainnet", nil, 0, local, nonce.NewMonotonicManager(nil), asset.NewStaticResolver([]asset.Asset{{ID: 0, Symbol: "BTC", Kind: asset.Perp}}), "test")
	result, err := c.CancelOrder(context.Background(), exchange.CancelRequest{Coin: "BTC", OID: 123})
	if err != nil {
		t.Fatal(err)
	}
	if result.Status != "ok" {
		t.Fatalf("status=%q", result.Status)
	}
}
