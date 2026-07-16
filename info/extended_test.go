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

	"github.com/Apexllcc/hyperliquid-go-sdk/info"
	"github.com/Apexllcc/hyperliquid-go-sdk/internal/hlerr"
	"github.com/Apexllcc/hyperliquid-go-sdk/transport"
)

type extendedRequestTransport func(context.Context, transport.RequestKind, any, any) error

func (f extendedRequestTransport) Request(ctx context.Context, kind transport.RequestKind, payload any, response any) error {
	return f(ctx, kind, payload, response)
}

func TestExtendedInfoMethodsUseOfficialRequestWires(t *testing.T) {
	t.Parallel()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var request map[string]any
		if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
			t.Fatal(err)
		}
		switch request["type"] {
		case "allMids":
			if request["dex"] != "xyz" {
				t.Fatalf("allMids request=%v", request)
			}
			_, _ = w.Write([]byte(`{"BTC":"1.25"}`))
		case "openOrders", "frontendOpenOrders":
			if request["user"] != "0xabc" || request["dex"] != "xyz" {
				t.Fatalf("orders request=%v", request)
			}
			_, _ = w.Write([]byte(`[]`))
		case "historicalOrders", "userTwapSliceFills", "userVaultEquities", "delegations", "delegatorRewards", "approvedBuilders":
			if request["user"] != "0xabc" {
				t.Fatalf("user request=%v", request)
			}
			_, _ = w.Write([]byte(`[]`))
		case "delegatorHistory":
			if request["user"] != "0xabc" {
				t.Fatalf("user request=%v", request)
			}
			_, _ = w.Write([]byte(`[{"time":1,"hash":"0xhash","delta":{"delegate":{"validator":"0xvalidator","amount":"2.5","isUndelegate":true}}}]`))
		case "maxBuilderFee":
			if request["user"] != "0xabc" || request["builder"] != "0xbuilder" {
				t.Fatalf("maxBuilderFee request=%v", request)
			}
			_, _ = w.Write([]byte(`42`))
		case "userRole":
			_, _ = w.Write([]byte(`{"role":"agent","data":{"user":"0xmaster"}}`))
		case "referral":
			_, _ = w.Write([]byte(`{"referredBy":null,"cumVlm":"1.125","unclaimedRewards":"0","claimedRewards":"0","builderRewards":"0","referrerState":{"stage":"needToTrade","data":{"required":"2"}},"rewardHistory":[],"tokenToState":[]}`))
		case "userDexAbstraction":
			_, _ = w.Write([]byte(`true`))
		case "userAbstraction":
			_, _ = w.Write([]byte(`"unifiedAccount"`))
		case "borrowLendReserveState":
			if request["token"] != float64(7) {
				t.Fatalf("reserve request=%v", request)
			}
			_, _ = w.Write([]byte(`{"borrowYearlyRate":"0.1","supplyYearlyRate":"0.05","balance":"2","utilization":"0.5","oraclePx":"3","ltv":"0.7","totalSupplied":"4","totalBorrowed":"2"}`))
		case "borrowLendUserState":
			_, _ = w.Write([]byte(`{"tokenToState":[[7,{"borrow":{"basis":"1","value":"2"},"supply":{"basis":"3","value":"4"}}]],"health":"healthy","healthFactor":null}`))
		case "allBorrowLendReserveStates":
			_, _ = w.Write([]byte(`[]`))
		default:
			w.WriteHeader(http.StatusBadRequest)
			_, _ = fmt.Fprintf(w, `{"message":"unexpected %v"}`, request["type"])
		}
	}))
	defer server.Close()
	c := info.NewClient(server.URL, transport.NewDefaultHTTPTransport(nil), time.Second, "test")
	ctx := context.Background()
	if mids, err := c.AllMidsForDEX(ctx, "xyz"); err != nil || mids["BTC"].String() != "1.25" {
		t.Fatalf("mids=%v err=%v", mids, err)
	}
	if _, err := c.OpenOrdersForDEX(ctx, "0xabc", "xyz"); err != nil {
		t.Fatal(err)
	}
	if _, err := c.FrontendOpenOrdersForDEX(ctx, "0xabc", "xyz"); err != nil {
		t.Fatal(err)
	}
	if fee, err := c.MaxBuilderFee(ctx, "0xabc", "0xbuilder"); err != nil || fee != 42 {
		t.Fatalf("fee=%d err=%v", fee, err)
	}
	if _, err := c.HistoricalOrders(ctx, "0xabc"); err != nil {
		t.Fatal(err)
	}
	if _, err := c.UserTwapSliceFills(ctx, "0xabc"); err != nil {
		t.Fatal(err)
	}
	if _, err := c.UserVaultEquities(ctx, "0xabc"); err != nil {
		t.Fatal(err)
	}
	if role, err := c.UserRole(ctx, "0xabc"); err != nil || role.Role != "agent" || role.Data == nil || role.Data.User != "0xmaster" {
		t.Fatalf("role=%+v err=%v", role, err)
	}
	if referral, err := c.Referral(ctx, "0xabc"); err != nil || referral.CumulativeVolume.String() != "1.125" {
		t.Fatalf("referral=%+v err=%v", referral, err)
	}
	if _, err := c.Delegations(ctx, "0xabc"); err != nil {
		t.Fatal(err)
	}
	if history, err := c.DelegatorHistory(ctx, "0xabc"); err != nil || history[0].Delta.Delegate == nil || history[0].Delta.Delegate.Amount.String() != "2.5" {
		t.Fatalf("history=%+v err=%v", history, err)
	}
	if _, err := c.DelegatorRewards(ctx, "0xabc"); err != nil {
		t.Fatal(err)
	}
	if _, err := c.ApprovedBuilders(ctx, "0xabc"); err != nil {
		t.Fatal(err)
	}
	if value, err := c.UserDEXAbstraction(ctx, "0xabc"); err != nil || value == nil || !*value {
		t.Fatalf("abstraction=%v err=%v", value, err)
	}
	if value, err := c.UserAbstraction(ctx, "0xabc"); err != nil || value != info.UserAbstractionUnifiedAccount {
		t.Fatalf("abstraction=%q err=%v", value, err)
	}
	if reserve, err := c.BorrowLendReserveState(ctx, 7); err != nil || reserve.OraclePx.String() != "3" {
		t.Fatalf("reserve=%+v err=%v", reserve, err)
	}
	if state, err := c.BorrowLendUserState(ctx, "0xabc"); err != nil || state.TokenToState[0].State.Supply.Value.String() != "4" {
		t.Fatalf("state=%+v err=%v", state, err)
	}
	if _, err := c.AllBorrowLendReserveStates(ctx); err != nil {
		t.Fatal(err)
	}
}

