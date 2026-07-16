package info

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/shopspring/decimal"
)

// HistoricalOrder is a completed or active order returned by historicalOrders.
type HistoricalOrder struct {
	Order           FrontendOpenOrder `json:"order"`
	Status          string            `json:"status"`
	StatusTimestamp int64             `json:"statusTimestamp"`
}

// TwapSliceFill associates a fill with its parent TWAP identifier.
type TwapSliceFill struct {
	Fill   UserFill `json:"fill"`
	TWAPID uint64   `json:"twapId"`
}

// UserVaultEquity is a user's equity in a vault.
type UserVaultEquity struct {
	VaultAddress         string          `json:"vaultAddress"`
	Equity               decimal.Decimal `json:"equity"`
	LockedUntilTimestamp int64           `json:"lockedUntilTimestamp"`
}

// UserRoleResponse identifies the account role. Data is populated for agent
// and subAccount roles; absent fields remain empty for user, vault, and missing.
type UserRoleResponse struct {
	Role string        `json:"role"`
	Data *UserRoleData `json:"data,omitempty"`
}

// UserRoleData describes the master relationship for agent/subAccount roles.
type UserRoleData struct {
	User   string `json:"user,omitempty"`
	Master string `json:"master,omitempty"`
}

// ReferralResponse is a user's referral and reward accounting state.
type ReferralResponse struct {
	ReferredBy       *Referrer          `json:"referredBy"`
	CumulativeVolume decimal.Decimal    `json:"cumVlm"`
	UnclaimedRewards decimal.Decimal    `json:"unclaimedRewards"`
	ClaimedRewards   decimal.Decimal    `json:"claimedRewards"`
	BuilderRewards   decimal.Decimal    `json:"builderRewards"`
	ReferrerState    ReferralState      `json:"referrerState"`
	RewardHistory    []ReferralReward   `json:"rewardHistory"`
	TokenToState     []TokenRewardState `json:"tokenToState"`
}

type Referrer struct {
	Referrer string `json:"referrer"`
	Code     string `json:"code"`
}
type ReferralState struct {
	Stage string             `json:"stage"`
	Data  *ReferralStateData `json:"data,omitempty"`
}
type ReferralStateData struct {
	Code           string                 `json:"code,omitempty"`
	Required       decimal.Decimal        `json:"required,omitempty"`
	Referrals      int                    `json:"nReferrals,omitempty"`
	ReferralStates []ReferralAccountState `json:"referralStates,omitempty"`
}
type ReferralAccountState struct {
	CumulativeVolume decimal.Decimal      `json:"cumVlm"`
	RewardedFees     decimal.Decimal      `json:"cumRewardedFeesSinceReferred"`
	ReferrerFees     decimal.Decimal      `json:"cumFeesRewardedToReferrer"`
	TimeJoined       int64                `json:"timeJoined"`
	User             string               `json:"user"`
	TokenToState     []TokenReferralState `json:"tokenToState"`
}
type ReferralReward struct {
	Earned         decimal.Decimal `json:"earned"`
	Volume         decimal.Decimal `json:"vlm"`
	ReferralVolume decimal.Decimal `json:"referralVlm"`
	Time           int64           `json:"time"`
}
type TokenRewardState struct {
	TokenID int
	State   ReferralRewardState
}
type TokenReferralState struct {
	TokenID int
	State   ReferralFeeState
}
type ReferralRewardState struct {
	CumulativeVolume decimal.Decimal `json:"cumVlm"`
	UnclaimedRewards decimal.Decimal `json:"unclaimedRewards"`
	ClaimedRewards   decimal.Decimal `json:"claimedRewards"`
	BuilderRewards   decimal.Decimal `json:"builderRewards"`
}
type ReferralFeeState struct {
	CumulativeVolume decimal.Decimal `json:"cumVlm"`
	RewardedFees     decimal.Decimal `json:"cumRewardedFeesSinceReferred"`
	ReferrerFees     decimal.Decimal `json:"cumFeesRewardedToReferrer"`
}

