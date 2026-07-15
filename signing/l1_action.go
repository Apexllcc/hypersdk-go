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
}

func (a OrderAction) MarshalMsgpack() ([]byte, error) {
	return marshalMap(func(e *msgpack.Encoder) error {
		if err := e.EncodeMapLen(3); err != nil {
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
		return nil
	})
}

// OrderWire contains precision-safe strings expected by the L1 order protocol.
type OrderWire struct {
	Asset      int
	IsBuy      bool
	Price      string
	Size       string
	ReduceOnly bool
	Type       LimitOrderType
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

// LimitOrderType is the L1 wire representation of a limit order.
type LimitOrderType struct{ TIF string }

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
		Type     string      `json:"type"`
		Orders   []OrderWire `json:"orders"`
		Grouping string      `json:"grouping"`
	}{"order", a.Orders, a.Grouping})
}
func (o OrderWire) MarshalJSON() ([]byte, error) {
	return json.Marshal(struct {
		Asset      int            `json:"a"`
		IsBuy      bool           `json:"b"`
		Price      string         `json:"p"`
		Size       string         `json:"s"`
		ReduceOnly bool           `json:"r"`
		Type       LimitOrderType `json:"t"`
		Cloid      *string        `json:"c,omitempty"`
	}{o.Asset, o.IsBuy, o.Price, o.Size, o.ReduceOnly, o.Type, o.Cloid})
}
func (l LimitOrderType) MarshalJSON() ([]byte, error) {
	return json.Marshal(map[string]any{"limit": map[string]string{"tif": l.TIF}})
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

type BatchModifyAction struct{ Modifies []ModifyWire }
type ModifyWire struct {
	OID   uint64
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
		if err := e.EncodeUint(m.OID); err != nil {
			return err
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
func (m ModifyWire) MarshalJSON() ([]byte, error) {
	return json.Marshal(struct {
		OID   uint64    `json:"oid"`
		Order OrderWire `json:"order"`
	}{m.OID, m.Order})
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
