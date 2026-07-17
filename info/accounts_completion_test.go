package info_test

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/Apexllcc/hypersdk-go/info"
	"github.com/Apexllcc/hypersdk-go/internal/hlerr"
	"github.com/Apexllcc/hypersdk-go/transport"
)

func TestInfoAccountCompletionFixturesUseOfficialWiresAndPreserveDecimals(t *testing.T) {
	t.Parallel()
	serverErr := make(chan error, 1)
	recordServerErr := func(err error) {
		select {
		case serverErr <- err:
		default:
		}
	}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var request struct {
			Type      string `json:"type"`
			Coin      string `json:"coin"`
			User      string `json:"user"`
			StartTime *int64 `json:"startTime"`
			EndTime   *int64 `json:"endTime"`
		}
		if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
			recordServerErr(err)
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		if request.User != "0xabc" && request.Type != "fundingHistory" {
			recordServerErr(fmt.Errorf("request user = %q", request.User))
		}
		switch request.Type {
		case "fundingHistory":
			if request.Coin != "BTC" || request.StartTime == nil || *request.StartTime != 10 || request.EndTime == nil || *request.EndTime != 20 {
				recordServerErr(fmt.Errorf("fundingHistory request = %#v", request))
			}
			_, _ = w.Write([]byte(`[{"coin":"BTC","fundingRate":"-0.0001","premium":"0.00125","time":20}]`))
		case "delegatorSummary":
			_, _ = w.Write([]byte(`{"delegated":"2.5","undelegated":"1.25","totalPendingWithdrawal":"0.5","nPendingWithdrawals":3}`))
		case "clearinghouseState":
			_, _ = w.Write([]byte(`{"assetPositions":[{"type":"oneWay","position":{"coin":"BTC","cumFunding":{"allTime":"1","sinceOpen":"2","sinceChange":"3"},"entryPx":"100","leverage":{"type":"isolated","value":5,"rawUsd":"7.5"},"liquidationPx":null,"marginUsed":"4","maxLeverage":10,"maxTradeSzs":["11.1","12.2"],"positionValue":"8","returnOnEquity":"-0.1","szi":"-0.25","unrealizedPnl":"0.125"}}],"crossMaintenanceMarginUsed":"0","crossMarginSummary":{"accountValue":"1","totalMarginUsed":"0","totalNtlPos":"0","totalRawUsd":"0"},"marginSummary":{"accountValue":"1","totalMarginUsed":"0","totalNtlPos":"0","totalRawUsd":"0"},"time":1,"withdrawable":"1"}`))
		case "spotClearinghouseState":
			_, _ = w.Write([]byte(`{"portfolioMarginEnabled":true,"balances":[{"coin":"USDC","token":0,"total":"7.5","hold":"1.5","spotHold":"0.5","entryNtl":"2","ltv":"0.1","borrowed":"0.2","supplied":"8"}],"evmEscrows":[{"coin":"USDC","token":0,"total":"0.25"}],"portfolioMarginRatio":"0.4","tokenToPortfolioBorrowRatio":[[0,"0.5"]],"tokenToAvailableAfterMaintenance":[[0,"6.5"]]}`))
		default:
			recordServerErr(fmt.Errorf("unexpected request type %q", request.Type))
			w.WriteHeader(http.StatusBadRequest)
		}
	}))
	defer server.Close()
	client := info.NewClient(server.URL, transport.NewDefaultHTTPTransport(nil), time.Second, "test")

	start, end := int64(10), int64(20)
	funding, err := client.FundingHistory(context.Background(), "BTC", start, &end)
	if err != nil || funding[0].FundingRate.String() != "-0.0001" || funding[0].Premium.String() != "0.00125" {
		t.Fatalf("funding=%+v err=%v", funding, err)
	}
	delegator, err := client.DelegatorSummary(context.Background(), "0xabc")
	if err != nil || delegator.Delegated.String() != "2.5" || delegator.PendingWithdrawals != 3 {
		t.Fatalf("delegator=%+v err=%v", delegator, err)
	}
	perp, err := client.ClearinghouseState(context.Background(), "0xabc")
	if err != nil || perp.AssetPositions[0].Position.MaxTradeSizes[1].String() != "12.2" || perp.AssetPositions[0].Position.Leverage.RawUsd.String() != "7.5" {
		t.Fatalf("perp=%+v err=%v", perp, err)
	}
	spot, err := client.SpotClearinghouseState(context.Background(), "0xabc")
	if err != nil || spot.PortfolioMarginEnabled == nil || !*spot.PortfolioMarginEnabled || spot.Balances[0].Supplied.String() != "8" || spot.EVMEscrows[0].Total.String() != "0.25" || spot.TokenToAvailableAfterMaintenance[0].Amount.String() != "6.5" {
		t.Fatalf("spot=%+v err=%v", spot, err)
	}
	select {
	case err := <-serverErr:
		t.Fatal(err)
	default:
	}
}

