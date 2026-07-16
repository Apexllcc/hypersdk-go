# Exchange API reference

`exchange.Client` submits signed actions to Hyperliquid's Exchange endpoint.
Construct it through the root client in normal applications. The client accepts
only `signer.DigestSigner`; it never owns a private key.

## Common submission contract

Every method below accepts `context.Context`. Unless noted otherwise it builds
an L1 action, obtains a monotonically increasing nonce for the configured
signer, computes the canonical L1 digest, checks low-S canonicality and the
recovered signer address, then sends **one** request. It never retries an
Exchange action: a timeout or network error cannot prove non-execution.

`WithVaultAddress` routes supported L1 actions for a vault/subaccount while
the master/API wallet signs. `WithExpiresAfter` applies only where the
protocol accepts it; it is never added to user-signed EIP-712 actions. Prices,
sizes, amounts and rates use `decimal.Decimal` or canonical strings—never
`float64`. A `types.MarketRef` is preferred for HIP-3; otherwise `Coin` must
resolve through the injected asset resolver.

Success returns `ActionResponse` with `Status == "ok"`. Its
`Response.Data` union is `DefaultActionResponseData`, `OrderResponseData`
(each entry is `Resting`, `Filled`, or per-order `Error`),
`CancelResponseData`, `TWAPOrderResponseData`, `TWAPCancelResponseData`,
`CreateVaultResponseData`, or forward-compatible `UnknownActionResponseData`.
An HTTP 2xx protocol rejection returns both the response and
`*ActionResponseError`; HTTP/status, validation, signing, nonce, resolver,
context, and decode failures return a non-nil error. Refer to the
[official Exchange endpoint](https://hyperliquid.gitbook.io/hyperliquid-docs/for-developers/api/exchange-endpoint)
and [signing documentation](https://hyperliquid.gitbook.io/hyperliquid-docs/for-developers/api/signing)
for authoritative wire schemas.

`SetRequestTransport` is construction-time injection for a custom action
path. A replacement must preserve the no-retry rule.

<!-- api: exchange.Client.SetRequestTransport -->
```go
func (c *exchange.Client) SetRequestTransport(request transport.RequestTransport)
```

## Orders, cancellation, and leverage

`OrderRequest` has `Coin` or `*types.MarketRef`, `IsBuy`, positive `Price` and
`Size`, `ReduceOnly`, a `LimitOrder` (`TIFGTC`, `TIFIOC`, `TIFALO`) or
`TriggerOrder` (`TPSLTakeProfit`/`TPSLStopLoss`, positive trigger price), and
optional 128-bit `ClientOrderID` and `Builder`. Size is checked against market
lot precision; price is checked against protocol significant-digit and tick
rules. Builder fees are in tenths of a basis point and are validated per asset.

<!-- api: exchange.Client.PlaceOrder -->
```go
func (c *exchange.Client) PlaceOrder(ctx context.Context, request exchange.OrderRequest) (exchange.OrderResponse, error)
```
Protocol action: `order`; returns the typed order-status union.

<!-- api: exchange.Client.PlaceOrders -->
```go
func (c *exchange.Client) PlaceOrders(ctx context.Context, requests []exchange.OrderRequest) (exchange.OrderResponse, error)
```
Protocol action: `order`; non-empty batch, with one consistent optional builder.

`ModifyRequest` chooses exactly one existing ID (`OID` or `*types.Cloid`) and
contains its replacement `OrderRequest`.

<!-- api: exchange.Client.ModifyOrder -->
```go
func (c *exchange.Client) ModifyOrder(ctx context.Context, request exchange.ModifyRequest) (exchange.ActionResponse, error)
```
Protocol action: `modify`.

<!-- api: exchange.Client.BatchModify -->
```go
func (c *exchange.Client) BatchModify(ctx context.Context, requests []exchange.ModifyRequest) (exchange.ActionResponse, error)
```
Protocol action: `batchModify`; requires a non-empty batch.

`CancelRequest` identifies an order by non-empty `Coin` and non-zero `OID`.
`CancelByCloidRequest` uses `Coin` and a validated `types.Cloid`.

<!-- api: exchange.Client.CancelOrder -->
```go
func (c *exchange.Client) CancelOrder(ctx context.Context, request exchange.CancelRequest) (exchange.ActionResponse, error)
```
Protocol action: `cancel`.

<!-- api: exchange.Client.CancelOrders -->
```go
func (c *exchange.Client) CancelOrders(ctx context.Context, requests []exchange.CancelRequest) (exchange.ActionResponse, error)
```
Protocol action: `cancel`; requires a non-empty batch.

<!-- api: exchange.Client.CancelByCloid -->
```go
func (c *exchange.Client) CancelByCloid(ctx context.Context, request exchange.CancelByCloidRequest) (exchange.ActionResponse, error)
```
Protocol action: `cancelByCloid`.

<!-- api: exchange.Client.CancelByCloids -->
```go
func (c *exchange.Client) CancelByCloids(ctx context.Context, requests []exchange.CancelByCloidRequest) (exchange.ActionResponse, error)
```
Protocol action: `cancelByCloid`; requires a non-empty batch.

<!-- api: exchange.Client.ScheduleCancel -->
```go
func (c *exchange.Client) ScheduleCancel(ctx context.Context, at *uint64) (exchange.ActionResponse, error)
```
Protocol action: `scheduleCancel`. A Unix-millisecond value schedules the
dead-man's switch; `nil` clears it. The protocol requires a future time and
enforces its own daily trigger limit.

`UpdateLeverageRequest` uses `Coin` xor `Market`, positive integral
`Leverage`, and `IsCross`. `UpdateIsolatedMarginRequest.Amount` is signed USDC
with at most six decimals; `TopUpIsolatedOnlyMarginRequest.Leverage` is a
positive decimal target.

<!-- api: exchange.Client.UpdateLeverage -->
```go
func (c *exchange.Client) UpdateLeverage(ctx context.Context, request exchange.UpdateLeverageRequest) (exchange.ActionResponse, error)
```
Protocol action: `updateLeverage`.

<!-- api: exchange.Client.UpdateIsolatedMargin -->
```go
func (c *exchange.Client) UpdateIsolatedMargin(ctx context.Context, request exchange.UpdateIsolatedMarginRequest) (exchange.ActionResponse, error)
```
Protocol action: `updateIsolatedMargin`; exact USDC is converted to protocol micros.

<!-- api: exchange.Client.TopUpIsolatedOnlyMargin -->
```go
func (c *exchange.Client) TopUpIsolatedOnlyMargin(ctx context.Context, request exchange.TopUpIsolatedOnlyMarginRequest) (exchange.ActionResponse, error)
```
Protocol action: `topUpIsolatedOnlyMargin`.

## TWAP and basic L1 administration

`TWAPOrderRequest` requires `Coin` xor `Market`, positive precision-valid
`Size`, and positive `Minutes`; `TWAPCancelRequest` additionally needs a
non-zero `TWAPID`.

<!-- api: exchange.Client.PlaceTWAP -->
```go
func (c *exchange.Client) PlaceTWAP(ctx context.Context, request exchange.TWAPOrderRequest) (exchange.ActionResponse, error)
```
Protocol action: `twapOrder`; data is `TWAPOrderResponseData` with running ID or error.

<!-- api: exchange.Client.CancelTWAP -->
```go
func (c *exchange.Client) CancelTWAP(ctx context.Context, request exchange.TWAPCancelRequest) (exchange.ActionResponse, error)
```
Protocol action: `twapCancel`; data is `TWAPCancelResponseData`.

<!-- api: exchange.Client.ReserveRequestWeight -->
```go
func (c *exchange.Client) ReserveRequestWeight(ctx context.Context, weight uint64) (exchange.ActionResponse, error)
```
Protocol action: `reserveRequestWeight`; `weight` must be positive.

<!-- api: exchange.Client.Noop -->
```go
func (c *exchange.Client) Noop(ctx context.Context, nonceValue uint64) (exchange.ActionResponse, error)
```
Protocol action: `noop`; directly consumes a positive caller-provided pending nonce.

## User-signed transfers, agent, and builder permissions

The following use the official EIP-712 User Signed Action domain. They allocate
a nonce from the configured signer, verify its signature, and do **not** use
`expiresAfter`. Address values must be non-zero; `decimal.Decimal` amounts are
positive and rendered canonically. They still follow the common response/error
contract.

<!-- api: exchange.Client.SendUSD -->
```go
func (c *exchange.Client) SendUSD(ctx context.Context, request exchange.USDSendRequest) (exchange.ActionResponse, error)
```
Protocol action: `usdSend`; core USDC transfer.

<!-- api: exchange.Client.SendSpot -->
```go
func (c *exchange.Client) SendSpot(ctx context.Context, request exchange.SpotSendRequest) (exchange.ActionResponse, error)
```
Protocol action: `spotSend`; `Token` is required.

<!-- api: exchange.Client.WithdrawFromBridge -->
```go
func (c *exchange.Client) WithdrawFromBridge(ctx context.Context, request exchange.WithdrawRequest) (exchange.ActionResponse, error)
```
Protocol action: `withdraw3`; bridge withdrawal request.

<!-- api: exchange.Client.TransferUSDClass -->
```go
func (c *exchange.Client) TransferUSDClass(ctx context.Context, request exchange.USDClassTransferRequest) (exchange.ActionResponse, error)
```
Protocol action: `usdClassTransfer`; supports configured subaccount/vault routing where protocol permits.

<!-- api: exchange.Client.SendAsset -->
```go
func (c *exchange.Client) SendAsset(ctx context.Context, request exchange.SendAssetRequest) (exchange.ActionResponse, error)
```
Protocol action: `sendAsset`; `Token` and optional source/destination DEX namespace select the asset route.

<!-- api: exchange.Client.SendToEVMWithData -->
```go
func (c *exchange.Client) SendToEVMWithData(ctx context.Context, request exchange.SendToEVMWithDataRequest) (exchange.ActionResponse, error)
```
Protocol action: `sendToEvmWithData`; token, amount, recipient, encoding, chain ID and gas limit are required.

<!-- api: exchange.Client.CDeposit -->
```go
func (c *exchange.Client) CDeposit(ctx context.Context, request exchange.StakingTransferRequest) (exchange.ActionResponse, error)
```
Protocol action: `cDeposit`; positive native-token wei.

<!-- api: exchange.Client.DepositStaking -->
```go
func (c *exchange.Client) DepositStaking(ctx context.Context, request exchange.StakingTransferRequest) (exchange.ActionResponse, error)
```
Alias for `CDeposit`.

<!-- api: exchange.Client.CWithdraw -->
```go
func (c *exchange.Client) CWithdraw(ctx context.Context, request exchange.StakingTransferRequest) (exchange.ActionResponse, error)
```
Protocol action: `cWithdraw`; positive native-token wei.

<!-- api: exchange.Client.WithdrawStaking -->
```go
func (c *exchange.Client) WithdrawStaking(ctx context.Context, request exchange.StakingTransferRequest) (exchange.ActionResponse, error)
```
Alias for `CWithdraw`.

<!-- api: exchange.Client.TokenDelegate -->
```go
func (c *exchange.Client) TokenDelegate(ctx context.Context, request exchange.TokenDelegateRequest) (exchange.ActionResponse, error)
```
Protocol action: `tokenDelegate`; non-zero validator and positive wei; `IsUndelegate` selects direction.

<!-- api: exchange.Client.Delegate -->
```go
func (c *exchange.Client) Delegate(ctx context.Context, validator common.Address, wei uint64) (exchange.ActionResponse, error)
```
Convenience form of `TokenDelegate`.

<!-- api: exchange.Client.Undelegate -->
```go
func (c *exchange.Client) Undelegate(ctx context.Context, validator common.Address, wei uint64) (exchange.ActionResponse, error)
```
Convenience form of `TokenDelegate` with `IsUndelegate`.

<!-- api: exchange.Client.ApproveAgent -->
```go
func (c *exchange.Client) ApproveAgent(ctx context.Context, request exchange.ApproveAgentRequest) (exchange.ActionResponse, error)
```
Protocol action: `approveAgent`; `AgentName` is optional.

<!-- api: exchange.Client.ApproveBuilderFee -->
```go
func (c *exchange.Client) ApproveBuilderFee(ctx context.Context, request exchange.ApproveBuilderFeeRequest) (exchange.ActionResponse, error)
```
Protocol action: `approveBuilderFee`; builder address and non-empty `MaxFeeRate` are required.

## Vaults, subaccounts, and multisig

Transfers below use the L1 route and intentionally sign outside a configured
trading vault where required by the protocol. USD values are exact
`decimal.Decimal` values (maximum six decimals). Vault/subaccount addresses
must be non-zero.

<!-- api: exchange.Client.TransferSubaccountUSD -->
```go
func (c *exchange.Client) TransferSubaccountUSD(ctx context.Context, request exchange.SubaccountTransferRequest) (exchange.ActionResponse, error)
```
Protocol action: `subAccountTransfer`.

<!-- api: exchange.Client.TransferSubaccountSpot -->
```go
func (c *exchange.Client) TransferSubaccountSpot(ctx context.Context, request exchange.SubaccountSpotTransferRequest) (exchange.ActionResponse, error)
```
Protocol action: `subAccountSpotTransfer`; token and positive amount required.

<!-- api: exchange.Client.TransferVaultUSD -->
```go
func (c *exchange.Client) TransferVaultUSD(ctx context.Context, request exchange.VaultTransferRequest) (exchange.ActionResponse, error)
```
Protocol action: `vaultTransfer`.

<!-- api: exchange.Client.CreateSubAccount -->
```go
func (c *exchange.Client) CreateSubAccount(ctx context.Context, name string) (exchange.ActionResponse, error)
```
Protocol action: `createSubAccount`; name must be 1–16 characters.

<!-- api: exchange.Client.ModifySubAccount -->
```go
func (c *exchange.Client) ModifySubAccount(ctx context.Context, request exchange.SubAccountModifyRequest) (exchange.ActionResponse, error)
```
Protocol action: `modifySubAccount`.

<!-- api: exchange.Client.CreateVault -->
```go
func (c *exchange.Client) CreateVault(ctx context.Context, request exchange.CreateVaultRequest) (exchange.ActionResponse, error)
```
Protocol action: `createVault`; name/description and at least 100 initial USD are checked. Success data is `CreateVaultResponseData`.

<!-- api: exchange.Client.ModifyVault -->
```go
func (c *exchange.Client) ModifyVault(ctx context.Context, request exchange.VaultModifyRequest) (exchange.ActionResponse, error)
```
Protocol action: `modifyVault`; vault address plus at least one setting required.

<!-- api: exchange.Client.DistributeVault -->
```go
func (c *exchange.Client) DistributeVault(ctx context.Context, request exchange.VaultDistributionRequest) (exchange.ActionResponse, error)
```
Protocol action: `vaultDistribute`.

<!-- api: exchange.Client.SetDisplayName -->
```go
func (c *exchange.Client) SetDisplayName(ctx context.Context, displayName string) (exchange.ActionResponse, error)
```
Protocol action: `setDisplayName`; at most 20 characters.

<!-- api: exchange.Client.ConvertToMultiSigUser -->
```go
func (c *exchange.Client) ConvertToMultiSigUser(ctx context.Context, signers *signing.MultiSigSignerSet) (exchange.ActionResponse, error)
```
Protocol action: `convertToMultiSigUser`; validates signer-set EIP-712 data.

<!-- api: exchange.Client.SubmitMultiSigL1 -->
```go
func (c *exchange.Client) SubmitMultiSigL1(ctx context.Context, config exchange.MultiSigConfig, action signing.L1Action) (exchange.ActionResponse, error)
```
Signs a supplied validated L1 action in the multisig envelope.

<!-- api: exchange.Client.SubmitMultiSigUserAction -->
```go
func (c *exchange.Client) SubmitMultiSigUserAction(ctx context.Context, config exchange.MultiSigConfig, action signing.UserSignedAction) (exchange.ActionResponse, error)
```
Signs a supplied validated EIP-712 user action in the multisig envelope.

## Advanced and specialised actions

These methods retain the same one-shot, validated signing/response contract.
The action identifier shown is the official `action.type`; specialized input
unions are intentionally delegated to their named Go type rather than an
untyped map.

| API | Signature | Action type / restriction |
| --- | --- | --- |
| <!-- api: exchange.Client.AgentSendAsset --> `AgentSendAsset` | `func(context.Context, exchange.AgentSendAssetRequest) (exchange.ActionResponse, error)` | `agentSendAsset`; positive amount, token and destination required; source subaccount is action data, not client vault. |
| <!-- api: exchange.Client.AuthorizeAQAV2Role --> `AuthorizeAQAV2Role` | `func(context.Context, exchange.AuthorizeAQAV2RoleRequest) (exchange.ActionResponse, error)` | `authorizeAqav2Role`; typed technical/treasury role. |
| <!-- api: exchange.Client.HIP3LiquidatorTransfer --> `HIP3LiquidatorTransfer` | `func(context.Context, exchange.HIP3LiquidatorTransferRequest) (exchange.ActionResponse, error)` | `hip3LiquidatorTransfer`; `NTL` is protocol micros and must satisfy its increment rule. |
| <!-- api: exchange.Client.UserOutcome --> `UserOutcome` | `func(context.Context, exchange.UserOutcomeRequest) (exchange.ActionResponse, error)` | `userOutcome`; exactly one typed outcome variant. |
| <!-- api: exchange.Client.UserDexAbstraction --> `UserDexAbstraction` | `func(context.Context, exchange.UserDexAbstractionRequest) (exchange.ActionResponse, error)` | `userDexAbstraction`; user-signed. |
| <!-- api: exchange.Client.UserSetAbstraction --> `UserSetAbstraction` | `func(context.Context, exchange.UserSetAbstractionRequest) (exchange.ActionResponse, error)` | `userSetAbstraction`; user-signed, typed abstraction. |
| <!-- api: exchange.Client.AgentEnableDexAbstraction --> `AgentEnableDexAbstraction` | `func(context.Context) (exchange.ActionResponse, error)` | `agentEnableDexAbstraction`. |
| <!-- api: exchange.Client.AgentSetAbstraction --> `AgentSetAbstraction` | `func(context.Context, exchange.AgentAbstraction) (exchange.ActionResponse, error)` | `agentSetAbstraction`; `i`, `u`, or `p`. |
| <!-- api: exchange.Client.ValidatorL1Stream --> `ValidatorL1Stream` | `func(context.Context, string) (exchange.ActionResponse, error)` | `validatorL1Stream`; canonical positive risk-free-rate string. |
| <!-- api: exchange.Client.ClaimRewards --> `ClaimRewards` | `func(context.Context) (exchange.ActionResponse, error)` | `claimRewards`. |
| <!-- api: exchange.Client.SetReferrer --> `SetReferrer` | `func(context.Context, string) (exchange.ActionResponse, error)` | `setReferrer`; non-empty referral code. |
| <!-- api: exchange.Client.EVMUserModify --> `EVMUserModify` | `func(context.Context, bool) (exchange.ActionResponse, error)` | `evmUserModify`. |
| <!-- api: exchange.Client.UseBigEVMBlocks --> `UseBigEVMBlocks` | `func(context.Context, bool) (exchange.ActionResponse, error)` | `useBigEvmBlocks`. |
| <!-- api: exchange.Client.GossipPriorityBid --> `GossipPriorityBid` | `func(context.Context, uint64, string, uint64) (exchange.ActionResponse, error)` | `gossipPriorityBid`; slot, IP, and gas limit. |
| <!-- api: exchange.Client.SubmitGossipPriorityBid --> `SubmitGossipPriorityBid` | `func(context.Context, uint64, string, uint64) (exchange.ActionResponse, error)` | Alias for `GossipPriorityBid`. |
| <!-- api: exchange.Client.CValidatorAction --> `CValidatorAction` | `func(context.Context, signing.CValidatorVariant) (exchange.ActionResponse, error)` | Typed validator action union. |
| <!-- api: exchange.Client.SubmitCValidatorAction --> `SubmitCValidatorAction` | `func(context.Context, signing.CValidatorVariant) (exchange.ActionResponse, error)` | Alias for `CValidatorAction`. |
| <!-- api: exchange.Client.CSignerAction --> `CSignerAction` | `func(context.Context, signing.CSignerVariant) (exchange.ActionResponse, error)` | Typed signer action union. |
| <!-- api: exchange.Client.FinalizeEVMContract --> `FinalizeEVMContract` | `func(context.Context, uint64, signing.FinalizeEVMContractInput) (exchange.ActionResponse, error)` | `finalizeEvmContract`; token ID and typed input. |
| <!-- api: exchange.Client.SubmitPerpDeploy --> `SubmitPerpDeploy` | `func(context.Context, signing.PerpDeployVariant) (exchange.ActionResponse, error)` | Typed perpetual deployment action union. |
| <!-- api: exchange.Client.SubmitSpotDeploy --> `SubmitSpotDeploy` | `func(context.Context, signing.SpotDeployVariant) (exchange.ActionResponse, error)` | Typed spot deployment action union. |

The advanced variants are protocol-evolving surfaces. Read their Go type
documentation and the linked official endpoint before enabling them in a
production service; this SDK does not silently accept arbitrary maps.
