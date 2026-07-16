package websocket

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
)

// ExplorerAction is the action attached to an Explorer transaction. Explorer
// actions are an open union: the stable discriminator is Type and the
// action-specific object fields are preserved exactly in Object. Historical
// user details can instead use a positional Tuple.
type ExplorerAction struct {
	Type   string                     `json:"-"`
	Object map[string]json.RawMessage `json:"-"`
	Tuple  []json.RawMessage          `json:"-"`
	Raw    json.RawMessage            `json:"-"`
}

func (a *ExplorerAction) UnmarshalJSON(data []byte) error {
	*a = ExplorerAction{Raw: append(json.RawMessage(nil), data...)}
	trimmed := bytes.TrimSpace(data)
	if len(trimmed) == 0 {
		return errors.New("explorer action is empty")
	}
	switch trimmed[0] {
	case '{':
		if err := json.Unmarshal(trimmed, &a.Object); err != nil {
			return err
		}
		if rawType := a.Object["type"]; rawType != nil {
			if err := json.Unmarshal(rawType, &a.Type); err != nil {
				return err
			}
		}
		return nil
	case '[':
		return json.Unmarshal(trimmed, &a.Tuple)
	default:
		return errors.New("explorer action must be an object or tuple")
	}
}

// ExplorerTransaction is a read-only transaction emitted by the Hyperliquid
// explorer RPC. Numeric protocol identifiers are integers, never float64.
type ExplorerTransaction struct {
	Action ExplorerAction `json:"action"`
	Block  uint64         `json:"block"`
	Error  *string        `json:"error"`
	Hash   string         `json:"hash"`
	Time   int64          `json:"time"`
	User   string         `json:"user"`
}

// ExplorerBlock is one block summary from the explorer RPC stream.
type ExplorerBlock struct {
	BlockTime       int64  `json:"blockTime"`
	Hash            string `json:"hash"`
	Height          uint64 `json:"height"`
	NumTransactions uint64 `json:"numTxs"`
	Proposer        string `json:"proposer"`
}

// ExplorerBlockSubscription delivers batches of block summaries. The explorer
// RPC sends these batches as raw JSON arrays rather than the usual Hyperliquid
// {channel,data} envelope.
type ExplorerBlockSubscription struct {
	*streamSubscription[[]ExplorerBlock]
}

// ExplorerTxsSubscription delivers batches of read-only explorer transactions.
type ExplorerTxsSubscription struct {
	*streamSubscription[[]ExplorerTransaction]
}

// SubscribeExplorerBlock subscribes to the explorer RPC block stream. It must
// be used with a Client configured for the RPC WebSocket URL, not the trading
// API WebSocket URL.
//
// This stream is a compatibility implementation based on the public explorer
// RPC behaviour and nktkas/hyperliquid. It is not an Info/Action WebSocket
// post request and never carries a signing or trading operation.
func (c *Client) SubscribeExplorerBlock(ctx context.Context) (*ExplorerBlockSubscription, error) {
	const key, channel = "explorerBlock", "explorerBlock_"
	subscription, err := subscribeStream(ctx, c, key, channel, newSubscriptionWire("explorerBlock", map[string]any{}), decodeJSON[[]ExplorerBlock], func([]ExplorerBlock) bool { return true }, nil)
	if err != nil {
		return nil, err
	}
	handle, current := c.cachePrivateHandle(key, subscription, func() any { return &ExplorerBlockSubscription{subscription} })
	if !current {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		return c.SubscribeExplorerBlock(ctx)
	}
	typed, ok := handle.(*ExplorerBlockSubscription)
	if !ok {
		return nil, errors.New("websocket subscription registry type conflict")
	}
	return typed, nil
}

// SubscribeExplorerTxs subscribes to the explorer RPC transaction stream. It
// must be used with a Client configured for the RPC WebSocket URL.
//
// This stream is a compatibility implementation based on the public explorer
// RPC behaviour and nktkas/hyperliquid. It is not an Info/Action WebSocket
// post request and never carries a signing or trading operation.
func (c *Client) SubscribeExplorerTxs(ctx context.Context) (*ExplorerTxsSubscription, error) {
	const key, channel = "explorerTxs", "explorerTxs_"
	subscription, err := subscribeStream(ctx, c, key, channel, newSubscriptionWire("explorerTxs", map[string]any{}), decodeJSON[[]ExplorerTransaction], func([]ExplorerTransaction) bool { return true }, nil)
	if err != nil {
		return nil, err
	}
	handle, current := c.cachePrivateHandle(key, subscription, func() any { return &ExplorerTxsSubscription{subscription} })
	if !current {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		return c.SubscribeExplorerTxs(ctx)
	}
	typed, ok := handle.(*ExplorerTxsSubscription)
	if !ok {
		return nil, errors.New("websocket subscription registry type conflict")
	}
	return typed, nil
}