func TestClearinghouseStateTreatsOfficialNaNLiquidationPriceAsUnavailable(t *testing.T) {
	t.Parallel()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{"assetPositions":[{"type":"oneWay","position":{"coin":"BTC","cumFunding":{"allTime":"0","sinceOpen":"0","sinceChange":"0"},"entryPx":null,"leverage":{"type":"cross","value":5},"liquidationPx":"NaN","marginUsed":"0","maxLeverage":10,"maxTradeSzs":["0","0"],"positionValue":"0","returnOnEquity":"0","szi":"0","unrealizedPnl":"0"}}],"crossMaintenanceMarginUsed":"0","crossMarginSummary":{"accountValue":"0","totalMarginUsed":"0","totalNtlPos":"0","totalRawUsd":"0"},"marginSummary":{"accountValue":"0","totalMarginUsed":"0","totalNtlPos":"0","totalRawUsd":"0"},"time":1,"withdrawable":"0"}`))
	}))
	defer server.Close()
	client := info.NewClient(server.URL, transport.NewDefaultHTTPTransport(nil), time.Second, "test")
	state, err := client.ClearinghouseState(context.Background(), "0xabc")
	if err != nil || state.AssetPositions[0].Position.LiquidationPx != nil {
		t.Fatalf("state=%+v err=%v", state, err)
	}
}

func TestClearinghouseStateForDEXUsesOfficialDEXWire(t *testing.T) {
	t.Parallel()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var request info.ClearinghouseStateRequest
		if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		if request.Type != "clearinghouseState" || request.User != "0xabc" || request.DEX != "xyz" {
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		_, _ = w.Write([]byte(`{"assetPositions":[],"crossMaintenanceMarginUsed":"0","crossMarginSummary":{"accountValue":"1","totalMarginUsed":"0","totalNtlPos":"0","totalRawUsd":"0"},"marginSummary":{"accountValue":"1","totalMarginUsed":"0","totalNtlPos":"0","totalRawUsd":"0"},"time":1,"withdrawable":"1"}`))
	}))
	defer server.Close()
	client := info.NewClient(server.URL, transport.NewDefaultHTTPTransport(nil), time.Second, "test")
	state, err := client.ClearinghouseStateForDEX(context.Background(), "0xabc", "xyz")
	if err != nil || state.MarginSummary.AccountValue.String() != "1" {
		t.Fatalf("state=%+v err=%v", state, err)
	}
}

func TestAccountCompletionEndpointsReturnAPIErrorsAndHonorCancellation(t *testing.T) {
	t.Parallel()
	apiFailure := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte(`{"code":"bad_request","message":"fixture error"}`))
	}))
	defer apiFailure.Close()
	client := info.NewClient(apiFailure.URL, transport.NewDefaultHTTPTransport(nil), time.Second, "test")
	if _, err := client.DelegatorSummary(context.Background(), "0xabc"); err == nil {
		t.Fatal("DelegatorSummary accepted API failure")
	} else {
		var apiErr *hlerr.APIError
		if !errors.As(err, &apiErr) || apiErr.Code != "bad_request" {
			t.Fatalf("error = %v, want APIError", err)
		}
	}

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := client.FundingHistory(ctx, "BTC", 0, nil); !errors.Is(err, context.Canceled) {
		t.Fatalf("FundingHistory error = %v, want context.Canceled", err)
	}
}

func TestAccountEndpointsPropagateAPIErrorsTimeoutsAndCanceledContexts(t *testing.T) {
	t.Parallel()
	apiFailure := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte(`{"code":"bad_request","message":"fixture error"}`))
	}))
	defer apiFailure.Close()

	releaseDeadlineServer := make(chan struct{})
	deadlineServer := httptest.NewServer(http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) {
		select {
		case <-r.Context().Done():
		case <-releaseDeadlineServer:
		}
	}))
	defer func() {
		close(releaseDeadlineServer)
		deadlineServer.Close()
	}()

	type endpointCall struct {
		name string
		call func(*info.Client, context.Context) error
	}
	start := int64(1)
	calls := []endpointCall{
		{"portfolio", func(c *info.Client, ctx context.Context) error { _, err := c.Portfolio(ctx, "0xabc"); return err }},
		{"userFunding", func(c *info.Client, ctx context.Context) error {
			_, err := c.UserFunding(ctx, "0xabc", &start, nil)
			return err
		}},
		{"userFees", func(c *info.Client, ctx context.Context) error { _, err := c.UserFees(ctx, "0xabc"); return err }},
		{"userRateLimit", func(c *info.Client, ctx context.Context) error { _, err := c.UserRateLimit(ctx, "0xabc"); return err }},
		{"delegatorSummary", func(c *info.Client, ctx context.Context) error {
			_, err := c.DelegatorSummary(ctx, "0xabc")
			return err
		}},
		{"subAccounts", func(c *info.Client, ctx context.Context) error { _, err := c.Subaccounts(ctx, "0xabc"); return err }},
		{"vaultDetails", func(c *info.Client, ctx context.Context) error {
			_, err := c.VaultDetails(ctx, "0xvault", nil)
			return err
		}},
		{"fundingHistory", func(c *info.Client, ctx context.Context) error {
			_, err := c.FundingHistory(ctx, "BTC", start, nil)
			return err
		}},
		{"clearinghouseState", func(c *info.Client, ctx context.Context) error {
			_, err := c.ClearinghouseState(ctx, "0xabc")
			return err
		}},
		{"spotClearinghouseState", func(c *info.Client, ctx context.Context) error {
			_, err := c.SpotClearinghouseState(ctx, "0xabc")
			return err
		}},
	}

	for _, tc := range calls {
		t.Run(tc.name+"/api_error", func(t *testing.T) {
			client := info.NewClient(apiFailure.URL, transport.NewDefaultHTTPTransport(nil), time.Second, "test")
			var apiErr *hlerr.APIError
			if err := tc.call(client, context.Background()); !errors.As(err, &apiErr) || apiErr.Code != "bad_request" {
				t.Fatalf("error = %v, want APIError", err)
			}
		})
		t.Run(tc.name+"/timeout", func(t *testing.T) {
			client := info.NewClient(deadlineServer.URL, transport.NewDefaultHTTPTransport(nil), 10*time.Millisecond, "test")
			if err := tc.call(client, context.Background()); !errors.Is(err, context.DeadlineExceeded) {
				t.Fatalf("error = %v, want context.DeadlineExceeded", err)
			}
		})
		t.Run(tc.name+"/canceled", func(t *testing.T) {
			client := info.NewClient(apiFailure.URL, transport.NewDefaultHTTPTransport(nil), time.Second, "test")
			ctx, cancel := context.WithCancel(context.Background())
			cancel()
			if err := tc.call(client, ctx); !errors.Is(err, context.Canceled) {
				t.Fatalf("error = %v, want context.Canceled", err)
			}
		})
	}
}
