package transport

import (
	"context"
	"net/http"
	"sync"
	"time"
)

// RequestLog describes one completed HTTP transport operation. It contains no
// request or response body, so it is safe for normal operational logging.
type RequestLog struct {
	Method     string
	Path       string
	RequestID  string
	StatusCode int
	Duration   time.Duration
	Err        error
}

// RequestMetric describes one completed HTTP transport operation. It mirrors
// RequestLog but is a separate type to keep metrics callbacks independent from
// logging configuration.
type RequestMetric struct {
	Method     string
	Path       string
	RequestID  string
	StatusCode int
	Duration   time.Duration
	Err        error
}

// RequestID attaches a generated X-Request-ID when the caller has not already
// supplied one. The incoming request is cloned before its headers are changed.
func RequestID(generate func() string) Middleware {
	return func(next HTTPTransport) HTTPTransport {
		return httpTransportFunc(func(ctx context.Context, request *http.Request) (*http.Response, error) {
			if generate == nil || request.Header.Get("X-Request-ID") != "" {
				return next.Do(ctx, request)
			}
			clone := request.Clone(ctx)
			clone.Header.Set("X-Request-ID", generate())
			return next.Do(ctx, clone)
		})
	}
}

// Logging invokes log after every completed transport attempt. A nil logger is
// a no-op.
func Logging(log func(RequestLog)) Middleware {
	return observe(func(event requestEvent) {
		if log != nil {
			log(RequestLog(event))
		}
	})
}

// Metrics invokes observe after every completed transport attempt. A nil
// observer is a no-op.
func Metrics(observeMetric func(RequestMetric)) Middleware {
	return observe(func(event requestEvent) {
		if observeMetric != nil {
			observeMetric(RequestMetric(event))
		}
	})
}

// RateLimit spaces transport attempts by interval. It honors context
// cancellation while queued. A non-positive interval leaves the transport
// unchanged.
func RateLimit(interval time.Duration) Middleware {
	return func(next HTTPTransport) HTTPTransport {
		if interval <= 0 {
			return next
		}
		limiter := &rateLimiter{interval: interval}
		return httpTransportFunc(func(ctx context.Context, request *http.Request) (*http.Response, error) {
			if err := limiter.wait(ctx); err != nil {
				return nil, err
			}
			return next.Do(ctx, request)
		})
	}
}

type requestEvent struct {
	Method     string
	Path       string
	RequestID  string
	StatusCode int
	Duration   time.Duration
	Err        error
}

func observe(callback func(requestEvent)) Middleware {
	return func(next HTTPTransport) HTTPTransport {
		return httpTransportFunc(func(ctx context.Context, request *http.Request) (*http.Response, error) {
			started := time.Now()
			response, err := next.Do(ctx, request)
			event := requestEvent{
				Method:    request.Method,
				Path:      request.URL.EscapedPath(),
				RequestID: request.Header.Get("X-Request-ID"),
				Duration:  time.Since(started),
				Err:       err,
			}
			if response != nil {
				event.StatusCode = response.StatusCode
			}
			callback(event)
			return response, err
		})
	}
}

type rateLimiter struct {
	mu       sync.Mutex
	next     time.Time
	interval time.Duration
}

type httpTransportFunc func(context.Context, *http.Request) (*http.Response, error)

func (f httpTransportFunc) Do(ctx context.Context, request *http.Request) (*http.Response, error) {
	return f(ctx, request)
}

func (l *rateLimiter) wait(ctx context.Context) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	now := time.Now()
	l.mu.Lock()
	start := now
	if l.next.After(start) {
		start = l.next
	}
	l.next = start.Add(l.interval)
	l.mu.Unlock()

	delay := time.Until(start)
	if delay <= 0 {
		return ctx.Err()
	}
	timer := time.NewTimer(delay)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	}
}
