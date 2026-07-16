// Package signing constructs Hyperliquid protocol digests. It never owns keys
// and always hands the final 32-byte digest to signer.DigestSigner.
package signing

import (
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"fmt"

	"github.com/Apexllcc/hyperliquid-go-sdk/signer"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/common/math"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/ethereum/go-ethereum/signer/core/apitypes"
	"github.com/vmihailenco/msgpack/v5"
)

// L1Components exposes intermediate bytes for fixed protocol test vectors.
type L1Components struct {
	ActionBytes  []byte
	ConnectionID [32]byte
}

// L1ActionComponents performs Hyperliquid's exact L1 action hashing: MessagePack
// action bytes, big-endian nonce, vault marker/value and optional expiry marker.
func L1ActionComponents(action any, nonce uint64, vaultAddress *common.Address, expiresAfter *uint64) (L1Components, error) {
	actionBytes, err := msgpack.Marshal(action)
	if err != nil {
		return L1Components{}, fmt.Errorf("encode L1 action: %w", err)
	}
	data := make([]byte, 0, len(actionBytes)+8+1+20+9)
	data = append(data, actionBytes...)
	var nonceBytes [8]byte
	binary.BigEndian.PutUint64(nonceBytes[:], nonce)
	data = append(data, nonceBytes[:]...)
	if vaultAddress == nil {
		data = append(data, 0)
	} else {
		data = append(data, 1)
		data = append(data, vaultAddress.Bytes()...)
	}
	if expiresAfter != nil {
		data = append(data, 0)
		var expiresBytes [8]byte
		binary.BigEndian.PutUint64(expiresBytes[:], *expiresAfter)
		data = append(data, expiresBytes[:]...)
	}
	var connectionID [32]byte
	copy(connectionID[:], crypto.Keccak256(data))
	return L1Components{actionBytes, connectionID}, nil
}

// ComputeL1ActionDigest returns the final EIP-712 Agent digest for an L1 action.
func ComputeL1ActionDigest(action any, nonce uint64, vaultAddress *common.Address, expiresAfter *uint64, isMainnet bool) (signer.Digest, error) {
	components, err := L1ActionComponents(action, nonce, vaultAddress, expiresAfter)
	if err != nil {
		return signer.Digest{}, err
	}
	source := "b"
	if isMainnet {
		source = "a"
	}
	chainID := math.NewHexOrDecimal256(1337)
	data := apitypes.TypedData{Types: apitypes.Types{"EIP712Domain": {{Name: "name", Type: "string"}, {Name: "version", Type: "string"}, {Name: "chainId", Type: "uint256"}, {Name: "verifyingContract", Type: "address"}}, "Agent": {{Name: "source", Type: "string"}, {Name: "connectionId", Type: "bytes32"}}}, PrimaryType: "Agent", Domain: apitypes.TypedDataDomain{Name: "Exchange", Version: "1", ChainId: chainID, VerifyingContract: "0x0000000000000000000000000000000000000000"}, Message: apitypes.TypedDataMessage{"source": source, "connectionId": "0x" + hex.EncodeToString(components.ConnectionID[:])}}
	raw, _, err := apitypes.TypedDataAndHash(data)
	if err != nil {
		return signer.Digest{}, fmt.Errorf("hash L1 typed data: %w", err)
	}
	var digest signer.Digest
	copy(digest[:], raw)
	return digest, nil
}

// CancelAction is the canonical L1 cancel action.
type CancelAction struct{ Cancels []CancelWire }

func (a CancelAction) MarshalMsgpack() ([]byte, error) {
	return marshalMap(func(e *msgpack.Encoder) error {
		if err := e.EncodeMapLen(2); err != nil {
			return err
		}
		if err := e.EncodeString("type"); err != nil {
			return err
		}
		if err := e.EncodeString("cancel"); err != nil {
			return err
		}
		if err := e.EncodeString("cancels"); err != nil {
			return err
		}
		return e.Encode(a.Cancels)
	})
}

// CancelWire identifies an asset and order ID.
type CancelWire struct {
	Asset int
	OID   uint64
}

