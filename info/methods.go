package info

import (
	"context"
	"fmt"
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
	if coin == "" {
		return L2BookResponse{}, fmt.Errorf("coin is required")
	}
	var r L2BookResponse
	err := c.call(ctx, L2BookRequest{Type: "l2Book", Coin: coin}, &r)
	return r, err
}
func (c *Client) CandleSnapshot(ctx context.Context, request CandleRequest) ([]Candle, error) {
	if request.Coin == "" || request.Interval == "" || request.StartTime < 0 || (request.EndTime != nil && *request.EndTime < request.StartTime) {
		return nil, fmt.Errorf("invalid candle request")
	}
	var r []Candle
	err := c.call(ctx, CandleSnapshotRequest{Type: "candleSnapshot", Req: request}, &r)
	return r, err
}
func (c *Client) ClearinghouseState(ctx context.Context, user string) (ClearinghouseStateResponse, error) {
	if user == "" {
		return ClearinghouseStateResponse{}, fmt.Errorf("user is required")
	}
	var r ClearinghouseStateResponse
	err := c.call(ctx, ClearinghouseStateRequest{Type: "clearinghouseState", User: user}, &r)
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
