package websocket_test

import (
	"bytes"
	"compress/flate"
	"context"
	"encoding/base64"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/Apexllcc/hypersdk-go/websocket"
	gws "github.com/gorilla/websocket"
)

// TestExtendedSubscriptionAPIExists fixes the supported public subscription
// surface. Wire/decode behaviour is exercised in the integration-style tests
// that follow it; this test intentionally fails to compile until each official
// subscription has a strongly typed entry point.
func TestExtendedSubscriptionAPIExists(t *testing.T) {
	client := websocket.NewClient("ws://127.0.0.1:1")
	defer func() { _ = client.Close() }()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	user := "0xabc"

	requests := []func() error{
		func() error { _, err := client.SubscribeNotification(ctx, user); return err },
		func() error { _, err := client.SubscribeWebData3(ctx, user); return err },
		func() error {
			_, err := client.SubscribeOpenOrders(ctx, websocket.UserDEXRequest{User: user})
			return err
		},
		func() error {
			_, err := client.SubscribeClearinghouseState(ctx, websocket.UserDEXRequest{User: user})
			return err
		},
		func() error {
			_, err := client.SubscribeActiveAssetData(ctx, websocket.ActiveAssetDataRequest{User: user, Coin: "BTC"})
			return err
		},
		func() error {
			_, err := client.SubscribeTWAPStates(ctx, websocket.UserDEXRequest{User: user})
			return err
		},
		func() error { _, err := client.SubscribeUserTWAPSliceFills(ctx, user); return err },
		func() error { _, err := client.SubscribeUserTWAPHistory(ctx, user); return err },
		func() error {
			_, err := client.SubscribeSpotState(ctx, websocket.SpotStateRequest{User: user})
			return err
		},
		func() error { _, err := client.SubscribeAllDEXsClearinghouseState(ctx, user); return err },
		func() error { _, err := client.SubscribeAllDEXsAssetCtxs(ctx); return err },
		func() error { _, err := client.SubscribeActiveSpotAssetCtx(ctx, "@1"); return err },
		func() error { _, err := client.SubscribeAssetCtxs(ctx, websocket.DEXRequest{}); return err },
		func() error { _, err := client.SubscribeFastAssetCtxs(ctx); return err },
		func() error { _, err := client.SubscribeSpotAssetCtxs(ctx); return err },
		func() error { _, err := client.SubscribeUserHistoricalOrders(ctx, user); return err },
		func() error { _, err := client.SubscribeOutcomeMetaUpdates(ctx); return err },
	}
	for _, subscribe := range requests {
		if err := subscribe(); err != nil {
			t.Fatalf("subscribe: %v", err)
		}
	}
}

