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

func TestCompatibilityInfoEndpointsUseOfficialWiresAndExactValues(t *testing.T) {
	t.Parallel()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var request map[string]any
		if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
			t.Fatal(err)
		}
		switch request["type"] {
		case "exchangeStatus":
			_, _ = w.Write([]byte(`{"time":1,"specialStatuses":null}`))
		case "extraAgents":
			assertCompatibilityUser(t, request)
			_, _ = w.Write([]byte(`[{"address":"0xagent","name":"bot","validUntil":null}]`))
		case "gossipPriorityAuctionStatus":
			_, _ = w.Write([]byte(`[["127.0.0.1",null],[{"currentGas":"1.5","durationSeconds":10,"endGas":null,"startGas":"2","startTimeSeconds":3}]]`))
		case "gossipRootIps":
			_, _ = w.Write([]byte(`["127.0.0.1"]`))
		case "isVip":
			assertCompatibilityUser(t, request)
			_, _ = w.Write([]byte(`true`))
		case "legalCheck":
			assertCompatibilityUser(t, request)
			_, _ = w.Write([]byte(`{"acceptedTerms":true,"userAllowed":true,"restrictions":"n"}`))
		case "preTransferCheck":
			if request["user"] != "0xabc" || request["source"] != "0xsource" {
				t.Fatalf("pre-transfer request=%v", request)
			}
			_, _ = w.Write([]byte(`{"fee":"1.25","isSanctioned":false,"userExists":true,"userHasSentTx":false}`))
		case "vaultSummaries":
			_, _ = w.Write([]byte(`[{"name":"v","vaultAddress":"0xvault","leader":"0xleader","tvl":"2.50","isClosed":false,"relationship":{"type":"normal","data":{}},"createTimeMillis":3}]`))
		case "validatorSummaries":
			_, _ = w.Write([]byte(`[{"validator":"0xvalidator","signer":"0xsigner","name":"v","description":"d","nRecentBlocks":4,"stake":"5.25","isJailed":false,"unjailableAfter":null,"isActive":true,"commission":"0.01","stats":[["day",{"uptimeFraction":"0.9","predictedApr":"0.1","nSamples":2}],["week",{"uptimeFraction":"0.8","predictedApr":"0.2","nSamples":3}],["month",{"uptimeFraction":"0.7","predictedApr":"0.3","nSamples":4}]]}]`))
		case "validatorL1Votes":
			_, _ = w.Write([]byte(`[{"expireTime":4,"action":{"D":"update"},"votes":["0xvalidator"],"quorumReached":true}]`))
		case "marginTable":
			if request["id"] != float64(7) {
				t.Fatalf("margin table request=%v", request)
			}
			_, _ = w.Write([]byte(`{"description":"tiered","marginTiers":[{"lowerBound":"1.25","maxLeverage":10}]}`))
		case "leadingVaults":
			assertCompatibilityUser(t, request)
			_, _ = w.Write([]byte(`[{"address":"0xvault","name":"v"}]`))
		case "subAccounts2":
			assertCompatibilityUser(t, request)
			_, _ = w.Write([]byte(`[{"name":"sub","subAccountUser":"0xsub","master":"0xabc","dexToClearinghouseState":[["",{"assetPositions":[],"crossMaintenanceMarginUsed":"0","crossMarginSummary":{"accountValue":"0","totalMarginUsed":"0","totalNtlPos":"0","totalRawUsd":"0"},"marginSummary":{"accountValue":"0","totalMarginUsed":"0","totalNtlPos":"0","totalRawUsd":"0"},"time":1,"withdrawable":"0"}]],"spotState":{"balances":[],"evmEscrows":[],"tokenToPortfolioBorrowRatio":[],"tokenToAvailableAfterMaintenance":[]}}]`))
		case "twapHistory":
			assertCompatibilityUser(t, request)
			_, _ = w.Write([]byte(`[{"time":2,"state":{"coin":"BTC","executedNtl":"1.25","executedSz":"0.5","minutes":10,"reduceOnly":false,"randomize":false,"side":"B","size":"2","timestamp":1,"user":"0xabc"},"status":{"status":"finished"},"twapId":9}]`))
		case "userToMultiSigSigners":
			assertCompatibilityUser(t, request)
			_, _ = w.Write([]byte(`{"authorizedUsers":["0xone","0xtwo"],"threshold":2}`))
		case "maxMarketOrderNtls":
			_, _ = w.Write([]byte(`[[10,"1000.25"]]`))
		case "liquidatable":
			_, _ = w.Write([]byte(`[{"user":"0xabc","positionIndex":{"isolated":{"asset":1}},"marginAvailable":["1.5","2.5"]}]`))
		case "settledOutcome":
			if request["outcome"] != float64(3) {
				t.Fatalf("settled outcome request=%v", request)
			}
			_, _ = w.Write([]byte(`{"spec":{"outcome":3,"name":"yes","description":"d","sideSpecs":[],"quoteToken":"USDC"},"settleFraction":"0.75","details":"done"}`))
		default:
			w.WriteHeader(http.StatusBadRequest)
			_, _ = fmt.Fprintf(w, `{"message":"unexpected %v"}`, request["type"])
		}
	}))
	defer server.Close()

	c := info.NewClient(server.URL, transport.NewDefaultHTTPTransport(nil), time.Second, "test")
	ctx := context.Background()
	if result, err := c.ExchangeStatus(ctx); err != nil || result.Time != 1 || result.SpecialStatuses != nil {
		t.Fatalf("status=%+v err=%v", result, err)
	}
	if result, err := c.ExtraAgents(ctx, "0xabc"); err != nil || len(result) != 1 || result[0].ValidUntil != nil {
		t.Fatalf("agents=%+v err=%v", result, err)
	}
	if result, err := c.GossipPriorityAuctionStatus(ctx); err != nil || result.PreviousWinners[0] == nil || *result.PreviousWinners[0] != "127.0.0.1" || result.Auctions[0].CurrentGas.String() != "1.5" {
		t.Fatalf("auction=%+v err=%v", result, err)
	}
	if result, err := c.GossipRootIPs(ctx); err != nil || result[0] != "127.0.0.1" {
		t.Fatalf("ips=%+v err=%v", result, err)
	}
	if result, err := c.IsVIP(ctx, "0xabc"); err != nil || result == nil || !*result {
		t.Fatalf("vip=%v err=%v", result, err)
	}
	if result, err := c.LegalCheck(ctx, "0xabc"); err != nil || !result.AcceptedTerms {
		t.Fatalf("legal=%+v err=%v", result, err)
	}
	if result, err := c.PreTransferCheck(ctx, "0xabc", "0xsource"); err != nil || result.Fee.String() != "1.25" {
		t.Fatalf("pre-transfer=%+v err=%v", result, err)
	}
	if result, err := c.VaultSummaries(ctx); err != nil || result[0].TVL.String() != "2.5" {
		t.Fatalf("vaults=%+v err=%v", result, err)
	}
	if result, err := c.ValidatorSummaries(ctx); err != nil || result[0].Stake.String() != "5.25" {
		t.Fatalf("validators=%+v err=%v", result, err)
	}
	if result, err := c.ValidatorL1Votes(ctx); err != nil || result[0].Action.D == nil || *result[0].Action.D != "update" {
		t.Fatalf("votes=%+v err=%v", result, err)
	}
	if result, err := c.MarginTable(ctx, 7); err != nil || result.MarginTiers[0].LowerBound.String() != "1.25" {
		t.Fatalf("margin=%+v err=%v", result, err)
	}
	if result, err := c.LeadingVaults(ctx, "0xabc"); err != nil || result[0].Address != "0xvault" {
		t.Fatalf("leading=%+v err=%v", result, err)
	}
	if result, err := c.Subaccounts2(ctx, "0xabc"); err != nil || result == nil || len(*result) != 1 || (*result)[0].DEXStates[0].DEX != "" {
		t.Fatalf("subaccounts=%+v err=%v", result, err)
	}
	if result, err := c.TWAPHistory(ctx, "0xabc"); err != nil || result[0].State.ExecutedNotional.String() != "1.25" || result[0].TWAPID == nil {
		t.Fatalf("twap=%+v err=%v", result, err)
	}
	if result, err := c.UserToMultiSigSigners(ctx, "0xabc"); err != nil || result == nil || result.Threshold != 2 {
		t.Fatalf("multisig=%+v err=%v", result, err)
	}
	if result, err := c.MaxMarketOrderNotionals(ctx); err != nil || result[0].Notional.String() != "1000.25" {
		t.Fatalf("max ntl=%+v err=%v", result, err)
	}
	if result, err := c.Liquidatable(ctx); err != nil || result[0].MarginAvailable[0].String() != "1.5" {
		t.Fatalf("liquidatable=%+v err=%v", result, err)
	}
	if result, err := c.SettledOutcome(ctx, 3); err != nil || result == nil || result.SettleFraction.String() != "0.75" {
		t.Fatalf("outcome=%+v err=%v", result, err)
	}
}

