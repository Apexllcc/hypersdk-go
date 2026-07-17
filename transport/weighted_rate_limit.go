package transport

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
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

// ErrWeightExceedsCapacity reports a request whose weight can never fit into
// an admission limiter's configured capacity.
var ErrWeightExceedsCapacity = errors.New("rate limit weight exceeds capacity")

// ErrInvalidRefillWindow reports a limiter configuration that cannot refill.
var ErrInvalidRefillWindow = errors.New("rate limit refill window must be positive")

// WeightPolicy determines the request and response weights for an API call.
// ResponseWeight is charged after a successful response so policies can account
// for endpoints whose official cost depends on the number of returned items.
// Policy methods can be invoked concurrently; implementations must be
// concurrency-safe.
type WeightPolicy interface {
	RequestWeight(RequestKind, any) uint64
	ResponseWeight(RequestKind, any, any) uint64
}

// AdmissionLimiter owns weighted admission state. A limiter can be shared by
// several middleware instances, for example by SDK clients behind one IP.
// Implementations must be concurrency-safe. A nil Wait error atomically
// reserves the requested weight, and Charge may be called concurrently with
// Wait to reserve a response-dependent surcharge.
type AdmissionLimiter interface {
	Wait(context.Context, uint64) error
	Charge(uint64)
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
	return WeightedRateLimitWithLimiter(policy, NewWeightLimiter(OfficialRateLimitBudget, officialRateLimitWindow))
}

// WeightedRateLimitWithLimiter applies policy through caller-supplied
// admission state. Passing the same limiter to multiple middleware instances
// shares one budget; nil policy or limiter leaves the transport unchanged.
func WeightedRateLimitWithLimiter(policy WeightPolicy, limiter AdmissionLimiter) Middleware {
	return func(next HTTPTransport) HTTPTransport {
		if policy == nil || limiter == nil {
			return next
		}
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
			var decoded any
			if json.Unmarshal(body, &decoded) == nil {
				limiter.Charge(policy.ResponseWeight(metadata.kind, metadata.payload, decoded))
			}
			if readErr != nil {
				response.Body = &replayReadCloser{reader: bytes.NewReader(body), err: readErr}
				return response, readErr
			}
			response.Body = io.NopCloser(bytes.NewReader(body))
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
	items := responseLength(response)
	if items == 0 {
		return 0
	}
	if kind == RequestExplorer && requestType(payload) == "blockList" {
		return uint64(items)
	}
	if kind != RequestInfo {
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
	return nestedExchangeBatchLength(action, 0)
}

func nestedExchangeBatchLength(action any, depth int) int {
	if depth == 8 {
		return 0
	}
	fields := payloadObject(action)
	if fields == nil {
		return 0
	}
	if field := exchangeBatchField(fields); field != "" {
		return batchLength(fields[field])
	}
	if actionType, _ := fields["type"].(string); actionType == "multiSig" {
		if payload := payloadObject(fields["payload"]); payload != nil {
			return nestedExchangeBatchLength(payload["action"], depth+1)
		}
	}
	return 0
}

// exchangeBatchField lists only Exchange action variants whose direct array
// payload is charged using the official 40-item batch schedule. Other arrays
// are ordinary action data and must not be inferred as batches.
func exchangeBatchField(action map[string]any) string {
	actionType, _ := action["type"].(string)
	switch actionType {
	case "order":
		return "orders"
	case "cancel", "cancelByCloid":
		return "cancels"
	case "batchModify":
		return "modifies"
	case "perpDeploy":
		for _, field := range []string{
			"setFundingMultipliers",
			"setFundingInterestRates",
			"setMarginTableIds",
			"setOpenInterestCaps",
			"setMarginModes",
			"setGrowthModes",
		} {
			if _, ok := action[field]; ok {
				return field
			}
		}
	}
	return ""
}

func batchLength(value any) int {
	reflectValue := reflect.ValueOf(value)
	if !reflectValue.IsValid() || (reflectValue.Kind() != reflect.Slice && reflectValue.Kind() != reflect.Array) {
		return 0
	}
	return reflectValue.Len()
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
	mu       sync.Mutex
	budget   float64
	tokens   float64
	window   time.Duration
	updated  time.Time
	queue    []*weightedWaiter
	changed  chan struct{}
	newTimer func(time.Duration) *time.Timer
}

type weightedWaiter struct {
	weight float64
	_      byte
}

func newWeightedRateLimiter(budget uint64, window time.Duration) *weightedRateLimiter {
	return &weightedRateLimiter{
		budget:   float64(budget),
		tokens:   float64(budget),
		window:   window,
		updated:  time.Now(),
		changed:  make(chan struct{}),
		newTimer: time.NewTimer,
	}
}

// NewWeightLimiter constructs a weighted admission limiter with the given
// capacity and refill window. Wait returns ErrWeightExceedsCapacity when a
// single requested weight is larger than capacity. For an invalid window it
// returns a limiter that rejects Wait immediately with ErrInvalidRefillWindow;
// use NewValidatedWeightLimiter when configuration errors should be returned at
// construction time.
func NewWeightLimiter(capacity uint64, window time.Duration) AdmissionLimiter {
	limiter, err := NewValidatedWeightLimiter(capacity, window)
	if err != nil {
		return rejectedAdmissionLimiter{err: err}
	}
	return limiter
}

// NewValidatedWeightLimiter constructs a weighted admission limiter after
// validating its refill window.
func NewValidatedWeightLimiter(capacity uint64, window time.Duration) (AdmissionLimiter, error) {
	if window <= 0 {
		return nil, ErrInvalidRefillWindow
	}
	return newWeightedRateLimiter(capacity, window), nil
}

type rejectedAdmissionLimiter struct{ err error }

func (l rejectedAdmissionLimiter) Wait(context.Context, uint64) error { return l.err }
func (rejectedAdmissionLimiter) Charge(uint64)                        {}

func (l *weightedRateLimiter) Wait(ctx context.Context, weight uint64) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if weight == 0 {
		return nil
	}
	if float64(weight) > l.budget {
		return ErrWeightExceedsCapacity
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
		isHead := len(l.queue) != 0 && l.queue[0] == waiter
		if isHead && l.tokens >= waiter.weight {
			l.tokens -= waiter.weight
			l.queue = l.queue[1:]
			l.notifyLocked()
			l.mu.Unlock()
			return nil
		}
		changed := l.changed
		var timer *time.Timer
		if isHead {
			newTimer := l.newTimer
			if newTimer == nil {
				newTimer = time.NewTimer
			}
			timer = newTimer(l.delayLocked(waiter.weight))
		}
		l.mu.Unlock()

		if timer == nil {
			select {
			case <-ctx.Done():
				l.remove(waiter)
				return ctx.Err()
			case <-changed:
			}
			continue
		}
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

type replayReadCloser struct {
	reader *bytes.Reader
	err    error
}

func (r *replayReadCloser) Read(data []byte) (int, error) {
	if r.reader.Len() > 0 {
		return r.reader.Read(data)
	}
	if r.err != nil {
		err := r.err
		r.err = nil
		return 0, err
	}
	return 0, io.EOF
}

func (*replayReadCloser) Close() error { return nil }