func TestUserRoleRetainsAbsentDataAndDelegatorHistoryRejectsAmbiguousUnions(t *testing.T) {
	t.Parallel()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var request map[string]any
		_ = json.NewDecoder(r.Body).Decode(&request)
		switch request["type"] {
		case "userRole":
			_, _ = w.Write([]byte(`{"role":"user"}`))
		case "delegatorHistory":
			_, _ = w.Write([]byte(`[{"time":1,"hash":"0xhash","delta":{"delegate":{"validator":"0xvalidator","amount":"1","isUndelegate":false},"cDeposit":{"amount":"1"}}}]`))
		default:
			w.WriteHeader(http.StatusBadRequest)
		}
	}))
	defer server.Close()
	c := info.NewClient(server.URL, transport.NewDefaultHTTPTransport(nil), time.Second, "test")
	role, err := c.UserRole(context.Background(), "0xabc")
	if err != nil || role.Role != "user" || role.Data != nil {
		t.Fatalf("role=%+v err=%v", role, err)
	}
	if _, err := c.DelegatorHistory(context.Background(), "0xabc"); err == nil {
		t.Fatal("ambiguous delegator history union accepted")
	}
}

func TestExtendedInfoMethodsUseInjectedRequestTransport(t *testing.T) {
	t.Parallel()
	c := info.NewClient("unused", transport.NewDefaultHTTPTransport(nil), time.Second, "test")
	c.SetRequestTransport(extendedRequestTransport(func(ctx context.Context, kind transport.RequestKind, payload any, response any) error {
		if kind != transport.RequestInfo {
			t.Fatalf("kind=%q", kind)
		}
		request, ok := payload.(struct {
			Type string `json:"type"`
		})
		if !ok || request.Type != "allPerpMetas" {
			t.Fatalf("payload=%#v", payload)
		}
		select {
		case <-ctx.Done():
			t.Fatal(ctx.Err())
		default:
		}
		*response.(*[]info.MetaResponse) = []info.MetaResponse{{}}
		return nil
	}))
	metas, err := c.AllPerpMetas(context.Background())
	if err != nil || len(metas) != 1 {
		t.Fatalf("metas=%+v err=%v", metas, err)
	}
}

