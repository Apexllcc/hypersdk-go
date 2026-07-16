package signing

import (
	"encoding/hex"
	"encoding/json"
	"fmt"
	"math/big"
	"strings"

	"github.com/Apexllcc/hyperliquid-go-sdk/signer"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/common/math"
	"github.com/ethereum/go-ethereum/signer/core/apitypes"
	"github.com/vmihailenco/msgpack/v5"
)

// CompactSignature is the canonical compact representation required for each
// inner signature in a multiSig action. R and S are trimmed hexadecimal
// integers, while V remains Ethereum's 27/28 recovery value.
type CompactSignature struct {
	R string `json:"r"`
	S string `json:"s"`
	V uint8  `json:"v"`
}

// CompactSignatureFromSignature verifies the supplied signature shape and
// converts it to Hyperliquid's compact multi-signature representation.
func CompactSignatureFromSignature(signature signer.Signature) (CompactSignature, error) {
	v, err := signer.NormalizeRecoveryID(signature.V)
	if err != nil {
		return CompactSignature{}, err
	}
	return CompactSignature{R: compactHex(signature.R[:]), S: compactHex(signature.S[:]), V: v + 27}, nil
}

func compactHex(value []byte) string {
	encoded := strings.TrimLeft(hex.EncodeToString(value), "0")
	if encoded == "" {
		encoded = "0"
	}
	return "0x" + encoded
}

// MultiSigPayload is the authorization payload carried by a multiSig wrapper.
// Action is intentionally generic because HyperCore permits the wrapper around
// every normal protocol action; callers should use SDK signing action types.
type MultiSigPayload struct {
	MultiSigUser string `json:"multiSigUser"`
	OuterSigner  string `json:"outerSigner"`
	Action       any    `json:"action"`
}

