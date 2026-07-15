package websocket

import (
	"context"
	"github.com/gorilla/websocket"
)

type l2SubscriptionWire struct {
	Method       string `json:"method"`
	Subscription struct {
		Type     string `json:"type"`
		Coin     string `json:"coin"`
		NSigFigs *int   `json:"nSigFigs,omitempty"`
		Mantissa *int   `json:"mantissa,omitempty"`
	} `json:"subscription"`
}

func newL2SubscriptionWire(request L2BookRequest) l2SubscriptionWire {
	wire := l2SubscriptionWire{Method: "subscribe"}
	wire.Subscription.Type = "l2Book"
	wire.Subscription.Coin = request.Coin
	wire.Subscription.NSigFigs = request.NSigFigs
	wire.Subscription.Mantissa = request.Mantissa
	return wire
}
func dial(ctx context.Context, url string) (*websocket.Conn, error) {
	connection, _, err := websocket.DefaultDialer.DialContext(ctx, url, nil)
	return connection, err
}