func (p *TokenRewardState) UnmarshalJSON(data []byte) error {
	var tuple []json.RawMessage
	if err := json.Unmarshal(data, &tuple); err != nil {
		return err
	}
	if len(tuple) != 2 {
		return fmt.Errorf("token reward state must contain token and state")
	}
	if err := json.Unmarshal(tuple[0], &p.TokenID); err != nil {
		return err
	}
	return json.Unmarshal(tuple[1], &p.State)
}
func (p *TokenReferralState) UnmarshalJSON(data []byte) error {
	var tuple []json.RawMessage
	if err := json.Unmarshal(data, &tuple); err != nil {
		return err
	}
	if len(tuple) != 2 {
		return fmt.Errorf("token referral state must contain token and state")
	}
	if err := json.Unmarshal(tuple[0], &p.TokenID); err != nil {
		return err
	}
	return json.Unmarshal(tuple[1], &p.State)
}

// Delegation is an active staking delegation.
type Delegation struct {
	Validator            string          `json:"validator"`
	Amount               decimal.Decimal `json:"amount"`
	LockedUntilTimestamp int64           `json:"lockedUntilTimestamp"`
}

// DelegatorHistoryEntry carries one staking history update.
type DelegatorHistoryEntry struct {
	Time  int64                 `json:"time"`
	Hash  string                `json:"hash"`
	Delta DelegatorHistoryDelta `json:"delta"`
}

// DelegatorHistoryDelta is the official staking-history action union. Exactly
// one optional member is normally populated by the server.
type DelegatorHistoryDelta struct {
	Delegate   *DelegatorDelegateUpdate   `json:"delegate,omitempty"`
	CDeposit   *DelegatorDepositUpdate    `json:"cDeposit,omitempty"`
	Withdrawal *DelegatorWithdrawalUpdate `json:"withdrawal,omitempty"`
}

func (d *DelegatorHistoryDelta) UnmarshalJSON(data []byte) error {
	type wire DelegatorHistoryDelta
	var decoded wire
	if err := json.Unmarshal(data, &decoded); err != nil {
		return err
	}
	branches := 0
	if decoded.Delegate != nil {
		branches++
	}
	if decoded.CDeposit != nil {
		branches++
	}
	if decoded.Withdrawal != nil {
		branches++
	}
	if branches != 1 {
		return fmt.Errorf("delegator history delta must contain exactly one known action")
	}
	*d = DelegatorHistoryDelta(decoded)
	return nil
}

type DelegatorDelegateUpdate struct {
	Validator    string          `json:"validator"`
	Amount       decimal.Decimal `json:"amount"`
	IsUndelegate bool            `json:"isUndelegate"`
}
type DelegatorDepositUpdate struct {
	Amount decimal.Decimal `json:"amount"`
}
type DelegatorWithdrawalUpdate struct {
	Amount decimal.Decimal `json:"amount"`
	Phase  string          `json:"phase"`
}
type DelegatorReward struct {
	Time        int64           `json:"time"`
	Source      string          `json:"source"`
	TotalAmount decimal.Decimal `json:"totalAmount"`
}

// BorrowLendReserveState contains the current state of a lending reserve.
type BorrowLendReserveState struct {
	BorrowYearlyRate decimal.Decimal `json:"borrowYearlyRate"`
	SupplyYearlyRate decimal.Decimal `json:"supplyYearlyRate"`
	Balance          decimal.Decimal `json:"balance"`
	Utilization      decimal.Decimal `json:"utilization"`
	OraclePx         decimal.Decimal `json:"oraclePx"`
	LTV              decimal.Decimal `json:"ltv"`
	TotalSupplied    decimal.Decimal `json:"totalSupplied"`
	TotalBorrowed    decimal.Decimal `json:"totalBorrowed"`
}
type BorrowLendReserve struct {
	ReserveID int
	State     BorrowLendReserveState
}

func (p *BorrowLendReserve) UnmarshalJSON(data []byte) error {
	var tuple []json.RawMessage
	if err := json.Unmarshal(data, &tuple); err != nil {
		return err
	}
	if len(tuple) != 2 {
		return fmt.Errorf("borrow lend reserve must contain token and state")
	}
	if err := json.Unmarshal(tuple[0], &p.ReserveID); err != nil {
		return err
	}
	return json.Unmarshal(tuple[1], &p.State)
}

