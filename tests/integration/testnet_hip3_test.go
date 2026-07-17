//go:build integration && testnet

package integration

import (
	"context"
	"fmt"
	"testing"
	"time"

	hyperliquid "github.com/Apexllcc/hypersdk-go"
	"github.com/Apexllcc/hypersdk-go/asset"
	"github.com/Apexllcc/hypersdk-go/exchange"
	"github.com/Apexllcc/hypersdk-go/types"
	"github.com/shopspring/decimal"
)

const (
	testnetHIP3DEX     = "xmas"
	testnetHIP3Coin    = "xmas:EUR"
	testnetHIP3IsCross = false
)

var testnetHIP3WorstNewMargin = decimal.NewFromInt(7) // ceil(20 USDC / 3x)

// TestTestnetHIP3EURWorkflow verifies the distinct HIP-3 path with a live
// Testnet market: DEX metadata and asset-ID resolution, leverage, a resting
// order and cancel, IOC fill, DEX-scoped position read, and reduce-only close.
// It has the same explicit opt-in and cleanup guarantees as the BTC workflows.
func TestTestnetHIP3EURWorkflow(t *testing.T) {
	signingKey := requireTradingTestnet(t)
	ctx, cancel := context.WithTimeout(context.Background(), testnetWorkflowTimeout)
	defer cancel()

	metadataClient, err := hyperliquid.NewClient(hyperliquid.WithTestnet(), hyperliquid.WithHTTPTimeout(10*time.Second))
	if err != nil {
		t.Fatalf("new Testnet metadata client: %v", err)
	}
	defer func() { _ = metadataClient.Close() }()
	hip3Asset, err := testnetHIP3Asset(ctx, metadataClient)
	if err != nil {
		t.Fatal(err)
	}
	client, err := hyperliquid.NewClient(
		hyperliquid.WithTestnet(),
		hyperliquid.WithDigestSigner(signingKey),
		hyperliquid.WithAssetResolver(asset.NewStaticResolver([]asset.Asset{hip3Asset})),
		hyperliquid.WithHTTPTimeout(10*time.Second),
	)
	if err != nil {
		t.Fatalf("new Testnet HIP-3 client: %v", err)
	}
	defer func() { _ = client.Close() }()

	address := signingKey.Address().Hex()
	abstraction := preflightTradingAccount(t, ctx, client, address, testnetHIP3WorstNewMargin)
	requireUnifiedTradingAcknowledgement(t, abstraction)
	if err := requireNoHIP3Exposure(ctx, client, address); err != nil {
		t.Skipf("HIP-3 Testnet account is not clean; no order submitted: %v", err)
	}

	market := types.MarketRef{Symbol: testnetHIP3Coin, Kind: types.HIP3, DEX: testnetHIP3DEX}
	previousAssetData, err := client.Info.ActiveAssetData(ctx, address, testnetHIP3Coin)
	if err != nil {
		t.Skipf("read existing HIP-3 leverage: %v", err)
	}
	previousLeverage := previousAssetData.Leverage
	if previousLeverage.Value <= 0 || (previousLeverage.Type != "cross" && previousLeverage.Type != "isolated") {
		t.Skip("HIP-3 leverage response is invalid; no account mutation submitted")
	}
	leverageMayHaveChanged := true
	defer func() {
		if !leverageMayHaveChanged {
			return
		}
		cleanupCtx, cleanupCancel := cleanupContext()
		defer cleanupCancel()
		if err := updateHIP3Leverage(cleanupCtx, client, market, previousLeverage.Type == "cross", uint64(previousLeverage.Value)); err != nil {
			t.Errorf("restore HIP-3 leverage after Testnet validation: %v", err)
		}
	}()
	if err := updateHIP3Leverage(ctx, client, market, testnetHIP3IsCross, 3); err != nil {
		t.Fatalf("set HIP-3 3x leverage: %v", err)
	}
	positionMayBeOpen := true // Covers an unexpected fill of the resting order too.
	defer func() {
		if !positionMayBeOpen {
			return
		}
		cleanupCtx, cleanupCancel := cleanupContext()
		defer cleanupCancel()
		if err := closeAndConfirmHIP3Position(cleanupCtx, client, address, hip3Asset.SzDecimals); err != nil {
			t.Errorf("cleanup HIP-3 Testnet position: %v", err)
		}
	}()

	mids, err := client.Info.AllMidsForDEX(ctx, testnetHIP3DEX)
	if err != nil {
		t.Fatalf("read HIP-3 mids: %v", err)
	}
	mid, ok := mids[testnetHIP3Coin]
	if !ok || !mid.IsPositive() {
		t.Skip("HIP-3 mid is unavailable; no order submitted")
	}
	limitPrice := significantPrice(mid.Mul(half), hip3Asset.SzDecimals, false)
	limitSize := sizeForNotional(t, limitPrice, hip3Asset.SzDecimals)
	limitCloid := newCloid(t)
	limitMayBeOpen := true
	defer func() {
		if !limitMayBeOpen {
			return
		}
		cleanupCtx, cleanupCancel := cleanupContext()
		defer cleanupCancel()
		if err := cancelAndConfirmHIP3Order(cleanupCtx, client, address, limitCloid); err != nil {
			t.Errorf("cleanup HIP-3 resting order: %v", err)
		}
	}()
	limitResponse, err := client.Exchange.PlaceOrder(ctx, exchange.OrderRequest{
		Market: &market, IsBuy: true, Price: limitPrice, Size: limitSize,
		Type: exchange.LimitOrder{TimeInForce: exchange.TIFGTC}, ClientOrderID: &limitCloid,
	})
	if err != nil {
		t.Fatalf("place HIP-3 resting limit order: %v", err)
	}
	if err := requireAcceptedOrders(limitResponse, 1); err != nil {
		t.Fatalf("HIP-3 resting limit order was rejected: %v", err)
	}
	if err := waitForHIP3Cloid(ctx, client, address, limitCloid, true); err != nil {
		t.Fatal(err)
	}
	if err := cancelAndConfirmHIP3Order(ctx, client, address, limitCloid); err != nil {
		t.Fatalf("cancel HIP-3 resting order: %v", err)
	}
	limitMayBeOpen = false
	t.Log("verified HIP-3 market reference, asset ID, resting order, and cancellation")

	book, err := client.Info.L2Book(ctx, testnetHIP3Coin)
	if err != nil {
		t.Fatalf("read HIP-3 L2 book: %v", err)
	}
	if len(book.Levels[1]) == 0 || !book.Levels[1][0].Price.IsPositive() {
		t.Skip("HIP-3 ask liquidity is unavailable; no IOC submitted")
	}
	marketPrice := significantPrice(book.Levels[1][0].Price, hip3Asset.SzDecimals, true)
	marketSize := hip3MinimumOrderSize(t, marketPrice, previousAssetData.MarkPx, hip3Asset.SzDecimals)
	marketResponse, err := client.Exchange.PlaceOrder(ctx, exchange.OrderRequest{
		Market: &market, IsBuy: true, Price: marketPrice, Size: marketSize,
		Type: exchange.LimitOrder{TimeInForce: exchange.TIFIOC},
	})
	if err != nil {
		t.Fatalf("place HIP-3 IOC: %v", err)
	}
	if err := requireAcceptedOrders(marketResponse, 1); err != nil {
		t.Fatalf("HIP-3 IOC was rejected: %v", err)
	}
	if err := waitForHIP3Position(ctx, client, address, marketSize, true); err != nil {
		t.Fatal(err)
	}
	assertMarginLimit(t, ctx, client, address, abstraction)
	if err := closeAndConfirmHIP3Position(ctx, client, address, hip3Asset.SzDecimals); err != nil {
		t.Fatalf("close HIP-3 position: %v", err)
	}
	positionMayBeOpen = false
	t.Log("verified HIP-3 IOC fill, DEX-scoped position read, and reduce-only close")
}

