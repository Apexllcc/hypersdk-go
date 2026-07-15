package websocket

import (
	"context"
	"errors"
	"fmt"
	"strings"
)

// UserFillsRequest identifies one user's fill stream. AggregateByTime combines
// partial fills from a single crossing order, as defined by Hyperliquid.
type UserFillsRequest struct {
	User            string `json:"user"`
	AggregateByTime bool   `json:"aggregateByTime,omitempty"`
}

// UserEventsSubscription streams fills, funding, liquidations, and external
// order cancellations for one user. Hyperliquid's "user" channel omits the
// address, so only one such subscription can safely share a client.
type UserEventsSubscription struct{ *streamSubscription[UserEvent] }

// OrderUpdatesSubscription streams status changes for one user. The channel
// payload also omits user, so it cannot be multiplexed across users.
type OrderUpdatesSubscription struct {
	*streamSubscription[[]OrderUpdate]
}

// UserFillsSubscription streams snapshot and incremental user fill batches.
type UserFillsSubscription struct {
	*streamSubscription[UserFillsEvent]
}

// UserFundingsSubscription streams snapshot and incremental funding batches.
type UserFundingsSubscription struct {
	*streamSubscription[UserFundingsEvent]
}

// UserLedgerSubscription streams deposits, withdrawals, transfers, and other
// non-funding ledger updates.
type UserLedgerSubscription struct {
	*streamSubscription[UserLedgerEvent]
}

func requireUser(user string) error {
	if strings.TrimSpace(user) == "" {
		return errors.New("user is required")
	}
	return nil
}

func onePerChannel(channel string) func(map[string]managedSubscription) error {
	return func(subscriptions map[string]managedSubscription) error {
		for _, subscription := range subscriptions {
			if subscription.subscriptionChannel() == channel {
				return fmt.Errorf("%s cannot be multiplexed because its messages omit user", channel)
			}
		}
		return nil
	}
}

func (c *Client) cachePrivateHandle(key string, subscription managedSubscription, makeHandle func() any) (any, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.subs[key] != subscription || subscription.isDone() {
		return nil, false
	}
	if handle := c.handles[key]; handle != nil {
		return handle, true
	}
	handle := makeHandle()
	c.handles[key] = handle
	return handle, true
}

func aggregationMode(subscription *streamSubscription[UserFillsEvent]) bool {
	aggregate, _ := subscription.subscriptionWire().Subscription["aggregateByTime"].(bool)
	return aggregate
}

// SubscribeUserEvents subscribes to the private userEvents feed.
func (c *Client) SubscribeUserEvents(ctx context.Context, user string) (*UserEventsSubscription, error) {
	if err := requireUser(user); err != nil {
		return nil, err
	}
	key := "userEvents:" + strings.ToLower(user)
	subscription, err := subscribeStream(ctx, c, key, "user", newSubscriptionWire("userEvents", map[string]any{"user": user}), decodeJSON[UserEvent], func(UserEvent) bool { return true }, onePerChannel("user"))
	if err != nil {
		return nil, err
	}
	handle, current := c.cachePrivateHandle(key, subscription, func() any { return &UserEventsSubscription{subscription} })
	if !current {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		return c.SubscribeUserEvents(ctx, user)
	}
	typed, ok := handle.(*UserEventsSubscription)
	if !ok {
		return nil, errors.New("websocket subscription registry type conflict")
	}
	return typed, nil
}

// SubscribeOrderUpdates subscribes to private order status changes.
func (c *Client) SubscribeOrderUpdates(ctx context.Context, user string) (*OrderUpdatesSubscription, error) {
	if err := requireUser(user); err != nil {
		return nil, err
	}
	key := "orderUpdates:" + strings.ToLower(user)
	subscription, err := subscribeStream(ctx, c, key, "orderUpdates", newSubscriptionWire("orderUpdates", map[string]any{"user": user}), decodeJSON[[]OrderUpdate], func([]OrderUpdate) bool { return true }, onePerChannel("orderUpdates"))
	if err != nil {
		return nil, err
	}
	handle, current := c.cachePrivateHandle(key, subscription, func() any { return &OrderUpdatesSubscription{subscription} })
	if !current {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		return c.SubscribeOrderUpdates(ctx, user)
	}
	typed, ok := handle.(*OrderUpdatesSubscription)
	if !ok {
		return nil, errors.New("websocket subscription registry type conflict")
	}
	return typed, nil
}

