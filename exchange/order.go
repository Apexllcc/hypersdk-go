package exchange

import (
	"context"
	"fmt"
	"strings"

	"github.com/Apexllcc/hyperliquid-go-sdk/asset"
	"github.com/Apexllcc/hyperliquid-go-sdk/signing"
	"github.com/Apexllcc/hyperliquid-go-sdk/types"
	"github.com/shopspring/decimal"
)

type TimeInForce string

const (
	TIFGTC TimeInForce = "Gtc"
	TIFIOC TimeInForce = "Ioc"
	TIFALO TimeInForce = "Alo"
)

type LimitOrder struct{ TimeInForce TimeInForce }
type OrderRequest struct {
	Coin          string
	Market        *types.MarketRef
	IsBuy         bool
	Price         decimal.Decimal
	Size          decimal.Decimal
	ReduceOnly    bool
	Type          LimitOrder
	ClientOrderID *types.Cloid
}
type OrderResponse = ActionResponse

func (c *Client) PlaceOrder(ctx context.Context, request OrderRequest) (OrderResponse, error) {
	return c.PlaceOrders(ctx, []OrderRequest{request})
}
func (c *Client) PlaceOrders(ctx context.Context, requests []OrderRequest) (OrderResponse, error) {
	if len(requests) == 0 {
		return OrderResponse{}, fmt.Errorf("at least one order is required")
	}
	if c.assets == nil {
		return OrderResponse{}, fmt.Errorf("asset resolver is required")
	}
	orders := make([]signing.OrderWire, 0, len(requests))
	for _, r := range requests {
		a, err := c.resolveAsset(ctx, r)
		if err != nil {
			return OrderResponse{}, err
		}
		if a.Kind != asset.Perp && a.Kind != asset.Spot {
			return OrderResponse{}, fmt.Errorf("unsupported asset kind")
		}
		p, err := formatPrice(r.Price, a)
		if err != nil {
			return OrderResponse{}, err
		}
		s, err := formatSize(r.Size, a.SzDecimals)
		if err != nil {
			return OrderResponse{}, err
		}
		if r.Type.TimeInForce != TIFGTC && r.Type.TimeInForce != TIFIOC && r.Type.TimeInForce != TIFALO {
			return OrderResponse{}, fmt.Errorf("invalid time in force")
		}
		var cloid *string
		if r.ClientOrderID != nil {
			value := r.ClientOrderID.String()
			cloid = &value
		}
		orders = append(orders, signing.OrderWire{Asset: a.ID, IsBuy: r.IsBuy, Price: p, Size: s, ReduceOnly: r.ReduceOnly, Type: signing.LimitOrderType{TIF: string(r.Type.TimeInForce)}, Cloid: cloid})
	}
	action := signing.OrderAction{Orders: orders, Grouping: "na"}
	return c.submitL1(ctx, action)
}
func (c *Client) resolveAsset(ctx context.Context, request OrderRequest) (asset.Asset, error) {
	if request.Market != nil {
		resolver, ok := c.assets.(asset.MarketResolver)
		if !ok {
			return asset.Asset{}, fmt.Errorf("asset resolver does not support market references")
		}
		return resolver.ResolveMarket(ctx, *request.Market)
	}
	if request.Coin == "" {
		return asset.Asset{}, fmt.Errorf("coin is required")
	}
	return c.assets.Resolve(ctx, request.Coin)
}
func formatPrice(value decimal.Decimal, a asset.Asset) (string, error) {
	if !value.IsPositive() {
		return "", fmt.Errorf("invalid price")
	}
	max := int32(6 - a.SzDecimals)
	if a.Kind == asset.Spot {
		max = 8 - int32(a.SzDecimals)
	}
	if !value.Truncate(max).Equal(value) {
		return "", fmt.Errorf("invalid price")
	}
	digits := strings.Trim(strings.TrimLeft(strings.ReplaceAll(value.String(), ".", ""), "0"), "0")
	if len(digits) > 5 {
		return "", fmt.Errorf("invalid price")
	}
	return value.String(), nil
}
func formatSize(value decimal.Decimal, szDecimals int) (string, error) {
	if !value.IsPositive() || !value.Truncate(int32(szDecimals)).Equal(value) {
		return "", fmt.Errorf("invalid size")
	}
	return value.String(), nil
}