func testnetHIP3Asset(ctx context.Context, client *hyperliquid.Client) (asset.Asset, error) {
	dexes, err := client.Info.PerpDEXs(ctx)
	if err != nil {
		return asset.Asset{}, fmt.Errorf("read Testnet perp DEXs: %w", err)
	}
	dexIndex := -1
	for index, dex := range dexes {
		if dex != nil && dex.Name == testnetHIP3DEX {
			dexIndex = index
			break
		}
	}
	if dexIndex < 0 {
		return asset.Asset{}, fmt.Errorf("Testnet HIP-3 DEX %q is unavailable", testnetHIP3DEX)
	}
	meta, err := client.Info.MetaForDEX(ctx, testnetHIP3DEX)
	if err != nil {
		return asset.Asset{}, fmt.Errorf("read Testnet HIP-3 metadata: %w", err)
	}
	for index, candidate := range meta.Universe {
		if candidate.Name == testnetHIP3Coin {
			return asset.Asset{ID: 100000 + dexIndex*10000 + index, Symbol: candidate.Name, Name: candidate.Name, Kind: asset.HIP3, SzDecimals: candidate.SzDecimals, DEX: testnetHIP3DEX}, nil
		}
	}
	return asset.Asset{}, fmt.Errorf("Testnet HIP-3 market %q is unavailable", testnetHIP3Coin)
}

