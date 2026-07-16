package exchange

import (
	"context"
	"encoding/hex"
	"fmt"
	"sort"
	"strings"

	"github.com/Apexllcc/hyperliquid-go-sdk/signer"
	"github.com/Apexllcc/hyperliquid-go-sdk/signing"
	"github.com/ethereum/go-ethereum/common"
)

// MultiSigConfig supplies the caller-owned authorized signer set for one
// multiSig submission. The Exchange client retains only its configured
// DigestSigner; this value is never stored by the client.
//
// AuthorizedUsers is the expected on-chain authorization set supplied by the
// caller. It lets the SDK locally reject duplicate or unauthorized signers;
// HyperCore remains the final authority for current authorization and threshold.
type MultiSigConfig struct {
	MultiSigUser    common.Address
	Leader          signer.DigestSigner
	AuthorizedUsers []common.Address
	Signers         []signer.DigestSigner
	Threshold       uint8
	// LeaderOwner is required only when Leader is an API wallet. It identifies
	// the authorized user which owns that API wallet; this relationship is
	// ultimately validated by HyperCore when the action is submitted.
	LeaderOwner *common.Address
}

// SubmitMultiSigL1 collects verified signatures for a canonical L1 action,
// deterministically sorts the inner signatures by signer address, and submits
// the final leader-signed wrapper. Exchange actions are never retried.
func (c *Client) SubmitMultiSigL1(ctx context.Context, config MultiSigConfig, action any) (ActionResponse, error) {
	if action == nil {
		return ActionResponse{}, fmt.Errorf("multi-sig L1 action is required")
	}
	if c.nonce == nil {
		return ActionResponse{}, fmt.Errorf("nonce manager is required")
	}
	if err := config.validate(); err != nil {
		return ActionResponse{}, err
	}
	nonceValue, err := c.nonce.Next(ctx, config.Leader.Address())
	if err != nil {
		return ActionResponse{}, err
	}
	return c.submitMultiSigL1AtNonce(ctx, config, action, nonceValue, c.submit.vaultAddress, c.submit.expiresAfter)
}

// SubmitMultiSigUserAction performs the corresponding multi-signature flow
// for an SDK-defined user-signed EIP-712 action.
func (c *Client) SubmitMultiSigUserAction(ctx context.Context, config MultiSigConfig, action signing.UserSignedAction) (ActionResponse, error) {
	if action == nil {
		return ActionResponse{}, fmt.Errorf("multi-sig user action is required")
	}
	if err := config.validate(); err != nil {
		return ActionResponse{}, err
	}
	nonceValue := action.ActionNonce()
	if nonceValue == 0 {
		return ActionResponse{}, fmt.Errorf("multi-sig user action nonce is required")
	}
	inner, err := c.collectMultiSigUserSignatures(ctx, config, action)
	if err != nil {
		return ActionResponse{}, err
	}
	wireAction, err := actionWire(action, c.network == "mainnet")
	if err != nil {
		return ActionResponse{}, err
	}
	envelope := signing.MultiSigEnvelopeAction{SignatureChainID: "0x66eee", Signatures: inner, Payload: signing.MultiSigPayload{MultiSigUser: strings.ToLower(config.MultiSigUser.Hex()), OuterSigner: strings.ToLower(config.Leader.Address().Hex()), Action: wireAction}}
	var vaultAddress *common.Address
	if !action.OmitOuterVaultAddress() {
		vaultAddress = c.submit.vaultAddress
	}
	digest, err := signing.ComputeMultiSigEnvelopeDigest(envelope, nonceValue, vaultAddress, nil, c.network == "mainnet")
	if err != nil {
		return ActionResponse{}, err
	}
	outer, err := signAndVerify(ctx, config.Leader, digest)
	if err != nil {
		return ActionResponse{}, err
	}
	return c.post(ctx, multiSigSubmission{Action: envelope, Nonce: nonceValue, Signature: wireSignatureFrom(outer), VaultAddress: vaultAddress})
}

func (c *Client) submitMultiSigL1AtNonce(ctx context.Context, config MultiSigConfig, action any, nonceValue uint64, vaultAddress *common.Address, expiresAfter *uint64) (ActionResponse, error) {
	if nonceValue == 0 {
		return ActionResponse{}, fmt.Errorf("multi-sig L1 nonce is required")
	}
	inner, err := c.collectMultiSigL1Signatures(ctx, config, action, nonceValue, vaultAddress, expiresAfter)
	if err != nil {
		return ActionResponse{}, err
	}
	envelope := signing.MultiSigEnvelopeAction{SignatureChainID: "0x66eee", Signatures: inner, Payload: signing.MultiSigPayload{MultiSigUser: strings.ToLower(config.MultiSigUser.Hex()), OuterSigner: strings.ToLower(config.Leader.Address().Hex()), Action: action}}
	digest, err := signing.ComputeMultiSigEnvelopeDigest(envelope, nonceValue, vaultAddress, expiresAfter, c.network == "mainnet")
	if err != nil {
		return ActionResponse{}, err
	}
	outer, err := signAndVerify(ctx, config.Leader, digest)
	if err != nil {
		return ActionResponse{}, err
	}
	return c.post(ctx, multiSigSubmission{Action: envelope, Nonce: nonceValue, Signature: wireSignatureFrom(outer), VaultAddress: c.outerVaultAddress(vaultAddress), ExpiresAfter: expiresAfter})
}