func TestExtendedSubscriptionsDecodeOfficialFixtures(t *testing.T) {
	upgrader := gws.Upgrader{}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		connection, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			t.Error(err)
			return
		}
		defer func() { _ = connection.Close() }()
		for range 16 {
			var request struct {
				Subscription map[string]any `json:"subscription"`
			}
			if err := connection.ReadJSON(&request); err != nil {
				t.Error(err)
				return
			}
			kind, _ := request.Subscription["type"].(string)
			validateExtendedWire(t, kind, request.Subscription)
			data := extendedFixture(t, kind)
			if err := connection.WriteJSON(map[string]any{"channel": kind, "data": data}); err != nil {
				t.Error(err)
				return
			}
		}
		<-time.After(time.Second)
	}))
	defer server.Close()

	client := websocket.NewClient("ws" + strings.TrimPrefix(server.URL, "http"))
	defer func() { _ = client.Close() }()
	ctx := context.Background()
	user := "0xabc"
	notifications, err := client.SubscribeNotification(ctx, user)
	if err != nil {
		t.Fatal(err)
	}
	webData, err := client.SubscribeWebData3(ctx, user)
	if err != nil {
		t.Fatal(err)
	}
	orders, err := client.SubscribeOpenOrders(ctx, websocket.UserDEXRequest{User: user})
	if err != nil {
		t.Fatal(err)
	}
	clearing, err := client.SubscribeClearinghouseState(ctx, websocket.UserDEXRequest{User: user})
	if err != nil {
		t.Fatal(err)
	}
	active, err := client.SubscribeActiveAssetData(ctx, websocket.ActiveAssetDataRequest{User: user, Coin: "BTC"})
	if err != nil {
		t.Fatal(err)
	}
	twap, err := client.SubscribeTWAPStates(ctx, websocket.UserDEXRequest{User: user})
	if err != nil {
		t.Fatal(err)
	}
	sliceFills, err := client.SubscribeUserTWAPSliceFills(ctx, user)
	if err != nil {
		t.Fatal(err)
	}
	history, err := client.SubscribeUserTWAPHistory(ctx, user)
	if err != nil {
		t.Fatal(err)
	}
	isPortfolioMargin := true
	spot, err := client.SubscribeSpotState(ctx, websocket.SpotStateRequest{User: user, IsPortfolioMargin: &isPortfolioMargin})
	if err != nil {
		t.Fatal(err)
	}
	allClearing, err := client.SubscribeAllDEXsClearinghouseState(ctx, user)
	if err != nil {
		t.Fatal(err)
	}
	allContexts, err := client.SubscribeAllDEXsAssetCtxs(ctx)
	if err != nil {
		t.Fatal(err)
	}
	contexts, err := client.SubscribeAssetCtxs(ctx, websocket.DEXRequest{})
	if err != nil {
		t.Fatal(err)
	}
	fast, err := client.SubscribeFastAssetCtxs(ctx)
	if err != nil {
		t.Fatal(err)
	}
	spotContexts, err := client.SubscribeSpotAssetCtxs(ctx)
	if err != nil {
		t.Fatal(err)
	}
	historical, err := client.SubscribeUserHistoricalOrders(ctx, user)
	if err != nil {
		t.Fatal(err)
	}
	outcomes, err := client.SubscribeOutcomeMetaUpdates(ctx)
	if err != nil {
		t.Fatal(err)
	}

	select {
	case event := <-notifications.Events():
		if event.Notification != "notice" {
			t.Fatalf("notification=%+v", event)
		}
	case <-time.After(time.Second):
		t.Fatal("notification timeout")
	}
	select {
	case event := <-webData.Events():
		if event.UserState.CumulativeLedger.String() != "1.25" || len(event.PerpDEXStates) != 1 || len(event.PerpDEXStates[0].LeadingVaults) != 1 || event.PerpDEXStates[0].LeadingVaults[0].Address != "0xvault" {
			t.Fatalf("webData=%+v", event)
		}
	case <-time.After(time.Second):
		t.Fatal("web data timeout")
	}
	select {
	case event := <-orders.Events():
		if event.User != user {
			t.Fatalf("orders=%+v", event)
		}
	case <-time.After(time.Second):
		t.Fatal("orders timeout")
	}
	select {
	case event := <-clearing.Events():
		if event.User != user {
			t.Fatalf("clearing=%+v", event)
		}
	case <-time.After(time.Second):
		t.Fatal("clearing timeout")
	}
	select {
	case event := <-active.Events():
		if event.MarkPx.String() != "100.1" {
			t.Fatalf("active=%+v", event)
		}
	case <-time.After(time.Second):
		t.Fatal("active timeout")
	}
	select {
	case event := <-twap.Events():
		if len(event.States) != 1 || event.States[0].State.Size.String() != "2.5" {
			t.Fatalf("twap=%+v", event)
		}
	case <-time.After(time.Second):
		t.Fatal("twap timeout")
	}
	select {
	case event := <-sliceFills.Events():
		if !event.IsSnapshot || event.User != user {
			t.Fatalf("slice fills=%+v", event)
		}
	case <-time.After(time.Second):
		t.Fatal("slice fills timeout")
	}
	select {
	case event := <-history.Events():
		if len(event.History) != 1 || event.History[0].Status.Status != "finished" {
			t.Fatalf("history=%+v", event)
		}
	case <-time.After(time.Second):
		t.Fatal("history timeout")
	}
	select {
	case event := <-spot.Events():
		if event.User != user {
			t.Fatalf("spot=%+v", event)
		}
	case <-time.After(time.Second):
		t.Fatal("spot timeout")
	}
	select {
	case event := <-allClearing.Events():
		if len(event.ClearinghouseStates) != 1 {
			t.Fatalf("allClearing=%+v", event)
		}
	case <-time.After(time.Second):
		t.Fatal("all clearing timeout")
	}
	select {
	case event := <-allContexts.Events():
		if len(event.Contexts) != 1 || event.Contexts[0].Contexts[0].MarkPx.String() != "9.9" {
			t.Fatalf("allContexts=%+v", event)
		}
	case <-time.After(time.Second):
		t.Fatal("all contexts timeout")
	}
	select {
	case event := <-contexts.Events():
		if len(event.Contexts) != 1 {
			t.Fatalf("contexts=%+v", event)
		}
	case <-time.After(time.Second):
		t.Fatal("contexts timeout")
	}
	select {
	case event := <-fast.Events():
		if event["BTC"].MarkPrice.String() != "11.2" {
			t.Fatalf("fast=%+v", event)
		}
	case <-time.After(time.Second):
		t.Fatal("fast timeout")
	}
	select {
	case event := <-spotContexts.Events():
		if len(event) != 1 || event[0].MarkPx.String() != "3.5" {
			t.Fatalf("spot contexts=%+v", event)
		}
	case <-time.After(time.Second):
		t.Fatal("spot contexts timeout")
	}
	select {
	case event := <-historical.Events():
		if !event.IsSnapshot || len(event.OrderHistory) != 1 {
			t.Fatalf("historical=%+v", event)
		}
	case <-time.After(time.Second):
		t.Fatal("historical timeout")
	}
	select {
	case event := <-outcomes.Events():
		if len(event.Updates) != 1 || event.Updates[0].OutcomeCreated == nil {
			t.Fatalf("outcomes=%+v", event)
		}
	case <-time.After(time.Second):
		t.Fatal("outcomes timeout")
	}
}

