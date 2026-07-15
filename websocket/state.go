package websocket

import "errors"

// SubscriptionState describes the lifecycle of one logical subscription. A
// connected state only confirms the shared socket is open; subscribed confirms
// the subscription request was written. Hyperliquid does not require callers
// to wait for the subscriptionResponse acknowledgement before receiving data.
type SubscriptionState string

const (
	SubscriptionStateConnecting   SubscriptionState = "connecting"
	SubscriptionStateConnected    SubscriptionState = "connected"
	SubscriptionStateReconnecting SubscriptionState = "reconnecting"
	SubscriptionStateSubscribed   SubscriptionState = "subscribed"
	SubscriptionStateUnsubscribed SubscriptionState = "unsubscribed"
	SubscriptionStateError        SubscriptionState = "error"
)

// SubscriptionStateEvent is emitted in causal order for one logical
// subscription. Error is set only for SubscriptionStateError.
type SubscriptionStateEvent struct {
	// Sequence starts at one and increases for every state transition. A gap
	// tells callers that an older non-terminal transition was coalesced because
	// they were not draining States quickly enough.
	Sequence uint64
	State    SubscriptionState
	Error    error
}

// statefulSubscription is kept separate from managedSubscription so existing
// custom internal subscriptions do not have to expose lifecycle state.
type statefulSubscription interface {
	stateChange(SubscriptionState, error)
}

// enqueueSubscriptionState keeps the most recent bounded lifecycle view. It
// intentionally never blocks socket read/reconnect paths: a slow optional
// observer cannot prevent the shared connection from recovering. Sequence
// makes any coalescing explicit to observers.
func enqueueSubscriptionState(events chan SubscriptionStateEvent, event SubscriptionStateEvent) {
	select {
	case events <- event:
		return
	default:
	}
	// Only the subscription's delivery lock calls this helper, so removing the
	// oldest event cannot race a concurrent producer.
	select {
	case <-events:
	default:
	}
	events <- event
}

func subscriptionStateError(err error) error {
	if err == nil {
		return errors.New("websocket subscription error state requires an error")
	}
	return err
}
