package websocket

import (
	"bytes"
	"compress/flate"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strings"

	"github.com/Apexllcc/hyperliquid-go-sdk/info"
	"github.com/shopspring/decimal"
)

// UserDEXRequest identifies an account stream on one perpetual DEX. An empty
// DEX denotes Hyperliquid's main perpetual DEX, as required by the protocol.
type UserDEXRequest struct {
	User string `json:"user"`
	DEX  string `json:"dex,omitempty"`
}

// DEXRequest identifies a public perpetual DEX stream. An empty DEX denotes
// the main perpetual DEX.
type DEXRequest struct {
	DEX string `json:"dex,omitempty"`
}

// ActiveAssetDataRequest identifies a user's live trade-capacity stream.
type ActiveAssetDataRequest struct {
	Coin string `json:"coin"`
	User string `json:"user"`
}

// SpotStateRequest identifies a user's spot-state stream. IsPortfolioMargin
// is the optional field documented by Hyperliquid; nil leaves the server
// default unchanged.
type SpotStateRequest struct {
	User              string `json:"user"`
	IsPortfolioMargin *bool  `json:"isPortfolioMargin,omitempty"`
}

// NotificationEvent is an account notification. The notification channel does
// not include the user address in its payload.
type NotificationEvent struct {
	Notification string `json:"notification"`
}

// WebData3Event is the documented aggregate user/dashboard payload. Economic
// values are retained as decimal.Decimal rather than float64.
type WebData3Event struct {
	UserState     WebData3UserState      `json:"userState"`
	PerpDEXStates []WebData3PerpDEXState `json:"perpDexStates"`
}

type WebData3UserState struct {
	AgentAddress          *string         `json:"agentAddress"`
	AgentValidUntil       *int64          `json:"agentValidUntil"`
	CumulativeLedger      decimal.Decimal `json:"cumLedger"`
	ServerTime            int64           `json:"serverTime"`
	IsVault               bool            `json:"isVault"`
	User                  string          `json:"user"`
	OptOutOfSpotDusting   bool            `json:"optOutOfSpotDusting,omitempty"`
	DEXAbstractionEnabled bool            `json:"dexAbstractionEnabled,omitempty"`
	Abstraction           string          `json:"abstraction,omitempty"`
}

type WebData3PerpDEXState struct {
	TotalVaultEquity       decimal.Decimal `json:"totalVaultEquity"`
	PerpsAtOpenInterestCap []string        `json:"perpsAtOpenInterestCap,omitempty"`
	LeadingVaults          []LeadingVault  `json:"leadingVaults,omitempty"`
}

// LeadingVault identifies a leading vault included in a WebData3 DEX state.
type LeadingVault struct {
	Address string `json:"address"`
	Name    string `json:"name"`
}

// OpenOrdersEvent is the live frontend-open-orders view for one user and DEX.
type OpenOrdersEvent struct {
	DEX    string                   `json:"dex"`
	User   string                   `json:"user"`
	Orders []info.FrontendOpenOrder `json:"orders"`
}

// ClearinghouseStateEvent is a live perpetual account state for one DEX.
type ClearinghouseStateEvent struct {
	DEX                string                          `json:"dex"`
	User               string                          `json:"user"`
	ClearinghouseState info.ClearinghouseStateResponse `json:"clearinghouseState"`
}

// TWAPState describes a live TWAP order state.
type TWAPState struct {
	Coin        string          `json:"coin"`
	ExecutedNtl decimal.Decimal `json:"executedNtl"`
	ExecutedSz  decimal.Decimal `json:"executedSz"`
	Minutes     int64           `json:"minutes"`
	Randomize   bool            `json:"randomize"`
	ReduceOnly  bool            `json:"reduceOnly"`
	Side        string          `json:"side"`
	Size        decimal.Decimal `json:"sz"`
	Timestamp   int64           `json:"timestamp"`
	User        string          `json:"user"`
}

// TWAPStateEntry is the documented [twapID, state] tuple.
type TWAPStateEntry struct {
	TWAPID uint64
	State  TWAPState
}

func (e *TWAPStateEntry) UnmarshalJSON(data []byte) error {
	var tuple []json.RawMessage
	if err := json.Unmarshal(data, &tuple); err != nil {
		return err
	}
	if len(tuple) != 2 {
		return fmt.Errorf("TWAP state must contain ID and state")
	}
	if err := json.Unmarshal(tuple[0], &e.TWAPID); err != nil {
		return err
	}
	return json.Unmarshal(tuple[1], &e.State)
}

