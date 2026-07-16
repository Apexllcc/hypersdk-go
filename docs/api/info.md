# Info API reference

This is the method-level reference for the unsigned `info.Client`. It is a
typed wrapper around `POST /info`; it is not a replacement for the
[official Info endpoint](https://hyperliquid.gitbook.io/hyperliquid-docs/for-developers/api/info-endpoint).
Only exported methods implemented by this SDK are listed here. Protocol field
definitions remain authoritative in the official documentation.

## Common contract

Every endpoint method takes `context.Context` as its first argument and sends
JSON with `Content-Type: application/json` to the configured Info endpoint.
`ctx` cancellation and the client's configured Info timeout stop the request.
Addresses are `string` values because the protocol accepts a 42-character
hexadecimal address; callers must use the **actual master or subaccount
address**, not an agent-wallet address. `coin` is a Hyperliquid market name:
perps use the `meta` universe name, spot uses `PURR/USDC` or `@<spot-index>`,
and a HIP-3 market is namespaced where the protocol requires it (for example
`dex:COIN`).

All prices, sizes, balances, rates, PnL, and notionals exposed by response
types use `decimal.Decimal`; do not convert them through `float64`. Timestamps
and time ranges are Unix milliseconds. A time-range response is limited by the
upstream endpoint (the general Info pagination limit is 500 elements or
distinct blocks); advance `startTime` using the final returned timestamp when
the official endpoint supports pagination. `UserFillsByTime` has a documented
2,000-item response cap and 10,000-fill history window; `CandleSnapshot`
retains at most 5,000 recent candles.

### Errors and retries

Unless an individual card says otherwise, its **failure** behavior is:

- Local validation returns an error before any request.
- A cancelled/deadline-exceeded context returns its context error.
- A non-2xx HTTP response returns `*hlerr.APIError`; an empty or incompatible
  response returns an error wrapping `hlerr.ErrUnexpectedResponse`.
- The Info client retries only HTTP/request-transport failures with status
  **429, 502, 503, or 504**, using its configured `transport.RetryPolicy` and
  honoring `Retry-After` for HTTP responses. Other status codes and decode
  errors are not retried. `WithInfoRetryPolicy` controls this behavior.

The method markers such as `<!-- api: info.Client.AllMids -->` are stable
coverage identifiers. A documentation check can compare these exact markers
with the exported `info.Client` method set.

## Market data and metadata

<!-- api: info.Client.AllMids -->
### AllMids

`func (c *Client) AllMids(ctx context.Context) (AllMidsResponse, error)`

| Item | Detail |
| --- | --- |
| Parameters | `ctx` is required; no endpoint parameters. |
| Protocol | `POST /info`, request `type: "allMids"`. |
| Success | `AllMidsResponse` (`map[string]decimal.Decimal`), keyed by market symbol; an empty book may use the latest trade as its mid. |
| Failure | Common Info failure model; no local validation. |

<!-- api: info.Client.AllMidsForDEX -->
### AllMidsForDEX

`func (c *Client) AllMidsForDEX(ctx context.Context, dex string) (AllMidsResponse, error)`

| Item | Detail |
| --- | --- |
| Parameters | `dex string` selects a perp DEX; empty selects the original DEX (and includes spot mids). |
| Protocol | `POST /info`, `type: "allMids"`, optional `dex`. |
| Success | `AllMidsResponse`, with decimal mids keyed by the selected DEX's market symbols. |
| Failure | Common Info failure model; the SDK forwards an empty or unknown DEX to the API. |

<!-- api: info.Client.Meta -->
### Meta

`func (c *Client) Meta(ctx context.Context) (MetaResponse, error)`

| Item | Detail |
| --- | --- |
| Parameters | `ctx` only; retrieves the original perp DEX. |
| Protocol | `POST /info`, `type: "meta"`. |
| Success | `MetaResponse`: `Universe` (`PerpAsset` name, size decimals, leverage and margin metadata), `MarginTables`, and `CollateralToken`. |
| Failure | Common Info failure model; no local validation. |

<!-- api: info.Client.MetaForDEX -->
### MetaForDEX

`func (c *Client) MetaForDEX(ctx context.Context, dex string) (MetaResponse, error)`

| Item | Detail |
| --- | --- |
| Parameters | `dex string`; empty selects the original DEX, a non-empty value selects a HIP-3 DEX. |
| Protocol | `POST /info`, `type: "meta"`, optional `dex`. |
| Success | `MetaResponse`; use `Universe` when resolving perp symbols and size precision. |
| Failure | Common Info failure model; DEX existence is validated by the API. |

<!-- api: info.Client.MetaAndAssetContexts -->
### MetaAndAssetContexts

`func (c *Client) MetaAndAssetContexts(ctx context.Context) (MetaAndAssetContextsResponse, error)`

| Item | Detail |
| --- | --- |
| Parameters | `ctx` only. |
| Protocol | `POST /info`, `type: "metaAndAssetCtxs"`. |
| Success | `MetaAndAssetContextsResponse`, decoded from the protocol tuple into `Meta` and `Contexts`; context prices/rates are `decimal.Decimal`. |
| Failure | Common Info failure model; an invalid two-element tuple is a decode error. |

<!-- api: info.Client.MetaAndAssetContextsForDEX -->
### MetaAndAssetContextsForDEX

`func (c *Client) MetaAndAssetContextsForDEX(ctx context.Context, dex string) (MetaAndAssetContextsResponse, error)`

| Item | Detail |
| --- | --- |
| Parameters | `dex string`; empty is the original DEX. |
| Protocol | `POST /info`, `type: "metaAndAssetCtxs"`, optional `dex`. |
| Success | `MetaAndAssetContextsResponse` for the selected DEX. |
| Failure | Common Info failure model; malformed tuple data is rejected during decoding. |

<!-- api: info.Client.AllPerpMetas -->
### AllPerpMetas

`func (c *Client) AllPerpMetas(ctx context.Context) ([]MetaResponse, error)`

| Item | Detail |
| --- | --- |
| Parameters | `ctx` only. |
| Protocol | `POST /info`, `type: "allPerpMetas"`. |
| Success | `[]MetaResponse`, one metadata object per supported perp DEX. |
| Failure | Common Info failure model; no local validation. |

<!-- api: info.Client.L2Book -->
### L2Book

`func (c *Client) L2Book(ctx context.Context, coin string) (L2BookResponse, error)`

| Item | Detail |
| --- | --- |
| Parameters | `coin string`, required market identifier. |
| Protocol | `POST /info`, `type: "l2Book"`, `coin`. |
| Success | `L2BookResponse`: market/time, optional decimal `Spread`, and bid/ask `Levels`; upstream returns at most 20 levels per side. |
| Failure | Empty `coin` is rejected locally; otherwise common Info failure model. |

<!-- api: info.Client.L2BookWithOptions -->
### L2BookWithOptions

`func (c *Client) L2BookWithOptions(ctx context.Context, request L2BookRequest) (L2BookResponse, error)`

| Item | Detail |
| --- | --- |
| Parameters | `request.Coin` is required. `NSigFigs` may be 2, 3, 4, 5, or nil (full precision); `Mantissa` is allowed only with `NSigFigs == 5` and must be 1, 2, or 5. `Type` is set by the SDK. |
| Protocol | `POST /info`, `type: "l2Book"`, `coin`, optional `nSigFigs`/`mantissa`. |
| Success | `L2BookResponse`, as for `L2Book`, with server-side level aggregation when requested. |
| Failure | Invalid coin/aggregation is rejected locally; otherwise common Info failure model. |

<!-- api: info.Client.CandleSnapshot -->
### CandleSnapshot

`func (c *Client) CandleSnapshot(ctx context.Context, request CandleRequest) ([]Candle, error)`

| Item | Detail |
| --- | --- |
| Parameters | `request.Coin` and `Interval` are required; `StartTime >= 0`; optional `EndTime >= StartTime`. Interval must be one of official values (`1m` through `1M`). |
| Protocol | `POST /info`, `type: "candleSnapshot"`, nested `req` with coin, interval, start/end milliseconds. |
| Success | `[]Candle`: open/high/low/close/volume are decimal; upstream exposes only the most recent 5,000 candles. |
| Failure | Invalid range or interval is rejected locally; otherwise common Info failure model. |

<!-- api: info.Client.RecentTrades -->
### RecentTrades

`func (c *Client) RecentTrades(ctx context.Context, coin string) ([]RecentTrade, error)`

| Item | Detail |
| --- | --- |
| Parameters | `coin string`, required market identifier. |
| Protocol | `POST /info`, `type: "recentTrades"`, `coin`. |
| Success | `[]RecentTrade` with coin, side, decimal price/size, millisecond time, hash, trade ID, and users. |
| Failure | Empty `coin` is rejected locally; otherwise common Info failure model. |

<!-- api: info.Client.FundingHistory -->
### FundingHistory

`func (c *Client) FundingHistory(ctx context.Context, coin string, startTime int64, endTime *int64) ([]FundingHistoryEntry, error)`

| Item | Detail |
| --- | --- |
| Parameters | `coin` required; `startTime >= 0`; optional `endTime` is protocol-defaulted when nil. |
| Protocol | `POST /info`, `type: "fundingHistory"`, coin and time range. |
| Success | `[]FundingHistoryEntry` with decimal funding rate/premium and time. Use the last timestamp to paginate where the endpoint truncates range data. |
| Failure | Empty coin or negative start time is rejected locally; otherwise common Info failure model. |

<!-- api: info.Client.PredictedFundings -->
### PredictedFundings

`func (c *Client) PredictedFundings(ctx context.Context) ([]PredictedFunding, error)`

| Item | Detail |
| --- | --- |
| Parameters | `ctx` only. |
| Protocol | `POST /info`, `type: "predictedFundings"`. |
| Success | `[]PredictedFunding`, decoded from protocol tuples into asset and venue predictions; `FundingRate` is decimal and venue data may be nil. |
| Failure | Common Info failure model; malformed tuples are decode errors. |

<!-- api: info.Client.PerpDEXs -->
### PerpDEXs

`func (c *Client) PerpDEXs(ctx context.Context) ([]*PerpDEX, error)`

| Item | Detail |
| --- | --- |
| Parameters | `ctx` only. |
| Protocol | `POST /info`, `type: "perpDexs"`. |
| Success | `[]*PerpDEX`, each with name, full name, deployer, and optional oracle updater/fee recipient. |
| Failure | Common Info failure model; no local validation. |

<!-- api: info.Client.PerpDEXLimits -->
### PerpDEXLimits

`func (c *Client) PerpDEXLimits(ctx context.Context, dex string) (*PerpDEXLimitsResponse, error)`

| Item | Detail |
| --- | --- |
| Parameters | `dex string`, required HIP-3 DEX name. |
| Protocol | `POST /info`, `type: "perpDexLimits"`, `dex`. |
| Success | Optional `*PerpDEXLimitsResponse`; server `null` is represented by nil. It includes total/per-market decimal OI caps and max transfer notional. |
| Failure | Empty DEX is rejected locally; otherwise common Info failure model. |

<!-- api: info.Client.PerpDEXStatus -->
### PerpDEXStatus

`func (c *Client) PerpDEXStatus(ctx context.Context, dex string) (PerpDEXStatusResponse, error)`

| Item | Detail |
| --- | --- |
| Parameters | `dex string`, required HIP-3 DEX name. |
| Protocol | `POST /info`, `type: "perpDexStatus"`, `dex`. |
| Success | `PerpDEXStatusResponse` with decimal `TotalNetDeposit`. |
| Failure | Empty DEX is rejected locally; otherwise common Info failure model. |

<!-- api: info.Client.PerpsAtOpenInterestCap -->
### PerpsAtOpenInterestCap

`func (c *Client) PerpsAtOpenInterestCap(ctx context.Context, dex string) ([]string, error)`

| Item | Detail |
| --- | --- |
| Parameters | `dex string`; empty selects the original DEX. |
| Protocol | `POST /info`, `type: "perpsAtOpenInterestCap"`, optional `dex`. |
| Success | `[]string` of symbols at their open-interest cap. |
| Failure | Common Info failure model; DEX validity is API-defined. |

<!-- api: info.Client.PerpAnnotation -->
### PerpAnnotation

`func (c *Client) PerpAnnotation(ctx context.Context, coin string) (*PerpAnnotationResponse, error)`

| Item | Detail |
| --- | --- |
| Parameters | `coin string`, required perp/HIP-3 market identifier. |
| Protocol | `POST /info`, `type: "perpAnnotation"`, `coin`. |
| Success | Optional `*PerpAnnotationResponse` with category, description, display name, and keywords. |
| Failure | Empty coin is rejected locally; otherwise common Info failure model. |

<!-- api: info.Client.PerpCategories -->
### PerpCategories

`func (c *Client) PerpCategories(ctx context.Context) ([]PerpCategory, error)`

| Item | Detail |
| --- | --- |
| Parameters | `ctx` only. |
| Protocol | `POST /info`, `type: "perpCategories"`. |
| Success | `[]PerpCategory`, tuple-decoded into coin and category. |
| Failure | Common Info failure model; malformed tuples are decode errors. |

<!-- api: info.Client.PerpConciseAnnotations -->
### PerpConciseAnnotations

`func (c *Client) PerpConciseAnnotations(ctx context.Context) ([]PerpConciseAnnotation, error)`

| Item | Detail |
| --- | --- |
| Parameters | `ctx` only. |
| Protocol | `POST /info`, `type: "perpConciseAnnotations"`. |
| Success | `[]PerpConciseAnnotation`, tuple-decoded into coin plus concise category/display/keyword metadata. |
| Failure | Common Info failure model; malformed tuples are decode errors. |

<!-- api: info.Client.MarginTable -->
### MarginTable

`func (c *Client) MarginTable(ctx context.Context, id int) (MarginTableResponse, error)`

| Item | Detail |
| --- | --- |
| Parameters | `id int` must be non-negative; use `MetaResponse.MarginTables` to discover IDs. |
| Protocol | `POST /info`, `type: "marginTable"`, `id`. |
| Success | `MarginTableResponse`, including tiered decimal bounds and leverage limits. |
| Failure | Negative ID is rejected locally; otherwise common Info failure model. |

<!-- api: info.Client.MaxMarketOrderNotionals -->
### MaxMarketOrderNotionals

`func (c *Client) MaxMarketOrderNotionals(ctx context.Context) ([]MaxMarketOrderNotional, error)`

| Item | Detail |
| --- | --- |
| Parameters | `ctx` only. |
| Protocol | `POST /info`, `type: "maxMarketOrderNtls"`. |
| Success | `[]MaxMarketOrderNotional`, market-level decimal market-order notional limits. |
| Failure | Common Info failure model; no local validation. |

<!-- api: info.Client.PerpDeployAuctionStatus -->
### PerpDeployAuctionStatus

`func (c *Client) PerpDeployAuctionStatus(ctx context.Context) (DeployAuctionStatus, error)`

| Item | Detail |
| --- | --- |
| Parameters | `ctx` only. |
| Protocol | `POST /info`, `type: "perpDeployAuctionStatus"`. |
| Success | `DeployAuctionStatus` with decimal gas values, duration, and start time. |
| Failure | Common Info failure model; no local validation. |

<!-- api: info.Client.SpotPairDeployAuctionStatus -->
### SpotPairDeployAuctionStatus

`func (c *Client) SpotPairDeployAuctionStatus(ctx context.Context) (DeployAuctionStatus, error)`

| Item | Detail |
| --- | --- |
| Parameters | `ctx` only. |
| Protocol | `POST /info`, `type: "spotPairDeployAuctionStatus"`. |
| Success | `DeployAuctionStatus` for spot-pair deployment. |
| Failure | Common Info failure model; no local validation. |

<!-- api: info.Client.SpotMeta -->
### SpotMeta

`func (c *Client) SpotMeta(ctx context.Context) (SpotMetaResponse, error)`

| Item | Detail |
| --- | --- |
| Parameters | `ctx` only. |
| Protocol | `POST /info`, `type: "spotMeta"`. |
| Success | `SpotMetaResponse`: token precision/IDs and tradable pair `Universe`; use pair index to form `@<index>` market names. |
| Failure | Common Info failure model; no local validation. |

<!-- api: info.Client.SpotMetaAndAssetContexts -->
### SpotMetaAndAssetContexts

`func (c *Client) SpotMetaAndAssetContexts(ctx context.Context) (SpotMetaAndAssetContextsResponse, error)`

| Item | Detail |
| --- | --- |
| Parameters | `ctx` only. |
| Protocol | `POST /info`, `type: "spotMetaAndAssetCtxs"`. |
| Success | `SpotMetaAndAssetContextsResponse`, tuple-decoded into spot metadata and decimal asset contexts. |
| Failure | Common Info failure model; invalid tuple shape is a decode error. |

<!-- api: info.Client.TokenDetails -->
### TokenDetails

`func (c *Client) TokenDetails(ctx context.Context, tokenID string) (TokenDetailsResponse, error)`

| Item | Detail |
| --- | --- |
| Parameters | `tokenID string`, required protocol token identifier. |
| Protocol | `POST /info`, `type: "tokenDetails"`, `tokenId`. |
| Success | `TokenDetailsResponse`: name/precision, decimal supply and price fields, and optional genesis/deployer/deploy metadata. |
| Failure | Empty token ID is rejected locally; otherwise common Info failure model. |

<!-- api: info.Client.OutcomeMeta -->
### OutcomeMeta

`func (c *Client) OutcomeMeta(ctx context.Context) (OutcomeMetaResponse, error)`

| Item | Detail |
| --- | --- |
| Parameters | `ctx` only. |
| Protocol | `POST /info`, `type: "outcomeMeta"`. |
| Success | `OutcomeMetaResponse` with outcome specifications and questions. |
| Failure | Common Info failure model; no local validation. |

<!-- api: info.Client.SettledOutcome -->
### SettledOutcome

`func (c *Client) SettledOutcome(ctx context.Context, outcome int) (*SettledOutcomeResponse, error)`

| Item | Detail |
| --- | --- |
| Parameters | `outcome int` must be non-negative. |
| Protocol | `POST /info`, `type: "settledOutcome"`, `outcome`. |
| Success | Optional `*SettledOutcomeResponse`, including settlement fraction/details and an optional validated active-or-settled question ID. |
| Failure | Negative outcome is rejected locally; common Info failure model otherwise. |

## Accounts, orders, and history

<!-- api: info.Client.ClearinghouseState -->
### ClearinghouseState

`func (c *Client) ClearinghouseState(ctx context.Context, user string) (ClearinghouseStateResponse, error)`

| Item | Detail |
| --- | --- |
| Parameters | `user string`, required actual account address. |
| Protocol | `POST /info`, `type: "clearinghouseState"`, `user`. |
| Success | `ClearinghouseStateResponse` with margin summary, cross/isolated positions, decimal account values and withdrawals. |
| Failure | Empty user is rejected locally; otherwise common Info failure model. |

<!-- api: info.Client.ClearinghouseStateForDEX -->
### ClearinghouseStateForDEX

`func (c *Client) ClearinghouseStateForDEX(ctx context.Context, user, dex string) (ClearinghouseStateResponse, error)`

| Item | Detail |
| --- | --- |
| Parameters | `user` required actual account; `dex` empty for original DEX or HIP-3 DEX name. |
| Protocol | `POST /info`, `type: "clearinghouseState"`, `user`, optional `dex`. |
| Success | `ClearinghouseStateResponse` for the selected DEX. |
| Failure | Empty user is rejected locally; otherwise common Info failure model. |

<!-- api: info.Client.SpotClearinghouseState -->
### SpotClearinghouseState

`func (c *Client) SpotClearinghouseState(ctx context.Context, user string) (SpotClearinghouseStateResponse, error)`

| Item | Detail |
| --- | --- |
| Parameters | `user string`, required actual account address. |
| Protocol | `POST /info`, `type: "spotClearinghouseState"`, `user`. |
| Success | `SpotClearinghouseStateResponse` with token totals/holds/entry notionals and decimal balances. |
| Failure | Empty user is rejected locally; otherwise common Info failure model. |

<!-- api: info.Client.ActiveAssetData -->
### ActiveAssetData

`func (c *Client) ActiveAssetData(ctx context.Context, user, coin string) (ActiveAssetDataResponse, error)`

| Item | Detail |
| --- | --- |
| Parameters | `user` is required actual account address; `coin` is required perp/spot market identifier. |
| Protocol | `POST /info`, `type: "activeAssetData"`, `user`, `coin`. |
| Success | `ActiveAssetDataResponse`, shared with the WebSocket channel; it contains account/position and leverage context with decimal economic fields. |
| Failure | Empty user or coin is rejected locally; otherwise common Info failure model. |

<!-- api: info.Client.OpenOrders -->
### OpenOrders

`func (c *Client) OpenOrders(ctx context.Context, user string) ([]OpenOrder, error)`

| Item | Detail |
| --- | --- |
| Parameters | `user string`, required actual account address. |
| Protocol | `POST /info`, `type: "openOrders"`, `user`. |
| Success | `[]OpenOrder`; original DEX responses include spot orders. |
| Failure | Empty user is rejected locally; otherwise common Info failure model. |

<!-- api: info.Client.OpenOrdersForDEX -->
### OpenOrdersForDEX

`func (c *Client) OpenOrdersForDEX(ctx context.Context, user, dex string) ([]OpenOrder, error)`

| Item | Detail |
| --- | --- |
| Parameters | `user` required; `dex` empty for original DEX or a HIP-3 DEX name. |
| Protocol | `POST /info`, `type: "openOrders"`, `user`, optional `dex`. |
| Success | `[]OpenOrder` for that perp DEX. |
| Failure | Empty user is rejected locally; otherwise common Info failure model. |

<!-- api: info.Client.FrontendOpenOrders -->
### FrontendOpenOrders

`func (c *Client) FrontendOpenOrders(ctx context.Context, user string) ([]FrontendOpenOrder, error)`

| Item | Detail |
| --- | --- |
| Parameters | `user string`, required actual account address. |
| Protocol | `POST /info`, `type: "frontendOpenOrders"`, `user`. |
| Success | `[]FrontendOpenOrder`, adding UI-oriented order fields to the open-order view. |
| Failure | Empty user is rejected locally; otherwise common Info failure model. |

<!-- api: info.Client.FrontendOpenOrdersForDEX -->
### FrontendOpenOrdersForDEX

`func (c *Client) FrontendOpenOrdersForDEX(ctx context.Context, user, dex string) ([]FrontendOpenOrder, error)`

| Item | Detail |
| --- | --- |
| Parameters | `user` required; `dex` is optional original/HIP-3 selector. |
| Protocol | `POST /info`, `type: "frontendOpenOrders"`, `user`, optional `dex`. |
| Success | `[]FrontendOpenOrder` for the selected DEX. |
| Failure | Empty user is rejected locally; otherwise common Info failure model. |

<!-- api: info.Client.OrderStatus -->
### OrderStatus

`func (c *Client) OrderStatus(ctx context.Context, user string, oid uint64) (OrderStatusResponse, error)`

| Item | Detail |
| --- | --- |
| Parameters | `user` required actual account; `oid` must be non-zero exchange order ID. |
| Protocol | `POST /info`, `type: "orderStatus"`, `user`, numeric `oid`. |
| Success | `OrderStatusResponse`: lifecycle `Status`, optional `Order`, and status timestamp. The response supports official open/filled/cancelled/rejected states. |
| Failure | Empty user or zero OID is rejected locally; a missing order is an API response rather than a local error. |

<!-- api: info.Client.OrderStatusByCloid -->
### OrderStatusByCloid

`func (c *Client) OrderStatusByCloid(ctx context.Context, user string, cloid types.Cloid) (OrderStatusResponse, error)`

| Item | Detail |
| --- | --- |
| Parameters | `user` required actual account; `cloid types.Cloid` is encoded as the protocol's 16-byte hex client order ID. |
| Protocol | `POST /info`, `type: "orderStatus"`, `user`, string `oid`. |
| Success | `OrderStatusResponse`, as for numeric OID lookup. |
| Failure | Empty user is rejected locally; otherwise common Info failure model. |

<!-- api: info.Client.UserFills -->
### UserFills

`func (c *Client) UserFills(ctx context.Context, user string, aggregateByTime bool) ([]UserFill, error)`

| Item | Detail |
| --- | --- |
| Parameters | `user` required actual account; `aggregateByTime` controls upstream partial-fill aggregation. |
| Protocol | `POST /info`, `type: "userFills"`, `user`, `aggregateByTime`. |
| Success | `[]UserFill`, most recent fills; upstream caps this response at 2,000 entries. |
| Failure | Empty user is rejected locally; otherwise common Info failure model. |

<!-- api: info.Client.UserFillsByTime -->
### UserFillsByTime

`func (c *Client) UserFillsByTime(ctx context.Context, user string, startTime int64, endTime *int64, aggregateByTime bool) ([]UserFill, error)`

| Item | Detail |
| --- | --- |
| Parameters | `user` required; `startTime >= 0`; optional `endTime` defaults upstream; `aggregateByTime` controls partial-fill aggregation. |
| Protocol | `POST /info`, `type: "userFillsByTime"`, user, inclusive start/end milliseconds, aggregation flag. |
| Success | `[]UserFill`; upstream returns at most 2,000 and retains only the 10,000 newest fills. |
| Failure | Empty user or negative start time is rejected locally; otherwise common Info failure model. |

<!-- api: info.Client.HistoricalOrders -->
### HistoricalOrders

`func (c *Client) HistoricalOrders(ctx context.Context, user string) ([]HistoricalOrder, error)`

| Item | Detail |
| --- | --- |
| Parameters | `user string`, required actual account address. |
| Protocol | `POST /info`, `type: "historicalOrders"`, `user`. |
| Success | `[]HistoricalOrder`, up to the official 2,000 most recent orders. |
| Failure | Empty user is rejected locally; otherwise common Info failure model. |

<!-- api: info.Client.Portfolio -->
### Portfolio

`func (c *Client) Portfolio(ctx context.Context, user string) ([]PortfolioPeriod, error)`

| Item | Detail |
| --- | --- |
| Parameters | `user string`, required actual account address. |
| Protocol | `POST /info`, `type: "portfolio"`, `user`. |
| Success | `[]PortfolioPeriod`, account-value/PnL history with decimal values. |
| Failure | Empty user is rejected locally; otherwise common Info failure model. |

<!-- api: info.Client.UserFunding -->
### UserFunding

`func (c *Client) UserFunding(ctx context.Context, user string, startTime, endTime *int64) ([]UserFundingEntry, error)`

| Item | Detail |
| --- | --- |
| Parameters | `user` required; nil start/end use API defaults; non-nil times must be non-negative and ordered. |
| Protocol | `POST /info`, `type: "userFunding"`, user and optional range milliseconds. |
| Success | `[]UserFundingEntry`, funding-payment history with decimal deltas. Paginate a truncated range by timestamp. |
| Failure | Empty user, negative times, or reversed range is rejected locally; otherwise common Info failure model. |

<!-- api: info.Client.UserNonFundingLedgerUpdates -->
### UserNonFundingLedgerUpdates

`func (c *Client) UserNonFundingLedgerUpdates(ctx context.Context, user string, startTime int64, endTime *int64) ([]NonFundingLedgerUpdate, error)`

| Item | Detail |
| --- | --- |
| Parameters | `user` required; `startTime >= 0`; optional `endTime >= startTime`. |
| Protocol | `POST /info`, `type: "userNonFundingLedgerUpdates"`, user and range milliseconds. |
| Success | `[]NonFundingLedgerUpdate`; `LedgerDelta` is forward-compatible and keeps economic fields decimal. Paginate by last timestamp if needed. |
| Failure | Invalid user/time range is rejected locally; otherwise common Info failure model. |

<!-- api: info.Client.UserTwapSliceFills -->
### UserTwapSliceFills

`func (c *Client) UserTwapSliceFills(ctx context.Context, user string) ([]TwapSliceFill, error)`

| Item | Detail |
| --- | --- |
| Parameters | `user string`, required actual account address. |
| Protocol | `POST /info`, `type: "userTwapSliceFills"`, `user`. |
| Success | `[]TwapSliceFill`, most recent TWAP slice fills (official response cap is 2,000). |
| Failure | Empty user is rejected locally; otherwise common Info failure model. |

<!-- api: info.Client.UserTwapSliceFillsByTime -->
### UserTwapSliceFillsByTime

`func (c *Client) UserTwapSliceFillsByTime(ctx context.Context, user string, startTime int64, endTime *int64) ([]TwapSliceFill, error)`

| Item | Detail |
| --- | --- |
| Parameters | `user` required; `startTime >= 0`; optional `endTime >= startTime`. |
| Protocol | `POST /info`, `type: "userTwapSliceFillsByTime"`, user and range milliseconds. |
| Success | `[]TwapSliceFill` in the requested window; paginate range responses by last timestamp as applicable. |
| Failure | Invalid user/time range is rejected locally; otherwise common Info failure model. |

<!-- api: info.Client.TWAPHistory -->
### TWAPHistory

`func (c *Client) TWAPHistory(ctx context.Context, user string) ([]TWAPHistoryEntry, error)`

| Item | Detail |
| --- | --- |
| Parameters | `user string`, required actual account address. |
| Protocol | `POST /info`, `type: "twapHistory"`, `user`. |
| Success | `[]TWAPHistoryEntry`, historical TWAP orders and status data. |
| Failure | Empty user is rejected locally; otherwise common Info failure model. |

## Account permissions, subaccounts, and transfers

<!-- api: info.Client.UserFees -->
### UserFees

`func (c *Client) UserFees(ctx context.Context, user string) (UserFeesResponse, error)`

| Item | Detail |
| --- | --- |
| Parameters | `user string`, required actual account address. |
| Protocol | `POST /info`, `type: "userFees"`, `user`. |
| Success | `UserFeesResponse`, including current fee schedule/rates as decimals. |
| Failure | Empty user is rejected locally; otherwise common Info failure model. |

<!-- api: info.Client.UserRateLimit -->
### UserRateLimit

`func (c *Client) UserRateLimit(ctx context.Context, user string) (UserRateLimitResponse, error)`

| Item | Detail |
| --- | --- |
| Parameters | `user string`, required actual account address. |
| Protocol | `POST /info`, `type: "userRateLimit"`, `user`. |
| Success | `UserRateLimitResponse`, current request-weight budget state. |
| Failure | Empty user is rejected locally; otherwise common Info failure model. |

<!-- api: info.Client.UserRole -->
### UserRole

`func (c *Client) UserRole(ctx context.Context, user string) (UserRoleResponse, error)`

| Item | Detail |
| --- | --- |
| Parameters | `user string`, required actual account address. |
| Protocol | `POST /info`, `type: "userRole"`, `user`. |
| Success | `UserRoleResponse`, role information such as user, agent, vault, subaccount, or missing. |
| Failure | Empty user is rejected locally; otherwise common Info failure model. |

<!-- api: info.Client.UserAbstraction -->
### UserAbstraction

`func (c *Client) UserAbstraction(ctx context.Context, user string) (UserAbstraction, error)`

| Item | Detail |
| --- | --- |
| Parameters | `user string`, required actual account address. |
| Protocol | `POST /info`, `type: "userAbstraction"`, `user`. |
| Success | `UserAbstraction`, one of the protocol values including `unifiedAccount`, `portfolioMargin`, `disabled`, `default`, or `dexAbstraction`. |
| Failure | Empty user is rejected locally; otherwise common Info failure model. |

<!-- api: info.Client.UserDEXAbstraction -->
### UserDEXAbstraction

`func (c *Client) UserDEXAbstraction(ctx context.Context, user string) (*bool, error)`

| Item | Detail |
| --- | --- |
| Parameters | `user string`, required actual account address. |
| Protocol | `POST /info`, `type: "userDexAbstraction"`, `user`. |
| Success | Optional `*bool`; nil preserves a protocol `null` distinct from false. |
| Failure | Empty user is rejected locally; otherwise common Info failure model. |

<!-- api: info.Client.ExtraAgents -->
### ExtraAgents

`func (c *Client) ExtraAgents(ctx context.Context, user string) ([]ExtraAgent, error)`

| Item | Detail |
| --- | --- |
| Parameters | `user string`, required actual account address. |
| Protocol | `POST /info`, `type: "extraAgents"`, `user`. |
| Success | `[]ExtraAgent`, extra agent-wallet records associated with the account. |
| Failure | Empty user is rejected locally; otherwise common Info failure model. |

<!-- api: info.Client.UserToMultiSigSigners -->
### UserToMultiSigSigners

`func (c *Client) UserToMultiSigSigners(ctx context.Context, user string) (*MultiSigSigners, error)`

| Item | Detail |
| --- | --- |
| Parameters | `user string`, required actual account address. |
| Protocol | `POST /info`, `type: "userToMultiSigSigners"`, `user`. |
| Success | Optional `*MultiSigSigners`, preserving an absent multisig configuration as nil. |
| Failure | Empty user is rejected locally; otherwise common Info failure model. |

<!-- api: info.Client.ApprovedBuilders -->
### ApprovedBuilders

`func (c *Client) ApprovedBuilders(ctx context.Context, user string) ([]string, error)`

| Item | Detail |
| --- | --- |
| Parameters | `user string`, required actual account address. |
| Protocol | `POST /info`, `type: "approvedBuilders"`, `user`. |
| Success | `[]string` of builder addresses approved by the user. |
| Failure | Empty user is rejected locally; otherwise common Info failure model. |

<!-- api: info.Client.MaxBuilderFee -->
### MaxBuilderFee

`func (c *Client) MaxBuilderFee(ctx context.Context, user, builder string) (int, error)`

| Item | Detail |
| --- | --- |
| Parameters | Both `user` and `builder` must be non-empty protocol addresses. |
| Protocol | `POST /info`, `type: "maxBuilderFee"`, user and builder. |
| Success | `int`, the approved maximum builder fee in protocol units. |
| Failure | Empty user/builder is rejected locally; otherwise common Info failure model. |

<!-- api: info.Client.Subaccounts -->
### Subaccounts

`func (c *Client) Subaccounts(ctx context.Context, user string) ([]Subaccount, error)`

| Item | Detail |
| --- | --- |
| Parameters | `user string`, required master account address. |
| Protocol | `POST /info`, `type: "subAccounts"`, `user`. |
| Success | `[]Subaccount`, account identities and summary state. |
| Failure | Empty user is rejected locally; otherwise common Info failure model. |

<!-- api: info.Client.Subaccounts2 -->
### Subaccounts2

`func (c *Client) Subaccounts2(ctx context.Context, user string) (*[]SubaccountV2, error)`

| Item | Detail |
| --- | --- |
| Parameters | `user string`, required master account address. |
| Protocol | `POST /info`, `type: "subAccounts2"`, `user`. |
| Success | Optional `*[]SubaccountV2`; nil preserves a null response. |
| Failure | Empty user is rejected locally; otherwise common Info failure model. |

<!-- api: info.Client.PreTransferCheck -->
### PreTransferCheck

`func (c *Client) PreTransferCheck(ctx context.Context, user, source string) (PreTransferCheckResponse, error)`

| Item | Detail |
| --- | --- |
| Parameters | `user` required actual account; `source string` is required protocol transfer source. |
| Protocol | `POST /info`, `type: "preTransferCheck"`, user and source. |
| Success | `PreTransferCheckResponse`, API eligibility/transfer-check information. |
| Failure | Empty user/source is rejected locally; otherwise common Info failure model. |

<!-- api: info.Client.IsVIP -->
### IsVIP

`func (c *Client) IsVIP(ctx context.Context, user string) (*bool, error)`

| Item | Detail |
| --- | --- |
| Parameters | `user string`, required actual account address. |
| Protocol | `POST /info`, `type: "isVip"`, `user`. |
| Success | Optional `*bool`; nil preserves a null result distinct from false. |
| Failure | Empty user is rejected locally; otherwise common Info failure model. |

<!-- api: info.Client.LegalCheck -->
### LegalCheck

`func (c *Client) LegalCheck(ctx context.Context, user string) (LegalCheckResponse, error)`

| Item | Detail |
| --- | --- |
| Parameters | `user string`, required actual account address. |
| Protocol | `POST /info`, `type: "legalCheck"`, `user`. |
| Success | `LegalCheckResponse`, protocol legal/eligibility information. |
| Failure | Empty user is rejected locally; otherwise common Info failure model. |

<!-- api: info.Client.Referral -->
### Referral

`func (c *Client) Referral(ctx context.Context, user string) (ReferralResponse, error)`

| Item | Detail |
| --- | --- |
| Parameters | `user string`, required actual account address. |
| Protocol | `POST /info`, `type: "referral"`, `user`. |
| Success | `ReferralResponse`, referral state; legacy reward history is distinct from non-funding ledger reward entries. |
| Failure | Empty user is rejected locally; otherwise common Info failure model. |

<!-- api: info.Client.SpotDeployState -->
### SpotDeployState

`func (c *Client) SpotDeployState(ctx context.Context, user string) (SpotDeployStateResponse, error)`

| Item | Detail |
| --- | --- |
| Parameters | `user string`, required actual account address. |
| Protocol | `POST /info`, `type: "spotDeployState"`, `user`. |
| Success | `SpotDeployStateResponse`, deploy token states, balances and gas-auction data; all economic fields are decimal. |
| Failure | Empty user is rejected locally; otherwise common Info failure model. |

## Vault, staking, and borrow/lend reads

<!-- api: info.Client.VaultDetails -->
### VaultDetails

`func (c *Client) VaultDetails(ctx context.Context, vaultAddress string, user *string) (*VaultDetailsResponse, error)`

| Item | Detail |
| --- | --- |
| Parameters | `vaultAddress string` required vault address; optional `user` includes caller-specific follower state. |
| Protocol | `POST /info`, `type: "vaultDetails"`, `vaultAddress`, optional `user`. |
| Success | Optional `*VaultDetailsResponse`, including vault state, followers, decimal equity/PnL/limits, and relationship data. |
| Failure | Empty vault address is rejected locally; otherwise common Info failure model. |

<!-- api: info.Client.VaultSummaries -->
### VaultSummaries

`func (c *Client) VaultSummaries(ctx context.Context) ([]VaultSummary, error)`

| Item | Detail |
| --- | --- |
| Parameters | `ctx` only. |
| Protocol | `POST /info`, `type: "vaultSummaries"`. |
| Success | `[]VaultSummary`, public vault summaries and decimal performance/equity fields. |
| Failure | Common Info failure model; no local validation. |

<!-- api: info.Client.UserVaultEquities -->
### UserVaultEquities

`func (c *Client) UserVaultEquities(ctx context.Context, user string) ([]UserVaultEquity, error)`

| Item | Detail |
| --- | --- |
| Parameters | `user string`, required actual account address. |
| Protocol | `POST /info`, `type: "userVaultEquities"`, `user`. |
| Success | `[]UserVaultEquity`, user's vault positions/deposits with decimal equity data. |
| Failure | Empty user is rejected locally; otherwise common Info failure model. |

<!-- api: info.Client.LeadingVaults -->
### LeadingVaults

`func (c *Client) LeadingVaults(ctx context.Context, user string) ([]LeadingVault, error)`

| Item | Detail |
| --- | --- |
| Parameters | `user string`, required actual account address. |
| Protocol | `POST /info`, `type: "leadingVaults"`, `user`. |
| Success | `[]LeadingVault`, ranked vault data for the user context. |
| Failure | Empty user is rejected locally; otherwise common Info failure model. |

<!-- api: info.Client.DelegatorSummary -->
### DelegatorSummary

`func (c *Client) DelegatorSummary(ctx context.Context, user string) (DelegatorSummaryResponse, error)`

| Item | Detail |
| --- | --- |
| Parameters | `user string`, required actual account address. |
| Protocol | `POST /info`, `type: "delegatorSummary"`, `user`. |
| Success | `DelegatorSummaryResponse`, stake/delegation balance summary using decimal amounts. |
| Failure | Empty user is rejected locally; otherwise common Info failure model. |

<!-- api: info.Client.Delegations -->
### Delegations

`func (c *Client) Delegations(ctx context.Context, user string) ([]Delegation, error)`

| Item | Detail |
| --- | --- |
| Parameters | `user string`, required actual account address. |
| Protocol | `POST /info`, `type: "delegations"`, `user`. |
| Success | `[]Delegation`, user's validator delegation records. |
| Failure | Empty user is rejected locally; otherwise common Info failure model. |

<!-- api: info.Client.DelegatorHistory -->
### DelegatorHistory

`func (c *Client) DelegatorHistory(ctx context.Context, user string) ([]DelegatorHistoryEntry, error)`

| Item | Detail |
| --- | --- |
| Parameters | `user string`, required actual account address. |
| Protocol | `POST /info`, `type: "delegatorHistory"`, `user`. |
| Success | `[]DelegatorHistoryEntry`, stake/delegation history. |
| Failure | Empty user is rejected locally; otherwise common Info failure model. |

<!-- api: info.Client.DelegatorRewards -->
### DelegatorRewards

`func (c *Client) DelegatorRewards(ctx context.Context, user string) ([]DelegatorReward, error)`

| Item | Detail |
| --- | --- |
| Parameters | `user string`, required actual account address. |
| Protocol | `POST /info`, `type: "delegatorRewards"`, `user`. |
| Success | `[]DelegatorReward`, staking reward history with decimal values. |
| Failure | Empty user is rejected locally; otherwise common Info failure model. |

<!-- api: info.Client.BorrowLendReserveState -->
### BorrowLendReserveState

`func (c *Client) BorrowLendReserveState(ctx context.Context, token int) (BorrowLendReserveState, error)`

| Item | Detail |
| --- | --- |
| Parameters | `token int` must be a non-negative protocol token index. |
| Protocol | `POST /info`, `type: "borrowLendReserveState"`, `token`. |
| Success | `BorrowLendReserveState`, reserve utilization/rate/liquidity state with decimals. |
| Failure | Negative token is rejected locally; otherwise common Info failure model. |

<!-- api: info.Client.AllBorrowLendReserveStates -->
### AllBorrowLendReserveStates

`func (c *Client) AllBorrowLendReserveStates(ctx context.Context) ([]BorrowLendReserve, error)`

| Item | Detail |
| --- | --- |
| Parameters | `ctx` only. |
| Protocol | `POST /info`, `type: "allBorrowLendReserveStates"`. |
| Success | `[]BorrowLendReserve`, reserve states for all known borrow/lend tokens. |
| Failure | Common Info failure model; no local validation. |

<!-- api: info.Client.BorrowLendUserState -->
### BorrowLendUserState

`func (c *Client) BorrowLendUserState(ctx context.Context, user string) (BorrowLendUserStateResponse, error)`

| Item | Detail |
| --- | --- |
| Parameters | `user string`, required actual account address. |
| Protocol | `POST /info`, `type: "borrowLendUserState"`, `user`. |
| Success | `BorrowLendUserStateResponse`: token states, health string, and optional decimal health factor. |
| Failure | Empty user is rejected locally; otherwise common Info failure model. |

<!-- api: info.Client.UserBorrowLendInterest -->
### UserBorrowLendInterest

`func (c *Client) UserBorrowLendInterest(ctx context.Context, user string, startTime int64, endTime *int64) ([]BorrowLendInterest, error)`

| Item | Detail |
| --- | --- |
| Parameters | `user` required; `startTime >= 0`; optional `endTime >= startTime`. |
| Protocol | `POST /info`, `type: "userBorrowLendInterest"`, user and range milliseconds. |
| Success | `[]BorrowLendInterest`, accrued decimal borrow/supply interest by token; paginate time ranges by last timestamp if required. |
| Failure | Invalid user/time range is rejected locally; otherwise common Info failure model. |

## Network, validators, deployment, and outcomes

<!-- api: info.Client.ExchangeStatus -->
### ExchangeStatus

`func (c *Client) ExchangeStatus(ctx context.Context) (ExchangeStatusResponse, error)`

| Item | Detail |
| --- | --- |
| Parameters | `ctx` only. |
| Protocol | `POST /info`, `type: "exchangeStatus"`. |
| Success | `ExchangeStatusResponse`, current exchange status data. |
| Failure | Common Info failure model; no local validation. |

<!-- api: info.Client.GossipPriorityAuctionStatus -->
### GossipPriorityAuctionStatus

`func (c *Client) GossipPriorityAuctionStatus(ctx context.Context) (GossipPriorityAuctionStatusResponse, error)`

| Item | Detail |
| --- | --- |
| Parameters | `ctx` only. |
| Protocol | `POST /info`, `type: "gossipPriorityAuctionStatus"`. |
| Success | `GossipPriorityAuctionStatusResponse`, current priority-auction state. |
| Failure | Common Info failure model; no local validation. |

<!-- api: info.Client.GossipRootIPs -->
### GossipRootIPs

`func (c *Client) GossipRootIPs(ctx context.Context) ([]string, error)`

| Item | Detail |
| --- | --- |
| Parameters | `ctx` only. |
| Protocol | `POST /info`, `type: "gossipRootIps"`. |
| Success | `[]string` of announced root IPs. |
| Failure | Common Info failure model; no local validation. |

<!-- api: info.Client.ValidatorSummaries -->
### ValidatorSummaries

`func (c *Client) ValidatorSummaries(ctx context.Context) ([]ValidatorSummary, error)`

| Item | Detail |
| --- | --- |
| Parameters | `ctx` only. |
| Protocol | `POST /info`, `type: "validatorSummaries"`. |
| Success | `[]ValidatorSummary`, validator public summaries. |
| Failure | Common Info failure model; no local validation. |

<!-- api: info.Client.ValidatorL1Votes -->
### ValidatorL1Votes

`func (c *Client) ValidatorL1Votes(ctx context.Context) ([]ValidatorL1Vote, error)`

| Item | Detail |
| --- | --- |
| Parameters | `ctx` only. |
| Protocol | `POST /info`, `type: "validatorL1Votes"`. |
| Success | `[]ValidatorL1Vote`, validator L1 voting data. |
| Failure | Common Info failure model; no local validation. |

<!-- api: info.Client.Liquidatable -->
### Liquidatable

`func (c *Client) Liquidatable(ctx context.Context) ([]LiquidatablePosition, error)`

| Item | Detail |
| --- | --- |
| Parameters | `ctx` only. |
| Protocol | `POST /info`, `type: "liquidatable"`. |
| Success | `[]LiquidatablePosition`, liquidatable market positions with decimal size. |
| Failure | Common Info failure model; no local validation. |

## Advanced configuration

<!-- api: info.Client.SetRequestTransport -->
### SetRequestTransport

`func (c *Client) SetRequestTransport(request transport.RequestTransport)`

| Item | Detail |
| --- | --- |
| Parameters | `request transport.RequestTransport`; nil restores HTTP transport. Only set it during construction—do not mutate a client already in use. |
| Protocol | No direct request. The transport receives future requests as `transport.RequestInfo`. |
| Success | No return value; changes the request path for subsequent Info calls. |
| Failure | The setter does not validate or return errors. Future transport/context/API/decode failures follow the common Info model; retryable request-transport status errors are retried. |

<!-- api: info.Client.Raw -->
### Raw

`func (c *Client) Raw(ctx context.Context, request any, response any) error`

| Item | Detail |
| --- | --- |
| Parameters | `request` is the caller-owned protocol request value; `response` must be a writable decode target. Prefer named methods for stable typing. |
| Protocol | `POST /info`; the caller supplies the complete body, including its protocol `type`. |
| Success | Decodes the successful JSON body into `response`; there is no SDK-owned response type. |
| Failure | JSON marshal errors, context/API/decode errors, and the common Info retry rules apply. There is no local schema validation. |

## Official references

- [Info endpoint and request schemas](https://hyperliquid.gitbook.io/hyperliquid-docs/for-developers/api/info-endpoint)
- [Asset IDs and market names](https://hyperliquid.gitbook.io/hyperliquid-docs/for-developers/api/asset-ids)
- [Tick and lot size](https://hyperliquid.gitbook.io/hyperliquid-docs/for-developers/api/tick-and-lot-size)
- [Rate limits and user limits](https://hyperliquid.gitbook.io/hyperliquid-docs/for-developers/api/rate-limits-and-user-limits)
