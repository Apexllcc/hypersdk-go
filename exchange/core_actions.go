package exchange

import (
	"context"
	"fmt"
	"math"

	"github.com/Apexllcc/hypersdk-go/asset"
	"github.com/Apexllcc/hypersdk-go/signing"
	"github.com/Apexllcc/hypersdk-go/types"
	"github.com/ethereum/go-ethereum/common"
	"github.com/shopspring/decimal"
)

// UpdateLeverageRequest selects a perpetual market and its target leverage.
// Market is preferred for HIP-3 markets; specify exactly one of Coin or Market.
type UpdateLeverageRequest struct {
	Coin     string
	Market   *types.MarketRef
	IsCross  bool
	Leverage uint64
}

// UpdateLeverage changes cross or isolated leverage through the L1 action path.
func (c *Client) UpdateLeverage(ctx context.Context, request UpdateLeverageRequest) (ActionResponse, error) {
	if request.Leverage == 0 {
		return ActionResponse{}, fmt.Errorf("leverage must be positive")
	}
	a, err := c.resolvePerpetualAsset(ctx, request.Coin, request.Market)
	if err != nil {
		return ActionResponse{}, err
	}
	return c.submitL1(ctx, signing.UpdateLeverageAction{Asset: a.ID, IsCross: request.IsCross, Leverage: request.Leverage})
}

// UpdateIsolatedMarginRequest adjusts an isolated position's margin in whole
// USDC units. Negative amounts remove margin; conversion to protocol micros is
// exact and never passes through float64.
type UpdateIsolatedMarginRequest struct {
	Coin   string
	Market *types.MarketRef
	IsBuy  bool
	Amount decimal.Decimal
}

// UpdateIsolatedMargin changes isolated margin through the L1 action path.
func (c *Client) UpdateIsolatedMargin(ctx context.Context, request UpdateIsolatedMarginRequest) (ActionResponse, error) {
	ntli, err := usdcMicros(request.Amount)
	if err != nil {
		return ActionResponse{}, err
	}
	a, err := c.resolvePerpetualAsset(ctx, request.Coin, request.Market)
	if err != nil {
		return ActionResponse{}, err
	}
	return c.submitL1(ctx, signing.UpdateIsolatedMarginAction{Asset: a.ID, IsBuy: request.IsBuy, NTLI: ntli})
}

// TopUpIsolatedOnlyMarginRequest targets leverage while allowing only a margin
// top-up. Leverage is serialized as a canonical decimal string.
type TopUpIsolatedOnlyMarginRequest struct {
	Coin     string
	Market   *types.MarketRef
	Leverage decimal.Decimal
}

// TopUpIsolatedOnlyMargin adds isolated margin to reach a target leverage.
func (c *Client) TopUpIsolatedOnlyMargin(ctx context.Context, request TopUpIsolatedOnlyMarginRequest) (ActionResponse, error) {
	if !request.Leverage.IsPositive() {
		return ActionResponse{}, fmt.Errorf("target leverage must be positive")
	}
	a, err := c.resolvePerpetualAsset(ctx, request.Coin, request.Market)
	if err != nil {
		return ActionResponse{}, err
	}
	return c.submitL1(ctx, signing.TopUpIsolatedOnlyMarginAction{Asset: a.ID, Leverage: request.Leverage.String()})
}

// ReserveRequestWeight reserves additional Exchange request weight for a fee.
func (c *Client) ReserveRequestWeight(ctx context.Context, weight uint64) (ActionResponse, error) {
	if weight == 0 {
		return ActionResponse{}, fmt.Errorf("request weight must be positive")
	}
	return c.submitL1WithoutVault(ctx, signing.ReserveRequestWeightAction{Weight: weight}, c.submit.expiresAfter)
}

// Noop invalidates the supplied pending nonce. It deliberately uses the caller
// nonce directly rather than allocating a new one from the NonceManager.
func (c *Client) Noop(ctx context.Context, nonceValue uint64) (ActionResponse, error) {
	if nonceValue == 0 {
		return ActionResponse{}, fmt.Errorf("noop nonce must be positive")
	}
	return c.submitL1AtNonceWithOuter(ctx, signing.NoopAction{}, nonceValue, nil, c.submit.expiresAfter, nil)
}

// AddressEncoding identifies a SendToEVMWithData destination representation.
type AddressEncoding string

const (
	AddressEncodingHex    AddressEncoding = "hex"
	AddressEncodingBase58 AddressEncoding = "base58"
)

// SendToEVMWithDataRequest transfers a Core token into an EVM recipient and
// supplies optional call data. Data is raw bytes and never a float value.
type SendToEVMWithDataRequest struct {
	Token                string
	Amount               decimal.Decimal
	SourceDEX            string
	DestinationRecipient string
	AddressEncoding      AddressEncoding
	DestinationChainID   uint32
	GasLimit             uint64
	Data                 []byte
}

