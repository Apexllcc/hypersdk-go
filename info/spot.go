package info

import (
	"context"
	"encoding/json"
	"fmt"
	"github.com/shopspring/decimal"
)

// SpotMetaResponse describes spot tokens and tradable spot pairs.
type SpotMetaResponse struct {
	Tokens   []SpotToken `json:"tokens"`
	Universe []SpotPair  `json:"universe"`
}
type SpotToken struct {
	Name        string `json:"name"`
	SzDecimals  int    `json:"szDecimals"`
	WeiDecimals int    `json:"weiDecimals"`
	Index       int    `json:"index"`
	TokenID     string `json:"tokenId"`
	IsCanonical bool   `json:"isCanonical"`
}
type SpotPair struct {
	Name        string `json:"name"`
	Tokens      [2]int `json:"tokens"`
	Index       int    `json:"index"`
	IsCanonical bool   `json:"isCanonical"`
}

// SpotAssetContext contains current spot market context values.
type SpotAssetContext struct {
	DayNtlVlm         decimal.Decimal  `json:"dayNtlVlm"`
	MarkPx            *decimal.Decimal `json:"markPx"`
	MidPx             *decimal.Decimal `json:"midPx"`
	PrevDayPx         *decimal.Decimal `json:"prevDayPx"`
	CirculatingSupply *decimal.Decimal `json:"circulatingSupply"`
	Coin              string           `json:"coin"`
}
type SpotMetaAndAssetContextsResponse struct {
	Meta     SpotMetaResponse
	Contexts []SpotAssetContext
}

func (r *SpotMetaAndAssetContextsResponse) UnmarshalJSON(data []byte) error {
	var tuple []json.RawMessage
	if err := json.Unmarshal(data, &tuple); err != nil {
		return err
	}
	if len(tuple) != 2 {
		return fmt.Errorf("spot meta and asset contexts must contain two elements")
	}
	if err := json.Unmarshal(tuple[0], &r.Meta); err != nil {
		return err
	}
	return json.Unmarshal(tuple[1], &r.Contexts)
}

func (c *Client) SpotMeta(ctx context.Context) (SpotMetaResponse, error) {
	var r SpotMetaResponse
	err := c.call(ctx, struct {
		Type string `json:"type"`
	}{"spotMeta"}, &r)
	return r, err
}
func (c *Client) SpotMetaAndAssetContexts(ctx context.Context) (SpotMetaAndAssetContextsResponse, error) {
	var r SpotMetaAndAssetContextsResponse
	err := c.call(ctx, struct {
		Type string `json:"type"`
	}{"spotMetaAndAssetCtxs"}, &r)
	return r, err
}
