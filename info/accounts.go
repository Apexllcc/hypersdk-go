package info

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/shopspring/decimal"
)

// PortfolioPeriod is a named portfolio period together with its time series.
// Hyperliquid represents portfolio entries as [period, data] tuples.
type PortfolioPeriod struct {
	Period string
	Data   PortfolioPeriodData
}

// PortfolioPeriodData contains account-value and PnL observations for a period.
type PortfolioPeriodData struct {
	AccountValueHistory []PortfolioValuePoint `json:"accountValueHistory"`
	PnLHistory          []PortfolioValuePoint `json:"pnlHistory"`
	Volume              decimal.Decimal       `json:"vlm"`
}

// PortfolioValuePoint is one timestamped portfolio value.
// Hyperliquid represents points as [timestamp, decimal-string] tuples.
type PortfolioValuePoint struct {
	Time  int64
	Value decimal.Decimal
}

func (p *PortfolioPeriod) UnmarshalJSON(data []byte) error {
	var tuple []json.RawMessage
	if err := json.Unmarshal(data, &tuple); err != nil {
		return err
	}
	if len(tuple) != 2 {
		return fmt.Errorf("portfolio period must contain period and data")
	}
	if err := json.Unmarshal(tuple[0], &p.Period); err != nil {
		return err
	}
	return json.Unmarshal(tuple[1], &p.Data)
}

func (p *PortfolioValuePoint) UnmarshalJSON(data []byte) error {
	var tuple []json.RawMessage
	if err := json.Unmarshal(data, &tuple); err != nil {
		return err
	}
	if len(tuple) != 2 {
		return fmt.Errorf("portfolio value point must contain timestamp and value")
	}
	if err := json.Unmarshal(tuple[0], &p.Time); err != nil {
		return err
	}
	return json.Unmarshal(tuple[1], &p.Value)
}

// UserFundingEntry is one funding payment in a user's funding history.
type UserFundingEntry struct {
	Delta UserFundingDelta `json:"delta"`
	Hash  string           `json:"hash"`
	Time  int64            `json:"time"`
}

// UserFundingDelta is the funding-payment delta attached to a funding entry.
type UserFundingDelta struct {
	Type        string          `json:"type"`
	Coin        string          `json:"coin"`
	FundingRate decimal.Decimal `json:"fundingRate"`
	Size        decimal.Decimal `json:"szi"`
	USDC        decimal.Decimal `json:"usdc"`
	Samples     *int64          `json:"nSamples"`
}

// UserFeesResponse is a user's current fee schedule and recent volume.
type UserFeesResponse struct {
	Trial                  any                 `json:"trial"`
	DailyUserVolume        []DailyUserVolume   `json:"dailyUserVlm"`
	FeeSchedule            FeeSchedule         `json:"feeSchedule"`
	UserCrossRate          decimal.Decimal     `json:"userCrossRate"`
	UserAddRate            decimal.Decimal     `json:"userAddRate"`
	UserSpotCrossRate      decimal.Decimal     `json:"userSpotCrossRate"`
	UserSpotAddRate        decimal.Decimal     `json:"userSpotAddRate"`
	ActiveReferralDiscount decimal.Decimal     `json:"activeReferralDiscount"`
	FeeTrialEscrow         decimal.Decimal     `json:"feeTrialEscrow"`
	NextTrialAvailableAt   *int64              `json:"nextTrialAvailableTimestamp"`
	StakingLink            *StakingLink        `json:"stakingLink"`
	ActiveStakingDiscount  StakingDiscountTier `json:"activeStakingDiscount"`
}

// DailyUserVolume is one daily-volume observation.
type DailyUserVolume struct {
	Date      string          `json:"date"`
	UserCross decimal.Decimal `json:"userCross"`
	UserAdd   decimal.Decimal `json:"userAdd"`
	Exchange  decimal.Decimal `json:"exchange"`
}

// FeeSchedule contains exchange-wide fee tiers and discounts.
type FeeSchedule struct {
	Cross                decimal.Decimal       `json:"cross"`
	Add                  decimal.Decimal       `json:"add"`
	SpotCross            decimal.Decimal       `json:"spotCross"`
	SpotAdd              decimal.Decimal       `json:"spotAdd"`
	Tiers                FeeTiers              `json:"tiers"`
	ReferralDiscount     decimal.Decimal       `json:"referralDiscount"`
	StakingDiscountTiers []StakingDiscountTier `json:"stakingDiscountTiers"`
}