// SubscribeUserFills subscribes to a user's fill snapshots and updates.
func (c *Client) SubscribeUserFills(ctx context.Context, request UserFillsRequest) (*UserFillsSubscription, error) {
	if err := requireUser(request.User); err != nil {
		return nil, err
	}
	key := "userFills:" + strings.ToLower(request.User)
	wire := newSubscriptionWire("userFills", map[string]any{"user": request.User})
	if request.AggregateByTime {
		wire.Subscription["aggregateByTime"] = true
	}
	subscription, err := subscribeStream(ctx, c, key, "userFills", wire, decodeJSON[UserFillsEvent], func(event UserFillsEvent) bool { return strings.EqualFold(event.User, request.User) }, nil)
	if err != nil {
		return nil, err
	}
	if aggregationMode(subscription) != request.AggregateByTime {
		return nil, ErrConflictingUserFillsSubscription
	}
	handle, current := c.cachePrivateHandle(key, subscription, func() any { return &UserFillsSubscription{subscription} })
	if !current {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		return c.SubscribeUserFills(ctx, request)
	}
	typed, ok := handle.(*UserFillsSubscription)
	if !ok {
		return nil, errors.New("websocket subscription registry type conflict")
	}
	return typed, nil
}

// SubscribeUserFundings subscribes to a user's funding snapshots and updates.
func (c *Client) SubscribeUserFundings(ctx context.Context, user string) (*UserFundingsSubscription, error) {
	if err := requireUser(user); err != nil {
		return nil, err
	}
	key := "userFundings:" + strings.ToLower(user)
	subscription, err := subscribeStream(ctx, c, key, "userFundings", newSubscriptionWire("userFundings", map[string]any{"user": user}), decodeJSON[UserFundingsEvent], func(event UserFundingsEvent) bool { return strings.EqualFold(event.User, user) }, nil)
	if err != nil {
		return nil, err
	}
	handle, current := c.cachePrivateHandle(key, subscription, func() any { return &UserFundingsSubscription{subscription} })
	if !current {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		return c.SubscribeUserFundings(ctx, user)
	}
	typed, ok := handle.(*UserFundingsSubscription)
	if !ok {
		return nil, errors.New("websocket subscription registry type conflict")
	}
	return typed, nil
}

// SubscribeUserNonFundingLedgerUpdates subscribes to a user's non-funding
// ledger snapshots and incremental updates.
func (c *Client) SubscribeUserNonFundingLedgerUpdates(ctx context.Context, user string) (*UserLedgerSubscription, error) {
	if err := requireUser(user); err != nil {
		return nil, err
	}
	key := "userNonFundingLedgerUpdates:" + strings.ToLower(user)
	subscription, err := subscribeStream(ctx, c, key, "userNonFundingLedgerUpdates", newSubscriptionWire("userNonFundingLedgerUpdates", map[string]any{"user": user}), decodeJSON[UserLedgerEvent], func(event UserLedgerEvent) bool { return strings.EqualFold(event.User, user) }, nil)
	if err != nil {
		return nil, err
	}
	handle, current := c.cachePrivateHandle(key, subscription, func() any { return &UserLedgerSubscription{subscription} })
	if !current {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		return c.SubscribeUserNonFundingLedgerUpdates(ctx, user)
	}
	typed, ok := handle.(*UserLedgerSubscription)
	if !ok {
		return nil, errors.New("websocket subscription registry type conflict")
	}
	return typed, nil
}
