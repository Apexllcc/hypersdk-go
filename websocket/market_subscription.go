package websocket

import (
	"context"
	"encoding/json"
	"errors"
	"sync"
)

type subscriptionWire struct {
	Method       string         `json:"method"`
	Subscription map[string]any `json:"subscription"`
}

func newSubscriptionWire(kind string, fields map[string]any) subscriptionWire {
	fields["type"] = kind
	return subscriptionWire{Method: "subscribe", Subscription: fields}
}

type streamSubscription[T any] struct {
	events     chan T
	errors     chan error
	done       chan struct{}
	once       sync.Once
	deliveryMu sync.Mutex
	client     *Client
	key        string
	channel    string
	wire       subscriptionWire
	decode     func(json.RawMessage) (T, error)
	match      func(T) bool
	ctx        context.Context
}

func newStreamSubscription[T any](ctx context.Context, client *Client, key, channel string, wire subscriptionWire, decode func(json.RawMessage) (T, error), match func(T) bool) *streamSubscription[T] {
	return &streamSubscription[T]{
		events: make(chan T, client.config.EventBuffer), errors: make(chan error, 1), done: make(chan struct{}),
		client: client, key: key, channel: channel, wire: wire, decode: decode, match: match, ctx: ctx,
	}
}

func (s *streamSubscription[T]) Events() <-chan T     { return s.events }
func (s *streamSubscription[T]) Errors() <-chan error { return s.errors }
func (s *streamSubscription[T]) Close() error {
	s.once.Do(func() {
		close(s.done)
		s.client.remove(s.key, s)
		s.deliveryMu.Lock()
		close(s.events)
		close(s.errors)
		s.deliveryMu.Unlock()
	})
	return nil
}

func (s *streamSubscription[T]) subscriptionKey() string            { return s.key }
func (s *streamSubscription[T]) subscriptionWire() subscriptionWire { return s.wire }
func (s *streamSubscription[T]) subscriptionChannel() string        { return s.channel }
func (s *streamSubscription[T]) isDone() bool {
	select {
	case <-s.done:
		return true
	default:
		return false
	}
}

func (s *streamSubscription[T]) deliverRaw(data json.RawMessage) {
	s.deliveryMu.Lock()
	defer s.deliveryMu.Unlock()
	if s.isDone() || stopped(s.ctx, s.done) {
		return
	}
	event, err := s.decode(data)
	if err != nil {
		s.reportLocked(err)
		return
	}
	if !s.match(event) {
		return
	}
	delivered, closed := deliver(s.events, event, s.client.config.Backpressure, s.ctx, s.done)
	if !delivered && !closed {
		s.reportLocked(ErrEventDropped)
	}
	_ = closed
}

func stopped(ctx context.Context, done <-chan struct{}) bool {
	select {
	case <-ctx.Done():
		return true
	case <-done:
		return true
	default:
		return false
	}
}

func (s *streamSubscription[T]) report(err error) {
	s.deliveryMu.Lock()
	defer s.deliveryMu.Unlock()
	s.reportLocked(err)
}

func (s *streamSubscription[T]) reportLocked(err error) {
	if s.isDone() {
		return
	}
	select {
	case s.errors <- err:
	default:
	}
}

func deliver[T any](events chan T, event T, policy BackpressurePolicy, ctx context.Context, done <-chan struct{}) (delivered, closed bool) {
	if stopped(ctx, done) {
		return false, true
	}
	switch policy {
	case BackpressureDropNewest:
		select {
		case events <- event:
			return true, false
		default:
			return false, false
		}
	case BackpressureDropOldest:
		select {
		case events <- event:
			return true, false
		default:
		}
		select {
		case <-events:
		default:
		}
		select {
		case events <- event:
			return true, false
		default:
			return false, false
		}
	default:
		select {
		case events <- event:
			return true, false
		case <-ctx.Done():
			return false, true
		case <-done:
			return false, true
		}
	}
}

func subscribeStream[T any](ctx context.Context, client *Client, key, channel string, wire subscriptionWire, decode func(json.RawMessage) (T, error), match func(T) bool, validate func(map[string]managedSubscription) error) (*streamSubscription[T], error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	client.mu.Lock()
	defer client.mu.Unlock()
	if client.closed {
		return nil, ErrWebSocketClosed
	}
	if validate != nil {
		if err := validate(client.subs); err != nil {
			return nil, err
		}
	}
	if existing := client.subs[key]; existing != nil {
		subscription, ok := existing.(*streamSubscription[T])
		if !ok {
			return nil, errors.New("websocket subscription registry type conflict")
		}
		return subscription, nil
	}
	subscription := newStreamSubscription(ctx, client, key, channel, wire, decode, match)
	client.subs[key] = subscription
	client.manager.notify()
	go func() {
		select {
		case <-ctx.Done():
			_ = subscription.Close()
		case <-subscription.done:
		}
	}()
	return subscription, nil
}

// AllMidsSubscription streams all mids.
type AllMidsSubscription struct {
	*streamSubscription[AllMidsEvent]
}

// TradesSubscription streams batches of trades.
type TradesSubscription struct {
	*streamSubscription[[]TradeEvent]
}

