package types

import "github.com/shopspring/decimal"

// MarketKind makes cross-market symbol resolution explicit.
type MarketKind string

const (
	Perpetual MarketKind = "perp"
	Spot      MarketKind = "spot"
	HIP3      MarketKind = "hip3"
	Outcome   MarketKind = "outcome"
)

// Asset identifies a market in a particular DEX namespace.
type Asset struct {
	ID         int
	Symbol     string
	Name       string
	Kind       MarketKind
	SzDecimals int
	DEX        string
}

// MarketRef is an unambiguous asset lookup key. Symbol alone is intentionally
// insufficient when perpetual, spot, HIP-3, or outcome venues share a name.
type MarketRef struct {
	Symbol string
	Kind   MarketKind
	DEX    string
}

// PriceSize is a precision-safe price/quantity pair.
type PriceSize struct {
	Price decimal.Decimal
	Size  decimal.Decimal
}
