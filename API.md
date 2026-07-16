# Public API matrix

This document lists the public SDK surface implemented in this repository. It
is an API index, not a substitute for the [official Hyperliquid API](https://hyperliquid.gitbook.io/hyperliquid-docs/for-developers/api).
Protocol definitions and field semantics follow the official documentation and
the official [Python SDK](https://github.com/hyperliquid-dex/hyperliquid-python-sdk).

All Info methods accept `context.Context`; all Exchange methods require a
configured `signer.DigestSigner` when they submit an action. See the Go API
reference for request and response field definitions.

## Root configuration

| API | Purpose |
| --- | --- |
| `NewClient(options...)` | Builds the composed Info, Exchange, Explorer, and WebSocket clients. |
| `WithMainnet`, `WithTestnet`, `WithNetwork` | Select the protocol endpoint group. |
| `WithInfoBaseURL`, `WithExchangeBaseURL`, `WithExplorerBaseURL` | Replace individual HTTP endpoints for controlled testing or custom routing. |
| `WithWebSocketURL`, `WithExplorerWebSocketURL` | Replace market and Explorer subscription endpoints independently. |
| `WithDigestSigner` | Injects the only signer abstraction used by Exchange. |
| `WithVaultAddress`, `WithExpiresAfter` | Configures supported L1 action vault routing/expiry. |
| `WithNonceManager`, `WithAssetResolver` | Replaces stateful protocol dependencies. |
| `WithHTTPTransport`, `WithHTTPClient`, `WithRequestTransport` | Replaces HTTP or Info/Exchange request paths. |
| `WithExplorerRequestTransport` | Replaces only the read-only Explorer request path. |
| `WithHTTPTimeout`, `WithInfoTimeout`, `WithExchangeTimeout` | Sets client/request deadlines. |
| `WithInfoRetryPolicy`, `WithMiddleware` | Configures unsigned Info retries and transport middleware. |
| `WithWebSocketConfig` | Configures reconnects, queues, ping/pong, and Dialer. |
| `WithUserAgent` | Replaces the SDK's outgoing User-Agent header. |
| `Client.Close` | Closes SDK-owned WebSocket/Explorer connection resources. |

## Info

For Go signatures, parameter constraints, protocol request types, response
models, pagination limits, retry behavior, and failure semantics for every
implemented Info method, see the [Info API reference](docs/api/info.md).

### Market and metadata

`AllMids`, `AllMidsForDEX`, `L2Book`, `L2BookWithOptions`, `CandleSnapshot`,
`RecentTrades`, `FundingHistory`, `PredictedFundings`, `Meta`,
`MetaForDEX`, `AllPerpMetas`, `MetaAndAssetContexts`,
`MetaAndAssetContextsForDEX`, `SpotMeta`, `SpotMetaAndAssetContexts`,
`PerpDEXs`, `PerpDEXStatus`, `PerpDEXLimits`, `PerpsAtOpenInterestCap`,
`PerpAnnotation`, `PerpConciseAnnotations`, `PerpCategories`, `MarginTable`,
`MaxMarketOrderNotionals`, `OutcomeMeta`, `SettledOutcome`, `TokenDetails`,
`PerpDeployAuctionStatus`, `SpotPairDeployAuctionStatus`, `ExchangeStatus`,
`GossipPriorityAuctionStatus`, `GossipRootIPs`, `Liquidatable`,
`ValidatorSummaries`, `ValidatorL1Votes`, and `LegalCheck`.

### Accounts, orders, history, and permissions

`ClearinghouseState`, `ClearinghouseStateForDEX`, `SpotClearinghouseState`,
`ActiveAssetData`, `OpenOrders`, `OpenOrdersForDEX`, `FrontendOpenOrders`,
`FrontendOpenOrdersForDEX`, `OrderStatus`, `OrderStatusByCloid`,
`UserFills`, `UserFillsByTime`, `HistoricalOrders`, `Portfolio`,
`UserFunding`, `UserFees`, `UserRateLimit`, `UserNonFundingLedgerUpdates`,
`UserTwapSliceFills`, `UserTwapSliceFillsByTime`, `TWAPHistory`,
`UserRole`, `UserAbstraction`, `UserDEXAbstraction`, `ExtraAgents`,
`UserToMultiSigSigners`, `ApprovedBuilders`, `MaxBuilderFee`, `IsVIP`,
`Referral`, `PreTransferCheck`, `Subaccounts`, `Subaccounts2`, and
`SpotDeployState`.

### Vault, staking, and borrow/lend reads

`VaultDetails`, `VaultSummaries`, `UserVaultEquities`, `LeadingVaults`,
`DelegatorSummary`, `Delegations`, `DelegatorHistory`, `DelegatorRewards`,
`BorrowLendUserState`, `BorrowLendReserveState`, `AllBorrowLendReserveStates`,
and `UserBorrowLendInterest`.

### Advanced request path

`Raw` submits a caller-specified Info request into a caller-specified response
value. Prefer named methods when available so field typing and compatibility
tests are retained.

## Exchange

Every action follows one submission path that constructs a final L1 or
user-signed digest, checks the signature's low-S canonical form and recovered
address, and submits exactly once. HTTP/transport ambiguity is intentionally
not retried.

### Orders and leverage

`PlaceOrder`, `PlaceOrders`, `ModifyOrder`, `BatchModify`, `CancelOrder`,
`CancelOrders`, `CancelByCloid`, `CancelByCloids`, `ScheduleCancel`,
`UpdateLeverage`, `UpdateIsolatedMargin`, `TopUpIsolatedOnlyMargin`,
`PlaceTWAP`, and `CancelTWAP`.

`OrderRequest` supports `LimitOrder` (`TIFGTC`, `TIFIOC`, `TIFALO`) and
`TriggerOrder` (`TPSLTakeProfit`, `TPSLStopLoss`), a client order ID, a
builder fee, reduce-only, and explicit `types.MarketRef` resolution.

### Agent, builder, account, and multisig actions

`ApproveAgent`, `ApproveBuilderFee`, `UserSetAbstraction`,
`UserDexAbstraction`, `AgentSetAbstraction`, `AgentEnableDexAbstraction`,
`AgentSendAsset`, `SetDisplayName`, `SetReferrer`, `ClaimRewards`,
`AuthorizeAQAV2Role`, `ConvertToMultiSigUser`, `SubmitMultiSigL1`, and
`SubmitMultiSigUserAction`.

### Transfers, vaults, subaccounts, and user-signed actions

`SendUSD`, `SendSpot`, `SendAsset`, `TransferUSDClass`, `SendToEVMWithData`,
`WithdrawFromBridge`, `TransferSubaccountUSD`, `TransferSubaccountSpot`,
`TransferVaultUSD`, `CreateSubAccount`, `ModifySubAccount`, `CreateVault`,
`ModifyVault`, and `DistributeVault`.

### Staking, deployment, and specialised actions

`CDeposit`, `CWithdraw`, `DepositStaking`, `WithdrawStaking`, `Delegate`,
`Undelegate`, `TokenDelegate`, `CSignerAction`, `CValidatorAction`,
`SubmitCValidatorAction`, `ValidatorL1Stream`, `ReserveRequestWeight`,
`Noop`, `UserOutcome`, `EVMUserModify`, `UseBigEVMBlocks`,
`FinalizeEVMContract`, `SubmitPerpDeploy`, `SubmitSpotDeploy`,
`HIP3LiquidatorTransfer`, `GossipPriorityBid`, and `SubmitGossipPriorityBid`.

### Typed action responses

`ActionResponse.Response.Data` is a sealed union selected by response type:
`DefaultActionResponseData`, `OrderResponseData`, `CancelResponseData`,
`TWAPOrderResponseData`, `TWAPCancelResponseData`, and
`CreateVaultResponseData`. Future unknown values are preserved as
`UnknownActionResponseData`; protocol rejection is `ActionResponseError`.

## Signing and asset resolution

| Package/API | Purpose |
| --- | --- |
| `signer.DigestSigner` | External signing seam: `Address` + final `SignDigest`. |
| `signer.LocalPrivateKeySigner` | Optional local development/Testnet signer. |
| `signer.Verify` | Low-S and recovered-address verification. |
| `signing.ComputeL1ActionDigest` | L1 action hash generation. |
| `signing.ComputeUserActionDigest` | User-signed EIP-712 digest generation. |
| `asset.MetaResolver` | TTL/coalesced official perp, spot, HIP-3 metadata resolver. |
| `asset.StaticResolver`, `asset.CachedResolver` | Deterministic/custom resolver implementations. |
| `asset.Resolver`, `MarketResolver`, `IDResolver` | Symbol, strict market namespace, and reverse-ID seams. |
| `types.MarketRef` | Explicit perp/spot/HIP-3 market identity. |

## Explorer

`Client.Explorer` is a separate read-only RPC client. It provides
`BlockDetails`, `TxDetails`, `UserDetails`, `ExplorerBlock`, `ExplorerTxs`,
`SetRequestTransport`, and `Close`. Explorer HTTP and Explorer WebSocket use
their dedicated endpoints; the official API WebSocket post protocol does not
accept Explorer request kinds.

## WebSocket

The `websocket.Client` shares a managed subscription connection and restores
subscriptions after reconnect. All subscription types expose `Events`,
`Errors`, `Close`, and `States` (the common `StatefulSubscription` interface).

### Public market streams

`SubscribeL2Book`, `SubscribeAllMids`, `SubscribeTrades`, `SubscribeCandle`,
`SubscribeBBO`, `SubscribeActiveAssetCtx`, `SubscribeActiveSpotAssetCtx`,
`SubscribeAssetCtxs`, `SubscribeFastAssetCtxs`, `SubscribeSpotAssetCtxs`, and
`SubscribeOutcomeMetaUpdates`.

### User and account streams

`SubscribeUserEvents`, `SubscribeOrderUpdates`, `SubscribeUserFills`,
`SubscribeUserFundings`, `SubscribeUserNonFundingLedgerUpdates`,
`SubscribeNotification`, `SubscribeWebData3`, `SubscribeOpenOrders`,
`SubscribeClearinghouseState`, `SubscribeActiveAssetData`,
`SubscribeTWAPStates`, `SubscribeUserTWAPSliceFills`,
`SubscribeUserTWAPHistory`, `SubscribeSpotState`,
`SubscribeAllDEXsClearinghouseState`, `SubscribeAllDEXsAssetCtxs`, and
`SubscribeUserHistoricalOrders`.

### Explorer and request transport

`SubscribeExplorerBlock`, `SubscribeExplorerTxs`, `PostInfo`, and `PostAction`.
`PostInfo` and `PostAction` are the request-transport building blocks used by
`WithRequestTransport`; Explorer RPC uses its distinct endpoint/transport.

## Test and compatibility entry points

| Command/path | Purpose |
| --- | --- |
| `go test ./...` | Unit, protocol fixture, signing vector, and resilience tests. |
| `go test -race ./...` | Race detector suite. |
| `go test -tags='integration testnet' ./tests/integration` | Explicitly gated Testnet integration suite. |
| `go test ./signing ./signer` | Fixed L1/EIP-712/R-S-V/recovery vectors. |
| `go run ./scripts/upstreamcheck -lock upstream.lock.json` | Offline validation of reviewed upstream lock. |
| `go run ./scripts/upstreamcheck -lock upstream.lock.json -network` | Read-only official docs/Python SDK drift check. |
| `.github/workflows/upstream-drift.yml` | Scheduled upstream drift report. |

See [README.md](README.md) for installation, safe examples, Testnet opt-in
rules, production controls, and the compatibility-update procedure.
