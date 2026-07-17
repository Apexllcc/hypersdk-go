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
	testnetOutcomeID   uint64 = 10395
	testnetOutcomeSide uint64 = 0
)

var (
	testnetOutcomeMinimumNotionalUSD = decimal.NewFromInt(10)
	testnetOutcomeMaxHoldUSD         = decimal.NewFromInt(11)
)

// TestTestnetOutcomeOrderLifecycle submits a real Testnet outcome order using
// the official outcome asset encoding, observes it by CLOID, then cancels it.
// It deliberately requires a one-sided book and uses ALO so the test cannot
// take liquidity or leave an outcome-token position.
func TestTestnetOutcomeOrderLifecycle(t *testing.T) {
	signingKey := requireTradingTestnet(t)
	ctx, cancel := context.WithTimeout(context.Background(), testnetWorkflowTimeout)
	defer cancel()

	metadataClient, err := hyperliquid.NewClient(hyperliquid.WithTestnet(), hyperliquid.WithHTTPTimeout(10*time.Second))
	if err != nil {
		t.Fatalf("new Testnet metadata client: %v", err)
	}
	defer func() { _ = metadataClient.Close() }()
	outcomeAsset, err := testnetOutcomeAsset(ctx, metadataClient)
	if err != nil {
		t.Fatal(err)
	}
	client, err := hyperliquid.NewClient(
		hyperliquid.WithTestnet(),
		hyperliquid.WithDigestSigner(signingKey),
		hyperliquid.WithAssetResolver(asset.NewStaticResolver([]asset.Asset{outcomeAsset})),
		hyperliquid.WithHTTPTimeout(10*time.Second),
	)
	if err != nil {
		t.Fatalf("new Testnet outcome client: %v", err)
	}
	defer func() { _ = client.Close() }()

	address := signingKey.Address().Hex()
	if err := requireNoOutcomeExposure(ctx, client, address, outcomeAsset); err != nil {
		t.Skipf("Testnet outcome account is not clean; no order submitted: %v", err)
	}
	baseline, err := outcomeBalances(ctx, client, address, outcomeAsset)
	if err != nil {
		t.Fatalf("read Testnet outcome baseline: %v", err)
	}
	book, err := client.Info.L2Book(ctx, outcomeAsset.Symbol)
	if err != nil {
		t.Fatalf("read Testnet outcome L2 book: %v", err)
	}
	if len(book.Levels[0]) == 0 || !book.Levels[0][0].Price.IsPositive() || len(book.Levels[1]) != 0 {
		t.Skip("selected Testnet outcome lacks the required bid-only book; no order submitted")
	}
	// Stay below the visible bid. ALO prevents an immediate match and this
	// additional distance reduces the chance of a later fill before cancel.
	price := significantPrice(book.Levels[0][0].Price.Mul(marketDiscount), outcomeAsset.SzDecimals, false)
	size := outcomeMinimumOrderSize(t, price)
	notional := price.Mul(size)
	preflightOutcomeSpotHold(t, baseline, notional)

	market := types.MarketRef{Symbol: outcomeAsset.Symbol, Kind: types.Outcome}
	cloid := newCloid(t)
	cleanupArmed := true
	defer func() {
		if !cleanupArmed {
			return
		}
		cleanupCtx, cleanupCancel := cleanupContext()
		defer cleanupCancel()
		if err := cleanupOutcomeOrder(cleanupCtx, client, address, outcomeAsset, cloid, baseline); err != nil {
			t.Errorf("cleanup Testnet outcome order: %v", err)
		}
	}()
	response, err := client.Exchange.PlaceOrder(ctx, exchange.OrderRequest{
		Market: &market, IsBuy: true, Price: price, Size: size,
		Type: exchange.LimitOrder{TimeInForce: exchange.TIFALO}, ClientOrderID: &cloid,
	})
	if err != nil {
		t.Fatalf("place Testnet outcome ALO: %v", err)
	}
	if err := requireAcceptedOrders(response, 1); err != nil {
		t.Fatalf("Testnet outcome ALO was rejected: %v", err)
	}
	if err := waitForOutcomeCloid(ctx, client, address, cloid, true); err != nil {
		t.Fatal(err)
	}
	if err := cancelAndConfirmOutcomeOrder(ctx, client, address, outcomeAsset.Symbol, cloid); err != nil {
		t.Fatalf("cancel Testnet outcome order: %v", err)
	}
	if err := waitForOutcomeBalanceBaseline(ctx, client, address, outcomeAsset, baseline); err != nil {
		t.Fatal(err)
	}
	// Keep cleanup armed until both the order and the collateral/token balances
	// have returned to their pre-test state. A partially filled ALO can have
	// been cancelled successfully while still leaving outcome tokens to unwind.
	cleanupArmed = false
	t.Logf("verified Outcome asset %d (%s), ALO order, CLOID lookup, and cancellation", outcomeAsset.ID, outcomeAsset.Symbol)
}

