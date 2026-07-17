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
	"github.com/Apexllcc/hypersdk-go/info"
	"github.com/Apexllcc/hypersdk-go/types"
	"github.com/shopspring/decimal"
)

func TestSpotAssetFromMetaUsesOfficialSpotID(t *testing.T) {
	meta := info.SpotMetaResponse{
		Tokens:   []info.SpotToken{{Name: "USDC", Index: 0}, {Name: "PURR", Index: 7, SzDecimals: 2}},
		Universe: []info.SpotPair{{Name: "@7", Tokens: [2]int{7, 0}, Index: 7}},
	}
	asset, base, err := spotAssetFromMeta(meta, meta.Universe[0])
	if err != nil {
		t.Fatal(err)
	}
	if asset.ID != 10007 || asset.Symbol != "@7" || asset.SzDecimals != 2 || base != "PURR" {
		t.Fatalf("spot asset = %+v, base = %q", asset, base)
	}
}

func TestSpotMarketAdmissionRejectsInsufficientDepthAndWideSpread(t *testing.T) {

	size := decimal.NewFromInt(3)
	shallow := info.L2BookResponse{Levels: [2][]info.BookLevel{
		{{Price: decimal.RequireFromString("4.50"), Size: decimal.NewFromInt(2)}},
		{{Price: decimal.RequireFromString("4.51"), Size: decimal.NewFromInt(3)}},
	}}
	if spotMarketAdmissionAllowed(shallow, size, size) {
		t.Fatal("shallow spot book was admitted for FrontendMarket testing")
	}
	wide := info.L2BookResponse{Levels: [2][]info.BookLevel{
		{{Price: decimal.RequireFromString("4.00"), Size: decimal.NewFromInt(3)}},
		{{Price: decimal.RequireFromString("4.50"), Size: decimal.NewFromInt(3)}},
	}}
	if spotMarketAdmissionAllowed(wide, size, size) {
		t.Fatal("wide spot book was admitted for FrontendMarket testing")
	}
}

func TestRoundSpotDownToLotPreservesOnlySubLotFeeDust(t *testing.T) {
	got := roundSpotDownToLot(decimal.RequireFromString("2.99791234"), 4)
	if want := decimal.RequireFromString("2.9979"); !got.Equal(want) {
		t.Fatalf("rounded sell size = %s; want %s", got, want)
	}
}

func TestSpotMarketRoundTripRequiresFeeRepresentableLotPrecision(t *testing.T) {
	fee := decimal.RequireFromString("0.0007")
	if supportsSpotMarketRoundTrip(0, fee) {
		t.Fatal("whole-token lot precision must not be admitted for a buy/sell market round trip")
	}
	if !supportsSpotMarketRoundTrip(4, fee) {
		t.Fatal("four decimal lot precision must be admitted for an integral market size")
	}
	if supportsSpotMarketRoundTrip(4, decimal.RequireFromString("0.00049")) {
		t.Fatal("a five-decimal fee must require five-decimal asset lot precision")
	}
}

func TestSpotMarketPlanRaisesBuySizeUntilFeeAdjustedExitMeetsMinimum(t *testing.T) {
	book := info.L2BookResponse{Levels: [2][]info.BookLevel{
		{{Price: decimal.RequireFromString("4.99"), Size: decimal.NewFromInt(10)}},
		{{Price: decimal.RequireFromString("5.00"), Size: decimal.NewFromInt(10)}},
	}}
	buySize, sellSize, ok := spotMarketOrderPlanForBook(book, 4, decimal.RequireFromString("0.0007"))
	if !ok {
		t.Fatal("fee-adjusted spot plan was rejected")
	}
	if !buySize.Equal(decimal.NewFromInt(3)) || !sellSize.Equal(decimal.RequireFromString("2.9979")) {
		t.Fatalf("plan buy=%s sell=%s; want 3 and 2.9979", buySize, sellSize)
	}
	if sellSize.Mul(book.Levels[0][0].Price).LessThan(testnetOutcomeMinimumNotionalUSD) {
		t.Fatalf("fee-adjusted sell notional %s is below minimum", sellSize.Mul(book.Levels[0][0].Price))
	}
}

