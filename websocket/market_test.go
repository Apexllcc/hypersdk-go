package websocket_test

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/Apexllcc/hyperliquid-go-sdk/websocket"
	gws "github.com/gorilla/websocket"
)

type failingDialer struct{ calls atomic.Int32 }

func (d *failingDialer) DialContext(context.Context, string) (*gws.Conn, error) {
	d.calls.Add(1)
	return nil, errors.New("dial intercepted")
}

type blockingDialer struct {
	started  chan struct{}
	finished chan struct{}
}

func (d *blockingDialer) DialContext(ctx context.Context, _ string) (*gws.Conn, error) {
	close(d.started)
	<-ctx.Done()
	close(d.finished)
	return nil, ctx.Err()
}

func TestAllMidsSubscriptionSendsOptionalDEXAndDecodesDecimals(t *testing.T) {
	t.Parallel()
	upgrader := gws.Upgrader{}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		conn, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			t.Error(err)
			return
		}
		defer func() { _ = conn.Close() }()
		var request struct {
			Method       string         `json:"method"`
			Subscription map[string]any `json:"subscription"`
		}
		if err := conn.ReadJSON(&request); err != nil {
			t.Error(err)
			return
		}
		if request.Method != "subscribe" || request.Subscription["type"] != "allMids" || request.Subscription["dex"] != "xyz" {
			t.Errorf("subscription=%+v", request)
		}
		_ = conn.WriteJSON(map[string]any{"channel": "allMids", "data": map[string]any{"mids": map[string]string{"BTC": "123.4500"}}})
		<-time.After(time.Second)
	}))
	defer server.Close()

	client := websocket.NewClient("ws" + strings.TrimPrefix(server.URL, "http"))
	subscription, err := client.SubscribeAllMids(context.Background(), websocket.AllMidsRequest{DEX: "xyz"})
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = subscription.Close() }()
	select {
	case event := <-subscription.Events():
		if got := event.Mids["BTC"].String(); got != "123.45" {
			t.Fatalf("mid=%s", got)
		}
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for allMids event")
	}
}

func TestMarketSubscriptionsDecodeTradesCandleAndBBO(t *testing.T) {
	t.Parallel()
	upgrader := gws.Upgrader{}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		conn, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			t.Error(err)
			return
		}
		defer func() { _ = conn.Close() }()
		var request struct {
			Subscription struct {
				Type string `json:"type"`
			} `json:"subscription"`
		}
		if err := conn.ReadJSON(&request); err != nil {
			t.Error(err)
			return
		}
		switch request.Subscription.Type {
		case "trades":
			_ = conn.WriteJSON(map[string]any{"channel": "trades", "data": []map[string]any{{"coin": "BTC", "side": "B", "px": "10.1", "sz": "2.3", "hash": "0xabc", "time": 1, "tid": 2, "users": []string{"0xa", "0xb"}}}})
		case "candle":
			_ = conn.WriteJSON(map[string]any{"channel": "candle", "data": []map[string]any{{"t": 1, "T": 2, "s": "BTC", "i": "1m", "o": "1.2", "c": "1.3", "h": "1.4", "l": "1.1", "v": "4.5", "n": 6}}})
		case "bbo":
			_ = conn.WriteJSON(map[string]any{"channel": "bbo", "data": map[string]any{"coin": "BTC", "time": 1, "bbo": []any{map[string]any{"px": "1.2", "sz": "3.4", "n": 5}, nil}}})
		default:
			t.Errorf("unexpected subscription type %q", request.Subscription.Type)
		}
		<-time.After(time.Second)
	}))
	defer server.Close()
	url := "ws" + strings.TrimPrefix(server.URL, "http")

	trades, err := websocket.NewClient(url).SubscribeTrades(context.Background(), websocket.TradesRequest{Coin: "BTC"})
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = trades.Close() }()
	candle, err := websocket.NewClient(url).SubscribeCandle(context.Background(), websocket.CandleRequest{Coin: "BTC", Interval: "1m"})
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = candle.Close() }()
	bbo, err := websocket.NewClient(url).SubscribeBBO(context.Background(), websocket.BBORequest{Coin: "BTC"})
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = bbo.Close() }()

	select {
	case events := <-trades.Events():
		if len(events) != 1 || events[0].Price.String() != "10.1" || events[0].Size.String() != "2.3" {
			t.Fatalf("trades=%+v", events)
		}
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for trades")
	}
	select {
	case events := <-candle.Events():
		if len(events) != 1 || events[0].Open.String() != "1.2" || events[0].Volume.String() != "4.5" {
			t.Fatalf("candle=%+v", events)
		}
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for candle")
	}
	select {
	case event := <-bbo.Events():
		if event.Bid == nil || event.Ask != nil || event.Bid.Price.String() != "1.2" {
			t.Fatalf("bbo=%+v", event)
		}
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for bbo")
	}
}