func TestExtendedSubscriptionsDeduplicateAndGuardAmbiguousNotification(t *testing.T) {
	client := websocket.NewClient("ws://127.0.0.1:1")
	defer func() { _ = client.Close() }()
	ctx := context.Background()
	first, err := client.SubscribeNotification(ctx, "0xabc")
	if err != nil {
		t.Fatal(err)
	}
	second, err := client.SubscribeNotification(ctx, "0xabc")
	if err != nil {
		t.Fatal(err)
	}
	if first != second {
		t.Fatal("equal notification requests did not reuse the stream handle")
	}
	if _, err := client.SubscribeNotification(ctx, "0xdef"); err == nil {
		t.Fatal("notification subscriptions for different users must not share an ambiguous channel")
	}

	openA, err := client.SubscribeOpenOrders(ctx, websocket.UserDEXRequest{User: "0xabc", DEX: "abc"})
	if err != nil {
		t.Fatal(err)
	}
	openB, err := client.SubscribeOpenOrders(ctx, websocket.UserDEXRequest{User: "0xabc", DEX: "abc"})
	if err != nil {
		t.Fatal(err)
	}
	if openA != openB {
		t.Fatal("equal open-order requests did not reuse the stream handle")
	}
}

func TestExtendedSubscriptionReconnectsAndResubscribes(t *testing.T) {
	upgrader := gws.Upgrader{}
	connections := make(chan struct{}, 2)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		connection, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			t.Error(err)
			return
		}
		defer func() { _ = connection.Close() }()
		var request struct {
			Subscription struct {
				Type string `json:"type"`
				User string `json:"user"`
			} `json:"subscription"`
		}
		if err := connection.ReadJSON(&request); err != nil {
			t.Error(err)
			return
		}
		if request.Subscription.Type != "userTwapHistory" || request.Subscription.User != "0xabc" {
			t.Errorf("subscription=%+v", request.Subscription)
			return
		}
		connections <- struct{}{}
		if len(connections) == 2 {
			if err := connection.WriteJSON(map[string]any{"channel": "userTwapHistory", "data": map[string]any{"user": "0xabc", "history": []any{}}}); err != nil {
				t.Error(err)
			}
			<-time.After(time.Second)
		}
	}))
	defer server.Close()
	client := websocket.NewClient("ws"+strings.TrimPrefix(server.URL, "http"), websocket.Config{ReconnectDelay: time.Millisecond})
	defer func() { _ = client.Close() }()
	subscription, err := client.SubscribeUserTWAPHistory(context.Background(), "0xabc")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = subscription.Close() }()
	select {
	case event := <-subscription.Events():
		if event.User != "0xabc" {
			t.Fatalf("event=%+v", event)
		}
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for reconnected extended subscription")
	}
	if len(connections) != 2 {
		t.Fatalf("connections = %d, want 2", len(connections))
	}
}