func assertCompatibilityUser(t *testing.T, request map[string]any) {
	t.Helper()
	if request["user"] != "0xabc" {
		t.Fatalf("request=%v", request)
	}
}

func TestCompatibilityInfoEndpointsPropagateSharedTransportFailures(t *testing.T) {
	t.Parallel()
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
	endpoints := []endpoint{
		{"exchangeStatus", func(c *info.Client, x context.Context) error { _, err := c.ExchangeStatus(x); return err }},
		{"extraAgents", func(c *info.Client, x context.Context) error { _, err := c.ExtraAgents(x, "0xabc"); return err }},
		{"gossipPriorityAuctionStatus", func(c *info.Client, x context.Context) error { _, err := c.GossipPriorityAuctionStatus(x); return err }},
		{"gossipRootIps", func(c *info.Client, x context.Context) error { _, err := c.GossipRootIPs(x); return err }},
		{"isVip", func(c *info.Client, x context.Context) error { _, err := c.IsVIP(x, "0xabc"); return err }},
		{"legalCheck", func(c *info.Client, x context.Context) error { _, err := c.LegalCheck(x, "0xabc"); return err }},
		{"preTransferCheck", func(c *info.Client, x context.Context) error {
			_, err := c.PreTransferCheck(x, "0xabc", "0xsource")
			return err
		}},
		{"vaultSummaries", func(c *info.Client, x context.Context) error { _, err := c.VaultSummaries(x); return err }},
		{"validatorSummaries", func(c *info.Client, x context.Context) error { _, err := c.ValidatorSummaries(x); return err }},
		{"validatorL1Votes", func(c *info.Client, x context.Context) error { _, err := c.ValidatorL1Votes(x); return err }},
		{"marginTable", func(c *info.Client, x context.Context) error { _, err := c.MarginTable(x, 1); return err }},
		{"leadingVaults", func(c *info.Client, x context.Context) error { _, err := c.LeadingVaults(x, "0xabc"); return err }},
		{"subAccounts2", func(c *info.Client, x context.Context) error { _, err := c.Subaccounts2(x, "0xabc"); return err }},
		{"twapHistory", func(c *info.Client, x context.Context) error { _, err := c.TWAPHistory(x, "0xabc"); return err }},
		{"userToMultiSigSigners", func(c *info.Client, x context.Context) error {
			_, err := c.UserToMultiSigSigners(x, "0xabc")
			return err
		}},
		{"maxMarketOrderNtls", func(c *info.Client, x context.Context) error { _, err := c.MaxMarketOrderNotionals(x); return err }},
		{"liquidatable", func(c *info.Client, x context.Context) error { _, err := c.Liquidatable(x); return err }},
		{"settledOutcome", func(c *info.Client, x context.Context) error { _, err := c.SettledOutcome(x, 1); return err }},
	}
	for _, tc := range endpoints {
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

func TestCompatibilityInfoEndpointsUseInjectedRequestTransport(t *testing.T) {
	t.Parallel()
	c := info.NewClient("unused", transport.NewDefaultHTTPTransport(nil), time.Second, "test")
	c.SetRequestTransport(extendedRequestTransport(func(_ context.Context, kind transport.RequestKind, payload any, response any) error {
		if kind != transport.RequestInfo {
			t.Fatalf("kind=%q", kind)
		}
		request, err := json.Marshal(payload)
		if err != nil {
			t.Fatal(err)
		}
		if string(request) != `{"type":"legalCheck","user":"0xabc"}` {
			t.Fatalf("request=%s", request)
		}
		*response.(*info.LegalCheckResponse) = info.LegalCheckResponse{AcceptedTerms: true, UserAllowed: true}
		return nil
	}))
	result, err := c.LegalCheck(context.Background(), "0xabc")
	if err != nil || !result.AcceptedTerms || !result.UserAllowed {
		t.Fatalf("result=%+v err=%v", result, err)
	}
}

func TestCompatibilityInfoUnionsRejectAmbiguousValuesAndInputsValidateBeforeTransport(t *testing.T) {
	t.Parallel()
	var action info.ValidatorL1VoteAction
	if err := json.Unmarshal([]byte(`{"D":"one","C":["two"]}`), &action); err == nil {
		t.Fatal("ambiguous validator L1 action accepted")
	}
	var gossip info.GossipPriorityAuctionStatusResponse
	if err := json.Unmarshal([]byte(`[]`), &gossip); err == nil {
		t.Fatal("short gossip status tuple accepted")
	}
	c := info.NewClient("unused", transport.NewDefaultHTTPTransport(nil), time.Second, "test")
	if _, err := c.MarginTable(context.Background(), -1); err == nil {
		t.Fatal("negative margin table ID accepted")
	}
	if _, err := c.SettledOutcome(context.Background(), -1); err == nil {
		t.Fatal("negative outcome accepted")
	}
	if _, err := c.PreTransferCheck(context.Background(), "", "0xsource"); err == nil {
		t.Fatal("empty transfer user accepted")
	}
}

func TestCompatibilityInfoModelsCurrentOutcomeGovernanceVariantsStrictly(t *testing.T) {
	t.Parallel()
	var vote info.ValidatorL1Vote
	if err := json.Unmarshal([]byte(`{"expireTime":1,"action":{"O":{"settleQuestion2":{"question":102,"outcomeSettlements":[{"outcome":542,"settleFraction":"1","details":"resolved","nameAndDescription":["Below","description"],"sideNames":["Yes","No"]}],"nameAndDescription":["CPI","question"]}}},"votes":[],"quorumReached":true}`), &vote); err != nil {
		t.Fatal(err)
	}
	if vote.Action.O == nil || vote.Action.O.SettleQuestion2 == nil || vote.Action.O.SettleQuestion2.OutcomeSettlements[0].SettleFraction.String() != "1" {
		t.Fatalf("vote=%+v", vote)
	}
	var settlement info.SettledOutcomeResponse
	if err := json.Unmarshal([]byte(`{"spec":{"outcome":542,"name":"Below","description":"d","sideSpecs":[],"quoteToken":"USDC"},"settleFraction":"1","details":"resolved","question":{"question":{"settled":102},"name":"CPI","description":"question"}}`), &settlement); err != nil {
		t.Fatal(err)
	}
	if settlement.Question == nil || settlement.Question.ID.Settled == nil || *settlement.Question.ID.Settled != 102 {
		t.Fatalf("settlement=%+v", settlement)
	}
	var action info.ValidatorL1VoteAction
	if err := json.Unmarshal([]byte(`{"O":{"settleOutcome":{"outcome":1,"settleFraction":"1","details":"d","nameAndDescription":["one"],"sideNames":["Yes","No"]}}}`), &action); err == nil {
		t.Fatal("short nameAndDescription accepted")
	}
	var status info.TWAPHistoryStatus
	if err := json.Unmarshal([]byte(`{"status":"error"}`), &status); err == nil {
		t.Fatal("TWAP error without description accepted")
	}
	if err := json.Unmarshal([]byte(`{"status":"finished","description":"unexpected"}`), &status); err == nil {
		t.Fatal("finished TWAP with description accepted")
	}
	if err := json.Unmarshal([]byte(`{"status":"finished"}`), &status); err != nil {
		t.Fatal(err)
	}
	var liquidatable info.LiquidatablePosition
	if err := json.Unmarshal([]byte(`{"user":"0xabc","positionIndex":{"isolated":{"asset":1}},"marginAvailable":["1"]}`), &liquidatable); err == nil {
		t.Fatal("short marginAvailable tuple accepted")
	}
	var validator info.ValidatorSummary
	if err := json.Unmarshal([]byte(`{"stats":[["day",{}]]}`), &validator); err == nil {
		t.Fatal("short validator stats tuple accepted")
	}
	if err := json.Unmarshal([]byte(`{"stats":[["day",{}],["day",{}],["month",{}]]}`), &validator); err == nil {
		t.Fatal("misordered validator stats tuple accepted")
	}
}

func TestCompatibilityInfoRetainsCurrentOutcomeGovernanceFields(t *testing.T) {
	t.Parallel()
	var settleVote info.ValidatorL1Vote
	if err := json.Unmarshal([]byte(`{"expireTime":1,"action":{"O":{"settleOutcome":{"outcome":1,"settleFraction":"1","details":"d","nameAndDescription":["Name","Description"],"sideNames":["Yes","No"]}}},"votes":[],"quorumReached":true}`), &settleVote); err != nil {
		t.Fatal(err)
	}
	settle := settleVote.Action.O.SettleOutcome
	if settle == nil || settle.NameAndDescription[0] != "Name" || settle.SideNames[1] != "No" {
		t.Fatalf("settle=%+v", settle)
	}
	var registerVote info.ValidatorL1Vote
	if err := json.Unmarshal([]byte(`{"expireTime":1,"action":{"O":{"registerTokensAndStandaloneOutcome":{"quoteToken":0,"nameAndDescription":["Name","Description"],"sideNames":["Yes","No"]}}},"votes":[],"quorumReached":true}`), &registerVote); err != nil {
		t.Fatal(err)
	}
	registered := registerVote.Action.O.RegisterTokensAndStandaloneOutcome
	if registered == nil || registered.NameAndDescription[1] != "Description" || registered.SideNames[0] != "Yes" {
		t.Fatalf("registered=%+v", registered)
	}
	var active info.SettledOutcomeQuestionID
	if err := json.Unmarshal([]byte(`{"active":32}`), &active); err != nil || active.Active == nil || *active.Active != 32 {
		t.Fatalf("active=%+v err=%v", active, err)
	}
	if err := json.Unmarshal([]byte(`{"active":32,"settled":32}`), &active); err == nil {
		t.Fatal("ambiguous question identity accepted")
	}
}
