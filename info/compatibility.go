package info

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/shopspring/decimal"
)

// ExchangeStatusResponse is the exchange's current server time and special status.
type ExchangeStatusResponse struct {
	Time            int64   `json:"time"`
	SpecialStatuses *string `json:"specialStatuses"`
}

// ExtraAgent is an additional API agent configured for an account.
type ExtraAgent struct {
	Address    string `json:"address"`
	Name       string `json:"name"`
	ValidUntil *int64 `json:"validUntil"`
}

// GossipPriorityAuctionStatusResponse reports prior winners and current auction state.
type GossipPriorityAuctionStatusResponse struct {
	PreviousWinners []*string
	Auctions        []DeployAuctionStatus
}

func (r *GossipPriorityAuctionStatusResponse) UnmarshalJSON(data []byte) error {
	var tuple []json.RawMessage
	if err := json.Unmarshal(data, &tuple); err != nil {
		return err
	}
	if len(tuple) != 2 {
		return fmt.Errorf("gossip priority auction status must contain winners and auctions")
	}
	if err := json.Unmarshal(tuple[0], &r.PreviousWinners); err != nil {
		return err
	}
	return json.Unmarshal(tuple[1], &r.Auctions)
}

// LegalCheckResponse is a user's platform eligibility status.
type LegalCheckResponse struct {
	AcceptedTerms bool   `json:"acceptedTerms"`
	UserAllowed   bool   `json:"userAllowed"`
	Restrictions  string `json:"restrictions,omitempty"`
}

// PreTransferCheckResponse describes activation and sanctions checks before a transfer.
type PreTransferCheckResponse struct {
	Fee           decimal.Decimal `json:"fee"`
	IsSanctioned  bool            `json:"isSanctioned"`
	UserExists    bool            `json:"userExists"`
	UserHasSentTx bool            `json:"userHasSentTx"`
}

// VaultSummary is the lightweight summary returned for recently created vaults.
type VaultSummary struct {
	Name             string            `json:"name"`
	VaultAddress     string            `json:"vaultAddress"`
	Leader           string            `json:"leader"`
	TVL              decimal.Decimal   `json:"tvl"`
	IsClosed         bool              `json:"isClosed"`
	Relationship     VaultRelationship `json:"relationship"`
	CreateTimeMillis int64             `json:"createTimeMillis"`
}

// ValidatorSummary is one validator's identity, stake, commission and statistics.
type ValidatorSummary struct {
	Validator       string                `json:"validator"`
	Signer          string                `json:"signer"`
	Name            string                `json:"name"`
	Description     string                `json:"description"`
	RecentBlocks    int64                 `json:"nRecentBlocks"`
	Stake           decimal.Decimal       `json:"stake"`
	IsJailed        bool                  `json:"isJailed"`
	UnjailableAfter *int64                `json:"unjailableAfter"`
	IsActive        bool                  `json:"isActive"`
	Commission      decimal.Decimal       `json:"commission"`
	Stats           ValidatorStatsPeriods `json:"stats"`
}

// ValidatorPeriodStats associates a reporting period with its validator metrics.
type ValidatorPeriodStats struct {
	Period string
	Stats  ValidatorStats
}

// ValidatorStats is a validator's availability and yield data for one period.
type ValidatorStats struct {
	UptimeFraction decimal.Decimal `json:"uptimeFraction"`
	PredictedAPR   decimal.Decimal `json:"predictedApr"`
	Samples        int64           `json:"nSamples"`
}

func (r *ValidatorPeriodStats) UnmarshalJSON(data []byte) error {
	var tuple []json.RawMessage
	if err := json.Unmarshal(data, &tuple); err != nil {
		return err
	}
	if len(tuple) != 2 {
		return fmt.Errorf("validator period stats must contain period and stats")
	}
	if err := json.Unmarshal(tuple[0], &r.Period); err != nil {
		return err
	}
	return json.Unmarshal(tuple[1], &r.Stats)
}

