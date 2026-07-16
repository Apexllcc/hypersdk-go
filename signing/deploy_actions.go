package signing

// This file contains the L1 action unions used for HIP-3 perpetual DEX and
// HIP-1/HIP-2 spot deployment.  Each action deliberately has one (and only
// one) variant key.  The marker methods make the public interfaces sealed:
// callers may use the protocol variants below but cannot accidentally inject
// an arbitrary map into a signed deployment action.

import (
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"unicode/utf8"

	"github.com/ethereum/go-ethereum/common"
	"github.com/shopspring/decimal"
	"github.com/vmihailenco/msgpack/v5"
)

// PerpDeployVariant is one official HIP-3 perpDeploy variant.
type PerpDeployVariant interface {
	perpDeployVariant()
	perpDeployKey() string
	validatePerpDeploy() error
}

// PerpDeployAction is the exact L1 action union for HIP-3 deployment and
// operation.  Variant must be one of the concrete types in this package.
type PerpDeployAction struct{ Variant PerpDeployVariant }

func (a PerpDeployAction) MarshalMsgpack() ([]byte, error) {
	if a.Variant == nil {
		return nil, fmt.Errorf("perpDeploy variant is required")
	}
	if err := a.Variant.validatePerpDeploy(); err != nil {
		return nil, err
	}
	return marshalMap(func(e *msgpack.Encoder) error {
		if err := e.EncodeMapLen(2); err != nil {
			return err
		}
		if err := e.EncodeString("type"); err != nil {
			return err
		}
		if err := e.EncodeString("perpDeploy"); err != nil {
			return err
		}
		if err := e.EncodeString(a.Variant.perpDeployKey()); err != nil {
			return err
		}
		return e.Encode(a.Variant)
	})
}

func (a PerpDeployAction) MarshalJSON() ([]byte, error) {
	return marshalDeployJSON("perpDeploy", a.Variant, func(v any) (string, error) {
		p, ok := v.(PerpDeployVariant)
		if !ok || p == nil {
			return "", fmt.Errorf("perpDeploy variant is required")
		}
		if err := p.validatePerpDeploy(); err != nil {
			return "", err
		}
		return p.perpDeployKey(), nil
	})
}

// SpotDeployVariant is one official HIP-1/HIP-2 spotDeploy variant.
type SpotDeployVariant interface {
	spotDeployVariant()
	spotDeployKey() string
	validateSpotDeploy() error
}

// SpotDeployAction is the exact L1 action union for HIP-1/HIP-2 deployment.
type SpotDeployAction struct{ Variant SpotDeployVariant }

func (a SpotDeployAction) MarshalMsgpack() ([]byte, error) {
	if a.Variant == nil {
		return nil, fmt.Errorf("spotDeploy variant is required")
	}
	if err := a.Variant.validateSpotDeploy(); err != nil {
		return nil, err
	}
	return marshalMap(func(e *msgpack.Encoder) error {
		if err := e.EncodeMapLen(2); err != nil {
			return err
		}
		if err := e.EncodeString("type"); err != nil {
			return err
		}
		if err := e.EncodeString("spotDeploy"); err != nil {
			return err
		}
		if err := e.EncodeString(a.Variant.spotDeployKey()); err != nil {
			return err
		}
		return e.Encode(a.Variant)
	})
}

func (a SpotDeployAction) MarshalJSON() ([]byte, error) {
	return marshalDeployJSON("spotDeploy", a.Variant, func(v any) (string, error) {
		p, ok := v.(SpotDeployVariant)
		if !ok || p == nil {
			return "", fmt.Errorf("spotDeploy variant is required")
		}
		if err := p.validateSpotDeploy(); err != nil {
			return "", err
		}
		return p.spotDeployKey(), nil
	})
}

func marshalDeployJSON(kind string, variant any, key func(any) (string, error)) ([]byte, error) {
	k, err := key(variant)
	if err != nil {
		return nil, err
	}
	// A two-field struct avoids exposing an arbitrary map in this security
	// sensitive path. Encoding the payload separately keeps the union key exact.
	payload, err := json.Marshal(variant)
	if err != nil {
		return nil, err
	}
	return []byte(`{"type":` + quoteJSON(kind) + `,` + quoteJSON(k) + `:` + string(payload) + `}`), nil
}
func quoteJSON(s string) string { b, _ := json.Marshal(s); return string(b) }

// StringPair is a protocol tuple of an asset/coin and a decimal string.
// Perp deployer tuple lists must be lexicographically sorted before signing.
type StringPair struct {
	Key   string
	Value string
}

func (p StringPair) MarshalMsgpack() ([]byte, error) {
	return marshalMap(func(e *msgpack.Encoder) error {
		if err := e.EncodeArrayLen(2); err != nil {
			return err
		}
		if err := e.EncodeString(p.Key); err != nil {
			return err
		}
		return e.EncodeString(p.Value)
	})
}
func (p StringPair) MarshalJSON() ([]byte, error) { return json.Marshal([2]string{p.Key, p.Value}) }

type StringUintPair struct {
	Key   string
	Value uint64
}

func (p StringUintPair) MarshalMsgpack() ([]byte, error) {
	return marshalMap(func(e *msgpack.Encoder) error {
		if err := e.EncodeArrayLen(2); err != nil {
			return err
		}
		if err := e.EncodeString(p.Key); err != nil {
			return err
		}
		return e.EncodeUint(p.Value)
	})
}
func (p StringUintPair) MarshalJSON() ([]byte, error) { return json.Marshal([2]any{p.Key, p.Value}) }