func (c CancelWire) MarshalMsgpack() ([]byte, error) {
	return marshalMap(func(e *msgpack.Encoder) error {
		if err := e.EncodeMapLen(2); err != nil {
			return err
		}
		if err := e.EncodeString("a"); err != nil {
			return err
		}
		if err := e.EncodeInt(int64(c.Asset)); err != nil {
			return err
		}
		if err := e.EncodeString("o"); err != nil {
			return err
		}
		return e.EncodeUint(c.OID)
	})
}

// OrderAction is the canonical L1 order action.
type OrderAction struct {
	Orders   []OrderWire
	Grouping string
	Builder  *BuilderWire
}

func (a OrderAction) MarshalMsgpack() ([]byte, error) {
	return marshalMap(func(e *msgpack.Encoder) error {
		length := 3
		if a.Builder != nil {
			length++
		}
		if err := e.EncodeMapLen(length); err != nil {
			return err
		}
		for _, pair := range []struct {
			k string
			v any
		}{{"type", "order"}, {"orders", a.Orders}, {"grouping", a.Grouping}} {
			if err := e.EncodeString(pair.k); err != nil {
				return err
			}
			if err := e.Encode(pair.v); err != nil {
				return err
			}
		}
		if a.Builder != nil {
			if err := e.EncodeString("builder"); err != nil {
				return err
			}
			return e.Encode(a.Builder)
		}
		return nil
	})
}

// BuilderWire describes an optional builder fee, in tenths of a basis point.
type BuilderWire struct {
	Address string
	Fee     uint64
}

func (b BuilderWire) MarshalMsgpack() ([]byte, error) {
	return marshalMap(func(e *msgpack.Encoder) error {
		if err := e.EncodeMapLen(2); err != nil {
			return err
		}
		if err := e.EncodeString("b"); err != nil {
			return err
		}
		if err := e.EncodeString(b.Address); err != nil {
			return err
		}
		if err := e.EncodeString("f"); err != nil {
			return err
		}
		return e.EncodeUint(b.Fee)
	})
}

// OrderWire contains precision-safe strings expected by the L1 order protocol.
type OrderWire struct {
	Asset      int
	IsBuy      bool
	Price      string
	Size       string
	ReduceOnly bool
	Type       OrderTypeWire
	Cloid      *string
}

func (o OrderWire) MarshalMsgpack() ([]byte, error) {
	return marshalMap(func(e *msgpack.Encoder) error {
		n := 6
		if o.Cloid != nil {
			n++
		}
		if err := e.EncodeMapLen(n); err != nil {
			return err
		}
		values := []struct {
			k string
			v any
		}{{"a", o.Asset}, {"b", o.IsBuy}, {"p", o.Price}, {"s", o.Size}, {"r", o.ReduceOnly}, {"t", o.Type}}
		for _, p := range values {
			if err := e.EncodeString(p.k); err != nil {
				return err
			}
			if err := e.Encode(p.v); err != nil {
				return err
			}
		}
		if o.Cloid != nil {
			if err := e.EncodeString("c"); err != nil {
				return err
			}
			return e.Encode(*o.Cloid)
		}
		return nil
	})
}

// OrderTypeWire is a canonical L1 order type payload.
type OrderTypeWire interface{ isOrderTypeWire() }

// LimitOrderType is the L1 wire representation of a limit order.
type LimitOrderType struct{ TIF string }

func (LimitOrderType) isOrderTypeWire() {}

func (l LimitOrderType) MarshalMsgpack() ([]byte, error) {
	return marshalMap(func(e *msgpack.Encoder) error {
		if err := e.EncodeMapLen(1); err != nil {
			return err
		}
		if err := e.EncodeString("limit"); err != nil {
			return err
		}
		if err := e.EncodeMapLen(1); err != nil {
			return err
		}
		if err := e.EncodeString("tif"); err != nil {
			return err
		}
		return e.EncodeString(l.TIF)
	})
}

// TriggerOrderType is the L1 wire representation of a take-profit or stop-loss
// market or limit order.
type TriggerOrderType struct {
	IsMarket  bool
	TriggerPx string
	TPSL      string
}

func (TriggerOrderType) isOrderTypeWire() {}

