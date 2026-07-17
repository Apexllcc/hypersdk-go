package info_test

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/Apexllcc/hypersdk-go/info"
	"github.com/Apexllcc/hypersdk-go/transport"
)

func TestAccountQueriesUseOfficialWiresAndDecodeDecimalResponses(t *testing.T) {
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
			User      string `json:"user"`
			StartTime *int64 `json:"startTime"`
			EndTime   *int64 `json:"endTime"`
			Vault     string `json:"vaultAddress"`
		}
		if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
			recordServerErr(err)
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		if request.User != "0xabc" && request.Type != "vaultDetails" {
			recordServerErr(fmt.Errorf("unexpected user %q", request.User))
		}
		switch request.Type {
		case "portfolio":
			_, _ = w.Write([]byte(`[["day",{"accountValueHistory":[[1,"12.50"]],"pnlHistory":[[1,"-2.5"]],"vlm":"42.0"}]]`))
		case "userFunding":
			if request.StartTime == nil || *request.StartTime != 1 || request.EndTime == nil || *request.EndTime != 2 {
				recordServerErr(fmt.Errorf("unexpected userFunding request: %#v", request))
			}
			_, _ = w.Write([]byte(`[{"delta":{"type":"funding","coin":"BTC","fundingRate":"0.0001","szi":"2.5","usdc":"-0.25","nSamples":null},"hash":"0x1","time":3}]`))
		case "userFees":
			_, _ = w.Write([]byte(`{"dailyUserVlm":[{"date":"2026-01-01","userCross":"1.2","userAdd":"3.4","exchange":"5.6"}],"feeSchedule":{"cross":"0.00045","add":"0.00015","spotCross":"0.0007","spotAdd":"0.0004","tiers":{"vip":[{"ntlCutoff":"5000000","cross":"0.0004","add":"0.00012","spotCross":"0.0006","spotAdd":"0.0003"}],"mm":[{"makerFractionCutoff":"0.005","add":"-0.00001"}]},"referralDiscount":"0.04","stakingDiscountTiers":[{"bpsOfMaxSupply":"1","discount":"0.05"}]},"userCrossRate":"0.000315","userAddRate":"0.000105","userSpotCrossRate":"0.00049","userSpotAddRate":"0.00028","activeReferralDiscount":"0.0","feeTrialEscrow":"0.0","nextTrialAvailableTimestamp":null,"stakingLink":{"type":"tradingUser","stakingUser":"0xstake"},"activeStakingDiscount":{"bpsOfMaxSupply":"1","discount":"0.05"}}`))
		case "userRateLimit":
			_, _ = w.Write([]byte(`{"cumVlm":"2854574.593578","nRequestsUsed":2890,"nRequestsCap":2864574,"nRequestsSurplus":0}`))
		case "subAccounts":
			_, _ = w.Write([]byte(`[{"subAccountUser":"0xsub","master":"0xabc","clearinghouseState":{"assetPositions":[],"crossMaintenanceMarginUsed":"0","crossMarginSummary":{"accountValue":"1","totalMarginUsed":"0","totalNtlPos":"0","totalRawUsd":"0"},"marginSummary":{"accountValue":"1","totalMarginUsed":"0","totalNtlPos":"0","totalRawUsd":"0"},"time":1,"withdrawable":"1"},"spotState":{"balances":[{"coin":"USDC","token":0,"total":"1.5","hold":"0.2","entryNtl":"1"}]}}]`))
		case "vaultDetails":
			if request.Vault != "0xvault" {
				recordServerErr(fmt.Errorf("unexpected vault %q", request.Vault))
			}
			_, _ = w.Write([]byte(`{"name":"Vault","vaultAddress":"0xvault","leader":"0xleader","description":"test","portfolio":[["day",{"accountValueHistory":[[1,"4"]],"pnlHistory":[[1,"1"]],"vlm":"2"}]],"leaderFraction":0.1,"leaderCommission":0.2,"followerState":null,"followers":[{"user":"0xfollower","vaultEquity":"3.5","pnl":"1","allTimePnl":"2","daysFollowing":1,"vaultEntryTime":2,"lockupUntil":4}],"maxDistributable":5.5,"maxWithdrawable":6.5,"allowDeposits":true,"alwaysCloseOnWithdraw":false,"isClosed":false,"apr":0.3,"relationship":{"type":"parent","data":{"childAddresses":["0xchild"]}}}`))
		default:
			recordServerErr(fmt.Errorf("unexpected request type %q", request.Type))
			w.WriteHeader(http.StatusBadRequest)
		}
	}))
	defer server.Close()
	c := info.NewClient(server.URL, transport.NewDefaultHTTPTransport(nil), time.Second, "test")

	portfolio, err := c.Portfolio(context.Background(), "0xabc")
	if err != nil || len(portfolio) != 1 || portfolio[0].Data.AccountValueHistory[0].Value.String() != "12.5" {
		t.Fatalf("portfolio=%+v err=%v", portfolio, err)
	}
	start, end := int64(1), int64(2)
	funding, err := c.UserFunding(context.Background(), "0xabc", &start, &end)
	if err != nil || len(funding) != 1 || funding[0].Delta.USDC.String() != "-0.25" {
		t.Fatalf("funding=%+v err=%v", funding, err)
	}
	fees, err := c.UserFees(context.Background(), "0xabc")
	if err != nil || fees.FeeSchedule.Tiers.VIP[0].NotionalCutoff.String() != "5000000" || fees.UserSpotAddRate.String() != "0.00028" {
		t.Fatalf("fees=%+v err=%v", fees, err)
	}
	rateLimit, err := c.UserRateLimit(context.Background(), "0xabc")
	if err != nil || rateLimit.CumulativeVolume.String() != "2854574.593578" || rateLimit.RequestsCap != 2864574 {
		t.Fatalf("rateLimit=%+v err=%v", rateLimit, err)
	}
	subaccounts, err := c.Subaccounts(context.Background(), "0xabc")
	if err != nil || len(subaccounts) != 1 || subaccounts[0].SpotState.Balances[0].Token != 0 {
		t.Fatalf("subaccounts=%+v err=%v", subaccounts, err)
	}
	vault, err := c.VaultDetails(context.Background(), "0xvault", nil)
	if err != nil || vault == nil || vault.LeaderFraction.String() != "0.1" || vault.Followers[0].VaultEquity.String() != "3.5" {
		t.Fatalf("vault=%+v err=%v", vault, err)
	}
	select {
	case err := <-serverErr:
		t.Fatal(err)
	default:
	}
}

func TestAccountQueriesRejectInvalidArguments(t *testing.T) {
	t.Parallel()
	c := info.NewClient("http://invalid.example", transport.NewDefaultHTTPTransport(nil), time.Second, "test")
	if _, err := c.Portfolio(context.Background(), ""); err == nil {
		t.Fatal("Portfolio accepted empty user")
	}
	negative := int64(-1)
	if _, err := c.UserFunding(context.Background(), "0xabc", &negative, nil); err == nil {
		t.Fatal("UserFunding accepted negative start time")
	}
	if _, err := c.VaultDetails(context.Background(), "", nil); err == nil {
		t.Fatal("VaultDetails accepted empty vault address")
	}
}

func TestVaultDetailsReturnsNilForUnknownVault(t *testing.T) {
	t.Parallel()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`null`))
	}))
	defer server.Close()
	c := info.NewClient(server.URL, transport.NewDefaultHTTPTransport(nil), time.Second, "test")
	vault, err := c.VaultDetails(context.Background(), "0xmissing", nil)
	if err != nil || vault != nil {
		t.Fatalf("vault=%+v err=%v", vault, err)
	}
}