// hip3MinimumOrderSize uses both the execution price and activeAssetData's
// mark price. HIP-3 venues can temporarily publish a book mid above the mark;
// the exchange applies its ten-USDC minimum against the latter. The 20-USDC
// ceiling retains a bounded exposure below the test account's 10-USDC margin
// cap at the workflow's 3x leverage.
func hip3MinimumOrderSize(t *testing.T, executionPrice, markPrice decimal.Decimal, szDecimals int) decimal.Decimal {
	t.Helper()
	minimumReference := executionPrice
	if markPrice.IsPositive() && markPrice.LessThan(minimumReference) {
		minimumReference = markPrice
	}
	step := decimal.New(1, -int32(szDecimals))
	size := testNotionalUSD.Div(minimumReference).Div(step).Ceil().Mul(step)
	if !size.IsPositive() || size.Mul(executionPrice).GreaterThan(decimal.NewFromInt(20)) {
		t.Skipf("HIP-3 minimum size %s at execution price %s has %s USDC value and would exceed the 20 USDC exposure ceiling", size, executionPrice, size.Mul(executionPrice))
	}
	return size
}

func requireNoHIP3Exposure(ctx context.Context, client *hyperliquid.Client, address string) error {
	orders, err := client.Info.OpenOrdersForDEX(ctx, address, testnetHIP3DEX)
	if err != nil {
		return fmt.Errorf("read HIP-3 open orders: %w", err)
	}
	for _, order := range orders {
		if order.Coin == testnetHIP3Coin {
			return fmt.Errorf("account already has HIP-3 open order %d", order.OID)
		}
	}
	state, err := client.Info.ClearinghouseStateForDEX(ctx, address, testnetHIP3DEX)
	if err != nil {
		return fmt.Errorf("read HIP-3 account state: %w", err)
	}
	for _, position := range state.AssetPositions {
		if position.Position.Coin == testnetHIP3Coin && !position.Position.Szi.IsZero() {
			return fmt.Errorf("account already has HIP-3 position %s", position.Position.Szi)
		}
	}
	return nil
}

func updateHIP3Leverage(ctx context.Context, client *hyperliquid.Client, market types.MarketRef, isCross bool, leverage uint64) error {
	response, err := client.Exchange.UpdateLeverage(ctx, exchange.UpdateLeverageRequest{Market: &market, IsCross: isCross, Leverage: leverage})
	if err != nil {
		return err
	}
	if _, ok := response.Response.Data.(exchange.DefaultActionResponseData); !ok || response.Response.Type != exchange.ActionResponseDefault {
		return fmt.Errorf("unexpected HIP-3 leverage response type %q", response.Response.Type)
	}
	return nil
}