func testnetOutcomeAsset(ctx context.Context, client *hyperliquid.Client) (asset.Asset, error) {
	meta, err := client.Info.OutcomeMeta(ctx)
	if err != nil {
		return asset.Asset{}, fmt.Errorf("read Testnet outcome metadata: %w", err)
	}
	for _, outcome := range meta.Outcomes {
		if outcome.Outcome == int(testnetOutcomeID) && len(outcome.SideSpecs) >= 2 {
			encoding := testnetOutcomeID*10 + testnetOutcomeSide
			return asset.Asset{
				ID: 100000000 + int(encoding), Symbol: fmt.Sprintf("#%d", encoding), Name: fmt.Sprintf("+%d", encoding),
				Kind: asset.Outcome, SzDecimals: 0,
			}, nil
		}
	}
	return asset.Asset{}, fmt.Errorf("Testnet outcome %d with binary sides is unavailable", testnetOutcomeID)
}

func requireNoOutcomeExposure(ctx context.Context, client *hyperliquid.Client, address string, outcome asset.Asset) error {
	orders, err := client.Info.OpenOrders(ctx, address)
	if err != nil {
		return fmt.Errorf("read open orders: %w", err)
	}
	for _, order := range orders {
		if order.Coin == outcome.Symbol {
			return fmt.Errorf("account already has outcome open order %d", order.OID)
		}
	}
	balance, err := outcomeTokenBalance(ctx, client, address, outcome)
	if err != nil {
		return err
	}
	if !balance.IsZero() {
		return fmt.Errorf("account already has outcome balance %s", balance)
	}
	return nil
}

func preflightOutcomeSpotHold(t *testing.T, baseline outcomeBalanceSnapshot, notional decimal.Decimal) {
	t.Helper()
	if baseline.usdcAvailable.LessThan(notional) {
		t.Skipf("Testnet USDC available %s is below outcome order notional %s", baseline.usdcAvailable, notional)
	}
	if baseline.usdcHold.Add(notional).GreaterThan(testnetOutcomeMaxHoldUSD) {
		t.Skipf("outcome order would increase Testnet spot hold to %s, above %s safety ceiling", baseline.usdcHold.Add(notional), testnetOutcomeMaxHoldUSD)
	}
}

func outcomeMinimumOrderSize(t *testing.T, price decimal.Decimal) decimal.Decimal {
	t.Helper()
	if !price.IsPositive() {
		t.Fatal("outcome bid is not positive")
	}
	size := testnetOutcomeMinimumNotionalUSD.Div(price).Ceil()
	if size.Mul(price).GreaterThan(testnetOutcomeMaxHoldUSD) {
		t.Skipf("outcome minimum order %s at %s has %s USDC value above %s safety ceiling", size, price, size.Mul(price), testnetOutcomeMaxHoldUSD)
	}
	return size
}

func cancelAndConfirmOutcomeOrder(ctx context.Context, client *hyperliquid.Client, address, coin string, cloid types.Cloid) error {
	response, cancelErr := client.Exchange.CancelByCloid(ctx, exchange.CancelByCloidRequest{Coin: coin, Cloid: cloid})
	if cancelErr == nil {
		cancelErr = requireAcceptedCancels(response, 1)
	}
	if outcomeErr := cancellationOutcome(cancelErr, waitForOutcomeCloid(ctx, client, address, cloid, false)); outcomeErr != nil {
		return fmt.Errorf("cancel outcome order: %w", outcomeErr)
	}
	return nil
}

func waitForOutcomeCloid(ctx context.Context, client *hyperliquid.Client, address string, cloid types.Cloid, wantOpen bool) error {
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
		return fmt.Errorf("outcome order %s was not visible", cloid)
	}
	return fmt.Errorf("outcome order %s is still open", cloid)
}

func outcomeTokenBalance(ctx context.Context, client *hyperliquid.Client, address string, outcome asset.Asset) (decimal.Decimal, error) {
	balances, err := outcomeBalances(ctx, client, address, outcome)
	return balances.token, err
}

