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

## Advanced actions and compatibility helpers

These methods retain the common signed-submission and no-retry contract above.
The `signing.*Variant` arguments are sealed protocol payload variants; callers
must use a documented variant rather than construct arbitrary wire JSON.

<!-- api: exchange.Client.AgentSendAsset -->
```go
func (c *exchange.Client) AgentSendAsset(ctx context.Context, request exchange.AgentSendAssetRequest) (exchange.ActionResponse, error)
```
Protocol action: `agentSendAsset`; validates the target address, token, and
positive decimal amount. Source/destination DEX routing and source subaccount
are explicit optional action data, not client vault routing.

<!-- api: exchange.Client.AuthorizeAQAV2Role -->
```go
func (c *exchange.Client) AuthorizeAQAV2Role(ctx context.Context, request exchange.AuthorizeAQAV2RoleRequest) (exchange.ActionResponse, error)
```
Protocol action: `authorizeAqav2Role`; submits the typed AQA role delegation
payload through `submitL1WithoutVault`: it has neither an L1 signing-vault
marker nor outer vault routing, while configured `expiresAfter` is retained.

<!-- api: exchange.Client.HIP3LiquidatorTransfer -->
```go
func (c *exchange.Client) HIP3LiquidatorTransfer(ctx context.Context, request exchange.HIP3LiquidatorTransferRequest) (exchange.ActionResponse, error)
```
Protocol action: `hip3LiquidatorTransfer`; moves HIP-3 DEX backstop-liquidity
notional. `DEX` is required and `NTL` must be a positive multiple of
1,000,000,000 protocol micros; `IsDeposit` selects the direction. It uses
`submitL1WithoutVault`, so vault signing/routing is omitted and configured
`expiresAfter` is retained.

<!-- api: exchange.Client.UserOutcome -->
```go
func (c *exchange.Client) UserOutcome(ctx context.Context, request exchange.UserOutcomeRequest) (exchange.ActionResponse, error)
```
L1 action: `userOutcome`; submits exactly one typed split/merge/negate outcome
operation and honors supported vault and `expiresAfter` configuration.

<!-- api: exchange.Client.UserDexAbstraction -->
```go
func (c *exchange.Client) UserDexAbstraction(ctx context.Context, request exchange.UserDexAbstractionRequest) (exchange.ActionResponse, error)
```
User-signed action: `userDexAbstraction`; applies the typed user DEX
abstraction setting without an L1 expiry.

<!-- api: exchange.Client.UserSetAbstraction -->
```go
func (c *exchange.Client) UserSetAbstraction(ctx context.Context, request exchange.UserSetAbstractionRequest) (exchange.ActionResponse, error)
```
User-signed action: `userSetAbstraction`; updates the typed account abstraction
mode and does not use `expiresAfter`.

<!-- api: exchange.Client.AgentEnableDexAbstraction -->
```go
func (c *exchange.Client) AgentEnableDexAbstraction(ctx context.Context) (exchange.ActionResponse, error)
```
Protocol action: `agentEnableDexAbstraction`; enables DEX abstraction for the
configured agent signer.

<!-- api: exchange.Client.AgentSetAbstraction -->
```go
func (c *exchange.Client) AgentSetAbstraction(ctx context.Context, abstraction exchange.AgentAbstraction) (exchange.ActionResponse, error)
```
Protocol action: `agentSetAbstraction`; `abstraction` is a typed supported
agent abstraction value.

<!-- api: exchange.Client.ValidatorL1Stream -->
```go
func (c *exchange.Client) ValidatorL1Stream(ctx context.Context, riskFreeRate string) (exchange.ActionResponse, error)
```
Protocol action: `validatorL1Stream`; submits the canonical protocol
`riskFreeRate` string.

<!-- api: exchange.Client.ClaimRewards -->
```go
func (c *exchange.Client) ClaimRewards(ctx context.Context) (exchange.ActionResponse, error)
```
Protocol action: `claimRewards`; has no action-specific caller parameters.

