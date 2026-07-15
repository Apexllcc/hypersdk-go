package exchange

import (
	"context"
	"fmt"

	"github.com/Apexllcc/hyperliquid-go-sdk/signing"
	"github.com/ethereum/go-ethereum/common"
	"github.com/shopspring/decimal"
)

// SubaccountTransferRequest moves whole-USDC units between the master account
// and a subaccount. USD uses the protocol's integer representation.
type SubaccountTransferRequest struct {
	SubaccountUser common.Address
	IsDeposit      bool
	USD            uint64
}

func (c *Client) TransferSubaccountUSD(ctx context.Context, request SubaccountTransferRequest) (ActionResponse, error) {
	if request.SubaccountUser == (common.Address{}) || request.USD == 0 {
		return ActionResponse{}, fmt.Errorf("subaccount address and USD amount are required")
	}
	return c.submitL1For(ctx, signing.SubaccountTransferAction{SubaccountUser: request.SubaccountUser.Hex(), IsDeposit: request.IsDeposit, USD: request.USD}, nil, c.submit.expiresAfter)
}

// SubaccountSpotTransferRequest moves a precision-safe spot token amount
// between the master account and a subaccount.
type SubaccountSpotTransferRequest struct {
	SubaccountUser common.Address
	IsDeposit      bool
	Token          string
	Amount         decimal.Decimal
}

func (c *Client) TransferSubaccountSpot(ctx context.Context, request SubaccountSpotTransferRequest) (ActionResponse, error) {
	if request.SubaccountUser == (common.Address{}) || request.Token == "" || !request.Amount.IsPositive() {
		return ActionResponse{}, fmt.Errorf("subaccount address, token, and positive amount are required")
	}
	return c.submitL1For(ctx, signing.SubaccountSpotTransferAction{SubaccountUser: request.SubaccountUser.Hex(), IsDeposit: request.IsDeposit, Token: request.Token, Amount: request.Amount.String()}, nil, c.submit.expiresAfter)
}

// VaultTransferRequest deposits or withdraws whole-USDC units from a vault.
type VaultTransferRequest struct {
	VaultAddress common.Address
	IsDeposit    bool
	USD          uint64
}

func (c *Client) TransferVaultUSD(ctx context.Context, request VaultTransferRequest) (ActionResponse, error) {
	if request.VaultAddress == (common.Address{}) || request.USD == 0 {
		return ActionResponse{}, fmt.Errorf("vault address and USD amount are required")
	}
	return c.submitL1For(ctx, signing.VaultTransferAction{VaultAddress: request.VaultAddress.Hex(), IsDeposit: request.IsDeposit, USD: request.USD}, nil, c.submit.expiresAfter)
}
