package websocket

import (
	"context"
	"testing"
)

func TestBackpressurePolicyLegacyOrdinalsAndNormalization(t *testing.T) {
	if got := uint8(BackpressureDropNewest); got != 1 {
		t.Fatalf("BackpressureDropNewest = %d, want legacy value 1", got)
	}
	if got := uint8(BackpressureDropOldest); got != 2 {
		t.Fatalf("BackpressureDropOldest = %d, want legacy value 2", got)
	}
	if BackpressureBlock == 0 {
		t.Fatal("BackpressureBlock must be an explicit nonzero policy")
	}

	tests := []struct {
		name string
		in   BackpressurePolicy
		want BackpressurePolicy
	}{
		{name: "zero value defaults safely", want: BackpressureDropOldest},
		{name: "explicit block remains configurable", in: BackpressureBlock, want: BackpressureBlock},
		{name: "legacy drop newest remains configurable", in: BackpressurePolicy(1), want: BackpressureDropNewest},
		{name: "legacy drop oldest remains configurable", in: BackpressurePolicy(2), want: BackpressureDropOldest},
		{name: "invalid value defaults safely", in: BackpressurePolicy(255), want: BackpressureDropOldest},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := (Config{Backpressure: test.in}).normalized().Backpressure; got != test.want {
				t.Fatalf("normalized backpressure = %d, want %d", got, test.want)
			}
		})
	}
}

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
