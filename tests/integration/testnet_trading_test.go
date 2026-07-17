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

	hyperliquid "github.com/Apexllcc/hypersdk-go"
	"github.com/Apexllcc/hypersdk-go/asset"
	"github.com/Apexllcc/hypersdk-go/exchange"
	"github.com/Apexllcc/hypersdk-go/info"
	"github.com/Apexllcc/hypersdk-go/signer"
	"github.com/Apexllcc/hypersdk-go/types"
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

func spotTokenWireName(meta info.SpotMetaResponse, balance info.SpotBalance) (string, error) {
	for _, token := range meta.Tokens {
		if token.Index == balance.Token && token.Name == balance.Coin && token.IsCanonical && token.TokenID != "" {
			return token.Name + ":" + token.TokenID, nil
		}
	}
	return "", fmt.Errorf("canonical spot token %q at index %d was not found in metadata", balance.Coin, balance.Token)
}

func existingSpotSendLedgerHashes(ctx context.Context, client *hyperliquid.Client, sender, recipient string, startTime int64, amount decimal.Decimal) (map[string]struct{}, error) {
	updates, err := client.Info.UserNonFundingLedgerUpdates(ctx, sender, startTime, nil)
	if err != nil {
		return nil, err
	}
	known := make(map[string]struct{})
	for _, update := range updates {
		if isMatchingSpotSendLedger(update, recipient, amount) && update.Hash != "" {
			known[update.Hash] = struct{}{}
		}
	}
	return known, nil
}

func waitForSpotSendLedger(ctx context.Context, client *hyperliquid.Client, sender, recipient string, startTime int64, amount decimal.Decimal, known map[string]struct{}) error {
	deadline := time.Now().Add(testnetStateWaitTimeout)
	var latestErr error
	for time.Now().Before(deadline) {
		updates, err := client.Info.UserNonFundingLedgerUpdates(ctx, sender, startTime, nil)
		if err == nil {
			for _, update := range updates {
				if isMatchingSpotSendLedger(update, recipient, amount) && update.Hash != "" {
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
		return fmt.Errorf("sender ledger did not confirm sendAsset of %s to %s: %v", amount, recipient, latestErr)
	}
	return fmt.Errorf("sender ledger did not confirm sendAsset of %s to %s", amount, recipient)
}

func isMatchingSpotSendLedger(update info.NonFundingLedgerUpdate, recipient string, amount decimal.Decimal) bool {
	return update.Delta.Type == "send" && update.Delta.SourceDEX == "spot" && update.Delta.DestinationDEX == "spot" && update.Delta.Token == "USDC" && strings.EqualFold(update.Delta.Destination, recipient) && update.Delta.Amount.Abs().Equal(amount)
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

func requireAcceptedCancels(response exchange.ActionResponse, expected int) error {
	data, ok := response.Response.Data.(exchange.CancelResponseData)
	if !ok || response.Response.Type != exchange.ActionResponseCancel {
		return cancelResponseErrorf("unexpected cancel response type %q", response.Response.Type)
	}
	if len(data.Statuses) != expected {
		return cancelResponseErrorf("cancel response has %d statuses, expected %d", len(data.Statuses), expected)
	}
	for index, status := range data.Statuses {
		if status.Error != nil {
			return cancelResponseErrorf("cancel %d rejected: %s", index, *status.Error)
		}
		if status.Success == nil {
			return cancelResponseErrorf("cancel %d has no accepted status", index)
		}
		if *status.Success != "success" {
			return cancelResponseErrorf("cancel %d returned unexpected success value %q", index, *status.Success)
		}
	}
	return nil
}

type cancelResponseValidationError struct{ err error }

func (e *cancelResponseValidationError) Error() string { return e.err.Error() }

func (e *cancelResponseValidationError) Unwrap() error { return e.err }

func cancelResponseErrorf(format string, args ...any) error {
	return &cancelResponseValidationError{err: fmt.Errorf(format, args...)}
}

func isDefinitiveCancelRejection(err error) bool {
	var actionErr *exchange.ActionResponseError
	if errors.As(err, &actionErr) {
		return true
	}
	var validationErr *cancelResponseValidationError
	return errors.As(err, &validationErr)
}

func cancellationOutcome(cancelErr, confirmationErr error) error {
	if confirmationErr == nil {
		if isDefinitiveCancelRejection(cancelErr) {
			return cancelErr
		}
		return nil
	}
	if cancelErr == nil {
		return confirmationErr
	}
	return fmt.Errorf("%w; confirm order state: %v", cancelErr, confirmationErr)
}

func cancelAndConfirmBTCOrder(ctx context.Context, client *hyperliquid.Client, address string, cloid types.Cloid) error {
	response, cancelErr := client.Exchange.CancelByCloid(ctx, exchange.CancelByCloidRequest{Coin: testnetBTC, Cloid: cloid})
	if cancelErr == nil {
		cancelErr = requireAcceptedCancels(response, 1)
	}
	if outcomeErr := cancellationOutcome(cancelErr, waitForCloidAbsent(ctx, client, address, cloid)); outcomeErr != nil {
		return fmt.Errorf("cancel BTC order: %w", outcomeErr)
	}
	return nil
}

func cancelAndConfirmBTCOrderOID(ctx context.Context, client *hyperliquid.Client, address string, oid uint64) error {
	response, cancelErr := client.Exchange.CancelOrder(ctx, exchange.CancelRequest{Coin: testnetBTC, OID: oid})
	if cancelErr == nil {
		cancelErr = requireAcceptedCancels(response, 1)
	}
	status, statusErr := client.Info.OrderStatus(ctx, address, oid)
	var confirmationErr error
	if statusErr != nil {
		confirmationErr = fmt.Errorf("read canceled BTC order status: %w", statusErr)
	} else if status.Status != "canceled" {
		confirmationErr = fmt.Errorf("BTC order %d status is %q after numeric cancel", oid, status.Status)
	}
	if outcomeErr := cancellationOutcome(cancelErr, confirmationErr); outcomeErr != nil {
		return fmt.Errorf("cancel BTC order by OID: %w", outcomeErr)
	}
	return nil
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