type BorrowLendAmount struct {
	Basis decimal.Decimal `json:"basis"`
	Value decimal.Decimal `json:"value"`
}
type BorrowLendTokenStateValue struct {
	Borrow BorrowLendAmount `json:"borrow"`
	Supply BorrowLendAmount `json:"supply"`
}
type BorrowLendTokenState struct {
	TokenID int
	State   BorrowLendTokenStateValue
}

func (p *BorrowLendTokenState) UnmarshalJSON(data []byte) error {
	var tuple []json.RawMessage
	if err := json.Unmarshal(data, &tuple); err != nil {
		return err
	}
	if len(tuple) != 2 {
		return fmt.Errorf("borrow lend token state must contain token and state")
	}
	if err := json.Unmarshal(tuple[0], &p.TokenID); err != nil {
		return err
	}
	return json.Unmarshal(tuple[1], &p.State)
}

type BorrowLendUserStateResponse struct {
	TokenToState []BorrowLendTokenState `json:"tokenToState"`
	Health       string                 `json:"health"`
	HealthFactor *decimal.Decimal       `json:"healthFactor"`
}

// UserAbstraction identifies the enabled account abstraction mode.
type UserAbstraction string

const (
	UserAbstractionUnifiedAccount  UserAbstraction = "unifiedAccount"
	UserAbstractionPortfolioMargin UserAbstraction = "portfolioMargin"
	UserAbstractionDisabled        UserAbstraction = "disabled"
	UserAbstractionDefault         UserAbstraction = "default"
)