// ValidatorStatsPeriods is the official fixed, ordered [day, week, month]
// validator-statistics tuple.
type ValidatorStatsPeriods [3]ValidatorPeriodStats

func (p *ValidatorStatsPeriods) UnmarshalJSON(data []byte) error {
	var periods []ValidatorPeriodStats
	if err := json.Unmarshal(data, &periods); err != nil {
		return err
	}
	if len(periods) != len(p) {
		return fmt.Errorf("validator stats must contain day, week, and month")
	}
	for i, expected := range [...]string{"day", "week", "month"} {
		if periods[i].Period != expected {
			return fmt.Errorf("validator stats period %d must be %q, got %q", i, expected, periods[i].Period)
		}
		p[i] = periods[i]
	}
	return nil
}

// ValidatorL1Vote is an active validator governance vote.
type ValidatorL1Vote struct {
	ExpireTime    int64                 `json:"expireTime"`
	Action        ValidatorL1VoteAction `json:"action"`
	Votes         []string              `json:"votes"`
	QuorumReached bool                  `json:"quorumReached"`
}

// ValidatorL1VoteAction is the protocol's tagged L1 governance action union.
// Exactly one member is set after decoding.
type ValidatorL1VoteAction struct {
	D *string                  `json:"D,omitempty"`
	C *[]string                `json:"C,omitempty"`
	O *OutcomeGovernanceVote   `json:"O,omitempty"`
	E *TokenTreasuryGovernance `json:"E,omitempty"`
}

// OutcomeGovernanceVote holds a single outcome-market governance action.
type OutcomeGovernanceVote struct {
	RegisterTokensAndStandaloneOutcome *RegisterStandaloneOutcome `json:"registerTokensAndStandaloneOutcome,omitempty"`
	RegisterTokensAndQuestion          *RegisterQuestion          `json:"registerTokensAndQuestion,omitempty"`
	SettleOutcome                      *SettleOutcomeVote         `json:"settleOutcome,omitempty"`
	SettleQuestion                     *SettleQuestionVote        `json:"settleQuestion,omitempty"`
	SettleQuestion2                    *SettleQuestion2Vote       `json:"settleQuestion2,omitempty"`
}

type RegisterStandaloneOutcome struct {
	QuoteToken         int        `json:"quoteToken"`
	NameAndDescription StringPair `json:"nameAndDescription"`
	SideNames          []string   `json:"sideNames"`
}
type RegisterQuestion struct {
	QuoteToken                 int          `json:"quoteToken"`
	QuestionNameAndDescription StringPair   `json:"questionNameAndDescription"`
	FallbackNameAndDescription StringPair   `json:"fallbackNameAndDescription"`
	NamedOutcomes              []StringPair `json:"namedOutcomes"`
}
type SettleOutcomeVote struct {
	Outcome            int             `json:"outcome"`
	SettleFraction     decimal.Decimal `json:"settleFraction"`
	Details            string          `json:"details"`
	NameAndDescription StringPair      `json:"nameAndDescription"`
	SideNames          []string        `json:"sideNames"`
}
type SettleQuestionVote struct {
	Question                  int                  `json:"question"`
	SettleFractionsAndDetails []QuestionSettlement `json:"settleFractionsAndDetails"`
}
type QuestionSettlement struct {
	Outcome int
	Details SettlementFractionDetails
}
type SettlementFractionDetails struct {
	SettleFraction decimal.Decimal
	Details        string
}

// SettleQuestion2Vote is the current outcome-market question settlement action.
type SettleQuestion2Vote struct {
	Question           int                  `json:"question"`
	OutcomeSettlements []OutcomeSettlement2 `json:"outcomeSettlements"`
	NameAndDescription StringPair           `json:"nameAndDescription"`
}

