// Package nonce provides replaceable nonce generation.
package nonce

import (
	"context"
	"github.com/ethereum/go-ethereum/common"
	"sync"
	"time"
)

// Manager allocates a nonce for a signing address.
type Manager interface {
	Next(context.Context, common.Address) (uint64, error)
}

// MonotonicManager uses max(nowMillis, last+1) per signer address.
type MonotonicManager struct {
	mu   sync.Mutex
	last map[common.Address]uint64
	now  func() time.Time
}

// NewMonotonicManager makes a concurrency-safe local nonce manager.
func NewMonotonicManager(now func() time.Time) *MonotonicManager {
	if now == nil {
		now = time.Now
	}
	return &MonotonicManager{last: make(map[common.Address]uint64), now: now}
}
func (m *MonotonicManager) Next(ctx context.Context, address common.Address) (uint64, error) {
	if err := ctx.Err(); err != nil {
		return 0, err
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	n := uint64(m.now().UnixMilli())
	if last := m.last[address]; n <= last {
		n = last + 1
	}
	m.last[address] = n
	return n, nil
}
