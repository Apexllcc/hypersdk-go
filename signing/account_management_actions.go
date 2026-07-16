package signing

import (
	"encoding/json"
	"fmt"
	"math/big"
	"sort"
	"strings"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/signer/core/apitypes"
	"github.com/vmihailenco/msgpack/v5"
)

// MultiSigSignerSet is the canonical authorization configuration used when a
// normal account converts to, updates, or exits multi-sig. A nil set converts
// an existing multi-sig account back to a normal account.
type MultiSigSignerSet struct {
	AuthorizedUsers []common.Address
	Threshold       uint8
}

// ConvertToMultiSigUserAction is Hyperliquid's EIP-712 account conversion
// action. Signers is deliberately serialized as a JSON string by the protocol.
type ConvertToMultiSigUserAction struct {
	Signers *MultiSigSignerSet
	Nonce   uint64
}

func (a ConvertToMultiSigUserAction) ActionNonce() uint64       { return a.Nonce }
func (ConvertToMultiSigUserAction) OmitOuterVaultAddress() bool { return false }

func (a ConvertToMultiSigUserAction) userSignedTypedData(isMainnet bool) (string, []apitypes.Type, apitypes.TypedDataMessage, error) {
	signers, err := a.signersJSON()
	if err != nil {
		return "", nil, nil, err
	}
	return "HyperliquidTransaction:ConvertToMultiSigUser", []apitypes.Type{{Name: "hyperliquidChain", Type: "string"}, {Name: "signers", Type: "string"}, {Name: "nonce", Type: "uint64"}}, apitypes.TypedDataMessage{"hyperliquidChain": userActionNetwork(isMainnet), "signers": signers, "nonce": new(big.Int).SetUint64(a.Nonce)}, nil
}

func (a ConvertToMultiSigUserAction) userSignedWire(isMainnet bool) (any, error) {
	signers, err := a.signersJSON()
	if err != nil {
		return nil, err
	}
	return struct {
		Type             string `json:"type"`
		HyperliquidChain string `json:"hyperliquidChain"`
		SignatureChainID string `json:"signatureChainId"`
		Signers          string `json:"signers"`
		Nonce            uint64 `json:"nonce"`
	}{"convertToMultiSigUser", userActionNetwork(isMainnet), "0x66eee", signers, a.Nonce}, nil
}

func (a ConvertToMultiSigUserAction) signersJSON() (string, error) {
	if a.Nonce == 0 {
		return "", fmt.Errorf("multi-sig conversion nonce is required")
	}
	if a.Signers == nil {
		return "null", nil
	}
	if len(a.Signers.AuthorizedUsers) == 0 || len(a.Signers.AuthorizedUsers) > 10 || a.Signers.Threshold == 0 || int(a.Signers.Threshold) > len(a.Signers.AuthorizedUsers) {
		return "", fmt.Errorf("multi-sig authorized users and threshold are invalid")
	}
	users := make([]string, len(a.Signers.AuthorizedUsers))
	seen := make(map[common.Address]struct{}, len(users))
	for i, user := range a.Signers.AuthorizedUsers {
		if user == (common.Address{}) {
			return "", fmt.Errorf("multi-sig authorized user is required")
		}
		if _, ok := seen[user]; ok {
			return "", fmt.Errorf("multi-sig authorized users must be unique")
		}
		seen[user] = struct{}{}
		users[i] = strings.ToLower(user.Hex())
	}
	sort.Strings(users)
	encoded, err := json.Marshal(struct {
		AuthorizedUsers []string `json:"authorizedUsers"`
		Threshold       uint8    `json:"threshold"`
	}{users, a.Signers.Threshold})
	if err != nil {
		return "", err
	}
	return string(encoded), nil
}

// CreateVaultAction creates a vault and seeds it with protocol USDC micros.
type CreateVaultAction struct {
	Name, Description string
	InitialUSD        uint64
	Nonce             uint64
}