// OutcomeSettlement2 is one named outcome resolved as part of settleQuestion2.
type OutcomeSettlement2 struct {
	Outcome            int             `json:"outcome"`
	SettleFraction     decimal.Decimal `json:"settleFraction"`
	Details            string          `json:"details"`
	NameAndDescription StringPair      `json:"nameAndDescription"`
	SideNames          []string        `json:"sideNames"`
}
type TokenTreasuryGovernance struct {
	Token                int    `json:"token"`
	TechnicalStaker      string `json:"technicalStaker"`
	TreasuryStaker       string `json:"treasuryStaker"`
	TreasuryEVMAddress   string `json:"treasuryEvmAddress"`
	EVMRebalanceContract string `json:"evmRebalanceContract"`
}

func (a *ValidatorL1VoteAction) UnmarshalJSON(data []byte) error {
	type wire ValidatorL1VoteAction
	var decoded wire
	if err := json.Unmarshal(data, &decoded); err != nil {
		return err
	}
	branches := 0
	if decoded.D != nil {
		branches++
	}
	if decoded.C != nil {
		branches++
	}
	if decoded.O != nil {
		branches++
	}
	if decoded.E != nil {
		branches++
	}
	if branches != 1 {
		return fmt.Errorf("validator L1 vote action must contain exactly one action")
	}
	*a = ValidatorL1VoteAction(decoded)
	return nil
}

func (v *OutcomeGovernanceVote) UnmarshalJSON(data []byte) error {
	type wire OutcomeGovernanceVote
	var decoded wire
	if err := json.Unmarshal(data, &decoded); err != nil {
		return err
	}
	branches := 0
	if decoded.RegisterTokensAndStandaloneOutcome != nil {
		branches++
	}
	if decoded.RegisterTokensAndQuestion != nil {
		branches++
	}
	if decoded.SettleOutcome != nil {
		branches++
	}
	if decoded.SettleQuestion != nil {
		branches++
	}
	if decoded.SettleQuestion2 != nil {
		branches++
	}
	if branches != 1 {
		return fmt.Errorf("outcome governance vote must contain exactly one action")
	}
	*v = OutcomeGovernanceVote(decoded)
	return nil
}

// StringPair is an exactly two-string protocol tuple.
type StringPair [2]string

func (p *StringPair) UnmarshalJSON(data []byte) error {
	var tuple []json.RawMessage
	if err := json.Unmarshal(data, &tuple); err != nil {
		return err
	}
	if len(tuple) != 2 {
		return fmt.Errorf("string pair must contain exactly two values")
	}
	if err := json.Unmarshal(tuple[0], &p[0]); err != nil {
		return err
	}
	return json.Unmarshal(tuple[1], &p[1])
}

func (s *QuestionSettlement) UnmarshalJSON(data []byte) error {
	var tuple []json.RawMessage
	if err := json.Unmarshal(data, &tuple); err != nil {
		return err
	}
	if len(tuple) != 2 {
		return fmt.Errorf("question settlement must contain outcome and details")
	}
	if err := json.Unmarshal(tuple[0], &s.Outcome); err != nil {
		return err
	}
	return json.Unmarshal(tuple[1], &s.Details)
}
func (s *SettlementFractionDetails) UnmarshalJSON(data []byte) error {
	var tuple []json.RawMessage
	if err := json.Unmarshal(data, &tuple); err != nil {
		return err
	}
	if len(tuple) != 2 {
		return fmt.Errorf("settlement details must contain fraction and details")
	}
	if err := json.Unmarshal(tuple[0], &s.SettleFraction); err != nil {
		return err
	}
	return json.Unmarshal(tuple[1], &s.Details)
}

// MarginTableResponse contains the tiers for one requested margin table.
type MarginTableResponse struct {
	Description string       `json:"description"`
	MarginTiers []MarginTier `json:"marginTiers"`
}

// LeadingVault identifies a vault led by a user.
type LeadingVault struct {
	Address string `json:"address"`
	Name    string `json:"name"`
}

// SubaccountV2 contains all perpetual DEX states and the account's spot state.
type SubaccountV2 struct {
	Name      string                         `json:"name"`
	User      string                         `json:"subAccountUser"`
	Master    string                         `json:"master"`
	DEXStates []DEXClearinghouseState        `json:"dexToClearinghouseState"`
	SpotState SpotClearinghouseStateResponse `json:"spotState"`
}

