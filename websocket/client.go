// Package websocket implements resilient Hyperliquid WebSocket subscriptions.
package websocket

import (
	"context"
	"sync"
	"time"

	"github.com/gorilla/websocket"
)

// Client owns a registry of subscriptions and closes each exactly once.
type Client struct {
	url       string
	config    Config
	mu        sync.Mutex
	closed    bool
	closeDone chan struct{}
	subs      map[string]managedSubscription
	handles   map[string]any
	manager   *connectionManager
	posts     *postManager
	subRate   *messageRateLimiter
	postRate  *messageRateLimiter
}

func NewClient(url string, configs ...Config) *Client {
	var config Config
	if len(configs) > 0 {
		config = configs[0]
	}
	normalized := config.normalized()
	client := &Client{url: url, config: normalized, subs: make(map[string]managedSubscription), handles: make(map[string]any)}
	client.closeDone = make(chan struct{})
	client.subRate = newMessageRateLimiter(normalized.MaxOutgoingMessagesPerMinute, time.Minute)
	client.postRate = newMessageRateLimiter(normalized.MaxOutgoingMessagesPerMinute, time.Minute)
	client.manager = newConnectionManager(client)
	client.posts = newPostManager(client)
	return client
}

// Close is idempotent and stops every active subscription.
func (c *Client) Close() error {
	if c == nil {
		return nil
	}
	c.mu.Lock()
	if c.closed {
		done := c.closeDone
		c.mu.Unlock()
		<-done
		return nil
	}
	c.closed = true
	subs := make([]managedSubscription, 0, len(c.subs))
	for _, s := range c.subs {
		subs = append(subs, s)
	}
	if c.closeDone == nil {
		c.closeDone = make(chan struct{})
	}
	done := c.closeDone
	manager, posts := c.manager, c.posts
	c.mu.Unlock()
	if manager != nil {
		manager.close()
	}
	if posts != nil {
		posts.close()
	}
	for _, s := range subs {
		_ = s.Close()
	}
	c.mu.Lock()
	close(done)
	c.mu.Unlock()
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