func (c *Client) AllMidsForDEX(ctx context.Context, dex string) (AllMidsResponse, error) {
	var r AllMidsResponse
	err := c.call(ctx, AllMidsRequest{Type: "allMids", DEX: dex}, &r)
	return r, err
}
func (c *Client) MetaAndAssetContextsForDEX(ctx context.Context, dex string) (MetaAndAssetContextsResponse, error) {
	var r MetaAndAssetContextsResponse
	err := c.call(ctx, MetaRequest{Type: "metaAndAssetCtxs", DEX: dex}, &r)
	return r, err
}
func (c *Client) OpenOrdersForDEX(ctx context.Context, user, dex string) ([]OpenOrder, error) {
	if user == "" {
		return nil, fmt.Errorf("user is required")
	}
	var r []OpenOrder
	err := c.call(ctx, struct {
		Type string `json:"type"`
		User string `json:"user"`
		DEX  string `json:"dex,omitempty"`
	}{"openOrders", user, dex}, &r)
	return r, err
}
func (c *Client) FrontendOpenOrdersForDEX(ctx context.Context, user, dex string) ([]FrontendOpenOrder, error) {
	if user == "" {
		return nil, fmt.Errorf("user is required")
	}
	var r []FrontendOpenOrder
	err := c.call(ctx, struct {
		Type string `json:"type"`
		User string `json:"user"`
		DEX  string `json:"dex,omitempty"`
	}{"frontendOpenOrders", user, dex}, &r)
	return r, err
}
func (c *Client) MaxBuilderFee(ctx context.Context, user, builder string) (int, error) {
	if user == "" || builder == "" {
		return 0, fmt.Errorf("user and builder are required")
	}
	var r int
	err := c.call(ctx, struct {
		Type    string `json:"type"`
		User    string `json:"user"`
		Builder string `json:"builder"`
	}{"maxBuilderFee", user, builder}, &r)
	return r, err
}
func (c *Client) HistoricalOrders(ctx context.Context, user string) ([]HistoricalOrder, error) {
	if user == "" {
		return nil, fmt.Errorf("user is required")
	}
	var r []HistoricalOrder
	err := c.call(ctx, userRequest{"historicalOrders", user}, &r)
	return r, err
}
func (c *Client) UserTwapSliceFills(ctx context.Context, user string) ([]TwapSliceFill, error) {
	if user == "" {
		return nil, fmt.Errorf("user is required")
	}
	var r []TwapSliceFill
	err := c.call(ctx, userRequest{"userTwapSliceFills", user}, &r)
	return r, err
}
func (c *Client) UserVaultEquities(ctx context.Context, user string) ([]UserVaultEquity, error) {
	if user == "" {
		return nil, fmt.Errorf("user is required")
	}
	var r []UserVaultEquity
	err := c.call(ctx, userRequest{"userVaultEquities", user}, &r)
	return r, err
}
func (c *Client) UserRole(ctx context.Context, user string) (UserRoleResponse, error) {
	if user == "" {
		return UserRoleResponse{}, fmt.Errorf("user is required")
	}
	var r UserRoleResponse
	err := c.call(ctx, userRequest{"userRole", user}, &r)
	return r, err
}
func (c *Client) Referral(ctx context.Context, user string) (ReferralResponse, error) {
	if user == "" {
		return ReferralResponse{}, fmt.Errorf("user is required")
	}
	var r ReferralResponse
	err := c.call(ctx, userRequest{"referral", user}, &r)
	return r, err
}
func (c *Client) Delegations(ctx context.Context, user string) ([]Delegation, error) {
	if user == "" {
		return nil, fmt.Errorf("user is required")
	}
	var r []Delegation
	err := c.call(ctx, userRequest{"delegations", user}, &r)
	return r, err
}
func (c *Client) DelegatorHistory(ctx context.Context, user string) ([]DelegatorHistoryEntry, error) {
	if user == "" {
		return nil, fmt.Errorf("user is required")
	}
	var r []DelegatorHistoryEntry
	err := c.call(ctx, userRequest{"delegatorHistory", user}, &r)
	return r, err
}
func (c *Client) DelegatorRewards(ctx context.Context, user string) ([]DelegatorReward, error) {
	if user == "" {
		return nil, fmt.Errorf("user is required")
	}
	var r []DelegatorReward
	err := c.call(ctx, userRequest{"delegatorRewards", user}, &r)
	return r, err
}
func (c *Client) ApprovedBuilders(ctx context.Context, user string) ([]string, error) {
	if user == "" {
		return nil, fmt.Errorf("user is required")
	}
	var r []string
	err := c.call(ctx, userRequest{"approvedBuilders", user}, &r)
	return r, err
}
func (c *Client) UserDEXAbstraction(ctx context.Context, user string) (*bool, error) {
	if user == "" {
		return nil, fmt.Errorf("user is required")
	}
	var r *bool
	err := c.call(ctx, userRequest{"userDexAbstraction", user}, &r)
	return r, err
}
func (c *Client) UserAbstraction(ctx context.Context, user string) (UserAbstraction, error) {
	if user == "" {
		return "", fmt.Errorf("user is required")
	}
	var r UserAbstraction
	err := c.call(ctx, userRequest{"userAbstraction", user}, &r)
	return r, err
}
func (c *Client) BorrowLendReserveState(ctx context.Context, token int) (BorrowLendReserveState, error) {
	if token < 0 {
		return BorrowLendReserveState{}, fmt.Errorf("token must be non-negative")
	}
	var r BorrowLendReserveState
	err := c.call(ctx, struct {
		Type  string `json:"type"`
		Token int    `json:"token"`
	}{"borrowLendReserveState", token}, &r)
	return r, err
}
func (c *Client) BorrowLendUserState(ctx context.Context, user string) (BorrowLendUserStateResponse, error) {
	if user == "" {
		return BorrowLendUserStateResponse{}, fmt.Errorf("user is required")
	}
	var r BorrowLendUserStateResponse
	err := c.call(ctx, userRequest{"borrowLendUserState", user}, &r)
	return r, err
}
func (c *Client) AllBorrowLendReserveStates(ctx context.Context) ([]BorrowLendReserve, error) {
	var r []BorrowLendReserve
	err := c.call(ctx, struct {
		Type string `json:"type"`
	}{"allBorrowLendReserveStates"}, &r)
	return r, err
}

// DeployAuctionStatus describes the Dutch auction used for perp and spot deployments.
type DeployAuctionStatus struct {
	CurrentGas       *decimal.Decimal `json:"currentGas"`
	DurationSeconds  int64            `json:"durationSeconds"`
	EndGas           *decimal.Decimal `json:"endGas"`
	StartGas         decimal.Decimal  `json:"startGas"`
	StartTimeSeconds int64            `json:"startTimeSeconds"`
}

// ActiveAssetDataResponse is a user's trade capacity for one perpetual asset.
type ActiveAssetDataResponse struct {
	User             string             `json:"user"`
	Coin             string             `json:"coin"`
	Leverage         Leverage           `json:"leverage"`
	MaxTradeSizes    [2]decimal.Decimal `json:"maxTradeSzs"`
	AvailableToTrade [2]decimal.Decimal `json:"availableToTrade"`
	MarkPx           decimal.Decimal    `json:"markPx"`
}

