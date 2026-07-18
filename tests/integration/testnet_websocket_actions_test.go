//go:build integration && testnet

package integration

import (
	"context"
	"fmt"
	"os"
	"testing"
	"time"

	hyperliquid "github.com/Apexllcc/hypersdk-go"
	"github.com/Apexllcc/hypersdk-go/asset"
	"github.com/Apexllcc/hypersdk-go/exchange"
	"github.com/Apexllcc/hypersdk-go/signer"
	"github.com/Apexllcc/hypersdk-go/types"
	"github.com/shopspring/decimal"
)

const testnetWebSocketActionEnv = "HL_TESTNET_WS_ACTION"

// TestTestnetWebSocketPostActionOrderLifecycle proves signed Exchange actions
// travel through websocket.Client.PostAction rather than HTTP. It is a
// deliberately separate opt-in from the normal Testnet trade tests because it
// opens and closes a small BTC position. Every signed action uses the same
// root WebSocket client as the injected RequestTransport.
func TestTestnetWebSocketPostActionOrderLifecycle(t *testing.T) {
	signingKey := requireWebSocketActionTestnet(t)
	ctx, cancel := context.WithTimeout(context.Background(), testnetWorkflowTimeout)
	defer cancel()

	metadataClient, err := hyperliquid.NewClient(hyperliquid.WithTestnet(), hyperliquid.WithHTTPTimeout(10*time.Second))
	if err != nil {
		t.Fatalf("new Testnet metadata client: %v", err)
	}
	defer func() { _ = metadataClient.Close() }()
	meta, err := metadataClient.Info.Meta(ctx)
	if err != nil {
		t.Fatalf("read Testnet BTC metadata: %v", err)
	}
	btcAsset, err := testnetBasePerpAsset(meta, testnetBTC)
	if err != nil {
		t.Fatal(err)
	}

	client, err := newTestnetWebSocketActionClient(signingKey, btcAsset)
	if err != nil {
		t.Fatalf("new Testnet WebSocket action client: %v", err)
	}
	defer func() { _ = client.Close() }()
	address := signingKey.Address().Hex()

	abstraction := preflightTradingAccount(t, ctx, client, address, worstNewMargin)
	requireUnifiedTradingAcknowledgement(t, abstraction)
	if usesSpotCollateral(abstraction) {
		preflightBTCPosition(t, ctx, client, address)
	}
	previousAssetData, err := client.Info.ActiveAssetData(ctx, address, testnetBTC)
	if err != nil {
		t.Fatalf("read Testnet BTC leverage: %v", err)
	}
	previousLeverage := previousAssetData.Leverage
	if previousLeverage.Value <= 0 || (previousLeverage.Type != "cross" && previousLeverage.Type != "isolated") {
		t.Skip("Testnet BTC leverage response is invalid; no WebSocket action submitted")
	}
	restoreLeverage := true
	defer func() {
		if !restoreLeverage {
			return
		}
		cleanupCtx, cleanupCancel := cleanupContext()
		defer cleanupCancel()
		if err := setAndConfirmBTCLeverage(cleanupCtx, client, address, abstraction, previousLeverage.Type == "cross", uint64(previousLeverage.Value)); err != nil {
			t.Errorf("restore BTC leverage after WebSocket action validation: %v", err)
		}
	}()
	if err := setAndConfirmBTCLeverage(ctx, client, address, abstraction, true, 3); err != nil {
		t.Fatalf("set BTC 3x leverage through PostAction: %v", err)
	}

	openOrders, err := client.Info.OpenOrders(ctx, address)
	if err != nil {
		t.Fatalf("read Testnet BTC open orders: %v", err)
	}
	for _, order := range openOrders {
		if order.Coin == testnetBTC {
			t.Skip("Testnet account already has a BTC open order; no WebSocket action submitted")
		}
	}
	mids, err := client.Info.AllMids(ctx)
	if err != nil {
		t.Fatalf("read Testnet BTC mid: %v", err)
	}
	mid, ok := mids[testnetBTC]
	if !ok || !mid.IsPositive() {
		t.Skip("Testnet BTC mid is unavailable; no WebSocket action submitted")
	}

	limitCloid := newCloid(t)
	tpCloid := newCloid(t)
	slCloid := newCloid(t)
	marketCloid := newCloid(t)
	cleanupArmed := true
	defer func() {
		if !cleanupArmed {
			return
		}
		for _, cloid := range []types.Cloid{limitCloid, tpCloid, slCloid, marketCloid} {
			cleanupCtx, cleanupCancel := cleanupContext()
			if err := cancelAndConfirmBTCOrder(cleanupCtx, client, address, cloid); err != nil {
				t.Errorf("cleanup BTC WebSocket action order %s: %v", cloid, err)
			}
			cleanupCancel()
		}
		closeCtx, closeCancel := cleanupContext()
		defer closeCancel()
		if err := closeAndConfirmBTCPosition(closeCtx, client, address, btcAsset.SzDecimals); err != nil {
			t.Errorf("cleanup BTC WebSocket action position: %v", err)
		}
	}()

	// A far-below-market GTC is guaranteed to be a limit-order lifecycle rather
	// than an accidental market fill under normal Testnet conditions.
	limitPrice := significantPrice(mid.Mul(half), btcAsset.SzDecimals, false)
	limitSize := sizeForNotional(t, limitPrice, btcAsset.SzDecimals)
	limitResponse, err := client.Exchange.PlaceOrder(ctx, exchange.OrderRequest{
		Coin: testnetBTC, IsBuy: true, Price: limitPrice, Size: limitSize,
		Type: exchange.LimitOrder{TimeInForce: exchange.TIFGTC}, ClientOrderID: &limitCloid,
	})
	if err != nil {
		t.Fatalf("place BTC GTC through PostAction: %v", err)
	}
	if err := requireAcceptedOrders(limitResponse, 1); err != nil {
		t.Fatalf("BTC GTC through PostAction was rejected: %v", err)
	}
	assertCloidOrderVisible(t, ctx, client, address, limitCloid)
	if err := cancelAndConfirmBTCOrder(ctx, client, address, limitCloid); err != nil {
		t.Fatalf("cancel BTC GTC through PostAction: %v", err)
	}
	if err := closeAndConfirmBTCPosition(ctx, client, address, btcAsset.SzDecimals); err != nil {
		t.Fatalf("close an unexpectedly filled BTC GTC through PostAction: %v", err)
	}
	assertMarginLimit(t, ctx, client, address, abstraction)

	// Stop buys trigger only above the current mid; take buys trigger only
	// below it. Use trigger-limit orders, not trigger-market orders: the
	// protected limit is the trigger price and the size is calculated from that
	// price, so either rare execution remains bounded to 11 USDC notional.
	tpPrice := significantPrice(mid.Mul(half), btcAsset.SzDecimals, false)
	slPrice := significantPrice(mid.Mul(decimal.NewFromInt(2)), btcAsset.SzDecimals, true)
	for _, request := range []struct {
		name  string
		cloid *types.Cloid
		order exchange.OrderRequest
	}{
		{
			name:  "take-profit",
			cloid: &tpCloid,
			order: exchange.OrderRequest{
				Coin: testnetBTC, IsBuy: true, Price: tpPrice, Size: sizeForNotional(t, tpPrice, btcAsset.SzDecimals),
				Type: exchange.TriggerOrder{IsMarket: false, TriggerPrice: tpPrice, TPSL: exchange.TPSLTakeProfit}, ClientOrderID: &tpCloid,
			},
		},
		{
			name:  "stop-loss",
			cloid: &slCloid,
			order: exchange.OrderRequest{
				Coin: testnetBTC, IsBuy: true, Price: slPrice, Size: sizeForNotional(t, slPrice, btcAsset.SzDecimals),
				Type: exchange.TriggerOrder{IsMarket: false, TriggerPrice: slPrice, TPSL: exchange.TPSLStopLoss}, ClientOrderID: &slCloid,
			},
		},
	} {
		response, err := client.Exchange.PlaceOrder(ctx, request.order)
		if err != nil {
			t.Fatalf("place BTC %s trigger through PostAction: %v", request.name, err)
		}
		if err := requireAcceptedOrders(response, 1); err != nil {
			t.Fatalf("BTC %s trigger through PostAction was rejected: %v", request.name, err)
		}
		assertCloidOrderVisible(t, ctx, client, address, *request.cloid)
		if err := cancelAndConfirmBTCOrder(ctx, client, address, *request.cloid); err != nil {
			t.Fatalf("cancel BTC %s trigger through PostAction: %v", request.name, err)
		}
		if err := closeAndConfirmBTCPosition(ctx, client, address, btcAsset.SzDecimals); err != nil {
			t.Fatalf("close an unexpectedly filled BTC %s trigger through PostAction: %v", request.name, err)
		}
		assertMarginLimit(t, ctx, client, address, abstraction)
	}

	// FrontendMarket is the Testnet frontend market-order marker requested by
	// the caller. BTC's deep book permits a bounded 11-USDC probe; a reduce-only
	// FrontendMarket order is armed before submission to unwind any fill.
	marketPrice := significantPrice(mid.Mul(marketPremium), btcAsset.SzDecimals, true)
	marketSize := sizeForNotional(t, marketPrice, btcAsset.SzDecimals)
	marketResponse, err := client.Exchange.PlaceOrder(ctx, exchange.OrderRequest{
		Coin: testnetBTC, IsBuy: true, Price: marketPrice, Size: marketSize,
		Type: exchange.LimitOrder{TimeInForce: exchange.TIFFrontendMarket}, ClientOrderID: &marketCloid,
	})
	if err != nil {
		t.Fatalf("place BTC FrontendMarket through PostAction: %v", err)
	}
	if err := requireAcceptedOrders(marketResponse, 1); err != nil {
		t.Fatalf("BTC FrontendMarket through PostAction was rejected: %v", err)
	}
	assertCloidOrderVisible(t, ctx, client, address, marketCloid)
	waitForBTCPosition(t, ctx, client, address, marketSize, true)
	if err := waitForBTCFillByCloid(ctx, client, address, marketCloid); err != nil {
		t.Fatal(err)
	}
	if err := closeAndConfirmBTCPosition(ctx, client, address, btcAsset.SzDecimals); err != nil {
		t.Fatalf("close BTC FrontendMarket position through PostAction: %v", err)
	}
	assertMarginLimit(t, ctx, client, address, abstraction)
	cleanupArmed = false
	if err := setAndConfirmBTCLeverage(ctx, client, address, abstraction, previousLeverage.Type == "cross", uint64(previousLeverage.Value)); err != nil {
		t.Fatalf("restore BTC leverage through PostAction: %v", err)
	}
	restoreLeverage = false
	t.Log("verified PostAction limit, take-profit, stop-loss, FrontendMarket, fill read, position close, and leverage restoration")
}

