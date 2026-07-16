package signing

import (
	"encoding/json"
	"fmt"
	"net"
	"reflect"
	"strings"

	"github.com/ethereum/go-ethereum/common"
	"github.com/vmihailenco/msgpack/v5"
)

// EVMUserModifyAction selects the HyperEVM block class used by an account.
// It is an L1 action; it must be signed with no active-pool marker.
type EVMUserModifyAction struct{ UsingBigBlocks bool }

// L1SigningVault selects the nil active-pool marker mandated by the protocol.
// The outer Exchange envelope may still carry the client's configured vault.
func (EVMUserModifyAction) L1SigningVault(*common.Address) *common.Address { return nil }

func (a EVMUserModifyAction) MarshalMsgpack() ([]byte, error) {
	return marshalMap(func(e *msgpack.Encoder) error {
		if err := e.EncodeMapLen(2); err != nil {
			return err
		}
		if err := e.EncodeString("type"); err != nil {
			return err
		}
		if err := e.EncodeString("evmUserModify"); err != nil {
			return err
		}
		if err := e.EncodeString("usingBigBlocks"); err != nil {
			return err
		}
		return e.EncodeBool(a.UsingBigBlocks)
	})
}
func (a EVMUserModifyAction) MarshalJSON() ([]byte, error) {
	return json.Marshal(struct {
		Type           string `json:"type"`
		UsingBigBlocks bool   `json:"usingBigBlocks"`
	}{"evmUserModify", a.UsingBigBlocks})
}

// GossipPriorityBidAction bids for a gossip-priority auction slot. SlotID is
// protocol-defined to be 0 or 1, and IP must be a literal IPv4 or IPv6 address.
type GossipPriorityBidAction struct {
	SlotID uint64
	IP     string
	MaxGas uint64
}

const minimumGossipPriorityBidGas uint64 = 10_000_000 // 0.1 HYPE in wei.

func (a GossipPriorityBidAction) validate() error {
	if a.SlotID > 1 {
		return fmt.Errorf("gossip priority slot ID must be 0 or 1")
	}
	if net.ParseIP(a.IP) == nil {
		return fmt.Errorf("gossip priority IP is invalid")
	}
	if a.MaxGas < minimumGossipPriorityBidGas {
		return fmt.Errorf("gossip priority max gas must be at least %d wei", minimumGossipPriorityBidGas)
	}
	return nil
}
func (a GossipPriorityBidAction) MarshalMsgpack() ([]byte, error) {
	if err := a.validate(); err != nil {
		return nil, err
	}
	return marshalMap(func(e *msgpack.Encoder) error {
		if err := e.EncodeMapLen(4); err != nil {
			return err
		}
		for _, pair := range []struct {
			key   string
			value any
		}{{"type", "gossipPriorityBid"}, {"slotId", a.SlotID}, {"ip", a.IP}, {"maxGas", a.MaxGas}} {
			if err := e.EncodeString(pair.key); err != nil {
				return err
			}
			if err := e.Encode(pair.value); err != nil {
				return err
			}
		}
		return nil
	})
}
func (a GossipPriorityBidAction) MarshalJSON() ([]byte, error) {
	if err := a.validate(); err != nil {
		return nil, err
	}
	return json.Marshal(struct {
		Type   string `json:"type"`
		SlotID uint64 `json:"slotId"`
		IP     string `json:"ip"`
		MaxGas uint64 `json:"maxGas"`
	}{"gossipPriorityBid", a.SlotID, a.IP, a.MaxGas})
}

// CValidatorVariant is the sealed set of validator-management L1 actions
// implemented by the official Python SDK.
type CValidatorVariant interface {
	cValidatorVariant()
	marshalCValidator(*msgpack.Encoder) error
	jsonCValidator() (any, error)
}

// CValidatorAction is a sealed union. Exactly one concrete CValidatorVariant
// must be supplied, preventing ambiguous action dictionaries.
type CValidatorAction struct{ Variant CValidatorVariant }

