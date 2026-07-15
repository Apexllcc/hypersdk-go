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
	"github.com/ethereum/go-ethereum/common"
)

// ActionResponse is the common Exchange response envelope.
type ActionResponse struct {
	Status   string             `json:"status"`
	Response ActionResponseBody `json:"response"`
}
type ActionResponseBody struct {
	Type string          `json:"type"`
	Data json.RawMessage `json:"data"`
}

// submitL1 is the sole L1 action submission path. It intentionally does not
// retry: a network failure does not prove the action was not executed.
func (c *Client) submitL1(ctx context.Context, action any) (ActionResponse, error) {
	if c.signer == nil {
		return ActionResponse{}, signer.ErrSignerRequired
	}
	nonceValue, err := c.nonce.Next(ctx, c.signer.Address())
	if err != nil {
		return ActionResponse{}, err
	}
	digest, err := signing.ComputeL1ActionDigest(action, nonceValue, c.submit.vaultAddress, c.submit.expiresAfter, c.network == "mainnet")
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
	}{Action: action, Nonce: nonceValue, Signature: wireSignature{R: "0x" + hex.EncodeToString(signature.R[:]), S: "0x" + hex.EncodeToString(signature.S[:]), V: v + 27}, VaultAddress: c.submit.vaultAddress, ExpiresAfter: c.submit.expiresAfter}
	return c.post(ctx, payload)
}

type wireSignature struct {
	R string `json:"r"`
	S string `json:"s"`
	V uint8  `json:"v"`
}

func (c *Client) post(ctx context.Context, payload any) (ActionResponse, error) {
	ctx, cancel := context.WithTimeout(ctx, c.timeout)
	defer cancel()
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
	defer response.Body.Close()
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
	return result, nil
}