// PerpDEXLimitsResponse is a builder-deployed DEX's open-interest limits.
// The server may return null when the DEX does not publish limits.
type PerpDEXLimitsResponse struct {
	TotalOICap     decimal.Decimal `json:"totalOiCap"`
	OISzCapPerPerp decimal.Decimal `json:"oiSzCapPerPerp"`
	MaxTransferNtl decimal.Decimal `json:"maxTransferNtl"`
	CoinToOICap    []CoinOICap     `json:"coinToOiCap"`
}
type CoinOICap struct {
	Coin string
	Cap  decimal.Decimal
}

func (p *CoinOICap) UnmarshalJSON(data []byte) error {
	var tuple []json.RawMessage
	if err := json.Unmarshal(data, &tuple); err != nil {
		return err
	}
	if len(tuple) != 2 {
		return fmt.Errorf("coin OI cap must contain coin and cap")
	}
	if err := json.Unmarshal(tuple[0], &p.Coin); err != nil {
		return err
	}
	return json.Unmarshal(tuple[1], &p.Cap)
}

type PerpDEXStatusResponse struct {
	TotalNetDeposit decimal.Decimal `json:"totalNetDeposit"`
}
type PerpAnnotationResponse struct {
	Category    string   `json:"category"`
	Description string   `json:"description"`
	DisplayName string   `json:"displayName,omitempty"`
	Keywords    []string `json:"keywords,omitempty"`
}
type PerpCategory struct {
	Coin     string
	Category string
}

func (p *PerpCategory) UnmarshalJSON(data []byte) error {
	var tuple []json.RawMessage
	if err := json.Unmarshal(data, &tuple); err != nil {
		return err
	}
	if len(tuple) != 2 {
		return fmt.Errorf("perp category must contain coin and category")
	}
	if err := json.Unmarshal(tuple[0], &p.Coin); err != nil {
		return err
	}
	return json.Unmarshal(tuple[1], &p.Category)
}

type ConcisePerpAnnotation struct {
	Category    string   `json:"category"`
	DisplayName string   `json:"displayName,omitempty"`
	Keywords    []string `json:"keywords,omitempty"`
}
type PerpConciseAnnotation struct {
	Coin       string
	Annotation ConcisePerpAnnotation
}

func (p *PerpConciseAnnotation) UnmarshalJSON(data []byte) error {
	var tuple []json.RawMessage
	if err := json.Unmarshal(data, &tuple); err != nil {
		return err
	}
	if len(tuple) != 2 {
		return fmt.Errorf("concise perp annotation must contain coin and annotation")
	}
	if err := json.Unmarshal(tuple[0], &p.Coin); err != nil {
		return err
	}
	return json.Unmarshal(tuple[1], &p.Annotation)
}

type SpotDeployStateResponse struct {
	States     []SpotDeployTokenState `json:"states"`
	GasAuction DeployAuctionStatus    `json:"gasAuction"`
}
type SpotDeployTokenState struct {
	Token                        int                 `json:"token"`
	Spec                         SpotDeployTokenSpec `json:"spec"`
	FullName                     *string             `json:"fullName"`
	DeployerTradingFeeShare      decimal.Decimal     `json:"deployerTradingFeeShare"`
	Spots                        []int               `json:"spots"`
	MaxSupply                    *decimal.Decimal    `json:"maxSupply"`
	HyperliquidityGenesisBalance decimal.Decimal     `json:"hyperliquidityGenesisBalance"`
	TotalGenesisBalanceWei       decimal.Decimal     `json:"totalGenesisBalanceWei"`
	UserGenesisBalances          []AddressBalance    `json:"userGenesisBalances"`
	ExistingTokenGenesisBalances []TokenBalance      `json:"existingTokenGenesisBalances"`
	BlacklistUsers               []string            `json:"blacklistUsers"`
}
type SpotDeployTokenSpec struct {
	Name        string `json:"name"`
	SzDecimals  int    `json:"szDecimals"`
	WeiDecimals int    `json:"weiDecimals"`
}
type AddressBalance struct {
	Address string
	Balance decimal.Decimal
}

