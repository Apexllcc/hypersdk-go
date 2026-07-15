package info_test

import (
	"bytes"
	"context"
	"io"
	"net/http"
	"sync/atomic"
	"testing"
	"time"

	"github.com/Apexllcc/hyperliquid-go-sdk/info"
	"github.com/Apexllcc/hyperliquid-go-sdk/transport"
)

type retryTransport struct{ calls atomic.Int32 }

func (t *retryTransport) Do(_ context.Context, r *http.Request) (*http.Response, error) {
	body, _ := io.ReadAll(r.Body)
	if string(body) != "{\"type\":\"allMids\"}" {
		return nil, io.ErrUnexpectedEOF
	}
	n := t.calls.Add(1)
	if n == 1 {
		return &http.Response{StatusCode: 429, Header: make(http.Header), Body: io.NopCloser(bytes.NewBufferString("rate"))}, nil
	}
	return &http.Response{StatusCode: 200, Header: make(http.Header), Body: io.NopCloser(bytes.NewBufferString(`{"BTC":"1"}`))}, nil
}
func TestAllMidsRetries429WithFreshBody(t *testing.T) {
	t.Parallel()
	tr := &retryTransport{}
	p := transport.DefaultRetryPolicy()
	p.BaseDelay = time.Nanosecond
	p.Jitter = nil
	c := info.NewClient("http://unused", tr, time.Second, "test", p)
	mids, err := c.AllMids(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if mids["BTC"].String() != "1" || tr.calls.Load() != 2 {
		t.Fatalf("calls=%d mids=%v", tr.calls.Load(), mids)
	}
}