func TestPerpAndSpotExtensionMethodsUseOfficialRequestWires(t *testing.T) {
	t.Parallel()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var request map[string]any
		if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
			t.Fatal(err)
		}
		switch request["type"] {
		case "perpsAtOpenInterestCap":
			if request["dex"] != "xyz" {
				t.Fatalf("cap request=%v", request)
			}
			_, _ = w.Write([]byte(`["xyz:BTC"]`))
		case "perpDeployAuctionStatus", "spotPairDeployAuctionStatus":
			_, _ = w.Write([]byte(`{"currentGas":"1.5","durationSeconds":10,"endGas":null,"startGas":"2","startTimeSeconds":3}`))
		case "activeAssetData":
			if request["user"] != "0xabc" || request["coin"] != "xyz:BTC" {
				t.Fatalf("active request=%v", request)
			}
			_, _ = w.Write([]byte(`{"user":"0xabc","coin":"xyz:BTC","leverage":{"type":"cross","value":5},"maxTradeSzs":["1","2"],"availableToTrade":["3","4"],"markPx":"5"}`))
		case "perpDexLimits":
			if request["dex"] != "xyz" {
				t.Fatalf("limits request=%v", request)
			}
			_, _ = w.Write([]byte(`{"totalOiCap":"1","oiSzCapPerPerp":"2","maxTransferNtl":"3","coinToOiCap":[["xyz:BTC","4"]]}`))
		case "perpDexStatus":
			if request["dex"] != "xyz" {
				t.Fatalf("status request=%v", request)
			}
			_, _ = w.Write([]byte(`{"totalNetDeposit":"1.25"}`))
		case "allPerpMetas":
			_, _ = w.Write([]byte(`[{"universe":[],"marginTables":[],"collateralToken":0}]`))
		case "perpAnnotation":
			if request["coin"] != "BTC" {
				t.Fatalf("annotation request=%v", request)
			}
			_, _ = w.Write([]byte(`{"category":"layer1","description":"L1","displayName":"Bitcoin","keywords":["btc"]}`))
		case "perpCategories":
			_, _ = w.Write([]byte(`[["BTC","layer1"]]`))
		case "perpConciseAnnotations":
			_, _ = w.Write([]byte(`[["BTC",{"category":"layer1","displayName":"Bitcoin","keywords":["btc"]}]]`))
		case "spotDeployState":
			if request["user"] != "0xabc" {
				t.Fatalf("spot deploy=%v", request)
			}
			_, _ = w.Write([]byte(`{"states":[],"gasAuction":{"currentGas":null,"durationSeconds":1,"endGas":null,"startGas":"1","startTimeSeconds":1}}`))
		case "tokenDetails":
			if request["tokenId"] != "0xtoken" {
				t.Fatalf("token request=%v", request)
			}
			_, _ = w.Write([]byte(`{"name":"TOKEN","maxSupply":"10","totalSupply":"9","circulatingSupply":"8","szDecimals":2,"weiDecimals":8,"midPx":"1","markPx":"1.1","prevDayPx":"0.9","genesis":null,"deployer":null,"deployGas":null,"deployTime":null,"seededUsdc":"2","nonCirculatingUserBalances":[],"futureEmissions":"3"}`))
		case "outcomeMeta":
			_, _ = w.Write([]byte(`{"outcomes":[],"questions":[]}`))
		default:
			w.WriteHeader(http.StatusBadRequest)
			_, _ = fmt.Fprintf(w, `{"message":"unexpected %v"}`, request["type"])
		}
	}))
	defer server.Close()
	c := info.NewClient(server.URL, transport.NewDefaultHTTPTransport(nil), time.Second, "test")
	ctx := context.Background()
	if cap, err := c.PerpsAtOpenInterestCap(ctx, "xyz"); err != nil || cap[0] != "xyz:BTC" {
		t.Fatalf("cap=%v err=%v", cap, err)
	}
	if a, err := c.PerpDeployAuctionStatus(ctx); err != nil || a.CurrentGas.String() != "1.5" {
		t.Fatalf("auction=%+v err=%v", a, err)
	}
	if active, err := c.ActiveAssetData(ctx, "0xabc", "xyz:BTC"); err != nil || active.MarkPx.String() != "5" {
		t.Fatalf("active=%+v err=%v", active, err)
	}
	if limits, err := c.PerpDEXLimits(ctx, "xyz"); err != nil || limits.CoinToOICap[0].Cap.String() != "4" {
		t.Fatalf("limits=%+v err=%v", limits, err)
	}
	if status, err := c.PerpDEXStatus(ctx, "xyz"); err != nil || status.TotalNetDeposit.String() != "1.25" {
		t.Fatalf("status=%+v err=%v", status, err)
	}
	if metas, err := c.AllPerpMetas(ctx); err != nil || len(metas) != 1 {
		t.Fatalf("metas=%+v err=%v", metas, err)
	}
	if annotation, err := c.PerpAnnotation(ctx, "BTC"); err != nil || annotation.DisplayName != "Bitcoin" {
		t.Fatalf("annotation=%+v err=%v", annotation, err)
	}
	if categories, err := c.PerpCategories(ctx); err != nil || categories[0].Category != "layer1" {
		t.Fatalf("categories=%+v err=%v", categories, err)
	}
	if annotations, err := c.PerpConciseAnnotations(ctx); err != nil || annotations[0].Annotation.DisplayName != "Bitcoin" {
		t.Fatalf("annotations=%+v err=%v", annotations, err)
	}
	if state, err := c.SpotDeployState(ctx, "0xabc"); err != nil || state.GasAuction.StartGas.String() != "1" {
		t.Fatalf("state=%+v err=%v", state, err)
	}
	if auction, err := c.SpotPairDeployAuctionStatus(ctx); err != nil || auction.DurationSeconds != 10 {
		t.Fatalf("auction=%+v err=%v", auction, err)
	}
	if token, err := c.TokenDetails(ctx, "0xtoken"); err != nil || token.CirculatingSupply.String() != "8" {
		t.Fatalf("token=%+v err=%v", token, err)
	}
	if outcomes, err := c.OutcomeMeta(ctx); err != nil || outcomes.Outcomes == nil || outcomes.Questions == nil {
		t.Fatalf("outcomes=%+v err=%v", outcomes, err)
	}
}