func TestSubscriptionUsesConfiguredDialer(t *testing.T) {
	dialer := &failingDialer{}
	client := websocket.NewClient("ws://unused", websocket.Config{Dialer: dialer, ReconnectDelay: time.Hour})
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	subscription, err := client.SubscribeAllMids(ctx, websocket.AllMidsRequest{})
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = subscription.Close() }()
	deadline := time.After(time.Second)
	for dialer.calls.Load() == 0 {
		select {
		case <-deadline:
			t.Fatal("configured dialer was not called")
		case <-time.After(time.Millisecond):
		}
	}
}

func TestClientSharesOneConnectionAcrossMarketSubscriptions(t *testing.T) {
	upgrader := gws.Upgrader{}
	connections := atomic.Int32{}
	receivedSubscriptions := make(chan struct{})
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		connections.Add(1)
		conn, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			t.Error(err)
			return
		}
		defer func() { _ = conn.Close() }()
		for i := 0; i < 3; i++ {
			var request map[string]any
			if err := conn.ReadJSON(&request); err != nil {
				t.Error(err)
				return
			}
		}
		close(receivedSubscriptions)
		<-time.After(time.Second)
	}))
	defer server.Close()
	client := websocket.NewClient("ws" + strings.TrimPrefix(server.URL, "http"))
	defer func() { _ = client.Close() }()
	if _, err := client.SubscribeTrades(context.Background(), websocket.TradesRequest{Coin: "BTC"}); err != nil {
		t.Fatal(err)
	}
	if _, err := client.SubscribeCandle(context.Background(), websocket.CandleRequest{Coin: "BTC", Interval: "1m"}); err != nil {
		t.Fatal(err)
	}
	if _, err := client.SubscribeBBO(context.Background(), websocket.BBORequest{Coin: "BTC"}); err != nil {
		t.Fatal(err)
	}
	deadline := time.After(time.Second)
	for connections.Load() != 1 {
		select {
		case <-deadline:
			t.Fatalf("connections=%d", connections.Load())
		case <-time.After(time.Millisecond):
		}
	}
	select {
	case <-receivedSubscriptions:
	case <-time.After(time.Second):
		t.Fatal("server did not receive all subscription messages")
	}
}

func TestHeartbeatUsesApplicationPing(t *testing.T) {
	upgrader := gws.Upgrader{}
	pingSeen := make(chan struct{})
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		conn, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			t.Error(err)
			return
		}
		defer func() { _ = conn.Close() }()
		if _, _, err := conn.ReadMessage(); err != nil { // subscription
			t.Error(err)
			return
		}
		var ping struct {
			Method string `json:"method"`
		}
		if err := conn.ReadJSON(&ping); err != nil {
			t.Error(err)
			return
		}
		if ping.Method != "ping" {
			t.Errorf("method=%q", ping.Method)
			return
		}
		_ = conn.WriteJSON(map[string]any{"channel": "pong"})
		close(pingSeen)
		<-time.After(time.Second)
	}))
	defer server.Close()
	client := websocket.NewClient("ws"+strings.TrimPrefix(server.URL, "http"), websocket.Config{PingInterval: time.Millisecond, PongWait: time.Second})
	defer func() { _ = client.Close() }()
	if _, err := client.SubscribeAllMids(context.Background(), websocket.AllMidsRequest{}); err != nil {
		t.Fatal(err)
	}
	select {
	case <-pingSeen:
	case <-time.After(time.Second):
		t.Fatal("did not receive application heartbeat")
	}
}