// spotAssetFromMeta applies the official spot encoding: 10000 plus the
// spotMeta universe index. The base token determines order-size precision.
func spotAssetFromMeta(meta info.SpotMetaResponse, pair info.SpotPair) (asset.Asset, string, error) {
	for _, token := range meta.Tokens {
		if token.Index == pair.Tokens[0] {
			return asset.Asset{ID: 10000 + pair.Index, Symbol: pair.Name, Name: pair.Name, Kind: asset.Spot, SzDecimals: token.SzDecimals}, token.Name, nil
		}
	}
	return asset.Asset{}, "", fmt.Errorf("spot pair %q has unknown base token index %d", pair.Name, pair.Tokens[0])
}

// TestTestnetSpotOrderWorkflow executes the non-taking spot lifecycle for one
// live Testnet pair: a below-bid ALO limit order, CLOID observation, and
// cancellation. Every mutation is Testnet-only and individually limited to the
// configured 100 USDC maximum notional. A FrontendMarket spot buy/sell is not
// automated here because its IOC semantics permit a sub-minimum partial fill;
// no safe, lossless cleanup exists for the resulting base-token balance.
func TestTestnetSpotOrderWorkflow(t *testing.T) {
	signingKey := requireTradingTestnet(t)
	ctx, cancel := context.WithTimeout(context.Background(), testnetWorkflowTimeout)
	defer cancel()

	metadataClient, err := hyperliquid.NewClient(hyperliquid.WithTestnet(), hyperliquid.WithHTTPTimeout(10*time.Second))
	if err != nil {
		t.Fatalf("new Testnet spot metadata client: %v", err)
	}
	defer func() { _ = metadataClient.Close() }()
	spotAsset, baseCoin, book, err := testnetSpotLimitMarket(ctx, metadataClient, signingKey.Address().Hex())
	if err != nil {
		t.Skipf("no safe Testnet USDC spot market is available: %v", err)
	}
	client, err := hyperliquid.NewClient(
		hyperliquid.WithTestnet(),
		hyperliquid.WithDigestSigner(signingKey),
		hyperliquid.WithAssetResolver(asset.NewStaticResolver([]asset.Asset{spotAsset})),
		hyperliquid.WithHTTPTimeout(10*time.Second),
	)
	if err != nil {
		t.Fatalf("new Testnet spot client: %v", err)
	}
	defer func() { _ = client.Close() }()

	address := signingKey.Address().Hex()
	abstraction, err := client.Info.UserAbstraction(ctx, address)
	if err != nil {
		t.Fatalf("read Testnet account abstraction: %v", err)
	}
	requireUnifiedTradingAcknowledgement(t, abstraction)
	baseline, err := spotBalances(ctx, client, address, baseCoin)
	if err != nil {
		t.Fatalf("read Testnet spot balance baseline: %v", err)
	}
	if err := requireNoSpotExposure(ctx, client, address, spotAsset, baseline); err != nil {
		t.Skipf("Testnet spot account is not clean; no order submitted: %v", err)
	}

	market := types.MarketRef{Symbol: spotAsset.Symbol, Kind: types.Spot}
	limitPrice := significantSpotPrice(book.Levels[0][0].Price.Mul(half), spotAsset.SzDecimals, false)
	limitSize := spotMinimumOrderSize(t, limitPrice, spotAsset.SzDecimals)
	preflightSpotOrder(t, baseline, limitPrice.Mul(limitSize))
	limitCloid := newCloid(t)
	limitCleanupArmed := true
	defer func() {
		if !limitCleanupArmed {
			return
		}
		cleanupCtx, cleanupCancel := cleanupContext()
		defer cleanupCancel()
		if err := cleanupSpotOrder(cleanupCtx, client, address, spotAsset, baseCoin, limitCloid, baseline); err != nil {
			t.Errorf("cleanup Testnet spot limit order: %v", err)
		}
	}()
	response, err := client.Exchange.PlaceOrder(ctx, exchange.OrderRequest{
		Market: &market, IsBuy: true, Price: limitPrice, Size: limitSize,
		Type: exchange.LimitOrder{TimeInForce: exchange.TIFALO}, ClientOrderID: &limitCloid,
	})
	if err != nil {
		t.Fatalf("place Testnet spot ALO: %v", err)
	}
	if err := requireAcceptedOrders(response, 1); err != nil {
		t.Fatalf("Testnet spot ALO was rejected: %v", err)
	}
	if err := waitForSpotCloid(ctx, client, address, limitCloid, true); err != nil {
		t.Fatal(err)
	}
	if err := cancelAndConfirmSpotOrder(ctx, client, address, spotAsset.Symbol, limitCloid); err != nil {
		t.Fatalf("cancel Testnet spot ALO: %v", err)
	}
	if err := waitForSpotBalanceBaseline(ctx, client, address, baseCoin, spotAsset.SzDecimals, baseline); err != nil {
		t.Fatal(err)
	}
	limitCleanupArmed = false
	t.Logf("verified Testnet spot ALO order %s, CLOID lookup, cancellation, and %s balance baseline", spotAsset.Symbol, baseCoin)

}

