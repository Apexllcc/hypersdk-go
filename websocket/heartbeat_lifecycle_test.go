package websocket

import (
	"context"
	"sync/atomic"
	"testing"
	"time"
)

func TestHeartbeatRepeatedStartStopJoinsEveryAdmissionWait(t *testing.T) {
	const generations = 50
	var active atomic.Int32
	for generation := range generations {
		entered := make(chan struct{})
		exited := make(chan struct{})
		stop, _ := startHeartbeat(context.Background(), func(ctx context.Context, _ any) error {
			active.Add(1)
			close(entered)
			<-ctx.Done()
			active.Add(-1)
			close(exited)
			return ctx.Err()
		}, Config{PingInterval: time.Microsecond})
		select {
		case <-entered:
		case <-time.After(time.Second):
			t.Fatalf("generation %d never entered admission wait", generation)
		}
		stop()
		select {
		case <-exited:
		default:
			t.Fatalf("generation %d stop returned before admission wait exited", generation)
		}
	}
	if got := active.Load(); got != 0 {
		t.Fatalf("active heartbeat waits = %d, want 0", got)
	}
}
