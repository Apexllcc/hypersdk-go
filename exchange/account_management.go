package exchange

import (
	"context"
	"fmt"
	"math"
	"strings"

	"github.com/Apexllcc/hypersdk-go/signing"
	"github.com/ethereum/go-ethereum/common"
	"github.com/shopspring/decimal"
)

// ConvertToMultiSigUser changes the current account's authorization set. Pass
// nil to convert an existing multi-sig account back to a normal account. The
// actual authorization update for an existing multi-sig account must itself be
// submitted through SubmitMultiSigUserAction.
func (c *Client) ConvertToMultiSigUser(ctx context.Context, signers *signing.MultiSigSignerSet) (ActionResponse, error) {
	nonceValue, err := c.userActionNonce(ctx)
	if err != nil {
		return ActionResponse{}, err
	}
	return c.submitUserSigned(ctx, signing.ConvertToMultiSigUserAction{Signers: signers, Nonce: nonceValue})
}

// CreateVaultRequest contains the initial vault metadata and an exact USDC
// amount. InitialUSD is serialized as protocol micros; decimal conversion is
// exact and never uses float64.
type CreateVaultRequest struct {
	Name        string
	Description string
	InitialUSD  decimal.Decimal
}

// CreateVault creates and funds a vault. The master account signs the action,
// while an optional configured vault address remains outer routing metadata.
func (c *Client) CreateVault(ctx context.Context, request CreateVaultRequest) (ActionResponse, error) {
	if len(request.Name) < 3 || len(request.Name) > 50 || len(request.Description) < 10 || len(request.Description) > 250 {
		return ActionResponse{}, fmt.Errorf("vault name must be 3-50 characters and description 10-250 characters")
	}
	micros, err := nonNegativeUSDCMicros(request.InitialUSD)
	if err != nil || micros < 100_000_000 {
		return ActionResponse{}, fmt.Errorf("vault initial USD must be at least 100")
	}
	if c.signer == nil || c.nonce == nil {
		return ActionResponse{}, fmt.Errorf("signer and nonce manager are required")
	}
	nonceValue, err := c.nonce.Next(ctx, c.signer.Address())
	if err != nil {
		return ActionResponse{}, err
	}
	return c.submitL1AtNonce(ctx, signing.CreateVaultAction{Name: request.Name, Description: request.Description, InitialUSD: uint64(micros), Nonce: nonceValue}, nonceValue, nil, c.submit.expiresAfter)
}

// VaultModifyRequest changes vault follower controls. At least one field must
// be supplied; nil fields are emitted as protocol nulls.
type VaultModifyRequest struct {
	VaultAddress                         common.Address
	AllowDeposits, AlwaysCloseOnWithdraw *bool
}

func (c *Client) ModifyVault(ctx context.Context, request VaultModifyRequest) (ActionResponse, error) {
	if request.VaultAddress == (common.Address{}) || (request.AllowDeposits == nil && request.AlwaysCloseOnWithdraw == nil) {
		return ActionResponse{}, fmt.Errorf("vault address and at least one vault setting are required")
	}
	return c.submitL1For(ctx, signing.VaultModifyAction{VaultAddress: strings.ToLower(request.VaultAddress.Hex()), AllowDeposits: request.AllowDeposits, AlwaysCloseOnWithdraw: request.AlwaysCloseOnWithdraw}, nil, c.submit.expiresAfter)
}

// VaultDistributionRequest distributes an exact USDC amount; zero closes the
// vault and is intentionally accepted by the protocol.
type VaultDistributionRequest struct {
	VaultAddress common.Address
	USD          decimal.Decimal
}

func (c *Client) DistributeVault(ctx context.Context, request VaultDistributionRequest) (ActionResponse, error) {
	if request.VaultAddress == (common.Address{}) || request.USD.IsNegative() {
		return ActionResponse{}, fmt.Errorf("vault address and non-negative USD amount are required")
	}
	micros, err := nonNegativeUSDCMicros(request.USD)
	if err != nil {
		return ActionResponse{}, err
	}
	return c.submitL1For(ctx, signing.VaultDistributeAction{VaultAddress: strings.ToLower(request.VaultAddress.Hex()), USD: uint64(micros)}, nil, c.submit.expiresAfter)
}

// SubAccountModifyRequest renames a master account's subaccount.
type SubAccountModifyRequest struct {
	SubAccountUser common.Address
	Name           string
}

func (c *Client) ModifySubAccount(ctx context.Context, request SubAccountModifyRequest) (ActionResponse, error) {
	if request.SubAccountUser == (common.Address{}) || len(request.Name) == 0 || len(request.Name) > 16 {
		return ActionResponse{}, fmt.Errorf("subaccount address and a 1-16 character name are required")
	}
	return c.submitL1For(ctx, signing.SubAccountModifyAction{SubAccountUser: strings.ToLower(request.SubAccountUser.Hex()), Name: request.Name}, nil, c.submit.expiresAfter)
}

// SetDisplayName sets a leaderboard display name. An empty string clears it.
func (c *Client) SetDisplayName(ctx context.Context, displayName string) (ActionResponse, error) {
	if len(displayName) > 20 {
		return ActionResponse{}, fmt.Errorf("display name must be at most 20 characters")
	}
	return c.submitL1(ctx, signing.SetDisplayNameAction{DisplayName: displayName})
}

func nonNegativeUSDCMicros(amount decimal.Decimal) (int64, error) {
	if amount.IsNegative() {
		return 0, fmt.Errorf("USD amount must be non-negative")
	}
	scaled := amount.Mul(decimal.New(1, 6))
	if !scaled.Equal(scaled.Truncate(0)) {
		return 0, fmt.Errorf("USD amount supports at most 6 decimal places")
	}
	if !scaled.LessThanOrEqual(decimal.NewFromInt(math.MaxInt64)) {
		return 0, fmt.Errorf("USD amount is outside protocol range")
	}
	return scaled.IntPart(), nil
}