// DEXClearinghouseState associates a DEX with a perpetual account state.
type DEXClearinghouseState struct {
	DEX   string
	State ClearinghouseStateResponse
}

func (s *DEXClearinghouseState) UnmarshalJSON(data []byte) error {
	var tuple []json.RawMessage
	if err := json.Unmarshal(data, &tuple); err != nil {
		return err
	}
	if len(tuple) != 2 {
		return fmt.Errorf("DEX clearinghouse state must contain DEX and state")
	}
	if err := json.Unmarshal(tuple[0], &s.DEX); err != nil {
		return err
	}
	return json.Unmarshal(tuple[1], &s.State)
}

// TWAPState is the state of a protocol-native TWAP order.
type TWAPState struct {
	Coin             string          `json:"coin"`
	ExecutedNotional decimal.Decimal `json:"executedNtl"`
	ExecutedSize     decimal.Decimal `json:"executedSz"`
	Minutes          uint64          `json:"minutes"`
	ReduceOnly       bool            `json:"reduceOnly"`
	Randomize        bool            `json:"randomize"`
	Side             string          `json:"side"`
	Size             decimal.Decimal `json:"size"`
	Timestamp        int64           `json:"timestamp"`
	User             string          `json:"user"`
}

// TWAPHistoryEntry is one completed or active TWAP order state.
type TWAPHistoryEntry struct {
	Time   int64             `json:"time"`
	State  TWAPState         `json:"state"`
	Status TWAPHistoryStatus `json:"status"`
	TWAPID *uint64           `json:"twapId,omitempty"`
}

// TWAPHistoryStatus is a TWAP completion or error status. An error carries a
// description; non-error terminal states do not.
type TWAPHistoryStatus struct {
	Status      string `json:"status"`
	Description string `json:"description,omitempty"`
}

func (s *TWAPHistoryStatus) UnmarshalJSON(data []byte) error {
	var wire struct {
		Status      string          `json:"status"`
		Description json.RawMessage `json:"description"`
	}
	if err := json.Unmarshal(data, &wire); err != nil {
		return err
	}
	switch wire.Status {
	case "finished", "activated", "terminated":
		if len(wire.Description) != 0 && string(wire.Description) != "null" {
			return fmt.Errorf("TWAP %s status must not carry a description", wire.Status)
		}
		s.Status, s.Description = wire.Status, ""
		return nil
	case "error":
		if len(wire.Description) == 0 || string(wire.Description) == "null" {
			return fmt.Errorf("TWAP error status requires a description")
		}
		if err := json.Unmarshal(wire.Description, &s.Description); err != nil {
			return err
		}
		s.Status = wire.Status
		return nil
	default:
		return fmt.Errorf("unknown TWAP history status %q", wire.Status)
	}
}

// MultiSigSigners contains an account's authorized multisig users and threshold.
type MultiSigSigners struct {
	AuthorizedUsers []string `json:"authorizedUsers"`
	Threshold       int64    `json:"threshold"`
}

// MaxMarketOrderNotional associates a leverage limit with a maximum market order notional.
type MaxMarketOrderNotional struct {
	MaxLeverage int
	Notional    decimal.Decimal
}

func (m *MaxMarketOrderNotional) UnmarshalJSON(data []byte) error {
	var tuple []json.RawMessage
	if err := json.Unmarshal(data, &tuple); err != nil {
		return err
	}
	if len(tuple) != 2 {
		return fmt.Errorf("max market order notional must contain leverage and notional")
	}
	if err := json.Unmarshal(tuple[0], &m.MaxLeverage); err != nil {
		return err
	}
	return json.Unmarshal(tuple[1], &m.Notional)
}

// LiquidatablePosition is an account position currently eligible for liquidation.
type LiquidatablePosition struct {
	User            string                    `json:"user"`
	PositionIndex   LiquidatablePositionIndex `json:"positionIndex"`
	MarginAvailable DecimalPair               `json:"marginAvailable"`
}

