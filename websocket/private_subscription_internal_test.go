package websocket

import (
	"context"
	"testing"
)

func TestCachePrivateHandleRejectsSubscriptionClosedBeforeCaching(t *testing.T) {
	client := NewClient("ws://unused")
	defer func() { _ = client.Close() }()
	const key = "userFundings:0xabc"
	subscription := newStreamSubscription(context.Background(), client, key, "userFundings", newSubscriptionWire("userFundings", map[string]any{"user": "0xabc"}), decodeJSON[UserFundingsEvent], func(UserFundingsEvent) bool { return true })
	client.mu.Lock()
	client.subs[key] = subscription
	client.mu.Unlock()

	// This models cancellation/Client.Close after registration but before the
	// public subscribe method caches its typed handle.
	if err := subscription.Close(); err != nil {
		t.Fatal(err)
	}
	handle, current := client.cachePrivateHandle(key, subscription, func() any { return "stale" })
	if current || handle != nil {
		t.Fatalf("closed subscription was cached: handle=%v current=%t", handle, current)
	}
	client.mu.Lock()
	defer client.mu.Unlock()
	if client.subs[key] != nil || client.handles[key] != nil {
		t.Fatalf("closed subscription leaked registry state: subs=%v handles=%v", client.subs[key], client.handles[key])
	}
}