type StringBoolPair struct {
	Key   string
	Value bool
}

func (p StringBoolPair) MarshalMsgpack() ([]byte, error) {
	return marshalMap(func(e *msgpack.Encoder) error {
		if err := e.EncodeArrayLen(2); err != nil {
			return err
		}
		if err := e.EncodeString(p.Key); err != nil {
			return err
		}
		return e.EncodeBool(p.Value)
	})
}
func (p StringBoolPair) MarshalJSON() ([]byte, error) { return json.Marshal([2]any{p.Key, p.Value}) }

// MarginMode is the HIP-3 per-market margin mode.
type MarginMode string

const (
	MarginModeStrictIsolated MarginMode = "strictIsolated"
	MarginModeNoCross        MarginMode = "noCross"
	MarginModeNormal         MarginMode = "normal"
)

func validMarginMode(m MarginMode, normal bool) bool {
	return m == MarginModeStrictIsolated || m == MarginModeNoCross || (normal && m == MarginModeNormal)
}

type PerpDexSchemaInput struct {
	FullName        string  `json:"fullName" msgpack:"fullName"`
	CollateralToken uint64  `json:"collateralToken" msgpack:"collateralToken"`
	OracleUpdater   *string `json:"oracleUpdater,omitempty" msgpack:"oracleUpdater,omitempty"`
}

func (s PerpDexSchemaInput) validate() error {
	if strings.TrimSpace(s.FullName) == "" {
		return fmt.Errorf("perp dex full name is required")
	}
	if s.OracleUpdater != nil && !common.IsHexAddress(*s.OracleUpdater) {
		return fmt.Errorf("oracle updater is not an address")
	}
	return nil
}

// MarshalMsgpack intentionally includes a nil oracleUpdater. The official
// Python SDK signs that explicit field for a schema-bearing register action.
func (s PerpDexSchemaInput) MarshalMsgpack() ([]byte, error) {
	return marshalMap(func(e *msgpack.Encoder) error {
		if err := e.EncodeMapLen(3); err != nil {
			return err
		}
		for _, field := range []struct {
			key string
			val any
		}{{"fullName", s.FullName}, {"collateralToken", s.CollateralToken}, {"oracleUpdater", lowerAddressPtr(s.OracleUpdater)}} {
			if err := e.EncodeString(field.key); err != nil {
				return err
			}
			if err := e.Encode(field.val); err != nil {
				return err
			}
		}
		return nil
	})
}

func (s PerpDexSchemaInput) MarshalJSON() ([]byte, error) {
	return json.Marshal(struct {
		FullName        string  `json:"fullName"`
		CollateralToken uint64  `json:"collateralToken"`
		OracleUpdater   *string `json:"oracleUpdater"`
	}{s.FullName, s.CollateralToken, lowerAddressPtr(s.OracleUpdater)})
}

type RegisterAssetRequest struct {
	Coin          string `json:"coin" msgpack:"coin"`
	SzDecimals    uint8  `json:"szDecimals" msgpack:"szDecimals"`
	OraclePx      string `json:"oraclePx" msgpack:"oraclePx"`
	MarginTableID uint64 `json:"marginTableId" msgpack:"marginTableId"`
	OnlyIsolated  bool   `json:"onlyIsolated" msgpack:"onlyIsolated"`
}

func (r RegisterAssetRequest) validate() error {
	if err := validCoinPrice(r.Coin, r.OraclePx); err != nil {
		return err
	}
	if r.MarginTableID == 0 {
		return fmt.Errorf("margin table ID must be non-zero")
	}
	return nil
}

type RegisterAssetRequest2 struct {
	Coin          string     `json:"coin" msgpack:"coin"`
	SzDecimals    uint8      `json:"szDecimals" msgpack:"szDecimals"`
	OraclePx      string     `json:"oraclePx" msgpack:"oraclePx"`
	MarginTableID uint64     `json:"marginTableId" msgpack:"marginTableId"`
	MarginMode    MarginMode `json:"marginMode" msgpack:"marginMode"`
}

func (r RegisterAssetRequest2) validate() error {
	if err := validCoinPrice(r.Coin, r.OraclePx); err != nil {
		return err
	}
	if r.MarginTableID == 0 {
		return fmt.Errorf("margin table ID must be non-zero")
	}
	if !validMarginMode(r.MarginMode, true) {
		return fmt.Errorf("invalid margin mode")
	}
	return nil
}

type RegisterAsset struct {
	MaxGas       *uint64              `json:"maxGas,omitempty" msgpack:"maxGas,omitempty"`
	AssetRequest RegisterAssetRequest `json:"assetRequest" msgpack:"assetRequest"`
	DEX          string               `json:"dex" msgpack:"dex"`
	Schema       *PerpDexSchemaInput  `json:"schema,omitempty" msgpack:"schema,omitempty"`
}

func (RegisterAsset) perpDeployVariant()    {}
func (RegisterAsset) perpDeployKey() string { return "registerAsset" }
func (a RegisterAsset) validatePerpDeploy() error {
	if err := validDEX(a.DEX); err != nil {
		return err
	}
	if err := a.AssetRequest.validate(); err != nil {
		return err
	}
	if a.Schema != nil {
		return a.Schema.validate()
	}
	return nil
}

