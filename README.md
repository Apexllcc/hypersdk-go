# Hyperliquid Go SDK

[![Go Reference](https://pkg.go.dev/badge/github.com/Apexllcc/hypersdk-go.svg)](https://pkg.go.dev/github.com/Apexllcc/hypersdk-go)

Precision-safe, transport-pluggable Go SDK for Hyperliquid. The root client
keeps unsigned read data, signed Exchange actions, and real-time subscriptions
separate:

```text
Client
├── Info       unsigned HTTP or request-transport reads
├── Exchange   signed, exactly-once action submission
├── WebSocket  shared subscription connection and reconnect management
└── Explorer   read-only Explorer RPC client
```

Prices and sizes are `decimal.Decimal` or protocol decimal strings; economic
values are never represented by `float64`.

中文说明见 [README_ZH.md](README_ZH.md). The complete, implemented public API
matrix is in [API.md](API.md).

## Install

```bash
go get github.com/Apexllcc/hypersdk-go
```

Requires Go 1.23 or newer. Mainnet is the default. Choose Testnet explicitly
for development and integration work.

```go
client, err := hyperliquid.NewClient(hyperliquid.WithTestnet())
if err != nil {
	return err
}
defer client.Close()
```

`Client.Close` closes SDK-owned WebSocket connections. Injected HTTP/request
transports, asset resolvers, nonce managers, and signers remain caller-owned.

## Read data

Info calls are unsigned and strongly typed. This example requests current
mid-prices and an account's perpetual state.

```go
ctx := context.Background()
client, err := hyperliquid.NewClient(hyperliquid.WithTestnet())
if err != nil {
	return err
}
defer client.Close()

mids, err := client.Info.AllMids(ctx)
if err != nil {
	return err
}
btcMid := mids["BTC"]

state, err := client.Info.ClearinghouseState(ctx, "0xYourAccount")
if err != nil {
	return err
}
fmt.Println("BTC mid:", btcMid, "positions:", state.AssetPositions)
```

See a runnable version at [examples/info/main.go](examples/info/main.go):

```bash
go run ./examples/info
```

Use `Info.Meta`, `Info.SpotMeta`, and `Info.PerpDEXs` for market metadata.
The default `asset.MetaResolver` loads perpetual, spot, and HIP-3 metadata,
refreshes it with a bounded TTL, coalesces concurrent refreshes, and rejects
ambiguous symbols. Prefer `types.MarketRef` where a symbol can exist in more
than one namespace:

```go
market := types.MarketRef{Symbol: "BTC", Kind: types.Perpetual}
// HIP-3 example: types.MarketRef{Symbol: "dex:BTC", Kind: types.HIP3, DEX: "dex"}
```

## Trading

Exchange does not own a private key. It accepts only `signer.DigestSigner`:
the SDK builds the final L1 or EIP-712 digest, calls `SignDigest`, then verifies
canonical low-S form and the recovered address against `Address()` before
sending the action. The SDK intentionally provides no remote/KMS/HSM/MPC
protocol or client; an external system can implement `DigestSigner` directly.

`signer.LocalPrivateKeySigner` is optional and intended for local development
or controlled Testnet environments. Never commit a key, pass it on a command
line, or use this quick-start unchanged for production key custody.

```go
key := os.Getenv("HL_TESTNET_PRIVATE_KEY")
if key == "" || os.Getenv("HL_TESTNET_TRADE") != "1" {
	return errors.New("explicit Testnet key and HL_TESTNET_TRADE=1 are required")
}

local, err := signer.NewLocalPrivateKeySignerFromHex(key)
if err != nil {
	return err
}
defer local.Close()

client, err := hyperliquid.NewClient(
	hyperliquid.WithTestnet(), // do not omit this in a Testnet workflow
	hyperliquid.WithDigestSigner(local),
)
if err != nil {
	return err
}
defer client.Close()

result, err := client.Exchange.PlaceOrder(ctx, exchange.OrderRequest{
	Market:  &types.MarketRef{Symbol: "BTC", Kind: types.Perpetual},
	IsBuy:   true,
	Price:   decimal.RequireFromString("10000"),
	Size:    decimal.RequireFromString("0.001"),
	Type:    exchange.LimitOrder{TimeInForce: exchange.TIFGTC},
	ReduceOnly: false,
})
if err != nil {
	return err // includes HTTP/API errors and typed Exchange rejections
}
fmt.Printf("action=%s response=%T\n", result.Status, result.Response.Data)
```

The order values above are deliberately illustrative; validate tick size,
size precision, account mode, leverage, collateral, and risk controls before
submitting any action. The runnable [dry-run](examples/exchange_limit_order/main.go)
constructs the typed request but **never** creates a signer or sends an order.

### Submission guarantees

- `Info` retries only 429, 502, 503, and 504, with bounded exponential delay,
  jitter, `Retry-After`, request-body replay, and context cancellation.
- `Exchange` is never automatically retried. A transport failure may have
  reached the exchange; reconcile with `Info.OrderStatus`, `OpenOrders`,
  `UserFills`, or your client order ID before deciding what to do next.
- The default nonce manager is monotonic per signer address. Supply
  `WithNonceManager` when sharing nonce allocation across processes.
- `WithVaultAddress` and `WithExpiresAfter` configure supported L1 actions.
  User-signed actions deliberately do not include `expiresAfter`.

## Real-time updates

Subscriptions share one managed connection. The client sends heartbeats,
reconnects with bounded exponential backoff and jitter, and restores logical
subscriptions after a successful reconnect. Every SDK subscription exposes
events, errors, `Close`, and lifecycle state transitions.

```go
ctx, cancel := context.WithCancel(context.Background())
defer cancel()

client, err := hyperliquid.NewClient(hyperliquid.WithTestnet())
if err != nil {
	return err
}
defer client.Close()

sub, err := client.WebSocket.SubscribeAllMids(ctx, websocket.AllMidsRequest{})
if err != nil {
	return err
}
defer sub.Close()

for {
	select {
	case update := <-sub.Events():
		fmt.Println(update.Mids["BTC"])
	case state := <-sub.States():
		fmt.Println("subscription state:", state.State)
	case err := <-sub.Errors():
		return err
	case <-ctx.Done():
		return ctx.Err()
	}
}
```

Run the public no-key example with `go run ./examples/websocket`.

`websocket.Config` controls event/state queue sizes, ping/pong, reconnect
policy, dialer, and slow-consumer behavior (`BackpressureBlock`,
`BackpressureDropNewest`, or `BackpressureDropOldest`). A custom `Dialer` must
honor context cancellation; otherwise an in-flight dial can delay `Close`.
`Subscribed` means the server returned the matching `subscriptionResponse`,
including after every reconnect. Configurable admission limits default to
1,000 active subscriptions, 10 unique users, 2,000 outgoing messages per
rolling minute across all WebSocket connections, and 100 simultaneous
WebSocket POSTs. The official caps are per IP. Inject the same
`SubscriptionAdmission`, `MessageAdmission`, and `PostAdmission` instances into
every Client sharing an IP to enforce one atomic boundary; construct them with
`NewSubscriptionAdmissionGate`, `NewMessageAdmissionLimiter`, and
`NewPostAdmissionGate`. Subscription leases count normalized server identities
per Client connection and refcount normalized users across Clients. Queued
outbound waits honor context cancellation and connection shutdown without
stalling inbound acknowledgements, events, or pongs. Subscription writes use a
bounded deadline, and shutdown closes the active socket before joining writers.

## Transport, rate limits, and observability

Use middleware to attach request IDs, limit HTTP attempt rate, and emit safe
request metadata. Middleware never logs request/response bodies.

```go
httpClient := &http.Client{Timeout: 10 * time.Second}
client, err := hyperliquid.NewClient(
	hyperliquid.WithTestnet(),
	hyperliquid.WithHTTPClient(httpClient),
	hyperliquid.WithInfoRetryPolicy(transport.RetryPolicy{
		MaxAttempts: 4,
		BaseDelay:   200 * time.Millisecond,
		MaxDelay:    3 * time.Second,
	}),
	hyperliquid.WithMiddleware(
		transport.RequestID(func() string { return fmt.Sprintf("request-%d", time.Now().UnixNano()) }),
		transport.RateLimit(50*time.Millisecond),
		transport.Logging(func(event transport.RequestLog) { log.Printf("%s %d", event.Path, event.StatusCode) }),
		transport.Metrics(func(event transport.RequestMetric) { log.Printf("latency=%s", event.Duration) }),
	),
)
```

For Hyperliquid's official shared REST budget, use
`hyperliquid.WithOfficialRateLimit()`. It applies a concurrency-safe,
context-cancellable 1200-weight-per-minute policy across Info, Exchange, and
Explorer HTTP attempts. The policy applies the documented endpoint, batch, and
response-size weights; it never retries Exchange actions. Use
`WithRateLimitPolicy` with a `transport.WeightPolicy` to provide a replacement
weight schedule. To share one admission budget across clients on the same IP,
construct `transport.NewWeightLimiter(capacity, window)` and pass it with
`WithRateLimitPolicyAndLimiter`.

`WithRequestTransport` can use the WebSocket post request path for Info and
Exchange. It remains caller-owned and does not change Exchange's no-retry
guarantee. Explorer requests use `WithExplorerRequestTransport` separately.

## Errors and response handling

Transport/HTTP failures are returned as `*hyperliquid.APIError`; invalid
caller input is usually a `*hyperliquid.ValidationError`. A protocol-level
Exchange rejection may arrive with HTTP 200 and is returned as
`*exchange.ActionResponseError`. Successful Exchange response payloads are a
typed union in `response.Response.Data`; unknown future variants are retained
as `exchange.UnknownActionResponseData`.

```go
var apiErr *hyperliquid.APIError
var actionErr *exchange.ActionResponseError
switch {
case errors.As(err, &apiErr):
	log.Printf("HTTP %d: %s", apiErr.StatusCode, apiErr.Message)
case errors.As(err, &actionErr):
	log.Printf("Exchange rejected %s: %s", actionErr.Status, actionErr.Message)
case err != nil:
	return err
}
```

## Testnet integration tests

Integration tests are compiled only when both tags are present, and all
network access requires explicit environment variables. They never target
Mainnet.

```bash
# Compiles the suite; tests remain skipped without the matching opt-ins.
go test -tags='integration testnet' ./tests/integration

# Read-only account, Info, and public WebSocket checks.
HL_TESTNET_READONLY=1 \
HL_TESTNET_ACCOUNT_ADDRESS=0xYourTestnetAccount \
go test -tags='integration testnet' ./tests/integration
```

Mutable Testnet tests additionally require `HL_TESTNET_TRADE=1` and
`HL_TESTNET_PRIVATE_KEY`; Unified/Portfolio tests require
`HL_TESTNET_UNIFIED_TRADE=1`, isolated-margin tests require
`HL_TESTNET_ISOLATED_TRADE=1`, and the one-way spot transfer test requires
`HL_TESTNET_TRANSFER=1`. Read the test source and use a disposable Testnet
account before enabling any mutable group.

## Protocol compatibility and signing vectors

Protocol field definitions follow the [official API](https://hyperliquid.gitbook.io/hyperliquid-docs/for-developers/api),
[official signing guide](https://hyperliquid.gitbook.io/hyperliquid-docs/for-developers/api/signing),
and the official [Python SDK](https://github.com/hyperliquid-dex/hyperliquid-python-sdk).
Fixed L1 and EIP-712 vectors cover action hashes, Vault, `expiresAfter`,
builder, agent approval, TWAP, multisig, R/S/V values, and recovered addresses.

```bash
go test ./signing ./signer
```

`upstream.lock.json` independently pins the API index, signing, Exchange,
Info, WebSocket/subscriptions, rate-limit, fees, staking, account-abstraction,
and portfolio-margin pages by SHA-256. It also pins the Python SDK by immutable
Git revision plus file digests. The online check reports deterministic
added/removed Info request types, Exchange action types, WebSocket subscription
types, Python methods, and extractable signing markers; it never rewrites the
lock. Verify it offline or check live upstream drift:

```bash
go run ./scripts/upstreamcheck -lock upstream.lock.json
go run ./scripts/upstreamcheck -lock upstream.lock.json -network -total-timeout=45s
```

The scheduled [upstream drift workflow](.github/workflows/upstream-drift.yml)
reports an upstream change rather than silently accepting it. Treat a drift as
a review trigger: compare protocol changes, update typed models/vectors/tests,
then intentionally refresh the lock.

## Production checklist

- Use a dedicated API wallet or an external `DigestSigner`; keep key custody
  outside the application process where practical.
- Persist/share nonce allocation for every signer used by multiple processes.
- Set per-request contexts and use a rate limiter suitable for your account
  and endpoint budget.
- Do not retry signed Exchange actions automatically; reconcile ambiguous
  outcomes via Info before resubmission.
- Monitor request status/latency, rate-limit responses, WebSocket state
  transitions, dropped-event errors, and reconnect duration.
- Use explicit `MarketRef` for spot or HIP-3 markets, validate decimal
  precision, and retain client order IDs for reconciliation.
- Test failure paths, backpressure settings, and reconnect behavior in Testnet
  before enabling production trading.

## Further reading

- [Complete public API matrix](API.md)
- [Info example](examples/info/main.go)
- [WebSocket example](examples/websocket/main.go)
- [Safe Exchange dry-run](examples/exchange_limit_order/main.go)
- [Official Hyperliquid API](https://hyperliquid.gitbook.io/hyperliquid-docs/for-developers/api)