func (t TriggerOrderType) MarshalMsgpack() ([]byte, error) {
	return marshalMap(func(e *msgpack.Encoder) error {
		if err := e.EncodeMapLen(1); err != nil {
			return err
		}
		if err := e.EncodeString("trigger"); err != nil {
			return err
		}
		if err := e.EncodeMapLen(3); err != nil {
			return err
		}
		for _, pair := range []struct {
			k string
			v any
		}{{"isMarket", t.IsMarket}, {"triggerPx", t.TriggerPx}, {"tpsl", t.TPSL}} {
			if err := e.EncodeString(pair.k); err != nil {
				return err
			}
			if err := e.Encode(pair.v); err != nil {
				return err
			}
		}
		return nil
	})
}
func (a CancelAction) MarshalJSON() ([]byte, error) {
	return json.Marshal(struct {
		Type    string       `json:"type"`
		Cancels []CancelWire `json:"cancels"`
	}{"cancel", a.Cancels})
}
func (c CancelWire) MarshalJSON() ([]byte, error) {
	return json.Marshal(struct {
		Asset int    `json:"a"`
		OID   uint64 `json:"o"`
	}{c.Asset, c.OID})
}
func (a OrderAction) MarshalJSON() ([]byte, error) {
	return json.Marshal(struct {
		Type     string       `json:"type"`
		Orders   []OrderWire  `json:"orders"`
		Grouping string       `json:"grouping"`
		Builder  *BuilderWire `json:"builder,omitempty"`
	}{"order", a.Orders, a.Grouping, a.Builder})
}
func (b BuilderWire) MarshalJSON() ([]byte, error) {
	return json.Marshal(struct {
		Address string `json:"b"`
		Fee     uint64 `json:"f"`
	}{b.Address, b.Fee})
}
func (o OrderWire) MarshalJSON() ([]byte, error) {
	return json.Marshal(struct {
		Asset      int           `json:"a"`
		IsBuy      bool          `json:"b"`
		Price      string        `json:"p"`
		Size       string        `json:"s"`
		ReduceOnly bool          `json:"r"`
		Type       OrderTypeWire `json:"t"`
		Cloid      *string       `json:"c,omitempty"`
	}{o.Asset, o.IsBuy, o.Price, o.Size, o.ReduceOnly, o.Type, o.Cloid})
}
func (l LimitOrderType) MarshalJSON() ([]byte, error) {
	return json.Marshal(map[string]any{"limit": map[string]string{"tif": l.TIF}})
}
func (t TriggerOrderType) MarshalJSON() ([]byte, error) {
	return json.Marshal(map[string]any{"trigger": struct {
		IsMarket  bool   `json:"isMarket"`
		TriggerPx string `json:"triggerPx"`
		TPSL      string `json:"tpsl"`
	}{t.IsMarket, t.TriggerPx, t.TPSL}})
}

type CancelByCloidAction struct{ Cancels []CancelByCloidWire }
type CancelByCloidWire struct {
	Asset int
	Cloid string
}

func (a CancelByCloidAction) MarshalMsgpack() ([]byte, error) {
	return marshalMap(func(e *msgpack.Encoder) error {
		if err := e.EncodeMapLen(2); err != nil {
			return err
		}
		if err := e.EncodeString("type"); err != nil {
			return err
		}
		if err := e.EncodeString("cancelByCloid"); err != nil {
			return err
		}
		if err := e.EncodeString("cancels"); err != nil {
			return err
		}
		return e.Encode(a.Cancels)
	})
}
func (c CancelByCloidWire) MarshalMsgpack() ([]byte, error) {
	return marshalMap(func(e *msgpack.Encoder) error {
		if err := e.EncodeMapLen(2); err != nil {
			return err
		}
		if err := e.EncodeString("asset"); err != nil {
			return err
		}
		if err := e.EncodeInt(int64(c.Asset)); err != nil {
			return err
		}
		if err := e.EncodeString("cloid"); err != nil {
			return err
		}
		return e.EncodeString(c.Cloid)
	})
}
func (a CancelByCloidAction) MarshalJSON() ([]byte, error) {
	return json.Marshal(struct {
		Type    string              `json:"type"`
		Cancels []CancelByCloidWire `json:"cancels"`
	}{"cancelByCloid", a.Cancels})
}
func (c CancelByCloidWire) MarshalJSON() ([]byte, error) {
	return json.Marshal(struct {
		Asset int    `json:"asset"`
		Cloid string `json:"cloid"`
	}{c.Asset, c.Cloid})
}

