//go:build integration && testnet

package integration

import (
	"context"
	"fmt"
	"os"
	"strconv"
	"strings"
	"testing"
	"time"

	hyperliquid "github.com/Apexllcc/hypersdk-go"
	"github.com/Apexllcc/hypersdk-go/asset"
	"github.com/Apexllcc/hypersdk-go/exchange"
	"github.com/Apexllcc/hypersdk-go/info"
	"github.com/Apexllcc/hypersdk-go/types"
	"github.com/shopspring/decimal"
)

const (
	testnetOutcomeID   uint64 = 10395
	testnetOutcomeSide uint64 = 0
)

var (
	testnetOutcomeMinimumNotionalUSD = decimal.NewFromInt(10)
	// Testnet spot/outcome tests may not reserve more than 100 USDC in total.
	testnetOutcomeMaxHoldUSD = decimal.NewFromInt(100)
	// A market round trip is only a protocol test; it must not cross a wide
	// Outcome spread. This cap is intentionally much lower than the notional cap.
	testnetOutcomeMaxRoundTripLossUSD = decimal.NewFromInt(1)
)

func TestOutcomeOrderSizeForPriceFitsSafetyCap(t *testing.T) {
	size, ok := outcomeOrderSizeForPrice(decimal.RequireFromString("0.485"))
	if !ok || !size.Equal(decimal.NewFromInt(21)) {
		t.Fatalf("outcome order size = %s, ok=%t; want 21, true", size, ok)
	}
}

func TestOutcomeMarketAdmissionRejectsInsufficientDepthAndWideSpread(t *testing.T) {
	size := decimal.NewFromInt(100)
	shallow := info.L2BookResponse{Levels: [2][]info.BookLevel{
		{{Price: decimal.RequireFromString("0.49"), Size: decimal.NewFromInt(12)}},
		{{Price: decimal.RequireFromString("0.50"), Size: decimal.NewFromInt(100)}},
	}}
	if outcomeMarketAdmissionAllowed(shallow, size) {
		t.Fatal("shallow Outcome book was admitted for FrontendMarket testing")
	}
	wide := info.L2BookResponse{Levels: [2][]info.BookLevel{
		{{Price: decimal.RequireFromString("0.09"), Size: decimal.NewFromInt(100)}},
		{{Price: decimal.RequireFromString("0.88"), Size: decimal.NewFromInt(100)}},
	}}
	if outcomeMarketAdmissionAllowed(wide, size) {
		t.Fatal("wide Outcome book was admitted for FrontendMarket testing")
	}
}

// TestTestnetOutcomeOrderLifecycle submits a real Testnet outcome order using
// the official outcome asset encoding, observes it by CLOID, then cancels it.
// It deliberately requires a one-sided book and uses a deeply below-bid ALO.
// ALO prevents taking liquidity at submission; the distant price reduces the
// residual risk of a later fill before cancellation.
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
	// ALO enforces post-only at submission. Keeping the price far below the bid
	// further reduces the chance of a later fill before cancellation.
	price := significantPrice(book.Levels[0][0].Price.Mul(half), outcomeAsset.SzDecimals, false)
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

// TestTestnetOutcomeFrontendMarketWorkflow documents why automatic Outcome
// market mutation is deliberately disabled. FrontendMarket is IOC rather than
// FOK; a sub-minimum partial fill cannot be unwound losslessly via the API.
func TestTestnetOutcomeFrontendMarketWorkflow(t *testing.T) {
	t.Skip("automatic Outcome FrontendMarket testing is disabled: IOC partial fills can be below the minimum exit notional, and the protocol provides no lossless atomic cleanup")
}

// TestTestnetOutcomeResidualRecovery is deliberately a separate opt-in escape
// hatch for a previously interrupted Outcome test. It never runs under the
// normal trading acknowledgement alone and only cleans positive +<encoding>
// balances belonging to the signing account.
func TestTestnetOutcomeResidualRecovery(t *testing.T) {
	if os.Getenv("HL_TESTNET_OUTCOME_RECOVERY") != "1" {
		t.Skip("set HL_TESTNET_OUTCOME_RECOVERY=1 to clean a residual Testnet Outcome balance")
	}
	signingKey := requireTradingTestnet(t)
	ctx, cancel := context.WithTimeout(context.Background(), testnetWorkflowTimeout)
	defer cancel()
	client, err := hyperliquid.NewClient(
		hyperliquid.WithTestnet(),
		hyperliquid.WithDigestSigner(signingKey),
		hyperliquid.WithHTTPTimeout(10*time.Second),
	)
	if err != nil {
		t.Fatalf("new Testnet Outcome recovery client: %v", err)
	}
	defer func() { _ = client.Close() }()
	address := signingKey.Address().Hex()
	state, err := client.Info.SpotClearinghouseState(ctx, address)
	if err != nil {
		t.Fatalf("read Testnet Outcome balances for recovery: %v", err)
	}
	usdc, err := usdcSpotBalance(state)
	if err != nil {
		t.Fatal(err)
	}
	for _, balance := range state.Balances {
		if !strings.HasPrefix(balance.Coin, "+") || !balance.Total.IsPositive() {
			continue
		}
		encoding, parseErr := strconv.Atoi(strings.TrimPrefix(balance.Coin, "+"))
		if parseErr != nil || encoding < 0 || encoding%10 > 1 {
			continue
		}
		outcome := asset.Asset{ID: 100000000 + encoding, Symbol: fmt.Sprintf("#%d", encoding), Name: balance.Coin, Kind: asset.Outcome, SzDecimals: 0}
		baseline := outcomeBalanceSnapshot{usdcHold: usdc.Hold, usdcAvailable: usdc.Total.Sub(usdc.Hold)}
		tradingClient, newErr := hyperliquid.NewClient(
			hyperliquid.WithTestnet(),
			hyperliquid.WithDigestSigner(signingKey),
			hyperliquid.WithAssetResolver(asset.NewStaticResolver([]asset.Asset{outcome})),
			hyperliquid.WithHTTPTimeout(10*time.Second),
		)
		if newErr != nil {
			t.Fatalf("new residual Outcome trading client: %v", newErr)
		}
		defer func() { _ = tradingClient.Close() }()
		if err := cleanupOutcomeBalance(ctx, tradingClient, address, outcome, baseline); err != nil {
			t.Fatalf("recover residual Outcome %s: %v", balance.Coin, err)
		}
		t.Logf("recovered residual Testnet Outcome balance %s", balance.Coin)
		return
	}
	t.Skip("no positive Testnet Outcome balance requires recovery")
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
	size, ok := outcomeOrderSizeForPrice(price)
	if !ok {
		t.Skipf("outcome minimum order at %s cannot fit within the %s USDC safety ceiling", price, testnetOutcomeMaxHoldUSD)
	}
	return size
}

