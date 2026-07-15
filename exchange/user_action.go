package exchange

import (
	"context"
	"fmt"

	"github.com/Apexllcc/hyperliquid-go-sdk/signing"
	"github.com/ethereum/go-ethereum/common"
	"github.com/shopspring/decimal"
)

// USDSendRequest transfers a decimal Core USDC amount to Destination.
type USDSendRequest struct {
	Destination common.Address
	Amount      decimal.Decimal
}

func (c *Client) SendUSD(ctx context.Context, request USDSendRequest) (ActionResponse, error) {
	if err := validateTransfer(request.Destination, request.Amount); err != nil {
		return ActionResponse{}, err
	}
	nonceValue, err := c.userActionNonce(ctx)
	if err != nil {
		return ActionResponse{}, err
	}
	return c.submitUserSigned(ctx, signing.USDSendAction{Destination: request.Destination, Amount: request.Amount.String(), Time: nonceValue})
}

// SpotSendRequest transfers a decimal amount of Token to Destination.
type SpotSendRequest struct {
	Destination common.Address
	Token       string
	Amount      decimal.Decimal
}

func (c *Client) SendSpot(ctx context.Context, request SpotSendRequest) (ActionResponse, error) {
	if err := validateTransfer(request.Destination, request.Amount); err != nil {
		return ActionResponse{}, err
	}
	if request.Token == "" {
		return ActionResponse{}, fmt.Errorf("spot token is required")
	}
	nonceValue, err := c.userActionNonce(ctx)
	if err != nil {
		return ActionResponse{}, err
	}
	return c.submitUserSigned(ctx, signing.SpotSendAction{Destination: request.Destination, Token: request.Token, Amount: request.Amount.String(), Time: nonceValue})
}

// WithdrawRequest initiates a bridge withdrawal of Core USDC.
type WithdrawRequest struct {
	Destination common.Address
	Amount      decimal.Decimal
}

func (c *Client) WithdrawFromBridge(ctx context.Context, request WithdrawRequest) (ActionResponse, error) {
	if err := validateTransfer(request.Destination, request.Amount); err != nil {
		return ActionResponse{}, err
	}
	nonceValue, err := c.userActionNonce(ctx)
	if err != nil {
		return ActionResponse{}, err
	}
	return c.submitUserSigned(ctx, signing.WithdrawFromBridgeAction{Destination: request.Destination, Amount: request.Amount.String(), Time: nonceValue})
}

// USDClassTransferRequest moves Core USDC between the spot and perp classes.
type USDClassTransferRequest struct {
	Amount decimal.Decimal
	ToPerp bool
}

func (c *Client) TransferUSDClass(ctx context.Context, request USDClassTransferRequest) (ActionResponse, error) {
	if !request.Amount.IsPositive() {
		return ActionResponse{}, fmt.Errorf("transfer amount must be positive")
	}
	nonceValue, err := c.userActionNonce(ctx)
	if err != nil {
		return ActionResponse{}, err
	}
	amount := request.Amount.String()
	if c.submit.vaultAddress != nil {
		amount += " subaccount:" + c.submit.vaultAddress.Hex()
	}
	return c.submitUserSigned(ctx, signing.USDClassTransferAction{Amount: amount, ToPerp: request.ToPerp, Nonce: nonceValue})
}

// SendAssetRequest moves a token between HyperCore DEX namespaces. An empty
// SourceDEX or DestinationDEX denotes the default perpetual DEX.
type SendAssetRequest struct {
	Destination                      common.Address
	SourceDEX, DestinationDEX, Token string
	Amount                           decimal.Decimal
	FromSubAccount                   *common.Address
}

func (c *Client) SendAsset(ctx context.Context, request SendAssetRequest) (ActionResponse, error) {
	if err := validateTransfer(request.Destination, request.Amount); err != nil {
		return ActionResponse{}, err
	}
	if request.Token == "" {
		return ActionResponse{}, fmt.Errorf("asset token is required")
	}
	fromSubAccount := request.FromSubAccount
	if fromSubAccount != nil && *fromSubAccount == (common.Address{}) {
		return ActionResponse{}, fmt.Errorf("from subaccount address is invalid")
	}
	if c.submit.vaultAddress != nil {
		if fromSubAccount != nil && *fromSubAccount != *c.submit.vaultAddress {
			return ActionResponse{}, fmt.Errorf("from subaccount does not match configured vault address")
		}
		fromSubAccount = c.submit.vaultAddress
	}
	nonceValue, err := c.userActionNonce(ctx)
	if err != nil {
		return ActionResponse{}, err
	}
	return c.submitUserSigned(ctx, signing.SendAssetAction{Destination: request.Destination, SourceDEX: request.SourceDEX, DestinationDEX: request.DestinationDEX, Token: request.Token, Amount: request.Amount.String(), FromSubAccount: fromSubAccount, Nonce: nonceValue})
}

// ApproveAgentRequest authorizes an API wallet. AgentName is optional; a nil
// name represents the one unnamed agent that Hyperliquid permits per account.
type ApproveAgentRequest struct {
	AgentAddress common.Address
	AgentName    *string
}

func (c *Client) ApproveAgent(ctx context.Context, request ApproveAgentRequest) (ActionResponse, error) {
	if request.AgentAddress == (common.Address{}) {
		return ActionResponse{}, fmt.Errorf("agent address is required")
	}
	nonceValue, err := c.userActionNonce(ctx)
	if err != nil {
		return ActionResponse{}, err
	}
	return c.submitUserSigned(ctx, signing.ApproveAgentAction{AgentAddress: request.AgentAddress, AgentName: request.AgentName, Nonce: nonceValue})
}

// ApproveBuilderFeeRequest permits a builder to charge up to MaxFeeRate (for
// example, "0.001%") on the signing account.
type ApproveBuilderFeeRequest struct {
	Builder    common.Address
	MaxFeeRate string
}

func (c *Client) ApproveBuilderFee(ctx context.Context, request ApproveBuilderFeeRequest) (ActionResponse, error) {
	if request.Builder == (common.Address{}) || request.MaxFeeRate == "" {
		return ActionResponse{}, fmt.Errorf("builder address and maximum fee rate are required")
	}
	nonceValue, err := c.userActionNonce(ctx)
	if err != nil {
		return ActionResponse{}, err
	}
	return c.submitUserSigned(ctx, signing.ApproveBuilderFeeAction{Builder: request.Builder, MaxFeeRate: request.MaxFeeRate, Nonce: nonceValue})
}

func (c *Client) userActionNonce(ctx context.Context) (uint64, error) {
	if c.signer == nil {
		return 0, fmt.Errorf("user-signed action: signer is required")
	}
	if c.nonce == nil {
		return 0, fmt.Errorf("user-signed action: nonce manager is required")
	}
	return c.nonce.Next(ctx, c.signer.Address())
}
func validateTransfer(destination common.Address, amount decimal.Decimal) error {
	if destination == (common.Address{}) {
		return fmt.Errorf("transfer destination is required")
	}
	if !amount.IsPositive() {
		return fmt.Errorf("transfer amount must be positive")
	}
	return nil
}
