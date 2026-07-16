//go:build integration && testnet

package integration

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strings"
	"testing"
	"time"

	hyperliquid "github.com/Apexllcc/hyperliquid-go-sdk"
	"github.com/Apexllcc/hyperliquid-go-sdk/asset"
	"github.com/Apexllcc/hyperliquid-go-sdk/exchange"
	"github.com/Apexllcc/hyperliquid-go-sdk/info"
	"github.com/Apexllcc/hyperliquid-go-sdk/signer"
	"github.com/Apexllcc/hyperliquid-go-sdk/types"
	"github.com/ethereum/go-ethereum/common"
	"github.com/shopspring/decimal"
)

const (
	testnetTradeEnableEnv    = "HL_TESTNET_TRADE"
	testnetPrivateKeyEnv     = "HL_TESTNET_PRIVATE_KEY"
	testnetUnifiedTradeEnv   = "HL_TESTNET_UNIFIED_TRADE"
	testnetIsolatedTradeEnv  = "HL_TESTNET_ISOLATED_TRADE"
	testnetTransferEnableEnv = "HL_TESTNET_TRANSFER"
	testnetBTC               = "BTC"
	testnetWorkflowTimeout   = 2 * time.Minute
	testnetStateWaitTimeout  = 15 * time.Second
)

var (
	testNotionalUSD        = decimal.NewFromInt(11)
	maxMarginUSD           = decimal.NewFromInt(10)
	worstNewMargin         = decimal.NewFromInt(4) // 11 USDC notional at 3x, rounded up.
	isolatedMarginUSD      = decimal.NewFromInt(1)
	minimumMarginIncrease  = decimal.RequireFromString("0.99")
	testnetTransferUSD     = decimal.NewFromInt(1)
	worstIsolatedNewMargin = worstNewMargin.Add(isolatedMarginUSD)
	half                   = decimal.RequireFromString("0.50")
	marketPremium          = decimal.RequireFromString("1.005")
	marketDiscount         = decimal.RequireFromString("0.995")
	takeProfitFactor       = decimal.RequireFromString("1.02")
	stopLossFactor         = decimal.RequireFromString("0.98")
)

var testnetTransferRecipient = common.HexToAddress("0xAc8a05B375722aa3651881197c1E5Dd109645B91")

