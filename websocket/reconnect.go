package websocket

import (
	"math/rand/v2"
	"time"
)

const (
	defaultReconnectDelay    = time.Second
	defaultReconnectMaxDelay = 30 * time.Second
)

// ReconnectPolicy determines how long the connection manager waits before a
// reconnect attempt. Attempt is zero for the first reconnect after a
// connection failure and increases while the connection remains unavailable.
// Implementations must be safe for concurrent calls.
type ReconnectPolicy interface {
	Delay(attempt int) time.Duration
}

// ReconnectPolicyFunc adapts a function into a ReconnectPolicy.
type ReconnectPolicyFunc func(attempt int) time.Duration

// Delay implements ReconnectPolicy.
func (f ReconnectPolicyFunc) Delay(attempt int) time.Duration { return f(attempt) }

// ReconnectJitter adjusts a calculated reconnect delay. It makes retrying
// clients less likely to reconnect simultaneously after a shared outage.
type ReconnectJitter func(delay time.Duration) time.Duration

// ExponentialReconnectPolicy grows reconnect delays exponentially up to a
// maximum. Its jitter is applied after capping, and its result is always in
// the inclusive range [0, MaxDelay].
type ExponentialReconnectPolicy struct {
	InitialDelay time.Duration
	MaxDelay     time.Duration
	Jitter       ReconnectJitter
}

// NewExponentialReconnectPolicy constructs a bounded exponential reconnect
// policy. A nil jitter uses a randomized equal-jitter strategy, producing a
// value in [delay/2, delay].
func NewExponentialReconnectPolicy(initialDelay, maxDelay time.Duration, jitter ReconnectJitter) *ExponentialReconnectPolicy {
	if initialDelay <= 0 {
		initialDelay = defaultReconnectDelay
	}
	if maxDelay <= 0 {
		maxDelay = defaultReconnectMaxDelay
	}
	if maxDelay < initialDelay {
		maxDelay = initialDelay
	}
	if jitter == nil {
		jitter = equalReconnectJitter
	}
	return &ExponentialReconnectPolicy{InitialDelay: initialDelay, MaxDelay: maxDelay, Jitter: jitter}
}

// Delay implements ReconnectPolicy.
func (p *ExponentialReconnectPolicy) Delay(attempt int) time.Duration {
	if p == nil {
		return 0
	}
	initialDelay, maxDelay := p.InitialDelay, p.MaxDelay
	if initialDelay <= 0 {
		initialDelay = defaultReconnectDelay
	}
	if maxDelay <= 0 {
		maxDelay = defaultReconnectMaxDelay
	}
	if maxDelay < initialDelay {
		maxDelay = initialDelay
	}
	if attempt < 0 {
		attempt = 0
	}
	delay := initialDelay
	for range attempt {
		if delay >= maxDelay || delay > maxDelay/2 {
			delay = maxDelay
			break
		}
		delay *= 2
	}
	jitter := p.Jitter
	if jitter == nil {
		jitter = equalReconnectJitter
	}
	delay = jitter(delay)
	if delay < 0 {
		return 0
	}
	if delay > maxDelay {
		return maxDelay
	}
	return delay
}

func equalReconnectJitter(delay time.Duration) time.Duration {
	if delay <= 0 {
		return 0
	}
	half := delay / 2
	return half + time.Duration(rand.Int64N(int64(delay-half)+1))
}