func (a RegisterAsset) MarshalMsgpack() ([]byte, error) {
	return marshalRegisterAsset(a.MaxGas, a.AssetRequest, a.DEX, a.Schema)
}
func (a RegisterAsset) MarshalJSON() ([]byte, error) {
	return json.Marshal(struct {
		MaxGas       *uint64              `json:"maxGas"`
		AssetRequest RegisterAssetRequest `json:"assetRequest"`
		DEX          string               `json:"dex"`
		Schema       *PerpDexSchemaInput  `json:"schema"`
	}{a.MaxGas, a.AssetRequest, a.DEX, a.Schema})
}

type RegisterAsset2 struct {
	MaxGas       *uint64               `json:"maxGas,omitempty" msgpack:"maxGas,omitempty"`
	AssetRequest RegisterAssetRequest2 `json:"assetRequest" msgpack:"assetRequest"`
	DEX          string                `json:"dex" msgpack:"dex"`
	Schema       *PerpDexSchemaInput   `json:"schema,omitempty" msgpack:"schema,omitempty"`
}

func (RegisterAsset2) perpDeployVariant()    {}
func (RegisterAsset2) perpDeployKey() string { return "registerAsset2" }
func (a RegisterAsset2) validatePerpDeploy() error {
	if err := validDEX(a.DEX); err != nil {
		return err
	}
	if err := a.AssetRequest.validate(); err != nil {
		return err
	}
	if a.Schema != nil {
		return a.Schema.validate()
	}
	return nil
}

func (a RegisterAsset2) MarshalMsgpack() ([]byte, error) {
	return marshalRegisterAsset(a.MaxGas, a.AssetRequest, a.DEX, a.Schema)
}
func (a RegisterAsset2) MarshalJSON() ([]byte, error) {
	return json.Marshal(struct {
		MaxGas       *uint64               `json:"maxGas"`
		AssetRequest RegisterAssetRequest2 `json:"assetRequest"`
		DEX          string                `json:"dex"`
		Schema       *PerpDexSchemaInput   `json:"schema"`
	}{a.MaxGas, a.AssetRequest, a.DEX, a.Schema})
}

func marshalRegisterAsset(maxGas *uint64, assetRequest any, dex string, schema *PerpDexSchemaInput) ([]byte, error) {
	// These keys remain present for nil values. That matches the official
	// Python SDK and makes the signed L1 action deterministic.
	return marshalMap(func(e *msgpack.Encoder) error {
		if err := e.EncodeMapLen(4); err != nil {
			return err
		}
		for _, field := range []struct {
			key string
			val any
		}{{"maxGas", maxGas}, {"assetRequest", assetRequest}, {"dex", dex}, {"schema", schema}} {
			if err := e.EncodeString(field.key); err != nil {
				return err
			}
			if err := e.Encode(field.val); err != nil {
				return err
			}
		}
		return nil
	})
}

type SetOracle struct {
	DEX             string         `json:"dex" msgpack:"dex"`
	OraclePxs       []StringPair   `json:"oraclePxs" msgpack:"oraclePxs"`
	MarkPxs         [][]StringPair `json:"markPxs" msgpack:"markPxs"`
	ExternalPerpPxs []StringPair   `json:"externalPerpPxs" msgpack:"externalPerpPxs"`
}

func (SetOracle) perpDeployVariant()    {}
func (SetOracle) perpDeployKey() string { return "setOracle" }
func (a SetOracle) validatePerpDeploy() error {
	if err := validDEX(a.DEX); err != nil {
		return err
	}
	if err := validSortedUnsignedDecimalPairs(a.OraclePxs); err != nil {
		return fmt.Errorf("oracle prices: %w", err)
	}
	if err := validSortedUnsignedDecimalPairs(a.ExternalPerpPxs); err != nil {
		return fmt.Errorf("external perp prices: %w", err)
	}
	for i, prices := range a.MarkPxs {
		if err := validSortedUnsignedDecimalPairs(prices); err != nil {
			return fmt.Errorf("mark prices %d: %w", i, err)
		}
	}
	return nil
}

type SetFundingMultipliers struct{ Values []StringPair }

func (SetFundingMultipliers) perpDeployVariant()    {}
func (SetFundingMultipliers) perpDeployKey() string { return "setFundingMultipliers" }
func (a SetFundingMultipliers) validatePerpDeploy() error {
	return validSortedUnsignedDecimalPairs(a.Values)
}
func (a SetFundingMultipliers) MarshalMsgpack() ([]byte, error) { return msgpack.Marshal(a.Values) }
func (a SetFundingMultipliers) MarshalJSON() ([]byte, error)    { return json.Marshal(a.Values) }

type SetFundingInterestRates struct{ Values []StringPair }

func (SetFundingInterestRates) perpDeployVariant()    {}
func (SetFundingInterestRates) perpDeployKey() string { return "setFundingInterestRates" }
func (a SetFundingInterestRates) validatePerpDeploy() error {
	if err := validSortedDecimalPairs(a.Values, false); err != nil {
		return err
	}
	max := decimal.RequireFromString("0.01")
	for _, p := range a.Values {
		d, _ := decimal.NewFromString(p.Value)
		if d.Abs().GreaterThan(max) {
			return fmt.Errorf("funding interest rate out of range")
		}
	}
	return nil
}
func (a SetFundingInterestRates) MarshalMsgpack() ([]byte, error) { return msgpack.Marshal(a.Values) }
func (a SetFundingInterestRates) MarshalJSON() ([]byte, error)    { return json.Marshal(a.Values) }