<!-- api: exchange.Client.SetReferrer -->
```go
func (c *exchange.Client) SetReferrer(ctx context.Context, code string) (exchange.ActionResponse, error)
```
Protocol action: `setReferrer`; `code` must satisfy the SDK's non-empty
referral-code validation.

<!-- api: exchange.Client.EVMUserModify -->
```go
func (c *exchange.Client) EVMUserModify(ctx context.Context, enabled bool) (exchange.ActionResponse, error)
```
Protocol action: `evmUserModify`; `enabled` is the explicit EVM-user flag.

<!-- api: exchange.Client.UseBigEVMBlocks -->
```go
func (c *exchange.Client) UseBigEVMBlocks(ctx context.Context, enabled bool) (exchange.ActionResponse, error)
```
Compatibility alias for `EVMUserModify`; it submits the `evmUserModify` action
with the same explicit block-mode flag.

<!-- api: exchange.Client.GossipPriorityBid -->
```go
func (c *exchange.Client) GossipPriorityBid(ctx context.Context, slotID uint64, ip string, maxGas uint64) (exchange.ActionResponse, error)
```
Protocol action: `gossipPriorityBid`; submits the typed slot/IP/max-gas bid.

<!-- api: exchange.Client.SubmitGossipPriorityBid -->
```go
func (c *exchange.Client) SubmitGossipPriorityBid(ctx context.Context, slotID uint64, ip string, maxGas uint64) (exchange.ActionResponse, error)
```
Compatibility alias for `GossipPriorityBid`; it submits the same protocol
action and validation.

<!-- api: exchange.Client.CValidatorAction -->
```go
func (c *exchange.Client) CValidatorAction(ctx context.Context, variant signing.CValidatorVariant) (exchange.ActionResponse, error)
```
Protocol action: `CValidatorAction`; a sealed non-nil typed variant selects
the validator action. Its L1 signing-vault marker is nil while configured
expiry and outer vault routing remain part of the envelope.

<!-- api: exchange.Client.SubmitCValidatorAction -->
```go
func (c *exchange.Client) SubmitCValidatorAction(ctx context.Context, variant signing.CValidatorVariant) (exchange.ActionResponse, error)
```
Compatibility alias for `CValidatorAction` with the same typed variant.

<!-- api: exchange.Client.CSignerAction -->
```go
func (c *exchange.Client) CSignerAction(ctx context.Context, variant signing.CSignerVariant) (exchange.ActionResponse, error)
```
Protocol action: `CSignerAction`; a sealed non-nil typed variant selects the
signer-management action. It uses the same nil L1 signing-vault marker with
configured expiry and outer vault routing retained in the envelope.

<!-- api: exchange.Client.FinalizeEVMContract -->
```go
func (c *exchange.Client) FinalizeEVMContract(ctx context.Context, token uint64, input signing.FinalizeEVMContractInput) (exchange.ActionResponse, error)
```
Protocol action: `finalizeEvmContract`; submits the token ID and typed contract
finalization input through `submitL1WithoutVault`, omitting vault
signing/routing while retaining configured `expiresAfter`.

<!-- api: exchange.Client.SubmitPerpDeploy -->
```go
func (c *exchange.Client) SubmitPerpDeploy(ctx context.Context, variant signing.PerpDeployVariant) (exchange.ActionResponse, error)
```
Protocol action: `perpDeploy`; a required sealed typed variant selects a
permitted HIP-3 deployment action. It uses nil L1 signing-vault selection and
retains configured expiry and outer vault routing.

<!-- api: exchange.Client.SubmitSpotDeploy -->
```go
func (c *exchange.Client) SubmitSpotDeploy(ctx context.Context, variant signing.SpotDeployVariant) (exchange.ActionResponse, error)
```
Protocol action: `spotDeploy`; a required sealed typed variant selects a
permitted HIP-1/HIP-2 deployment action. It uses nil L1 signing-vault
selection and retains configured expiry and outer vault routing.

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
