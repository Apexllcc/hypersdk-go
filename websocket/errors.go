package websocket

import "errors"

var ErrWebSocketClosed = errors.New("websocket client is closed")
var ErrDuplicateSubscription = errors.New("duplicate subscription")
