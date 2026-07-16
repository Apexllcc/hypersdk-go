package info

import (
	"context"
	"fmt"

	"github.com/Apexllcc/hyperliquid-go-sdk/internal/validation"
)

func (c *Client) AllMids(ctx context.Context) (AllMidsResponse, error) {
	var r AllMidsResponse
	err := c.call(ctx, AllMidsRequest{Type: "allMids"}, &r)
	return r, err
}
func (c *Client) Meta(ctx context.Context) (MetaResponse, error) {
	return c.MetaForDEX(ctx, "")
}

// MetaForDEX retrieves perpetual metadata for the original DEX (empty name)
// or an explicit builder-deployed HIP-3 DEX.
func (c *Client) MetaForDEX(ctx context.Context, dex string) (MetaResponse, error) {
	var r MetaResponse
	err := c.call(ctx, MetaRequest{Type: "meta", DEX: dex}, &r)
	return r, err
}
func (c *Client) L2Book(ctx context.Context, coin string) (L2BookResponse, error) {
	return c.L2BookWithOptions(ctx, L2BookRequest{Coin: coin})
}

// L2BookWithOptions retrieves a market order book with the optional official
// nSigFigs/mantissa aggregation controls. Omitted nSigFigs retains full
// precision; mantissa is valid only when nSigFigs is 5.
func (c *Client) L2BookWithOptions(ctx context.Context, request L2BookRequest) (L2BookResponse, error) {
	if request.Coin == "" {
		return L2BookResponse{}, fmt.Errorf("coin is required")
	}
	if err := validation.L2BookAggregation(request.NSigFigs, request.Mantissa); err != nil {
		return L2BookResponse{}, err
	}
	var r L2BookResponse
	request.Type = "l2Book"
	err := c.call(ctx, request, &r)
	return r, err
}
func (c *Client) CandleSnapshot(ctx context.Context, request CandleRequest) ([]Candle, error) {
	if request.Coin == "" || request.Interval == "" || request.StartTime < 0 || (request.EndTime != nil && *request.EndTime < request.StartTime) {
		return nil, fmt.Errorf("invalid candle request")
	}
	if err := validation.CandleInterval(request.Interval); err != nil {
		return nil, err
	}
	var r []Candle
	err := c.call(ctx, CandleSnapshotRequest{Type: "candleSnapshot", Req: request}, &r)
	return r, err
}
func (c *Client) ClearinghouseState(ctx context.Context, user string) (ClearinghouseStateResponse, error) {
	return c.ClearinghouseStateForDEX(ctx, user, "")
}

// ClearinghouseStateForDEX retrieves a perpetual account state for the base
// DEX (empty name) or a builder-deployed HIP-3 DEX.
func (c *Client) ClearinghouseStateForDEX(ctx context.Context, user, dex string) (ClearinghouseStateResponse, error) {
	if user == "" {
		return ClearinghouseStateResponse{}, fmt.Errorf("user is required")
	}
	var r ClearinghouseStateResponse
	err := c.call(ctx, ClearinghouseStateRequest{Type: "clearinghouseState", User: user, DEX: dex}, &r)
	return r, err
}
func (c *Client) SpotClearinghouseState(ctx context.Context, user string) (SpotClearinghouseStateResponse, error) {
	if user == "" {
		return SpotClearinghouseStateResponse{}, fmt.Errorf("user is required")
	}
	var r SpotClearinghouseStateResponse
	err := c.call(ctx, SpotClearinghouseStateRequest{Type: "spotClearinghouseState", User: user}, &r)
	return r, err
}
