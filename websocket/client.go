// Package websocket implements resilient Hyperliquid WebSocket subscriptions.
package websocket

import (
	"context"
	"sync"
	"sync/atomic"

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
	messages  MessageAdmissionLimiter
	postGate  PostAdmissionGate
	subGate   SubscriptionAdmissionGate
	subOwner  uint64
	subLeases map[string]func()
}

var nextSubscriptionAdmissionOwner atomic.Uint64

func NewClient(url string, configs ...Config) *Client {
	var config Config
	if len(configs) > 0 {
		config = configs[0]
	}
	normalized := config.normalized()
	client := &Client{
		url:       url,
		config:    normalized,
		subs:      make(map[string]managedSubscription),
		handles:   make(map[string]any),
		subGate:   normalized.SubscriptionAdmission,
		subOwner:  nextSubscriptionAdmissionOwner.Add(1),
		subLeases: make(map[string]func()),
	}
	client.closeDone = make(chan struct{})
	client.messages = normalized.MessageAdmission
	client.postGate = normalized.PostAdmission
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
	// Blocking delivery selects on each subscription's done channel. Close the
	// logical handles before joining the manager so a full event buffer cannot
	// leave the read loop and Client.Close waiting on each other.
	for _, s := range subs {
		_ = s.Close()
	}
	if manager != nil {
		manager.close()
	}
	if posts != nil {
		posts.close()
	}
	c.mu.Lock()
	close(done)
	c.mu.Unlock()
	return nil
}
func (c *Client) remove(key string, s managedSubscription) {
	identity := serverSubscriptionIdentity(s.subscriptionWire().Subscription)
	removed := false
	present := false
	fingerprint := ""
	var release func()
	c.manager.commitMu.Lock()
	c.mu.Lock()
	if c.subs[key] == s {
		delete(c.subs, key)
		delete(c.handles, key)
		removed = true
		present, fingerprint = c.subscriptionIdentityStateLocked(identity)
		if !present {
			release = c.subLeases[identity]
			delete(c.subLeases, identity)
		}
	}
	c.mu.Unlock()
	if removed {
		c.manager.registryChangedLocked(identity, present, fingerprint)
	} else {
		c.manager.notify()
	}
	c.manager.commitMu.Unlock()
	if release != nil {
		release()
	}
}

func (c *Client) subscriptionIdentityState(identity string) (bool, string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.subscriptionIdentityStateLocked(identity)
}

func (c *Client) subscriptionIdentityStateLocked(identity string) (bool, string) {
	count := 0
	var representative subscriptionWire
	for _, subscription := range c.subs {
		if serverSubscriptionIdentity(subscription.subscriptionWire().Subscription) != identity {
			continue
		}
		count++
		representative = subscription.subscriptionWire()
	}
	if count == 0 {
		return false, ""
	}
	if count > 1 {
		representative = canonicalSubscriptionWire(representative)
	}
	return true, subscriptionWireFingerprint(representative)
}

func (c *Client) detachSubscriptionIdentity(identity string) []managedSubscription {
	c.manager.commitMu.Lock()
	c.mu.Lock()
	detached := make([]managedSubscription, 0)
	for key, subscription := range c.subs {
		if serverSubscriptionIdentity(subscription.subscriptionWire().Subscription) != identity {
			continue
		}
		detached = append(detached, subscription)
		delete(c.subs, key)
		delete(c.handles, key)
	}
	release := c.subLeases[identity]
	delete(c.subLeases, identity)
	c.mu.Unlock()
	if len(detached) > 0 {
		c.manager.registryChangedLocked(identity, false, "")
	}
	c.manager.commitMu.Unlock()
	if release != nil {
		release()
	}
	return detached
}

func (c *Client) dial(ctx context.Context) (*websocket.Conn, error) {
	return c.config.Dialer.DialContext(ctx, c.url)
}
