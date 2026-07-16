package signing

import (
	"encoding/json"
	"fmt"
	"math/big"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/signer/core/apitypes"
	"github.com/vmihailenco/msgpack/v5"
)

// UserDexAbstractionAction is the legacy user-signed HIP-3 abstraction action.
// New callers should prefer UserSetAbstractionAction.
type UserDexAbstractionAction struct {
	User    common.Address
	Enabled bool
	Nonce   uint64
}

func (a UserDexAbstractionAction) ActionNonce() uint64       { return a.Nonce }
func (UserDexAbstractionAction) OmitOuterVaultAddress() bool { return false }

func (a UserDexAbstractionAction) userSignedTypedData(isMainnet bool) (string, []apitypes.Type, apitypes.TypedDataMessage, error) {
	if err := a.validate(); err != nil {
		return "", nil, nil, err
	}
	return "HyperliquidTransaction:UserDexAbstraction", []apitypes.Type{{Name: "hyperliquidChain", Type: "string"}, {Name: "user", Type: "address"}, {Name: "enabled", Type: "bool"}, {Name: "nonce", Type: "uint64"}}, apitypes.TypedDataMessage{"hyperliquidChain": userActionNetwork(isMainnet), "user": a.User.Hex(), "enabled": a.Enabled, "nonce": new(big.Int).SetUint64(a.Nonce)}, nil
}

func (a UserDexAbstractionAction) userSignedWire(isMainnet bool) (any, error) {
	if err := a.validate(); err != nil {
		return nil, err
	}
	return struct {
		Type             string `json:"type"`
		HyperliquidChain string `json:"hyperliquidChain"`
		SignatureChainID string `json:"signatureChainId"`
		User             string `json:"user"`
		Enabled          bool   `json:"enabled"`
		Nonce            uint64 `json:"nonce"`
	}{"userDexAbstraction", userActionNetwork(isMainnet), "0x66eee", a.User.Hex(), a.Enabled, a.Nonce}, nil
}

func (a UserDexAbstractionAction) validate() error {
	if a.User == (common.Address{}) || a.Nonce == 0 {
		return fmt.Errorf("user DEX abstraction requires user and nonce")
	}
	return nil
}

// UserSetAbstractionAction selects disabled, unifiedAccount, or
// portfolioMargin for a user or subaccount through EIP-712.
type UserSetAbstractionAction struct {
	User        common.Address
	Abstraction string
	Nonce       uint64
}

func (a UserSetAbstractionAction) ActionNonce() uint64       { return a.Nonce }
func (UserSetAbstractionAction) OmitOuterVaultAddress() bool { return false }

func (a UserSetAbstractionAction) userSignedTypedData(isMainnet bool) (string, []apitypes.Type, apitypes.TypedDataMessage, error) {
	if err := a.validate(); err != nil {
		return "", nil, nil, err
	}
	return "HyperliquidTransaction:UserSetAbstraction", []apitypes.Type{{Name: "hyperliquidChain", Type: "string"}, {Name: "user", Type: "address"}, {Name: "abstraction", Type: "string"}, {Name: "nonce", Type: "uint64"}}, apitypes.TypedDataMessage{"hyperliquidChain": userActionNetwork(isMainnet), "user": a.User.Hex(), "abstraction": a.Abstraction, "nonce": new(big.Int).SetUint64(a.Nonce)}, nil
}

func (a UserSetAbstractionAction) userSignedWire(isMainnet bool) (any, error) {
	if err := a.validate(); err != nil {
		return nil, err
	}
	return struct {
		Type             string `json:"type"`
		HyperliquidChain string `json:"hyperliquidChain"`
		SignatureChainID string `json:"signatureChainId"`
		User             string `json:"user"`
		Abstraction      string `json:"abstraction"`
		Nonce            uint64 `json:"nonce"`
	}{"userSetAbstraction", userActionNetwork(isMainnet), "0x66eee", a.User.Hex(), a.Abstraction, a.Nonce}, nil
}

func (a UserSetAbstractionAction) validate() error {
	if a.User == (common.Address{}) || a.Nonce == 0 {
		return fmt.Errorf("user abstraction requires user and nonce")
	}
	switch a.Abstraction {
	case "disabled", "unifiedAccount", "portfolioMargin":
		return nil
	default:
		return fmt.Errorf("unsupported user abstraction %q", a.Abstraction)
	}
}

// AgentEnableDexAbstractionAction is the deprecated agent-only L1 action.
type AgentEnableDexAbstractionAction struct{}

func (AgentEnableDexAbstractionAction) MarshalMsgpack() ([]byte, error) {
	return marshalMap(func(e *msgpack.Encoder) error {
		if err := e.EncodeMapLen(1); err != nil {
			return err
		}
		if err := e.EncodeString("type"); err != nil {
			return err
		}
		return e.EncodeString("agentEnableDexAbstraction")
	})
}
func (AgentEnableDexAbstractionAction) MarshalJSON() ([]byte, error) {
	return json.Marshal(struct {
		Type string `json:"type"`
	}{"agentEnableDexAbstraction"})
}

// AgentSetAbstractionAction changes an agent's compact abstraction setting.
type AgentSetAbstractionAction struct{ Abstraction string }

func (a AgentSetAbstractionAction) MarshalMsgpack() ([]byte, error) {
	return marshalMap(func(e *msgpack.Encoder) error {
		if err := e.EncodeMapLen(2); err != nil {
			return err
		}
		if err := e.EncodeString("type"); err != nil {
			return err
		}
		if err := e.EncodeString("agentSetAbstraction"); err != nil {
			return err
		}
		if err := e.EncodeString("abstraction"); err != nil {
			return err
		}
		return e.EncodeString(a.Abstraction)
	})
}
func (a AgentSetAbstractionAction) MarshalJSON() ([]byte, error) {
	return json.Marshal(struct {
		Type        string `json:"type"`
		Abstraction string `json:"abstraction"`
	}{"agentSetAbstraction", a.Abstraction})
}

// ValidatorL1StreamAction lets a validator vote on the aligned quote asset's
// risk-free rate. RiskFreeRate stays a canonical decimal string.
type ValidatorL1StreamAction struct{ RiskFreeRate string }

func (a ValidatorL1StreamAction) MarshalMsgpack() ([]byte, error) {
	return marshalMap(func(e *msgpack.Encoder) error {
		if err := e.EncodeMapLen(2); err != nil {
			return err
		}
		if err := e.EncodeString("type"); err != nil {
			return err
		}
		if err := e.EncodeString("validatorL1Stream"); err != nil {
			return err
		}
		if err := e.EncodeString("riskFreeRate"); err != nil {
			return err
		}
		return e.EncodeString(a.RiskFreeRate)
	})
}
func (a ValidatorL1StreamAction) MarshalJSON() ([]byte, error) {
	return json.Marshal(struct {
		Type         string `json:"type"`
		RiskFreeRate string `json:"riskFreeRate"`
	}{"validatorL1Stream", a.RiskFreeRate})
}

// ClaimRewardsAction claims validator rewards through the L1 action path.
type ClaimRewardsAction struct{}

func (ClaimRewardsAction) MarshalMsgpack() ([]byte, error) {
	return marshalMap(func(e *msgpack.Encoder) error {
		if err := e.EncodeMapLen(1); err != nil {
			return err
		}
		if err := e.EncodeString("type"); err != nil {
			return err
		}
		return e.EncodeString("claimRewards")
	})
}
func (ClaimRewardsAction) MarshalJSON() ([]byte, error) {
	return json.Marshal(struct {
		Type string `json:"type"`
	}{"claimRewards"})
}
