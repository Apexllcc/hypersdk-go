# WebSocket API reference

`websocket.Client` owns one shared managed subscription connection. Create it
through the root client in normal applications. Each subscribe method writes
the official `subscribe` message for its channel and returns a typed handle;
it does not sign or trade. The authoritative channel schemas are in the
[official WebSocket documentation](https://hyperliquid.gitbook.io/hyperliquid-docs/for-developers/api/websocket).

## Common subscription semantics

Every method takes `context.Context`; invalid required strings and a canceled
context fail before/while subscribing. A returned subscription exposes:

```go
Events() <-chan T
Errors() <-chan error
States() <-chan websocket.SubscriptionStateEvent
Close() error
```

`Events` contains the documented typed event shown below. Decimal-valued wire
fields are retained as strings or `decimal.Decimal` in their corresponding Go
models; economic data is never decoded through `float64`. `Errors` reports
decode/connection failures. `States` emits `connecting`, `connected`,
`subscribed`, `reconnecting`, `error`, and `unsubscribed`. Sequence gaps mean
a slow state observer caused older non-terminal state events to be coalesced.
`subscribed` is published only after the matching server
`subscriptionResponse`; a rejection or missing acknowledgement is reported as
an error and causes reconnection. Every reconnect resends each live normalized
server wire identity once and requires a fresh acknowledgement before any of
its logical handles returns to `subscribed`.

The server may normalize acknowledgement fields by adding omitted defaults or
normalizing user-address case. Matching is protocol-aware rather than raw JSON
equality. Logically distinct handles that the server normalizes to one identity
share one wire subscription; closing one handle keeps the wire subscription
active until its last logical reference closes. This includes all logical
`spotState` `IsPortfolioMargin` variants: the server acknowledges their real
identity as `ignorePortfolioMargin:false`, so nil, false, and true handles
retain separate Go lifecycles while sharing one canonical wire/refcount. A
permanent server rejection terminates and removes only the matching logical
group so it is not replayed on reconnect.

The connection uses ping/pong, configurable exponential reconnect backoff and
jitter, and replays live logical subscriptions after a reconnect. Event queues
use the configured bounded backpressure policy (`Block`, `DropNewest`, or
`DropOldest`). A caller must continuously drain all desired channels.
`Close` on a subscription unregisters it; `Client.Close` closes all current
subscriptions, interrupts reconnect/dial work, waits for manager shutdown, and
is safe for concurrent and repeated calls. A custom `Dialer` must honor context
cancellation. Reconnection restores subscriptions, but consumers should still
reconcile state from Info after disconnects because events can be missed while
offline. `userEvents`, `orderUpdates`, and `notification` payloads omit the
user address, so each of those channels can represent only one user per
`websocket.Client`.

The official limits are per IP: 1,000 active server subscription identities,
10 unique subscription users, 2,000 outgoing messages per rolling minute, and
100 simultaneous WebSocket POST requests. One Client shares its message budget
between subscribe, unsubscribe, heartbeat, and POST. The fields
`MaxActiveSubscriptions`, `MaxUniqueUsers`,
`MaxOutgoingMessagesPerMinute`, and `MaxConcurrentPosts` can lower or raise
the private gates constructed when no gate is injected. To enforce one boundary
across multiple Clients, construct one `SubscriptionAdmissionGate` with
`NewSubscriptionAdmissionGate`, one `MessageAdmissionLimiter` with
`NewMessageAdmissionLimiter`, and one `PostAdmissionGate` with
`NewPostAdmissionGate`; inject those same concurrency-safe instances through
`Config.SubscriptionAdmission`, `Config.MessageAdmission`, and
`Config.PostAdmission` on every Client sharing the IP. An injected gate owns
its capacities, so the corresponding `Max*` constructor fields do not resize
it.

`SubscriptionAdmissionGate` acquisition is atomic and non-blocking. Each
Client is an opaque owner: server-equivalent logical handles within that Client
share one lease, while the same identity on another Client consumes another
slot because it creates another server connection subscription. Normalized
user references are shared across all owners and released only after their last
leased identity closes. The Client owns each returned lease and releases it
after the last logical reference closes, a permanent rejection removes the
identity, or the Client closes; callers own the injected gate and may reuse it
for the lifetime of all participating Clients. Custom gate implementations must
be concurrency-safe and return idempotent, non-blocking release functions.

POST calls beyond the concurrent bound wait and honor their context;
outgoing-message waits also honor subscription, connection-generation, caller,
and Client cancellation. Subscription writes use a generation-bound outbound
scheduler, so a waiting write does not stall inbound acknowledgements, events,
or pongs. The default `SubscriptionAckTimeout` is 10 seconds.

## Market streams

`AllMidsRequest` selects the default or a named DEX. `L2BookRequest` uses
non-empty `Coin`, optional aggregation settings, and optional `Fast *bool`.
`Fast` is sent as the official `fast` field when non-nil, and omitted, false,
and true are distinct subscription identities. `TradesRequest`,
`CandleRequest`, `BBORequest`, and `ActiveAssetCtxRequest` also validate their
market/interval parameters before sending their respective channel request.

<!-- api: websocket.Client.SubscribeL2Book -->
```go
func (c *websocket.Client) SubscribeL2Book(ctx context.Context, request websocket.L2BookRequest) (*websocket.L2BookSubscription, error)
```
Channel: `l2Book`; event: `websocket.L2BookEvent` (book snapshot/update).

<!-- api: websocket.Client.SubscribeAllMids -->
```go
func (c *websocket.Client) SubscribeAllMids(ctx context.Context, request websocket.AllMidsRequest) (*websocket.AllMidsSubscription, error)
```
Channel: `allMids`; event: `websocket.AllMidsEvent`.
Only one `AllMidsRequest` (default or a chosen DEX) may be active per client;
a different request returns `websocket.ErrAmbiguousAllMids`.

<!-- api: websocket.Client.SubscribeTrades -->
```go
func (c *websocket.Client) SubscribeTrades(ctx context.Context, request websocket.TradesRequest) (*websocket.TradesSubscription, error)
```
Channel: `trades`; event: `websocket.TradesEvent`.

<!-- api: websocket.Client.SubscribeCandle -->
```go
func (c *websocket.Client) SubscribeCandle(ctx context.Context, request websocket.CandleRequest) (*websocket.CandleSubscription, error)
```
Channel: `candle`; event: `websocket.CandleEvent`.

<!-- api: websocket.Client.SubscribeBBO -->
```go
func (c *websocket.Client) SubscribeBBO(ctx context.Context, request websocket.BBORequest) (*websocket.BBOSubscription, error)
```
Channel: `bbo`; event: `websocket.BBOEvent`.

<!-- api: websocket.Client.SubscribeActiveAssetCtx -->
```go
func (c *websocket.Client) SubscribeActiveAssetCtx(ctx context.Context, request websocket.ActiveAssetCtxRequest) (*websocket.ActiveAssetCtxSubscription, error)
```
Channel: `activeAssetCtx`; event: `websocket.ActiveAssetCtxEvent`.

<!-- api: websocket.Client.SubscribeActiveSpotAssetCtx -->
```go
func (c *websocket.Client) SubscribeActiveSpotAssetCtx(ctx context.Context, coin string) (*websocket.ActiveSpotAssetCtxSubscription, error)
```
Channel: `activeAssetCtx`; this is the SDK's spot ergonomic alias for the
official shared perp/spot channel. `coin` must be non-empty; event:
`websocket.ActiveAssetCtxEvent`.

<!-- api: websocket.Client.SubscribeAssetCtxs -->
```go
func (c *websocket.Client) SubscribeAssetCtxs(ctx context.Context, request websocket.DEXRequest) (*websocket.AssetCtxsSubscription, error)
```
Channel: `assetCtxs`; event: `websocket.AssetCtxsEvent` for the selected DEX.

<!-- api: websocket.Client.SubscribeFastAssetCtxs -->
```go
func (c *websocket.Client) SubscribeFastAssetCtxs(ctx context.Context) (*websocket.FastAssetCtxsSubscription, error)
```
Channel: `fastAssetCtxs`; event: `websocket.FastAssetCtxsEvent`.

<!-- api: websocket.Client.SubscribeSpotAssetCtxs -->
```go
func (c *websocket.Client) SubscribeSpotAssetCtxs(ctx context.Context) (*websocket.SpotAssetCtxsSubscription, error)
```
Channel: `spotAssetCtxs`; event: `websocket.SpotAssetCtxsEvent`.

<!-- api: websocket.Client.SubscribeOutcomeMetaUpdates -->
```go
func (c *websocket.Client) SubscribeOutcomeMetaUpdates(ctx context.Context) (*websocket.OutcomeMetaUpdatesSubscription, error)
```
Channel: `outcomeMetaUpdates`; event: `websocket.OutcomeMetaUpdatesEvent`.

## User, order, and account streams

`user` is the actual account address, not the API-wallet address. It must be
non-empty. The protocol omits `user` in `userEvents` and `orderUpdates` event
payloads, so one client cannot multiplex distinct users on either channel.
`SubscribeUserFills` keys one stream per user; reusing it with a different
`AggregateByTime` setting returns `ErrConflictingUserFillsSubscription`.
`UserDEXRequest` carries a user and DEX namespace; `ActiveAssetDataRequest`
and `SpotStateRequest` use their corresponding explicit user/market fields.

<!-- api: websocket.Client.SubscribeUserEvents -->
```go
func (c *websocket.Client) SubscribeUserEvents(ctx context.Context, user string) (*websocket.UserEventsSubscription, error)
```
Channel: `userEvents`; event: `websocket.UserEvent` (fills, funding, liquidations, cancellations).

<!-- api: websocket.Client.SubscribeOrderUpdates -->
```go
func (c *websocket.Client) SubscribeOrderUpdates(ctx context.Context, user string) (*websocket.OrderUpdatesSubscription, error)
```
Channel: `orderUpdates`; event: `[]websocket.OrderUpdate`.

<!-- api: websocket.Client.SubscribeUserFills -->
```go
func (c *websocket.Client) SubscribeUserFills(ctx context.Context, request websocket.UserFillsRequest) (*websocket.UserFillsSubscription, error)
```
Channel: `userFills`; event: `websocket.UserFillsEvent`; `AggregateByTime` merges partial fills from one crossing order.

<!-- api: websocket.Client.SubscribeUserFundings -->
```go
func (c *websocket.Client) SubscribeUserFundings(ctx context.Context, user string) (*websocket.UserFundingsSubscription, error)
```
Channel: `userFundings`; event: `websocket.UserFundingsEvent`.

<!-- api: websocket.Client.SubscribeUserNonFundingLedgerUpdates -->
```go
func (c *websocket.Client) SubscribeUserNonFundingLedgerUpdates(ctx context.Context, user string) (*websocket.UserLedgerSubscription, error)
```
Channel: `userNonFundingLedgerUpdates`; event: `websocket.UserLedgerEvent`.

<!-- api: websocket.Client.SubscribeNotification -->
```go
func (c *websocket.Client) SubscribeNotification(ctx context.Context, user string) (*websocket.NotificationSubscription, error)
```
Channel: `notification`; event: `websocket.NotificationEvent`.

<!-- api: websocket.Client.SubscribeWebData3 -->
```go
func (c *websocket.Client) SubscribeWebData3(ctx context.Context, user string) (*websocket.WebData3Subscription, error)
```
Channel: `webData3`; event: `websocket.WebData3Event`.

<!-- api: websocket.Client.SubscribeOpenOrders -->
```go
func (c *websocket.Client) SubscribeOpenOrders(ctx context.Context, request websocket.UserDEXRequest) (*websocket.OpenOrdersStreamSubscription, error)
```
Channel: `openOrders`; event: `websocket.OpenOrdersStreamEvent`.

<!-- api: websocket.Client.SubscribeClearinghouseState -->
```go
func (c *websocket.Client) SubscribeClearinghouseState(ctx context.Context, request websocket.UserDEXRequest) (*websocket.ClearinghouseStateSubscription, error)
```
Channel: `clearinghouseState`; event: `websocket.ClearinghouseStateEvent`.

<!-- api: websocket.Client.SubscribeActiveAssetData -->
```go
func (c *websocket.Client) SubscribeActiveAssetData(ctx context.Context, request websocket.ActiveAssetDataRequest) (*websocket.ActiveAssetDataSubscription, error)
```
Channel: `activeAssetData`; event: `types.ActiveAssetDataResponse`.

<!-- api: websocket.Client.SubscribeTWAPStates -->
```go
func (c *websocket.Client) SubscribeTWAPStates(ctx context.Context, request websocket.UserDEXRequest) (*websocket.TWAPStatesSubscription, error)
```
Channel: `twapStates`; event: `websocket.TWAPStatesEvent`.

<!-- api: websocket.Client.SubscribeUserTWAPSliceFills -->
```go
func (c *websocket.Client) SubscribeUserTWAPSliceFills(ctx context.Context, user string) (*websocket.UserTWAPSliceFillsSubscription, error)
```
Channel: `userTwapSliceFills`; event: `websocket.UserTWAPSliceFillsEvent`.

<!-- api: websocket.Client.SubscribeUserTWAPHistory -->
```go
func (c *websocket.Client) SubscribeUserTWAPHistory(ctx context.Context, user string) (*websocket.UserTWAPHistorySubscription, error)
```
Channel: `userTwapHistory`; event: `websocket.UserTWAPHistoryEvent`.

<!-- api: websocket.Client.SubscribeSpotState -->
```go
func (c *websocket.Client) SubscribeSpotState(ctx context.Context, request websocket.SpotStateRequest) (*websocket.SpotStateSubscription, error)
```
Channel: `spotState`; event: `websocket.SpotStateEvent`.

<!-- api: websocket.Client.SubscribeAllDEXsClearinghouseState -->
```go
func (c *websocket.Client) SubscribeAllDEXsClearinghouseState(ctx context.Context, user string) (*websocket.AllDEXsClearinghouseStateSubscription, error)
```
Channel: `allDexsClearinghouseState`; event: `websocket.AllDEXsClearinghouseStateEvent`.

<!-- api: websocket.Client.SubscribeAllDEXsAssetCtxs -->
```go
func (c *websocket.Client) SubscribeAllDEXsAssetCtxs(ctx context.Context) (*websocket.AllDEXsAssetCtxsSubscription, error)
```
Channel: `allDexsAssetCtxs`; event: `websocket.AllDEXsAssetCtxsEvent`.

<!-- api: websocket.Client.SubscribeUserHistoricalOrders -->
```go
func (c *websocket.Client) SubscribeUserHistoricalOrders(ctx context.Context, user string) (*websocket.UserHistoricalOrdersSubscription, error)
```
Channel: `userHistoricalOrders`; event: `websocket.UserHistoricalOrdersEvent`.

## Request transport and Explorer-compatible streams

`Request`, `PostInfo`, and `PostAction` implement `transport.RequestTransport`
over a dedicated request WebSocket connection. They are low-level building
blocks for injected Info/Exchange request paths, not subscriptions. `payload`
and `response` are caller-owned typed protocol values; the request has the
given context deadline. Each call performs one WebSocket post attempt. Info
retry policy belongs to `info.Client` when it invokes an injected request
transport; `PostInfo` itself does not retry. `PostAction` and Exchange actions
never retry after an ambiguous network failure.

<!-- api: websocket.Client.Request -->
```go
func (c *websocket.Client) Request(ctx context.Context, kind transport.RequestKind, payload any, response any) error
```

| Item | Detail |
| --- | --- |
| Parameters | `ctx` controls dialing, serialized writes, and the response wait. `kind` must be `transport.RequestInfo` or `transport.RequestAction`; `payload` is the request body and `response` is the optional caller-owned decode target. A nil `response` deliberately ignores a successful payload. |
| Protocol | Sends one official `{"method":"post","id":...,"request":{"type":kind,"payload":...}}` envelope on the shared request connection. Info responses unwrap their inner `{type,data}` payload before decoding; Action responses decode their HTTP-equivalent payload directly. |
| Success | Returns `nil` only after a matching `post` response has been received and, when non-nil, decoded into `response`. One reusable request connection is shared across concurrent callers but writes are serialized. At most 100 calls are in flight by default; additional calls wait for admission and honor `ctx`. |
| Failure | Returns the context error, `ErrUnsupportedPostRequest` for any kind other than Info/Action, `ErrWebSocketClosed`, dial/write/read errors, `*PostError` for server post errors, or `ErrUnexpectedPostResponse` for a mismatched or malformed response/decode failure. A request is never replayed after disconnection. |

<!-- api: websocket.Client.PostInfo -->
```go
func (c *websocket.Client) PostInfo(ctx context.Context, payload any, response any) error
```

| Item | Detail |
| --- | --- |
| Parameters | Equivalent to `Request(ctx, transport.RequestInfo, payload, response)`. `payload` and `response` follow the same ownership and decoding rules as `Request`. |
| Protocol | Sends one WebSocket post request with type `info` and unwraps the Info `{type,data}` response envelope. |
| Success | Returns `nil` after the Info `data` payload is decoded into a non-nil `response`, or after the payload is intentionally ignored for a nil `response`. |
| Failure | Uses the same context, connection, server-error, and decode failures as `Request`. It does not retry itself; an `info.Client` that uses this method through `transport.RequestTransport` applies its configured retry policy to retryable 429/502/503/504 `*PostError` responses. |

<!-- api: websocket.Client.PostAction -->
```go
func (c *websocket.Client) PostAction(ctx context.Context, payload any, response any) error
```

| Item | Detail |
| --- | --- |
| Parameters | Equivalent to `Request(ctx, transport.RequestAction, payload, response)`. The caller supplies the already-signed Action request payload and optional decode target. |
| Protocol | Sends one WebSocket post request with type `action`; its successful payload is decoded as the HTTP-equivalent Exchange action response without an Info inner envelope. |
| Success | Returns `nil` after a matching Action response is decoded, or after a successful payload is intentionally ignored for a nil `response`. |
| Failure | Uses the same context, connection, server-error, and decode failures as `Request`. It never automatically retries, and neither does `exchange.Client`, because a timeout or disconnect cannot prove that a signed action was not executed. |

The following require a client pointed at the **Explorer RPC WebSocket URL**,
not the trading API WebSocket. Explorer compatibility is documented separately
in [Explorer API](explorer.md); it is read-only and does not use Info/Action
post messages.

<!-- api: websocket.Client.SubscribeExplorerBlock -->
```go
func (c *websocket.Client) SubscribeExplorerBlock(ctx context.Context) (*websocket.ExplorerBlockSubscription, error)
```
Channel: `explorerBlock`; event: `[]websocket.ExplorerBlock`.

<!-- api: websocket.Client.SubscribeExplorerTxs -->
```go
func (c *websocket.Client) SubscribeExplorerTxs(ctx context.Context) (*websocket.ExplorerTxsSubscription, error)
```
Channel: `explorerTxs`; event: `[]websocket.ExplorerTransaction`.

<!-- api: websocket.Client.Close -->
```go
func (c *websocket.Client) Close() error
```
Idempotently closes subscription and request connections. It is not a protocol
unsubscribe acknowledgement and must not be used concurrently with subsequent
new subscriptions on the same client.
