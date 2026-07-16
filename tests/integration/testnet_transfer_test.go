//go:build integration && testnet

package integration

import (
	"context"
	"errors"
	"fmt"
	"os"
	"testing"
	"time"

	hyperliquid "github.com/Apexllcc/hyperliquid-go-sdk"
	"github.com/Apexllcc/hyperliquid-go-sdk/exchange"
)

// TestTestnetUnifiedSendAssetWorkflow sends exactly one Testnet Spot USDC to
// the recipient explicitly supplied by the user. sendAsset is the documented
// generalized route for transfers from a Unified account's spot namespace.
func TestTestnetUnifiedSendAssetWorkflow(t *testing.T) {
	signingKey := requireTradingTestnet(t)
	if os.Getenv(testnetTransferEnableEnv) != "1" {
		t.Skip("set HL_TESTNET_TRANSFER=1 to enable the one-way Testnet Spot USDC transfer validation")
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
		t.Skipf("Testnet Spot USDC transfer workflow currently requires Unified/Portfolio sender collateral, got %s", senderAbstraction)
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
		t.Skipf("Testnet sender has insufficient available Spot USDC for one-USDC transfer: %s", senderUSDC.Total.Sub(senderUSDC.Hold))
	}
	spotMeta, err := client.Info.SpotMeta(ctx)
	if err != nil {
		t.Fatalf("read Testnet spot metadata: %v", err)
	}
	usdcToken, err := spotTokenWireName(spotMeta, senderUSDC)
	if err != nil {
		t.Fatal(err)
	}
	ledgerSnapshotStart := time.Now().Add(-15 * time.Minute).UnixMilli()
	knownTransferHashes, err := existingSpotSendLedgerHashes(ctx, client, sender, recipient, ledgerSnapshotStart, testnetTransferUSD)
	if err != nil {
		t.Fatalf("snapshot prior Testnet spotSend ledger entries: %v", err)
	}
	ledgerStart := time.Now().Add(-time.Second).UnixMilli()
	response, err := client.Exchange.SendAsset(ctx, exchange.SendAssetRequest{
		Destination:    testnetTransferRecipient,
		SourceDEX:      "spot",
		DestinationDEX: "spot",
		Token:          usdcToken,
		Amount:         testnetTransferUSD,
	})
	if err != nil {
		var actionErr *exchange.ActionResponseError
		if errors.As(err, &actionErr) {
			t.Fatalf("Testnet sendAsset was definitively rejected by the exchange: %v", actionErr)
		}
		reconcileCtx, reconcileCancel := cleanupContext()
		defer reconcileCancel()
		if reconcileErr := waitForSpotSendLedger(reconcileCtx, client, sender, recipient, ledgerStart, testnetTransferUSD, knownTransferHashes); reconcileErr == nil {
			t.Log("Testnet sendAsset response was lost but sender ledger confirms the transfer")
			return
		} else {
			t.Fatalf("Testnet sendAsset outcome is unknown; do not retry automatically: %v; reconciliation: %v", err, reconcileErr)
		}
	}
	if response.Status != "ok" || response.Response.Type != exchange.ActionResponseDefault {
		reconcileCtx, reconcileCancel := cleanupContext()
		defer reconcileCancel()
		if reconcileErr := waitForSpotSendLedger(reconcileCtx, client, sender, recipient, ledgerStart, testnetTransferUSD, knownTransferHashes); reconcileErr == nil {
			t.Logf("Testnet sendAsset returned an unexpected envelope but sender ledger confirms the transfer (status=%q type=%q)", response.Status, response.Response.Type)
			return
		} else {
			t.Fatalf("Testnet sendAsset outcome is unknown; do not retry automatically: unexpected status=%q type=%q; reconciliation: %v", response.Status, response.Response.Type, reconcileErr)
		}
	}
	if err := waitForSpotSendLedger(ctx, client, sender, recipient, ledgerStart, testnetTransferUSD, knownTransferHashes); err != nil {
		t.Fatal(err)
	}
	if recipientState, recipientErr := client.Info.SpotClearinghouseState(ctx, recipient); recipientErr != nil {
		t.Logf("read Testnet recipient spot state after sendAsset: %v", recipientErr)
	} else if balance, found := usdcSpotBalanceOrZero(recipientState); found {
		t.Logf("recipient Testnet spot USDC total after usdSend: %s", balance.Total)
	}
	t.Logf("verified one Testnet Spot USDC sendAsset transfer to %s", recipient)
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