// L1SigningVault selects the nil active-pool marker mandated by the protocol.
func (CValidatorAction) L1SigningVault(*common.Address) *common.Address { return nil }

func (a CValidatorAction) validate() error {
	if a.Variant == nil {
		return fmt.Errorf("validator action variant is required")
	}
	v := reflect.ValueOf(a.Variant)
	if v.Kind() == reflect.Pointer && v.IsNil() {
		return fmt.Errorf("validator action variant is required")
	}
	return nil
}
func (a CValidatorAction) MarshalMsgpack() ([]byte, error) {
	if err := a.validate(); err != nil {
		return nil, err
	}
	return marshalMap(func(e *msgpack.Encoder) error {
		if err := e.EncodeMapLen(2); err != nil {
			return err
		}
		if err := e.EncodeString("type"); err != nil {
			return err
		}
		if err := e.EncodeString("CValidatorAction"); err != nil {
			return err
		}
		return a.Variant.marshalCValidator(e)
	})
}
func (a CValidatorAction) MarshalJSON() ([]byte, error) {
	if err := a.validate(); err != nil {
		return nil, err
	}
	v, err := a.Variant.jsonCValidator()
	if err != nil {
		return nil, err
	}
	fields, ok := v.(map[string]any)
	if !ok {
		return nil, fmt.Errorf("validator action variant has invalid JSON wire value")
	}
	fields["type"] = "CValidatorAction"
	return json.Marshal(fields)
}

// CValidatorUnregister unregisters the caller's validator.
type CValidatorUnregister struct{}

func (CValidatorUnregister) cValidatorVariant() {}
func (CValidatorUnregister) marshalCValidator(e *msgpack.Encoder) error {
	if err := e.EncodeString("unregister"); err != nil {
		return err
	}
	return e.EncodeNil()
}
func (CValidatorUnregister) jsonCValidator() (any, error) {
	return map[string]any{"unregister": nil}, nil
}

// CValidatorRegister registers a validator profile.
type CValidatorRegister struct {
	Profile    CValidatorProfile
	Unjailed   bool
	InitialWei uint64
}

func (CValidatorRegister) cValidatorVariant() {}
func (a CValidatorRegister) marshalCValidator(e *msgpack.Encoder) error {
	if err := a.validate(); err != nil {
		return err
	}
	if err := e.EncodeString("register"); err != nil {
		return err
	}
	return e.Encode(struct {
		Profile    CValidatorProfile `msgpack:"profile"`
		Unjailed   bool              `msgpack:"unjailed"`
		InitialWei uint64            `msgpack:"initial_wei"`
	}{a.Profile, a.Unjailed, a.InitialWei})
}
func (a CValidatorRegister) jsonCValidator() (any, error) {
	if err := a.validate(); err != nil {
		return nil, err
	}
	return map[string]any{"register": struct {
		Profile    CValidatorProfile `json:"profile"`
		Unjailed   bool              `json:"unjailed"`
		InitialWei uint64            `json:"initial_wei"`
	}{a.Profile, a.Unjailed, a.InitialWei}}, nil
}
func (a CValidatorRegister) validate() error {
	if err := a.Profile.validateRequired(); err != nil {
		return err
	}
	if a.InitialWei == 0 {
		return fmt.Errorf("validator initial wei must be positive")
	}
	return nil
}

// CValidatorChangeProfile modifies a validator profile. Pointer fields encode
// the protocol's required explicit null values, never omitted fields.
type CValidatorChangeProfile struct {
	NodeIP             *string
	Name               *string
	Description        *string
	Unjailed           bool
	DisableDelegations *bool
	CommissionBPS      *uint64
	Signer             *common.Address
}

