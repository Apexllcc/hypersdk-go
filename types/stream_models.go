package types

import (
	"encoding/json"
	"fmt"

	"github.com/shopspring/decimal"
)

// OpenOrder is an active order returned by the Info API and order streams.
type OpenOrder struct {
	Coin      string          `json:"coin"`
	LimitPx   decimal.Decimal `json:"limitPx"`
	OID       uint64          `json:"oid"`
	Side      string          `json:"side"`
	Size      decimal.Decimal `json:"sz"`
	Timestamp int64           `json:"timestamp"`
	Cloid     *string         `json:"cloid,omitempty"`
}

// FrontendOpenOrder is the frontend order representation shared by Info and
// WebSocket order streams.
type FrontendOpenOrder struct {
	OpenOrder
	IsPositionTPSL   bool            `json:"isPositionTpsl"`
	IsTrigger        bool            `json:"isTrigger"`
	OrderType        string          `json:"orderType"`
	OriginalSize     decimal.Decimal `json:"origSz"`
	ReduceOnly       bool            `json:"reduceOnly"`
	TimeInForce      string          `json:"tif"`
	TriggerCondition string          `json:"triggerCondition"`
	TriggerPx        decimal.Decimal `json:"triggerPx"`
}

// UnmarshalJSON accepts both documented frontend-open-order objects and the
// orderStatus envelope, whose frontend fields wrap the base order in `order`.
func (o *FrontendOpenOrder) UnmarshalJSON(data []byte) error {
	type flat FrontendOpenOrder
	var decoded flat
	if err := json.Unmarshal(data, &decoded); err != nil {
		return err
	}
	if decoded.OID != 0 {
		*o = FrontendOpenOrder(decoded)
		return nil
	}
	var nested struct {
		Order OpenOrder `json:"order"`
		flat
	}
	if err := json.Unmarshal(data, &nested); err != nil {
		return err
	}
	decoded = nested.flat
	decoded.OpenOrder = nested.Order
	*o = FrontendOpenOrder(decoded)
	return nil
}

// UserFill is an execution returned by the Info API and fill streams.
type UserFill struct {
	ClosedPnl     decimal.Decimal `json:"closedPnl"`
	Coin          string          `json:"coin"`
	Crossed       bool            `json:"crossed"`
	Dir           string          `json:"dir"`
	Hash          string          `json:"hash"`
	OID           uint64          `json:"oid"`
	Px            decimal.Decimal `json:"px"`
	Side          string          `json:"side"`
	StartPosition decimal.Decimal `json:"startPosition"`
	Size          decimal.Decimal `json:"sz"`
	Time          int64           `json:"time"`
	Fee           decimal.Decimal `json:"fee"`
	FeeToken      string          `json:"feeToken"`
	Tid           uint64          `json:"tid"`
	Cloid         *string         `json:"cloid,omitempty"`
}

// HistoricalOrder is a completed or active order returned by historicalOrders.
type HistoricalOrder struct {
	Order           FrontendOpenOrder `json:"order"`
	Status          string            `json:"status"`
	StatusTimestamp int64             `json:"statusTimestamp"`
}

// TwapSliceFill associates a fill with its parent TWAP identifier.
type TwapSliceFill struct {
	Fill   UserFill `json:"fill"`
	TWAPID uint64   `json:"twapId"`
}

// AssetContext contains current perpetual market context values.
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

// SpotAssetContext contains current spot market context values.
type SpotAssetContext struct {
	DayNtlVlm         decimal.Decimal  `json:"dayNtlVlm"`
	MarkPx            *decimal.Decimal `json:"markPx"`
	MidPx             *decimal.Decimal `json:"midPx"`
	PrevDayPx         *decimal.Decimal `json:"prevDayPx"`
	CirculatingSupply *decimal.Decimal `json:"circulatingSupply"`
	Coin              string           `json:"coin"`
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

type SpotEscrow struct {
	Coin  string          `json:"coin"`
	Token int             `json:"token"`
	Total decimal.Decimal `json:"total"`
}

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