type ScheduleCancelAction struct{ Time *uint64 }

func (a ScheduleCancelAction) MarshalMsgpack() ([]byte, error) {
	return marshalMap(func(e *msgpack.Encoder) error {
		n := 1
		if a.Time != nil {
			n++
		}
		if err := e.EncodeMapLen(n); err != nil {
			return err
		}
		if err := e.EncodeString("type"); err != nil {
			return err
		}
		if err := e.EncodeString("scheduleCancel"); err != nil {
			return err
		}
		if a.Time != nil {
			if err := e.EncodeString("time"); err != nil {
				return err
			}
			return e.EncodeUint(*a.Time)
		}
		return nil
	})
}
func (a ScheduleCancelAction) MarshalJSON() ([]byte, error) {
	return json.Marshal(struct {
		Type string  `json:"type"`
		Time *uint64 `json:"time,omitempty"`
	}{"scheduleCancel", a.Time})
}

// TWAPOrderAction starts a protocol-native time-weighted average price order.
type TWAPOrderAction struct{ TWAP TWAPWire }

// TWAPWire contains the exact compact L1 fields used by the Exchange protocol.
// Size is a canonical decimal string, never a binary floating point value.
type TWAPWire struct {
	Asset      int
	IsBuy      bool
	Size       string
	ReduceOnly bool
	Minutes    uint64
	Randomize  bool
}

func (a TWAPOrderAction) MarshalMsgpack() ([]byte, error) {
	return marshalMap(func(e *msgpack.Encoder) error {
		if err := e.EncodeMapLen(2); err != nil {
			return err
		}
		if err := e.EncodeString("type"); err != nil {
			return err
		}
		if err := e.EncodeString("twapOrder"); err != nil {
			return err
		}
		if err := e.EncodeString("twap"); err != nil {
			return err
		}
		return e.Encode(a.TWAP)
	})
}

func (a TWAPOrderAction) MarshalJSON() ([]byte, error) {
	return json.Marshal(struct {
		Type string   `json:"type"`
		TWAP TWAPWire `json:"twap"`
	}{Type: "twapOrder", TWAP: a.TWAP})
}

func (t TWAPWire) MarshalMsgpack() ([]byte, error) {
	return marshalMap(func(e *msgpack.Encoder) error {
		if err := e.EncodeMapLen(6); err != nil {
			return err
		}
		for _, pair := range []struct {
			key   string
			value any
		}{
			{"a", t.Asset},
			{"b", t.IsBuy},
			{"s", t.Size},
			{"r", t.ReduceOnly},
			{"m", t.Minutes},
			{"t", t.Randomize},
		} {
			if err := e.EncodeString(pair.key); err != nil {
				return err
			}
			if pair.key == "m" {
				if err := encodeCompactUint(e, t.Minutes); err != nil {
					return err
				}
				continue
			}
			if err := e.Encode(pair.value); err != nil {
				return err
			}
		}
		return nil
	})
}

func (t TWAPWire) MarshalJSON() ([]byte, error) {
	return json.Marshal(struct {
		Asset      int    `json:"a"`
		IsBuy      bool   `json:"b"`
		Size       string `json:"s"`
		ReduceOnly bool   `json:"r"`
		Minutes    uint64 `json:"m"`
		Randomize  bool   `json:"t"`
	}{t.Asset, t.IsBuy, t.Size, t.ReduceOnly, t.Minutes, t.Randomize})
}

// TWAPCancelAction cancels a native TWAP by asset and TWAP ID.
type TWAPCancelAction struct {
	Asset  int
	TWAPID uint64
}

func (a TWAPCancelAction) MarshalMsgpack() ([]byte, error) {
	return marshalMap(func(e *msgpack.Encoder) error {
		if err := e.EncodeMapLen(3); err != nil {
			return err
		}
		for _, pair := range []struct {
			key   string
			value any
		}{{"type", "twapCancel"}, {"a", a.Asset}, {"t", a.TWAPID}} {
			if err := e.EncodeString(pair.key); err != nil {
				return err
			}
			if pair.key == "t" {
				if err := encodeCompactUint(e, a.TWAPID); err != nil {
					return err
				}
				continue
			}
			if err := e.Encode(pair.value); err != nil {
				return err
			}
		}
		return nil
	})
}

