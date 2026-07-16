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

// AgentSendAssetRequest transfers an asset as an approved agent through the
// L1 path. FromSubAccount selects the source account; it is not an outer
// vault field and therefore does not inherit this Client's configured vault.
type AgentSendAssetRequest struct {
	Destination                      common.Address
	SourceDEX, DestinationDEX, Token string
	Amount                           decimal.Decimal
	FromSubAccount                   *common.Address
}

func (c *Client) AgentSendAsset(ctx context.Context, request AgentSendAssetRequest) (ActionResponse, error) {
	if err := validateTransfer(request.Destination, request.Amount); err != nil {
		return ActionResponse{}, err
	}
	if request.Token == "" {
		return ActionResponse{}, fmt.Errorf("asset token is required")
	}
	fromSubAccount := ""
	if request.FromSubAccount != nil {
		if *request.FromSubAccount == (common.Address{}) {
			return ActionResponse{}, fmt.Errorf("from subaccount address is invalid")
		}
		fromSubAccount = request.FromSubAccount.Hex()
	}
	nonceValue, err := c.userActionNonce(ctx)
	if err != nil {
		return ActionResponse{}, err
	}
	action := signing.AgentSendAssetAction{Destination: request.Destination.Hex(), SourceDEX: request.SourceDEX, DestinationDEX: request.DestinationDEX, Token: request.Token, Amount: request.Amount.String(), FromSubAccount: fromSubAccount, Nonce: nonceValue}
	return c.submitL1AtNonceWithOuter(ctx, action, nonceValue, nil, c.submit.expiresAfter, nil)
}

// AQAV2Role is a role in an aligned quote asset v2 deployment.
type AQAV2Role = signing.AQAV2Role

const (
	AQAV2RoleTechnical = signing.AQAV2RoleTechnical
	AQAV2RoleTreasury  = signing.AQAV2RoleTreasury
)

type AuthorizeAQAV2RoleRequest struct {
	Token uint64
	Role  AQAV2Role
}

// AuthorizeAQAV2Role grants an aligned quote asset v2 technical or treasury
// role using the master/API wallet's L1 signature.
func (c *Client) AuthorizeAQAV2Role(ctx context.Context, request AuthorizeAQAV2RoleRequest) (ActionResponse, error) {
	action := signing.AuthorizeAQAV2RoleAction{Token: request.Token, Role: signing.AQAV2Role(request.Role)}
	return c.submitL1WithoutVault(ctx, action, c.submit.expiresAfter)
}

// HIP3LiquidatorTransferRequest moves quote-token micros into or out of a
// HIP-3 DEX backstop liquidator. NTL must be in 1,000-token increments.
type HIP3LiquidatorTransferRequest struct {
	DEX       string
	NTL       uint64
	IsDeposit bool
}

func (c *Client) HIP3LiquidatorTransfer(ctx context.Context, request HIP3LiquidatorTransferRequest) (ActionResponse, error) {
	action := signing.HIP3LiquidatorTransferAction{DEX: request.DEX, NTL: request.NTL, IsDeposit: request.IsDeposit}
	return c.submitL1WithoutVault(ctx, action, c.submit.expiresAfter)
}

// UserOutcomeRequest is a strongly typed union for outcome-share actions.
// Exactly one field must be non-nil. Nil merge amounts mean all available
// shares, matching the protocol's explicit nullable field.
type UserOutcomeRequest struct {
	SplitOutcome  *SplitOutcomeRequest
	MergeOutcome  *MergeOutcomeRequest
	MergeQuestion *MergeQuestionRequest
	NegateOutcome *NegateOutcomeRequest
}

type SplitOutcomeRequest struct {
	Outcome uint64
	Amount  decimal.Decimal
}
type MergeOutcomeRequest struct {
	Outcome uint64
	Amount  *decimal.Decimal
}
type MergeQuestionRequest struct {
	Question uint64
	Amount   *decimal.Decimal
}
type NegateOutcomeRequest struct {
	Question uint64
	Outcome  uint64
	Amount   decimal.Decimal
}

// UserOutcome submits one split, merge, or negate action over outcome shares.
// It follows the ordinary L1 envelope and therefore honors this Client's vault
// and expiresAfter configuration.
func (c *Client) UserOutcome(ctx context.Context, request UserOutcomeRequest) (ActionResponse, error) {
	action := signing.UserOutcomeAction{}
	if request.SplitOutcome != nil {
		action.SplitOutcome = &signing.SplitOutcome{Outcome: request.SplitOutcome.Outcome, Amount: request.SplitOutcome.Amount.String()}
	}
	if request.MergeOutcome != nil {
		amount := decimalString(request.MergeOutcome.Amount)
		action.MergeOutcome = &signing.MergeOutcome{Outcome: request.MergeOutcome.Outcome, Amount: amount}
	}
	if request.MergeQuestion != nil {
		amount := decimalString(request.MergeQuestion.Amount)
		action.MergeQuestion = &signing.MergeQuestion{Question: request.MergeQuestion.Question, Amount: amount}
	}
	if request.NegateOutcome != nil {
		action.NegateOutcome = &signing.NegateOutcome{Question: request.NegateOutcome.Question, Outcome: request.NegateOutcome.Outcome, Amount: request.NegateOutcome.Amount.String()}
	}
	if err := actionValidationError(action); err != nil {
		return ActionResponse{}, err
	}
	return c.submitL1(ctx, action)
}

func decimalString(value *decimal.Decimal) *string {
	if value == nil {
		return nil
	}
	formatted := value.String()
	return &formatted
}

func actionValidationError(action signing.L1Action) error {
	_, err := action.MarshalMsgpack()
	return err
}

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