// TestTestnetBTCTradingWorkflow is deliberately difficult to enable: it is
// compiled only with integration,testnet tags and requires an explicit trading
// acknowledgement plus a Testnet key. It never targets mainnet. Every action
// uses a fresh CLOID and the cleanup path cancels test orders and closes the
// position with reduce-only IOC before returning.
func TestTestnetBTCTradingWorkflow(t *testing.T) {
	signingKey := requireTradingTestnet(t)
	ctx, cancel := context.WithTimeout(context.Background(), testnetWorkflowTimeout)
	defer cancel()

	metadataClient, err := hyperliquid.NewClient(hyperliquid.WithTestnet(), hyperliquid.WithHTTPTimeout(10*time.Second))
	if err != nil {
		t.Fatalf("new testnet metadata client: %v", err)
	}
	defer func() { _ = metadataClient.Close() }()
	meta, err := metadataClient.Info.Meta(ctx)
	if err != nil {
		t.Fatalf("read testnet perp metadata: %v", err)
	}
	btcAsset, err := testnetBasePerpAsset(meta, testnetBTC)
	if err != nil {
		t.Fatal(err)
	}
	client, err := hyperliquid.NewClient(
		hyperliquid.WithTestnet(),
		hyperliquid.WithDigestSigner(signingKey),
		// Resolve BTC from the Testnet base-perp metadata, then retain only this
		// exact asset to avoid an unrelated HIP-3 metadata fan-out before orders.
		hyperliquid.WithAssetResolver(asset.NewStaticResolver([]asset.Asset{btcAsset})),
		hyperliquid.WithHTTPTimeout(10*time.Second),
	)
	if err != nil {
		t.Fatalf("new testnet client: %v", err)
	}
	defer func() { _ = client.Close() }()

	address := signingKey.Address().Hex()

	abstraction := preflightTradingAccount(t, ctx, client, address, worstNewMargin)
	t.Logf("testnet BTC workflow uses %s collateral model", abstraction)
	requireUnifiedTradingAcknowledgement(t, abstraction)
	if usesSpotCollateral(abstraction) {
		preflightBTCPosition(t, ctx, client, address)
	}
	openOrders, err := client.Info.OpenOrders(ctx, address)
	if err != nil {
		t.Fatalf("read testnet open orders: %v", err)
	}
	for _, order := range openOrders {
		if order.Coin == testnetBTC {
			t.Skip("testnet account already has a BTC open order; no order submitted")
		}
	}
	if _, err := client.Info.UserFills(ctx, address, false); err != nil {
		t.Fatalf("read testnet fills: %v", err)
	}

	mids, err := client.Info.AllMids(ctx)
	if err != nil {
		t.Fatalf("read testnet BTC mid: %v", err)
	}
	mid, ok := mids[testnetBTC]
	if !ok || !mid.IsPositive() {
		t.Skip("testnet BTC mid is unavailable; no order submitted")
	}

	previousAssetData, err := client.Info.ActiveAssetData(ctx, address, testnetBTC)
	if err != nil {
		t.Skipf("cannot read existing BTC leverage; no account mutation submitted: %v", err)
	}
	previousLeverage := previousAssetData.Leverage
	if previousLeverage.Value <= 0 || (previousLeverage.Type != "cross" && previousLeverage.Type != "isolated") {
		t.Skip("testnet BTC leverage response is invalid; no account mutation submitted")
	}
	leverageMayHaveChanged := true // Set before submission: a transport error has an unknown server outcome.
	defer func() {
		if !leverageMayHaveChanged {
			return
		}
		cleanupCtx, cleanupCancel := cleanupContext()
		defer cleanupCancel()
		if err := setAndConfirmBTCLeverage(cleanupCtx, client, address, abstraction, previousLeverage.Type == "cross", uint64(previousLeverage.Value)); err != nil {
			t.Errorf("restore BTC leverage after testnet validation: %v", err)
		}
	}()
	if err := setAndConfirmBTCLeverage(ctx, client, address, abstraction, true, 3); err != nil {
		t.Fatalf("set BTC 3x leverage: %v", err)
	}
	t.Log("verified BTC 3x leverage")

	limitPrice := significantPrice(mid.Mul(half), btcAsset.SzDecimals, false)
	limitSize := sizeForNotional(t, limitPrice, btcAsset.SzDecimals)
	limitCloid := newCloid(t)
	modifiedLimitCloid := newCloid(t)
	batchedLimitCloid := newCloid(t)
	limitMayBeOpen := true // Register cleanup before submission to cover ambiguous transport failures.
	defer func() {
		if !limitMayBeOpen {
			return
		}
		for _, cloid := range []types.Cloid{limitCloid, modifiedLimitCloid, batchedLimitCloid} {
			cancelCtx, cancelCancel := cleanupContext()
			if err := cancelAndConfirmBTCOrder(cancelCtx, client, address, cloid); err != nil {
				t.Errorf("cleanup BTC limit order: %v", err)
			}
			cancelCancel()
		}
		closeCtx, closeCancel := cleanupContext()
		defer closeCancel()
		if err := closeAndConfirmBTCPosition(closeCtx, client, address, btcAsset.SzDecimals); err != nil {
			t.Errorf("cleanup BTC limit order position: %v", err)
		}
	}()
	limitResponse, err := client.Exchange.PlaceOrder(ctx, exchange.OrderRequest{
		Coin: testnetBTC, IsBuy: true, Price: limitPrice, Size: limitSize,
		Type: exchange.LimitOrder{TimeInForce: exchange.TIFGTC}, ClientOrderID: &limitCloid,
	})
	if err != nil {
		t.Fatalf("place BTC limit order: %v", err)
	}
	if err := requireAcceptedOrders(limitResponse, 1); err != nil {
		t.Fatalf("BTC limit order was rejected: %v", err)
	}
	assertCloidOrderVisible(t, ctx, client, address, limitCloid)
	limitStatus, err := client.Info.OrderStatusByCloid(ctx, address, limitCloid)
	if err != nil || limitStatus.Order == nil {
		t.Fatalf("read BTC limit order by CLOID: %v", err)
	}
	if limitStatus.Order.Cloid == nil || *limitStatus.Order.Cloid != limitCloid.String() {
		t.Fatal("BTC limit order CLOID response does not match the submitted CLOID")
	}
	limitOIDStatus, err := client.Info.OrderStatus(ctx, address, limitStatus.Order.OID)
	if err != nil || limitOIDStatus.Order == nil || limitOIDStatus.Order.OID != limitStatus.Order.OID {
		t.Fatalf("read BTC limit order by OID: %v", err)
	}
	modifiedPrice := significantPrice(limitPrice.Mul(marketDiscount), btcAsset.SzDecimals, false)
	modifyResponse, err := client.Exchange.ModifyOrder(ctx, exchange.ModifyRequest{
		OID: limitStatus.Order.OID,
		Order: exchange.OrderRequest{
			Coin: testnetBTC, IsBuy: true, Price: modifiedPrice, Size: limitSize,
			Type: exchange.LimitOrder{TimeInForce: exchange.TIFGTC}, ClientOrderID: &modifiedLimitCloid,
		},
	})
	if err != nil {
		t.Fatalf("modify BTC limit order: %v", err)
	}
	if err := requireAcceptedOrders(modifyResponse, 1); err != nil {
		t.Fatalf("BTC limit modification was rejected: %v", err)
	}
	assertCloidOrderVisible(t, ctx, client, address, modifiedLimitCloid)
	modifiedStatus, err := client.Info.OrderStatusByCloid(ctx, address, modifiedLimitCloid)
	if err != nil || modifiedStatus.Order == nil || modifiedStatus.Order.Cloid == nil || *modifiedStatus.Order.Cloid != modifiedLimitCloid.String() {
		t.Fatalf("read modified BTC limit order by CLOID: %v", err)
	}
	assertOrderNotOpen(t, ctx, client, address, limitStatus.Order.OID)
	batchPrice := significantPrice(modifiedPrice.Mul(marketDiscount), btcAsset.SzDecimals, false)
	batchResponse, err := client.Exchange.BatchModify(ctx, []exchange.ModifyRequest{{
		Cloid: &modifiedLimitCloid,
		Order: exchange.OrderRequest{
			Coin: testnetBTC, IsBuy: true, Price: batchPrice, Size: limitSize,
			Type: exchange.LimitOrder{TimeInForce: exchange.TIFGTC}, ClientOrderID: &batchedLimitCloid,
		},
	}})
	if err != nil {
		t.Fatalf("batch modify BTC limit order: %v", err)
	}
	if err := requireAcceptedOrders(batchResponse, 1); err != nil {
		t.Fatalf("BTC batch modification was rejected: %v", err)
	}
	assertCloidOrderVisible(t, ctx, client, address, batchedLimitCloid)
	batchedStatus, err := client.Info.OrderStatusByCloid(ctx, address, batchedLimitCloid)
	if err != nil || batchedStatus.Order == nil {
		t.Fatalf("read batch-modified BTC limit order: %v", err)
	}
	if batchedStatus.Order.Cloid == nil || *batchedStatus.Order.Cloid != batchedLimitCloid.String() {
		t.Fatal("batch-modified BTC limit order CLOID does not match the submitted CLOID")
	}
	assertOrderNotOpen(t, ctx, client, address, modifiedStatus.Order.OID)
	if err := cancelAndConfirmBTCOrderOID(ctx, client, address, batchedStatus.Order.OID); err != nil {
		t.Fatalf("cancel BTC limit order by OID: %v", err)
	}
	if err := closeAndConfirmBTCPosition(ctx, client, address, btcAsset.SzDecimals); err != nil {
		t.Fatalf("close BTC position from limit order: %v", err)
	}
	limitMayBeOpen = false
	t.Log("verified OID/CLOID status, modify, batch modify, and numeric cancel")

	marketPrice := significantPrice(mid.Mul(marketPremium), btcAsset.SzDecimals, true)
	marketSize := sizeForNotional(t, marketPrice, btcAsset.SzDecimals)
	marketCloid := newCloid(t)
	positionMayBeOpen := true // Reduce-only cleanup is armed before a possibly ambiguous submission.
	defer func() {
		if !positionMayBeOpen {
			return
		}
		cleanupCtx, cleanupCancel := cleanupContext()
		defer cleanupCancel()
		if err := closeAndConfirmBTCPosition(cleanupCtx, client, address, btcAsset.SzDecimals); err != nil {
			t.Errorf("cleanup BTC testnet position: %v", err)
		}
	}()
	marketResponse, err := client.Exchange.PlaceOrder(ctx, exchange.OrderRequest{
		Coin: testnetBTC, IsBuy: true, Price: marketPrice, Size: marketSize,
		Type: exchange.LimitOrder{TimeInForce: exchange.TIFIOC}, ClientOrderID: &marketCloid,
	})
	if err != nil {
		t.Fatalf("place BTC market IOC: %v", err)
	}
	if err := requireAcceptedOrders(marketResponse, 1); err != nil {
		t.Fatalf("BTC market IOC was rejected: %v", err)
	}
	waitForBTCPosition(t, ctx, client, address, marketSize, true)
	assertBTCPositionLeverage(t, ctx, client, address, "cross", 3)
	assertMarginLimit(t, ctx, client, address, abstraction)
	t.Log("verified BTC IOC order and margin safety limit")

	tpCloid := newCloid(t)
	slCloid := newCloid(t)
	triggersMayBeOpen := true // Register before submission for the same unknown-outcome safety rule.
	defer func() {
		if !triggersMayBeOpen {
			return
		}
		for _, cloid := range []types.Cloid{tpCloid, slCloid} {
			cleanupCtx, cleanupCancel := cleanupContext()
			if err := cancelAndConfirmBTCOrder(cleanupCtx, client, address, cloid); err != nil {
				t.Errorf("cleanup BTC TP/SL: %v", err)
			}
			cleanupCancel()
		}
	}()
	triggerPrice := significantPrice(mid.Mul(marketDiscount), btcAsset.SzDecimals, false)
	triggerResponse, err := client.Exchange.PlaceOrders(ctx, []exchange.OrderRequest{
		{
			Coin: testnetBTC, IsBuy: false, Price: triggerPrice, Size: marketSize, ReduceOnly: true,
			Type: exchange.TriggerOrder{IsMarket: true, TriggerPrice: significantPrice(mid.Mul(takeProfitFactor), btcAsset.SzDecimals, true), TPSL: exchange.TPSLTakeProfit}, ClientOrderID: &tpCloid,
		},
		{
			Coin: testnetBTC, IsBuy: false, Price: triggerPrice, Size: marketSize, ReduceOnly: true,
			Type: exchange.TriggerOrder{IsMarket: true, TriggerPrice: significantPrice(mid.Mul(stopLossFactor), btcAsset.SzDecimals, false), TPSL: exchange.TPSLStopLoss}, ClientOrderID: &slCloid,
		},
	})
	if err != nil {
		t.Fatalf("place BTC TP/SL: %v", err)
	}
	if err := requireAcceptedOrders(triggerResponse, 2); err != nil {
		t.Fatalf("BTC TP/SL was rejected: %v", err)
	}
	assertCloidOrderVisible(t, ctx, client, address, tpCloid)
	assertCloidOrderVisible(t, ctx, client, address, slCloid)
	for _, cloid := range []types.Cloid{tpCloid, slCloid} {
		if err := cancelAndConfirmBTCOrder(ctx, client, address, cloid); err != nil {
			t.Fatalf("cancel BTC TP/SL: %v", err)
		}
	}
	triggersMayBeOpen = false
	t.Log("verified and canceled BTC TP/SL orders")

	if err := closeAndConfirmBTCPosition(ctx, client, address, btcAsset.SzDecimals); err != nil {
		t.Fatalf("close BTC testnet position: %v", err)
	}
	positionMayBeOpen = false
	t.Log("verified reduce-only BTC position close")
}

