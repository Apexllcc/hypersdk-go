package exchange

import (
	"context"
	"fmt"
	"github.com/Apexllcc/hypersdk-go/signing"
	"github.com/Apexllcc/hypersdk-go/types"
)

// CancelRequest identifies one order by market and order ID.
type CancelRequest struct {
	Coin string
	OID  uint64
}

// CancelOrder cancels a single order through the shared L1 signed path.
func (c *Client) CancelOrder(ctx context.Context, request CancelRequest) (ActionResponse, error) {
	return c.CancelOrders(ctx, []CancelRequest{request})
}

// CancelOrders cancels multiple orders in one action.
func (c *Client) CancelOrders(ctx context.Context, requests []CancelRequest) (ActionResponse, error) {
	if len(requests) == 0 {
		return ActionResponse{}, fmt.Errorf("at least one cancel is required")
	}
	if c.assets == nil {
		return ActionResponse{}, fmt.Errorf("asset resolver is required")
	}
	cancels := make([]signing.CancelWire, 0, len(requests))
	for _, request := range requests {
		if request.Coin == "" || request.OID == 0 {
			return ActionResponse{}, fmt.Errorf("invalid cancel request")
		}
		asset, err := c.assets.Resolve(ctx, request.Coin)
		if err != nil {
			return ActionResponse{}, err
		}
		cancels = append(cancels, signing.CancelWire{Asset: asset.ID, OID: request.OID})
	}
	return c.submitL1(ctx, signing.CancelAction{Cancels: cancels})
}

// CancelByCloidRequest identifies an order by its validated client order ID.
type CancelByCloidRequest struct {
	Coin  string
	Cloid types.Cloid
}

func (c *Client) CancelByCloid(ctx context.Context, request CancelByCloidRequest) (ActionResponse, error) {
	return c.CancelByCloids(ctx, []CancelByCloidRequest{request})
}
func (c *Client) CancelByCloids(ctx context.Context, requests []CancelByCloidRequest) (ActionResponse, error) {
	if len(requests) == 0 {
		return ActionResponse{}, fmt.Errorf("at least one cancel is required")
	}
	if c.assets == nil {
		return ActionResponse{}, fmt.Errorf("asset resolver is required")
	}
	cancels := make([]signing.CancelByCloidWire, 0, len(requests))
	for _, request := range requests {
		if request.Coin == "" {
			return ActionResponse{}, fmt.Errorf("invalid cancel request")
		}
		asset, err := c.assets.Resolve(ctx, request.Coin)
		if err != nil {
			return ActionResponse{}, err
		}
		cancels = append(cancels, signing.CancelByCloidWire{Asset: asset.ID, Cloid: request.Cloid.String()})
	}
	return c.submitL1(ctx, signing.CancelByCloidAction{Cancels: cancels})
}

// ScheduleCancel schedules a global cancellation time, or clears it when at is nil.
func (c *Client) ScheduleCancel(ctx context.Context, at *uint64) (ActionResponse, error) {
	return c.submitL1(ctx, signing.ScheduleCancelAction{Time: at})
}