func (p *AddressBalance) UnmarshalJSON(data []byte) error {
	var tuple []json.RawMessage
	if err := json.Unmarshal(data, &tuple); err != nil {
		return err
	}
	if len(tuple) != 2 {
		return fmt.Errorf("address balance must contain address and balance")
	}
	if err := json.Unmarshal(tuple[0], &p.Address); err != nil {
		return err
	}
	return json.Unmarshal(tuple[1], &p.Balance)
}

type TokenBalance struct {
	Token   int
	Balance decimal.Decimal
}

func (p *TokenBalance) UnmarshalJSON(data []byte) error {
	var tuple []json.RawMessage
	if err := json.Unmarshal(data, &tuple); err != nil {
		return err
	}
	if len(tuple) != 2 {
		return fmt.Errorf("token balance must contain token and balance")
	}
	if err := json.Unmarshal(tuple[0], &p.Token); err != nil {
		return err
	}
	return json.Unmarshal(tuple[1], &p.Balance)
}

type TokenDetailsResponse struct {
	Name                       string           `json:"name"`
	MaxSupply                  decimal.Decimal  `json:"maxSupply"`
	TotalSupply                decimal.Decimal  `json:"totalSupply"`
	CirculatingSupply          decimal.Decimal  `json:"circulatingSupply"`
	SzDecimals                 int              `json:"szDecimals"`
	WeiDecimals                int              `json:"weiDecimals"`
	MidPx                      decimal.Decimal  `json:"midPx"`
	MarkPx                     decimal.Decimal  `json:"markPx"`
	PrevDayPx                  decimal.Decimal  `json:"prevDayPx"`
	Genesis                    *TokenGenesis    `json:"genesis"`
	Deployer                   *string          `json:"deployer"`
	DeployGas                  *decimal.Decimal `json:"deployGas"`
	DeployTime                 *string          `json:"deployTime"`
	SeededUSDC                 decimal.Decimal  `json:"seededUsdc"`
	NonCirculatingUserBalances []AddressBalance `json:"nonCirculatingUserBalances"`
	FutureEmissions            decimal.Decimal  `json:"futureEmissions"`
}
type TokenGenesis struct {
	UserBalances          []AddressBalance `json:"userBalances"`
	ExistingTokenBalances []TokenBalance   `json:"existingTokenBalances"`
	BlacklistUsers        []string         `json:"blacklistUsers"`
}
type OutcomeMetaResponse struct {
	Outcomes  []Outcome         `json:"outcomes"`
	Questions []OutcomeQuestion `json:"questions"`
}
type Outcome struct {
	Outcome     int               `json:"outcome"`
	Name        string            `json:"name"`
	Description string            `json:"description"`
	SideSpecs   []OutcomeSideSpec `json:"sideSpecs"`
	QuoteToken  string            `json:"quoteToken"`
}
type OutcomeSideSpec struct {
	Name  string `json:"name"`
	Token *int   `json:"token,omitempty"`
}
type OutcomeQuestion struct {
	Question             int    `json:"question"`
	Name                 string `json:"name"`
	Description          string `json:"description"`
	FallbackOutcome      int    `json:"fallbackOutcome"`
	NamedOutcomes        []int  `json:"namedOutcomes"`
	SettledNamedOutcomes []int  `json:"settledNamedOutcomes"`
}