func outcomeOrderSizeForPrice(price decimal.Decimal) (decimal.Decimal, bool) {
	if !price.IsPositive() {
		return decimal.Zero, false
	}
	size := minimumOutcomeOrderSize(price)
	return size, !size.Mul(price).GreaterThan(testnetOutcomeMaxHoldUSD)
}

func minimumOutcomeOrderSize(price decimal.Decimal) decimal.Decimal {
	return testnetOutcomeMinimumNotionalUSD.Div(price).Ceil()
}

func outcomeMarketAdmissionAllowed(book info.L2BookResponse, size decimal.Decimal) bool {
	if !size.IsPositive() || len(book.Levels[0]) == 0 || len(book.Levels[1]) == 0 {
		return false
	}
	bid, ask := book.Levels[0][0], book.Levels[1][0]
	if !bid.Price.IsPositive() || !ask.Price.IsPositive() || bid.Price.GreaterThanOrEqual(ask.Price) || bid.Size.LessThan(size) || ask.Size.LessThan(size) {
		return false
	}
	return ask.Price.Sub(bid.Price).Mul(size).LessThanOrEqual(testnetOutcomeMaxRoundTripLossUSD)
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
	// A missing-order response can mean the post-only order filled between observation and
	// cancellation. Confirm absence before reconciling balances so an order
	// cannot fill after the snapshot and escape reconciliation.
	if err := ensureOutcomeOrderAbsent(ctx, client, address, outcome.Symbol, cloid); err != nil {
		return err
	}
	return cleanupOutcomeBalance(ctx, client, address, outcome, baseline)
}

func cleanupOutcomeBalance(ctx context.Context, client *hyperliquid.Client, address string, outcome asset.Asset, baseline outcomeBalanceSnapshot) error {
	for {
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
// a post-only order has already filled or a prior cancellation succeeded. CLOID absence
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
	bid := significantSpotPrice(book.Levels[0][0].Price, outcome.SzDecimals, false)
	minimumSellSize := minimumOutcomeOrderSize(bid)
	if size.LessThan(minimumSellSize) {
		if len(book.Levels[1]) == 0 || !book.Levels[1][0].Price.IsPositive() {
			return fmt.Errorf("acquired outcome value %s is below the minimum exit notional and no ask liquidity is available to top up", size.Mul(bid))
		}
		ask := significantSpotPrice(book.Levels[1][0].Price, outcome.SzDecimals, true)
		supplement := minimumSellSize.Sub(size)
		minimumBuySize := minimumOutcomeOrderSize(ask)
		if supplement.LessThan(minimumBuySize) {
			supplement = minimumBuySize
		}
		if supplement.Mul(ask).GreaterThan(testnetOutcomeMaxHoldUSD) {
			return fmt.Errorf("outcome top-up %s at %s exceeds %s USDC safety ceiling", supplement, ask, testnetOutcomeMaxHoldUSD)
		}
		if ask.Sub(bid).Mul(supplement).GreaterThan(testnetOutcomeMaxRoundTripLossUSD) {
			return fmt.Errorf("outcome top-up would incur %s USDC round-trip loss above %s safety ceiling", ask.Sub(bid).Mul(supplement), testnetOutcomeMaxRoundTripLossUSD)
		}
		response, err := client.Exchange.PlaceOrder(ctx, exchange.OrderRequest{
			Coin: outcome.Symbol, IsBuy: true, Price: ask, Size: supplement,
			Type: exchange.LimitOrder{TimeInForce: exchange.TIFFrontendMarket},
		})
		if err != nil {
			return fmt.Errorf("top up outcome tokens for minimum exit: %w", err)
		}
		return requireAcceptedOrders(response, 1)
	}
	response, err := client.Exchange.PlaceOrder(ctx, exchange.OrderRequest{
		Coin: outcome.Symbol, IsBuy: false, Price: bid, Size: size,
		Type: exchange.LimitOrder{TimeInForce: exchange.TIFFrontendMarket},
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
