package websocket

import (
	"context"
	"sync"
)

// PostAdmissionGate atomically reserves one in-flight WebSocket POST slot.
// Implementations must be concurrency-safe across Clients, honor ctx, and
// return an idempotent release function for every successful acquisition.
type PostAdmissionGate interface {
	Acquire(ctx context.Context) (release func(), err error)
}

type semaphorePostAdmissionGate struct {
	permits chan struct{}
}

// NewPostAdmissionGate returns a shareable concurrent POST admission boundary.
func NewPostAdmissionGate(limit int) PostAdmissionGate {
	if limit <= 0 {
		limit = DefaultMaxConcurrentPosts
	}
	return &semaphorePostAdmissionGate{permits: make(chan struct{}, limit)}
}

func (g *semaphorePostAdmissionGate) Acquire(ctx context.Context) (func(), error) {
	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	case g.permits <- struct{}{}:
	}
	var once sync.Once
	return func() { once.Do(func() { <-g.permits }) }, nil
}