func (c *Client) collectMultiSigL1Signatures(ctx context.Context, config MultiSigConfig, action any, nonceValue uint64, vaultAddress *common.Address, expiresAfter *uint64) ([]signing.CompactSignature, error) {
	entries := make([]multiSigSignatureEntry, 0, len(config.Signers))
	for _, contribution := range config.Signers {
		digest, err := signing.ComputeMultiSigL1PayloadDigest(action, config.MultiSigUser, config.Leader.Address(), nonceValue, vaultAddress, expiresAfter, c.network == "mainnet")
		if err != nil {
			return nil, err
		}
		sig, err := signAndVerify(ctx, contribution, digest)
		if err != nil {
			return nil, err
		}
		compact, err := signing.CompactSignatureFromSignature(sig)
		if err != nil {
			return nil, err
		}
		entries = append(entries, multiSigSignatureEntry{address: contribution.Address(), signature: compact})
	}
	return sortMultiSigSignatures(entries), nil
}

func (c *Client) collectMultiSigUserSignatures(ctx context.Context, config MultiSigConfig, action signing.UserSignedAction) ([]signing.CompactSignature, error) {
	entries := make([]multiSigSignatureEntry, 0, len(config.Signers))
	for _, contribution := range config.Signers {
		digest, err := signing.ComputeMultiSigUserActionDigest(action, config.MultiSigUser, config.Leader.Address(), c.network == "mainnet")
		if err != nil {
			return nil, err
		}
		sig, err := signAndVerify(ctx, contribution, digest)
		if err != nil {
			return nil, err
		}
		compact, err := signing.CompactSignatureFromSignature(sig)
		if err != nil {
			return nil, err
		}
		entries = append(entries, multiSigSignatureEntry{address: contribution.Address(), signature: compact})
	}
	return sortMultiSigSignatures(entries), nil
}

type multiSigSignatureEntry struct {
	address   common.Address
	signature signing.CompactSignature
}

func sortMultiSigSignatures(entries []multiSigSignatureEntry) []signing.CompactSignature {
	sort.Slice(entries, func(i, j int) bool {
		return strings.ToLower(entries[i].address.Hex()) < strings.ToLower(entries[j].address.Hex())
	})
	result := make([]signing.CompactSignature, len(entries))
	for i, entry := range entries {
		result[i] = entry.signature
	}
	return result
}

func (c MultiSigConfig) validate() error {
	if c.MultiSigUser == (common.Address{}) || c.Leader == nil || len(c.AuthorizedUsers) == 0 || len(c.AuthorizedUsers) > 10 || c.Threshold == 0 || int(c.Threshold) > len(c.AuthorizedUsers) || len(c.Signers) < int(c.Threshold) || len(c.Signers) > len(c.AuthorizedUsers) {
		return fmt.Errorf("invalid multi-sig configuration")
	}
	authorized := make(map[common.Address]struct{}, len(c.AuthorizedUsers))
	for _, address := range c.AuthorizedUsers {
		if address == (common.Address{}) {
			return fmt.Errorf("multi-sig authorized user is required")
		}
		if _, exists := authorized[address]; exists {
			return fmt.Errorf("multi-sig authorized users must be unique")
		}
		authorized[address] = struct{}{}
	}
	if c.LeaderOwner == nil {
		if _, ok := authorized[c.Leader.Address()]; !ok {
			return fmt.Errorf("multi-sig leader is not authorized")
		}
	} else {
		if *c.LeaderOwner == (common.Address{}) {
			return fmt.Errorf("multi-sig leader owner is required")
		}
		if _, ok := authorized[*c.LeaderOwner]; !ok {
			return fmt.Errorf("multi-sig leader owner is not authorized")
		}
	}
	contributors := make(map[common.Address]struct{}, len(c.Signers))
	for _, contribution := range c.Signers {
		if contribution == nil {
			return fmt.Errorf("multi-sig signer is required")
		}
		address := contribution.Address()
		if _, ok := authorized[address]; !ok {
			return fmt.Errorf("multi-sig signer %s is not authorized", address)
		}
		if _, exists := contributors[address]; exists {
			return fmt.Errorf("multi-sig signers must be unique")
		}
		contributors[address] = struct{}{}
	}
	return nil
}

func signAndVerify(ctx context.Context, s signer.DigestSigner, digest signer.Digest) (signer.Signature, error) {
	signature, err := s.SignDigest(ctx, digest)
	if err != nil {
		return signer.Signature{}, fmt.Errorf("sign digest: %w", err)
	}
	if err := signer.Verify(digest, signature, s.Address()); err != nil {
		return signer.Signature{}, fmt.Errorf("verify signature: %w", err)
	}
	return signature, nil
}

func wireSignatureFrom(signature signer.Signature) wireSignature {
	v, _ := signer.NormalizeRecoveryID(signature.V)
	return wireSignature{R: "0x" + hex.EncodeToString(signature.R[:]), S: "0x" + hex.EncodeToString(signature.S[:]), V: v + 27}
}

type multiSigSubmission struct {
	Action       signing.MultiSigEnvelopeAction `json:"action"`
	Nonce        uint64                         `json:"nonce"`
	Signature    wireSignature                  `json:"signature"`
	VaultAddress *common.Address                `json:"vaultAddress,omitempty"`
	ExpiresAfter *uint64                        `json:"expiresAfter,omitempty"`
}

func actionWire(action signing.UserSignedAction, isMainnet bool) (any, error) {
	// The concrete wire struct preserves field order for the outer MessagePack
	// hash; round-tripping through a map would lose integer precision.
	return signing.MultiSigUserPayloadWire(action, isMainnet)
}