// TWAPStatesEvent contains every active TWAP for one user and DEX.
type TWAPStatesEvent struct {
	DEX    string           `json:"dex"`
	User   string           `json:"user"`
	States []TWAPStateEntry `json:"states"`
}

// UserTWAPSliceFillsEvent is a snapshot or incremental TWAP fill batch.
type UserTWAPSliceFillsEvent struct {
	User           string               `json:"user"`
	TWAPSliceFills []info.TwapSliceFill `json:"twapSliceFills"`
	IsSnapshot     bool                 `json:"isSnapshot,omitempty"`
}

// TWAPHistoryStatus is the result of a historical TWAP. Description is set
// only for the protocol's error variant.
type TWAPHistoryStatus struct {
	Status      string `json:"status"`
	Description string `json:"description,omitempty"`
}

// TWAPHistoryEntry records one completed, terminated, or active TWAP.
type TWAPHistoryEntry struct {
	Time   int64             `json:"time"`
	State  TWAPState         `json:"state"`
	Status TWAPHistoryStatus `json:"status"`
	TWAPID *uint64           `json:"twapId,omitempty"`
}

// UserTWAPHistoryEvent is a snapshot or incremental TWAP history batch.
type UserTWAPHistoryEvent struct {
	User       string             `json:"user"`
	History    []TWAPHistoryEntry `json:"history"`
	IsSnapshot bool               `json:"isSnapshot,omitempty"`
}

// SpotStateEvent is a live spot account state.
type SpotStateEvent struct {
	User      string                              `json:"user"`
	SpotState info.SpotClearinghouseStateResponse `json:"spotState"`
}

