package transport

import (
	"context"
	"errors"
	"io"
	"net/http"
	"strings"
	"sync"
	"testing"
	"time"
)

func TestOfficialWeightPolicyRequestWeights(t *testing.T) {
	policy := OfficialWeightPolicy()
	tests := []struct {
		name    string
		kind    RequestKind
		payload any
		want    uint64
	}{
		{"cheap info", RequestInfo, map[string]any{"type": "allMids"}, 2},
		{"ordinary info", RequestInfo, map[string]any{"type": "meta"}, 20},
		{"expensive info", RequestInfo, map[string]any{"type": "userRole"}, 60},
		{"exchange action", RequestAction, map[string]any{"action": map[string]any{"type": "order", "orders": make([]any, 79)}}, 2},
		{"exchange batch boundary", RequestAction, map[string]any{"action": map[string]any{"type": "order", "orders": make([]any, 80)}}, 3},
		{"explorer", RequestExplorer, map[string]any{"type": "blockDetails"}, 40},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := policy.RequestWeight(test.kind, test.payload); got != test.want {
				t.Fatalf("RequestWeight(%s, %#v) = %d, want %d", test.kind, test.payload, got, test.want)
			}
		})
	}
}

func TestOfficialWeightPolicyResponseSurcharges(t *testing.T) {
	policy := OfficialWeightPolicy()
	tests := []struct {
		name     string
		payload  any
		response any
		want     uint64
	}{
		{"eligible info response", map[string]any{"type": "userFills"}, make([]any, 40), 2},
		{"candle response", map[string]any{"type": "candleSnapshot"}, make([]any, 120), 2},
		{"ordinary info response", map[string]any{"type": "meta"}, make([]any, 200), 0},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := policy.ResponseWeight(RequestInfo, test.payload, test.response); got != test.want {
				t.Fatalf("ResponseWeight(%#v, %#v) = %d, want %d", test.payload, test.response, got, test.want)
			}
		})
	}
}

func TestWeightedRateLimiterRefillsAndHonorsCancellation(t *testing.T) {
	limiter := newWeightedRateLimiter(2, 40*time.Millisecond)
	if err := limiter.Wait(context.Background(), 2); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Millisecond)
	defer cancel()
	if err := limiter.Wait(ctx, 1); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("Wait error = %v, want context deadline exceeded", err)
	}
	time.Sleep(25 * time.Millisecond)
	if err := limiter.Wait(context.Background(), 1); err != nil {
		t.Fatalf("Wait after refill = %v", err)
	}
}

func TestWeightedRateLimiterPreservesConcurrentFIFO(t *testing.T) {
	limiter := newWeightedRateLimiter(1, 15*time.Millisecond)
	if err := limiter.Wait(context.Background(), 1); err != nil {
		t.Fatal(err)
	}
	started := make(chan string, 3)
	for _, name := range []string{"one", "two", "three"} {
		name := name
		go func() {
			if err := limiter.Wait(context.Background(), 1); err == nil {
				started <- name
			}
		}()
		time.Sleep(time.Millisecond)
	}
	for _, want := range []string{"one", "two", "three"} {
		select {
		case got := <-started:
			if got != want {
				t.Fatalf("admission order = %q, want %q", got, want)
			}
		case <-time.After(time.Second):
			t.Fatal("timed out waiting for admission")
		}
	}
}

func TestWeightedRateLimitPassesTypedRequestAndResponseToPolicy(t *testing.T) {
	policy := &recordingWeightPolicy{}
	base := transportFunc(func(_ context.Context, _ *http.Request) (*http.Response, error) {
		return &http.Response{StatusCode: http.StatusOK, Body: io.NopCloser(strings.NewReader(`[{"fill": 1}, {"fill": 2}]`))}, nil
	})
	wrapped := WeightedRateLimit(policy)(base)
	request, _ := http.NewRequest(http.MethodPost, "http://example.test", nil)
	request = request.WithContext(ContextWithRequestMetadata(request.Context(), RequestInfo, map[string]any{"type": "userFills"}))
	response, err := wrapped.Do(request.Context(), request)
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	if body, err := io.ReadAll(response.Body); err != nil || string(body) != `[{"fill": 1}, {"fill": 2}]` {
		t.Fatalf("response body = %q, %v", body, err)
	}
	policy.mu.Lock()
	defer policy.mu.Unlock()
	if policy.requestKind != RequestInfo || policy.requestType != "userFills" || policy.responseItems != 2 {
		t.Fatalf("policy records = %#v", policy)
	}
}

func TestWeightedRateLimitDoesNotReplayExchangeAttempt(t *testing.T) {
	policy := &recordingWeightPolicy{}
	attempts := 0
	want := errors.New("network failed")
	base := transportFunc(func(_ context.Context, _ *http.Request) (*http.Response, error) {
		attempts++
		return nil, want
	})
	wrapped := WeightedRateLimit(policy)(base)
	request, _ := http.NewRequest(http.MethodPost, "http://example.test", nil)
	request = request.WithContext(ContextWithRequestMetadata(request.Context(), RequestAction, map[string]any{"action": map[string]any{"type": "order"}}))
	_, err := wrapped.Do(request.Context(), request)
	if !errors.Is(err, want) {
		t.Fatalf("Do error = %v, want %v", err, want)
	}
	if attempts != 1 {
		t.Fatalf("Exchange attempts = %d, want 1", attempts)
	}
}

type recordingWeightPolicy struct {
	mu            sync.Mutex
	requestKind   RequestKind
	requestType   string
	responseItems int
}

func (p *recordingWeightPolicy) RequestWeight(kind RequestKind, payload any) uint64 {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.requestKind = kind
	p.requestType = requestType(payload)
	return 1
}

func (p *recordingWeightPolicy) ResponseWeight(_ RequestKind, _ any, response any) uint64 {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.responseItems = responseLength(response)
	return 0
}