type HaltTrading struct {
	Coin     string `json:"coin" msgpack:"coin"`
	IsHalted bool   `json:"isHalted" msgpack:"isHalted"`
}

func (HaltTrading) perpDeployVariant()          {}
func (HaltTrading) perpDeployKey() string       { return "haltTrading" }
func (a HaltTrading) validatePerpDeploy() error { return validCoin(a.Coin) }

type SetMarginTableIDs struct{ Values []StringUintPair }

func (SetMarginTableIDs) perpDeployVariant()    {}
func (SetMarginTableIDs) perpDeployKey() string { return "setMarginTableIds" }
func (a SetMarginTableIDs) validatePerpDeploy() error {
	if err := validSortedUintPairs(a.Values); err != nil {
		return err
	}
	for _, v := range a.Values {
		if v.Value == 0 {
			return fmt.Errorf("margin table ID must be non-zero")
		}
	}
	return nil
}
func (a SetMarginTableIDs) MarshalMsgpack() ([]byte, error) { return msgpack.Marshal(a.Values) }
func (a SetMarginTableIDs) MarshalJSON() ([]byte, error)    { return json.Marshal(a.Values) }

type RawMarginTier struct {
	LowerBound  uint64 `json:"lowerBound" msgpack:"lowerBound"`
	MaxLeverage uint8  `json:"maxLeverage" msgpack:"maxLeverage"`
}
type RawMarginTable struct {
	Description string          `json:"description" msgpack:"description"`
	MarginTiers []RawMarginTier `json:"marginTiers" msgpack:"marginTiers"`
}
type InsertMarginTable struct {
	DEX         string         `json:"dex" msgpack:"dex"`
	MarginTable RawMarginTable `json:"marginTable" msgpack:"marginTable"`
}

func (InsertMarginTable) perpDeployVariant()    {}
func (InsertMarginTable) perpDeployKey() string { return "insertMarginTable" }
func (a InsertMarginTable) validatePerpDeploy() error {
	if err := validDEX(a.DEX); err != nil {
		return err
	}
	if len(a.MarginTable.MarginTiers) == 0 || len(a.MarginTable.MarginTiers) > 3 {
		return fmt.Errorf("margin table must contain one to three tiers")
	}
	if strings.TrimSpace(a.MarginTable.Description) == "" {
		return fmt.Errorf("margin table description is required")
	}
	var prior RawMarginTier
	for i, t := range a.MarginTable.MarginTiers {
		if t.MaxLeverage < 1 || t.MaxLeverage > 50 {
			return fmt.Errorf("max leverage out of range")
		}
		if i > 0 && (t.LowerBound <= prior.LowerBound || t.MaxLeverage > prior.MaxLeverage) {
			return fmt.Errorf("margin tiers must increase lower bound and decrease max leverage")
		}
		prior = t
	}
	return nil
}

type SetFeeRecipient struct {
	DEX          string `json:"dex" msgpack:"dex"`
	FeeRecipient string `json:"feeRecipient" msgpack:"feeRecipient"`
}

func (SetFeeRecipient) perpDeployVariant()    {}
func (SetFeeRecipient) perpDeployKey() string { return "setFeeRecipient" }
func (a SetFeeRecipient) validatePerpDeploy() error {
	if err := validDEX(a.DEX); err != nil {
		return err
	}
	if !common.IsHexAddress(a.FeeRecipient) {
		return fmt.Errorf("fee recipient is not an address")
	}
	return nil
}
func (a SetFeeRecipient) MarshalMsgpack() ([]byte, error) {
	return marshalMap(func(e *msgpack.Encoder) error {
		if err := e.EncodeMapLen(2); err != nil {
			return err
		}
		if err := e.EncodeString("dex"); err != nil {
			return err
		}
		if err := e.EncodeString(a.DEX); err != nil {
			return err
		}
		if err := e.EncodeString("feeRecipient"); err != nil {
			return err
		}
		return e.EncodeString(strings.ToLower(a.FeeRecipient))
	})
}
func (a SetFeeRecipient) MarshalJSON() ([]byte, error) {
	return json.Marshal(struct {
		DEX          string `json:"dex"`
		FeeRecipient string `json:"feeRecipient"`
	}{a.DEX, strings.ToLower(a.FeeRecipient)})
}

type SetOpenInterestCaps struct{ Values []StringUintPair }

func (SetOpenInterestCaps) perpDeployVariant()    {}
func (SetOpenInterestCaps) perpDeployKey() string { return "setOpenInterestCaps" }
func (a SetOpenInterestCaps) validatePerpDeploy() error {
	if err := validSortedUintPairs(a.Values); err != nil {
		return err
	}
	for _, v := range a.Values {
		if v.Value == 0 {
			return fmt.Errorf("open interest cap must be positive")
		}
	}
	return nil
}
func (a SetOpenInterestCaps) MarshalMsgpack() ([]byte, error) { return msgpack.Marshal(a.Values) }
func (a SetOpenInterestCaps) MarshalJSON() ([]byte, error)    { return json.Marshal(a.Values) }

