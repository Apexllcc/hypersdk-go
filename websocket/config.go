package websocket

import "time"

// Config limits reconnect behavior and in-memory delivery queues.
type Config struct {
	ReconnectDelay time.Duration
	EventBuffer    int
	PingInterval   time.Duration
	PongWait       time.Duration
}

func (c Config) normalized() Config {
	if c.ReconnectDelay <= 0 {
		c.ReconnectDelay = time.Second
	}
	if c.EventBuffer <= 0 {
		c.EventBuffer = 64
	}
	if c.PingInterval <= 0 {
		c.PingInterval = 15 * time.Second
	}
	if c.PongWait <= 0 {
		c.PongWait = 45 * time.Second
	}
	return c
}