// encodeCompactUint follows the integer representation produced by Python's
// msgpack.packb for non-negative protocol numbers.
func encodeCompactUint(e *msgpack.Encoder, value uint64) error {
	if value <= uint64(^uint64(0)>>1) {
		return e.EncodeInt(int64(value))
	}
	return e.EncodeUint(value)
}

func (a TWAPCancelAction) MarshalJSON() ([]byte, error) {
	return json.Marshal(struct {
		Type   string `json:"type"`
		Asset  int    `json:"a"`
		TWAPID uint64 `json:"t"`
	}{"twapCancel", a.Asset, a.TWAPID})
}

// SubaccountTransferAction moves whole-USDC units between a master account and
// one subaccount. It is an L1 action and is always signed by the master.
type SubaccountTransferAction struct {
	SubaccountUser string
	IsDeposit      bool
	USD            uint64
}

func (a SubaccountTransferAction) MarshalMsgpack() ([]byte, error) {
	return marshalMap(func(e *msgpack.Encoder) error {
		if err := e.EncodeMapLen(4); err != nil {
			return err
		}
		for _, pair := range []struct {
			k string
			v any
		}{{"type", "subAccountTransfer"}, {"subAccountUser", a.SubaccountUser}, {"isDeposit", a.IsDeposit}} {
			if err := e.EncodeString(pair.k); err != nil {
				return err
			}
			if err := e.Encode(pair.v); err != nil {
				return err
			}
		}
		if err := e.EncodeString("usd"); err != nil {
			return err
		}
		return e.EncodeUint(a.USD)
	})
}
func (a SubaccountTransferAction) MarshalJSON() ([]byte, error) {
	return json.Marshal(struct {
		Type           string `json:"type"`
		SubaccountUser string `json:"subAccountUser"`
		IsDeposit      bool   `json:"isDeposit"`
		USD            uint64 `json:"usd"`
	}{"subAccountTransfer", a.SubaccountUser, a.IsDeposit, a.USD})
}

// SubaccountSpotTransferAction moves a precision-safe spot token amount
// between a master account and one subaccount.
type SubaccountSpotTransferAction struct {
	SubaccountUser, Token, Amount string
	IsDeposit                     bool
}

func (a SubaccountSpotTransferAction) MarshalMsgpack() ([]byte, error) {
	return marshalMap(func(e *msgpack.Encoder) error {
		if err := e.EncodeMapLen(5); err != nil {
			return err
		}
		for _, pair := range []struct {
			k string
			v any
		}{{"type", "subAccountSpotTransfer"}, {"subAccountUser", a.SubaccountUser}, {"isDeposit", a.IsDeposit}, {"token", a.Token}, {"amount", a.Amount}} {
			if err := e.EncodeString(pair.k); err != nil {
				return err
			}
			if err := e.Encode(pair.v); err != nil {
				return err
			}
		}
		return nil
	})
}
func (a SubaccountSpotTransferAction) MarshalJSON() ([]byte, error) {
	return json.Marshal(struct {
		Type           string `json:"type"`
		SubaccountUser string `json:"subAccountUser"`
		IsDeposit      bool   `json:"isDeposit"`
		Token          string `json:"token"`
		Amount         string `json:"amount"`
	}{"subAccountSpotTransfer", a.SubaccountUser, a.IsDeposit, a.Token, a.Amount})
}

// VaultTransferAction deposits to or withdraws whole-USDC units from a vault.
type VaultTransferAction struct {
	VaultAddress string
	IsDeposit    bool
	USD          uint64
}