func TestLedgerAndTimedInfoExtensionsUseOfficialRequestWires(t *testing.T) {
	t.Parallel()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var request map[string]any
		if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
			t.Fatal(err)
		}
		if request["user"] != "0xabc" || request["startTime"] != float64(10) || request["endTime"] != float64(20) {
			t.Fatalf("request=%v", request)
		}
		switch request["type"] {
		case "userNonFundingLedgerUpdates":
			_, _ = w.Write([]byte(`[{"time":20,"hash":"0xhash","delta":{"type":"spotTransfer","token":"HYPE","amount":"1.25","usdcValue":"2.5","user":"0xabc","destination":"0xdst","fee":"0.01","nativeTokenFee":"0.02","nonce":3,"feeToken":"USDC"}}]`))
		case "userTwapSliceFillsByTime":
			_, _ = w.Write([]byte(`[]`))
		case "userBorrowLendInterest":
			_, _ = w.Write([]byte(`[{"time":20,"token":"USDC","borrow":"0.1","supply":"0.2"}]`))
		default:
			w.WriteHeader(http.StatusBadRequest)
		}
	}))
	defer server.Close()
	c := info.NewClient(server.URL, transport.NewDefaultHTTPTransport(nil), time.Second, "test")
	end := int64(20)
	ledger, err := c.UserNonFundingLedgerUpdates(context.Background(), "0xabc", 10, &end)
	if err != nil || ledger[0].Delta.Amount.String() != "1.25" || ledger[0].Delta.Type != "spotTransfer" {
		t.Fatalf("ledger=%+v err=%v", ledger, err)
	}
	if _, err := c.UserTwapSliceFillsByTime(context.Background(), "0xabc", 10, &end); err != nil {
		t.Fatal(err)
	}
	interest, err := c.UserBorrowLendInterest(context.Background(), "0xabc", 10, &end)
	if err != nil || interest[0].Supply.String() != "0.2" {
		t.Fatalf("interest=%+v err=%v", interest, err)
	}
}

