//go:build integration && testnet

package integration

import (
	"context"
	"testing"
	"time"

	hyperliquid "github.com/Apexllcc/hypersdk-go"
	"github.com/Apexllcc/hypersdk-go/asset"
	"github.com/Apexllcc/hypersdk-go/exchange"
	"github.com/shopspring/decimal"
)

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
