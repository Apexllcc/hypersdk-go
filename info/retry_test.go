package info_test

import (
	"bytes"
	"context"
	"errors"
	"io"
	"net/http"
	"sync/atomic"
	"testing"
	"time"

	"github.com/Apexllcc/hypersdk-go/info"
	"github.com/Apexllcc/hypersdk-go/transport"
)

type trackingReadCloser struct {
	io.Reader
	closed atomic.Bool
}

func (r *trackingReadCloser) Close() error {
	r.closed.Store(true)
	return nil
}

type retryTransport struct {
	calls       atomic.Int32
	firstStatus int
	retryAfter  string
}

func (t *retryTransport) Do(_ context.Context, r *http.Request) (*http.Response, error) {
	body, _ := io.ReadAll(r.Body)
	if string(body) != "{\"type\":\"allMids\"}" {
		return nil, io.ErrUnexpectedEOF
	}
	n := t.calls.Add(1)
	if n == 1 {
		header := make(http.Header)
		if t.retryAfter != "" {
			header.Set("Retry-After", t.retryAfter)
		}
		return &http.Response{StatusCode: t.firstStatus, Header: header, Body: io.NopCloser(bytes.NewBufferString("rate"))}, nil
	}
	return &http.Response{StatusCode: 200, Header: make(http.Header), Body: io.NopCloser(bytes.NewBufferString(`{"BTC":"1"}`))}, nil
}

func TestAllMidsRetryAfterIsBoundedByPolicy(t *testing.T) {
	tr := &retryTransport{firstStatus: http.StatusTooManyRequests, retryAfter: "60"}
	policy := transport.DefaultRetryPolicy()
	policy.BaseDelay = time.Nanosecond
	policy.MaxDelay = 5 * time.Millisecond
	policy.Jitter = nil
	c := info.NewClient("http://unused", tr, time.Second, "test", policy)

	started := time.Now()
	_, err := c.AllMids(context.Background())
	elapsed := time.Since(started)
	if err != nil {
		t.Fatal(err)
	}
	if elapsed > 250*time.Millisecond {
		t.Fatalf("retry waited %s, policy maximum is %s", elapsed, policy.MaxDelay)
	}
	if tr.calls.Load() != 2 {
		t.Fatalf("calls = %d, want 2", tr.calls.Load())
	}
}

func TestAllMidsRetryWaitStopsWhenContextIsCanceled(t *testing.T) {
	body := &trackingReadCloser{Reader: bytes.NewBufferString("unavailable")}
	oneShot := retryOnceTransport{called: make(chan struct{}), response: &http.Response{StatusCode: http.StatusServiceUnavailable, Header: make(http.Header), Body: body}}
	policy := transport.DefaultRetryPolicy()
	policy.BaseDelay = time.Second
	policy.MaxDelay = time.Second
	policy.Jitter = nil
	client := info.NewClient("http://unused", &oneShot, 2*time.Second, "test", policy)

	ctx, cancel := context.WithCancel(context.Background())
	result := make(chan error, 1)
	go func() { _, err := client.AllMids(ctx); result <- err }()
	<-oneShot.called
	cancel()

	select {
	case err := <-result:
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("AllMids error = %v, want context.Canceled", err)
		}
	case <-time.After(250 * time.Millisecond):
		t.Fatal("AllMids did not stop waiting after context cancellation")
	}
	if !body.closed.Load() {
		t.Fatal("retry response body was not closed")
	}
	if oneShot.calls.Load() != 1 {
		t.Fatalf("calls = %d, want 1", oneShot.calls.Load())
	}
}

type retryOnceTransport struct {
	calls    atomic.Int32
	called   chan struct{}
	response *http.Response
}

func (t *retryOnceTransport) Do(_ context.Context, _ *http.Request) (*http.Response, error) {
	if t.calls.Add(1) == 1 {
		close(t.called)
		return t.response, nil
	}
	return nil, errors.New("unexpected retry")
}
func TestAllMidsRetriesTransientStatusWithFreshBody(t *testing.T) {
	for _, status := range []int{429, 502, 503, 504} {
		t.Run(http.StatusText(status), func(t *testing.T) {
			tr := &retryTransport{firstStatus: status}
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
		})
	}
}