// CandleSubscription streams candles.
type CandleSubscription struct {
	*streamSubscription[[]CandleEvent]
}

// BBOSubscription streams best bid and offer updates.
type BBOSubscription struct{ *streamSubscription[BBOEvent] }

func (c *Client) SubscribeAllMids(ctx context.Context, request AllMidsRequest) (*AllMidsSubscription, error) {
	wire := newSubscriptionWire("allMids", map[string]any{})
	if request.DEX != "" {
		wire.Subscription["dex"] = request.DEX
	}
	key := allMidsKey(request)
	subscription, err := subscribeStream(ctx, c, key, "allMids", wire, decodeJSON[AllMidsEvent], func(AllMidsEvent) bool { return true }, func(subscriptions map[string]managedSubscription) error {
		for otherKey, other := range subscriptions {
			if other.subscriptionChannel() == "allMids" && otherKey != key {
				return ErrAmbiguousAllMids
			}
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	c.mu.Lock()
	if c.subs[key] != subscription || subscription.isDone() {
		c.mu.Unlock()
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		return c.SubscribeAllMids(ctx, request)
	}
	if handle := c.handles[key]; handle != nil {
		if typed := handle.(*AllMidsSubscription); typed.streamSubscription == subscription {
			c.mu.Unlock()
			return typed, nil
		}
		delete(c.handles, key)
	}
	handle := &AllMidsSubscription{subscription}
	c.handles[key] = handle
	c.mu.Unlock()
	return handle, nil
}

func (c *Client) SubscribeTrades(ctx context.Context, request TradesRequest) (*TradesSubscription, error) {
	if request.Coin == "" {
		return nil, errors.New("coin is required")
	}
	wire := newSubscriptionWire("trades", map[string]any{"coin": request.Coin})
	subscription, err := subscribeStream(ctx, c, tradesKey(request), "trades", wire, decodeJSON[[]TradeEvent], func(events []TradeEvent) bool { return len(events) > 0 && events[0].Coin == request.Coin }, nil)
	if err != nil {
		return nil, err
	}
	key := tradesKey(request)
	c.mu.Lock()
	if c.subs[key] != subscription || subscription.isDone() {
		c.mu.Unlock()
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		return c.SubscribeTrades(ctx, request)
	}
	if handle := c.handles[key]; handle != nil {
		if typed := handle.(*TradesSubscription); typed.streamSubscription == subscription {
			c.mu.Unlock()
			return typed, nil
		}
		delete(c.handles, key)
	}
	handle := &TradesSubscription{subscription}
	c.handles[key] = handle
	c.mu.Unlock()
	return handle, nil
}

func (c *Client) SubscribeCandle(ctx context.Context, request CandleRequest) (*CandleSubscription, error) {
	if request.Coin == "" {
		return nil, errors.New("coin is required")
	}
	if request.Interval == "" {
		return nil, errors.New("interval is required")
	}
	wire := newSubscriptionWire("candle", map[string]any{"coin": request.Coin, "interval": request.Interval})
	subscription, err := subscribeStream(ctx, c, candleKey(request), "candle", wire, decodeJSON[[]CandleEvent], func(events []CandleEvent) bool {
		return len(events) > 0 && events[0].Coin == request.Coin && events[0].Interval == request.Interval
	}, nil)
	if err != nil {
		return nil, err
	}
	key := candleKey(request)
	c.mu.Lock()
	if c.subs[key] != subscription || subscription.isDone() {
		c.mu.Unlock()
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		return c.SubscribeCandle(ctx, request)
	}
	if handle := c.handles[key]; handle != nil {
		if typed := handle.(*CandleSubscription); typed.streamSubscription == subscription {
			c.mu.Unlock()
			return typed, nil
		}
		delete(c.handles, key)
	}
	handle := &CandleSubscription{subscription}
	c.handles[key] = handle
	c.mu.Unlock()
	return handle, nil
}

func (c *Client) SubscribeBBO(ctx context.Context, request BBORequest) (*BBOSubscription, error) {
	if request.Coin == "" {
		return nil, errors.New("coin is required")
	}
	wire := newSubscriptionWire("bbo", map[string]any{"coin": request.Coin})
	subscription, err := subscribeStream(ctx, c, bboKey(request), "bbo", wire, decodeJSON[BBOEvent], func(event BBOEvent) bool { return event.Coin == request.Coin }, nil)
	if err != nil {
		return nil, err
	}
	key := bboKey(request)
	c.mu.Lock()
	if c.subs[key] != subscription || subscription.isDone() {
		c.mu.Unlock()
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		return c.SubscribeBBO(ctx, request)
	}
	if handle := c.handles[key]; handle != nil {
		if typed := handle.(*BBOSubscription); typed.streamSubscription == subscription {
			c.mu.Unlock()
			return typed, nil
		}
		delete(c.handles, key)
	}
	handle := &BBOSubscription{subscription}
	c.handles[key] = handle
	c.mu.Unlock()
	return handle, nil
}

func decodeJSON[T any](data json.RawMessage) (T, error) {
	var value T
	err := json.Unmarshal(data, &value)
	return value, err
}