// TestTestnetBTCIsolatedWorkflow is an independently acknowledged Testnet
// workflow for isolated margin. It opens a small BTC position at isolated 3x,
// adjusts exactly one USDC of isolated margin, verifies the position state,
// then closes it reduce-only and restores the account's original leverage.
func TestTestnetBTCIsolatedWorkflow(t *testing.T) {
	signingKey := requireTradingTestnet(t)
	requireIsolatedTradingAcknowledgement(t)
	ctx, cancel := context.WithTimeout(context.Background(), testnetWorkflowTimeout)
	defer cancel()

	metadataClient, err := hyperliquid.NewClient(hyperliquid.WithTestnet(), hyperliquid.WithHTTPTimeout(10*time.Second))
	if err != nil {
		t.Fatalf("new testnet metadata client: %v", err)
	}
	defer func() { _ = metadataClient.Close() }()
	meta, err := metadataClient.Info.Meta(ctx)
	if err != nil {
		t.Fatalf("read testnet perp metadata: %v", err)
	}
	btcAsset, err := testnetBasePerpAsset(meta, testnetBTC)
	if err != nil {
		t.Fatal(err)
	}
	client, err := hyperliquid.NewClient(
		hyperliquid.WithTestnet(),
		hyperliquid.WithDigestSigner(signingKey),
		hyperliquid.WithAssetResolver(asset.NewStaticResolver([]asset.Asset{btcAsset})),
		hyperliquid.WithHTTPTimeout(10*time.Second),
	)
	if err != nil {
		t.Fatalf("new testnet client: %v", err)
	}
	defer func() { _ = client.Close() }()

	address := signingKey.Address().Hex()
	abstraction := preflightTradingAccount(t, ctx, client, address, worstIsolatedNewMargin)
	t.Logf("testnet BTC isolated workflow uses %s collateral model", abstraction)
	requireUnifiedTradingAcknowledgement(t, abstraction)
	if usesSpotCollateral(abstraction) {
		preflightBTCPosition(t, ctx, client, address)
	}
	openOrders, err := client.Info.OpenOrders(ctx, address)
	if err != nil {
		t.Fatalf("read testnet open orders: %v", err)
	}
	for _, order := range openOrders {
		if order.Coin == testnetBTC {
			t.Skip("testnet account already has a BTC open order; no order submitted")
		}
	}

	previousAssetData, err := client.Info.ActiveAssetData(ctx, address, testnetBTC)
	if err != nil {
		t.Skipf("cannot read existing BTC leverage; no account mutation submitted: %v", err)
	}
	previousLeverage := previousAssetData.Leverage
	if previousLeverage.Value <= 0 || (previousLeverage.Type != "cross" && previousLeverage.Type != "isolated") {
		t.Skip("testnet BTC leverage response is invalid; no account mutation submitted")
	}
	leverageMayHaveChanged := true // Submission failures can still have reached the server.
	defer func() {
		if !leverageMayHaveChanged {
			return
		}
		cleanupCtx, cleanupCancel := cleanupContext()
		defer cleanupCancel()
		if err := setAndConfirmBTCLeverage(cleanupCtx, client, address, abstraction, previousLeverage.Type == "cross", uint64(previousLeverage.Value)); err != nil {
			t.Errorf("restore BTC leverage after isolated validation: %v", err)
		}
	}()
	if err := setAndConfirmBTCLeverage(ctx, client, address, abstraction, false, 3); err != nil {
		t.Fatalf("set BTC isolated 3x leverage: %v", err)
	}

	mids, err := client.Info.AllMids(ctx)
	if err != nil {
		t.Fatalf("read testnet BTC mid: %v", err)
	}
	mid, ok := mids[testnetBTC]
	if !ok || !mid.IsPositive() {
		t.Skip("testnet BTC mid is unavailable; no order submitted")
	}
	marketPrice := significantPrice(mid.Mul(marketPremium), btcAsset.SzDecimals, true)
	marketSize := sizeForNotional(t, marketPrice, btcAsset.SzDecimals)
	positionMayBeOpen := true // Arm reduce-only cleanup before the submit call.
	defer func() {
		if !positionMayBeOpen {
			return
		}
		cleanupCtx, cleanupCancel := cleanupContext()
		defer cleanupCancel()
		if err := closeAndConfirmBTCPosition(cleanupCtx, client, address, btcAsset.SzDecimals); err != nil {
			t.Errorf("cleanup isolated BTC testnet position: %v", err)
		}
	}()
	response, err := client.Exchange.PlaceOrder(ctx, exchange.OrderRequest{
		Coin: testnetBTC, IsBuy: true, Price: marketPrice, Size: marketSize,
		Type: exchange.LimitOrder{TimeInForce: exchange.TIFIOC},
	})
	if err != nil {
		t.Fatalf("place isolated BTC market IOC: %v", err)
	}
	if err := requireAcceptedOrders(response, 1); err != nil {
		t.Fatalf("isolated BTC market IOC was rejected: %v", err)
	}
	waitForBTCPosition(t, ctx, client, address, marketSize, true)
	assertBTCPositionLeverage(t, ctx, client, address, "isolated", 3)
	assertMarginLimit(t, ctx, client, address, abstraction)
	marginBefore, err := currentBTCIsolatedRawUSD(ctx, client, address)
	if err != nil {
		t.Fatalf("read isolated BTC raw USD before adjustment: %v", err)
	}
	var (
		spotHoldBefore     decimal.Decimal
		haveSpotHoldBefore bool
	)
	if usesSpotCollateral(abstraction) {
		spotHoldBefore, err = currentUSDCSpotHold(ctx, client, address)
		if err != nil {
			t.Logf("read unified USDC hold before isolated BTC margin adjustment: %v", err)
		} else {
			haveSpotHoldBefore = true
		}
	}

	marginResponse, err := client.Exchange.UpdateIsolatedMargin(ctx, exchange.UpdateIsolatedMarginRequest{
		Coin: testnetBTC, IsBuy: true, Amount: isolatedMarginUSD,
	})
	if err != nil {
		t.Fatalf("add isolated BTC margin: %v", err)
	}
	if _, ok := marginResponse.Response.Data.(exchange.DefaultActionResponseData); !ok || marginResponse.Response.Type != exchange.ActionResponseDefault {
		t.Fatalf("unexpected isolated-margin response type %q", marginResponse.Response.Type)
	}
	if err := waitForBTCIsolatedRawUSDIncrease(ctx, client, address, marginBefore, minimumMarginIncrease); err != nil {
		t.Fatalf("confirm isolated BTC margin adjustment: %v", err)
	}
	if haveSpotHoldBefore {
		spotHoldAfter, holdErr := currentUSDCSpotHold(ctx, client, address)
		if holdErr != nil {
			t.Logf("read unified USDC hold after isolated BTC margin adjustment: %v", holdErr)
		} else {
			t.Logf("unified USDC hold changed by %s after isolated BTC margin adjustment", spotHoldAfter.Sub(spotHoldBefore))
		}
	}
	assertBTCPositionLeverage(t, ctx, client, address, "isolated", 3)
	assertMarginLimit(t, ctx, client, address, abstraction)
	t.Log("verified isolated BTC 3x IOC order and one-USDC margin adjustment")

	if err := closeAndConfirmBTCPosition(ctx, client, address, btcAsset.SzDecimals); err != nil {
		t.Fatalf("close isolated BTC testnet position: %v", err)
	}
	positionMayBeOpen = false
	t.Log("verified isolated BTC reduce-only position close")
}

