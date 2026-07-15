package websocket

import (
	"context"
	"time"
)

func waitForReconnect(ctx context.Context, done <-chan struct{}, delay time.Duration) bool {
	timer := time.NewTimer(delay)
	defer timer.Stop()
	select {
	case <-timer.C:
		return true
	case <-ctx.Done():
		return false
	case <-done:
		return false
	}
}