func testnetSpotLimitMarket(ctx context.Context, client *hyperliquid.Client, address string) (asset.Asset, string, info.L2BookResponse, error) {
	meta, err := client.Info.SpotMeta(ctx)
	if err != nil {
		return asset.Asset{}, "", info.L2BookResponse{}, err
	}
	state, err := client.Info.SpotClearinghouseState(ctx, address)
	if err != nil {
		return asset.Asset{}, "", info.L2BookResponse{}, fmt.Errorf("read Testnet spot balances: %w", err)
	}
	nonZeroBalances := make(map[string]bool, len(state.Balances))
	for _, balance := range state.Balances {
		nonZeroBalances[balance.Coin] = !balance.Total.IsZero()
	}
	tokens := make(map[int]info.SpotToken, len(meta.Tokens))
	for _, token := range meta.Tokens {
		tokens[token.Index] = token
	}
	for _, pair := range meta.Universe {
		base, baseOK := tokens[pair.Tokens[0]]
		quote, quoteOK := tokens[pair.Tokens[1]]
		if !baseOK || !quoteOK || base.Name == "USDC" || quote.Name != "USDC" || pair.Name == "" {
			continue
		}
		spotAsset, baseCoin, assetErr := spotAssetFromMeta(meta, pair)
		if assetErr != nil {
			continue
		}
		if nonZeroBalances[baseCoin] {
			continue
		}
		book, bookErr := client.Info.L2Book(ctx, spotAsset.Symbol)
		if bookErr != nil || len(book.Levels[0]) == 0 || !book.Levels[0][0].Price.IsPositive() {
			continue
		}
		limitPrice := significantSpotPrice(book.Levels[0][0].Price.Mul(half), spotAsset.SzDecimals, false)
		if _, ok := spotOrderSizeForPrice(limitPrice, spotAsset.SzDecimals); !ok {
			continue
		}
		return spotAsset, baseCoin, book, nil
	}
	return asset.Asset{}, "", info.L2BookResponse{}, fmt.Errorf("no USDC-quoted pair has bid liquidity")
}

type spotBalanceSnapshot struct {
	token         decimal.Decimal
	usdcHold      decimal.Decimal
	usdcAvailable decimal.Decimal
}