// TestTestnetUSDSendWorkflow sends exactly one Testnet Core USDC to the
// recipient explicitly supplied by the user. It is intentionally one-way and
// independently gated because the recipient is outside this test's control.
func TestTestnetUSDSendWorkflow(t *testing.T) {
	signingKey := requireTradingTestnet(t)
	if os.Getenv(testnetTransferEnableEnv) != "1" {
		t.Skip("set HL_TESTNET_TRANSFER=1 to enable the one-way Testnet USDC transfer validation")
	}
	ctx, cancel := context.WithTimeout(context.Background(), testnetWorkflowTimeout)
	defer cancel()
	client, err := hyperliquid.NewClient(
		hyperliquid.WithTestnet(),
		hyperliquid.WithDigestSigner(signingKey),
		hyperliquid.WithHTTPTimeout(10*time.Second),
	)
	if err != nil {
		t.Fatalf("new testnet transfer client: %v", err)
	}
	defer func() { _ = client.Close() }()

	sender := signingKey.Address().Hex()
	recipient := testnetTransferRecipient.Hex()
	senderAbstraction, err := client.Info.UserAbstraction(ctx, sender)
	if err != nil {
		t.Fatalf("read Testnet sender account abstraction: %v", err)
	}
	if !usesSpotCollateral(senderAbstraction) {
		t.Skipf("Testnet USD transfer workflow currently requires Unified/Portfolio sender collateral, got %s", senderAbstraction)
	}
	senderState, err := client.Info.SpotClearinghouseState(ctx, sender)
	if err != nil {
		t.Fatalf("read Testnet sender spot state: %v", err)
	}
	senderUSDC, err := usdcSpotBalance(senderState)
	if err != nil {
		t.Fatal(err)
	}
	if senderUSDC.Total.Sub(senderUSDC.Hold).LessThan(testnetTransferUSD) {
		t.Skipf("Testnet sender has insufficient available USDC for one-USDC transfer: %s", senderUSDC.Total.Sub(senderUSDC.Hold))
	}
	ledgerSnapshotStart := time.Now().Add(-15 * time.Minute).UnixMilli()
	knownTransferHashes, err := existingUSDSendLedgerHashes(ctx, client, sender, recipient, ledgerSnapshotStart, testnetTransferUSD)
	if err != nil {
		t.Fatalf("snapshot prior Testnet usdSend ledger entries: %v", err)
	}
	ledgerStart := time.Now().Add(-time.Second).UnixMilli()
	response, err := client.Exchange.SendUSD(ctx, exchange.USDSendRequest{
		Destination: testnetTransferRecipient,
		Amount:      testnetTransferUSD,
	})
	if err != nil {
		if isExpectedUSDSendUnifiedModeRejection(err) {
			t.Skipf("Testnet usdSend is disabled for this Unified account: %v", err)
		}
		var actionErr *exchange.ActionResponseError
		if errors.As(err, &actionErr) {
			t.Fatalf("Testnet usdSend was definitively rejected by the exchange: %v", actionErr)
		}
		reconcileCtx, reconcileCancel := cleanupContext()
		defer reconcileCancel()
		if reconcileErr := waitForUSDSendLedger(reconcileCtx, client, sender, recipient, ledgerStart, testnetTransferUSD, knownTransferHashes); reconcileErr == nil {
			t.Log("Testnet usdSend response was lost but sender ledger confirms the transfer")
			return
		} else {
			t.Fatalf("Testnet usdSend outcome is unknown; do not retry automatically: %v; reconciliation: %v", err, reconcileErr)
		}
	}
	if response.Status != "ok" || response.Response.Type != "usdSend" {
		reconcileCtx, reconcileCancel := cleanupContext()
		defer reconcileCancel()
		if reconcileErr := waitForUSDSendLedger(reconcileCtx, client, sender, recipient, ledgerStart, testnetTransferUSD, knownTransferHashes); reconcileErr == nil {
			t.Logf("Testnet usdSend returned an unexpected envelope but sender ledger confirms the transfer (status=%q type=%q)", response.Status, response.Response.Type)
			return
		} else {
			t.Fatalf("Testnet usdSend outcome is unknown; do not retry automatically: unexpected status=%q type=%q; reconciliation: %v", response.Status, response.Response.Type, reconcileErr)
		}
	}
	if err := waitForUSDSendLedger(ctx, client, sender, recipient, ledgerStart, testnetTransferUSD, knownTransferHashes); err != nil {
		t.Fatal(err)
	}
	if recipientState, recipientErr := client.Info.SpotClearinghouseState(ctx, recipient); recipientErr != nil {
		t.Logf("read Testnet recipient spot state after usdSend: %v", recipientErr)
	} else if balance, found := usdcSpotBalanceOrZero(recipientState); found {
		t.Logf("recipient Testnet spot USDC total after usdSend: %s", balance.Total)
	}
	t.Logf("verified one Testnet Core USDC transfer to %s", recipient)
}

