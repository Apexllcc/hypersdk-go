package info

import (
	"encoding/json"
	"fmt"
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

// ClearinghouseStateResponse is a perpetual account state.
type ClearinghouseStateResponse struct {
	AssetPositions             []AssetPosition `json:"assetPositions"`
	CrossMaintenanceMarginUsed decimal.Decimal `json:"crossMaintenanceMarginUsed"`
	CrossMarginSummary         MarginSummary   `json:"crossMarginSummary"`
	MarginSummary              MarginSummary   `json:"marginSummary"`
	Time                       int64           `json:"time"`
	Withdrawable               decimal.Decimal `json:"withdrawable"`
}
type AssetPosition struct {
	Position Position `json:"position"`
	Type     string   `json:"type"`
}
type Position struct {
	Coin           string             `json:"coin"`
	CumFunding     Funding            `json:"cumFunding"`
	EntryPx        *decimal.Decimal   `json:"entryPx"`
	Leverage       Leverage           `json:"leverage"`
	LiquidationPx  *decimal.Decimal   `json:"liquidationPx"`
	MarginUsed     decimal.Decimal    `json:"marginUsed"`
	MaxLeverage    int                `json:"maxLeverage"`
	MaxTradeSizes  [2]decimal.Decimal `json:"maxTradeSzs"`
	PositionValue  decimal.Decimal    `json:"positionValue"`
	ReturnOnEquity decimal.Decimal    `json:"returnOnEquity"`
	Szi            decimal.Decimal    `json:"szi"`
	UnrealizedPnl  decimal.Decimal    `json:"unrealizedPnl"`
}

// UnmarshalJSON accepts Hyperliquid's historical "NaN" liquidation-price
// sentinel as an unavailable value while retaining exact decimals for actual
// prices.
func (p *Position) UnmarshalJSON(data []byte) error {
	type positionAlias Position
	var wire struct {
		*positionAlias
		LiquidationPx json.RawMessage `json:"liquidationPx"`
	}
	wire.positionAlias = (*positionAlias)(p)
	if err := json.Unmarshal(data, &wire); err != nil {
		return err
	}
	if len(wire.LiquidationPx) == 0 || string(wire.LiquidationPx) == "null" {
		p.LiquidationPx = nil
		return nil
	}
	var raw string
	if err := json.Unmarshal(wire.LiquidationPx, &raw); err != nil {
		return err
	}
	if raw == "NaN" {
		p.LiquidationPx = nil
		return nil
	}
	value, err := decimal.NewFromString(raw)
	if err != nil {
		return fmt.Errorf("liquidation price: %w", err)
	}
	p.LiquidationPx = &value
	return nil
}

type Funding struct {
	AllTime     decimal.Decimal `json:"allTime"`
	SinceOpen   decimal.Decimal `json:"sinceOpen"`
	SinceChange decimal.Decimal `json:"sinceChange"`
}
type Leverage struct {
	Type   string           `json:"type"`
	Value  int              `json:"value"`
	RawUsd *decimal.Decimal `json:"rawUsd,omitempty"`
}
type MarginSummary struct {
	AccountValue    decimal.Decimal `json:"accountValue"`
	TotalMarginUsed decimal.Decimal `json:"totalMarginUsed"`
	TotalNtlPos     decimal.Decimal `json:"totalNtlPos"`
	TotalRawUsd     decimal.Decimal `json:"totalRawUsd"`
}

// SpotClearinghouseStateResponse is a spot account state.
type SpotClearinghouseStateResponse struct {
	PortfolioMarginEnabled           *bool                            `json:"portfolioMarginEnabled,omitempty"`
	Balances                         []SpotBalance                    `json:"balances"`
	EVMEscrows                       []SpotEscrow                     `json:"evmEscrows"`
	PortfolioMarginRatio             *decimal.Decimal                 `json:"portfolioMarginRatio"`
	TokenToPortfolioBorrowRatio      []TokenDecimalPair               `json:"tokenToPortfolioBorrowRatio"`
	TokenToAvailableAfterMaintenance []TokenAvailableAfterMaintenance `json:"tokenToAvailableAfterMaintenance"`
}
type SpotBalance struct {
	Coin     string           `json:"coin"`
	Token    int              `json:"token"`
	Hold     decimal.Decimal  `json:"hold"`
	Total    decimal.Decimal  `json:"total"`
	EntryNtl *decimal.Decimal `json:"entryNtl,omitempty"`
	SpotHold *decimal.Decimal `json:"spotHold,omitempty"`
	LTV      *decimal.Decimal `json:"ltv,omitempty"`
	Borrowed *decimal.Decimal `json:"borrowed,omitempty"`
	Supplied *decimal.Decimal `json:"supplied,omitempty"`
}

// SpotEscrow is a balance escrowed on HyperEVM.
type SpotEscrow struct {
	Coin  string          `json:"coin"`
	Token int             `json:"token"`
	Total decimal.Decimal `json:"total"`
}

// TokenDecimalPair associates a token ID with a decimal value. The wire format
// is a [token, value] tuple.
type TokenDecimalPair struct {
	Token int
	Value decimal.Decimal
}

func (p *TokenDecimalPair) UnmarshalJSON(data []byte) error {
	var tuple []json.RawMessage
	if err := json.Unmarshal(data, &tuple); err != nil {
		return err
	}
	if len(tuple) != 2 {
		return fmt.Errorf("token decimal pair must contain token and value")
	}
	if err := json.Unmarshal(tuple[0], &p.Token); err != nil {
		return err
	}
	return json.Unmarshal(tuple[1], &p.Value)
}

// TokenAvailableAfterMaintenance associates a token ID with its remaining
// available balance after maintenance requirements. The wire format is a
// [token, amount] tuple.
type TokenAvailableAfterMaintenance struct {
	Token  int
	Amount decimal.Decimal
}

func (p *TokenAvailableAfterMaintenance) UnmarshalJSON(data []byte) error {
	var tuple []json.RawMessage
	if err := json.Unmarshal(data, &tuple); err != nil {
		return err
	}
	if len(tuple) != 2 {
		return fmt.Errorf("available-after-maintenance pair must contain token and amount")
	}
	if err := json.Unmarshal(tuple[0], &p.Token); err != nil {
		return err
	}
	return json.Unmarshal(tuple[1], &p.Amount)
}
