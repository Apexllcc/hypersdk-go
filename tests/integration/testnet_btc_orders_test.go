//go:build integration && testnet

package integration

import (
	"context"
	"testing"
	"time"

	hyperliquid "github.com/Apexllcc/hypersdk-go"
	"github.com/Apexllcc/hypersdk-go/asset"
	"github.com/Apexllcc/hypersdk-go/exchange"
	"github.com/Apexllcc/hypersdk-go/types"
)

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
		t.Fatalf("BTC limit order response = %#v, want CLOID %q", limitStatus.Order, limitCloid.String())
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
		t.Fatalf("modified BTC limit order response = %#v, want CLOID %q; read error: %v", modifiedStatus.Order, modifiedLimitCloid.String(), err)
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
		t.Fatalf("batch-modified BTC limit order response = %#v, want CLOID %q", batchedStatus.Order, batchedLimitCloid.String())
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