// DEXClearinghouseState is one [dex, clearinghouseState] tuple.
type DEXClearinghouseState struct {
	DEX   string
	State info.ClearinghouseStateResponse
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

// AllDEXsClearinghouseStateEvent contains a user's account state on each DEX.
type AllDEXsClearinghouseStateEvent struct {
	User                string                  `json:"user"`
	ClearinghouseStates []DEXClearinghouseState `json:"clearinghouseStates"`
}

// DEXAssetContexts is one [dex, contexts] tuple.
type DEXAssetContexts struct {
	DEX      string
	Contexts []info.AssetContext
}

func (c *DEXAssetContexts) UnmarshalJSON(data []byte) error {
	var tuple []json.RawMessage
	if err := json.Unmarshal(data, &tuple); err != nil {
		return err
	}
	if len(tuple) != 2 {
		return fmt.Errorf("DEX asset contexts must contain DEX and contexts")
	}
	if err := json.Unmarshal(tuple[0], &c.DEX); err != nil {
		return err
	}
	return json.Unmarshal(tuple[1], &c.Contexts)
}

// AllDEXsAssetCtxsEvent contains perpetual contexts for every DEX.
type AllDEXsAssetCtxsEvent struct {
	Contexts []DEXAssetContexts `json:"ctxs"`
}

// AssetCtxsEvent contains current perp contexts on a single DEX.
type AssetCtxsEvent struct {
	DEX      string              `json:"dex"`
	Contexts []info.AssetContext `json:"ctxs"`
}

// FastAssetCtx is a compact mark/mid update. Both values are optional because
// incremental messages include only changed fields.
type FastAssetCtx struct {
	MarkPrice *decimal.Decimal `json:"markPx,omitempty"`
	MidPrice  *decimal.Decimal `json:"midPx,omitempty"`
}

// FastAssetCtxsEvent is the decompressed compact mark/mid map keyed by coin.
type FastAssetCtxsEvent map[string]FastAssetCtx

// SpotAssetCtxsEvent contains current contexts for all spot assets.
type SpotAssetCtxsEvent []info.SpotAssetContext

// ActiveSpotAssetCtxSubscription is the spot-specialized spelling of the
// protocol's activeAssetCtx feed. Its wire request is intentionally identical
// to ActiveAssetCtx; Hyperliquid selects the spot variant from the coin.
type ActiveSpotAssetCtxSubscription = ActiveAssetCtxSubscription

// UserHistoricalOrdersEvent is a snapshot or incremental historical-order batch.
type UserHistoricalOrdersEvent struct {
	User         string                 `json:"user"`
	OrderHistory []info.HistoricalOrder `json:"orderHistory"`
	IsSnapshot   bool                   `json:"isSnapshot,omitempty"`
}

// OutcomeMetaUpdatesEvent holds prediction-market outcome and question updates.
type OutcomeMetaUpdatesEvent struct {
	Updates []OutcomeMetaUpdate `json:"updates"`
}

// OutcomeMetaUpdate is a tagged union; exactly one known pointer is set.
// Unknown variants retain their raw JSON to preserve forward compatibility.
type OutcomeMetaUpdate struct {
	OutcomeCreated  *OutcomeCreatedEvent  `json:"-"`
	OutcomeSettled  *OutcomeSettledEvent  `json:"-"`
	QuestionUpdated *QuestionUpdatedEvent `json:"-"`
	QuestionSettled *QuestionSettledEvent `json:"-"`
	Raw             json.RawMessage       `json:"-"`
}

type OutcomeCreatedEvent struct {
	Outcome     uint64            `json:"outcome"`
	Name        string            `json:"name"`
	Description string            `json:"description"`
	SideSpecs   []OutcomeSideSpec `json:"sideSpecs"`
	QuoteToken  string            `json:"quoteToken"`
}
type OutcomeSideSpec struct {
	Name  string  `json:"name"`
	Token *uint64 `json:"token,omitempty"`
}
type OutcomeSettledEvent struct {
	Outcome uint64 `json:"outcome"`
}
type QuestionUpdatedEvent struct {
	Question             uint64   `json:"question"`
	Name                 string   `json:"name"`
	Description          string   `json:"description"`
	FallbackOutcome      uint64   `json:"fallbackOutcome"`
	NamedOutcomes        []uint64 `json:"namedOutcomes"`
	SettledNamedOutcomes []uint64 `json:"settledNamedOutcomes"`
}
type QuestionSettledEvent struct {
	Question uint64 `json:"question"`
}

func (u *OutcomeMetaUpdate) UnmarshalJSON(data []byte) error {
	var tag map[string]json.RawMessage
	if err := json.Unmarshal(data, &tag); err != nil {
		return err
	}
	*u = OutcomeMetaUpdate{Raw: append(json.RawMessage(nil), data...)}
	switch {
	case tag["outcomeCreated"] != nil:
		u.OutcomeCreated = new(OutcomeCreatedEvent)
		return json.Unmarshal(tag["outcomeCreated"], u.OutcomeCreated)
	case tag["outcomeSettled"] != nil:
		u.OutcomeSettled = new(OutcomeSettledEvent)
		return json.Unmarshal(tag["outcomeSettled"], u.OutcomeSettled)
	case tag["questionUpdated"] != nil:
		u.QuestionUpdated = new(QuestionUpdatedEvent)
		return json.Unmarshal(tag["questionUpdated"], u.QuestionUpdated)
	case tag["questionSettled"] != nil:
		u.QuestionSettled = new(QuestionSettledEvent)
		return json.Unmarshal(tag["questionSettled"], u.QuestionSettled)
	default:
		return nil
	}
}

type NotificationSubscription struct {
	*streamSubscription[NotificationEvent]
}
type WebData3Subscription struct {
	*streamSubscription[WebData3Event]
}
type OpenOrdersStreamSubscription struct {
	*streamSubscription[OpenOrdersEvent]
}
type ClearinghouseStateSubscription struct {
	*streamSubscription[ClearinghouseStateEvent]
}
type ActiveAssetDataSubscription struct {
	*streamSubscription[info.ActiveAssetDataResponse]
}
type TWAPStatesSubscription struct {
	*streamSubscription[TWAPStatesEvent]
}
type UserTWAPSliceFillsSubscription struct {
	*streamSubscription[UserTWAPSliceFillsEvent]
}
type UserTWAPHistorySubscription struct {
	*streamSubscription[UserTWAPHistoryEvent]
}
type SpotStateSubscription struct {
	*streamSubscription[SpotStateEvent]
}
type AllDEXsClearinghouseStateSubscription struct {
	*streamSubscription[AllDEXsClearinghouseStateEvent]
}
type AllDEXsAssetCtxsSubscription struct {
	*streamSubscription[AllDEXsAssetCtxsEvent]
}
type AssetCtxsSubscription struct {
	*streamSubscription[AssetCtxsEvent]
}
type FastAssetCtxsSubscription struct {
	*streamSubscription[FastAssetCtxsEvent]
}
type SpotAssetCtxsSubscription struct {
	*streamSubscription[SpotAssetCtxsEvent]
}
type UserHistoricalOrdersSubscription struct {
	*streamSubscription[UserHistoricalOrdersEvent]
}
type OutcomeMetaUpdatesSubscription struct {
	*streamSubscription[OutcomeMetaUpdatesEvent]
}

func extendedHandle[T any, H any](c *Client, key string, subscription *streamSubscription[T], newHandle func(*streamSubscription[T]) H) (H, error) {
	var zero H
	handle, current := c.cachePrivateHandle(key, subscription, func() any { return newHandle(subscription) })
	if !current {
		return zero, ErrWebSocketClosed
	}
	typed, ok := handle.(H)
	if !ok {
		return zero, errors.New("websocket subscription registry type conflict")
	}
	return typed, nil
}

func subscribeUser[T any, H any](ctx context.Context, c *Client, kind, key, user string, fields map[string]any, decode func(json.RawMessage) (T, error), match func(T) bool, oneChannel bool, newHandle func(*streamSubscription[T]) H) (H, error) {
	var zero H
	if err := requireUser(user); err != nil {
		return zero, err
	}
	subscription, err := subscribeStream(ctx, c, key, kind, newSubscriptionWire(kind, fields), decode, match, func(subscriptions map[string]managedSubscription) error {
		if oneChannel {
			return onePerChannel(kind)(subscriptions)
		}
		return nil
	})
	if err != nil {
		return zero, err
	}
	return extendedHandle(c, key, subscription, newHandle)
}

// SubscribeNotification streams account notifications. Its payload has no
// user field, so distinct users cannot safely share the same client channel.
func (c *Client) SubscribeNotification(ctx context.Context, user string) (*NotificationSubscription, error) {
	key := "notification:" + strings.ToLower(user)
	return subscribeUser(ctx, c, "notification", key, user, map[string]any{"user": user}, decodeJSON[NotificationEvent], func(NotificationEvent) bool { return true }, true, func(s *streamSubscription[NotificationEvent]) *NotificationSubscription {
		return &NotificationSubscription{s}
	})
}

func (c *Client) SubscribeWebData3(ctx context.Context, user string) (*WebData3Subscription, error) {
	key := "webData3:" + strings.ToLower(user)
	return subscribeUser(ctx, c, "webData3", key, user, map[string]any{"user": user}, decodeJSON[WebData3Event], func(event WebData3Event) bool { return strings.EqualFold(event.UserState.User, user) }, false, func(s *streamSubscription[WebData3Event]) *WebData3Subscription { return &WebData3Subscription{s} })
}

func (c *Client) SubscribeOpenOrders(ctx context.Context, request UserDEXRequest) (*OpenOrdersStreamSubscription, error) {
	key := "openOrders:" + strings.ToLower(request.User) + ":" + request.DEX
	return subscribeUser(ctx, c, "openOrders", key, request.User, map[string]any{"user": request.User, "dex": request.DEX}, decodeJSON[OpenOrdersEvent], func(event OpenOrdersEvent) bool {
		return strings.EqualFold(event.User, request.User) && event.DEX == request.DEX
	}, false, func(s *streamSubscription[OpenOrdersEvent]) *OpenOrdersStreamSubscription {
		return &OpenOrdersStreamSubscription{s}
	})
}

func (c *Client) SubscribeClearinghouseState(ctx context.Context, request UserDEXRequest) (*ClearinghouseStateSubscription, error) {
	key := "clearinghouseState:" + strings.ToLower(request.User) + ":" + request.DEX
	return subscribeUser(ctx, c, "clearinghouseState", key, request.User, map[string]any{"user": request.User, "dex": request.DEX}, decodeJSON[ClearinghouseStateEvent], func(event ClearinghouseStateEvent) bool {
		return strings.EqualFold(event.User, request.User) && event.DEX == request.DEX
	}, false, func(s *streamSubscription[ClearinghouseStateEvent]) *ClearinghouseStateSubscription {
		return &ClearinghouseStateSubscription{s}
	})
}

func (c *Client) SubscribeActiveAssetData(ctx context.Context, request ActiveAssetDataRequest) (*ActiveAssetDataSubscription, error) {
	if strings.TrimSpace(request.Coin) == "" {
		return nil, errors.New("coin is required")
	}
	key := "activeAssetData:" + strings.ToLower(request.User) + ":" + request.Coin
	return subscribeUser(ctx, c, "activeAssetData", key, request.User, map[string]any{"user": request.User, "coin": request.Coin}, decodeJSON[info.ActiveAssetDataResponse], func(event info.ActiveAssetDataResponse) bool {
		return strings.EqualFold(event.User, request.User) && event.Coin == request.Coin
	}, false, func(s *streamSubscription[info.ActiveAssetDataResponse]) *ActiveAssetDataSubscription {
		return &ActiveAssetDataSubscription{s}
	})
}

// SubscribeActiveSpotAssetCtx subscribes to a spot activeAssetCtx stream. The
// official protocol uses the same "activeAssetCtx" subscription type for perp
// and spot assets, so this method is an ergonomic alias, not another wire API.
func (c *Client) SubscribeActiveSpotAssetCtx(ctx context.Context, coin string) (*ActiveSpotAssetCtxSubscription, error) {
	return c.SubscribeActiveAssetCtx(ctx, ActiveAssetCtxRequest{Coin: coin})
}

func (c *Client) SubscribeTWAPStates(ctx context.Context, request UserDEXRequest) (*TWAPStatesSubscription, error) {
	key := "twapStates:" + strings.ToLower(request.User) + ":" + request.DEX
	return subscribeUser(ctx, c, "twapStates", key, request.User, map[string]any{"user": request.User, "dex": request.DEX}, decodeJSON[TWAPStatesEvent], func(event TWAPStatesEvent) bool {
		return strings.EqualFold(event.User, request.User) && event.DEX == request.DEX
	}, false, func(s *streamSubscription[TWAPStatesEvent]) *TWAPStatesSubscription {
		return &TWAPStatesSubscription{s}
	})
}

func (c *Client) SubscribeUserTWAPSliceFills(ctx context.Context, user string) (*UserTWAPSliceFillsSubscription, error) {
	key := "userTwapSliceFills:" + strings.ToLower(user)
	return subscribeUser(ctx, c, "userTwapSliceFills", key, user, map[string]any{"user": user}, decodeJSON[UserTWAPSliceFillsEvent], func(event UserTWAPSliceFillsEvent) bool { return strings.EqualFold(event.User, user) }, false, func(s *streamSubscription[UserTWAPSliceFillsEvent]) *UserTWAPSliceFillsSubscription {
		return &UserTWAPSliceFillsSubscription{s}
	})
}

func (c *Client) SubscribeUserTWAPHistory(ctx context.Context, user string) (*UserTWAPHistorySubscription, error) {
	key := "userTwapHistory:" + strings.ToLower(user)
	return subscribeUser(ctx, c, "userTwapHistory", key, user, map[string]any{"user": user}, decodeJSON[UserTWAPHistoryEvent], func(event UserTWAPHistoryEvent) bool { return strings.EqualFold(event.User, user) }, false, func(s *streamSubscription[UserTWAPHistoryEvent]) *UserTWAPHistorySubscription {
		return &UserTWAPHistorySubscription{s}
	})
}

func (c *Client) SubscribeSpotState(ctx context.Context, request SpotStateRequest) (*SpotStateSubscription, error) {
	key := fmt.Sprintf("spotState:%s:%v", strings.ToLower(request.User), request.IsPortfolioMargin)
	fields := map[string]any{"user": request.User}
	if request.IsPortfolioMargin != nil {
		fields["isPortfolioMargin"] = *request.IsPortfolioMargin
	}
	return subscribeUser(ctx, c, "spotState", key, request.User, fields, decodeJSON[SpotStateEvent], func(event SpotStateEvent) bool { return strings.EqualFold(event.User, request.User) }, false, func(s *streamSubscription[SpotStateEvent]) *SpotStateSubscription { return &SpotStateSubscription{s} })
}

func (c *Client) SubscribeAllDEXsClearinghouseState(ctx context.Context, user string) (*AllDEXsClearinghouseStateSubscription, error) {
	key := "allDexsClearinghouseState:" + strings.ToLower(user)
	return subscribeUser(ctx, c, "allDexsClearinghouseState", key, user, map[string]any{"user": user}, decodeJSON[AllDEXsClearinghouseStateEvent], func(event AllDEXsClearinghouseStateEvent) bool { return strings.EqualFold(event.User, user) }, false, func(s *streamSubscription[AllDEXsClearinghouseStateEvent]) *AllDEXsClearinghouseStateSubscription {
		return &AllDEXsClearinghouseStateSubscription{s}
	})
}

func subscribePublic[T any, H any](ctx context.Context, c *Client, kind, key string, fields map[string]any, decode func(json.RawMessage) (T, error), match func(T) bool, newHandle func(*streamSubscription[T]) H) (H, error) {
	var zero H
	subscription, err := subscribeStream(ctx, c, key, kind, newSubscriptionWire(kind, fields), decode, match, nil)
	if err != nil {
		return zero, err
	}
	return extendedHandle(c, key, subscription, newHandle)
}

func (c *Client) SubscribeAllDEXsAssetCtxs(ctx context.Context) (*AllDEXsAssetCtxsSubscription, error) {
	return subscribePublic(ctx, c, "allDexsAssetCtxs", "allDexsAssetCtxs", map[string]any{}, decodeJSON[AllDEXsAssetCtxsEvent], func(AllDEXsAssetCtxsEvent) bool { return true }, func(s *streamSubscription[AllDEXsAssetCtxsEvent]) *AllDEXsAssetCtxsSubscription {
		return &AllDEXsAssetCtxsSubscription{s}
	})
}

func (c *Client) SubscribeAssetCtxs(ctx context.Context, request DEXRequest) (*AssetCtxsSubscription, error) {
	key := "assetCtxs:" + request.DEX
	return subscribePublic(ctx, c, "assetCtxs", key, map[string]any{"dex": request.DEX}, decodeJSON[AssetCtxsEvent], func(event AssetCtxsEvent) bool { return event.DEX == request.DEX }, func(s *streamSubscription[AssetCtxsEvent]) *AssetCtxsSubscription { return &AssetCtxsSubscription{s} })
}

func decodeFastAssetCtxs(data json.RawMessage) (FastAssetCtxsEvent, error) {
	var encoded string
	if err := json.Unmarshal(data, &encoded); err != nil {
		return nil, err
	}
	compressed, err := base64.StdEncoding.DecodeString(encoded)
	if err != nil {
		return nil, fmt.Errorf("decode fast asset contexts base64: %w", err)
	}
	reader := flate.NewReader(bytes.NewReader(compressed))
	defer func() { _ = reader.Close() }()
	plain, err := io.ReadAll(reader)
	if err != nil {
		return nil, fmt.Errorf("inflate fast asset contexts: %w", err)
	}
	var event FastAssetCtxsEvent
	if err := json.Unmarshal(plain, &event); err != nil {
		return nil, fmt.Errorf("decode fast asset contexts JSON: %w", err)
	}
	return event, nil
}

func (c *Client) SubscribeFastAssetCtxs(ctx context.Context) (*FastAssetCtxsSubscription, error) {
	return subscribePublic(ctx, c, "fastAssetCtxs", "fastAssetCtxs", map[string]any{}, decodeFastAssetCtxs, func(FastAssetCtxsEvent) bool { return true }, func(s *streamSubscription[FastAssetCtxsEvent]) *FastAssetCtxsSubscription {
		return &FastAssetCtxsSubscription{s}
	})
}

func (c *Client) SubscribeSpotAssetCtxs(ctx context.Context) (*SpotAssetCtxsSubscription, error) {
	return subscribePublic(ctx, c, "spotAssetCtxs", "spotAssetCtxs", map[string]any{}, decodeJSON[SpotAssetCtxsEvent], func(SpotAssetCtxsEvent) bool { return true }, func(s *streamSubscription[SpotAssetCtxsEvent]) *SpotAssetCtxsSubscription {
		return &SpotAssetCtxsSubscription{s}
	})
}

func (c *Client) SubscribeUserHistoricalOrders(ctx context.Context, user string) (*UserHistoricalOrdersSubscription, error) {
	key := "userHistoricalOrders:" + strings.ToLower(user)
	return subscribeUser(ctx, c, "userHistoricalOrders", key, user, map[string]any{"user": user}, decodeJSON[UserHistoricalOrdersEvent], func(event UserHistoricalOrdersEvent) bool { return strings.EqualFold(event.User, user) }, false, func(s *streamSubscription[UserHistoricalOrdersEvent]) *UserHistoricalOrdersSubscription {
		return &UserHistoricalOrdersSubscription{s}
	})
}

func (c *Client) SubscribeOutcomeMetaUpdates(ctx context.Context) (*OutcomeMetaUpdatesSubscription, error) {
	return subscribePublic(ctx, c, "outcomeMetaUpdates", "outcomeMetaUpdates", map[string]any{}, decodeJSON[OutcomeMetaUpdatesEvent], func(OutcomeMetaUpdatesEvent) bool { return true }, func(s *streamSubscription[OutcomeMetaUpdatesEvent]) *OutcomeMetaUpdatesSubscription {
		return &OutcomeMetaUpdatesSubscription{s}
	})
}
