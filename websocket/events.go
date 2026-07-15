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