func spotBalances(ctx context.Context, client *hyperliquid.Client, address, baseCoin string) (spotBalanceSnapshot, error) {
	state, err := client.Info.SpotClearinghouseState(ctx, address)
	if err != nil {
		return spotBalanceSnapshot{}, fmt.Errorf("read spot balance: %w", err)
	}
	usdc, err := usdcSpotBalance(state)
	if err != nil {
		return spotBalanceSnapshot{}, err
	}
	balances := spotBalanceSnapshot{usdcHold: usdc.Hold, usdcAvailable: usdc.Total.Sub(usdc.Hold)}
	for _, balance := range state.Balances {
		if balance.Coin == baseCoin {
			balances.token = balance.Total
			break
		}
	}
	return balances, nil
}

func requireNoSpotExposure(ctx context.Context, client *hyperliquid.Client, address string, spotAsset asset.Asset, baseline spotBalanceSnapshot) error {
	orders, err := client.Info.OpenOrders(ctx, address)
	if err != nil {
		return fmt.Errorf("read open orders: %w", err)
	}
	for _, order := range orders {
		if order.Coin == spotAsset.Symbol {
			return fmt.Errorf("account already has spot open order %d", order.OID)
		}
	}
	if !baseline.token.IsZero() {
		return fmt.Errorf("account already has %s spot balance", baseline.token)
	}
	return nil
}

func preflightSpotOrder(t *testing.T, baseline spotBalanceSnapshot, notional decimal.Decimal) {
	t.Helper()
	if baseline.usdcAvailable.LessThan(notional) {
		t.Skipf("Testnet USDC available %s is below spot order notional %s", baseline.usdcAvailable, notional)
	}
	if baseline.usdcHold.Add(notional).GreaterThan(testnetOutcomeMaxHoldUSD) {
		t.Skipf("spot order would increase Testnet USDC hold to %s, above %s safety ceiling", baseline.usdcHold.Add(notional), testnetOutcomeMaxHoldUSD)
	}
}

func spotMinimumOrderSize(t *testing.T, price decimal.Decimal, szDecimals int) decimal.Decimal {
	t.Helper()
	size, ok := spotOrderSizeForPrice(price, szDecimals)
	if !ok {
		t.Skipf("spot minimum order at %s cannot fit within the %s USDC safety ceiling", price, testnetOutcomeMaxHoldUSD)
	}
	return size
}

// spotMarketOrderPlanForBook chooses an integral buy size such that, after the
// actual taker fee is deducted in the received asset and the amount is rounded
// down to its lot, the sell can still meet Hyperliquid's minimum notional.
func spotMarketOrderPlanForBook(book info.L2BookResponse, szDecimals int, takerFeeRate decimal.Decimal) (buySize, sellSize decimal.Decimal, ok bool) {
	one := decimal.NewFromInt(1)
	if szDecimals < 0 || takerFeeRate.IsNegative() || takerFeeRate.GreaterThanOrEqual(one) || len(book.Levels[0]) == 0 || len(book.Levels[1]) == 0 {
		return decimal.Zero, decimal.Zero, false
	}
	bid, ask := book.Levels[0][0].Price, book.Levels[1][0].Price
	if !bid.IsPositive() || !ask.IsPositive() || bid.GreaterThanOrEqual(ask) {
		return decimal.Zero, decimal.Zero, false
	}
	buySize = testnetOutcomeMinimumNotionalUSD.Div(bid).Div(one.Sub(takerFeeRate)).Ceil()
	for buySize.Mul(ask).LessThanOrEqual(testnetOutcomeMaxHoldUSD) {
		sellSize = roundSpotDownToLot(buySize.Mul(one.Sub(takerFeeRate)), szDecimals)
		if sellSize.IsPositive() && !sellSize.Mul(bid).LessThan(testnetOutcomeMinimumNotionalUSD) {
			return buySize, sellSize, true
		}
		buySize = buySize.Add(one)
	}
	return decimal.Zero, decimal.Zero, false
}

