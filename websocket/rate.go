package websocket

import (
	"context"
	"sync"
	"time"
)

// MessageAdmissionLimiter atomically reserves one outbound WebSocket message.
// Implementations must be safe for concurrent use by multiple Clients and
// must return promptly when ctx is canceled.
type MessageAdmissionLimiter interface {
	Wait(ctx context.Context) error
}

type messageRateLimiter struct {
	mu     sync.Mutex
	limit  int
	window time.Duration
	sent   []time.Time
}

func newMessageRateLimiter(limit int, window time.Duration) *messageRateLimiter {
	return &messageRateLimiter{limit: limit, window: window, sent: make([]time.Time, 0, limit)}
}

// NewMessageAdmissionLimiter returns a concurrency-safe rolling-window
// admission boundary. Sharing the returned value across Clients enforces one
// combined budget across all of their WebSocket connections.
func NewMessageAdmissionLimiter(limit int, window time.Duration) MessageAdmissionLimiter {
	if limit <= 0 {
		limit = DefaultMaxOutgoingMessagesPerMinute
	}
	if window <= 0 {
		window = time.Minute
	}
	return newMessageRateLimiter(limit, window)
}

func (l *messageRateLimiter) Wait(ctx context.Context) error {
	for {
		if err := ctx.Err(); err != nil {
			return err
		}
		l.mu.Lock()
		now := time.Now()
		cutoff := now.Add(-l.window)
		first := 0
		for first < len(l.sent) && !l.sent[first].After(cutoff) {
			first++
		}
		if first > 0 {
			copy(l.sent, l.sent[first:])
			l.sent = l.sent[:len(l.sent)-first]
		}
		if len(l.sent) < l.limit {
			l.sent = append(l.sent, now)
			l.mu.Unlock()
			return nil
		}
		wait := time.Until(l.sent[0].Add(l.window))
		l.mu.Unlock()
		timer := time.NewTimer(wait)
		select {
		case <-ctx.Done():
			if !timer.Stop() {
				<-timer.C
			}
			return ctx.Err()
		case <-timer.C:
		}
	}
}
