// Package websocket implements resilient Hyperliquid WebSocket subscriptions.
package websocket

import (
	"context"
	"sync"

	"github.com/gorilla/websocket"
)

// Client owns a registry of subscriptions and closes each exactly once.
type Client struct {
	url     string
	config  Config
	mu      sync.Mutex
	closed  bool
	subs    map[string]managedSubscription
	handles map[string]any
	manager *connectionManager
}

func NewClient(url string, configs ...Config) *Client {
	var config Config
	if len(configs) > 0 {
		config = configs[0]
	}
	client := &Client{url: url, config: config.normalized(), subs: make(map[string]managedSubscription), handles: make(map[string]any)}
	client.manager = newConnectionManager(client)
	return client
}

// Close is idempotent and stops every active subscription.
func (c *Client) Close() error {
	c.mu.Lock()
	if c.closed {
		c.mu.Unlock()
		return nil
	}
	c.closed = true
	subs := make([]managedSubscription, 0, len(c.subs))
	for _, s := range c.subs {
		subs = append(subs, s)
	}
	c.mu.Unlock()
	c.manager.close()
	for _, s := range subs {
		_ = s.Close()
	}
	return nil
}
func (c *Client) remove(key string, s managedSubscription) {
	c.mu.Lock()
	if c.subs[key] == s {
		delete(c.subs, key)
		delete(c.handles, key)
	}
	c.mu.Unlock()
	c.manager.notify()
}

func (c *Client) dial(ctx context.Context) (*websocket.Conn, error) {
	return c.config.Dialer.DialContext(ctx, c.url)
}