// FeeTiers groups VIP and market-maker fee tiers.
type FeeTiers struct {
	VIP []VIPFeeTier `json:"vip"`
	MM  []MMFeeTier  `json:"mm"`
}

// VIPFeeTier defines the rates that apply after a notional-volume cutoff.
type VIPFeeTier struct {
	NotionalCutoff decimal.Decimal `json:"ntlCutoff"`
	Cross          decimal.Decimal `json:"cross"`
	Add            decimal.Decimal `json:"add"`
	SpotCross      decimal.Decimal `json:"spotCross"`
	SpotAdd        decimal.Decimal `json:"spotAdd"`
}

// MMFeeTier defines a market-maker maker-fraction cutoff and add rate.
type MMFeeTier struct {
	MakerFractionCutoff decimal.Decimal `json:"makerFractionCutoff"`
	Add                 decimal.Decimal `json:"add"`
}

// StakingDiscountTier defines a staking-based fee discount.
type StakingDiscountTier struct {
	BPSOfMaxSupply decimal.Decimal `json:"bpsOfMaxSupply"`
	Discount       decimal.Decimal `json:"discount"`
}

// StakingLink is the permanent link between a trading and staking account.
type StakingLink struct {
	Type        string `json:"type"`
	StakingUser string `json:"stakingUser"`
}

// UserRateLimitResponse describes the current per-user API request budget.
type UserRateLimitResponse struct {
	CumulativeVolume decimal.Decimal `json:"cumVlm"`
	RequestsUsed     int64           `json:"nRequestsUsed"`
	RequestsCap      int64           `json:"nRequestsCap"`
	RequestsSurplus  int64           `json:"nRequestsSurplus"`
}

// DelegatorSummaryResponse describes a user's staked-token balance summary.
type DelegatorSummaryResponse struct {
	Delegated              decimal.Decimal `json:"delegated"`
	Undelegated            decimal.Decimal `json:"undelegated"`
	TotalPendingWithdrawal decimal.Decimal `json:"totalPendingWithdrawal"`
	PendingWithdrawals     int64           `json:"nPendingWithdrawals"`
}

// Subaccount is a master account's subaccount and its perp and spot state.
type Subaccount struct {
	Name               string                         `json:"name"`
	SubaccountUser     string                         `json:"subAccountUser"`
	Master             string                         `json:"master"`
	ClearinghouseState ClearinghouseStateResponse     `json:"clearinghouseState"`
	SpotState          SpotClearinghouseStateResponse `json:"spotState"`
}

// VaultDetailsResponse describes a vault, its portfolio and its followers.
type VaultDetailsResponse struct {
	Name                  string            `json:"name"`
	VaultAddress          string            `json:"vaultAddress"`
	Leader                string            `json:"leader"`
	Description           string            `json:"description"`
	Portfolio             []PortfolioPeriod `json:"portfolio"`
	LeaderFraction        decimal.Decimal   `json:"leaderFraction"`
	LeaderCommission      decimal.Decimal   `json:"leaderCommission"`
	FollowerState         *VaultFollower    `json:"followerState"`
	Followers             []VaultFollower   `json:"followers"`
	MaxDistributable      decimal.Decimal   `json:"maxDistributable"`
	MaxWithdrawable       decimal.Decimal   `json:"maxWithdrawable"`
	AllowDeposits         bool              `json:"allowDeposits"`
	AlwaysCloseOnWithdraw bool              `json:"alwaysCloseOnWithdraw"`
	IsClosed              bool              `json:"isClosed"`
	APR                   decimal.Decimal   `json:"apr"`
	Relationship          VaultRelationship `json:"relationship"`
}

// VaultFollower is one vault follower.
type VaultFollower struct {
	User           string          `json:"user"`
	VaultEquity    decimal.Decimal `json:"vaultEquity"`
	PnL            decimal.Decimal `json:"pnl"`
	AllTimePnL     decimal.Decimal `json:"allTimePnl"`
	DaysFollowing  int64           `json:"daysFollowing"`
	VaultEntryTime int64           `json:"vaultEntryTime"`
	LockupUntil    int64           `json:"lockupUntil"`
}