type outcomeBalanceSnapshot struct {
	token         decimal.Decimal
	usdcHold      decimal.Decimal
	usdcAvailable decimal.Decimal
}

func outcomeBalances(ctx context.Context, client *hyperliquid.Client, address string, outcome asset.Asset) (outcomeBalanceSnapshot, error) {
	state, err := client.Info.SpotClearinghouseState(ctx, address)
	if err != nil {
		return outcomeBalanceSnapshot{}, fmt.Errorf("read outcome spot balance: %w", err)
	}
	usdc, err := usdcSpotBalance(state)
	if err != nil {
		return outcomeBalanceSnapshot{}, err
	}
	result := outcomeBalanceSnapshot{usdcHold: usdc.Hold, usdcAvailable: usdc.Total.Sub(usdc.Hold)}
	for _, balance := range state.Balances {
		if balance.Coin == outcome.Symbol || balance.Coin == outcome.Name {
			result.token = balance.Total
			break
		}
	}
	return result, nil
}

func cleanupOutcomeOrder(ctx context.Context, client *hyperliquid.Client, address string, outcome asset.Asset, cloid types.Cloid, baseline outcomeBalanceSnapshot) error {
	for {
		// A missing-order response can mean the ALO filled between observation
		// and cancellation. Confirm absence before each balance snapshot so an
		// order cannot fill after the snapshot and escape reconciliation.
		if err := ensureOutcomeOrderAbsent(ctx, client, address, outcome.Symbol, cloid); err != nil {
			return err
		}
		balances, err := outcomeBalances(ctx, client, address, outcome)
		if err != nil {
			return err
		}
		if balances.token.Equal(baseline.token) && balances.usdcHold.Equal(baseline.usdcHold) {
			return nil
		}
		if balances.token.GreaterThan(baseline.token) {
			if err := exitOutcomeTokens(ctx, client, outcome, balances.token.Sub(baseline.token)); err != nil {
				return fmt.Errorf("exit unexpectedly acquired outcome tokens: %w", err)
			}
		}
		select {
		case <-ctx.Done():
			return fmt.Errorf("outcome cleanup did not return balances to baseline: %w", ctx.Err())
		case <-time.After(100 * time.Millisecond):
		}
	}
}

// ensureOutcomeOrderAbsent is deliberately less strict than the normal-path
// cancel assertion: during cleanup, a missing-order response is expected when
// an ALO has already filled or a prior cancellation succeeded. CLOID absence
// is the authoritative cleanup condition; it must be established before each
// balance reconciliation pass.
func ensureOutcomeOrderAbsent(ctx context.Context, client *hyperliquid.Client, address, coin string, cloid types.Cloid) error {
	_, _ = client.Exchange.CancelByCloid(ctx, exchange.CancelByCloidRequest{Coin: coin, Cloid: cloid})
	if err := waitForOutcomeCloid(ctx, client, address, cloid, false); err != nil {
		return fmt.Errorf("confirm outcome order absence: %w", err)
	}
	return nil
}

func exitOutcomeTokens(ctx context.Context, client *hyperliquid.Client, outcome asset.Asset, size decimal.Decimal) error {
	book, err := client.Info.L2Book(ctx, outcome.Symbol)
	if err != nil {
		return err
	}
	if len(book.Levels[0]) == 0 || !book.Levels[0][0].Price.IsPositive() {
		return fmt.Errorf("no bid liquidity")
	}
	price := book.Levels[0][0].Price
	if size.Mul(price).LessThan(testnetOutcomeMinimumNotionalUSD) {
		return fmt.Errorf("acquired outcome value %s is below the minimum exit notional", size.Mul(price))
	}
	response, err := client.Exchange.PlaceOrder(ctx, exchange.OrderRequest{
		Coin: outcome.Symbol, IsBuy: false, Price: price, Size: size,
		Type: exchange.LimitOrder{TimeInForce: exchange.TIFIOC},
	})
	if err != nil {
		return err
	}
	return requireAcceptedOrders(response, 1)
}

func waitForOutcomeBalanceBaseline(ctx context.Context, client *hyperliquid.Client, address string, outcome asset.Asset, baseline outcomeBalanceSnapshot) error {
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		balances, err := outcomeBalances(ctx, client, address, outcome)
		if err == nil && balances.token.Equal(baseline.token) && balances.usdcHold.Equal(baseline.usdcHold) {
			return nil
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(100 * time.Millisecond):
		}
	}
	return fmt.Errorf("outcome balance or USDC hold did not return to baseline")
}
