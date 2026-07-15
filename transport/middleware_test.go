package transport

import (
	"context"
	"io"
	"net/http"
	"strings"
	"sync"
	"testing"
	"time"
)

type transportFunc func(context.Context, *http.Request) (*http.Response, error)

func (f transportFunc) Do(ctx context.Context, request *http.Request) (*http.Response, error) {
	return f(ctx, request)
}

func TestRequestIDAddsOnlyMissingHeader(t *testing.T) {
	var ids []string
	base := transportFunc(func(_ context.Context, request *http.Request) (*http.Response, error) {
		ids = append(ids, request.Header.Get("X-Request-ID"))
		return successResponse(), nil
	})
	wrapped := RequestID(func() string { return "generated" })(base)

	first, _ := http.NewRequest(http.MethodGet, "http://example.test", nil)
	if _, err := wrapped.Do(context.Background(), first); err != nil {
		t.Fatal(err)
	}
	second, _ := http.NewRequest(http.MethodGet, "http://example.test", nil)
	second.Header.Set("X-Request-ID", "caller-id")
	if _, err := wrapped.Do(context.Background(), second); err != nil {
		t.Fatal(err)
	}
	if strings.Join(ids, ",") != "generated,caller-id" {
		t.Fatalf("request IDs = %v", ids)
	}
}

func TestLoggingAndMetricsObserveTransportResult(t *testing.T) {
	var logs []RequestLog
	var metrics []RequestMetric
	base := transportFunc(func(_ context.Context, _ *http.Request) (*http.Response, error) {
		return &http.Response{StatusCode: http.StatusAccepted, Body: io.NopCloser(strings.NewReader("ok"))}, nil
	})
	wrapped := Logging(func(event RequestLog) { logs = append(logs, event) })(Metrics(func(event RequestMetric) { metrics = append(metrics, event) })(base))
	request, _ := http.NewRequest(http.MethodPost, "http://example.test/path", nil)
	if _, err := wrapped.Do(context.Background(), request); err != nil {
		t.Fatal(err)
	}
	if len(logs) != 1 || logs[0].StatusCode != http.StatusAccepted || logs[0].Method != http.MethodPost || logs[0].Path != "/path" || logs[0].Err != nil {
		t.Fatalf("logs = %#v", logs)
	}
	if len(metrics) != 1 || metrics[0].StatusCode != http.StatusAccepted || metrics[0].Duration < 0 || metrics[0].Err != nil {
		t.Fatalf("metrics = %#v", metrics)
	}
}

func TestRateLimitHonorsContextWhileWaiting(t *testing.T) {
	base := transportFunc(func(_ context.Context, _ *http.Request) (*http.Response, error) { return successResponse(), nil })
	wrapped := RateLimit(100 * time.Millisecond)(base)
	request, _ := http.NewRequest(http.MethodGet, "http://example.test", nil)
	if _, err := wrapped.Do(context.Background(), request); err != nil {
		t.Fatal(err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	started := time.Now()
	_, err := wrapped.Do(ctx, request)
	if err != context.Canceled {
		t.Fatalf("RateLimit error = %v, want context.Canceled", err)
	}
	if elapsed := time.Since(started); elapsed > 50*time.Millisecond {
		t.Fatalf("RateLimit returned after %s after cancellation", elapsed)
	}
}

func TestRateLimitSerializesRequests(t *testing.T) {
	var (
		mu     sync.Mutex
		starts []time.Time
	)
	base := transportFunc(func(_ context.Context, _ *http.Request) (*http.Response, error) {
		mu.Lock()
		starts = append(starts, time.Now())
		mu.Unlock()
		return successResponse(), nil
	})
	interval := 20 * time.Millisecond
	wrapped := RateLimit(interval)(base)
	request, _ := http.NewRequest(http.MethodGet, "http://example.test", nil)
	if _, err := wrapped.Do(context.Background(), request); err != nil {
		t.Fatal(err)
	}
	if _, err := wrapped.Do(context.Background(), request); err != nil {
		t.Fatal(err)
	}
	mu.Lock()
	defer mu.Unlock()
	if len(starts) != 2 || starts[1].Sub(starts[0]) < interval-2*time.Millisecond {
		t.Fatalf("starts = %v, interval = %s", starts, interval)
	}
}

func successResponse() *http.Response {
	return &http.Response{StatusCode: http.StatusOK, Body: io.NopCloser(strings.NewReader("ok"))}
}