func TestDuplicateMarketSubscriptionReturnsSameHandle(t *testing.T) {
	client := websocket.NewClient("ws://unused")
	defer func() { _ = client.Close() }()
	first, err := client.SubscribeAllMids(context.Background(), websocket.AllMidsRequest{})
	if err != nil {
		t.Fatal(err)
	}
	second, err := client.SubscribeAllMids(context.Background(), websocket.AllMidsRequest{})
	if err != nil {
		t.Fatal(err)
	}
	if first != second {
		t.Fatal("duplicate allMids subscription returned a distinct handle")
	}
}

func TestAllMidsRejectsAmbiguousDEXSubscription(t *testing.T) {
	client := websocket.NewClient("ws://unused")
	defer func() { _ = client.Close() }()
	first, err := client.SubscribeAllMids(context.Background(), websocket.AllMidsRequest{DEX: "dex-a"})
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = first.Close() }()
	if _, err := client.SubscribeAllMids(context.Background(), websocket.AllMidsRequest{DEX: "dex-b"}); !errors.Is(err, websocket.ErrAmbiguousAllMids) {
		t.Fatalf("error=%v", err)
	}
}

func TestCanceledSubscriptionCannotLeaveStaleMarketHandle(t *testing.T) {
	client := websocket.NewClient("ws://unused")
	defer func() { _ = client.Close() }()
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := client.SubscribeTrades(ctx, websocket.TradesRequest{Coin: "BTC"}); !errors.Is(err, context.Canceled) {
		t.Fatalf("cancelled subscription error=%v", err)
	}
	active, err := client.SubscribeTrades(context.Background(), websocket.TradesRequest{Coin: "BTC"})
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = active.Close() }()
	again, err := client.SubscribeTrades(context.Background(), websocket.TradesRequest{Coin: "BTC"})
	if err != nil {
		t.Fatal(err)
	}
	if active != again {
		t.Fatal("active subscription did not own the cached handle")
	}
}

func TestClosingLastSubscriptionCancelsInFlightDial(t *testing.T) {
	dialer := &blockingDialer{started: make(chan struct{}), finished: make(chan struct{})}
	client := websocket.NewClient("ws://unused", websocket.Config{Dialer: dialer})
	defer func() { _ = client.Close() }()
	subscription, err := client.SubscribeAllMids(context.Background(), websocket.AllMidsRequest{})
	if err != nil {
		t.Fatal(err)
	}
	select {
	case <-dialer.started:
	case <-time.After(time.Second):
		t.Fatal("dial did not start")
	}
	if err := subscription.Close(); err != nil {
		t.Fatal(err)
	}
	select {
	case <-dialer.finished:
	case <-time.After(time.Second):
		t.Fatal("closing the last subscription did not cancel the dial")
	}
}

func TestUserEventDecodesFillAndFundingVariants(t *testing.T) {
	for _, test := range []struct {
		name  string
		data  string
		check func(*testing.T, websocket.UserEvent)
	}{
		{
			name: "fill",
			data: `{"fills":[{"coin":"BTC","px":"1.2","sz":"3.4","side":"B","time":1,"startPosition":"0","dir":"Open Long","closedPnl":"0","hash":"0xabc","oid":2,"crossed":true,"fee":"0.01","tid":3,"feeToken":"USDC"}]}`,
			check: func(t *testing.T, event websocket.UserEvent) {
				if len(event.Fills) != 1 || event.Fills[0].Price.String() != "1.2" || event.Fills[0].Size.String() != "3.4" {
					t.Fatalf("fills=%+v", event.Fills)
				}
			},
		},
		{
			name: "funding",
			data: `{"funding":{"time":1,"coin":"BTC","usdc":"2.3","szi":"4.5","fundingRate":"0.001"}}`,
			check: func(t *testing.T, event websocket.UserEvent) {
				if event.Funding == nil || event.Funding.USDC.String() != "2.3" || event.Funding.Size.String() != "4.5" {
					t.Fatalf("funding=%+v", event.Funding)
				}
			},
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			var event websocket.UserEvent
			if err := json.Unmarshal([]byte(test.data), &event); err != nil {
				t.Fatal(err)
			}
			test.check(t, event)
		})
	}
}