// SendToEVMWithData signs and submits a User-Signed EIP-712 transfer.
func (c *Client) SendToEVMWithData(ctx context.Context, request SendToEVMWithDataRequest) (ActionResponse, error) {
	if request.Token == "" || !request.Amount.IsPositive() || request.DestinationRecipient == "" || request.DestinationChainID == 0 || request.GasLimit == 0 {
		return ActionResponse{}, fmt.Errorf("token, positive amount, recipient, destination chain ID, and gas limit are required")
	}
	if request.AddressEncoding != AddressEncodingHex && request.AddressEncoding != AddressEncodingBase58 {
		return ActionResponse{}, fmt.Errorf("unsupported destination address encoding")
	}
	if request.AddressEncoding == AddressEncodingHex && !common.IsHexAddress(request.DestinationRecipient) {
		return ActionResponse{}, fmt.Errorf("hex destination recipient is invalid")
	}
	nonceValue, err := c.userActionNonce(ctx)
	if err != nil {
		return ActionResponse{}, err
	}
	return c.submitUserSigned(ctx, signing.SendToEVMWithDataAction{Token: request.Token, Amount: request.Amount.String(), SourceDEX: request.SourceDEX, DestinationRecipient: request.DestinationRecipient, AddressEncoding: string(request.AddressEncoding), DestinationChainID: request.DestinationChainID, GasLimit: request.GasLimit, Data: append([]byte(nil), request.Data...), Nonce: nonceValue})
}

// StakingTransferRequest transfers an exact native-token amount in wei.
type StakingTransferRequest struct{ Wei uint64 }

// CDeposit deposits native token from Core spot into staking.
func (c *Client) CDeposit(ctx context.Context, request StakingTransferRequest) (ActionResponse, error) {
	if request.Wei == 0 {
		return ActionResponse{}, fmt.Errorf("staking deposit wei must be positive")
	}
	nonceValue, err := c.userActionNonce(ctx)
	if err != nil {
		return ActionResponse{}, err
	}
	return c.submitUserSigned(ctx, signing.CDepositAction{Wei: request.Wei, Nonce: nonceValue})
}

// DepositStaking is a descriptive alias for CDeposit.
func (c *Client) DepositStaking(ctx context.Context, request StakingTransferRequest) (ActionResponse, error) {
	return c.CDeposit(ctx, request)
}

// CWithdraw withdraws native token from staking into Core spot.
func (c *Client) CWithdraw(ctx context.Context, request StakingTransferRequest) (ActionResponse, error) {
	if request.Wei == 0 {
		return ActionResponse{}, fmt.Errorf("staking withdrawal wei must be positive")
	}
	nonceValue, err := c.userActionNonce(ctx)
	if err != nil {
		return ActionResponse{}, err
	}
	return c.submitUserSigned(ctx, signing.CWithdrawAction{Wei: request.Wei, Nonce: nonceValue})
}

// WithdrawStaking is a descriptive alias for CWithdraw.
func (c *Client) WithdrawStaking(ctx context.Context, request StakingTransferRequest) (ActionResponse, error) {
	return c.CWithdraw(ctx, request)
}

// TokenDelegateRequest delegates or undelegates native staking balance.
type TokenDelegateRequest struct {
	Validator    common.Address
	Wei          uint64
	IsUndelegate bool
}

// TokenDelegate signs and submits a validator delegation user action.
func (c *Client) TokenDelegate(ctx context.Context, request TokenDelegateRequest) (ActionResponse, error) {
	if request.Validator == (common.Address{}) || request.Wei == 0 {
		return ActionResponse{}, fmt.Errorf("validator address and positive wei are required")
	}
	nonceValue, err := c.userActionNonce(ctx)
	if err != nil {
		return ActionResponse{}, err
	}
	return c.submitUserSigned(ctx, signing.TokenDelegateAction{Validator: request.Validator, Wei: request.Wei, IsUndelegate: request.IsUndelegate, Nonce: nonceValue})
}

// Delegate stakes native token with a validator.
func (c *Client) Delegate(ctx context.Context, validator common.Address, wei uint64) (ActionResponse, error) {
	return c.TokenDelegate(ctx, TokenDelegateRequest{Validator: validator, Wei: wei})
}

// Undelegate unstakes native token from a validator.
func (c *Client) Undelegate(ctx context.Context, validator common.Address, wei uint64) (ActionResponse, error) {
	return c.TokenDelegate(ctx, TokenDelegateRequest{Validator: validator, Wei: wei, IsUndelegate: true})
}

func (c *Client) resolvePerpetualAsset(ctx context.Context, coin string, market *types.MarketRef) (asset.Asset, error) {
	if c.assets == nil {
		return asset.Asset{}, fmt.Errorf("asset resolver is required")
	}
	var (
		resolved asset.Asset
		err      error
	)
	if market != nil {
		if coin != "" {
			return asset.Asset{}, fmt.Errorf("specify either coin or market reference, not both")
		}
		resolver, ok := c.assets.(asset.MarketResolver)
		if !ok {
			return asset.Asset{}, fmt.Errorf("asset resolver does not support market references")
		}
		resolved, err = resolver.ResolveMarket(ctx, *market)
	} else {
		if coin == "" {
			return asset.Asset{}, fmt.Errorf("coin is required")
		}
		resolved, err = c.assets.Resolve(ctx, coin)
	}
	if err != nil {
		return asset.Asset{}, err
	}
	if resolved.Kind != asset.Perp && resolved.Kind != asset.HIP3 {
		return asset.Asset{}, fmt.Errorf("action requires a perpetual asset")
	}
	return resolved, nil
}

func usdcMicros(amount decimal.Decimal) (int64, error) {
	if amount.IsZero() {
		return 0, fmt.Errorf("isolated margin amount must be non-zero")
	}
	scaled := amount.Mul(decimal.New(1, 6))
	if !scaled.Equal(scaled.Truncate(0)) {
		return 0, fmt.Errorf("isolated margin supports at most 6 decimal places")
	}
	if !scaled.GreaterThanOrEqual(decimal.NewFromInt(math.MinInt64)) || !scaled.LessThanOrEqual(decimal.NewFromInt(math.MaxInt64)) {
		return 0, fmt.Errorf("isolated margin amount is outside protocol range")
	}
	return scaled.IntPart(), nil
}
