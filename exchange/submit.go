package exchange

import (
	"bytes"
	"context"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"

	"github.com/Apexllcc/hyperliquid-go-sdk/internal/hlerr"
	"github.com/Apexllcc/hyperliquid-go-sdk/signer"
	"github.com/Apexllcc/hyperliquid-go-sdk/signing"
	"github.com/Apexllcc/hyperliquid-go-sdk/transport"
	"github.com/ethereum/go-ethereum/common"
)

// submitL1 is the sole L1 action submission path. It intentionally does not
// retry: a network failure does not prove the action was not executed.
func (c *Client) submitL1(ctx context.Context, action any) (ActionResponse, error) {
	return c.submitL1For(ctx, action, c.submit.vaultAddress, c.submit.expiresAfter)
}

// submitL1For is used by L1 actions that must be signed outside a configured
// trading vault/subaccount (for example vault and subaccount fund transfers).
func (c *Client) submitL1For(ctx context.Context, action any, vaultAddress *common.Address, expiresAfter *uint64) (ActionResponse, error) {
	if c.signer == nil {
		return ActionResponse{}, signer.ErrSignerRequired
	}
	if c.nonce == nil {
		return ActionResponse{}, fmt.Errorf("nonce manager is required")
	}
	nonceValue, err := c.nonce.Next(ctx, c.signer.Address())
	if err != nil {
		return ActionResponse{}, err
	}
	digest, err := signing.ComputeL1ActionDigest(action, nonceValue, vaultAddress, expiresAfter, c.network == "mainnet")
	if err != nil {
		return ActionResponse{}, err
	}
	signature, err := c.signer.SignDigest(ctx, digest)
	if err != nil {
		return ActionResponse{}, fmt.Errorf("sign digest: %w", err)
	}
	if err := signer.Verify(digest, signature, c.signer.Address()); err != nil {
		return ActionResponse{}, fmt.Errorf("verify signature: %w", err)
	}
	v, err := signer.NormalizeRecoveryID(signature.V)
	if err != nil {
		return ActionResponse{}, err
	}
	payload := struct {
		Action       any             `json:"action"`
		Nonce        uint64          `json:"nonce"`
		Signature    wireSignature   `json:"signature"`
		VaultAddress *common.Address `json:"vaultAddress,omitempty"`
		ExpiresAfter *uint64         `json:"expiresAfter,omitempty"`
	}{Action: action, Nonce: nonceValue, Signature: wireSignature{R: "0x" + hex.EncodeToString(signature.R[:]), S: "0x" + hex.EncodeToString(signature.S[:]), V: v + 27}, VaultAddress: c.outerVaultAddress(vaultAddress), ExpiresAfter: expiresAfter}
	return c.post(ctx, payload)
}

// submitUserSigned submits an EIP-712 user action. User-signed digests never
// include an L1 vault marker or expiresAfter. The outer vault address remains
// protocol action-specific routing metadata, matching the official SDK.
func (c *Client) submitUserSigned(ctx context.Context, action signing.UserSignedAction) (ActionResponse, error) {
	if c.signer == nil {
		return ActionResponse{}, signer.ErrSignerRequired
	}
	rawAction, err := signing.MarshalUserSignedAction(action, c.network == "mainnet")
	if err != nil {
		return ActionResponse{}, err
	}
	digest, err := signing.ComputeUserActionDigest(action, c.network == "mainnet")
	if err != nil {
		return ActionResponse{}, err
	}
	signature, err := c.signer.SignDigest(ctx, digest)
	if err != nil {
		return ActionResponse{}, fmt.Errorf("sign digest: %w", err)
	}
	if err := signer.Verify(digest, signature, c.signer.Address()); err != nil {
		return ActionResponse{}, fmt.Errorf("verify signature: %w", err)
	}
	v, err := signer.NormalizeRecoveryID(signature.V)
	if err != nil {
		return ActionResponse{}, err
	}
	var vaultAddress *common.Address
	if !action.OmitOuterVaultAddress() {
		vaultAddress = c.submit.vaultAddress
	}
	return c.post(ctx, struct {
		Action       json.RawMessage `json:"action"`
		Nonce        uint64          `json:"nonce"`
		Signature    wireSignature   `json:"signature"`
		VaultAddress *common.Address `json:"vaultAddress,omitempty"`
	}{Action: rawAction, Nonce: action.ActionNonce(), Signature: wireSignature{R: "0x" + hex.EncodeToString(signature.R[:]), S: "0x" + hex.EncodeToString(signature.S[:]), V: v + 27}, VaultAddress: vaultAddress})
}

// outerVaultAddress preserves the official Exchange payload routing behavior:
// a non-trading L1 action hashes the explicit signing vault (often nil), while
// the outer request retains this client's configured vault/subaccount address.
func (c *Client) outerVaultAddress(signingVault *common.Address) *common.Address {
	if signingVault != nil {
		return signingVault
	}
	return c.submit.vaultAddress
}

type wireSignature struct {
	R string `json:"r"`
	S string `json:"s"`
	V uint8  `json:"v"`
}

func (c *Client) post(ctx context.Context, payload any) (ActionResponse, error) {
	ctx, cancel := context.WithTimeout(ctx, c.timeout)
	defer cancel()
	if c.request != nil {
		var result ActionResponse
		if err := c.request.Request(ctx, transport.RequestAction, payload, &result); err != nil {
			return ActionResponse{}, err
		}
		if result.Error != nil {
			return result, result.Error
		}
		return result, nil
	}
	raw, err := json.Marshal(payload)
	if err != nil {
		return ActionResponse{}, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL, bytes.NewReader(raw))
	if err != nil {
		return ActionResponse{}, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("User-Agent", c.userAgent)
	response, err := c.transport.Do(ctx, req)
	if err != nil {
		if response != nil && response.Body != nil {
			_ = response.Body.Close()
		}
		return ActionResponse{}, err
	}
	if response == nil || response.Body == nil {
		return ActionResponse{}, fmt.Errorf("%w: nil HTTP response", hlerr.ErrUnexpectedResponse)
	}
	defer func() { _ = response.Body.Close() }()
	body, err := io.ReadAll(response.Body)
	if err != nil {
		return ActionResponse{}, err
	}
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return ActionResponse{}, &hlerr.APIError{StatusCode: response.StatusCode, Message: string(body), Body: body}
	}
	var result ActionResponse
	if err := json.Unmarshal(body, &result); err != nil {
		return ActionResponse{}, fmt.Errorf("%w: %w", hlerr.ErrUnexpectedResponse, err)
	}
	if result.Error != nil {
		return result, result.Error
	}
	return result, nil
}
