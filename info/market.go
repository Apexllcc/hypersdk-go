package info

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/Apexllcc/hypersdk-go/types"
	"github.com/shopspring/decimal"
)

type AssetContext = types.AssetContext
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

// PredictedFunding is one asset's projected funding across supported venues.
// The Info endpoint encodes it as [asset, exchanges]. Extra tuple elements are
// ignored so later protocol extensions remain backward compatible.
type PredictedFunding struct {
	Asset     string
	Exchanges []PredictedFundingVenue
}

// PredictedFundingVenue is one venue's projected funding. Data is nil when a
// venue does not currently publish a funding prediction.
type PredictedFundingVenue struct {
	Exchange string
	Data     *PredictedFundingData
}

// PredictedFundingData contains the numeric prediction values published by a
// venue. FundingRate is decimal to avoid binary floating-point loss.
type PredictedFundingData struct {
	FundingRate          decimal.Decimal `json:"fundingRate"`
	NextFundingTime      int64           `json:"nextFundingTime"`
	FundingIntervalHours *int64          `json:"fundingIntervalHours,omitempty"`
}

func (p *PredictedFunding) UnmarshalJSON(data []byte) error {
	var tuple []json.RawMessage
	if err := json.Unmarshal(data, &tuple); err != nil {
		return err
	}
	if len(tuple) < 2 {
		return fmt.Errorf("predicted funding must contain asset and exchanges")
	}
	if err := json.Unmarshal(tuple[0], &p.Asset); err != nil {
		return fmt.Errorf("decode predicted funding asset: %w", err)
	}
	if err := json.Unmarshal(tuple[1], &p.Exchanges); err != nil {
		return fmt.Errorf("decode predicted funding venues: %w", err)
	}
	return nil
}

func (p *PredictedFundingVenue) UnmarshalJSON(data []byte) error {
	var tuple []json.RawMessage
	if err := json.Unmarshal(data, &tuple); err != nil {
		return err
	}
	if len(tuple) < 2 {
		return fmt.Errorf("predicted funding venue must contain exchange and data")
	}
	if err := json.Unmarshal(tuple[0], &p.Exchange); err != nil {
		return fmt.Errorf("decode predicted funding exchange: %w", err)
	}
	if string(tuple[1]) == "null" {
		p.Data = nil
		return nil
	}
	p.Data = new(PredictedFundingData)
	if err := json.Unmarshal(tuple[1], p.Data); err != nil {
		return fmt.Errorf("decode predicted funding data: %w", err)
	}
	return nil
}

func (c *Client) PredictedFundings(ctx context.Context) ([]PredictedFunding, error) {
	var r []PredictedFunding
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
