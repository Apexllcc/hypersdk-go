package exchange

import (
	"context"
	"fmt"

	"github.com/Apexllcc/hypersdk-go/signing"
)

// EVMUserModify chooses the account's HyperEVM large-block setting. The
// dual-block protocol signs this action without the configured trading vault.
func (c *Client) EVMUserModify(ctx context.Context, enabled bool) (ActionResponse, error) {
	return c.submitL1For(ctx, signing.EVMUserModifyAction{UsingBigBlocks: enabled}, nil, c.submit.expiresAfter)
}

// UseBigEVMBlocks is a descriptive alias for EVMUserModify.
func (c *Client) UseBigEVMBlocks(ctx context.Context, enabled bool) (ActionResponse, error) {
	return c.EVMUserModify(ctx, enabled)
}

// GossipPriorityBid submits a priority-gossip auction bid. Unlike EVM
// preference actions, this action signs through the configured trading vault.
func (c *Client) GossipPriorityBid(ctx context.Context, slotID uint64, ip string, maxGas uint64) (ActionResponse, error) {
	action := signing.GossipPriorityBidAction{SlotID: slotID, IP: ip, MaxGas: maxGas}
	if _, err := action.MarshalMsgpack(); err != nil {
		return ActionResponse{}, err
	}
	return c.submitL1(ctx, action)
}

// SubmitGossipPriorityBid is a descriptive alias for GossipPriorityBid.
func (c *Client) SubmitGossipPriorityBid(ctx context.Context, slotID uint64, ip string, maxGas uint64) (ActionResponse, error) {
	return c.GossipPriorityBid(ctx, slotID, ip, maxGas)
}

// CValidatorAction submits one sealed, official-Python-compatible
// validator action. Validator actions sign without a configured trading vault.
func (c *Client) CValidatorAction(ctx context.Context, variant signing.CValidatorVariant) (ActionResponse, error) {
	action := signing.CValidatorAction{Variant: variant}
	if _, err := action.MarshalMsgpack(); err != nil {
		return ActionResponse{}, err
	}
	return c.submitL1For(ctx, action, nil, c.submit.expiresAfter)
}

// SubmitCValidatorAction is a descriptive alias for CValidatorAction.
func (c *Client) SubmitCValidatorAction(ctx context.Context, variant signing.CValidatorVariant) (ActionResponse, error) {
	return c.CValidatorAction(ctx, variant)
}

// CSignerAction jails or unjails the current validator signer. It uses the
// validated official-Python L1 schema: its L1 digest has a nil vault marker,
// while the outer request retains the configured vault routing address.
func (c *Client) CSignerAction(ctx context.Context, variant signing.CSignerVariant) (ActionResponse, error) {
	action := signing.CSignerAction{Variant: variant}
	if _, err := action.MarshalMsgpack(); err != nil {
		return ActionResponse{}, err
	}
	return c.submitL1For(ctx, action, nil, c.submit.expiresAfter)
}

// FinalizeEVMContract finalizes a spot token's EVM contract link using a
// sealed proof input. The action is signed outside the configured trading vault.
func (c *Client) FinalizeEVMContract(ctx context.Context, token uint64, input signing.FinalizeEVMContractInput) (ActionResponse, error) {
	action := signing.FinalizeEVMContractAction{Token: token, Input: input}
	if _, err := action.MarshalMsgpack(); err != nil {
		return ActionResponse{}, fmt.Errorf("finalize EVM contract: %w", err)
	}
	return c.submitL1WithoutVault(ctx, action, c.submit.expiresAfter)
}
