package exchange

import (
	"context"
	"fmt"
	"github.com/Apexllcc/hyperliquid-go-sdk/signing"
)

// ModifyRequest replaces an existing numeric order ID with a validated order.
type ModifyRequest struct {
	OID   uint64
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
		if request.OID == 0 {
			return ActionResponse{}, fmt.Errorf("invalid order ID")
		}
		wire, err := c.orderWireForModify(ctx, request.Order)
		if err != nil {
			return ActionResponse{}, err
		}
		modifies = append(modifies, signing.ModifyWire{OID: request.OID, Order: wire})
	}
	return c.submitL1(ctx, signing.BatchModifyAction{Modifies: modifies})
}
func (c *Client) orderWireForModify(ctx context.Context, request OrderRequest) (signing.OrderWire, error) {
	asset, err := c.resolveAsset(ctx, request)
	if err != nil {
		return signing.OrderWire{}, err
	}
	price, err := formatPrice(request.Price, asset)
	if err != nil {
		return signing.OrderWire{}, err
	}
	size, err := formatSize(request.Size, asset.SzDecimals)
	if err != nil {
		return signing.OrderWire{}, err
	}
	if request.Type.TimeInForce != TIFGTC && request.Type.TimeInForce != TIFIOC && request.Type.TimeInForce != TIFALO {
		return signing.OrderWire{}, fmt.Errorf("invalid time in force")
	}
	var cloid *string
	if request.ClientOrderID != nil {
		value := request.ClientOrderID.String()
		cloid = &value
	}
	return signing.OrderWire{Asset: asset.ID, IsBuy: request.IsBuy, Price: price, Size: size, ReduceOnly: request.ReduceOnly, Type: signing.LimitOrderType{TIF: string(request.Type.TimeInForce)}, Cloid: cloid}, nil
}