type SubDeployerInput struct {
	Variant string `json:"variant" msgpack:"variant"`
	User    string `json:"user" msgpack:"user"`
	Allowed bool   `json:"allowed" msgpack:"allowed"`
}
type SetSubDeployers struct {
	DEX          string             `json:"dex" msgpack:"dex"`
	SubDeployers []SubDeployerInput `json:"subDeployers" msgpack:"subDeployers"`
}

func (SetSubDeployers) perpDeployVariant()    {}
func (SetSubDeployers) perpDeployKey() string { return "setSubDeployers" }
func (a SetSubDeployers) validatePerpDeploy() error {
	if err := validDEX(a.DEX); err != nil {
		return err
	}
	if len(a.SubDeployers) == 0 {
		return fmt.Errorf("sub deployers are required")
	}
	lastVariant, lastUser := "", ""
	for _, s := range a.SubDeployers {
		if strings.TrimSpace(s.Variant) == "" || !common.IsHexAddress(s.User) {
			return fmt.Errorf("sub deployer variant and address are required")
		}
		user := strings.ToLower(s.User)
		if lastVariant != "" && (s.Variant < lastVariant || (s.Variant == lastVariant && user <= lastUser)) {
			return fmt.Errorf("sub deployers must be strictly sorted by variant and user")
		}
		lastVariant, lastUser = s.Variant, user
	}
	return nil
}
func (s SubDeployerInput) MarshalMsgpack() ([]byte, error) {
	return marshalMap(func(e *msgpack.Encoder) error {
		if err := e.EncodeMapLen(3); err != nil {
			return err
		}
		for _, f := range []struct {
			k string
			v any
		}{{"variant", s.Variant}, {"user", strings.ToLower(s.User)}, {"allowed", s.Allowed}} {
			if err := e.EncodeString(f.k); err != nil {
				return err
			}
			if err := e.Encode(f.v); err != nil {
				return err
			}
		}
		return nil
	})
}
func (s SubDeployerInput) MarshalJSON() ([]byte, error) {
	return json.Marshal(struct {
		Variant string `json:"variant"`
		User    string `json:"user"`
		Allowed bool   `json:"allowed"`
	}{s.Variant, strings.ToLower(s.User), s.Allowed})
}

type CoinMarginMode struct {
	Coin string
	Mode MarginMode
}

func (p CoinMarginMode) MarshalMsgpack() ([]byte, error) {
	return marshalMap(func(e *msgpack.Encoder) error {
		if err := e.EncodeArrayLen(2); err != nil {
			return err
		}
		if err := e.EncodeString(p.Coin); err != nil {
			return err
		}
		return e.EncodeString(string(p.Mode))
	})
}
func (p CoinMarginMode) MarshalJSON() ([]byte, error) {
	return json.Marshal([2]string{p.Coin, string(p.Mode)})
}

type SetMarginModes struct{ Values []CoinMarginMode }

func (SetMarginModes) perpDeployVariant()    {}
func (SetMarginModes) perpDeployKey() string { return "setMarginModes" }
func (a SetMarginModes) validatePerpDeploy() error {
	last := ""
	for _, v := range a.Values {
		if err := validCoin(v.Coin); err != nil {
			return err
		}
		if !validMarginMode(v.Mode, false) {
			return fmt.Errorf("margin mode must be strictIsolated or noCross")
		}
		if last != "" && v.Coin <= last {
			return fmt.Errorf("margin modes must be strictly sorted")
		}
		last = v.Coin
	}
	return nil
}
func (a SetMarginModes) MarshalMsgpack() ([]byte, error) { return msgpack.Marshal(a.Values) }
func (a SetMarginModes) MarshalJSON() ([]byte, error)    { return json.Marshal(a.Values) }

type SetFeeScale struct {
	DEX   string `json:"dex" msgpack:"dex"`
	Scale string `json:"scale" msgpack:"scale"`
}

func (SetFeeScale) perpDeployVariant()    {}
func (SetFeeScale) perpDeployKey() string { return "setFeeScale" }
func (a SetFeeScale) validatePerpDeploy() error {
	if err := validDEX(a.DEX); err != nil {
		return err
	}
	d, err := decimal.NewFromString(a.Scale)
	if err != nil || d.IsNegative() || d.GreaterThan(decimal.NewFromInt(3)) {
		return fmt.Errorf("fee scale must be between 0 and 3")
	}
	return nil
}

type SetGrowthModes struct{ Values []StringBoolPair }

func (SetGrowthModes) perpDeployVariant()    {}
func (SetGrowthModes) perpDeployKey() string { return "setGrowthModes" }
func (a SetGrowthModes) validatePerpDeploy() error {
	last := ""
	for _, v := range a.Values {
		if err := validCoin(v.Key); err != nil {
			return err
		}
		if last != "" && v.Key <= last {
			return fmt.Errorf("growth modes must be strictly sorted")
		}
		last = v.Key
	}
	return nil
}
func (a SetGrowthModes) MarshalMsgpack() ([]byte, error) { return msgpack.Marshal(a.Values) }
func (a SetGrowthModes) MarshalJSON() ([]byte, error)    { return json.Marshal(a.Values) }

type SetPerpAnnotation struct {
	Coin        string   `json:"coin" msgpack:"coin"`
	Category    string   `json:"category" msgpack:"category"`
	Description string   `json:"description" msgpack:"description"`
	DisplayName *string  `json:"displayName" msgpack:"displayName"`
	Keywords    []string `json:"keywords" msgpack:"keywords"`
}

