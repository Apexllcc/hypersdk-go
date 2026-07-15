package transport

import (
	"math/rand"
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
	if p.MaxAttempts <= 0 {
		p.MaxAttempts = 3
	}
	if p.BaseDelay <= 0 {
		p.BaseDelay = 100 * time.Millisecond
	}
	if p.MaxDelay <= 0 {
		p.MaxDelay = 2 * time.Second
	}
	d := p.BaseDelay << attempt
	if d > p.MaxDelay {
		d = p.MaxDelay
	}
	if p.Jitter != nil {
		d += p.Jitter(d)
	}
	return d
}
