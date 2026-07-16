# Hyperliquid Go SDK

[![Go Reference](https://pkg.go.dev/badge/github.com/Apexllcc/hyperliquid-go-sdk.svg)](https://pkg.go.dev/github.com/Apexllcc/hyperliquid-go-sdk)

面向 Hyperliquid 的精度安全、Transport 可替换 Go SDK。根客户端将无签名读数据、
签名 Exchange 操作和实时订阅分开：

```text
Client
├── Info       无签名 HTTP 或 RequestTransport 读取
├── Exchange   签名且只提交一次的 action
├── WebSocket  共享订阅连接及重连管理
└── Explorer   只读 Explorer RPC 客户端
```

价格和数量使用 `decimal.Decimal` 或协议十进制字符串；经济数值绝不使用 `float64`。

English version: [README.md](README.md)。完整的已实现公开 API 矩阵见 [API.md](API.md)。

## 安装

```bash
go get github.com/Apexllcc/hyperliquid-go-sdk
```

需要 Go 1.23 或更高版本。默认网络为 Mainnet；开发和集成工作必须显式选择 Testnet。

```go
client, err := hyperliquid.NewClient(hyperliquid.WithTestnet())
if err != nil {
	return err
}
defer client.Close()
```

`Client.Close` 关闭 SDK 自己拥有的 WebSocket 连接。注入的 HTTP/Request Transport、
Asset Resolver、Nonce Manager 和 Signer 仍由调用方拥有和关闭。

## 读取数据

Info 调用不需要签名，且请求与响应为强类型。下面读取当前中间价及账户永续状态：

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

可运行版本见 [examples/info/main.go](examples/info/main.go)：

```bash
go run ./examples/info
```

使用 `Info.Meta`、`Info.SpotMeta` 和 `Info.PerpDEXs` 获取市场元数据。默认
`asset.MetaResolver` 加载永续、现货和 HIP-3 元数据，按 TTL 刷新、合并并发刷新，
并拒绝有歧义的 symbol。当 symbol 可能存在于多个命名空间时，应使用
`types.MarketRef`：

```go
market := types.MarketRef{Symbol: "BTC", Kind: types.Perpetual}
// HIP-3 示例：types.MarketRef{Symbol: "dex:BTC", Kind: types.HIP3, DEX: "dex"}
```

## 交易

Exchange 不持有私钥，只接受 `signer.DigestSigner`：SDK 构造最终 L1 或 EIP-712
digest，调用 `SignDigest`，再在发送前校验规范低 S 签名，并确认恢复地址等于
`Address()`。SDK 有意不实现 remote/KMS/HSM/MPC 协议或客户端；外部系统可自行
实现 `DigestSigner`。

`signer.LocalPrivateKeySigner` 是可选实现，仅用于本地开发或受控 Testnet 环境。
绝不提交私钥、通过命令行传递私钥，或将以下 quick-start 原样用于生产密钥托管。

```go
key := os.Getenv("HL_TESTNET_PRIVATE_KEY")
if key == "" || os.Getenv("HL_TESTNET_TRADE") != "1" {
	return errors.New("需要显式 Testnet key 和 HL_TESTNET_TRADE=1")
}

local, err := signer.NewLocalPrivateKeySignerFromHex(key)
if err != nil {
	return err
}
defer local.Close()

client, err := hyperliquid.NewClient(
	hyperliquid.WithTestnet(), // Testnet 工作流中不得省略
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
	return err // 包括 HTTP/API 错误和强类型 Exchange 拒绝
}
fmt.Printf("action=%s response=%T\n", result.Status, result.Response.Data)
```

以上订单值仅用于说明。提交前必须校验 tick size、数量精度、账户模式、杠杆、
抵押品和风控。可运行的[干运行示例](examples/exchange_limit_order/main.go)只构造
强类型请求，**不会**创建 signer 或发送订单。

### 提交保证

- `Info` 只重试 429、502、503、504；支持有界指数退避、jitter、`Retry-After`、
  请求体重发和 context 取消。
- `Exchange` 永不自动重试。网络错误时 action 可能已到达交易所；再次操作前请用
  `Info.OrderStatus`、`OpenOrders`、`UserFills` 或 client order ID 完成对账。
- 默认 Nonce Manager 按 signer 地址单调递增。跨进程共享 nonce 时请通过
  `WithNonceManager` 注入共享实现。
- `WithVaultAddress`、`WithExpiresAfter` 配置支持的 L1 action。User-signed action
  刻意不包含 `expiresAfter`。

## 实时更新

所有订阅复用一个受管理连接。客户端发送心跳，以有界指数退避和 jitter 重连，并在
连接成功后恢复逻辑订阅。每个 SDK 订阅都提供事件、错误、`Close` 和生命周期状态。

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

运行无私钥的公开示例：`go run ./examples/websocket`。

`websocket.Config` 可控制事件/状态队列、ping/pong、重连策略、Dialer 和慢消费者
行为（`BackpressureBlock`、`BackpressureDropNewest`、`BackpressureDropOldest`）。
自定义 `Dialer` 必须响应 context 取消，否则进行中的 dial 会延迟 `Close`。

## Transport、限流与可观测性

使用 middleware 添加 request ID、限制 HTTP 尝试频率，并输出安全的请求元数据。
Middleware 不记录请求或响应正文。

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

`WithRequestTransport` 可让 Info 和 Exchange 使用 WebSocket post 请求路径；它仍由
调用方拥有，且不会改变 Exchange 的“绝不自动重试”保证。Explorer 请求应单独使用
`WithExplorerRequestTransport`。

## 错误与响应处理

Transport/HTTP 失败以 `*hyperliquid.APIError` 返回；调用方输入无效通常为
`*hyperliquid.ValidationError`。协议层 Exchange 拒绝可能以 HTTP 200 返回，并作为
`*exchange.ActionResponseError` 返回。成功 Exchange payload 是
`response.Response.Data` 中的强类型 union；未知的未来变体保留为
`exchange.UnknownActionResponseData`。

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

## Testnet 集成测试

集成测试只在同时带上两个 build tag 时编译，且所有网络访问都要求显式环境变量。
它们绝不指向 Mainnet。

```bash
# 编译测试套件；没有相应 opt-in 时测试仍会 skip。
go test -tags='integration testnet' ./tests/integration

# 只读账户、Info 和公开 WebSocket 检查。
HL_TESTNET_READONLY=1 \
HL_TESTNET_ACCOUNT_ADDRESS=0xYourTestnetAccount \
go test -tags='integration testnet' ./tests/integration
```

可变 Testnet 测试还要求 `HL_TESTNET_TRADE=1` 和 `HL_TESTNET_PRIVATE_KEY`；
Unified/Portfolio 测试要求 `HL_TESTNET_UNIFIED_TRADE=1`，逐仓保证金测试要求
`HL_TESTNET_ISOLATED_TRADE=1`，单向现货转账测试要求 `HL_TESTNET_TRANSFER=1`。
启用任何可变测试前，请阅读测试源码并使用可废弃的 Testnet 账户。

## 协议兼容性与签名向量

协议字段以[官方 API](https://hyperliquid.gitbook.io/hyperliquid-docs/for-developers/api)、
[官方签名指南](https://hyperliquid.gitbook.io/hyperliquid-docs/for-developers/api/signing)
和官方 [Python SDK](https://github.com/hyperliquid-dex/hyperliquid-python-sdk) 为准。
固定 L1 和 EIP-712 向量覆盖 action hash、Vault、`expiresAfter`、builder、agent
approval、TWAP、multisig、R/S/V 和恢复地址。

```bash
go test ./signing ./signer
```

`upstream.lock.json` 使用 SHA-256 固定已审查的官方文档，使用不可变 Git revision 与
文件 digest 固定 Python SDK。可离线校验 lock，也可检查在线上游漂移：

```bash
go run ./scripts/upstreamcheck -lock upstream.lock.json
go run ./scripts/upstreamcheck -lock upstream.lock.json -network -total-timeout=45s
```

定期运行的 [upstream drift workflow](.github/workflows/upstream-drift.yml)
会报告上游变化，而不是静默接受它。发生漂移时应比较协议变更、更新强类型模型/向量/
测试，然后有意刷新 lock。

## 生产检查表

- 使用专用 API wallet 或外部 `DigestSigner`；在可行时让密钥托管在应用进程之外。
- 对每个被多进程使用的 signer 持久化/共享 nonce 分配。
- 设置每请求 context，并按账户和端点预算使用 Rate Limit。
- 不自动重试签名 Exchange action；重发前使用 Info 对账不确定结果。
- 监控请求状态/延迟、限流响应、WebSocket 状态变化、丢弃事件错误和重连时长。
- 对现货或 HIP-3 使用显式 `MarketRef`，校验十进制精度，并保存 client order ID 供对账。
- 启用生产交易前，在 Testnet 验证故障路径、backpressure 设置和重连行为。

## 更多资料

- [完整公开 API 矩阵](API.md)
- [Info 示例](examples/info/main.go)
- [WebSocket 示例](examples/websocket/main.go)
- [安全的 Exchange 干运行](examples/exchange_limit_order/main.go)
- [Hyperliquid 官方 API](https://hyperliquid.gitbook.io/hyperliquid-docs/for-developers/api)