func (SetPerpAnnotation) perpDeployVariant()    {}
func (SetPerpAnnotation) perpDeployKey() string { return "setPerpAnnotation" }
func (a SetPerpAnnotation) validatePerpDeploy() error {
	if err := validCoin(a.Coin); err != nil {
		return err
	}
	if utf8.RuneCountInString(a.Category) > 15 || utf8.RuneCountInString(a.Description) > 400 {
		return fmt.Errorf("annotation category or description exceeds protocol limit")
	}
	for _, k := range a.Keywords {
		if strings.TrimSpace(k) == "" {
			return fmt.Errorf("annotation keyword is empty")
		}
	}
	return nil
}

// DisableDEX permanently disables the named HIP-3 DEX.
type DisableDEX string

func (DisableDEX) perpDeployVariant()          {}
func (DisableDEX) perpDeployKey() string       { return "disableDex" }
func (a DisableDEX) validatePerpDeploy() error { return validDEX(string(a)) }
func (a DisableDEX) MarshalMsgpack() ([]byte, error) {
	return marshalMap(func(e *msgpack.Encoder) error { return e.EncodeString(string(a)) })
}
func (a DisableDEX) MarshalJSON() ([]byte, error) { return json.Marshal(string(a)) }

// RegisterToken2 is phase one of a HIP-1/HIP-2 deployment.
type RegisterToken2 struct {
	Spec     TokenSpec `json:"spec" msgpack:"spec"`
	MaxGas   uint64    `json:"maxGas" msgpack:"maxGas"`
	FullName *string   `json:"fullName,omitempty" msgpack:"fullName,omitempty"`
}
type TokenSpec struct {
	Name        string `json:"name" msgpack:"name"`
	SzDecimals  uint8  `json:"szDecimals" msgpack:"szDecimals"`
	WeiDecimals uint8  `json:"weiDecimals" msgpack:"weiDecimals"`
}

func (RegisterToken2) spotDeployVariant()    {}
func (RegisterToken2) spotDeployKey() string { return "registerToken2" }
func (a RegisterToken2) validateSpotDeploy() error {
	if strings.TrimSpace(a.Spec.Name) == "" {
		return fmt.Errorf("token name is required")
	}
	return nil
}

type AddressWei struct {
	User string
	Wei  string
}

func (p AddressWei) MarshalMsgpack() ([]byte, error) {
	return marshalMap(func(e *msgpack.Encoder) error {
		if err := e.EncodeArrayLen(2); err != nil {
			return err
		}
		if err := e.EncodeString(strings.ToLower(p.User)); err != nil {
			return err
		}
		return e.EncodeString(p.Wei)
	})
}
func (p AddressWei) MarshalJSON() ([]byte, error) {
	return json.Marshal([2]string{strings.ToLower(p.User), p.Wei})
}

type TokenWei struct {
	Token uint64
	Wei   string
}

func (p TokenWei) MarshalMsgpack() ([]byte, error) {
	return marshalMap(func(e *msgpack.Encoder) error {
		if err := e.EncodeArrayLen(2); err != nil {
			return err
		}
		if err := e.EncodeUint(p.Token); err != nil {
			return err
		}
		return e.EncodeString(p.Wei)
	})
}
func (p TokenWei) MarshalJSON() ([]byte, error) { return json.Marshal([2]any{p.Token, p.Wei}) }

type AddressBlacklist struct {
	User        string
	Blacklisted bool
}

func (p AddressBlacklist) MarshalMsgpack() ([]byte, error) {
	return marshalMap(func(e *msgpack.Encoder) error {
		if err := e.EncodeArrayLen(2); err != nil {
			return err
		}
		if err := e.EncodeString(strings.ToLower(p.User)); err != nil {
			return err
		}
		return e.EncodeBool(p.Blacklisted)
	})
}
func (p AddressBlacklist) MarshalJSON() ([]byte, error) {
	return json.Marshal([2]any{strings.ToLower(p.User), p.Blacklisted})
}

type UserGenesis struct {
	Token               uint64             `json:"token" msgpack:"token"`
	UserAndWei          []AddressWei       `json:"userAndWei" msgpack:"userAndWei"`
	ExistingTokenAndWei []TokenWei         `json:"existingTokenAndWei" msgpack:"existingTokenAndWei"`
	BlacklistUsers      []AddressBlacklist `json:"blacklistUsers,omitempty" msgpack:"blacklistUsers,omitempty"`
}

func (UserGenesis) spotDeployVariant()    {}
func (UserGenesis) spotDeployKey() string { return "userGenesis" }
func (a UserGenesis) validateSpotDeploy() error {
	for _, p := range a.UserAndWei {
		if !common.IsHexAddress(p.User) || !unsignedDecimal(p.Wei) {
			return fmt.Errorf("invalid user genesis allocation")
		}
	}
	for _, p := range a.ExistingTokenAndWei {
		if !unsignedDecimal(p.Wei) {
			return fmt.Errorf("invalid existing token genesis allocation")
		}
	}
	for _, p := range a.BlacklistUsers {
		if !common.IsHexAddress(p.User) {
			return fmt.Errorf("invalid blacklist address")
		}
	}
	return nil
}

