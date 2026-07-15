package transport

import (
	"math/rand"
	"net/http"
	"strconv"
	"strings"
	"time"
)

// RetryPolicy defines bounded retry timing for unsigned Info requests only.
// Exchange clients must not use it because an uncertain signed action can have
// been accepted by the network.
type RetryPolicy struct {
	MaxAttempts int
	BaseDelay   time.Duration
	MaxDelay    time.Duration
	Jitter      func(time.Duration) time.Duration
}

func DefaultRetryPolicy() RetryPolicy {
	return RetryPolicy{MaxAttempts: 3, BaseDelay: 100 * time.Millisecond, MaxDelay: 2 * time.Second, Jitter: func(d time.Duration) time.Duration { return time.Duration(rand.Float64() * float64(d) / 2) }}
}

func (p RetryPolicy) Delay(attempt int) time.Duration {
	p = p.normalized()
	d := p.BaseDelay
	for range max(attempt, 0) {
		if d >= p.MaxDelay-d {
			d = p.MaxDelay
			break
		}
		d *= 2
	}
	if p.Jitter != nil {
		d += p.Jitter(d)
	}
	if d < 0 {
		return 0
	}
	if d > p.MaxDelay {
		return p.MaxDelay
	}
	return d
}

// RetryAfterDelay parses a server-provided Retry-After header. It accepts the
// HTTP standard's delay-seconds and HTTP-date forms and always bounds waiting
// by MaxDelay. Invalid or already elapsed values are ignored.
func (p RetryPolicy) RetryAfterDelay(header http.Header, now time.Time) (time.Duration, bool) {
	value := strings.TrimSpace(header.Get("Retry-After"))
	if value == "" {
		return 0, false
	}
	var delay time.Duration
	if seconds, err := strconv.ParseInt(value, 10, 64); err == nil {
		if seconds < 0 || seconds > int64((1<<63-1)/int64(time.Second)) {
			return 0, false
		}
		delay = time.Duration(seconds) * time.Second
	} else {
		retryAt, err := http.ParseTime(value)
		if err != nil {
			return 0, false
		}
		delay = retryAt.Sub(now)
		if delay <= 0 {
			return 0, false
		}
	}
	maxDelay := p.normalized().MaxDelay
	if delay > maxDelay {
		delay = maxDelay
	}
	return delay, true
}

func (p RetryPolicy) normalized() RetryPolicy {
	defaults := DefaultRetryPolicy()
	if p.MaxAttempts <= 0 {
		p.MaxAttempts = defaults.MaxAttempts
	}
	if p.BaseDelay <= 0 {
		p.BaseDelay = defaults.BaseDelay
	}
	if p.MaxDelay <= 0 {
		p.MaxDelay = defaults.MaxDelay
	}
	if p.BaseDelay > p.MaxDelay {
		p.BaseDelay = p.MaxDelay
	}
	return p
}
