package exchange

import (
	"context"
	"fmt"

	"github.com/Apexllcc/hyperliquid-go-sdk/signing"
	"github.com/ethereum/go-ethereum/common"
	"github.com/shopspring/decimal"
)

// UserAbstraction is the public wire value accepted by userSetAbstraction.
type UserAbstraction string

const (
	UserAbstractionDisabled        UserAbstraction = "disabled"
	UserAbstractionUnifiedAccount  UserAbstraction = "unifiedAccount"
	UserAbstractionPortfolioMargin UserAbstraction = "portfolioMargin"
)

// AgentAbstraction is the compact wire value accepted by agentSetAbstraction.
type AgentAbstraction string

const (
	AgentAbstractionDisabled        AgentAbstraction = "i"
	AgentAbstractionUnifiedAccount  AgentAbstraction = "u"
	AgentAbstractionPortfolioMargin AgentAbstraction = "p"
)

// UserDexAbstractionRequest controls deprecated HIP-3 DEX abstraction for a
// user or subaccount. Prefer UserSetAbstraction for new integrations.
type UserDexAbstractionRequest struct {
	User    common.Address
	Enabled bool
}

func (c *Client) UserDexAbstraction(ctx context.Context, request UserDexAbstractionRequest) (ActionResponse, error) {
	if request.User == (common.Address{}) {
		return ActionResponse{}, fmt.Errorf("user address is required")
	}
	nonceValue, err := c.userActionNonce(ctx)
	if err != nil {
		return ActionResponse{}, err
	}
	return c.submitUserSigned(ctx, signing.UserDexAbstractionAction{User: request.User, Enabled: request.Enabled, Nonce: nonceValue})
}

// UserSetAbstractionRequest selects the account abstraction for a user or
// subaccount. It is signed with the master account as EIP-712 user action.
type UserSetAbstractionRequest struct {
	User        common.Address
	Abstraction UserAbstraction
}

func (c *Client) UserSetAbstraction(ctx context.Context, request UserSetAbstractionRequest) (ActionResponse, error) {
	if request.User == (common.Address{}) {
		return ActionResponse{}, fmt.Errorf("user address is required")
	}
	if !validUserAbstraction(request.Abstraction) {
		return ActionResponse{}, fmt.Errorf("unsupported user abstraction %q", request.Abstraction)
	}
	nonceValue, err := c.userActionNonce(ctx)
	if err != nil {
		return ActionResponse{}, err
	}
	return c.submitUserSigned(ctx, signing.UserSetAbstractionAction{User: request.User, Abstraction: string(request.Abstraction), Nonce: nonceValue})
}

// AgentEnableDexAbstraction enables the deprecated HIP-3 abstraction path for
// the signing agent. Prefer AgentSetAbstraction for new integrations.
func (c *Client) AgentEnableDexAbstraction(ctx context.Context) (ActionResponse, error) {
	return c.submitL1(ctx, signing.AgentEnableDexAbstractionAction{})
}

// AgentSetAbstraction selects disabled, unified account, or portfolio margin
// for the signing agent through an L1 action.
func (c *Client) AgentSetAbstraction(ctx context.Context, abstraction AgentAbstraction) (ActionResponse, error) {
	if !validAgentAbstraction(abstraction) {
		return ActionResponse{}, fmt.Errorf("unsupported agent abstraction %q", abstraction)
	}
	return c.submitL1(ctx, signing.AgentSetAbstractionAction{Abstraction: string(abstraction)})
}

// ValidatorL1Stream submits a canonical decimal risk-free rate vote.
func (c *Client) ValidatorL1Stream(ctx context.Context, riskFreeRate string) (ActionResponse, error) {
	if _, err := decimal.NewFromString(riskFreeRate); err != nil || riskFreeRate == "" {
		return ActionResponse{}, fmt.Errorf("risk-free rate must be a decimal string")
	}
	return c.submitL1(ctx, signing.ValidatorL1StreamAction{RiskFreeRate: riskFreeRate})
}

// ClaimRewards claims any available validator rewards through an L1 action.
func (c *Client) ClaimRewards(ctx context.Context) (ActionResponse, error) {
	return c.submitL1(ctx, signing.ClaimRewardsAction{})
}

// SetReferrer associates the signing account with the supplied referral code.
// The protocol specifies an L1 action that is signed as the master account,
// not as a configured trading vault.
func (c *Client) SetReferrer(ctx context.Context, code string) (ActionResponse, error) {
	if code == "" {
		return ActionResponse{}, fmt.Errorf("referral code is required")
	}
	return c.submitL1For(ctx, signing.SetReferrerAction{Code: code}, nil, c.submit.expiresAfter)
}

// CreateSubAccount creates a named subaccount. The master account signs the
// L1 action even when this Client is configured to trade through a subaccount.
func (c *Client) CreateSubAccount(ctx context.Context, name string) (ActionResponse, error) {
	if name == "" {
		return ActionResponse{}, fmt.Errorf("subaccount name is required")
	}
	return c.submitL1For(ctx, signing.CreateSubAccountAction{Name: name}, nil, c.submit.expiresAfter)
}

func validUserAbstraction(abstraction UserAbstraction) bool {
	switch abstraction {
	case UserAbstractionDisabled, UserAbstractionUnifiedAccount, UserAbstractionPortfolioMargin:
		return true
	default:
		return false
	}
}

func validAgentAbstraction(abstraction AgentAbstraction) bool {
	switch abstraction {
	case AgentAbstractionDisabled, AgentAbstractionUnifiedAccount, AgentAbstractionPortfolioMargin:
		return true
	default:
		return false
	}
}