func (a VaultTransferAction) MarshalMsgpack() ([]byte, error) {
	return marshalMap(func(e *msgpack.Encoder) error {
		if err := e.EncodeMapLen(4); err != nil {
			return err
		}
		for _, pair := range []struct {
			k string
			v any
		}{{"type", "vaultTransfer"}, {"vaultAddress", a.VaultAddress}, {"isDeposit", a.IsDeposit}} {
			if err := e.EncodeString(pair.k); err != nil {
				return err
			}
			if err := e.Encode(pair.v); err != nil {
				return err
			}
		}
		if err := e.EncodeString("usd"); err != nil {
			return err
		}
		return e.EncodeUint(a.USD)
	})
}
func (a VaultTransferAction) MarshalJSON() ([]byte, error) {
	return json.Marshal(struct {
		Type         string `json:"type"`
		VaultAddress string `json:"vaultAddress"`
		IsDeposit    bool   `json:"isDeposit"`
		USD          uint64 `json:"usd"`
	}{"vaultTransfer", a.VaultAddress, a.IsDeposit, a.USD})
}

type BatchModifyAction struct{ Modifies []ModifyWire }
type ModifyWire struct {
	OID   uint64
	Cloid *string
	Order OrderWire
}

func (a BatchModifyAction) MarshalMsgpack() ([]byte, error) {
	return marshalMap(func(e *msgpack.Encoder) error {
		if err := e.EncodeMapLen(2); err != nil {
			return err
		}
		if err := e.EncodeString("type"); err != nil {
			return err
		}
		if err := e.EncodeString("batchModify"); err != nil {
			return err
		}
		if err := e.EncodeString("modifies"); err != nil {
			return err
		}
		return e.Encode(a.Modifies)
	})
}
func (m ModifyWire) MarshalMsgpack() ([]byte, error) {
	return marshalMap(func(e *msgpack.Encoder) error {
		if err := e.EncodeMapLen(2); err != nil {
			return err
		}
		if err := e.EncodeString("oid"); err != nil {
			return err
		}
		if m.Cloid != nil {
			if err := e.EncodeString(*m.Cloid); err != nil {
				return err
			}
		} else {
			if err := e.EncodeUint(m.OID); err != nil {
				return err
			}
		}
		if err := e.EncodeString("order"); err != nil {
			return err
		}
		return e.Encode(m.Order)
	})
}
func (a BatchModifyAction) MarshalJSON() ([]byte, error) {
	return json.Marshal(struct {
		Type     string       `json:"type"`
		Modifies []ModifyWire `json:"modifies"`
	}{"batchModify", a.Modifies})
}

// UpdateLeverageAction changes a perpetual asset's cross or isolated leverage.
type UpdateLeverageAction struct {
	Asset    int
	IsCross  bool
	Leverage uint64
}

func (a UpdateLeverageAction) MarshalMsgpack() ([]byte, error) {
	return marshalMap(func(e *msgpack.Encoder) error {
		if err := e.EncodeMapLen(4); err != nil {
			return err
		}
		if err := e.EncodeString("type"); err != nil {
			return err
		}
		if err := e.EncodeString("updateLeverage"); err != nil {
			return err
		}
		if err := e.EncodeString("asset"); err != nil {
			return err
		}
		if err := e.EncodeInt(int64(a.Asset)); err != nil {
			return err
		}
		if err := e.EncodeString("isCross"); err != nil {
			return err
		}
		if err := e.EncodeBool(a.IsCross); err != nil {
			return err
		}
		if err := e.EncodeString("leverage"); err != nil {
			return err
		}
		return e.EncodeUint(a.Leverage)
	})
}

func (a UpdateLeverageAction) MarshalJSON() ([]byte, error) {
	return json.Marshal(struct {
		Type     string `json:"type"`
		Asset    int    `json:"asset"`
		IsCross  bool   `json:"isCross"`
		Leverage uint64 `json:"leverage"`
	}{"updateLeverage", a.Asset, a.IsCross, a.Leverage})
}

// UpdateIsolatedMarginAction adjusts isolated margin in protocol USDC micros.
type UpdateIsolatedMarginAction struct {
	Asset int
	IsBuy bool
	NTLI  int64
}