func extendedFixture(t *testing.T, kind string) any {
	t.Helper()
	user := "0xabc"
	state := map[string]any{"assetPositions": []any{}, "crossMaintenanceMarginUsed": "0", "crossMarginSummary": map[string]any{"accountValue": "0", "totalMarginUsed": "0", "totalNtlPos": "0", "totalRawUsd": "0"}, "marginSummary": map[string]any{"accountValue": "0", "totalMarginUsed": "0", "totalNtlPos": "0", "totalRawUsd": "0"}, "time": 1, "withdrawable": "0"}
	twapState := map[string]any{"coin": "BTC", "executedNtl": "1", "executedSz": "1", "minutes": 5, "randomize": false, "reduceOnly": false, "side": "B", "sz": "2.5", "timestamp": 1, "user": user}
	context := map[string]any{"dayNtlVlm": "1", "funding": "0", "markPx": "9.9", "midPx": "9.8", "openInterest": "2", "oraclePx": "10", "prevDayPx": "8", "impactPxs": []any{"9", "10"}}
	switch kind {
	case "notification":
		return map[string]any{"notification": "notice"}
	case "webData3":
		return map[string]any{"userState": map[string]any{"cumLedger": "1.25", "serverTime": 1, "isVault": false, "user": user}, "perpDexStates": []any{map[string]any{"totalVaultEquity": "0", "leadingVaults": []any{map[string]any{"address": "0xvault", "name": "vault"}}}}}
	case "openOrders":
		return map[string]any{"user": user, "dex": "", "orders": []any{}}
	case "clearinghouseState":
		return map[string]any{"user": user, "dex": "", "clearinghouseState": state}
	case "activeAssetData":
		return map[string]any{"user": user, "coin": "BTC", "leverage": map[string]any{"type": "cross", "value": 2}, "maxTradeSzs": []any{"1", "2"}, "availableToTrade": []any{"1", "2"}, "markPx": "100.1"}
	case "twapStates":
		return map[string]any{"user": user, "dex": "", "states": []any{[]any{1, twapState}}}
	case "userTwapSliceFills":
		return map[string]any{"user": user, "isSnapshot": true, "twapSliceFills": []any{}}
	case "userTwapHistory":
		return map[string]any{"user": user, "isSnapshot": true, "history": []any{map[string]any{"time": 1, "state": twapState, "status": map[string]any{"status": "finished"}, "twapId": 1}}}
	case "spotState":
		return map[string]any{"user": user, "spotState": map[string]any{"balances": []any{}}}
	case "allDexsClearinghouseState":
		return map[string]any{"user": user, "clearinghouseStates": []any{[]any{"", state}}}
	case "allDexsAssetCtxs":
		return map[string]any{"ctxs": []any{[]any{"", []any{context}}}}
	case "assetCtxs":
		return map[string]any{"dex": "", "ctxs": []any{context}}
	case "fastAssetCtxs":
		return rawDeflateBase64(t, `{"BTC":{"markPx":"11.2","midPx":"11.1"}}`)
	case "spotAssetCtxs":
		return []any{map[string]any{"dayNtlVlm": "1", "markPx": "3.5", "midPx": "3.4", "prevDayPx": "3", "circulatingSupply": "100", "coin": "@1"}}
	case "userHistoricalOrders":
		return map[string]any{"user": user, "isSnapshot": true, "orderHistory": []any{map[string]any{"order": map[string]any{"coin": "BTC", "limitPx": "1", "oid": 1, "side": "B", "sz": "1", "timestamp": 1, "isPositionTpsl": false, "isTrigger": false, "orderType": "Limit", "origSz": "1", "reduceOnly": false, "triggerCondition": "N/A", "triggerPx": "0"}, "status": "filled", "statusTimestamp": 1}}}
	case "outcomeMetaUpdates":
		return map[string]any{"updates": []any{map[string]any{"outcomeCreated": map[string]any{"outcome": 1, "name": "yes", "description": "yes", "sideSpecs": []any{}, "quoteToken": "USDC"}}}}
	default:
		t.Fatalf("unknown fixture kind %q", kind)
		return nil
	}
}

func validateExtendedWire(t *testing.T, kind string, got map[string]any) {
	t.Helper()
	user := "0xabc"
	want := map[string]any{"type": kind}
	switch kind {
	case "notification", "webData3", "userTwapSliceFills", "userTwapHistory", "allDexsClearinghouseState", "userHistoricalOrders":
		want["user"] = user
	case "openOrders", "clearinghouseState", "twapStates":
		want["user"] = user
		want["dex"] = ""
	case "activeAssetData":
		want["user"] = user
		want["coin"] = "BTC"
	case "spotState":
		want["user"] = user
		want["isPortfolioMargin"] = true
	case "assetCtxs":
		want["dex"] = ""
	case "allDexsAssetCtxs", "fastAssetCtxs", "spotAssetCtxs", "outcomeMetaUpdates":
	default:
		t.Fatalf("unrecognized subscription wire %q", kind)
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("%s wire = %#v, want %#v", kind, got, want)
	}
}

func rawDeflateBase64(t *testing.T, value string) string {
	t.Helper()
	var buffer bytes.Buffer
	writer, err := flate.NewWriter(&buffer, flate.DefaultCompression)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := writer.Write([]byte(value)); err != nil {
		t.Fatal(err)
	}
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}
	return base64.StdEncoding.EncodeToString(buffer.Bytes())
}