// VaultRelationship describes a vault's parent/child relationship.
type VaultRelationship struct {
	Type string                `json:"type"`
	Data VaultRelationshipData `json:"data"`
}

// VaultRelationshipData carries child vault addresses for parent vaults.
type VaultRelationshipData struct {
	ChildAddresses []string `json:"childAddresses"`
}

// Portfolio retrieves a user's portfolio periods.
func (c *Client) Portfolio(ctx context.Context, user string) ([]PortfolioPeriod, error) {
	if user == "" {
		return nil, fmt.Errorf("user is required")
	}
	var response []PortfolioPeriod
	err := c.call(ctx, userRequest{Type: "portfolio", User: user}, &response)
	return response, err
}

// UserFunding retrieves a user's funding-payment history. Times are in milliseconds.
// Omit startTime and/or endTime to use the API defaults.
func (c *Client) UserFunding(ctx context.Context, user string, startTime, endTime *int64) ([]UserFundingEntry, error) {
	if user == "" || (startTime != nil && *startTime < 0) || (endTime != nil && *endTime < 0) || (startTime != nil && endTime != nil && *endTime < *startTime) {
		return nil, fmt.Errorf("invalid user funding request")
	}
	var response []UserFundingEntry
	err := c.call(ctx, userFundingRequest{Type: "userFunding", User: user, StartTime: startTime, EndTime: endTime}, &response)
	return response, err
}

// UserFees retrieves a user's fee schedule and current fee rates.
func (c *Client) UserFees(ctx context.Context, user string) (UserFeesResponse, error) {
	if user == "" {
		return UserFeesResponse{}, fmt.Errorf("user is required")
	}
	var response UserFeesResponse
	err := c.call(ctx, userRequest{Type: "userFees", User: user}, &response)
	return response, err
}

// UserRateLimit retrieves a user's current request budget.
func (c *Client) UserRateLimit(ctx context.Context, user string) (UserRateLimitResponse, error) {
	if user == "" {
		return UserRateLimitResponse{}, fmt.Errorf("user is required")
	}
	var response UserRateLimitResponse
	err := c.call(ctx, userRequest{Type: "userRateLimit", User: user}, &response)
	return response, err
}

// DelegatorSummary retrieves a user's staking balance summary.
func (c *Client) DelegatorSummary(ctx context.Context, user string) (DelegatorSummaryResponse, error) {
	if user == "" {
		return DelegatorSummaryResponse{}, fmt.Errorf("user is required")
	}
	var response DelegatorSummaryResponse
	err := c.call(ctx, userRequest{Type: "delegatorSummary", User: user}, &response)
	return response, err
}

// Subaccounts retrieves all subaccounts for a master account.
func (c *Client) Subaccounts(ctx context.Context, user string) ([]Subaccount, error) {
	if user == "" {
		return nil, fmt.Errorf("user is required")
	}
	var response []Subaccount
	err := c.call(ctx, userRequest{Type: "subAccounts", User: user}, &response)
	return response, err
}

// VaultDetails retrieves a vault's public details. user optionally includes user-specific follower state.
func (c *Client) VaultDetails(ctx context.Context, vaultAddress string, user *string) (*VaultDetailsResponse, error) {
	if vaultAddress == "" {
		return nil, fmt.Errorf("vault address is required")
	}
	var response *VaultDetailsResponse
	err := c.call(ctx, vaultDetailsRequest{Type: "vaultDetails", VaultAddress: vaultAddress, User: user}, &response)
	return response, err
}

type userRequest struct {
	Type string `json:"type"`
	User string `json:"user"`
}

type userFundingRequest struct {
	Type      string `json:"type"`
	User      string `json:"user"`
	StartTime *int64 `json:"startTime,omitempty"`
	EndTime   *int64 `json:"endTime,omitempty"`
}

type vaultDetailsRequest struct {
	Type         string  `json:"type"`
	VaultAddress string  `json:"vaultAddress"`
	User         *string `json:"user,omitempty"`
}
