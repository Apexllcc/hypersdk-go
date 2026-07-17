package websocket

import (
	"context"
	"encoding/json"
	"errors"
	"sync"

	"github.com/Apexllcc/hyperliquid-go-sdk/internal/validation"
)

// Subscription is the common lifecycle surface for all future stream types.
type Subscription interface {
	Errors() <-chan error
	Close() error
}

// StatefulSubscription is a Subscription that exposes connection lifecycle
// transitions. All subscriptions created by Client implement it; the separate
// interface preserves compatibility for callers with their own Subscription
// implementations.
type StatefulSubscription interface {
	Subscription
	// States reports connection and server-acknowledged subscription lifecycle transitions.
	States() <-chan SubscriptionStateEvent
}

// L2BookSubscription delivers L2 book events and reconnect errors.
type L2BookSubscription struct {
	events     chan L2BookEvent
	errors     chan error
	states     chan SubscriptionStateEvent
	done       chan struct{}
	once       sync.Once
	deliveryMu sync.Mutex
	lastState  SubscriptionState
	stateSeq   uint64
	client     *Client
	key        string
	request    L2BookRequest
	ctx        context.Context
}

func (c *Client) SubscribeL2Book(ctx context.Context, request L2BookRequest) (*L2BookSubscription, error) {
	if request.Coin == "" {
		return nil, errors.New("coin is required")
	}
	if err := validation.L2BookAggregation(request.NSigFigs, request.Mantissa); err != nil {
		return nil, err
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	c.mu.Lock()
	if c.closed {
		c.mu.Unlock()
		return nil, ErrWebSocketClosed
	}
	key := l2BookKey(request)
	if existing := c.subs[key]; existing != nil {
		subscription, ok := existing.(*L2BookSubscription)
		c.mu.Unlock()
		if !ok {
			return nil, errors.New("websocket subscription registry type conflict")
		}
		return subscription, nil
	}
	if err := c.admitSubscription(newL2SubscriptionWire(request)); err != nil {
		c.mu.Unlock()
		return nil, err
	}
	s := &L2BookSubscription{events: make(chan L2BookEvent, c.config.EventBuffer), errors: make(chan error, 1), states: make(chan SubscriptionStateEvent, c.config.StateBuffer), done: make(chan struct{}), client: c, key: key, request: request, ctx: ctx}
	c.subs[key] = s
	c.mu.Unlock()
	s.stateChange(SubscriptionStateConnecting, nil)
	c.manager.notify()
	go func() {
		select {
		case <-ctx.Done():
			_ = s.Close()
		case <-s.done:
		}
	}()
	return s, nil
}
func (s *L2BookSubscription) Events() <-chan L2BookEvent { return s.events }
func (s *L2BookSubscription) Errors() <-chan error       { return s.errors }
func (s *L2BookSubscription) States() <-chan SubscriptionStateEvent {
	return s.states
}
func (s *L2BookSubscription) Close() error {
	s.once.Do(func() {
		close(s.done)
		s.client.remove(s.key, s)
		s.deliveryMu.Lock()
		s.stateSeq++
		enqueueSubscriptionState(s.states, SubscriptionStateEvent{Sequence: s.stateSeq, State: SubscriptionStateUnsubscribed})
		close(s.events)
		close(s.errors)
		close(s.states)
		s.deliveryMu.Unlock()
	})
	return nil
}

func (s *L2BookSubscription) subscriptionKey() string { return s.key }
func (s *L2BookSubscription) subscriptionWire() subscriptionWire {
	return newL2SubscriptionWire(s.request)
}
func (s *L2BookSubscription) subscriptionChannel() string { return "l2Book" }
func (s *L2BookSubscription) isDone() bool {
	select {
	case <-s.done:
		return true
	default:
		return false
	}
}
func (s *L2BookSubscription) deliverRaw(data json.RawMessage) {
	s.deliveryMu.Lock()
	defer s.deliveryMu.Unlock()
	if s.isDone() || stopped(s.ctx, s.done) {
		return
	}
	var event L2BookEvent
	if err := json.Unmarshal(data, &event); err != nil {
		s.reportLocked(err)
		return
	}
	if event.Coin != s.request.Coin {
		return
	}
	delivered, closed := deliver(s.events, event, s.client.config.Backpressure, s.ctx, s.done)
	if !delivered && !closed {
		s.reportLocked(ErrEventDropped)
	}
}
func (s *L2BookSubscription) report(err error) {
	s.deliveryMu.Lock()
	defer s.deliveryMu.Unlock()
	s.reportLocked(err)
}
func (s *L2BookSubscription) reportLocked(err error) {
	if s.isDone() {
		return
	}
	select {
	case s.errors <- err:
	default:
	}
}

func (s *L2BookSubscription) stateChange(state SubscriptionState, err error) {
	s.deliveryMu.Lock()
	defer s.deliveryMu.Unlock()
	if s.isDone() {
		return
	}
	if state != SubscriptionStateError && s.lastState == state {
		return
	}
	if state == SubscriptionStateError {
		err = subscriptionStateError(err)
	} else {
		err = nil
	}
	s.stateSeq++
	enqueueSubscriptionState(s.states, SubscriptionStateEvent{Sequence: s.stateSeq, State: state, Error: err})
	s.lastState = state
}