func isExpectedUSDSendUnifiedModeRejection(err error) bool {
	var actionErr *exchange.ActionResponseError
	return errors.As(err, &actionErr) && actionErr.Message == "Action disabled when unified account is active"
}

func TestIsExpectedUSDSendUnifiedModeRejection(t *testing.T) {
	tests := []struct {
		name string
		err  error
		want bool
	}{
		{
			name: "exact unified-mode rejection",
			err:  &exchange.ActionResponseError{Status: "err", Message: "Action disabled when unified account is active"},
			want: true,
		},
		{
			name: "wrapped unified-mode rejection",
			err:  fmt.Errorf("submit: %w", &exchange.ActionResponseError{Status: "err", Message: "Action disabled when unified account is active"}),
			want: true,
		},
		{
			name: "other protocol rejection",
			err:  &exchange.ActionResponseError{Status: "err", Message: "Insufficient margin"},
			want: false,
		},
		{
			name: "transport error",
			err:  errors.New("connection reset"),
			want: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := isExpectedUSDSendUnifiedModeRejection(tt.err); got != tt.want {
				t.Fatalf("isExpectedUSDSendUnifiedModeRejection(%v) = %t, want %t", tt.err, got, tt.want)
			}
		})
	}
}

func testnetBasePerpAsset(meta info.MetaResponse, symbol string) (asset.Asset, error) {
	for id, candidate := range meta.Universe {
		if candidate.Name == symbol {
			return asset.Asset{ID: id, Symbol: symbol, Name: symbol, Kind: asset.Perp, SzDecimals: candidate.SzDecimals}, nil
		}
	}
	return asset.Asset{}, fmt.Errorf("testnet base perp %q is unavailable", symbol)
}

func usdcSpotBalance(state info.SpotClearinghouseStateResponse) (info.SpotBalance, error) {
	for _, balance := range state.Balances {
		if balance.Coin == "USDC" {
			return balance, nil
		}
	}
	return info.SpotBalance{}, fmt.Errorf("account has no USDC spot balance")
}

func usdcSpotBalanceOrZero(state info.SpotClearinghouseStateResponse) (info.SpotBalance, bool) {
	balance, err := usdcSpotBalance(state)
	return balance, err == nil
}

func existingUSDSendLedgerHashes(ctx context.Context, client *hyperliquid.Client, sender, recipient string, startTime int64, amount decimal.Decimal) (map[string]struct{}, error) {
	updates, err := client.Info.UserNonFundingLedgerUpdates(ctx, sender, startTime, nil)
	if err != nil {
		return nil, err
	}
	known := make(map[string]struct{})
	for _, update := range updates {
		if isMatchingUSDSendLedger(update, recipient, amount) && update.Hash != "" {
			known[update.Hash] = struct{}{}
		}
	}
	return known, nil
}

func waitForUSDSendLedger(ctx context.Context, client *hyperliquid.Client, sender, recipient string, startTime int64, amount decimal.Decimal, known map[string]struct{}) error {
	deadline := time.Now().Add(testnetStateWaitTimeout)
	var latestErr error
	for time.Now().Before(deadline) {
		updates, err := client.Info.UserNonFundingLedgerUpdates(ctx, sender, startTime, nil)
		if err == nil {
			for _, update := range updates {
				if isMatchingUSDSendLedger(update, recipient, amount) && update.Hash != "" {
					if _, alreadyKnown := known[update.Hash]; alreadyKnown {
						continue
					}
					return nil
				}
			}
		} else {
			latestErr = err
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(100 * time.Millisecond):
		}
	}
	if latestErr != nil {
		return fmt.Errorf("sender ledger did not confirm usdSend of %s to %s: %v", amount, recipient, latestErr)
	}
	return fmt.Errorf("sender ledger did not confirm usdSend of %s to %s", amount, recipient)
}

func isMatchingUSDSendLedger(update info.NonFundingLedgerUpdate, recipient string, amount decimal.Decimal) bool {
	return update.Delta.Type == "usdSend" && strings.EqualFold(update.Delta.Destination, recipient) && (update.Delta.USDC.Abs().Equal(amount) || update.Delta.Amount.Abs().Equal(amount))
}

func requireTradingTestnet(t *testing.T) *signer.LocalPrivateKeySigner {
	t.Helper()
	if os.Getenv(testnetTradeEnableEnv) != "1" {
		t.Skip("set HL_TESTNET_TRADE=1 to enable Testnet trading validation")
	}
	key := os.Getenv(testnetPrivateKeyEnv)
	if key == "" {
		t.Skip("set HL_TESTNET_PRIVATE_KEY to a Testnet trading key")
	}
	s, err := signer.NewLocalPrivateKeySignerFromHex(key)
	if err != nil {
		t.Fatal("parse HL_TESTNET_PRIVATE_KEY")
	}
	t.Cleanup(func() { _ = s.Close() })
	return s
}

