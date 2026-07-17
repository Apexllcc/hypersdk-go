package websocket

import (
	"context"
	"testing"
)

func TestDefaultConfigDropsOldestEventsForSlowConsumers(t *testing.T) {
	config := (Config{EventBuffer: 1}).normalized()
	if config.Backpressure != BackpressureDropOldest {
		t.Fatalf("default backpressure = %v, want BackpressureDropOldest", config.Backpressure)
	}

	events := make(chan int, config.EventBuffer)
	done := make(chan struct{})
	if delivered, closed := deliver(events, 1, config.Backpressure, context.Background(), done); !delivered || closed {
		t.Fatalf("first delivery = (%t, %t)", delivered, closed)
	}
	if delivered, closed := deliver(events, 2, config.Backpressure, context.Background(), done); !delivered || closed {
		t.Fatalf("second delivery = (%t, %t)", delivered, closed)
	}
	if got := <-events; got != 2 {
		t.Fatalf("event = %d, want latest event 2", got)
	}
}

func TestDeliverDropOldestRetainsLatestEvent(t *testing.T) {
	events := make(chan int, 1)
	done := make(chan struct{})
	if delivered, closed := deliver(events, 1, BackpressureDropOldest, context.Background(), done); !delivered || closed {
		t.Fatalf("first delivery=(%t, %t)", delivered, closed)
	}
	if delivered, closed := deliver(events, 2, BackpressureDropOldest, context.Background(), done); !delivered || closed {
		t.Fatalf("second delivery=(%t, %t)", delivered, closed)
	}
	if got := <-events; got != 2 {
		t.Fatalf("event=%d", got)
	}
}

func TestDeliverDropNewestPreservesQueuedEvent(t *testing.T) {
	events := make(chan int, 1)
	done := make(chan struct{})
	_, _ = deliver(events, 1, BackpressureDropNewest, context.Background(), done)
	if delivered, closed := deliver(events, 2, BackpressureDropNewest, context.Background(), done); delivered || closed {
		t.Fatalf("second delivery=(%t, %t)", delivered, closed)
	}
	if got := <-events; got != 1 {
		t.Fatalf("event=%d", got)
	}
}
