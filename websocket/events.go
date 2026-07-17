package websocket

import (
	"encoding/json"
	"fmt"

	"github.com/shopspring/decimal"
)

// L2BookRequest identifies one L2 book stream.
type L2BookRequest struct {
	Coin     string `json:"coin"`
	NSigFigs *int   `json:"nSigFigs,omitempty"`
	Mantissa *int   `json:"mantissa,omitempty"`
	Fast     *bool  `json:"fast,omitempty"`
}

// L2BookEvent is one L2 book event. Bid levels are Levels[0], and ask levels
// are Levels[1]. Price and size remain exact decimals through Level.
type L2BookEvent struct {
	Coin   string           `json:"coin"`
	Time   int64            `json:"time"`
	Spread *decimal.Decimal `json:"spread,omitempty"`
	Levels BookLevels       `json:"levels"`
}

// BookLevels is Hyperliquid's fixed [bids, asks] order-book tuple.
type BookLevels [2][]Level

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

// ActiveAssetCtxRequest identifies a perp or spot active asset context. For
// HIP-3 markets, Coin is the official namespaced market symbol (for example,
// "dex:BTC"); activeAssetCtx has no separate DEX field in the protocol.
type ActiveAssetCtxRequest struct {
	Coin string `json:"coin"`
}

// ActiveAssetCtxEvent is the union returned by the activeAssetCtx feed. Exactly
// one of Perp or Spot is populated. The union is selected from the documented
// distinguishing context fields rather than guessed from the symbol.
type ActiveAssetCtxEvent struct {
	Coin string        `json:"coin"`
	Perp *PerpAssetCtx `json:"-"`
	Spot *SpotAssetCtx `json:"-"`
}

// SharedAssetCtx fields are shared by perp and spot market contexts.
// All economic quantities use decimal.Decimal to avoid float64 loss.
type SharedAssetCtx struct {
	DayNotionalVolume decimal.Decimal  `json:"dayNtlVlm"`
	PreviousDayPrice  decimal.Decimal  `json:"prevDayPx"`
	MarkPrice         decimal.Decimal  `json:"markPx"`
	MidPrice          *decimal.Decimal `json:"midPx,omitempty"`
}

// PerpAssetCtx is the perp variant of an active asset context.
type PerpAssetCtx struct {
	SharedAssetCtx
	Funding      decimal.Decimal `json:"funding"`
	OpenInterest decimal.Decimal `json:"openInterest"`
	OraclePrice  decimal.Decimal `json:"oraclePx"`
}

// SpotAssetCtx is the spot variant of an active asset context.
type SpotAssetCtx struct {
	SharedAssetCtx
	CirculatingSupply decimal.Decimal `json:"circulatingSupply"`
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
	Liquidation   *FillLiquidation `json:"liquidation,omitempty"`
}

// FillLiquidation describes the liquidation which produced a fill.
type FillLiquidation struct {
	LiquidatedUser string          `json:"liquidatedUser,omitempty"`
	MarkPrice      decimal.Decimal `json:"markPx"`
	Method         string          `json:"method"`
}

// UserFunding is one funding payment emitted by user event streams.
type UserFunding struct {
	Time        int64           `json:"time"`
	Coin        string          `json:"coin"`
	USDC        decimal.Decimal `json:"usdc"`
	Size        decimal.Decimal `json:"szi"`
	FundingRate decimal.Decimal `json:"fundingRate"`
	Samples     *int64          `json:"nSamples,omitempty"`
}

// OrderUpdate is a status transition for one user order.
type OrderUpdate struct {
	Order           BasicOrder `json:"order"`
	Status          string     `json:"status"`
	StatusTimestamp int64      `json:"statusTimestamp"`
}

// BasicOrder is the order representation emitted by the orderUpdates stream.
type BasicOrder struct {
	Coin       string          `json:"coin"`
	Side       string          `json:"side"`
	LimitPrice decimal.Decimal `json:"limitPx"`
	Size       decimal.Decimal `json:"sz"`
	OrderID    int64           `json:"oid"`
	Timestamp  int64           `json:"timestamp"`
	OriginalSz decimal.Decimal `json:"origSz"`
	Cloid      string          `json:"cloid,omitempty"`
}

// UserFillsEvent is the snapshot or incremental userFills payload.
type UserFillsEvent struct {
	IsSnapshot bool       `json:"isSnapshot,omitempty"`
	User       string     `json:"user"`
	Fills      []UserFill `json:"fills"`
}