func requireUnifiedTradingAcknowledgement(t *testing.T, abstraction info.UserAbstraction) {
	t.Helper()
	if usesSpotCollateral(abstraction) && os.Getenv(testnetUnifiedTradeEnv) != "1" {
		t.Skip("set HL_TESTNET_UNIFIED_TRADE=1 to acknowledge mutable Unified/Portfolio Testnet trading")
	}
}

func requireIsolatedTradingAcknowledgement(t *testing.T) {
	t.Helper()
	if os.Getenv(testnetIsolatedTradeEnv) != "1" {
		t.Skip("set HL_TESTNET_ISOLATED_TRADE=1 to enable mutable Testnet isolated-margin validation")
	}
}

func preflightTradingAccount(t *testing.T, ctx context.Context, client *hyperliquid.Client, address string, maximumNewMargin decimal.Decimal) info.UserAbstraction {
	t.Helper()
	abstraction, err := client.Info.UserAbstraction(ctx, address)
	if err != nil {
		t.Fatalf("read testnet account abstraction: %v", err)
	}
	switch abstraction {
	case info.UserAbstractionUnifiedAccount, info.UserAbstractionPortfolioMargin:
		spotState, err := client.Info.SpotClearinghouseState(ctx, address)
		if err != nil {
			t.Fatalf("read testnet spot balance: %v", err)
		}
		preflightUnifiedCollateral(t, spotState, maximumNewMargin)
		return abstraction
	case info.UserAbstractionDefault, info.UserAbstractionDisabled, info.UserAbstractionDEXAbstraction:
		state, err := client.Info.ClearinghouseState(ctx, address)
		if err != nil {
			t.Fatalf("read testnet account state: %v", err)
		}
		if state.MarginSummary.AccountValue.LessThan(maxMarginUSD) {
			t.Skipf("testnet perp account value %s is below the required 10 USDC safety floor", state.MarginSummary.AccountValue)
		}
		if state.MarginSummary.TotalMarginUsed.Add(maximumNewMargin).GreaterThan(maxMarginUSD) {
			t.Skip("existing margin plus worst-case test margin would exceed 10 USDC")
		}
		for _, position := range state.AssetPositions {
			if position.Position.Coin == testnetBTC && !position.Position.Szi.IsZero() {
				t.Skip("testnet account already has a BTC position; no order submitted")
			}
		}
		return abstraction
	default:
		t.Skipf("unsupported testnet account abstraction %q; no order submitted", abstraction)
	}
	return abstraction
}

// usesSpotCollateral follows the official account-abstraction model: Unified
// Account and Portfolio Margin expose collateral exclusively through the spot
// clearinghouse state. Perp DEX states remain the source for Standard and the
// deprecated DEX-abstraction mode, whose USDC is a perps balance.
func usesSpotCollateral(abstraction info.UserAbstraction) bool {
	return abstraction == info.UserAbstractionUnifiedAccount || abstraction == info.UserAbstractionPortfolioMargin
}

func preflightUnifiedCollateral(t *testing.T, spotState info.SpotClearinghouseStateResponse, maximumNewMargin decimal.Decimal) {
	t.Helper()
	for _, balance := range spotState.Balances {
		if balance.Coin != "USDC" {
			continue
		}
		if balance.Total.Sub(balance.Hold).LessThan(maxMarginUSD) {
			t.Skipf("testnet unified USDC available %s is below the required 10 USDC safety floor", balance.Total.Sub(balance.Hold))
		}
		if balance.Hold.Add(maximumNewMargin).GreaterThan(maxMarginUSD) {
			t.Skip("existing unified USDC hold plus worst-case test margin would exceed 10 USDC")
		}
		return
	}
	t.Skip("testnet unified account has no USDC balance; no order submitted")
}

func preflightBTCPosition(t *testing.T, ctx context.Context, client *hyperliquid.Client, address string) {
	t.Helper()
	state, err := client.Info.ClearinghouseState(ctx, address)
	if err != nil {
		t.Fatalf("read testnet BTC position: %v", err)
	}
	for _, position := range state.AssetPositions {
		if position.Position.Coin == testnetBTC && !position.Position.Szi.IsZero() {
			t.Skip("testnet account already has a BTC position; no order submitted")
		}
	}
}

func assertMarginLimit(t *testing.T, ctx context.Context, client *hyperliquid.Client, address string, abstraction info.UserAbstraction) {
	t.Helper()
	if usesSpotCollateral(abstraction) {
		spotState, err := client.Info.SpotClearinghouseState(ctx, address)
		if err != nil {
			t.Fatalf("read post-trade unified collateral: %v", err)
		}
		for _, balance := range spotState.Balances {
			if balance.Coin == "USDC" {
				if balance.Hold.GreaterThan(maxMarginUSD) {
					t.Fatalf("testnet unified USDC hold exceeds 10 USDC: %s", balance.Hold)
				}
				return
			}
		}
		t.Fatalf("testnet unified account lost its USDC balance after BTC order")
	}
	state, err := client.Info.ClearinghouseState(ctx, address)
	if err != nil {
		t.Fatalf("read post-trade margin: %v", err)
	}
	if state.MarginSummary.TotalMarginUsed.GreaterThan(maxMarginUSD) {
		t.Fatalf("testnet total margin exceeds 10 USDC: %s", state.MarginSummary.TotalMarginUsed)
	}
}

func assertCloidOrderVisible(t *testing.T, ctx context.Context, client *hyperliquid.Client, address string, cloid types.Cloid) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		status, err := client.Info.OrderStatusByCloid(ctx, address, cloid)
		if err == nil && status.Order != nil {
			return
		}
		time.Sleep(100 * time.Millisecond)
	}
	t.Fatal("submitted testnet order was not visible by CLOID")
}

func assertOrderNotOpen(t *testing.T, ctx context.Context, client *hyperliquid.Client, address string, oid uint64) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		status, err := client.Info.OrderStatus(ctx, address, oid)
		if err == nil && status.Status != "open" {
			return
		}
		select {
		case <-ctx.Done():
			t.Fatal(ctx.Err())
		case <-time.After(100 * time.Millisecond):
		}
	}
	t.Fatalf("replaced BTC order %d remained open", oid)
}

func waitForBTCPosition(t *testing.T, ctx context.Context, client *hyperliquid.Client, address string, size decimal.Decimal, open bool) {
	t.Helper()
	if err := waitForBTCPositionState(ctx, client, address, size, open); err != nil {
		t.Fatal(err)
	}
}

