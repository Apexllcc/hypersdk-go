package transport

import (
	"context"
	"errors"
	"io"
	"net/http"
	"reflect"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/Apexllcc/hyperliquid-go-sdk/signing"
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

func TestOfficialWeightPolicyBlockListResponseSurcharge(t *testing.T) {
	policy := OfficialWeightPolicy()
	if got := policy.ResponseWeight(RequestExplorer, map[string]any{"type": "blockList"}, make([]any, 3)); got != 3 {
		t.Fatalf("blockList response weight = %d, want 3", got)
	}
}

func TestOfficialWeightPolicyFindsNestedAndTypedExchangeBatches(t *testing.T) {
	type order struct{ ID int }
	policy := OfficialWeightPolicy()
	tests := []struct {
		name    string
		payload any
		want    uint64
	}{
		{
			name: "typed slice",
			payload: map[string]any{"action": map[string]any{
				"type": "order", "orders": make([]order, 79),
			}},
			want: 2,
		},
		{
			name: "multi-sig envelope",
			payload: struct {
				Action signing.MultiSigEnvelopeAction `json:"action"`
			}{Action: signing.MultiSigEnvelopeAction{Payload: signing.MultiSigPayload{Action: map[string]any{
				"type": "order", "orders": make([]order, 80),
			}}}},
			want: 3,
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := policy.RequestWeight(RequestAction, test.payload); got != test.want {
				t.Fatalf("exchange weight = %d, want %d", got, test.want)
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

func TestWeightLimiterRejectsWeightOverCapacity(t *testing.T) {
	limiter := NewWeightLimiter(2, time.Minute)
	if err := limiter.Wait(context.Background(), 3); !errors.Is(err, ErrWeightExceedsCapacity) {
		t.Fatalf("Wait error = %v, want ErrWeightExceedsCapacity", err)
	}
}

func TestWeightedRateLimiterFollowersDoNotCreateRefillTimers(t *testing.T) {
	limiter := newWeightedRateLimiter(1, time.Hour)
	limiter.tokens = 0
	limiter.queue = []*weightedWaiter{{weight: 1}}
	timerCreated := make(chan struct{}, 1)
	limiter.newTimer = func(time.Duration) *time.Timer {
		timerCreated <- struct{}{}
		return time.NewTimer(time.Hour)
	}
	changed := limiter.changed
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { done <- limiter.Wait(ctx, 1) }()
	select {
	case <-changed:
	case <-time.After(time.Second):
		t.Fatal("follower did not join the queue")
	}
	select {
	case <-timerCreated:
		t.Fatal("non-head waiter created a refill timer")
	case <-time.After(30 * time.Millisecond):
	}
	cancel()
	if err := <-done; !errors.Is(err, context.Canceled) {
		t.Fatalf("Wait error = %v, want context.Canceled", err)
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

func TestWeightedRateLimitUsesInjectedAdmissionLimiter(t *testing.T) {
	limiter := &recordingAdmissionLimiter{}
	policy := fixedWeightPolicy{request: 2, response: 3}
	base := transportFunc(func(_ context.Context, _ *http.Request) (*http.Response, error) {
		return &http.Response{StatusCode: http.StatusOK, Body: io.NopCloser(strings.NewReader(`[]`))}, nil
	})
	request, _ := http.NewRequest(http.MethodPost, "http://example.test", nil)
	request = request.WithContext(ContextWithRequestMetadata(request.Context(), RequestInfo, map[string]any{"type": "anything"}))
	if _, err := WeightedRateLimitWithLimiter(policy, limiter)(base).Do(request.Context(), request); err != nil {
		t.Fatal(err)
	}
	if got := limiter.weights("wait"); !reflect.DeepEqual(got, []uint64{2}) {
		t.Fatalf("admitted weights = %v, want [2]", got)
	}
	if got := limiter.weights("charge"); !reflect.DeepEqual(got, []uint64{3}) {
		t.Fatalf("charged weights = %v, want [3]", got)
	}
}

func TestWeightedRateLimitSurchargeConsumesAdmissionCapacity(t *testing.T) {
	limiter := NewWeightLimiter(2, time.Hour)
	policy := fixedWeightPolicy{request: 1, response: 1}
	base := transportFunc(func(_ context.Context, _ *http.Request) (*http.Response, error) {
		return &http.Response{StatusCode: http.StatusOK, Body: io.NopCloser(strings.NewReader(`[]`))}, nil
	})
	wrapped := WeightedRateLimitWithLimiter(policy, limiter)(base)
	request, _ := http.NewRequest(http.MethodPost, "http://example.test", nil)
	request = request.WithContext(ContextWithRequestMetadata(request.Context(), RequestInfo, map[string]any{"type": "anything"}))
	if _, err := wrapped.Do(request.Context(), request); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(request.Context(), 20*time.Millisecond)
	defer cancel()
	_, err := wrapped.Do(ctx, request.WithContext(ctx))
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("second request error = %v, want deadline exceeded", err)
	}
}

type fixedWeightPolicy struct{ request, response uint64 }

func (p fixedWeightPolicy) RequestWeight(RequestKind, any) uint64       { return p.request }
func (p fixedWeightPolicy) ResponseWeight(RequestKind, any, any) uint64 { return p.response }

type recordingAdmissionLimiter struct {
	mu    sync.Mutex
	calls []admissionCall
}

type admissionCall struct {
	name   string
	weight uint64
}

func (l *recordingAdmissionLimiter) Wait(_ context.Context, weight uint64) error {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.calls = append(l.calls, admissionCall{name: "wait", weight: weight})
	return nil
}

func (l *recordingAdmissionLimiter) Charge(weight uint64) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.calls = append(l.calls, admissionCall{name: "charge", weight: weight})
}

func (l *recordingAdmissionLimiter) weights(name string) []uint64 {
	l.mu.Lock()
	defer l.mu.Unlock()
	var weights []uint64
	for _, call := range l.calls {
		if call.name == name {
			weights = append(weights, call.weight)
		}
	}
	return weights
}

func TestWeightedRateLimitPropagatesResponseReadError(t *testing.T) {
	want := errors.New("read failed")
	base := transportFunc(func(_ context.Context, _ *http.Request) (*http.Response, error) {
		return &http.Response{StatusCode: http.StatusOK, Body: &partialErrorBody{data: []byte(`[{"fill":1}]`), err: want}}, nil
	})
	request, _ := http.NewRequest(http.MethodPost, "http://example.test", nil)
	request = request.WithContext(ContextWithRequestMetadata(request.Context(), RequestInfo, map[string]any{"type": "userFills"}))
	response, err := WeightedRateLimit(OfficialWeightPolicy())(base).Do(request.Context(), request)
	if !errors.Is(err, want) {
		t.Fatalf("Do error = %v, want %v", err, want)
	}
	if response == nil || response.Body == nil {
		t.Fatal("response body was not preserved")
	}
	defer response.Body.Close()
	if body, readErr := io.ReadAll(response.Body); string(body) != `[{"fill":1}]` || !errors.Is(readErr, want) {
		t.Fatalf("replayed body = %q, %v", body, readErr)
	}
}

type partialErrorBody struct {
	data []byte
	err  error
	read bool
}

func (b *partialErrorBody) Read(p []byte) (int, error) {
	if !b.read {
		b.read = true
		return copy(p, b.data), nil
	}
	return 0, b.err
}

func (*partialErrorBody) Close() error { return nil }

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
