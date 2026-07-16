package types

import "github.com/shopspring/decimal"

// Leverage is the margin mode and leverage configuration reported for an asset.
type Leverage struct {
	Type   string           `json:"type"`
	Value  int              `json:"value"`
	RawUsd *decimal.Decimal `json:"rawUsd,omitempty"`
}

// ActiveAssetDataResponse is a user's trade capacity for one perpetual asset.
// It is shared by the Info endpoint and the activeAssetData WebSocket channel.
type ActiveAssetDataResponse struct {
	User             string             `json:"user"`
	Coin             string             `json:"coin"`
	Leverage         Leverage           `json:"leverage"`
	MaxTradeSizes    [2]decimal.Decimal `json:"maxTradeSzs"`
	AvailableToTrade [2]decimal.Decimal `json:"availableToTrade"`
	MarkPx           decimal.Decimal    `json:"markPx"`
}
