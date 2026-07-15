package websocket

import "errors"

var ErrWebSocketClosed = errors.New("websocket client is closed")
var ErrDuplicateSubscription = errors.New("duplicate subscription")
var ErrEventDropped = errors.New("websocket subscription event dropped due to backpressure")
var ErrAmbiguousAllMids = errors.New("only one allMids DEX subscription is supported per WebSocket client")
var ErrConflictingUserFillsSubscription = errors.New("userFills aggregation mode conflicts with an active subscription for the same user")
