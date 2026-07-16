# Explorer API reference

`explorer.Client` is a separate, read-only compatibility client for
Hyperliquid's Explorer RPC. Explorer HTTP schemas are not part of the current
official trading API reference; this implementation is verified against the
public Explorer RPC behaviour and cross-checked against
[nktkas/hyperliquid](https://github.com/nktkas/hyperliquid). It does not sign,
submit, or retry trading actions.

Explorer uses dedicated HTTP and WebSocket URLs. The official API WebSocket
Info/Action post protocol does not accept Explorer request kinds. Construct the
root client with Explorer endpoints, or construct an Explorer client with a
separate `websocket.Client` configured for the Explorer RPC WebSocket URL.

## HTTP methods

HTTP calls accept `context.Context`, use the client's configured timeout, and
return a typed response. Status failures and protocol responses whose type is
`error` return an error (`*hlerr.APIError` where applicable); malformed or
empty responses are decode/unexpected-response errors. This package does not
promise a closed union for Explorer action bodies: `ExplorerAction` preserves
its discriminator plus raw object/tuple JSON for forward compatibility.

<!-- api: explorer.Client.SetRequestTransport -->
```go
func (c *explorer.Client) SetRequestTransport(request transport.RequestTransport)
```
Construction-time injection of the read-only Explorer request path. Do not
mutate it while the client is in use.

<!-- api: explorer.Client.BlockDetails -->
```go
func (c *explorer.Client) BlockDetails(ctx context.Context, height uint64) (explorer.BlockDetailsResponse, error)
```
RPC type: `blockDetails`. `height` must be non-zero. On success the response
contains `Type` and `BlockDetails`, which embeds `ExplorerBlock` and its
`[]ExplorerTransaction` (`txs`).

<!-- api: explorer.Client.TxDetails -->
```go
func (c *explorer.Client) TxDetails(ctx context.Context, hash string) (explorer.TxDetailsResponse, error)
```
RPC type: `txDetails`. `hash` must be an exact 32-byte hexadecimal value. On
success the response contains `Type` and one `ExplorerTransaction` (`tx`).

<!-- api: explorer.Client.UserDetails -->
```go
func (c *explorer.Client) UserDetails(ctx context.Context, user string) (explorer.UserDetailsResponse, error)
```
RPC type: `userDetails`. `user` must be an exact 20-byte hexadecimal Ethereum
address. On success it contains `Type` and `[]ExplorerTransaction` (`txs`).

`ExplorerTransaction` has integer `Block`/`Time`, hash, user, optional error,
and an open `ExplorerAction`; it intentionally preserves protocol action data
rather than converting unknown economic values through `float64`.

## Explorer WebSocket streams

The streams below are HTTP-independent and are lazily opened when an Explorer
WebSocket URL is configured. They return the same resilient subscription
handles as the market WebSocket: `Events`, `Errors`, `States`, and `Close`.
They reconnect/replay according to the configured `websocket.Config`; see
[WebSocket API](websocket.md) for lifecycle and slow-consumer semantics.

<!-- api: explorer.Client.ExplorerBlock -->
```go
func (c *explorer.Client) ExplorerBlock(ctx context.Context) (*explorer.ExplorerBlockSubscription, error)
```
Explorer channel: `explorerBlock`. Success returns batches of
`[]explorer.Block`. A missing Explorer WebSocket configuration is an error;
this is not a trading API subscription.

<!-- api: explorer.Client.ExplorerTxs -->
```go
func (c *explorer.Client) ExplorerTxs(ctx context.Context) (*explorer.ExplorerTxsSubscription, error)
```
Explorer channel: `explorerTxs`. Success returns batches of
`[]explorer.Transaction`. A missing Explorer WebSocket configuration is an
error; this is not a trading API subscription.

<!-- api: explorer.Client.Close -->
```go
func (c *explorer.Client) Close() error
```
Idempotently releases only the lazily created Explorer subscription client. It
does not close Info, Exchange, or the market WebSocket client. An externally
injected Explorer WebSocket client remains caller-owned and is not closed.
