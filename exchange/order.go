package exchange

import (
	"context"
	"fmt"
	"strings"

	"github.com/Apexllcc/hypersdk-go/asset"
	"github.com/Apexllcc/hypersdk-go/signing"
	"github.com/Apexllcc/hypersdk-go/types"
	"github.com/ethereum/go-ethereum/common"
	"github.com/shopspring/decimal"
)

type TimeInForce string

const (
	TIFGTC TimeInForce = "Gtc"
	TIFIOC TimeInForce = "Ioc"
	TIFALO TimeInForce = "Alo"
)

type LimitOrder struct{ TimeInForce TimeInForce }

func (LimitOrder) isOrderType() {}

// TriggerTPSL identifies whether a trigger is take-profit or stop-loss.
type TriggerTPSL string

const (
	TPSLTakeProfit TriggerTPSL = "tp"
	TPSLStopLoss   TriggerTPSL = "sl"
)

// TriggerOrder creates a take-profit or stop-loss order. Price remains the
// execution price (and is used as the protected limit for non-market orders).
type TriggerOrder struct {
	IsMarket     bool
	TriggerPrice decimal.Decimal
	TPSL         TriggerTPSL
}

func (TriggerOrder) isOrderType() {}

type orderType interface{ isOrderType() }

// Builder receives an optional fee measured in tenths of one basis point.
type Builder struct {
	Address common.Address
	Fee     uint64
}

type OrderRequest struct {
	Coin          string
	Market        *types.MarketRef
	IsBuy         bool
	Price         decimal.Decimal
	Size          decimal.Decimal
	ReduceOnly    bool
	Type          orderType
	ClientOrderID *types.Cloid
	Builder       *Builder
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
	resolvedAssets := make([]asset.Asset, 0, len(requests))
	for _, r := range requests {
		a, err := c.resolveAsset(ctx, r)
		if err != nil {
			return OrderResponse{}, err
		}
		// HIP-3 markets use the same L1 order wire format as base perpetuals;
		// only their asset ID and DEX namespace differ.
		if a.Kind != asset.Perp && a.Kind != asset.Spot && a.Kind != asset.HIP3 && a.Kind != asset.Outcome {
			return OrderResponse{}, fmt.Errorf("unsupported asset kind")
		}
		order, err := c.orderWire(ctx, r, a)
		if err != nil {
			return OrderResponse{}, err
		}
		orders = append(orders, order)
		resolvedAssets = append(resolvedAssets, a)
	}
	builder, err := builderForOrders(requests, resolvedAssets)
	if err != nil {
		return OrderResponse{}, err
	}
	action := signing.OrderAction{Orders: orders, Grouping: "na", Builder: builder}
	return c.submitL1(ctx, action)
}

func builderForOrders(requests []OrderRequest, assets []asset.Asset) (*signing.BuilderWire, error) {
	if len(requests) != len(assets) {
		return nil, fmt.Errorf("builder validation requires resolved assets")
	}
	var output *signing.BuilderWire
	firstHasBuilder := requests[0].Builder != nil
	for index, request := range requests {
		if (request.Builder != nil) != firstHasBuilder {
			return nil, fmt.Errorf("all orders in a batch must use the same builder")
		}
		if request.Builder == nil {
			continue
		}
		if request.Builder.Address == (common.Address{}) {
			return nil, fmt.Errorf("builder address is required")
		}
		if request.Builder.Fee == 0 {
			return nil, fmt.Errorf("builder fee must be positive")
		}
		maximum := uint64(100)
		if assets[index].Kind == asset.Spot || assets[index].Kind == asset.Outcome {
			maximum = 1000
		}
		if request.Builder.Fee > maximum {
			return nil, fmt.Errorf("builder fee %d exceeds maximum %d for %s", request.Builder.Fee, maximum, assets[index].Kind)
		}
		candidate := &signing.BuilderWire{Address: strings.ToLower(request.Builder.Address.Hex()), Fee: request.Builder.Fee}
		if output == nil {
			output = candidate
			continue
		}
		if *output != *candidate {
			return nil, fmt.Errorf("all orders in a batch must use the same builder")
		}
	}
	return output, nil
}

func (c *Client) orderWire(ctx context.Context, request OrderRequest, a asset.Asset) (signing.OrderWire, error) {
	p, err := formatPrice(request.Price, a)
	if err != nil {
		return signing.OrderWire{}, err
	}
	s, err := formatSize(request.Size, a.SzDecimals)
	if err != nil {
		return signing.OrderWire{}, err
	}
	var kind signing.OrderTypeWire
	switch orderType := request.Type.(type) {
	case LimitOrder:
		if orderType.TimeInForce != TIFGTC && orderType.TimeInForce != TIFIOC && orderType.TimeInForce != TIFALO {
			return signing.OrderWire{}, fmt.Errorf("invalid time in force")
		}
		kind = signing.LimitOrderType{TIF: string(orderType.TimeInForce)}
	case TriggerOrder:
		if orderType.TPSL != TPSLTakeProfit && orderType.TPSL != TPSLStopLoss {
			return signing.OrderWire{}, fmt.Errorf("invalid trigger type")
		}
		triggerPrice, err := formatPrice(orderType.TriggerPrice, a)
		if err != nil {
			return signing.OrderWire{}, fmt.Errorf("invalid trigger price: %w", err)
		}
		kind = signing.TriggerOrderType{IsMarket: orderType.IsMarket, TriggerPx: triggerPrice, TPSL: string(orderType.TPSL)}
	default:
		return signing.OrderWire{}, fmt.Errorf("invalid order type")
	}
	var cloid *string
	if request.ClientOrderID != nil {
		value := request.ClientOrderID.String()
		cloid = &value
	}
	return signing.OrderWire{Asset: a.ID, IsBuy: request.IsBuy, Price: p, Size: s, ReduceOnly: request.ReduceOnly, Type: kind, Cloid: cloid}, nil
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
	if a.Kind == asset.Spot || a.Kind == asset.Outcome {
		max = 8 - int32(a.SzDecimals)
	}
	if !value.Truncate(max).Equal(value) {
		return "", fmt.Errorf("invalid price")
	}
	// The protocol permits integer prices regardless of their number of
	// significant figures. Decimal prices remain limited to five figures.
	if value.Equal(value.Truncate(0)) {
		return value.String(), nil
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
