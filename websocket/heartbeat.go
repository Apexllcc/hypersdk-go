package websocket

import (
	"context"
	"sync"
	"time"
)

func startHeartbeat(parent context.Context, writeJSON func(context.Context, any) error, config Config) (func(), <-chan error) {
	ctx, cancel := context.WithCancel(parent)
	stopped := make(chan struct{})
	errors := make(chan error, 1)
	go func() {
		defer close(stopped)
		ticker := time.NewTicker(config.PingInterval)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				if err := writeJSON(ctx, struct {
					Method string `json:"method"`
				}{Method: "ping"}); err != nil {
					if ctx.Err() != nil {
						return
					}
					select {
					case errors <- err:
					default:
					}
					return
				}
			}
		}
	}()
	var once sync.Once
	return func() {
		once.Do(cancel)
		<-stopped
	}, errors
}