func (c *Client) PerpsAtOpenInterestCap(ctx context.Context, dex string) ([]string, error) {
	var r []string
	err := c.call(ctx, struct {
		Type string `json:"type"`
		DEX  string `json:"dex,omitempty"`
	}{"perpsAtOpenInterestCap", dex}, &r)
	return r, err
}
func (c *Client) PerpDeployAuctionStatus(ctx context.Context) (DeployAuctionStatus, error) {
	var r DeployAuctionStatus
	err := c.call(ctx, struct {
		Type string `json:"type"`
	}{"perpDeployAuctionStatus"}, &r)
	return r, err
}
func (c *Client) ActiveAssetData(ctx context.Context, user, coin string) (ActiveAssetDataResponse, error) {
	if user == "" || coin == "" {
		return ActiveAssetDataResponse{}, fmt.Errorf("user and coin are required")
	}
	var r ActiveAssetDataResponse
	err := c.call(ctx, struct {
		Type string `json:"type"`
		User string `json:"user"`
		Coin string `json:"coin"`
	}{"activeAssetData", user, coin}, &r)
	return r, err
}
func (c *Client) PerpDEXLimits(ctx context.Context, dex string) (*PerpDEXLimitsResponse, error) {
	if dex == "" {
		return nil, fmt.Errorf("DEX is required")
	}
	var r *PerpDEXLimitsResponse
	err := c.call(ctx, struct {
		Type string `json:"type"`
		DEX  string `json:"dex"`
	}{"perpDexLimits", dex}, &r)
	return r, err
}
func (c *Client) PerpDEXStatus(ctx context.Context, dex string) (PerpDEXStatusResponse, error) {
	if dex == "" {
		return PerpDEXStatusResponse{}, fmt.Errorf("DEX is required")
	}
	var r PerpDEXStatusResponse
	err := c.call(ctx, struct {
		Type string `json:"type"`
		DEX  string `json:"dex"`
	}{"perpDexStatus", dex}, &r)
	return r, err
}
func (c *Client) AllPerpMetas(ctx context.Context) ([]MetaResponse, error) {
	var r []MetaResponse
	err := c.call(ctx, struct {
		Type string `json:"type"`
	}{"allPerpMetas"}, &r)
	return r, err
}
func (c *Client) PerpAnnotation(ctx context.Context, coin string) (*PerpAnnotationResponse, error) {
	if coin == "" {
		return nil, fmt.Errorf("coin is required")
	}
	var r *PerpAnnotationResponse
	err := c.call(ctx, struct {
		Type string `json:"type"`
		Coin string `json:"coin"`
	}{"perpAnnotation", coin}, &r)
	return r, err
}
func (c *Client) PerpCategories(ctx context.Context) ([]PerpCategory, error) {
	var r []PerpCategory
	err := c.call(ctx, struct {
		Type string `json:"type"`
	}{"perpCategories"}, &r)
	return r, err
}
func (c *Client) PerpConciseAnnotations(ctx context.Context) ([]PerpConciseAnnotation, error) {
	var r []PerpConciseAnnotation
	err := c.call(ctx, struct {
		Type string `json:"type"`
	}{"perpConciseAnnotations"}, &r)
	return r, err
}
func (c *Client) SpotDeployState(ctx context.Context, user string) (SpotDeployStateResponse, error) {
	if user == "" {
		return SpotDeployStateResponse{}, fmt.Errorf("user is required")
	}
	var r SpotDeployStateResponse
	err := c.call(ctx, userRequest{"spotDeployState", user}, &r)
	return r, err
}
func (c *Client) SpotPairDeployAuctionStatus(ctx context.Context) (DeployAuctionStatus, error) {
	var r DeployAuctionStatus
	err := c.call(ctx, struct {
		Type string `json:"type"`
	}{"spotPairDeployAuctionStatus"}, &r)
	return r, err
}
func (c *Client) TokenDetails(ctx context.Context, tokenID string) (TokenDetailsResponse, error) {
	if tokenID == "" {
		return TokenDetailsResponse{}, fmt.Errorf("token ID is required")
	}
	var r TokenDetailsResponse
	err := c.call(ctx, struct {
		Type    string `json:"type"`
		TokenID string `json:"tokenId"`
	}{"tokenDetails", tokenID}, &r)
	return r, err
}
func (c *Client) OutcomeMeta(ctx context.Context) (OutcomeMetaResponse, error) {
	var r OutcomeMetaResponse
	err := c.call(ctx, struct {
		Type string `json:"type"`
	}{"outcomeMeta"}, &r)
	return r, err
}

// NonFundingLedgerUpdate is one account ledger update other than funding.
type NonFundingLedgerUpdate struct {
	Time  int64       `json:"time"`
	Hash  string      `json:"hash"`
	Delta LedgerDelta `json:"delta"`
}

