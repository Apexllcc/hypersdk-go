package websocket

import (
	"context"
	"reflect"
	"time"

	"github.com/gorilla/websocket"
)

const (
	DefaultMaxActiveSubscriptions          = 1000
	DefaultMaxUniqueUsers                  = 10
	DefaultMaxOutgoingMessagesPerMinute    = 2000
	DefaultMaxConcurrentPosts              = 100
	defaultSubscriptionAcknowledgementWait = 10 * time.Second
)

// Dialer opens WebSocket connections. Implementations must honor context
// cancellation; otherwise Client.Close can block until an in-flight dial
// returns. It provides a seam for custom network transports and deterministic
// tests.
type Dialer interface {
	DialContext(ctx context.Context, url string) (*websocket.Conn, error)
}

type defaultDialer struct{}

func (defaultDialer) DialContext(ctx context.Context, url string) (*websocket.Conn, error) {
	connection, _, err := websocket.DefaultDialer.DialContext(ctx, url, nil)
	return connection, err
}

// BackpressurePolicy controls what happens when an event consumer cannot keep
// up with a bounded subscription queue.
type BackpressurePolicy uint8

const (
	// BackpressureBlock preserves every event but pauses socket reads while the
	// consumer is slow.
	BackpressureBlock BackpressurePolicy = iota
	// BackpressureDropNewest keeps queued events and drops the incoming event.
	BackpressureDropNewest
	// BackpressureDropOldest drops the oldest queued event to retain the latest.
	BackpressureDropOldest
)

// Config limits reconnect behavior and in-memory delivery queues.
type Config struct {
	// ReconnectDelay retains its legacy fixed-delay behavior when it is the only
	// reconnect setting supplied. When combined with ReconnectMaxDelay or
	// ReconnectJitter, it is the initial delay of the default exponential policy.
	ReconnectDelay time.Duration
	// ReconnectMaxDelay caps the default exponential reconnect policy and opts
	// into that policy when used with ReconnectDelay.
	ReconnectMaxDelay time.Duration
	// ReconnectJitter adjusts delays from the default reconnect policy. A nil
	// value uses randomized equal jitter.
	ReconnectJitter ReconnectJitter
	// ReconnectPolicy replaces the default exponential reconnect policy.
	ReconnectPolicy ReconnectPolicy
	EventBuffer     int
	// StateBuffer is the number of lifecycle transitions retained for every
	// subscription. When it fills, the oldest non-terminal transition is
	// coalesced so slow state observers cannot block reconnects; callers can
	// detect this through gaps in SubscriptionStateEvent.Sequence.
	StateBuffer  int
	PingInterval time.Duration
	PongWait     time.Duration
	Dialer       Dialer
	Backpressure BackpressurePolicy
	// SubscriptionAckTimeout bounds how long a subscribe or unsubscribe request
	// may remain unacknowledged before the shared connection is re-established.
	SubscriptionAckTimeout time.Duration
	// MaxActiveSubscriptions is the number of distinct logical subscriptions
	// admitted to the shared subscription connection.
	MaxActiveSubscriptions int
	// MaxUniqueUsers is the number of distinct user addresses referenced by
	// active subscriptions on one client.
	MaxUniqueUsers int
	// MaxOutgoingMessagesPerMinute applies independently to the shared
	// subscription socket and reusable POST socket. Waiting honors cancellation.
	MaxOutgoingMessagesPerMinute int
	// MaxConcurrentPosts bounds in-flight WebSocket POST calls. Additional calls
	// wait for admission and honor their context cancellation.
	MaxConcurrentPosts int
}

func (c Config) normalized() Config {
	legacyReconnectDelay := c.ReconnectDelay > 0 && c.ReconnectMaxDelay <= 0 && c.ReconnectJitter == nil
	if c.ReconnectDelay <= 0 {
		c.ReconnectDelay = defaultReconnectDelay
	}
	if c.ReconnectMaxDelay <= 0 {
		c.ReconnectMaxDelay = defaultReconnectMaxDelay
	}
	if c.ReconnectMaxDelay < c.ReconnectDelay {
		c.ReconnectMaxDelay = c.ReconnectDelay
	}
	if reconnectPolicyIsNil(c.ReconnectPolicy) {
		if legacyReconnectDelay {
			c.ReconnectPolicy = ReconnectPolicyFunc(func(int) time.Duration { return c.ReconnectDelay })
		} else {
			c.ReconnectPolicy = NewExponentialReconnectPolicy(c.ReconnectDelay, c.ReconnectMaxDelay, c.ReconnectJitter)
		}
	}
	if c.EventBuffer <= 0 {
		c.EventBuffer = 64
	}
	if c.StateBuffer <= 0 {
		c.StateBuffer = 64
	}
	if c.PingInterval <= 0 {
		c.PingInterval = 15 * time.Second
	}
	if c.PongWait <= 0 {
		c.PongWait = 45 * time.Second
	}
	if c.Dialer == nil {
		c.Dialer = defaultDialer{}
	}
	if c.Backpressure > BackpressureDropOldest {
		c.Backpressure = BackpressureBlock
	}
	if c.SubscriptionAckTimeout <= 0 {
		c.SubscriptionAckTimeout = defaultSubscriptionAcknowledgementWait
	}
	if c.MaxActiveSubscriptions <= 0 {
		c.MaxActiveSubscriptions = DefaultMaxActiveSubscriptions
	}
	if c.MaxUniqueUsers <= 0 {
		c.MaxUniqueUsers = DefaultMaxUniqueUsers
	}
	if c.MaxOutgoingMessagesPerMinute <= 0 {
		c.MaxOutgoingMessagesPerMinute = DefaultMaxOutgoingMessagesPerMinute
	}
	if c.MaxConcurrentPosts <= 0 {
		c.MaxConcurrentPosts = DefaultMaxConcurrentPosts
	}
	return c
}

func reconnectPolicyIsNil(policy ReconnectPolicy) bool {
	if policy == nil {
		return true
	}
	v := reflect.ValueOf(policy)
	switch v.Kind() {
	case reflect.Chan, reflect.Func, reflect.Interface, reflect.Map, reflect.Pointer, reflect.Slice:
		return v.IsNil()
	default:
		return false
	}
}