func (p MultiSigPayload) MarshalMsgpack() ([]byte, error) {
	return marshalMap(func(e *msgpack.Encoder) error {
		if err := e.EncodeMapLen(3); err != nil {
			return err
		}
		for _, pair := range []struct {
			key string
			val any
		}{{"multiSigUser", p.MultiSigUser}, {"outerSigner", p.OuterSigner}, {"action", p.Action}} {
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

// MultiSigEnvelopeAction is the wire wrapper submitted to Exchange.
type MultiSigEnvelopeAction struct {
	SignatureChainID string             `json:"signatureChainId"`
	Signatures       []CompactSignature `json:"signatures"`
	Payload          MultiSigPayload    `json:"payload"`
}

func (a MultiSigEnvelopeAction) MarshalJSON() ([]byte, error) {
	return json.Marshal(struct {
		Type             string             `json:"type"`
		SignatureChainID string             `json:"signatureChainId"`
		Signatures       []CompactSignature `json:"signatures"`
		Payload          MultiSigPayload    `json:"payload"`
	}{"multiSig", a.SignatureChainID, a.Signatures, a.Payload})
}

// multiSigEnvelopeBody is deliberately separate because the outer EIP-712
// signature hashes the wrapper without its wire-only type field.
type multiSigEnvelopeBody MultiSigEnvelopeAction

func (a multiSigEnvelopeBody) MarshalMsgpack() ([]byte, error) {
	return marshalMap(func(e *msgpack.Encoder) error {
		if err := e.EncodeMapLen(3); err != nil {
			return err
		}
		for _, pair := range []struct {
			key string
			val any
		}{{"signatureChainId", a.SignatureChainID}, {"signatures", a.Signatures}, {"payload", a.Payload}} {
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

// ComputeMultiSigL1PayloadDigest constructs the exact per-authorized-user
// digest for an L1 action. Both addresses are normalized to lower case before
// hashing, as in the official SDK.
func ComputeMultiSigL1PayloadDigest(action any, multiSigUser, outerSigner common.Address, nonce uint64, vaultAddress *common.Address, expiresAfter *uint64, isMainnet bool) (signer.Digest, error) {
	if action == nil || multiSigUser == (common.Address{}) || outerSigner == (common.Address{}) || nonce == 0 {
		return signer.Digest{}, fmt.Errorf("multi-sig L1 action, user, leader, and nonce are required")
	}
	return ComputeL1ActionDigest([]any{strings.ToLower(multiSigUser.Hex()), strings.ToLower(outerSigner.Hex()), action}, nonce, vaultAddress, expiresAfter, isMainnet)
}

// ComputeMultiSigUserActionDigest constructs an authorized user's EIP-712
// contribution to a multi-sig user-signed action.
func ComputeMultiSigUserActionDigest(action UserSignedAction, multiSigUser, outerSigner common.Address, isMainnet bool) (signer.Digest, error) {
	if action == nil || multiSigUser == (common.Address{}) || outerSigner == (common.Address{}) {
		return signer.Digest{}, fmt.Errorf("multi-sig user action, user, and leader are required")
	}
	primaryType, fields, message, err := action.userSignedTypedData(isMainnet)
	if err != nil {
		return signer.Digest{}, err
	}
	if len(fields) == 0 {
		return signer.Digest{}, fmt.Errorf("multi-sig user action has no EIP-712 fields")
	}
	extended := make([]apitypes.Type, 0, len(fields)+2)
	extended = append(extended, fields[0], apitypes.Type{Name: "payloadMultiSigUser", Type: "address"}, apitypes.Type{Name: "outerSigner", Type: "address"})
	extended = append(extended, fields[1:]...)
	message["payloadMultiSigUser"] = strings.ToLower(multiSigUser.Hex())
	message["outerSigner"] = strings.ToLower(outerSigner.Hex())
	return userActionTypedDataDigest(primaryType, extended, message)
}

// ComputeMultiSigEnvelopeDigest constructs the final leader signature digest
// around a fully collected multi-sig wrapper.
func ComputeMultiSigEnvelopeDigest(action MultiSigEnvelopeAction, nonce uint64, vaultAddress *common.Address, expiresAfter *uint64, isMainnet bool) (signer.Digest, error) {
	if nonce == 0 || action.SignatureChainID != "0x66eee" || action.Payload.MultiSigUser == "" || action.Payload.OuterSigner == "" || len(action.Signatures) == 0 {
		return signer.Digest{}, fmt.Errorf("invalid multi-sig envelope")
	}
	components, err := L1ActionComponents(multiSigEnvelopeBody(action), nonce, vaultAddress, expiresAfter)
	if err != nil {
		return signer.Digest{}, err
	}
	data := apitypes.TypedData{
		Types: apitypes.Types{
			"EIP712Domain": {
				{Name: "name", Type: "string"}, {Name: "version", Type: "string"}, {Name: "chainId", Type: "uint256"}, {Name: "verifyingContract", Type: "address"},
			},
			"HyperliquidTransaction:SendMultiSig": {
				{Name: "hyperliquidChain", Type: "string"}, {Name: "multiSigActionHash", Type: "bytes32"}, {Name: "nonce", Type: "uint64"},
			},
		},
		PrimaryType: "HyperliquidTransaction:SendMultiSig",
		Domain:      apitypes.TypedDataDomain{Name: "HyperliquidSignTransaction", Version: "1", ChainId: math.NewHexOrDecimal256(int64(defaultUserActionSignatureChainID)), VerifyingContract: "0x0000000000000000000000000000000000000000"},
		Message:     apitypes.TypedDataMessage{"hyperliquidChain": userActionNetwork(isMainnet), "multiSigActionHash": "0x" + hex.EncodeToString(components.ConnectionID[:]), "nonce": new(big.Int).SetUint64(nonce)},
	}
	raw, _, err := apitypes.TypedDataAndHash(data)
	if err != nil {
		return signer.Digest{}, fmt.Errorf("hash multi-sig envelope: %w", err)
	}
	var digest signer.Digest
	copy(digest[:], raw)
	return digest, nil
}
