package info

import (
	"encoding/json"
	"fmt"

	"github.com/Apexllcc/hyperliquid-go-sdk/types"
	"github.com/shopspring/decimal"
)

// AllMidsResponse maps an exact market symbol to its decimal mid price.
type AllMidsResponse map[string]decimal.Decimal

// MetaResponse is the perpetual metadata response.
type MetaResponse struct {
	Universe        []PerpAsset   `json:"universe"`
	MarginTables    []MarginTable `json:"marginTables"`
	CollateralToken int           `json:"collateralToken"`
}
type PerpAsset struct {
	Name          string `json:"name"`
	SzDecimals    int    `json:"szDecimals"`
	MaxLeverage   int    `json:"maxLeverage"`
	MarginTableID int    `json:"marginTableId,omitempty"`
	OnlyIsolated  *bool  `json:"onlyIsolated,omitempty"`
	IsDelisted    bool   `json:"isDelisted,omitempty"`
	MarginMode    string `json:"marginMode,omitempty"`
}
type MarginTable struct {
	ID          int
	Description string       `json:"description"`
	MarginTiers []MarginTier `json:"marginTiers"`
}
type MarginTier struct {
	LowerBound  decimal.Decimal `json:"lowerBound"`
	MaxLeverage int             `json:"maxLeverage"`
}

func (m *MarginTable) UnmarshalJSON(data []byte) error {
	var tuple []json.RawMessage
	if err := json.Unmarshal(data, &tuple); err != nil {
		return err
	}
	if len(tuple) != 2 {
		return fmt.Errorf("margin table must contain ID and definition")
	}
	if err := json.Unmarshal(tuple[0], &m.ID); err != nil {
		return err
	}
	return json.Unmarshal(tuple[1], (*marginTableDefinition)(m))
}

type marginTableDefinition MarginTable

// L2BookResponse is the L2 order book snapshot.
type L2BookResponse struct {
	Coin   string           `json:"coin"`
	Time   int64            `json:"time"`
	Spread *decimal.Decimal `json:"spread,omitempty"`
	Levels [2][]BookLevel   `json:"levels"`
}
type BookLevel struct {
	Price      decimal.Decimal `json:"px"`
	Size       decimal.Decimal `json:"sz"`
	OrderCount int             `json:"n"`
}

// Candle is one OHLCV candle; all economic values remain decimal strings.
type Candle struct {
	Time       int64           `json:"t"`
	CloseTime  int64           `json:"T"`
	Symbol     string          `json:"s"`
	Interval   string          `json:"i"`
	Open       decimal.Decimal `json:"o"`
	Close      decimal.Decimal `json:"c"`
	High       decimal.Decimal `json:"h"`
	Low        decimal.Decimal `json:"l"`
	Volume     decimal.Decimal `json:"v"`
	TradeCount int             `json:"n"`
}

// Shared account DTO aliases preserve the Info package's public API while the
// same types are also used by WebSocket account streams.
type ClearinghouseStateResponse = types.ClearinghouseStateResponse
type AssetPosition = types.AssetPosition
type Position = types.Position
type Funding = types.Funding
type Leverage = types.Leverage
type MarginSummary = types.MarginSummary
type SpotClearinghouseStateResponse = types.SpotClearinghouseStateResponse
type SpotBalance = types.SpotBalance
type SpotEscrow = types.SpotEscrow
type TokenDecimalPair = types.TokenDecimalPair
type TokenAvailableAfterMaintenance = types.TokenAvailableAfterMaintenance
