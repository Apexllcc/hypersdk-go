package websocket

import (
	"encoding/json"

	"github.com/shopspring/decimal"
)

// L2BookRequest identifies one L2 book stream.
type L2BookRequest struct {
	Coin     string `json:"coin"`
	NSigFigs *int   `json:"nSigFigs,omitempty"`
	Mantissa *int   `json:"mantissa,omitempty"`
}

// L2BookEvent is one L2 book event. Levels retain their protocol JSON until a
// shared order-book DTO is introduced.
type L2BookEvent struct {
	Coin   string          `json:"coin"`
	Time   int64           `json:"time"`
	Levels json.RawMessage `json:"levels"`
}

// AllMidsRequest identifies the allMids stream. DEX is optional; when empty,
// Hyperliquid uses the first perp DEX and includes spot mids.
type AllMidsRequest struct {
	DEX string `json:"dex,omitempty"`
}

// AllMidsEvent contains exact decimal mids keyed by market symbol.
type AllMidsEvent struct {
	Mids map[string]decimal.Decimal `json:"mids"`
}

// TradesRequest identifies one market trade stream.
type TradesRequest struct {
	Coin string `json:"coin"`
}

// TradeEvent is one trade update. Price and Size are exact decimals.
type TradeEvent struct {
	Coin  string          `json:"coin"`
	Side  string          `json:"side"`
	Price decimal.Decimal `json:"px"`
	Size  decimal.Decimal `json:"sz"`
	Hash  string          `json:"hash"`
	Time  int64           `json:"time"`
	TID   int64           `json:"tid"`
	Users [2]string       `json:"users"`
}

// CandleRequest identifies one candle stream.
type CandleRequest struct {
	Coin     string `json:"coin"`
	Interval string `json:"interval"`
}

// CandleEvent is one OHLCV candle. Price and volume values are exact decimals.
type CandleEvent struct {
	OpenTime  int64           `json:"t"`
	CloseTime int64           `json:"T"`
	Coin      string          `json:"s"`
	Interval  string          `json:"i"`
	Open      decimal.Decimal `json:"o"`
	Close     decimal.Decimal `json:"c"`
	High      decimal.Decimal `json:"h"`
	Low       decimal.Decimal `json:"l"`
	Volume    decimal.Decimal `json:"v"`
	NumTrades int64           `json:"n"`
}

// BBORequest identifies one best-bid-offer stream.
type BBORequest struct {
	Coin string `json:"coin"`
}

// Level is one bid or offer level.
type Level struct {
	Price decimal.Decimal `json:"px"`
	Size  decimal.Decimal `json:"sz"`
	Count int64           `json:"n"`
}

// BBOEvent contains the current best bid and ask; either can be nil.
type BBOEvent struct {
	Coin string `json:"coin"`
	Time int64  `json:"time"`
	Bid  *Level `json:"-"`
	Ask  *Level `json:"-"`
}

// UserEvent is the tagged union emitted by the userEvents subscription. Exactly
// one of Fills, Funding, Liquidation, or NonUserCancel is normally populated.
type UserEvent struct {
	Fills         []UserFill           `json:"fills,omitempty"`
	Funding       *UserFunding         `json:"funding,omitempty"`
	Liquidation   *LiquidationEvent    `json:"liquidation,omitempty"`
	NonUserCancel []NonUserCancelEvent `json:"nonUserCancel,omitempty"`
}

// UserFill is one user trade fill emitted by user event streams.
type UserFill struct {
	Coin          string           `json:"coin"`
	Price         decimal.Decimal  `json:"px"`
	Size          decimal.Decimal  `json:"sz"`
	Side          string           `json:"side"`
	Time          int64            `json:"time"`
	StartPosition decimal.Decimal  `json:"startPosition"`
	Direction     string           `json:"dir"`
	ClosedPnL     decimal.Decimal  `json:"closedPnl"`
	Hash          string           `json:"hash"`
	OrderID       int64            `json:"oid"`
	Crossed       bool             `json:"crossed"`
	Fee           decimal.Decimal  `json:"fee"`
	TradeID       int64            `json:"tid"`
	FeeToken      string           `json:"feeToken"`
	BuilderFee    *decimal.Decimal `json:"builderFee,omitempty"`
}

// UserFunding is one funding payment emitted by user event streams.
type UserFunding struct {
	Time        int64           `json:"time"`
	Coin        string          `json:"coin"`
	USDC        decimal.Decimal `json:"usdc"`
	Size        decimal.Decimal `json:"szi"`
	FundingRate decimal.Decimal `json:"fundingRate"`
}

// LiquidationEvent represents a liquidation user event.
type LiquidationEvent struct {
	LiquidationID          int64           `json:"lid"`
	Liquidator             string          `json:"liquidator"`
	LiquidatedUser         string          `json:"liquidated_user"`
	LiquidatedNotionalSize decimal.Decimal `json:"liquidated_ntl_pos"`
	LiquidatedAccountValue decimal.Decimal `json:"liquidated_account_value"`
}

// NonUserCancelEvent identifies an order cancelled outside the user action path.
type NonUserCancelEvent struct {
	Coin    string `json:"coin"`
	OrderID int64  `json:"oid"`
}

func (event *BBOEvent) UnmarshalJSON(data []byte) error {
	var wire struct {
		Coin string    `json:"coin"`
		Time int64     `json:"time"`
		BBO  [2]*Level `json:"bbo"`
	}
	if err := json.Unmarshal(data, &wire); err != nil {
		return err
	}
	event.Coin, event.Time, event.Bid, event.Ask = wire.Coin, wire.Time, wire.BBO[0], wire.BBO[1]
	return nil
}