func assertBTCPositionLeverage(t *testing.T, ctx context.Context, client *hyperliquid.Client, address, leverageType string, leverage int) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		state, err := client.Info.ClearinghouseState(ctx, address)
		if err == nil {
			for _, position := range state.AssetPositions {
				if position.Position.Coin == testnetBTC && position.Position.Leverage.Type == leverageType && position.Position.Leverage.Value == leverage {
					return
				}
			}
		}
		select {
		case <-ctx.Done():
			t.Fatal(ctx.Err())
		case <-time.After(100 * time.Millisecond):
		}
	}
	t.Fatalf("BTC position did not report %s %dx leverage", leverageType, leverage)
}

func waitForBTCPositionState(ctx context.Context, client *hyperliquid.Client, address string, size decimal.Decimal, open bool) error {
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		state, err := client.Info.ClearinghouseState(ctx, address)
		if err == nil {
			for _, position := range state.AssetPositions {
				if position.Position.Coin == testnetBTC {
					if open && position.Position.Szi.Abs().GreaterThanOrEqual(size) {
						return nil
					}
					if !open && position.Position.Szi.IsZero() {
						return nil
					}
				}
			}
			if !open {
				return nil
			}
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(100 * time.Millisecond):
		}
	}
	if open {
		return fmt.Errorf("BTC market order did not open the expected testnet position")
	}
	return fmt.Errorf("BTC reduce-only close did not clear the testnet position")
}

func cleanupContext() (context.Context, context.CancelFunc) {
	return context.WithTimeout(context.Background(), 10*time.Second)
}

func updateBTCLeverage(ctx context.Context, client *hyperliquid.Client, isCross bool, leverage uint64) error {
	response, err := client.Exchange.UpdateLeverage(ctx, exchange.UpdateLeverageRequest{
		Coin: testnetBTC, IsCross: isCross, Leverage: leverage,
	})
	if err != nil {
		return err
	}
	if _, ok := response.Response.Data.(exchange.DefaultActionResponseData); !ok || response.Response.Type != exchange.ActionResponseDefault {
		return fmt.Errorf("unexpected BTC leverage response type %q", response.Response.Type)
	}
	return nil
}

func setAndConfirmBTCLeverage(ctx context.Context, client *hyperliquid.Client, address string, abstraction info.UserAbstraction, isCross bool, leverage uint64) error {
	updateErr := updateBTCLeverage(ctx, client, isCross, leverage)
	if usesSpotCollateral(abstraction) {
		return updateErr
	}
	if confirmErr := waitForBTCLeverage(ctx, client, address, isCross, leverage); confirmErr == nil {
		return nil
	} else if updateErr != nil {
		return fmt.Errorf("set BTC leverage: %w; confirm leverage: %v", updateErr, confirmErr)
	} else {
		return confirmErr
	}
}

func waitForBTCLeverage(ctx context.Context, client *hyperliquid.Client, address string, isCross bool, leverage uint64) error {
	deadline := time.Now().Add(5 * time.Second)
	wantType := "isolated"
	if isCross {
		wantType = "cross"
	}
	for time.Now().Before(deadline) {
		assetData, err := client.Info.ActiveAssetData(ctx, address, testnetBTC)
		if err == nil && assetData.Leverage.Type == wantType && assetData.Leverage.Value == int(leverage) {
			return nil
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(100 * time.Millisecond):
		}
	}
	return fmt.Errorf("BTC leverage did not become %dx %s", leverage, wantType)
}

func requireAcceptedOrders(response exchange.OrderResponse, expected int) error {
	data, ok := response.Response.Data.(exchange.OrderResponseData)
	if !ok || response.Response.Type != exchange.ActionResponseOrder {
		return fmt.Errorf("unexpected order response type %q", response.Response.Type)
	}
	if len(data.Statuses) != expected {
		return fmt.Errorf("order response has %d statuses, expected %d", len(data.Statuses), expected)
	}
	for index, status := range data.Statuses {
		if status.Error != nil {
			return fmt.Errorf("order %d rejected: %s", index, *status.Error)
		}
		if status.Resting == nil && status.Filled == nil {
			return fmt.Errorf("order %d has no accepted status", index)
		}
	}
	return nil
}

func cancelAndConfirmBTCOrder(ctx context.Context, client *hyperliquid.Client, address string, cloid types.Cloid) error {
	response, cancelErr := client.Exchange.CancelByCloid(ctx, exchange.CancelByCloidRequest{Coin: testnetBTC, Cloid: cloid})
	if cancelErr == nil {
		if data, ok := response.Response.Data.(exchange.CancelResponseData); !ok || response.Response.Type != exchange.ActionResponseCancel {
			cancelErr = fmt.Errorf("unexpected cancel response type %q", response.Response.Type)
		} else if len(data.Statuses) != 1 || data.Statuses[0].Success == nil || data.Statuses[0].Error != nil {
			cancelErr = fmt.Errorf("BTC cancel was not accepted")
		}
	}
	if absentErr := waitForCloidAbsent(ctx, client, address, cloid); absentErr == nil {
		return nil
	} else if cancelErr != nil {
		return fmt.Errorf("cancel BTC order: %w; confirm absence: %v", cancelErr, absentErr)
	} else {
		return absentErr
	}
}

func cancelAndConfirmBTCOrderOID(ctx context.Context, client *hyperliquid.Client, address string, oid uint64) error {
	response, cancelErr := client.Exchange.CancelOrder(ctx, exchange.CancelRequest{Coin: testnetBTC, OID: oid})
	if cancelErr == nil {
		if data, ok := response.Response.Data.(exchange.CancelResponseData); !ok || response.Response.Type != exchange.ActionResponseCancel {
			cancelErr = fmt.Errorf("unexpected numeric cancel response type %q", response.Response.Type)
		} else if len(data.Statuses) != 1 || data.Statuses[0].Success == nil || data.Statuses[0].Error != nil {
			cancelErr = fmt.Errorf("numeric BTC cancel was not accepted")
		}
	}
	status, statusErr := client.Info.OrderStatus(ctx, address, oid)
	if statusErr == nil && status.Status == "canceled" {
		return nil
	}
	if cancelErr != nil {
		return fmt.Errorf("cancel BTC order by OID: %w; confirm canceled status: %v", cancelErr, statusErr)
	}
	if statusErr != nil {
		return fmt.Errorf("confirm canceled BTC order by OID: %w", statusErr)
	}
	return fmt.Errorf("BTC order %d status is %q after numeric cancel", oid, status.Status)
}