func cancelAndConfirmHIP3Order(ctx context.Context, client *hyperliquid.Client, address string, cloid types.Cloid) error {
	response, cancelErr := client.Exchange.CancelByCloid(ctx, exchange.CancelByCloidRequest{Coin: testnetHIP3Coin, Cloid: cloid})
	if cancelErr == nil {
		cancelErr = requireAcceptedCancels(response, 1)
	}
	confirmErr := waitForHIP3Cloid(ctx, client, address, cloid, false)
	if outcomeErr := cancellationOutcome(cancelErr, confirmErr); outcomeErr != nil {
		return fmt.Errorf("cancel HIP-3 order: %w", outcomeErr)
	}
	return nil
}

func waitForHIP3Cloid(ctx context.Context, client *hyperliquid.Client, address string, cloid types.Cloid, wantOpen bool) error {
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		orders, err := client.Info.OpenOrdersForDEX(ctx, address, testnetHIP3DEX)
		if err == nil {
			found := false
			for _, order := range orders {
				if order.Cloid != nil && *order.Cloid == cloid.String() {
					found = true
					break
				}
			}
			if found == wantOpen {
				return nil
			}
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(100 * time.Millisecond):
		}
	}
	if wantOpen {
		return fmt.Errorf("HIP-3 order %s was not visible", cloid)
	}
	return fmt.Errorf("HIP-3 order %s is still open", cloid)
}

func waitForHIP3Position(ctx context.Context, client *hyperliquid.Client, address string, size decimal.Decimal, wantOpen bool) error {
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		state, err := client.Info.ClearinghouseStateForDEX(ctx, address, testnetHIP3DEX)
		if err == nil {
			for _, position := range state.AssetPositions {
				if position.Position.Coin == testnetHIP3Coin {
					if wantOpen && position.Position.Szi.Abs().GreaterThanOrEqual(size) {
						return nil
					}
					if !wantOpen && position.Position.Szi.IsZero() {
						return nil
					}
				}
			}
			if !wantOpen {
				return nil
			}
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(100 * time.Millisecond):
		}
	}
	if wantOpen {
		return fmt.Errorf("HIP-3 IOC did not open expected position")
	}
	return fmt.Errorf("HIP-3 reduce-only close did not clear position")
}

func closeAndConfirmHIP3Position(ctx context.Context, client *hyperliquid.Client, address string, szDecimals int) error {
	state, err := client.Info.ClearinghouseStateForDEX(ctx, address, testnetHIP3DEX)
	if err != nil {
		return fmt.Errorf("read HIP-3 position for close: %w", err)
	}
	size := decimal.Zero
	for _, position := range state.AssetPositions {
		if position.Position.Coin == testnetHIP3Coin {
			size = position.Position.Szi
			break
		}
	}
	if size.IsZero() {
		return nil
	}
	book, err := client.Info.L2Book(ctx, testnetHIP3Coin)
	if err != nil {
		return fmt.Errorf("read HIP-3 L2 book for close: %w", err)
	}
	isBuy := size.IsNegative()
	level := 0 // Closing a long sells into the best bid.
	if isBuy {
		level = 1 // Closing a short buys from the best ask.
	}
	if len(book.Levels[level]) == 0 || !book.Levels[level][0].Price.IsPositive() {
		return fmt.Errorf("HIP-3 %s liquidity is unavailable for close", map[bool]string{true: "ask", false: "bid"}[isBuy])
	}
	price := significantPrice(book.Levels[level][0].Price, szDecimals, isBuy)
	response, err := client.Exchange.PlaceOrder(ctx, exchange.OrderRequest{
		Coin: testnetHIP3Coin, IsBuy: isBuy, Price: price, Size: size.Abs(), ReduceOnly: true,
		Type: exchange.LimitOrder{TimeInForce: exchange.TIFIOC},
	})
	if err != nil {
		return err
	}
	if err := requireAcceptedOrders(response, 1); err != nil {
		return err
	}
	return waitForHIP3Position(ctx, client, address, decimal.Zero, false)
}