func requireWebSocketActionTestnet(t *testing.T) *signer.LocalPrivateKeySigner {
	t.Helper()
	if os.Getenv(testnetWebSocketActionEnv) != "1" {
		t.Skip("set HL_TESTNET_WS_ACTION=1 to enable mutable Testnet PostAction validation")
	}
	return requireTradingTestnet(t)
}

func newTestnetWebSocketActionClient(signingKey *signer.LocalPrivateKeySigner, btcAsset asset.Asset) (*hyperliquid.Client, error) {
	client, err := hyperliquid.NewClient(
		hyperliquid.WithTestnet(),
		hyperliquid.WithDigestSigner(signingKey),
		hyperliquid.WithAssetResolver(asset.NewStaticResolver([]asset.Asset{btcAsset})),
		hyperliquid.WithHTTPTimeout(10*time.Second),
	)
	if err != nil {
		return nil, err
	}
	// Inject before issuing any request. Exchange.submitL1 then calls
	// websocket.Client.Request with RequestAction, which dispatches PostAction.
	// Info reads share the same connection through RequestInfo for end-to-end
	// protocol and lifecycle verification.
	client.Info.SetRequestTransport(client.WebSocket)
	client.Exchange.SetRequestTransport(client.WebSocket)
	return client, nil
}

func waitForBTCFillByCloid(ctx context.Context, client *hyperliquid.Client, address string, cloid types.Cloid) error {
	deadline := time.Now().Add(testnetStateWaitTimeout)
	for time.Now().Before(deadline) {
		fills, err := client.Info.UserFills(ctx, address, false)
		if err == nil {
			for _, fill := range fills {
				if fill.Coin == testnetBTC && fill.Cloid != nil && *fill.Cloid == cloid.String() {
					return nil
				}
			}
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(100 * time.Millisecond):
		}
	}
	return fmt.Errorf("BTC FrontendMarket fill for CLOID %s was not returned by userFills", cloid)
}