// UserFundingsEvent is the snapshot or incremental userFundings payload.
type UserFundingsEvent struct {
	IsSnapshot bool          `json:"isSnapshot,omitempty"`
	User       string        `json:"user"`
	Fundings   []UserFunding `json:"fundings"`
}

// UserLedgerEvent is the snapshot or incremental non-funding ledger payload.
type UserLedgerEvent struct {
	IsSnapshot bool          `json:"isSnapshot,omitempty"`
	User       string        `json:"user"`
	Updates    []LedgerEntry `json:"nonFundingLedgerUpdates"`
}

// LedgerEntry is one non-funding ledger update.
type LedgerEntry struct {
	Time  int64       `json:"time"`
	Hash  string      `json:"hash"`
	Delta LedgerDelta `json:"delta"`
}

// LedgerDelta is a tagged union. Exactly one typed payload is set for known
// protocol update kinds. Unknown variants are retained as Raw instead of being
// silently discarded, so SDK consumers can handle protocol additions safely.
type LedgerDelta struct {
	Type                  string                       `json:"type"`
	Deposit               *LedgerDeposit               `json:"-"`
	Withdraw              *LedgerWithdraw              `json:"-"`
	InternalTransfer      *LedgerInternalTransfer      `json:"-"`
	SubaccountTransfer    *LedgerSubaccountTransfer    `json:"-"`
	Liquidation           *LedgerLiquidation           `json:"-"`
	VaultDelta            *LedgerVaultDelta            `json:"-"`
	VaultWithdrawal       *LedgerVaultWithdrawal       `json:"-"`
	VaultLeaderCommission *LedgerVaultLeaderCommission `json:"-"`
	SpotTransfer          *LedgerSpotTransfer          `json:"-"`
	AccountClassTransfer  *LedgerAccountClassTransfer  `json:"-"`
	SpotGenesis           *LedgerSpotGenesis           `json:"-"`
	RewardsClaim          *LedgerRewardsClaim          `json:"-"`
	Raw                   json.RawMessage              `json:"-"`
}

