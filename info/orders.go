package info

import (
	"context"
	"fmt"

	"github.com/Apexllcc/hyperliquid-go-sdk/types"
	"github.com/shopspring/decimal"
)

// OpenOrder is an active order returned by the Info API.
type OpenOrder struct {
	Coin      string          `json:"coin"`
	LimitPx   decimal.Decimal `json:"limitPx"`
	OID       uint64          `json:"oid"`
	Side      string          `json:"side"`
	Size      decimal.Decimal `json:"sz"`
	Timestamp int64           `json:"timestamp"`
	Cloid     *string         `json:"cloid,omitempty"`
}
type FrontendOpenOrder struct {
	OpenOrder
	IsPositionTPSL   bool            `json:"isPositionTpsl"`
	IsTrigger        bool            `json:"isTrigger"`
	OrderType        string          `json:"orderType"`
	OriginalSize     decimal.Decimal `json:"origSz"`
	ReduceOnly       bool            `json:"reduceOnly"`
	TriggerCondition string          `json:"triggerCondition"`
	TriggerPx        decimal.Decimal `json:"triggerPx"`
	TIF              string          `json:"tif"`
}
type OrderStatusResponse struct {
	Status          string             `json:"status"`
	Order           *FrontendOpenOrder `json:"order,omitempty"`
	StatusTimestamp int64              `json:"statusTimestamp,omitempty"`
}
type UserFill struct {
	ClosedPnl     decimal.Decimal `json:"closedPnl"`
	Coin          string          `json:"coin"`
	Crossed       bool            `json:"crossed"`
	Dir           string          `json:"dir"`
	Hash          string          `json:"hash"`
	OID           uint64          `json:"oid"`
	Px            decimal.Decimal `json:"px"`
	Side          string          `json:"side"`
	StartPosition decimal.Decimal `json:"startPosition"`
	Size          decimal.Decimal `json:"sz"`
	Time          int64           `json:"time"`
	Fee           decimal.Decimal `json:"fee"`
	FeeToken      string          `json:"feeToken"`
	Tid           uint64          `json:"tid"`
	Cloid         *string         `json:"cloid,omitempty"`
}

func (c *Client) OpenOrders(ctx context.Context, user string) ([]OpenOrder, error) {
	if user == "" {
		return nil, fmt.Errorf("user is required")
	}
	var r []OpenOrder
	err := c.call(ctx, struct {
		Type string `json:"type"`
		User string `json:"user"`
	}{"openOrders", user}, &r)
	return r, err
}
func (c *Client) FrontendOpenOrders(ctx context.Context, user string) ([]FrontendOpenOrder, error) {
	if user == "" {
		return nil, fmt.Errorf("user is required")
	}
	var r []FrontendOpenOrder
	err := c.call(ctx, struct {
		Type string `json:"type"`
		User string `json:"user"`
	}{"frontendOpenOrders", user}, &r)
	return r, err
}
func (c *Client) OrderStatus(ctx context.Context, user string, oid uint64) (OrderStatusResponse, error) {
	if user == "" || oid == 0 {
		return OrderStatusResponse{}, fmt.Errorf("invalid order status request")
	}
	var r OrderStatusResponse
	err := c.call(ctx, struct {
		Type string `json:"type"`
		User string `json:"user"`
		OID  uint64 `json:"oid"`
	}{"orderStatus", user, oid}, &r)
	return r, err
}

// OrderStatusByCloid returns an order's status using its 128-bit client order
// identifier. The Info API accepts a CLOID string in the oid field.
func (c *Client) OrderStatusByCloid(ctx context.Context, user string, cloid types.Cloid) (OrderStatusResponse, error) {
	if user == "" {
		return OrderStatusResponse{}, fmt.Errorf("invalid order status request")
	}
	var r OrderStatusResponse
	err := c.call(ctx, struct {
		Type string `json:"type"`
		User string `json:"user"`
		OID  string `json:"oid"`
	}{"orderStatus", user, cloid.String()}, &r)
	return r, err
}
func (c *Client) UserFills(ctx context.Context, user string, aggregateByTime bool) ([]UserFill, error) {
	if user == "" {
		return nil, fmt.Errorf("user is required")
	}
	var r []UserFill
	err := c.call(ctx, struct {
		Type      string `json:"type"`
		User      string `json:"user"`
		Aggregate bool   `json:"aggregateByTime"`
	}{"userFills", user, aggregateByTime}, &r)
	return r, err
}
func (c *Client) UserFillsByTime(ctx context.Context, user string, startTime int64, endTime *int64, aggregateByTime bool) ([]UserFill, error) {
	if user == "" || startTime < 0 {
		return nil, fmt.Errorf("invalid fills request")
	}
	var r []UserFill
	err := c.call(ctx, struct {
		Type      string `json:"type"`
		User      string `json:"user"`
		StartTime int64  `json:"startTime"`
		EndTime   *int64 `json:"endTime,omitempty"`
		Aggregate bool   `json:"aggregateByTime"`
	}{"userFillsByTime", user, startTime, endTime, aggregateByTime}, &r)
	return r, err
}