func closeBTCPosition(ctx context.Context, client *hyperliquid.Client, address string, szDecimals int) error {
	size, err := currentBTCPosition(ctx, client, address)
	if err != nil {
		return err
	}
	if size.IsZero() {
		return nil
	}
	mids, err := client.Info.AllMids(ctx)
	if err != nil {
		return fmt.Errorf("read latest BTC mid for close: %w", err)
	}
	mid, ok := mids[testnetBTC]
	if !ok || !mid.IsPositive() {
		return fmt.Errorf("latest BTC mid is unavailable for close")
	}
	isBuy := size.IsNegative()
	price := significantPrice(mid.Mul(marketDiscount), szDecimals, false)
	if isBuy {
		price = significantPrice(mid.Mul(marketPremium), szDecimals, true)
	}
	response, err := client.Exchange.PlaceOrder(ctx, exchange.OrderRequest{
		Coin: testnetBTC, IsBuy: isBuy, Price: price, Size: size.Abs(), ReduceOnly: true,
		Type: exchange.LimitOrder{TimeInForce: exchange.TIFIOC},
	})
	if err != nil {
		return err
	}
	return requireAcceptedOrders(response, 1)
}

func closeAndConfirmBTCPosition(ctx context.Context, client *hyperliquid.Client, address string, szDecimals int) error {
	closeErr := closeBTCPosition(ctx, client, address, szDecimals)
	if absentErr := waitForBTCPositionState(ctx, client, address, decimal.Zero, false); absentErr == nil {
		return nil
	} else if closeErr != nil {
		return fmt.Errorf("close BTC position: %w; confirm zero position: %v", closeErr, absentErr)
	} else {
		return absentErr
	}
}

func currentBTCPosition(ctx context.Context, client *hyperliquid.Client, address string) (decimal.Decimal, error) {
	state, err := client.Info.ClearinghouseState(ctx, address)
	if err != nil {
		return decimal.Zero, fmt.Errorf("read BTC position for close: %w", err)
	}
	for _, position := range state.AssetPositions {
		if position.Position.Coin == testnetBTC {
			return position.Position.Szi, nil
		}
	}
	return decimal.Zero, nil
}

func currentBTCIsolatedRawUSD(ctx context.Context, client *hyperliquid.Client, address string) (decimal.Decimal, error) {
	state, err := client.Info.ClearinghouseState(ctx, address)
	if err != nil {
		return decimal.Zero, fmt.Errorf("read BTC isolated position: %w", err)
	}
	for _, position := range state.AssetPositions {
		if position.Position.Coin == testnetBTC && !position.Position.Szi.IsZero() {
			if position.Position.Leverage.Type != "isolated" || position.Position.Leverage.RawUsd == nil {
				return decimal.Zero, fmt.Errorf("BTC position does not expose isolated leverage rawUsd")
			}
			return *position.Position.Leverage.RawUsd, nil
		}
	}
	return decimal.Zero, fmt.Errorf("BTC position is absent while reading isolated raw USD")
}

func currentUSDCSpotHold(ctx context.Context, client *hyperliquid.Client, address string) (decimal.Decimal, error) {
	state, err := client.Info.SpotClearinghouseState(ctx, address)
	if err != nil {
		return decimal.Zero, fmt.Errorf("read unified USDC hold: %w", err)
	}
	for _, balance := range state.Balances {
		if balance.Coin == "USDC" {
			return balance.Hold, nil
		}
	}
	return decimal.Zero, fmt.Errorf("unified account has no USDC balance")
}

func waitForBTCIsolatedRawUSDIncrease(ctx context.Context, client *hyperliquid.Client, address string, before, minimumIncrease decimal.Decimal) error {
	deadline := time.Now().Add(testnetStateWaitTimeout)
	latest := before
	var latestErr error
	for time.Now().Before(deadline) {
		after, err := currentBTCIsolatedRawUSD(ctx, client, address)
		if err == nil {
			latest = after
			if after.Sub(before).GreaterThanOrEqual(minimumIncrease) {
				return nil
			}
		} else {
			latestErr = err
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(100 * time.Millisecond):
		}
	}
	if latestErr != nil {
		return fmt.Errorf("BTC isolated rawUsd did not increase by at least %s USDC (before %s, latest %s; last read: %v)", minimumIncrease, before, latest, latestErr)
	}
	return fmt.Errorf("BTC isolated rawUsd did not increase by at least %s USDC (before %s, latest %s)", minimumIncrease, before, latest)
}

func waitForCloidAbsent(ctx context.Context, client *hyperliquid.Client, address string, cloid types.Cloid) error {
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		orders, err := client.Info.OpenOrders(ctx, address)
		if err == nil {
			found := false
			for _, order := range orders {
				if order.Cloid != nil && *order.Cloid == cloid.String() {
					found = true
					break
				}
			}
			if !found {
				return nil
			}
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(100 * time.Millisecond):
		}
	}
	return fmt.Errorf("BTC order %s is still open", cloid)
}

func sizeForNotional(t *testing.T, price decimal.Decimal, szDecimals int) decimal.Decimal {
	t.Helper()
	if !price.IsPositive() {
		t.Fatal("BTC price is not positive")
	}
	size := testNotionalUSD.Div(price).Truncate(int32(szDecimals))
	if !size.IsPositive() || size.Mul(price).LessThan(decimal.NewFromInt(10)) {
		t.Skip("BTC price prevents a 10-11 USDC order at Testnet lot precision")
	}
	return size
}

func significantPrice(value decimal.Decimal, szDecimals int, roundUp bool) decimal.Decimal {
	canonical, err := decimal.NewFromString(value.String())
	if err != nil || !canonical.IsPositive() {
		panic(fmt.Sprintf("invalid positive price %s", value))
	}
	maxDecimals := 6 - szDecimals
	if maxDecimals < 0 {
		panic(fmt.Sprintf("invalid perpetual size precision %d", szDecimals))
	}
	decimalStep := decimal.New(1, -int32(maxDecimals))
	if roundUp {
		canonical = canonical.Div(decimalStep).Ceil().Mul(decimalStep)
	} else {
		canonical = canonical.Div(decimalStep).Floor().Mul(decimalStep)
	}
	digits := canonical.NumDigits()
	if digits <= 5 {
		return canonical
	}
	step := decimal.New(1, int32(digits-5))
	if roundUp {
		return canonical.Div(step).Ceil().Mul(step)
	}
	return canonical.Div(step).Floor().Mul(step)
}

func newCloid(t *testing.T) types.Cloid {
	t.Helper()
	cloid, err := types.NewCloid()
	if err != nil {
		t.Fatalf("generate test order cloid: %v", err)
	}
	return cloid
}
