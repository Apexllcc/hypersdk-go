// Package websocket implements resilient Hyperliquid WebSocket subscriptions.
package websocket

import "sync"

// Client owns a registry of subscriptions and closes each exactly once.
type Client struct {
	url    string
	config Config
	mu     sync.Mutex
	closed bool
	subs   map[string]*L2BookSubscription
}

func NewClient(url string, configs ...Config) *Client {
	var config Config
	if len(configs) > 0 {
		config = configs[0]
	}
	return &Client{url: url, config: config.normalized(), subs: make(map[string]*L2BookSubscription)}
}

// Close is idempotent and stops every active subscription.
func (c *Client) Close() error {
	c.mu.Lock()
	if c.closed {
		c.mu.Unlock()
		return nil
	}
	c.closed = true
	subs := make([]*L2BookSubscription, 0, len(c.subs))
	for _, s := range c.subs {
		subs = append(subs, s)
	}
	c.mu.Unlock()
	for _, s := range subs {
		_ = s.Close()
	}
	return nil
}
func (c *Client) remove(key string, s *L2BookSubscription) {
	c.mu.Lock()
	if c.subs[key] == s {
		delete(c.subs, key)
	}
	c.mu.Unlock()
}