func TestExtendedInfoEndpointsPropagateAPIErrorsTimeoutsAndCancellation(t *testing.T) {
	apiFailure := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte(`{"code":"bad_request","message":"fixture error"}`))
	}))
	defer apiFailure.Close()
	block := make(chan struct{})
	timeoutServer := httptest.NewServer(http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) {
		select {
		case <-r.Context().Done():
		case <-block:
		}
	}))
	defer func() { close(block); timeoutServer.Close() }()
	type endpoint struct {
		name string
		call func(*info.Client, context.Context) error
	}
	end := int64(2)
	calls := []endpoint{
		{"allMidsForDEX", func(c *info.Client, x context.Context) error { _, err := c.AllMidsForDEX(x, "xyz"); return err }},
		{"metaAndAssetCtxsForDEX", func(c *info.Client, x context.Context) error {
			_, err := c.MetaAndAssetContextsForDEX(x, "xyz")
			return err
		}},
		{"openOrdersForDEX", func(c *info.Client, x context.Context) error {
			_, err := c.OpenOrdersForDEX(x, "0xabc", "xyz")
			return err
		}},
		{"frontendOpenOrdersForDEX", func(c *info.Client, x context.Context) error {
			_, err := c.FrontendOpenOrdersForDEX(x, "0xabc", "xyz")
			return err
		}},
		{"maxBuilderFee", func(c *info.Client, x context.Context) error {
			_, err := c.MaxBuilderFee(x, "0xabc", "0xbuilder")
			return err
		}},
		{"historicalOrders", func(c *info.Client, x context.Context) error { _, err := c.HistoricalOrders(x, "0xabc"); return err }},
		{"userTwapSliceFills", func(c *info.Client, x context.Context) error { _, err := c.UserTwapSliceFills(x, "0xabc"); return err }},
		{"userVaultEquities", func(c *info.Client, x context.Context) error { _, err := c.UserVaultEquities(x, "0xabc"); return err }},
		{"userRole", func(c *info.Client, x context.Context) error { _, err := c.UserRole(x, "0xabc"); return err }},
		{"referral", func(c *info.Client, x context.Context) error { _, err := c.Referral(x, "0xabc"); return err }},
		{"delegations", func(c *info.Client, x context.Context) error { _, err := c.Delegations(x, "0xabc"); return err }},
		{"delegatorHistory", func(c *info.Client, x context.Context) error { _, err := c.DelegatorHistory(x, "0xabc"); return err }},
		{"delegatorRewards", func(c *info.Client, x context.Context) error { _, err := c.DelegatorRewards(x, "0xabc"); return err }},
		{"approvedBuilders", func(c *info.Client, x context.Context) error { _, err := c.ApprovedBuilders(x, "0xabc"); return err }},
		{"userDexAbstraction", func(c *info.Client, x context.Context) error { _, err := c.UserDEXAbstraction(x, "0xabc"); return err }},
		{"userAbstraction", func(c *info.Client, x context.Context) error { _, err := c.UserAbstraction(x, "0xabc"); return err }},
		{"borrowLendReserveState", func(c *info.Client, x context.Context) error { _, err := c.BorrowLendReserveState(x, 1); return err }},
		{"borrowLendUserState", func(c *info.Client, x context.Context) error { _, err := c.BorrowLendUserState(x, "0xabc"); return err }},
		{"allBorrowLendReserveStates", func(c *info.Client, x context.Context) error { _, err := c.AllBorrowLendReserveStates(x); return err }},
		{"perpsAtOpenInterestCap", func(c *info.Client, x context.Context) error {
			_, err := c.PerpsAtOpenInterestCap(x, "xyz")
			return err
		}},
		{"perpDeployAuctionStatus", func(c *info.Client, x context.Context) error { _, err := c.PerpDeployAuctionStatus(x); return err }},
		{"activeAssetData", func(c *info.Client, x context.Context) error {
			_, err := c.ActiveAssetData(x, "0xabc", "BTC")
			return err
		}},
		{"perpDexLimits", func(c *info.Client, x context.Context) error { _, err := c.PerpDEXLimits(x, "xyz"); return err }},
		{"perpDexStatus", func(c *info.Client, x context.Context) error { _, err := c.PerpDEXStatus(x, "xyz"); return err }},
		{"allPerpMetas", func(c *info.Client, x context.Context) error { _, err := c.AllPerpMetas(x); return err }},
		{"perpAnnotation", func(c *info.Client, x context.Context) error { _, err := c.PerpAnnotation(x, "BTC"); return err }},
		{"perpCategories", func(c *info.Client, x context.Context) error { _, err := c.PerpCategories(x); return err }},
		{"perpConciseAnnotations", func(c *info.Client, x context.Context) error { _, err := c.PerpConciseAnnotations(x); return err }},
		{"spotDeployState", func(c *info.Client, x context.Context) error { _, err := c.SpotDeployState(x, "0xabc"); return err }},
		{"spotPairDeployAuctionStatus", func(c *info.Client, x context.Context) error { _, err := c.SpotPairDeployAuctionStatus(x); return err }},
		{"tokenDetails", func(c *info.Client, x context.Context) error { _, err := c.TokenDetails(x, "0xtoken"); return err }},
		{"outcomeMeta", func(c *info.Client, x context.Context) error { _, err := c.OutcomeMeta(x); return err }},
		{"userNonFundingLedgerUpdates", func(c *info.Client, x context.Context) error {
			_, err := c.UserNonFundingLedgerUpdates(x, "0xabc", 1, &end)
			return err
		}},
		{"userTwapSliceFillsByTime", func(c *info.Client, x context.Context) error {
			_, err := c.UserTwapSliceFillsByTime(x, "0xabc", 1, &end)
			return err
		}},
		{"userBorrowLendInterest", func(c *info.Client, x context.Context) error {
			_, err := c.UserBorrowLendInterest(x, "0xabc", 1, &end)
			return err
		}},
	}
	for _, tc := range calls {
		t.Run(tc.name+"/api_error", func(t *testing.T) {
			var apiErr *hlerr.APIError
			err := tc.call(info.NewClient(apiFailure.URL, transport.NewDefaultHTTPTransport(nil), time.Second, "test"), context.Background())
			if !errors.As(err, &apiErr) || apiErr.Code != "bad_request" {
				t.Fatalf("error=%v", err)
			}
		})
		t.Run(tc.name+"/timeout", func(t *testing.T) {
			err := tc.call(info.NewClient(timeoutServer.URL, transport.NewDefaultHTTPTransport(nil), 10*time.Millisecond, "test"), context.Background())
			if !errors.Is(err, context.DeadlineExceeded) {
				t.Fatalf("error=%v", err)
			}
		})
		t.Run(tc.name+"/canceled", func(t *testing.T) {
			ctx, cancel := context.WithCancel(context.Background())
			cancel()
			err := tc.call(info.NewClient(apiFailure.URL, transport.NewDefaultHTTPTransport(nil), time.Second, "test"), ctx)
			if !errors.Is(err, context.Canceled) {
				t.Fatalf("error=%v", err)
			}
		})
	}
}