func supportsSpotMarketRoundTrip(szDecimals int, takerFeeRate decimal.Decimal) bool {
	if szDecimals < 0 || takerFeeRate.IsNegative() {
		return false
	}
	feeDecimals := 0
	if takerFeeRate.Exponent() < 0 {
		feeDecimals = int(-takerFeeRate.Exponent())
	}
	return szDecimals >= feeDecimals
}

func spotOrderSizeForPrice(price decimal.Decimal, szDecimals int) (decimal.Decimal, bool) {
	if !price.IsPositive() || szDecimals < 0 {
		return decimal.Zero, false
	}
	step := decimal.New(1, -int32(szDecimals))
	size := testnetOutcomeMinimumNotionalUSD.Div(price).Div(step).Ceil().Mul(step)
	return size, size.IsPositive() && !size.Mul(price).GreaterThan(testnetOutcomeMaxHoldUSD)
}

func significantSpotPrice(value decimal.Decimal, szDecimals int, roundUp bool) decimal.Decimal {
	canonical, err := decimal.NewFromString(value.String())
	if err != nil || !canonical.IsPositive() {
		panic(fmt.Sprintf("invalid positive spot price %s", value))
	}
	maxDecimals := 8 - szDecimals
	if maxDecimals < 0 {
		panic(fmt.Sprintf("invalid spot size precision %d", szDecimals))
	}
	if canonical.Equal(canonical.Truncate(0)) {
		return canonical
	}
	allowedDecimals := 5 - integerDigits(canonical)
	if canonical.LessThan(decimal.NewFromInt(1)) {
		allowedDecimals = 5 + leadingFractionalZeros(canonical)
	}
	if allowedDecimals > maxDecimals {
		allowedDecimals = maxDecimals
	}
	step := decimal.New(1, -int32(allowedDecimals))
	if roundUp {
		return canonical.Div(step).Ceil().Mul(step)
	}
	return canonical.Div(step).Floor().Mul(step)
}

func spotMarketAdmissionAllowed(book info.L2BookResponse, buySize, sellSize decimal.Decimal) bool {
	if !buySize.IsPositive() || !sellSize.IsPositive() || len(book.Levels[0]) == 0 || len(book.Levels[1]) == 0 {
		return false
	}
	bid, ask := book.Levels[0][0], book.Levels[1][0]
	if !bid.Price.IsPositive() || !ask.Price.IsPositive() || bid.Price.GreaterThanOrEqual(ask.Price) || bid.Size.LessThan(sellSize) || ask.Size.LessThan(buySize) {
		return false
	}
	return ask.Price.Sub(bid.Price).Mul(buySize).LessThanOrEqual(testnetOutcomeMaxRoundTripLossUSD)
}

func roundSpotDownToLot(size decimal.Decimal, szDecimals int) decimal.Decimal {
	if !size.IsPositive() || szDecimals < 0 {
		return decimal.Zero
	}
	step := decimal.New(1, -int32(szDecimals))
	return size.Div(step).Floor().Mul(step)
}

func cancelAndConfirmSpotOrder(ctx context.Context, client *hyperliquid.Client, address, coin string, cloid types.Cloid) error {
	response, cancelErr := client.Exchange.CancelByCloid(ctx, exchange.CancelByCloidRequest{Coin: coin, Cloid: cloid})
	if cancelErr == nil {
		cancelErr = requireAcceptedCancels(response, 1)
	}
	if outcomeErr := cancellationOutcome(cancelErr, waitForSpotCloid(ctx, client, address, cloid, false)); outcomeErr != nil {
		return fmt.Errorf("cancel spot order: %w", outcomeErr)
	}
	return nil
}

func waitForSpotCloid(ctx context.Context, client *hyperliquid.Client, address string, cloid types.Cloid, wantOpen bool) error {
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
		return fmt.Errorf("spot order %s was not visible", cloid)
	}
	return fmt.Errorf("spot order %s is still open", cloid)
}

