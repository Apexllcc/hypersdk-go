package info

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/Apexllcc/hyperliquid-go-sdk/transport"
)

type requestTransportFunc func(context.Context, transport.RequestKind, any, any) error

func (f requestTransportFunc) Request(ctx context.Context, kind transport.RequestKind, payload any, response any) error {
	return f(ctx, kind, payload, response)
}

type requestStatusError int

func (e requestStatusError) Error() string   { return "request failed" }
func (e requestStatusError) StatusCode() int { return int(e) }

func TestInfoRequestTransportRetriesOnlyTransientStatus(t *testing.T) {
	attempts := 0
	client := NewClient("unused", transport.NewDefaultHTTPTransport(nil), time.Second, "test", transport.RetryPolicy{MaxAttempts: 2, BaseDelay: time.Millisecond, MaxDelay: time.Millisecond, Jitter: func(time.Duration) time.Duration { return 0 }})
	client.SetRequestTransport(requestTransportFunc(func(_ context.Context, kind transport.RequestKind, _ any, response any) error {
		attempts++
		if kind != transport.RequestInfo {
			t.Fatalf("kind=%q", kind)
		}
		if attempts == 1 {
			return requestStatusError(503)
		}
		*response.(*map[string]string) = map[string]string{"BTC": "1"}
		return nil
	}))
	result := map[string]string{}
	if err := client.Raw(context.Background(), map[string]string{"type": "allMids"}, &result); err != nil {
		t.Fatal(err)
	}
	if attempts != 2 || result["BTC"] != "1" {
		t.Fatalf("attempts=%d result=%v", attempts, result)
	}
}

func TestInfoRequestTransportDoesNotRetryUnknownError(t *testing.T) {
	attempts := 0
	want := errors.New("network uncertain")
	client := NewClient("unused", transport.NewDefaultHTTPTransport(nil), time.Second, "test")
	client.SetRequestTransport(requestTransportFunc(func(context.Context, transport.RequestKind, any, any) error {
		attempts++
		return want
	}))
	err := client.Raw(context.Background(), map[string]string{"type": "allMids"}, &map[string]any{})
	if !errors.Is(err, want) || attempts != 1 {
		t.Fatalf("error=%v attempts=%d", err, attempts)
	}
}