func (CValidatorChangeProfile) cValidatorVariant() {}
func (a CValidatorChangeProfile) marshalCValidator(e *msgpack.Encoder) error {
	if err := a.validate(); err != nil {
		return err
	}
	if err := e.EncodeString("changeProfile"); err != nil {
		return err
	}
	return e.Encode(a.wire())
}
func (a CValidatorChangeProfile) jsonCValidator() (any, error) {
	if err := a.validate(); err != nil {
		return nil, err
	}
	return map[string]any{"changeProfile": a.wire()}, nil
}
func (a CValidatorChangeProfile) validate() error {
	if a.NodeIP != nil && net.ParseIP(*a.NodeIP) == nil {
		return fmt.Errorf("validator node IP is invalid")
	}
	if a.Signer != nil && *a.Signer == (common.Address{}) {
		return fmt.Errorf("validator signer is invalid")
	}
	return nil
}
func (a CValidatorChangeProfile) wire() cValidatorChangeProfileWire {
	var nodeIP any
	if a.NodeIP != nil {
		nodeIP = struct {
			IP string `json:"Ip" msgpack:"Ip"`
		}{*a.NodeIP}
	}
	var signerValue any
	if a.Signer != nil {
		signerValue = strings.ToLower(a.Signer.Hex())
	}
	return cValidatorChangeProfileWire{NodeIP: nodeIP, Name: a.Name, Description: a.Description, Unjailed: a.Unjailed, DisableDelegations: a.DisableDelegations, CommissionBPS: a.CommissionBPS, Signer: signerValue}
}

type cValidatorChangeProfileWire struct {
	NodeIP             any     `json:"node_ip" msgpack:"node_ip"`
	Name               *string `json:"name" msgpack:"name"`
	Description        *string `json:"description" msgpack:"description"`
	Unjailed           bool    `json:"unjailed" msgpack:"unjailed"`
	DisableDelegations *bool   `json:"disable_delegations" msgpack:"disable_delegations"`
	CommissionBPS      *uint64 `json:"commission_bps" msgpack:"commission_bps"`
	Signer             any     `json:"signer" msgpack:"signer"`
}

// CValidatorProfile is used only while registering. All fields are required by
// the official Python SDK action constructor.
type CValidatorProfile struct {
	NodeIP              string
	Name                string
	Description         string
	DelegationsDisabled bool
	CommissionBPS       uint64
	Signer              common.Address
}

func (p CValidatorProfile) validateRequired() error {
	if net.ParseIP(p.NodeIP) == nil || p.Name == "" || p.Description == "" || p.Signer == (common.Address{}) {
		return fmt.Errorf("validator profile requires valid IP, name, description, and signer")
	}
	return nil
}
func (p CValidatorProfile) MarshalJSON() ([]byte, error) {
	if err := p.validateRequired(); err != nil {
		return nil, err
	}
	return json.Marshal(struct {
		NodeIP struct {
			IP string `json:"Ip"`
		} `json:"node_ip"`
		Name                string `json:"name"`
		Description         string `json:"description"`
		DelegationsDisabled bool   `json:"delegations_disabled"`
		CommissionBPS       uint64 `json:"commission_bps"`
		Signer              string `json:"signer"`
	}{NodeIP: struct {
		IP string `json:"Ip"`
	}{p.NodeIP}, Name: p.Name, Description: p.Description, DelegationsDisabled: p.DelegationsDisabled, CommissionBPS: p.CommissionBPS, Signer: strings.ToLower(p.Signer.Hex())})
}
func (p CValidatorProfile) MarshalMsgpack() ([]byte, error) {
	if err := p.validateRequired(); err != nil {
		return nil, err
	}
	return marshalMap(func(e *msgpack.Encoder) error {
		if err := e.EncodeMapLen(6); err != nil {
			return err
		}
		for _, pair := range []struct {
			key   string
			value any
		}{{"node_ip", struct {
			IP string `msgpack:"Ip"`
		}{p.NodeIP}}, {"name", p.Name}, {"description", p.Description}, {"delegations_disabled", p.DelegationsDisabled}, {"commission_bps", p.CommissionBPS}, {"signer", strings.ToLower(p.Signer.Hex())}} {
			if err := e.EncodeString(pair.key); err != nil {
				return err
			}
			if err := e.Encode(pair.value); err != nil {
				return err
			}
		}
		return nil
	})
}