// DecimalPair is an exactly two-decimal protocol tuple.
type DecimalPair [2]decimal.Decimal

func (p *DecimalPair) UnmarshalJSON(data []byte) error {
	var tuple []json.RawMessage
	if err := json.Unmarshal(data, &tuple); err != nil {
		return err
	}
	if len(tuple) != 2 {
		return fmt.Errorf("decimal pair must contain exactly two values")
	}
	if err := json.Unmarshal(tuple[0], &p[0]); err != nil {
		return err
	}
	return json.Unmarshal(tuple[1], &p[1])
}

type LiquidatablePositionIndex struct {
	Isolated *LiquidatableIsolatedPosition `json:"isolated,omitempty"`
}
type LiquidatableIsolatedPosition struct {
	Asset int `json:"asset"`
}

// SettledOutcomeResponse contains the settlement information for an outcome.
type SettledOutcomeResponse struct {
	Spec           Outcome                 `json:"spec"`
	SettleFraction decimal.Decimal         `json:"settleFraction"`
	Details        string                  `json:"details"`
	Question       *SettledOutcomeQuestion `json:"question,omitempty"`
}

// SettledOutcomeQuestion identifies the question associated with a settled outcome.
type SettledOutcomeQuestion struct {
	ID          SettledOutcomeQuestionID `json:"question"`
	Name        string                   `json:"name"`
	Description string                   `json:"description"`
}

// SettledOutcomeQuestionID is the active-or-settled question identifier union.
type SettledOutcomeQuestionID struct {
	Active  *int `json:"active,omitempty"`
	Settled *int `json:"settled,omitempty"`
}

func (q *SettledOutcomeQuestionID) UnmarshalJSON(data []byte) error {
	type wire SettledOutcomeQuestionID
	var decoded wire
	if err := json.Unmarshal(data, &decoded); err != nil {
		return err
	}
	if (decoded.Active == nil) == (decoded.Settled == nil) {
		return fmt.Errorf("settled outcome question must contain exactly one of active or settled")
	}
	*q = SettledOutcomeQuestionID(decoded)
	return nil
}