type Genesis struct {
	Token            uint64 `json:"token" msgpack:"token"`
	MaxSupply        string `json:"maxSupply" msgpack:"maxSupply"`
	NoHyperliquidity bool   `json:"noHyperliquidity,omitempty" msgpack:"noHyperliquidity,omitempty"`
}

func (Genesis) spotDeployVariant()    {}
func (Genesis) spotDeployKey() string { return "genesis" }
func (a Genesis) validateSpotDeploy() error {
	if !unsignedDecimal(a.MaxSupply) {
		return fmt.Errorf("max supply must be an unsigned decimal string")
	}
	return nil
}

type RegisterSpot struct {
	Tokens [2]uint64 `json:"tokens" msgpack:"tokens"`
}

func (RegisterSpot) spotDeployVariant()    {}
func (RegisterSpot) spotDeployKey() string { return "registerSpot" }
func (a RegisterSpot) validateSpotDeploy() error {
	if a.Tokens[0] == a.Tokens[1] {
		return fmt.Errorf("base and quote token must differ")
	}
	return nil
}

type RegisterHyperliquidity struct {
	Spot          uint64  `json:"spot" msgpack:"spot"`
	StartPx       string  `json:"startPx" msgpack:"startPx"`
	OrderSz       string  `json:"orderSz" msgpack:"orderSz"`
	NOrders       uint64  `json:"nOrders" msgpack:"nOrders"`
	NSeededLevels *uint64 `json:"nSeededLevels,omitempty" msgpack:"nSeededLevels,omitempty"`
}

func (RegisterHyperliquidity) spotDeployVariant()    {}
func (RegisterHyperliquidity) spotDeployKey() string { return "registerHyperliquidity" }
func (a RegisterHyperliquidity) validateSpotDeploy() error {
	if !positiveDecimal(a.StartPx) || !positiveDecimal(a.OrderSz) {
		return fmt.Errorf("start price and order size must be positive decimal strings")
	}
	return nil
}

type SetDeployerTradingFeeShare struct {
	Token uint64 `json:"token" msgpack:"token"`
	Share string `json:"share" msgpack:"share"`
}

func (SetDeployerTradingFeeShare) spotDeployVariant()    {}
func (SetDeployerTradingFeeShare) spotDeployKey() string { return "setDeployerTradingFeeShare" }
func (a SetDeployerTradingFeeShare) validateSpotDeploy() error {
	if !strings.HasSuffix(a.Share, "%") {
		return fmt.Errorf("fee share must be a percentage string")
	}
	d, err := decimal.NewFromString(strings.TrimSuffix(a.Share, "%"))
	if err != nil || d.IsNegative() || d.GreaterThan(decimal.NewFromInt(100)) {
		return fmt.Errorf("fee share must be between 0%% and 100%%")
	}
	return nil
}

type EnableQuoteToken struct {
	Token uint64 `json:"token" msgpack:"token"`
}

func (EnableQuoteToken) spotDeployVariant()        {}
func (EnableQuoteToken) spotDeployKey() string     { return "enableQuoteToken" }
func (EnableQuoteToken) validateSpotDeploy() error { return nil }

type DisableQuoteToken struct {
	Token uint64 `json:"token" msgpack:"token"`
}

func (DisableQuoteToken) spotDeployVariant()        {}
func (DisableQuoteToken) spotDeployKey() string     { return "disableQuoteToken" }
func (DisableQuoteToken) validateSpotDeploy() error { return nil }

// RequestEVMContract requests an ERC-20 link for a HIP-1 token. The EVM
// contract is finalized separately by the EVM deployer.
type RequestEVMContract struct {
	Token               uint64 `json:"token" msgpack:"token"`
	Address             string `json:"address" msgpack:"address"`
	EVMExtraWeiDecimals int8   `json:"evmExtraWeiDecimals" msgpack:"evmExtraWeiDecimals"`
}

func (RequestEVMContract) spotDeployVariant()    {}
func (RequestEVMContract) spotDeployKey() string { return "requestEvmContract" }
func (a RequestEVMContract) validateSpotDeploy() error {
	if !common.IsHexAddress(a.Address) {
		return fmt.Errorf("EVM contract address is invalid")
	}
	if a.EVMExtraWeiDecimals < -2 || a.EVMExtraWeiDecimals > 18 {
		return fmt.Errorf("EVM extra wei decimals must be between -2 and 18")
	}
	return nil
}
func (a RequestEVMContract) MarshalMsgpack() ([]byte, error) {
	return marshalMap(func(e *msgpack.Encoder) error {
		if err := e.EncodeMapLen(3); err != nil {
			return err
		}
		for _, f := range []struct {
			k string
			v any
		}{{"token", a.Token}, {"address", strings.ToLower(a.Address)}, {"evmExtraWeiDecimals", a.EVMExtraWeiDecimals}} {
			if err := e.EncodeString(f.k); err != nil {
				return err
			}
			if err := e.Encode(f.v); err != nil {
				return err
			}
		}
		return nil
	})
}
func (a RequestEVMContract) MarshalJSON() ([]byte, error) {
	return json.Marshal(struct {
		Token               uint64 `json:"token"`
		Address             string `json:"address"`
		EVMExtraWeiDecimals int8   `json:"evmExtraWeiDecimals"`
	}{a.Token, strings.ToLower(a.Address), a.EVMExtraWeiDecimals})
}

