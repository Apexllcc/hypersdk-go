package websocket

import (
	"testing"
	"time"
)

func TestDefaultWebSocketAdmissionLimits(t *testing.T) {
	config := (Config{}).normalized()
	if config.MaxActiveSubscriptions != 1000 {
		t.Fatalf("MaxActiveSubscriptions = %d", config.MaxActiveSubscriptions)
	}
	if config.MaxUniqueUsers != 10 {
		t.Fatalf("MaxUniqueUsers = %d", config.MaxUniqueUsers)
	}
	if config.MaxOutgoingMessagesPerMinute != 2000 {
		t.Fatalf("MaxOutgoingMessagesPerMinute = %d", config.MaxOutgoingMessagesPerMinute)
	}
	if config.MaxConcurrentPosts != 100 {
		t.Fatalf("MaxConcurrentPosts = %d", config.MaxConcurrentPosts)
	}
	if config.SubscriptionAckTimeout != 10*time.Second {
		t.Fatalf("SubscriptionAckTimeout = %s", config.SubscriptionAckTimeout)
	}
}