func (c *Client) ExchangeStatus(ctx context.Context) (ExchangeStatusResponse, error) {
	var r ExchangeStatusResponse
	err := c.call(ctx, struct {
		Type string `json:"type"`
	}{"exchangeStatus"}, &r)
	return r, err
}
func (c *Client) ExtraAgents(ctx context.Context, user string) ([]ExtraAgent, error) {
	if user == "" {
		return nil, fmt.Errorf("user is required")
	}
	var r []ExtraAgent
	err := c.call(ctx, userRequest{"extraAgents", user}, &r)
	return r, err
}
func (c *Client) GossipPriorityAuctionStatus(ctx context.Context) (GossipPriorityAuctionStatusResponse, error) {
	var r GossipPriorityAuctionStatusResponse
	err := c.call(ctx, struct {
		Type string `json:"type"`
	}{"gossipPriorityAuctionStatus"}, &r)
	return r, err
}
func (c *Client) GossipRootIPs(ctx context.Context) ([]string, error) {
	var r []string
	err := c.call(ctx, struct {
		Type string `json:"type"`
	}{"gossipRootIps"}, &r)
	return r, err
}
func (c *Client) IsVIP(ctx context.Context, user string) (*bool, error) {
	if user == "" {
		return nil, fmt.Errorf("user is required")
	}
	var r *bool
	err := c.call(ctx, userRequest{"isVip", user}, &r)
	return r, err
}
func (c *Client) LegalCheck(ctx context.Context, user string) (LegalCheckResponse, error) {
	if user == "" {
		return LegalCheckResponse{}, fmt.Errorf("user is required")
	}
	var r LegalCheckResponse
	err := c.call(ctx, userRequest{"legalCheck", user}, &r)
	return r, err
}
func (c *Client) PreTransferCheck(ctx context.Context, user, source string) (PreTransferCheckResponse, error) {
	if user == "" || source == "" {
		return PreTransferCheckResponse{}, fmt.Errorf("user and source are required")
	}
	var r PreTransferCheckResponse
	err := c.call(ctx, struct {
		Type   string `json:"type"`
		User   string `json:"user"`
		Source string `json:"source"`
	}{"preTransferCheck", user, source}, &r)
	return r, err
}
func (c *Client) VaultSummaries(ctx context.Context) ([]VaultSummary, error) {
	var r []VaultSummary
	err := c.call(ctx, struct {
		Type string `json:"type"`
	}{"vaultSummaries"}, &r)
	return r, err
}
func (c *Client) ValidatorSummaries(ctx context.Context) ([]ValidatorSummary, error) {
	var r []ValidatorSummary
	err := c.call(ctx, struct {
		Type string `json:"type"`
	}{"validatorSummaries"}, &r)
	return r, err
}
func (c *Client) ValidatorL1Votes(ctx context.Context) ([]ValidatorL1Vote, error) {
	var r []ValidatorL1Vote
	err := c.call(ctx, struct {
		Type string `json:"type"`
	}{"validatorL1Votes"}, &r)
	return r, err
}
func (c *Client) MarginTable(ctx context.Context, id int) (MarginTableResponse, error) {
	if id < 0 {
		return MarginTableResponse{}, fmt.Errorf("margin table ID must be non-negative")
	}
	var r MarginTableResponse
	err := c.call(ctx, struct {
		Type string `json:"type"`
		ID   int    `json:"id"`
	}{"marginTable", id}, &r)
	return r, err
}
func (c *Client) LeadingVaults(ctx context.Context, user string) ([]LeadingVault, error) {
	if user == "" {
		return nil, fmt.Errorf("user is required")
	}
	var r []LeadingVault
	err := c.call(ctx, userRequest{"leadingVaults", user}, &r)
	return r, err
}
func (c *Client) Subaccounts2(ctx context.Context, user string) (*[]SubaccountV2, error) {
	if user == "" {
		return nil, fmt.Errorf("user is required")
	}
	var r *[]SubaccountV2
	err := c.call(ctx, userRequest{"subAccounts2", user}, &r)
	return r, err
}
func (c *Client) TWAPHistory(ctx context.Context, user string) ([]TWAPHistoryEntry, error) {
	if user == "" {
		return nil, fmt.Errorf("user is required")
	}
	var r []TWAPHistoryEntry
	err := c.call(ctx, userRequest{"twapHistory", user}, &r)
	return r, err
}
func (c *Client) UserToMultiSigSigners(ctx context.Context, user string) (*MultiSigSigners, error) {
	if user == "" {
		return nil, fmt.Errorf("user is required")
	}
	var r *MultiSigSigners
	err := c.call(ctx, userRequest{"userToMultiSigSigners", user}, &r)
	return r, err
}
func (c *Client) MaxMarketOrderNotionals(ctx context.Context) ([]MaxMarketOrderNotional, error) {
	var r []MaxMarketOrderNotional
	err := c.call(ctx, struct {
		Type string `json:"type"`
	}{"maxMarketOrderNtls"}, &r)
	return r, err
}
func (c *Client) Liquidatable(ctx context.Context) ([]LiquidatablePosition, error) {
	var r []LiquidatablePosition
	err := c.call(ctx, struct {
		Type string `json:"type"`
	}{"liquidatable"}, &r)
	return r, err
}
func (c *Client) SettledOutcome(ctx context.Context, outcome int) (*SettledOutcomeResponse, error) {
	if outcome < 0 {
		return nil, fmt.Errorf("outcome must be non-negative")
	}
	var r *SettledOutcomeResponse
	err := c.call(ctx, struct {
		Type    string `json:"type"`
		Outcome int    `json:"outcome"`
	}{"settledOutcome", outcome}, &r)
	return r, err
}