// LedgerDelta models every stable scalar shared by the protocol's
// forward-extensible non-funding ledger action union. Fields that do not apply
// to the action Type retain their zero value; future action-specific fields are
// safely ignored by JSON decoding.
type LedgerDelta struct {
	Type                string               `json:"type"`
	USDC                decimal.Decimal      `json:"usdc"`
	ToPerp              bool                 `json:"toPerp,omitempty"`
	User                string               `json:"user,omitempty"`
	Destination         string               `json:"destination,omitempty"`
	Fee                 decimal.Decimal      `json:"fee"`
	NativeTokenFee      decimal.Decimal      `json:"nativeTokenFee"`
	FeeToken            string               `json:"feeToken,omitempty"`
	Nonce               *int64               `json:"nonce,omitempty"`
	Token               string               `json:"token,omitempty"`
	Amount              decimal.Decimal      `json:"amount"`
	USDCValue           decimal.Decimal      `json:"usdcValue"`
	LiquidatedNtlPos    decimal.Decimal      `json:"liquidatedNtlPos"`
	AccountValue        decimal.Decimal      `json:"accountValue"`
	LeverageType        string               `json:"leverageType,omitempty"`
	LiquidatedPositions []LiquidatedPosition `json:"liquidatedPositions,omitempty"`
	Vault               string               `json:"vault,omitempty"`
	RequestedUSD        decimal.Decimal      `json:"requestedUsd"`
	Commission          decimal.Decimal      `json:"commission"`
	ClosingCost         decimal.Decimal      `json:"closingCost"`
	Basis               decimal.Decimal      `json:"basis"`
	NetWithdrawnUSD     decimal.Decimal      `json:"netWithdrawnUsd"`
	SourceDEX           string               `json:"sourceDex,omitempty"`
	DestinationDEX      string               `json:"destinationDex,omitempty"`
	IsDeposit           *bool                `json:"isDeposit,omitempty"`
	Operation           string               `json:"operation,omitempty"`
	InterestAmount      decimal.Decimal      `json:"interestAmount"`
	DEX                 string               `json:"dex,omitempty"`
}
type LiquidatedPosition struct {
	Coin string          `json:"coin"`
	Size decimal.Decimal `json:"szi"`
}

// BorrowLendInterest is an accrued interest update for one token.
type BorrowLendInterest struct {
	Time   int64           `json:"time"`
	Token  string          `json:"token"`
	Borrow decimal.Decimal `json:"borrow"`
	Supply decimal.Decimal `json:"supply"`
}

func (c *Client) UserNonFundingLedgerUpdates(ctx context.Context, user string, startTime int64, endTime *int64) ([]NonFundingLedgerUpdate, error) {
	if user == "" || startTime < 0 || (endTime != nil && (*endTime < startTime || *endTime < 0)) {
		return nil, fmt.Errorf("invalid non-funding ledger request")
	}
	var r []NonFundingLedgerUpdate
	err := c.call(ctx, struct {
		Type      string `json:"type"`
		User      string `json:"user"`
		StartTime int64  `json:"startTime"`
		EndTime   *int64 `json:"endTime,omitempty"`
	}{"userNonFundingLedgerUpdates", user, startTime, endTime}, &r)
	return r, err
}
func (c *Client) UserTwapSliceFillsByTime(ctx context.Context, user string, startTime int64, endTime *int64) ([]TwapSliceFill, error) {
	if user == "" || startTime < 0 || (endTime != nil && (*endTime < startTime || *endTime < 0)) {
		return nil, fmt.Errorf("invalid TWAP fills request")
	}
	var r []TwapSliceFill
	err := c.call(ctx, struct {
		Type      string `json:"type"`
		User      string `json:"user"`
		StartTime int64  `json:"startTime"`
		EndTime   *int64 `json:"endTime,omitempty"`
	}{"userTwapSliceFillsByTime", user, startTime, endTime}, &r)
	return r, err
}
func (c *Client) UserBorrowLendInterest(ctx context.Context, user string, startTime int64, endTime *int64) ([]BorrowLendInterest, error) {
	if user == "" || startTime < 0 || (endTime != nil && (*endTime < startTime || *endTime < 0)) {
		return nil, fmt.Errorf("invalid borrow/lend interest request")
	}
	var r []BorrowLendInterest
	err := c.call(ctx, struct {
		Type      string `json:"type"`
		User      string `json:"user"`
		StartTime int64  `json:"startTime"`
		EndTime   *int64 `json:"endTime,omitempty"`
	}{"userBorrowLendInterest", user, startTime, endTime}, &r)
	return r, err
}
