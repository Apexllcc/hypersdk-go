package exchange

import (
	"context"
	"testing"
	"time"

	"github.com/Apexllcc/hyperliquid-go-sdk/asset"
	"github.com/Apexllcc/hyperliquid-go-sdk/nonce"
	"github.com/Apexllcc/hyperliquid-go-sdk/transport"
)

type actionRequestTransport func(context.Context, transport.RequestKind, any, any) error

func (f actionRequestTransport) Request(ctx context.Context, kind transport.RequestKind, payload any, response any) error {
	return f(ctx, kind, payload, response)
}

func TestExchangeRequestTransportSubmitsActionOnce(t *testing.T) {
	client := NewClient("unused", "mainnet", transport.NewDefaultHTTPTransport(nil), time.Second, nil, nonce.NewMonotonicManager(nil), asset.NewStaticResolver(nil), "test")
	attempts := 0
	client.SetRequestTransport(actionRequestTransport(func(_ context.Context, kind transport.RequestKind, _ any, response any) error {
		attempts++
		if kind != transport.RequestAction {
			t.Fatalf("kind=%q, want action", kind)
		}
		response.(*ActionResponse).Status = "ok"
		return nil
	}))
	response, err := client.post(context.Background(), map[string]string{"action": "already-signed"})
	if err != nil || response.Status != "ok" || attempts != 1 {
		t.Fatalf("response=%+v err=%v attempts=%d", response, err, attempts)
	}
}
