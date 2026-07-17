package transport

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"math"
	"net/http"
	"reflect"
	"sync"
	"time"
)

const (
	// OfficialRateLimitBudget is Hyperliquid's shared REST budget per minute.
	OfficialRateLimitBudget uint64 = 1200
	officialRateLimitWindow        = time.Minute
)

// WeightPolicy determines the request and response weights for an API call.
// ResponseWeight is charged after a successful response so policies can account
// for endpoints whose official cost depends on the number of returned items.
type WeightPolicy interface {
	RequestWeight(RequestKind, any) uint64
	ResponseWeight(RequestKind, any, any) uint64
}

type requestMetadataKey struct{}

type requestMetadata struct {
	kind    RequestKind
	payload any
}

// ContextWithRequestMetadata attaches API metadata used by weighted HTTP
// middleware. API clients set it automatically; custom HTTP callers may use it
// when they need a policy to distinguish Info, Exchange, and Explorer calls.
func ContextWithRequestMetadata(ctx context.Context, kind RequestKind, payload any) context.Context {
	return context.WithValue(ctx, requestMetadataKey{}, requestMetadata{kind: kind, payload: payload})
}

// WeightedRateLimit applies a shared official-style weight budget to HTTP
// attempts. It never retries requests. Calls without request metadata pass
// through unchanged, preserving generic HTTP transport behavior.
func WeightedRateLimit(policy WeightPolicy) Middleware {
	return func(next HTTPTransport) HTTPTransport {
		if policy == nil {
			return next
		}
		limiter := newWeightedRateLimiter(OfficialRateLimitBudget, officialRateLimitWindow)
		return httpTransportFunc(func(ctx context.Context, request *http.Request) (*http.Response, error) {
			metadata, ok := ctx.Value(requestMetadataKey{}).(requestMetadata)
			if !ok {
				metadata, ok = request.Context().Value(requestMetadataKey{}).(requestMetadata)
			}
			if !ok {
				return next.Do(ctx, request)
			}
			if err := limiter.Wait(ctx, policy.RequestWeight(metadata.kind, metadata.payload)); err != nil {
				return nil, err
			}
			response, err := next.Do(ctx, request)
			if err != nil || response == nil || response.Body == nil || response.StatusCode < http.StatusOK || response.StatusCode >= http.StatusMultipleChoices {
				return response, err
			}
			body, readErr := io.ReadAll(response.Body)
			_ = response.Body.Close()
			if readErr != nil {
				response.Body = io.NopCloser(bytes.NewReader(body))
				return response, nil
			}
			response.Body = io.NopCloser(bytes.NewReader(body))
			var decoded any
			if json.Unmarshal(body, &decoded) == nil {
				limiter.Charge(policy.ResponseWeight(metadata.kind, metadata.payload, decoded))
			}
			return response, nil
		})
	}
}

// OfficialWeightPolicy returns Hyperliquid's documented REST weight schedule.
func OfficialWeightPolicy() WeightPolicy { return officialWeightPolicy{} }

type officialWeightPolicy struct{}

func (officialWeightPolicy) RequestWeight(kind RequestKind, payload any) uint64 {
	switch kind {
	case RequestAction:
		return 1 + uint64(exchangeBatchLength(payload)/40)
	case RequestExplorer:
		return 40
	case RequestInfo:
		switch requestType(payload) {
		case "l2Book", "allMids", "clearinghouseState", "orderStatus", "spotClearinghouseState", "exchangeStatus":
			return 2
		case "userRole":
			return 60
		default:
			return 20
		}
	default:
		return 1
	}
}

func (officialWeightPolicy) ResponseWeight(kind RequestKind, payload, response any) uint64 {
	if kind != RequestInfo {
		return 0
	}
	items := responseLength(response)
	if items == 0 {
		return 0
	}
	switch requestType(payload) {
	case "recentTrades", "historicalOrders", "userFills", "userFillsByTime", "fundingHistory", "userFunding", "nonUserFundingUpdates", "twapHistory", "userTwapSliceFills", "userTwapSliceFillsByTime", "delegatorHistory", "delegatorRewards", "validatorStats":
		return uint64(items / 20)
	case "candleSnapshot":
		return uint64(items / 60)
	default:
		return 0
	}
}

func requestType(payload any) string {
	value := payloadObject(payload)
	typeName, _ := value["type"].(string)
	return typeName
}