type EnableAlignedQuoteToken struct {
	Token uint64 `json:"token" msgpack:"token"`
}

func (EnableAlignedQuoteToken) spotDeployVariant()        {}
func (EnableAlignedQuoteToken) spotDeployKey() string     { return "enableAlignedQuoteToken" }
func (EnableAlignedQuoteToken) validateSpotDeploy() error { return nil }

// The following three variants are implemented because the official Python SDK
// currently exposes them, although the HIP-1/HIP-2 deployment overview focuses
// on the eight variants above.
type EnableFreezePrivilege struct {
	Token uint64 `json:"token" msgpack:"token"`
}

func (EnableFreezePrivilege) spotDeployVariant()        {}
func (EnableFreezePrivilege) spotDeployKey() string     { return "enableFreezePrivilege" }
func (EnableFreezePrivilege) validateSpotDeploy() error { return nil }

type FreezeUser struct {
	Token  uint64 `json:"token" msgpack:"token"`
	User   string `json:"user" msgpack:"user"`
	Freeze bool   `json:"freeze" msgpack:"freeze"`
}

func (FreezeUser) spotDeployVariant()    {}
func (FreezeUser) spotDeployKey() string { return "freezeUser" }
func (a FreezeUser) validateSpotDeploy() error {
	if !common.IsHexAddress(a.User) {
		return fmt.Errorf("freeze user is not an address")
	}
	return nil
}
func (a FreezeUser) MarshalMsgpack() ([]byte, error) {
	return marshalMap(func(e *msgpack.Encoder) error {
		if err := e.EncodeMapLen(3); err != nil {
			return err
		}
		for _, f := range []struct {
			k string
			v any
		}{{"token", a.Token}, {"user", strings.ToLower(a.User)}, {"freeze", a.Freeze}} {
			if err := e.EncodeString(f.k); err != nil {
				return err
			}
			if err := e.Encode(f.v); err != nil {
				return err
			}
		}
		return nil
	})
}
func (a FreezeUser) MarshalJSON() ([]byte, error) {
	return json.Marshal(struct {
		Token  uint64 `json:"token"`
		User   string `json:"user"`
		Freeze bool   `json:"freeze"`
	}{a.Token, strings.ToLower(a.User), a.Freeze})
}

type RevokeFreezePrivilege struct {
	Token uint64 `json:"token" msgpack:"token"`
}

func (RevokeFreezePrivilege) spotDeployVariant()        {}
func (RevokeFreezePrivilege) spotDeployKey() string     { return "revokeFreezePrivilege" }
func (RevokeFreezePrivilege) validateSpotDeploy() error { return nil }

func validDEX(dex string) error {
	if strings.TrimSpace(dex) == "" {
		return fmt.Errorf("dex is required")
	}
	return nil
}
func validCoin(coin string) error {
	if strings.TrimSpace(coin) == "" {
		return fmt.Errorf("coin is required")
	}
	return nil
}
func validCoinPrice(coin, price string) error {
	if err := validCoin(coin); err != nil {
		return err
	}
	if !positiveDecimal(price) {
		return fmt.Errorf("oracle price must be a positive decimal string")
	}
	return nil
}
func positiveDecimal(s string) bool {
	d, err := decimal.NewFromString(s)
	return err == nil && d.IsPositive()
}
func unsignedDecimal(s string) bool {
	d, err := decimal.NewFromString(s)
	return err == nil && !d.IsNegative()
}
func lowerAddressPtr(address *string) *string {
	if address == nil {
		return nil
	}
	normalized := strings.ToLower(*address)
	return &normalized
}
func validSortedDecimalPairs(values []StringPair, nonEmpty bool) error {
	if nonEmpty && len(values) == 0 {
		return fmt.Errorf("values are required")
	}
	last := ""
	for _, v := range values {
		if err := validCoin(v.Key); err != nil {
			return err
		}
		if _, err := decimal.NewFromString(v.Value); err != nil {
			return fmt.Errorf("%s is not a decimal", v.Key)
		}
		if last != "" && v.Key <= last {
			return fmt.Errorf("tuples must be lexicographically sorted with no duplicates")
		}
		last = v.Key
	}
	return nil
}
func validSortedUnsignedDecimalPairs(values []StringPair) error {
	last := ""
	for _, v := range values {
		if err := validCoin(v.Key); err != nil {
			return err
		}
		if !unsignedDecimal(v.Value) {
			return fmt.Errorf("%s is not an unsigned decimal", v.Key)
		}
		if last != "" && v.Key <= last {
			return fmt.Errorf("tuples must be lexicographically sorted with no duplicates")
		}
		last = v.Key
	}
	return nil
}
func validSortedUintPairs(values []StringUintPair) error {
	last := ""
	for _, v := range values {
		if err := validCoin(v.Key); err != nil {
			return err
		}
		if last != "" && v.Key <= last {
			return fmt.Errorf("tuples must be lexicographically sorted with no duplicates")
		}
		last = v.Key
	}
	return nil
}

// SortStringPairs is supplied for callers preparing official tuple-list input.
// It returns a copy; callers must still choose the values deliberately.
func SortStringPairs(values []StringPair) []StringPair {
	out := append([]StringPair(nil), values...)
	sort.Slice(out, func(i, j int) bool { return out[i].Key < out[j].Key })
	return out
}
