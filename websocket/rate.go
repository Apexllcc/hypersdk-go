package websocket

import (
	"context"
	"sync"
	"time"
)

type messageRateLimiter struct {
	mu     sync.Mutex
	limit  int
	window time.Duration
	sent   []time.Time
}

func newMessageRateLimiter(limit int, window time.Duration) *messageRateLimiter {
	return &messageRateLimiter{limit: limit, window: window, sent: make([]time.Time, 0, limit)}
}

func (l *messageRateLimiter) wait(ctx context.Context, stopped <-chan struct{}) error {
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
		case <-stopped:
			if !timer.Stop() {
				<-timer.C
			}
			return ErrWebSocketClosed
		case <-timer.C:
		}
	}
}