func (a CreateVaultAction) MarshalMsgpack() ([]byte, error) {
	return marshalAccountMap([]accountPair{{"type", "createVault"}, {"name", a.Name}, {"description", a.Description}, {"initialUsd", a.InitialUSD}, {"nonce", a.Nonce}})
}
func (a CreateVaultAction) MarshalJSON() ([]byte, error) {
	return json.Marshal(struct {
		Type        string `json:"type"`
		Name        string `json:"name"`
		Description string `json:"description"`
		InitialUSD  uint64 `json:"initialUsd"`
		Nonce       uint64 `json:"nonce"`
	}{"createVault", a.Name, a.Description, a.InitialUSD, a.Nonce})
}

// VaultModifyAction changes the deposit and withdrawal controls of a vault.
// Nil booleans are encoded as JSON/msgpack null exactly as the official schema.
type VaultModifyAction struct {
	VaultAddress                         string
	AllowDeposits, AlwaysCloseOnWithdraw *bool
}

func (a VaultModifyAction) MarshalMsgpack() ([]byte, error) {
	return marshalAccountMap([]accountPair{{"type", "vaultModify"}, {"vaultAddress", a.VaultAddress}, {"allowDeposits", a.AllowDeposits}, {"alwaysCloseOnWithdraw", a.AlwaysCloseOnWithdraw}})
}
func (a VaultModifyAction) MarshalJSON() ([]byte, error) {
	return json.Marshal(struct {
		Type                  string `json:"type"`
		VaultAddress          string `json:"vaultAddress"`
		AllowDeposits         *bool  `json:"allowDeposits"`
		AlwaysCloseOnWithdraw *bool  `json:"alwaysCloseOnWithdraw"`
	}{"vaultModify", a.VaultAddress, a.AllowDeposits, a.AlwaysCloseOnWithdraw})
}

// VaultDistributeAction distributes protocol USDC micros to vault followers.
// USD 0 is meaningful: it closes the vault.
type VaultDistributeAction struct {
	VaultAddress string
	USD          uint64
}

func (a VaultDistributeAction) MarshalMsgpack() ([]byte, error) {
	return marshalAccountMap([]accountPair{{"type", "vaultDistribute"}, {"vaultAddress", a.VaultAddress}, {"usd", a.USD}})
}
func (a VaultDistributeAction) MarshalJSON() ([]byte, error) {
	return json.Marshal(struct {
		Type         string `json:"type"`
		VaultAddress string `json:"vaultAddress"`
		USD          uint64 `json:"usd"`
	}{"vaultDistribute", a.VaultAddress, a.USD})
}

// SubAccountModifyAction renames a subaccount.
type SubAccountModifyAction struct {
	SubAccountUser string
	Name           string
}

func (a SubAccountModifyAction) MarshalMsgpack() ([]byte, error) {
	return marshalAccountMap([]accountPair{{"type", "subAccountModify"}, {"subAccountUser", a.SubAccountUser}, {"name", a.Name}})
}
func (a SubAccountModifyAction) MarshalJSON() ([]byte, error) {
	return json.Marshal(struct {
		Type           string `json:"type"`
		SubAccountUser string `json:"subAccountUser"`
		Name           string `json:"name"`
	}{"subAccountModify", a.SubAccountUser, a.Name})
}

// SetDisplayNameAction sets or clears a user's leaderboard display name.
type SetDisplayNameAction struct{ DisplayName string }

func (a SetDisplayNameAction) MarshalMsgpack() ([]byte, error) {
	return marshalAccountMap([]accountPair{{"type", "setDisplayName"}, {"displayName", a.DisplayName}})
}
func (a SetDisplayNameAction) MarshalJSON() ([]byte, error) {
	return json.Marshal(struct {
		Type        string `json:"type"`
		DisplayName string `json:"displayName"`
	}{"setDisplayName", a.DisplayName})
}

type accountPair struct {
	key string
	val any
}

func marshalAccountMap(pairs []accountPair) ([]byte, error) {
	return marshalMap(func(e *msgpack.Encoder) error {
		if err := e.EncodeMapLen(len(pairs)); err != nil {
			return err
		}
		for _, pair := range pairs {
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
