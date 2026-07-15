package exchange

import (
	"context"
	"fmt"

	"github.com/Apexllcc/hyperliquid-go-sdk/asset"
	"github.com/Apexllcc/hyperliquid-go-sdk/signing"
	"github.com/Apexllcc/hyperliquid-go-sdk/types"
	"github.com/shopspring/decimal"
)

// TWAPOrderRequest starts a protocol-native TWAP order. Minutes is the total
// duration and Size is precision-checked against the resolved asset metadata.
type TWAPOrderRequest struct {
	Coin       string
	Market     *types.MarketRef
	IsBuy      bool
	Size       decimal.Decimal
	ReduceOnly bool
	Minutes    uint64
	Randomize  bool
}

// PlaceTWAP starts a signed L1 TWAP order. Exchange actions are deliberately
// never retried because a transport failure does not establish non-execution.
func (c *Client) PlaceTWAP(ctx context.Context, request TWAPOrderRequest) (ActionResponse, error) {
	if !request.Size.IsPositive() {
		return ActionResponse{}, fmt.Errorf("TWAP size must be positive")
	}
	if request.Minutes == 0 {
		return ActionResponse{}, fmt.Errorf("TWAP minutes must be positive")
	}
	resolved, err := c.resolveTWAPAsset(ctx, request.Coin, request.Market)
	if err != nil {
		return ActionResponse{}, err
	}
	size, err := formatSize(request.Size, resolved.SzDecimals)
	if err != nil {
		return ActionResponse{}, fmt.Errorf("invalid TWAP size: %w", err)
	}
	return c.submitL1(ctx, signing.TWAPOrderAction{TWAP: signing.TWAPWire{
		Asset:      resolved.ID,
		IsBuy:      request.IsBuy,
		Size:       size,
		ReduceOnly: request.ReduceOnly,
		Minutes:    request.Minutes,
		Randomize:  request.Randomize,
	}})
}

// TWAPCancelRequest identifies a native TWAP by market and protocol ID.
type TWAPCancelRequest struct {
	Coin   string
	Market *types.MarketRef
	TWAPID uint64
}

// CancelTWAP cancels a signed L1 TWAP order.
func (c *Client) CancelTWAP(ctx context.Context, request TWAPCancelRequest) (ActionResponse, error) {
	if request.TWAPID == 0 {
		return ActionResponse{}, fmt.Errorf("TWAP ID must be positive")
	}
	resolved, err := c.resolveTWAPAsset(ctx, request.Coin, request.Market)
	if err != nil {
		return ActionResponse{}, err
	}
	return c.submitL1(ctx, signing.TWAPCancelAction{Asset: resolved.ID, TWAPID: request.TWAPID})
}

func (c *Client) resolveTWAPAsset(ctx context.Context, coin string, market *types.MarketRef) (asset.Asset, error) {
	if c.assets == nil {
		return asset.Asset{}, fmt.Errorf("asset resolver is required")
	}
	if market != nil {
		if coin != "" {
			return asset.Asset{}, fmt.Errorf("specify either TWAP coin or market reference, not both")
		}
		resolver, ok := c.assets.(asset.MarketResolver)
		if !ok {
			return asset.Asset{}, fmt.Errorf("asset resolver does not support market references")
		}
		return resolver.ResolveMarket(ctx, *market)
	}
	if coin == "" {
		return asset.Asset{}, fmt.Errorf("TWAP coin is required")
	}
	return c.assets.Resolve(ctx, coin)
}