func exchangeBatchLength(payload any) int {
	outer := payloadObject(payload)
	action, ok := outer["action"]
	if !ok {
		return 0
	}
	fields := payloadObject(action)
	for _, name := range []string{"orders", "cancels", "modifies"} {
		if batch, ok := fields[name].([]any); ok {
			return len(batch)
		}
	}
	return 0
}

func payloadObject(payload any) map[string]any {
	if object, ok := payload.(map[string]any); ok {
		return object
	}
	encoded, err := json.Marshal(payload)
	if err != nil {
		return nil
	}
	var object map[string]any
	if json.Unmarshal(encoded, &object) != nil {
		return nil
	}
	return object
}

func responseLength(response any) int {
	if response == nil {
		return 0
	}
	value := reflect.ValueOf(response)
	for value.Kind() == reflect.Interface || value.Kind() == reflect.Pointer {
		if value.IsNil() {
			return 0
		}
		value = value.Elem()
	}
	if value.Kind() != reflect.Slice && value.Kind() != reflect.Array {
		return 0
	}
	return value.Len()
}

// weightedRateLimiter is a FIFO token bucket. It is deliberately shared by
// each middleware instance, so concurrent client calls consume one budget.
type weightedRateLimiter struct {
	mu      sync.Mutex
	budget  float64
	tokens  float64
	window  time.Duration
	updated time.Time
	queue   []*weightedWaiter
	changed chan struct{}
}

type weightedWaiter struct {
	weight float64
	_      byte
}

func newWeightedRateLimiter(budget uint64, window time.Duration) *weightedRateLimiter {
	return &weightedRateLimiter{
		budget:  float64(budget),
		tokens:  float64(budget),
		window:  window,
		updated: time.Now(),
		changed: make(chan struct{}),
	}
}

func (l *weightedRateLimiter) Wait(ctx context.Context, weight uint64) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if weight == 0 {
		return nil
	}
	waiter := &weightedWaiter{weight: float64(weight)}
	l.mu.Lock()
	l.queue = append(l.queue, waiter)
	l.notifyLocked()
	l.mu.Unlock()

	for {
		if err := ctx.Err(); err != nil {
			l.remove(waiter)
			return err
		}
		l.mu.Lock()
		l.refillLocked(time.Now())
		if len(l.queue) != 0 && l.queue[0] == waiter && l.tokens >= waiter.weight {
			l.tokens -= waiter.weight
			l.queue = l.queue[1:]
			l.notifyLocked()
			l.mu.Unlock()
			return nil
		}
		changed := l.changed
		delay := l.delayLocked(waiter.weight)
		l.mu.Unlock()

		timer := time.NewTimer(delay)
		select {
		case <-ctx.Done():
			stopTimer(timer)
			l.remove(waiter)
			return ctx.Err()
		case <-changed:
			stopTimer(timer)
		case <-timer.C:
		}
	}
}

func (l *weightedRateLimiter) Charge(weight uint64) {
	if weight == 0 {
		return
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	l.refillLocked(time.Now())
	l.tokens -= float64(weight)
	l.notifyLocked()
}

func (l *weightedRateLimiter) refillLocked(now time.Time) {
	if l.window <= 0 || l.budget <= 0 {
		return
	}
	elapsed := now.Sub(l.updated)
	if elapsed <= 0 {
		return
	}
	l.tokens = math.Min(l.budget, l.tokens+elapsed.Seconds()*l.budget/l.window.Seconds())
	l.updated = now
}

func (l *weightedRateLimiter) delayLocked(weight float64) time.Duration {
	if l.budget <= 0 || l.window <= 0 {
		return time.Hour
	}
	missing := weight - l.tokens
	if missing <= 0 {
		return time.Millisecond
	}
	delay := time.Duration(math.Ceil(missing / l.budget * float64(l.window)))
	if delay <= 0 {
		return time.Nanosecond
	}
	return delay
}

func (l *weightedRateLimiter) remove(waiter *weightedWaiter) {
	l.mu.Lock()
	defer l.mu.Unlock()
	for i, queued := range l.queue {
		if queued == waiter {
			copy(l.queue[i:], l.queue[i+1:])
			l.queue[len(l.queue)-1] = nil
			l.queue = l.queue[:len(l.queue)-1]
			l.notifyLocked()
			return
		}
	}
}

func (l *weightedRateLimiter) notifyLocked() {
	close(l.changed)
	l.changed = make(chan struct{})
}

func stopTimer(timer *time.Timer) {
	if !timer.Stop() {
		select {
		case <-timer.C:
		default:
		}
	}
}