func cleanupSpotOrder(ctx context.Context, client *hyperliquid.Client, address string, spotAsset asset.Asset, baseCoin string, cloid types.Cloid, baseline spotBalanceSnapshot) error {
	for {
		if cloid != (types.Cloid{}) {
			// A missing order is normal after a successful cancellation or a fill;
			// CLOID absence, not the cancel response, is the cleanup condition.
			_, _ = client.Exchange.CancelByCloid(ctx, exchange.CancelByCloidRequest{Coin: spotAsset.Symbol, Cloid: cloid})
			if err := waitForSpotCloid(ctx, client, address, cloid, false); err != nil {
				return err
			}
		}
		balances, err := spotBalances(ctx, client, address, baseCoin)
		if err != nil {
			return err
		}
		if spotBalancesReconciled(balances, baseline, spotAsset.SzDecimals) {
			return nil
		}
		if balances.token.GreaterThan(baseline.token) {
			if err := exitSpotTokens(ctx, client, spotAsset, balances.token.Sub(baseline.token)); err != nil {
				return fmt.Errorf("exit unexpectedly acquired spot tokens: %w", err)
			}
		}
		select {
		case <-ctx.Done():
			return fmt.Errorf("spot cleanup did not return balances to baseline: %w", ctx.Err())
		case <-time.After(100 * time.Millisecond):
		}
	}
}

func exitSpotTokens(ctx context.Context, client *hyperliquid.Client, spotAsset asset.Asset, size decimal.Decimal) error {
	book, err := client.Info.L2Book(ctx, spotAsset.Symbol)
	if err != nil {
		return err
	}
	if len(book.Levels[0]) == 0 || !book.Levels[0][0].Price.IsPositive() {
		return fmt.Errorf("no spot bid liquidity")
	}
	price := significantSpotPrice(book.Levels[0][0].Price, spotAsset.SzDecimals, false)
	size = roundSpotDownToLot(size, spotAsset.SzDecimals)
	if size.Mul(price).LessThan(testnetOutcomeMinimumNotionalUSD) {
		return fmt.Errorf("acquired spot value %s is below the minimum exit notional", size.Mul(price))
	}
	response, err := client.Exchange.PlaceOrder(ctx, exchange.OrderRequest{
		Coin: spotAsset.Symbol, IsBuy: false, Price: price, Size: size,
		Type: exchange.LimitOrder{TimeInForce: exchange.TIFFrontendMarket},
	})
	if err != nil {
		return err
	}
	return requireAcceptedOrders(response, 1)
}

func waitForSpotBalanceBaseline(ctx context.Context, client *hyperliquid.Client, address, baseCoin string, szDecimals int, baseline spotBalanceSnapshot) error {
	deadline := time.Now().Add(testnetStateWaitTimeout)
	for time.Now().Before(deadline) {
		balances, err := spotBalances(ctx, client, address, baseCoin)
		if err == nil && spotBalancesReconciled(balances, baseline, szDecimals) {
			return nil
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(100 * time.Millisecond):
		}
	}
	return fmt.Errorf("spot token balance or USDC hold did not return to baseline")
}

// spotBalancesReconciled permits only sub-lot positive base-token fee dust.
// The API rejects that dust as an order size, so retrying a FrontendMarket sell
// could not remove it and would otherwise turn a successful safety test into an
// unbounded cleanup loop. USDC collateral must still return exactly to baseline.
func spotBalancesReconciled(actual, baseline spotBalanceSnapshot, szDecimals int) bool {
	if !actual.usdcHold.Equal(baseline.usdcHold) || actual.token.LessThan(baseline.token) {
		return false
	}
	residual := actual.token.Sub(baseline.token)
	if residual.IsZero() {
		return true
	}
	if szDecimals < 0 {
		return false
	}
	return residual.LessThan(decimal.New(1, -int32(szDecimals)))
}
