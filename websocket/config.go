package websocket

import (
	"context"
	"time"

	"github.com/gorilla/websocket"
)

// Dialer opens WebSocket connections. It provides a seam for custom network
// transports and deterministic tests.
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
	ReconnectDelay time.Duration
	EventBuffer    int
	// StateBuffer is the number of lifecycle transitions retained for every
	// subscription. When it fills, the oldest non-terminal transition is
	// coalesced so slow state observers cannot block reconnects; callers can
	// detect this through gaps in SubscriptionStateEvent.Sequence.
	StateBuffer  int
	PingInterval time.Duration
	PongWait     time.Duration
	Dialer       Dialer
	Backpressure BackpressurePolicy
}

func (c Config) normalized() Config {
	if c.ReconnectDelay <= 0 {
		c.ReconnectDelay = time.Second
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
	return c
}