func (a UpdateIsolatedMarginAction) MarshalMsgpack() ([]byte, error) {
	return marshalMap(func(e *msgpack.Encoder) error {
		if err := e.EncodeMapLen(4); err != nil {
			return err
		}
		if err := e.EncodeString("type"); err != nil {
			return err
		}
		if err := e.EncodeString("updateIsolatedMargin"); err != nil {
			return err
		}
		if err := e.EncodeString("asset"); err != nil {
			return err
		}
		if err := e.EncodeInt(int64(a.Asset)); err != nil {
			return err
		}
		if err := e.EncodeString("isBuy"); err != nil {
			return err
		}
		if err := e.EncodeBool(a.IsBuy); err != nil {
			return err
		}
		if err := e.EncodeString("ntli"); err != nil {
			return err
		}
		return e.EncodeInt(a.NTLI)
	})
}

func (a UpdateIsolatedMarginAction) MarshalJSON() ([]byte, error) {
	return json.Marshal(struct {
		Type  string `json:"type"`
		Asset int    `json:"asset"`
		IsBuy bool   `json:"isBuy"`
		NTLI  int64  `json:"ntli"`
	}{"updateIsolatedMargin", a.Asset, a.IsBuy, a.NTLI})
}

// TopUpIsolatedOnlyMarginAction targets leverage while only adding margin.
type TopUpIsolatedOnlyMarginAction struct {
	Asset    int
	Leverage string
}

func (a TopUpIsolatedOnlyMarginAction) MarshalMsgpack() ([]byte, error) {
	return marshalMap(func(e *msgpack.Encoder) error {
		if err := e.EncodeMapLen(3); err != nil {
			return err
		}
		for _, pair := range []struct {
			key string
			val any
		}{{"type", "topUpIsolatedOnlyMargin"}, {"asset", a.Asset}, {"leverage", a.Leverage}} {
			if err := e.EncodeString(pair.key); err != nil {
				return err
			}
			if err := e.Encode(pair.val); err != nil {
				return err
			}
		}
		return nil
	})
}

func (a TopUpIsolatedOnlyMarginAction) MarshalJSON() ([]byte, error) {
	return json.Marshal(struct {
		Type     string `json:"type"`
		Asset    int    `json:"asset"`
		Leverage string `json:"leverage"`
	}{"topUpIsolatedOnlyMargin", a.Asset, a.Leverage})
}

// ReserveRequestWeightAction purchases additional request weight.
type ReserveRequestWeightAction struct{ Weight uint64 }

func (a ReserveRequestWeightAction) MarshalMsgpack() ([]byte, error) {
	return marshalMap(func(e *msgpack.Encoder) error {
		if err := e.EncodeMapLen(2); err != nil {
			return err
		}
		if err := e.EncodeString("type"); err != nil {
			return err
		}
		if err := e.EncodeString("reserveRequestWeight"); err != nil {
			return err
		}
		if err := e.EncodeString("weight"); err != nil {
			return err
		}
		return e.EncodeUint(a.Weight)
	})
}

func (a ReserveRequestWeightAction) MarshalJSON() ([]byte, error) {
	return json.Marshal(struct {
		Type   string `json:"type"`
		Weight uint64 `json:"weight"`
	}{"reserveRequestWeight", a.Weight})
}

// NoopAction consumes a chosen nonce without changing account state.
type NoopAction struct{}

func (NoopAction) MarshalMsgpack() ([]byte, error) {
	return marshalMap(func(e *msgpack.Encoder) error {
		if err := e.EncodeMapLen(1); err != nil {
			return err
		}
		if err := e.EncodeString("type"); err != nil {
			return err
		}
		return e.EncodeString("noop")
	})
}

func (NoopAction) MarshalJSON() ([]byte, error) {
	return json.Marshal(struct {
		Type string `json:"type"`
	}{"noop"})
}
func (m ModifyWire) MarshalJSON() ([]byte, error) {
	oid := any(m.OID)
	if m.Cloid != nil {
		oid = *m.Cloid
	}
	return json.Marshal(struct {
		OID   any       `json:"oid"`
		Order OrderWire `json:"order"`
	}{oid, m.Order})
}
func marshalMap(fn func(*msgpack.Encoder) error) ([]byte, error) {
	var b []byte
	e := msgpack.NewEncoder(sliceWriter{&b})
	if err := fn(e); err != nil {
		return nil, err
	}
	return b, nil
}

type sliceWriter struct{ p *[]byte }

func (w sliceWriter) Write(data []byte) (int, error) {
	*w.p = append(*w.p, data...)
	return len(data), nil
}