type LedgerDeposit struct {
	Type string          `json:"type"`
	USDC decimal.Decimal `json:"usdc"`
}
type LedgerWithdraw struct {
	Type  string          `json:"type"`
	USDC  decimal.Decimal `json:"usdc"`
	Nonce int64           `json:"nonce"`
	Fee   decimal.Decimal `json:"fee"`
}
type LedgerInternalTransfer struct {
	Type        string          `json:"type"`
	USDC        decimal.Decimal `json:"usdc"`
	User        string          `json:"user"`
	Destination string          `json:"destination"`
	Fee         decimal.Decimal `json:"fee"`
}
type LedgerSubaccountTransfer struct {
	Type        string          `json:"type"`
	USDC        decimal.Decimal `json:"usdc"`
	User        string          `json:"user"`
	Destination string          `json:"destination"`
}
type LedgerLiquidation struct {
	Type                string               `json:"type"`
	AccountValue        decimal.Decimal      `json:"accountValue"`
	LeverageType        string               `json:"leverageType"`
	LiquidatedPositions []LiquidatedPosition `json:"liquidatedPositions"`
}
type LiquidatedPosition struct {
	Coin string          `json:"coin"`
	Size decimal.Decimal `json:"szi"`
}
type LedgerVaultDelta struct {
	Type  string          `json:"type"`
	Vault string          `json:"vault"`
	USDC  decimal.Decimal `json:"usdc"`
}
type LedgerVaultWithdrawal struct {
	Type            string          `json:"type"`
	Vault           string          `json:"vault"`
	User            string          `json:"user"`
	RequestedUSD    decimal.Decimal `json:"requestedUsd"`
	Commission      decimal.Decimal `json:"commission"`
	ClosingCost     decimal.Decimal `json:"closingCost"`
	Basis           decimal.Decimal `json:"basis"`
	NetWithdrawnUSD decimal.Decimal `json:"netWithdrawnUsd"`
}
type LedgerVaultLeaderCommission struct {
	Type string          `json:"type"`
	User string          `json:"user"`
	USDC decimal.Decimal `json:"usdc"`
}
type LedgerSpotTransfer struct {
	Type        string          `json:"type"`
	Token       string          `json:"token"`
	Amount      decimal.Decimal `json:"amount"`
	USDCValue   decimal.Decimal `json:"usdcValue"`
	User        string          `json:"user"`
	Destination string          `json:"destination"`
	Fee         decimal.Decimal `json:"fee"`
}
type LedgerAccountClassTransfer struct {
	Type   string          `json:"type"`
	USDC   decimal.Decimal `json:"usdc"`
	ToPerp bool            `json:"toPerp"`
}
type LedgerSpotGenesis struct {
	Type   string          `json:"type"`
	Token  string          `json:"token"`
	Amount decimal.Decimal `json:"amount"`
}
type LedgerRewardsClaim struct {
	Type   string          `json:"type"`
	Amount decimal.Decimal `json:"amount"`
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

func (event *ActiveAssetCtxEvent) UnmarshalJSON(data []byte) error {
	var wire struct {
		Coin string          `json:"coin"`
		Ctx  json.RawMessage `json:"ctx"`
	}
	if err := json.Unmarshal(data, &wire); err != nil {
		return err
	}
	if wire.Coin == "" {
		return fmt.Errorf("active asset context coin is required")
	}
	var tag struct {
		Funding           *json.RawMessage `json:"funding"`
		CirculatingSupply *json.RawMessage `json:"circulatingSupply"`
	}
	if err := json.Unmarshal(wire.Ctx, &tag); err != nil {
		return err
	}
	*event = ActiveAssetCtxEvent{Coin: wire.Coin}
	switch {
	case tag.Funding != nil && tag.CirculatingSupply == nil:
		event.Perp = &PerpAssetCtx{}
		return json.Unmarshal(wire.Ctx, event.Perp)
	case tag.CirculatingSupply != nil && tag.Funding == nil:
		event.Spot = &SpotAssetCtx{}
		return json.Unmarshal(wire.Ctx, event.Spot)
	case tag.Funding != nil && tag.CirculatingSupply != nil:
		return fmt.Errorf("active asset context has both perp and spot discriminator fields")
	default:
		return fmt.Errorf("active asset context has no perp or spot discriminator field")
	}
}

func (delta *LedgerDelta) UnmarshalJSON(data []byte) error {
	var tag struct {
		Type string `json:"type"`
	}
	if err := json.Unmarshal(data, &tag); err != nil {
		return err
	}
	*delta = LedgerDelta{Type: tag.Type, Raw: append(json.RawMessage(nil), data...)}
	switch tag.Type {
	case "deposit":
		delta.Deposit = &LedgerDeposit{}
		return json.Unmarshal(data, delta.Deposit)
	case "withdraw":
		delta.Withdraw = &LedgerWithdraw{}
		return json.Unmarshal(data, delta.Withdraw)
	case "internalTransfer":
		delta.InternalTransfer = &LedgerInternalTransfer{}
		return json.Unmarshal(data, delta.InternalTransfer)
	case "subAccountTransfer":
		delta.SubaccountTransfer = &LedgerSubaccountTransfer{}
		return json.Unmarshal(data, delta.SubaccountTransfer)
	case "liquidation":
		delta.Liquidation = &LedgerLiquidation{}
		return json.Unmarshal(data, delta.Liquidation)
	case "vaultCreate", "vaultDeposit", "vaultDistribution":
		delta.VaultDelta = &LedgerVaultDelta{}
		return json.Unmarshal(data, delta.VaultDelta)
	case "vaultWithdraw":
		delta.VaultWithdrawal = &LedgerVaultWithdrawal{}
		return json.Unmarshal(data, delta.VaultWithdrawal)
	case "vaultLeaderCommission":
		delta.VaultLeaderCommission = &LedgerVaultLeaderCommission{}
		return json.Unmarshal(data, delta.VaultLeaderCommission)
	case "spotTransfer":
		delta.SpotTransfer = &LedgerSpotTransfer{}
		return json.Unmarshal(data, delta.SpotTransfer)
	case "accountClassTransfer":
		delta.AccountClassTransfer = &LedgerAccountClassTransfer{}
		return json.Unmarshal(data, delta.AccountClassTransfer)
	case "spotGenesis":
		delta.SpotGenesis = &LedgerSpotGenesis{}
		return json.Unmarshal(data, delta.SpotGenesis)
	case "rewardsClaim":
		delta.RewardsClaim = &LedgerRewardsClaim{}
		return json.Unmarshal(data, delta.RewardsClaim)
	case "":
		return fmt.Errorf("ledger delta type is required")
	default:
		return nil
	}
}
