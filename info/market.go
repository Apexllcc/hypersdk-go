package info

import (
	"context"
	"encoding/json"
	"fmt"
	"github.com/shopspring/decimal"
)

type AssetContext struct {
	DayNtlVlm    decimal.Decimal    `json:"dayNtlVlm"`
	Funding      decimal.Decimal    `json:"funding"`
	MarkPx       decimal.Decimal    `json:"markPx"`
	MidPx        *decimal.Decimal   `json:"midPx"`
	OpenInterest decimal.Decimal    `json:"openInterest"`
	OraclePx     decimal.Decimal    `json:"oraclePx"`
	Premium      *decimal.Decimal   `json:"premium"`
	PrevDayPx    decimal.Decimal    `json:"prevDayPx"`
	ImpactPxs    []*decimal.Decimal `json:"impactPxs"`
}
type MetaAndAssetContextsResponse struct {
	Meta     MetaResponse
	Contexts []AssetContext
}

func (r *MetaAndAssetContextsResponse) UnmarshalJSON(data []byte) error {
	var tuple []json.RawMessage
	if err := json.Unmarshal(data, &tuple); err != nil {
		return err
	}
	if len(tuple) != 2 {
		return fmt.Errorf("meta and asset contexts must contain two elements")
	}
	if err := json.Unmarshal(tuple[0], &r.Meta); err != nil {
		return err
	}
	return json.Unmarshal(tuple[1], &r.Contexts)
}

type RecentTrade struct {
	Coin  string          `json:"coin"`
	Side  string          `json:"side"`
	Px    decimal.Decimal `json:"px"`
	Sz    decimal.Decimal `json:"sz"`
	Time  int64           `json:"time"`
	Hash  string          `json:"hash"`
	Tid   uint64          `json:"tid"`
	Users []string        `json:"users"`
}
type FundingHistoryEntry struct {
	Coin        string          `json:"coin"`
	FundingRate decimal.Decimal `json:"fundingRate"`
	Premium     decimal.Decimal `json:"premium"`
	Time        int64           `json:"time"`
}
type PerpDEX struct {
	Name          string  `json:"name"`
	FullName      string  `json:"fullName"`
	Deployer      string  `json:"deployer"`
	OracleUpdater *string `json:"oracleUpdater"`
	FeeRecipient  *string `json:"feeRecipient"`
}

func (c *Client) MetaAndAssetContexts(ctx context.Context) (MetaAndAssetContextsResponse, error) {
	var r MetaAndAssetContextsResponse
	err := c.call(ctx, struct {
		Type string `json:"type"`
	}{"metaAndAssetCtxs"}, &r)
	return r, err
}
func (c *Client) RecentTrades(ctx context.Context, coin string) ([]RecentTrade, error) {
	if coin == "" {
		return nil, fmt.Errorf("coin is required")
	}
	var r []RecentTrade
	err := c.call(ctx, struct {
		Type string `json:"type"`
		Coin string `json:"coin"`
	}{"recentTrades", coin}, &r)
	return r, err
}
func (c *Client) FundingHistory(ctx context.Context, coin string, startTime int64, endTime *int64) ([]FundingHistoryEntry, error) {
	if coin == "" || startTime < 0 {
		return nil, fmt.Errorf("invalid funding history request")
	}
	var r []FundingHistoryEntry
	err := c.call(ctx, struct {
		Type      string `json:"type"`
		Coin      string `json:"coin"`
		StartTime int64  `json:"startTime"`
		EndTime   *int64 `json:"endTime,omitempty"`
	}{"fundingHistory", coin, startTime, endTime}, &r)
	return r, err
}
func (c *Client) PredictedFundings(ctx context.Context) (json.RawMessage, error) {
	var r json.RawMessage
	err := c.call(ctx, struct {
		Type string `json:"type"`
	}{"predictedFundings"}, &r)
	return r, err
}
func (c *Client) PerpDEXs(ctx context.Context) ([]*PerpDEX, error) {
	var r []*PerpDEX
	err := c.call(ctx, struct {
		Type string `json:"type"`
	}{"perpDexs"}, &r)
	return r, err
}