// FinalizeEVMContractInput is the sealed proof variant required to link a
// HyperCore token to an EVM contract.
type FinalizeEVMContractInput interface {
	finalizeEVMContractInput()
	marshalFinalizeEVM(*msgpack.Encoder) error
	jsonFinalizeEVM() any
}
type FinalizeEVMContractAction struct {
	Token uint64
	Input FinalizeEVMContractInput
}

// L1SigningVault selects the nil active-pool marker mandated by the protocol.
func (FinalizeEVMContractAction) L1SigningVault(*common.Address) *common.Address { return nil }

// L1OuterVault omits vault routing too: the official HyperCore-to-HyperEVM
// finalization request sends vaultAddress as null.
func (FinalizeEVMContractAction) L1OuterVault(*common.Address) *common.Address { return nil }

func (a FinalizeEVMContractAction) validate() error {
	if a.Input == nil {
		return fmt.Errorf("finalize EVM contract input is required")
	}
	v := reflect.ValueOf(a.Input)
	if v.Kind() == reflect.Pointer && v.IsNil() {
		return fmt.Errorf("finalize EVM contract input is required")
	}
	return nil
}
func (a FinalizeEVMContractAction) MarshalMsgpack() ([]byte, error) {
	if err := a.validate(); err != nil {
		return nil, err
	}
	return marshalMap(func(e *msgpack.Encoder) error {
		if err := e.EncodeMapLen(3); err != nil {
			return err
		}
		if err := e.EncodeString("type"); err != nil {
			return err
		}
		if err := e.EncodeString("finalizeEvmContract"); err != nil {
			return err
		}
		if err := e.EncodeString("token"); err != nil {
			return err
		}
		if err := e.EncodeUint(a.Token); err != nil {
			return err
		}
		if err := e.EncodeString("input"); err != nil {
			return err
		}
		return a.Input.marshalFinalizeEVM(e)
	})
}
func (a FinalizeEVMContractAction) MarshalJSON() ([]byte, error) {
	if err := a.validate(); err != nil {
		return nil, err
	}
	return json.Marshal(struct {
		Type  string `json:"type"`
		Token uint64 `json:"token"`
		Input any    `json:"input"`
	}{"finalizeEvmContract", a.Token, a.Input.jsonFinalizeEVM()})
}

type FinalizeEVMCreate struct{ Nonce uint64 }

func (FinalizeEVMCreate) finalizeEVMContractInput() {}
func (v FinalizeEVMCreate) marshalFinalizeEVM(e *msgpack.Encoder) error {
	if err := e.EncodeMapLen(1); err != nil {
		return err
	}
	if err := e.EncodeString("create"); err != nil {
		return err
	}
	if err := e.EncodeMapLen(1); err != nil {
		return err
	}
	if err := e.EncodeString("nonce"); err != nil {
		return err
	}
	return e.EncodeUint(v.Nonce)
}
func (v FinalizeEVMCreate) jsonFinalizeEVM() any {
	return map[string]any{"create": map[string]uint64{"nonce": v.Nonce}}
}

type FinalizeEVMFirstStorageSlot struct{}

func (FinalizeEVMFirstStorageSlot) finalizeEVMContractInput() {}
func (FinalizeEVMFirstStorageSlot) marshalFinalizeEVM(e *msgpack.Encoder) error {
	return e.EncodeString("firstStorageSlot")
}
func (FinalizeEVMFirstStorageSlot) jsonFinalizeEVM() any { return "firstStorageSlot" }

type FinalizeEVMCustomStorageSlot struct{}

func (FinalizeEVMCustomStorageSlot) finalizeEVMContractInput() {}
func (FinalizeEVMCustomStorageSlot) marshalFinalizeEVM(e *msgpack.Encoder) error {
	return e.EncodeString("customStorageSlot")
}
func (FinalizeEVMCustomStorageSlot) jsonFinalizeEVM() any { return "customStorageSlot" }
