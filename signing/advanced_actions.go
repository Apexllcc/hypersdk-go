package signing

import (
	"encoding/json"
	"fmt"
	"math/big"
	"strings"

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

// AgentSendAssetAction transfers an asset through an approved agent wallet.
// Unlike SendAssetAction, this is an L1 Agent action and its inner nonce must
// exactly equal the request-envelope nonce.
type AgentSendAssetAction struct {
	Destination    string
	SourceDEX      string
	DestinationDEX string
	Token          string
	Amount         string
	FromSubAccount string
	Nonce          uint64
}

func (a AgentSendAssetAction) validate() error {
	if !common.IsHexAddress(a.Destination) || a.Token == "" || a.Nonce == 0 {
		return fmt.Errorf("agent send asset requires destination, token, and nonce")
	}
	if a.FromSubAccount != "" && !common.IsHexAddress(a.FromSubAccount) {
		return fmt.Errorf("agent send asset subaccount is not an address")
	}
	amount, ok := new(big.Rat).SetString(a.Amount)
	if !ok || amount.Sign() <= 0 {
		return fmt.Errorf("agent send asset amount must be a positive decimal string")
	}
	return nil
}

func (a AgentSendAssetAction) MarshalMsgpack() ([]byte, error) {
	if err := a.validate(); err != nil {
		return nil, err
	}
	return marshalMap(func(e *msgpack.Encoder) error {
		if err := e.EncodeMapLen(8); err != nil {
			return err
		}
		for _, field := range []struct {
			key string
			val any
		}{{"type", "agentSendAsset"}, {"destination", strings.ToLower(a.Destination)}, {"sourceDex", a.SourceDEX}, {"destinationDex", a.DestinationDEX}, {"token", a.Token}, {"amount", a.Amount}, {"fromSubAccount", strings.ToLower(a.FromSubAccount)}, {"nonce", a.Nonce}} {
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

func (a AgentSendAssetAction) MarshalJSON() ([]byte, error) {
	if err := a.validate(); err != nil {
		return nil, err
	}
	return json.Marshal(struct {
		Type           string `json:"type"`
		Destination    string `json:"destination"`
		SourceDEX      string `json:"sourceDex"`
		DestinationDEX string `json:"destinationDex"`
		Token          string `json:"token"`
		Amount         string `json:"amount"`
		FromSubAccount string `json:"fromSubAccount"`
		Nonce          uint64 `json:"nonce"`
	}{"agentSendAsset", strings.ToLower(a.Destination), a.SourceDEX, a.DestinationDEX, a.Token, a.Amount, strings.ToLower(a.FromSubAccount), a.Nonce})
}

// AQAV2Role is an explicitly authorized aligned-quote-asset role.
type AQAV2Role string

const (
	AQAV2RoleTechnical AQAV2Role = "technical"
	AQAV2RoleTreasury  AQAV2Role = "treasury"
)

// AuthorizeAQAV2RoleAction assigns an AQAv2 technical or treasury role.
type AuthorizeAQAV2RoleAction struct {
	Token uint64
	Role  AQAV2Role
}

func (a AuthorizeAQAV2RoleAction) validate() error {
	if a.Role != AQAV2RoleTechnical && a.Role != AQAV2RoleTreasury {
		return fmt.Errorf("unsupported AQA v2 role %q", a.Role)
	}
	return nil
}
func (a AuthorizeAQAV2RoleAction) MarshalMsgpack() ([]byte, error) {
	if err := a.validate(); err != nil {
		return nil, err
	}
	return marshalMap(func(e *msgpack.Encoder) error {
		if err := e.EncodeMapLen(3); err != nil {
			return err
		}
		for _, field := range []struct {
			key string
			val any
		}{{"type", "authorizeAqav2Role"}, {"token", a.Token}, {"role", string(a.Role)}} {
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
func (a AuthorizeAQAV2RoleAction) MarshalJSON() ([]byte, error) {
	if err := a.validate(); err != nil {
		return nil, err
	}
	return json.Marshal(struct {
		Type  string `json:"type"`
		Token uint64 `json:"token"`
		Role  string `json:"role"`
	}{"authorizeAqav2Role", a.Token, string(a.Role)})
}

// HIP3LiquidatorTransferAction moves quote-token micros into or out of a
// HIP-3 DEX backstop liquidator. The protocol minimum unit is 1,000 tokens.
type HIP3LiquidatorTransferAction struct {
	DEX       string
	NTL       uint64
	IsDeposit bool
}

const hip3LiquidatorMinimumNTL uint64 = 1_000_000_000

func (a HIP3LiquidatorTransferAction) validate() error {
	if strings.TrimSpace(a.DEX) == "" {
		return fmt.Errorf("HIP-3 liquidator DEX is required")
	}
	if a.NTL == 0 || a.NTL%hip3LiquidatorMinimumNTL != 0 {
		return fmt.Errorf("HIP-3 liquidator notional must be a positive multiple of %d", hip3LiquidatorMinimumNTL)
	}
	return nil
}

// UserOutcomeAction manually splits, merges, or negates outcome shares. Exactly
// one variant is required. Nullable merge amounts are encoded as MsgPack nil,
// as required by the Exchange action schema.
type UserOutcomeAction struct {
	SplitOutcome  *SplitOutcome
	MergeOutcome  *MergeOutcome
	MergeQuestion *MergeQuestion
	NegateOutcome *NegateOutcome
}

type SplitOutcome struct {
	Outcome uint64 `json:"outcome"`
	Amount  string `json:"amount"`
}

type MergeOutcome struct {
	Outcome uint64  `json:"outcome"`
	Amount  *string `json:"amount"`
}

type MergeQuestion struct {
	Question uint64  `json:"question"`
	Amount   *string `json:"amount"`
}

type NegateOutcome struct {
	Question uint64 `json:"question"`
	Outcome  uint64 `json:"outcome"`
	Amount   string `json:"amount"`
}

func validPositiveDecimal(value string) bool {
	amount, ok := new(big.Rat).SetString(value)
	return ok && amount.Sign() > 0
}

func (a UserOutcomeAction) validate() error {
	variants := 0
	if a.SplitOutcome != nil {
		variants++
		if !validPositiveDecimal(a.SplitOutcome.Amount) {
			return fmt.Errorf("split outcome amount must be a positive decimal string")
		}
	}
	if a.MergeOutcome != nil {
		variants++
		if a.MergeOutcome.Amount != nil && !validPositiveDecimal(*a.MergeOutcome.Amount) {
			return fmt.Errorf("merge outcome amount must be a positive decimal string or nil")
		}
	}
	if a.MergeQuestion != nil {
		variants++
		if a.MergeQuestion.Amount != nil && !validPositiveDecimal(*a.MergeQuestion.Amount) {
			return fmt.Errorf("merge question amount must be a positive decimal string or nil")
		}
	}
	if a.NegateOutcome != nil {
		variants++
		if !validPositiveDecimal(a.NegateOutcome.Amount) {
			return fmt.Errorf("negate outcome amount must be a positive decimal string")
		}
	}
	if variants != 1 {
		return fmt.Errorf("exactly one user outcome variant is required")
	}
	return nil
}

func (a UserOutcomeAction) MarshalMsgpack() ([]byte, error) {
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
		if err := e.EncodeString("userOutcome"); err != nil {
			return err
		}
		if a.SplitOutcome != nil {
			if err := e.EncodeString("splitOutcome"); err != nil {
				return err
			}
			return encodeOutcomeFields(e, "outcome", a.SplitOutcome.Outcome, "amount", a.SplitOutcome.Amount)
		}
		if a.MergeOutcome != nil {
			if err := e.EncodeString("mergeOutcome"); err != nil {
				return err
			}
			return encodeOutcomeFields(e, "outcome", a.MergeOutcome.Outcome, "amount", a.MergeOutcome.Amount)
		}
		if a.MergeQuestion != nil {
			if err := e.EncodeString("mergeQuestion"); err != nil {
				return err
			}
			return encodeOutcomeFields(e, "question", a.MergeQuestion.Question, "amount", a.MergeQuestion.Amount)
		}
		if err := e.EncodeString("negateOutcome"); err != nil {
			return err
		}
		if err := e.EncodeMapLen(3); err != nil {
			return err
		}
		for _, field := range []struct {
			key   string
			value any
		}{{"question", a.NegateOutcome.Question}, {"outcome", a.NegateOutcome.Outcome}, {"amount", a.NegateOutcome.Amount}} {
			if err := e.EncodeString(field.key); err != nil {
				return err
			}
			if err := e.Encode(field.value); err != nil {
				return err
			}
		}
		return nil
	})
}

func encodeOutcomeFields(e *msgpack.Encoder, firstKey string, firstValue uint64, secondKey string, secondValue any) error {
	if err := e.EncodeMapLen(2); err != nil {
		return err
	}
	if err := e.EncodeString(firstKey); err != nil {
		return err
	}
	if err := e.Encode(firstValue); err != nil {
		return err
	}
	if err := e.EncodeString(secondKey); err != nil {
		return err
	}
	return e.Encode(secondValue)
}

func (a UserOutcomeAction) MarshalJSON() ([]byte, error) {
	if err := a.validate(); err != nil {
		return nil, err
	}
	if a.SplitOutcome != nil {
		return json.Marshal(struct {
			Type         string        `json:"type"`
			SplitOutcome *SplitOutcome `json:"splitOutcome"`
		}{"userOutcome", a.SplitOutcome})
	}
	if a.MergeOutcome != nil {
		return json.Marshal(struct {
			Type         string        `json:"type"`
			MergeOutcome *MergeOutcome `json:"mergeOutcome"`
		}{"userOutcome", a.MergeOutcome})
	}
	if a.MergeQuestion != nil {
		return json.Marshal(struct {
			Type          string         `json:"type"`
			MergeQuestion *MergeQuestion `json:"mergeQuestion"`
		}{"userOutcome", a.MergeQuestion})
	}
	return json.Marshal(struct {
		Type          string         `json:"type"`
		NegateOutcome *NegateOutcome `json:"negateOutcome"`
	}{"userOutcome", a.NegateOutcome})
}
func (a HIP3LiquidatorTransferAction) MarshalMsgpack() ([]byte, error) {
	if err := a.validate(); err != nil {
		return nil, err
	}
	return marshalMap(func(e *msgpack.Encoder) error {
		if err := e.EncodeMapLen(4); err != nil {
			return err
		}
		for _, field := range []struct {
			key string
			val any
		}{{"type", "hip3LiquidatorTransfer"}, {"dex", a.DEX}, {"ntl", a.NTL}, {"isDeposit", a.IsDeposit}} {
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
func (a HIP3LiquidatorTransferAction) MarshalJSON() ([]byte, error) {
	if err := a.validate(); err != nil {
		return nil, err
	}
	return json.Marshal(struct {
		Type      string `json:"type"`
		DEX       string `json:"dex"`
		NTL       uint64 `json:"ntl"`
		IsDeposit bool   `json:"isDeposit"`
	}{"hip3LiquidatorTransfer", a.DEX, a.NTL, a.IsDeposit})
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
