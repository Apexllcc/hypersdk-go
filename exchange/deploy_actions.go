package exchange

import (
	"context"
	"fmt"

	"github.com/Apexllcc/hyperliquid-go-sdk/signing"
)

// SubmitPerpDeploy submits one sealed, strongly typed HIP-3 deployer action.
// Deployment actions are L1 actions signed by the master/API wallet, rather
// than by a configured trading vault. The configured vault is nevertheless
// retained as outer routing metadata, exactly as in the official Python SDK.
// No Exchange action is retried after a network failure.
func (c *Client) SubmitPerpDeploy(ctx context.Context, variant signing.PerpDeployVariant) (ActionResponse, error) {
	if variant == nil {
		return ActionResponse{}, fmt.Errorf("perpDeploy variant is required")
	}
	return c.submitL1For(ctx, signing.PerpDeployAction{Variant: variant}, nil, c.submit.expiresAfter)
}

// SubmitSpotDeploy submits one sealed, strongly typed HIP-1/HIP-2 deployer
// action. Deployments are L1 actions; they are never represented as
// User-Signed EIP-712 actions and never retried.
func (c *Client) SubmitSpotDeploy(ctx context.Context, variant signing.SpotDeployVariant) (ActionResponse, error) {
	if variant == nil {
		return ActionResponse{}, fmt.Errorf("spotDeploy variant is required")
	}
	return c.submitL1For(ctx, signing.SpotDeployAction{Variant: variant}, nil, c.submit.expiresAfter)
}
