package exchange

import (
	"context"
	"fmt"
	"github.com/Apexllcc/hypersdk-go/signing"
	"github.com/Apexllcc/hypersdk-go/types"
)

// ModifyRequest replaces an existing numeric order ID with a validated order.
type ModifyRequest struct {
	OID   uint64
	Cloid *types.Cloid
	Order OrderRequest
}

func (c *Client) ModifyOrder(ctx context.Context, request ModifyRequest) (ActionResponse, error) {
	return c.BatchModify(ctx, []ModifyRequest{request})
}
func (c *Client) BatchModify(ctx context.Context, requests []ModifyRequest) (ActionResponse, error) {
	if len(requests) == 0 {
		return ActionResponse{}, fmt.Errorf("at least one modification is required")
	}
	if c.assets == nil {
		return ActionResponse{}, fmt.Errorf("asset resolver is required")
	}
	modifies := make([]signing.ModifyWire, 0, len(requests))
	for _, request := range requests {
		if (request.OID == 0) == (request.Cloid == nil) {
			return ActionResponse{}, fmt.Errorf("specify exactly one numeric order ID or cloid")
		}
		wire, err := c.orderWireForModify(ctx, request.Order)
		if err != nil {
			return ActionResponse{}, err
		}
		modify := signing.ModifyWire{OID: request.OID, Order: wire}
		if request.Cloid != nil {
			cloid := request.Cloid.String()
			modify.Cloid = &cloid
		}
		modifies = append(modifies, modify)
	}
	return c.submitL1(ctx, signing.BatchModifyAction{Modifies: modifies})
}
func (c *Client) orderWireForModify(ctx context.Context, request OrderRequest) (signing.OrderWire, error) {
	asset, err := c.resolveAsset(ctx, request)
	if err != nil {
		return signing.OrderWire{}, err
	}
	return c.orderWire(ctx, request, asset)
}
