package websocket

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

	"github.com/gorilla/websocket"
)

func TestReconnectPolicyExponentiallyBacksOffAndCaps(t *testing.T) {
	t.Parallel()
	policy := NewExponentialReconnectPolicy(10*time.Millisecond, 35*time.Millisecond, func(delay time.Duration) time.Duration {
		return delay
	})

	want := []time.Duration{10 * time.Millisecond, 20 * time.Millisecond, 35 * time.Millisecond, 35 * time.Millisecond}
	for attempt, expected := range want {
		if got := policy.Delay(attempt); got != expected {
			t.Fatalf("attempt %d: delay=%s, want %s", attempt, got, expected)
		}
	}
}

func TestReconnectPolicyAppliesInjectedJitter(t *testing.T) {
	t.Parallel()
	policy := NewExponentialReconnectPolicy(time.Second, 10*time.Second, func(delay time.Duration) time.Duration {
		return delay / 4
	})

	if got, want := policy.Delay(2), time.Second; got != want {
		t.Fatalf("delay=%s, want %s", got, want)
	}
}

func TestConfigReconnectDelayRetainsLegacyFixedDelay(t *testing.T) {
	t.Parallel()
	config := (Config{ReconnectDelay: 7 * time.Millisecond}).normalized()

	if got, want := config.ReconnectPolicy.Delay(0), 7*time.Millisecond; got != want {
		t.Fatalf("first reconnect delay=%s, want %s", got, want)
	}
	if got, want := config.ReconnectPolicy.Delay(5), 7*time.Millisecond; got != want {
		t.Fatalf("legacy reconnect delay=%s, want %s", got, want)
	}
}

func TestDefaultConfigUsesBoundedExponentialPolicyWithJitter(t *testing.T) {
	t.Parallel()
	config := (Config{}).normalized()
	policy, ok := config.ReconnectPolicy.(*ExponentialReconnectPolicy)
	if !ok {
		t.Fatalf("default policy type=%T, want exponential", config.ReconnectPolicy)
	}
	if policy.InitialDelay != time.Second || policy.MaxDelay != 30*time.Second {
		t.Fatalf("policy=%+v", policy)
	}
	if got := policy.Delay(0); got < 500*time.Millisecond || got > time.Second {
		t.Fatalf("jittered initial delay=%s, want within [500ms, 1s]", got)
	}
}

func TestConfigUsesCustomReconnectPolicy(t *testing.T) {
	t.Parallel()
	policy := ReconnectPolicyFunc(func(attempt int) time.Duration { return time.Duration(attempt+1) * time.Millisecond })
	config := (Config{ReconnectPolicy: policy}).normalized()

	if got, want := config.ReconnectPolicy.Delay(4), 5*time.Millisecond; got != want {
		t.Fatalf("custom policy delay=%s, want %s", got, want)
	}
}

func TestConfigReplacesTypedNilReconnectPolicy(t *testing.T) {
	t.Parallel()
	var policy ReconnectPolicyFunc
	config := (Config{ReconnectPolicy: policy}).normalized()

	if got := config.ReconnectPolicy.Delay(0); got < 500*time.Millisecond || got > time.Second {
		t.Fatalf("typed-nil policy was not replaced by the default: delay=%s", got)
	}
}

func TestConnectionManagerUsesReconnectPolicyAttempt(t *testing.T) {
	t.Parallel()
	attempts := make(chan int, 1)
	client := &Client{
		config: Config{ReconnectPolicy: ReconnectPolicyFunc(func(attempt int) time.Duration {
			attempts <- attempt
			return 0
		})},
		subs: map[string]managedSubscription{"test": reconnectTestSubscription{}},
	}
	manager := &connectionManager{client: client, wake: make(chan struct{}, 1), done: make(chan struct{})}

	if result := manager.waitReconnect(3); result != reconnectWaitElapsed {
		t.Fatalf("waitReconnect result=%d, want elapsed", result)
	}
	select {
	case attempt := <-attempts:
		if attempt != 3 {
			t.Fatalf("attempt=%d, want 3", attempt)
		}
	default:
		t.Fatal("reconnect policy was not called")
	}
}

func TestConnectionManagerResetsBackoffAfterSuccessfulDial(t *testing.T) {
	upgrader := websocket.Upgrader{}
	var connections atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		connection, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			t.Error(err)
			return
		}
		connections.Add(1)
		_ = connection.Close()
	}))
	defer server.Close()

	attempts := make(chan int, 8)
	client := NewClient("ws"+strings.TrimPrefix(server.URL, "http"), Config{ReconnectPolicy: ReconnectPolicyFunc(func(attempt int) time.Duration {
		attempts <- attempt
		return time.Millisecond
	})})
	defer func() { _ = client.Close() }()
	subscription, err := client.SubscribeAllMids(context.Background(), AllMidsRequest{})
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = subscription.Close() }()

	for i := 0; i < 2; i++ {
		select {
		case attempt := <-attempts:
			if attempt != 0 {
				t.Fatalf("reconnect attempt=%d, want reset attempt 0", attempt)
			}
		case <-time.After(time.Second):
			t.Fatalf("timed out after %d successful connections", connections.Load())
		}
	}
}

func TestSubscriptionWakeDoesNotConsumeReconnectAttempt(t *testing.T) {
	attempts := make(chan int, 2)
	client := NewClient("ws://unused", Config{
		Dialer: reconnectFailingDialer{},
		ReconnectPolicy: ReconnectPolicyFunc(func(attempt int) time.Duration {
			attempts <- attempt
			return time.Hour
		}),
	})
	defer func() { _ = client.Close() }()
	first, err := client.SubscribeAllMids(context.Background(), AllMidsRequest{})
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = first.Close() }()
	select {
	case attempt := <-attempts:
		if attempt != 0 {
			t.Fatalf("initial attempt=%d, want 0", attempt)
		}
	case <-time.After(time.Second):
		t.Fatal("first reconnect wait did not start")
	}

	second, err := client.SubscribeTrades(context.Background(), TradesRequest{Coin: "BTC"})
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = second.Close() }()
	select {
	case attempt := <-attempts:
		if attempt != 0 {
			t.Fatalf("wake consumed reconnect attempt: got %d, want 0", attempt)
		}
	case <-time.After(time.Second):
		t.Fatal("wake did not trigger an immediate reconnect")
	}
}

func TestClientCloseInterruptsLongReconnectWait(t *testing.T) {
	backoffEntered := make(chan struct{})
	extraDial := make(chan struct{}, 1)
	var dials atomic.Int32
	client := NewClient("ws://unused", Config{
		Dialer: reconnectCloseTestDialer{dials: &dials, extraDial: extraDial},
		ReconnectPolicy: ReconnectPolicyFunc(func(int) time.Duration {
			select {
			case <-backoffEntered:
			default:
				close(backoffEntered)
			}
			return time.Hour
		}),
	})
	subscription, err := client.SubscribeAllMids(context.Background(), AllMidsRequest{})
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = subscription.Close() }()
	select {
	case <-backoffEntered:
	case <-time.After(time.Second):
		t.Fatal("reconnect wait did not begin")
	}

	beforeClose := dials.Load()
	started := time.Now()
	if err := client.Close(); err != nil {
		t.Fatal(err)
	}
	if elapsed := time.Since(started); elapsed > time.Second {
		t.Fatalf("Close waited %s during reconnect backoff", elapsed)
	}
	assertNoUnexpectedDial(t, extraDial)
	if got := dials.Load(); got != beforeClose {
		t.Fatalf("dials after Close = %d, want %d", got, beforeClose)
	}
}

func TestInitialSubscriptionKeepsFirstReconnectDelay(t *testing.T) {
	backoffEntered := make(chan struct{})
	extraDial := make(chan struct{}, 1)
	var dials atomic.Int32
	client := NewClient("ws://unused", Config{
		Dialer: reconnectCloseTestDialer{dials: &dials, extraDial: extraDial},
		ReconnectPolicy: ReconnectPolicyFunc(func(int) time.Duration {
			select {
			case <-backoffEntered:
			default:
				close(backoffEntered)
			}
			return time.Hour
		}),
	})
	defer func() { _ = client.Close() }()
	subscription, err := client.SubscribeAllMids(context.Background(), AllMidsRequest{})
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = subscription.Close() }()
	select {
	case <-backoffEntered:
	case <-time.After(time.Second):
		t.Fatal("first reconnect delay did not begin")
	}
	assertNoUnexpectedDial(t, extraDial)
	if got := dials.Load(); got != 1 {
		t.Fatalf("dials during first reconnect delay = %d, want 1", got)
	}
}

func TestClientCloseWaitsForInFlightDial(t *testing.T) {
	dialer := newCloseWaitDialer()
	client := NewClient("ws://unused", Config{Dialer: dialer})
	subscription, err := client.SubscribeAllMids(context.Background(), AllMidsRequest{})
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = subscription.Close() }()
	select {
	case <-dialer.entered:
	case <-time.After(time.Second):
		t.Fatal("dial did not begin")
	}

	closed := make(chan error, 1)
	go func() { closed <- client.Close() }()
	select {
	case <-dialer.canceled:
	case <-time.After(time.Second):
		t.Fatal("Close did not cancel in-flight dial")
	}
	select {
	case err := <-closed:
		t.Fatalf("Close returned before the in-flight dial exited: %v", err)
	default:
	}
	close(dialer.allowReturn)
	select {
	case err := <-closed:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(time.Second):
		t.Fatal("Close did not wait for the in-flight dial to exit")
	}
}

func assertNoUnexpectedDial(t *testing.T, extraDial <-chan struct{}) {
	t.Helper()
	select {
	case <-extraDial:
		t.Fatal("unexpected extra dial")
	case <-time.After(100 * time.Millisecond):
	}
}

type reconnectTestSubscription struct{}

type reconnectFailingDialer struct{}

func (reconnectFailingDialer) DialContext(context.Context, string) (*websocket.Conn, error) {
	return nil, errors.New("dial failed")
}

type reconnectCloseTestDialer struct {
	dials     *atomic.Int32
	extraDial chan<- struct{}
}

type closeWaitDialer struct {
	entered     chan struct{}
	canceled    chan struct{}
	allowReturn chan struct{}
}

func newCloseWaitDialer() closeWaitDialer {
	return closeWaitDialer{entered: make(chan struct{}), canceled: make(chan struct{}), allowReturn: make(chan struct{})}
}

func (d closeWaitDialer) DialContext(ctx context.Context, _ string) (*websocket.Conn, error) {
	close(d.entered)
	<-ctx.Done()
	close(d.canceled)
	<-d.allowReturn
	return nil, ctx.Err()
}

func (d reconnectCloseTestDialer) DialContext(context.Context, string) (*websocket.Conn, error) {
	if d.dials.Add(1) > 1 && d.extraDial != nil {
		select {
		case d.extraDial <- struct{}{}:
		default:
		}
	}
	return nil, errors.New("dial failed")
}

func (reconnectTestSubscription) Errors() <-chan error               { return nil }
func (reconnectTestSubscription) Close() error                       { return nil }
func (reconnectTestSubscription) subscriptionKey() string            { return "test" }
func (reconnectTestSubscription) subscriptionWire() subscriptionWire { return subscriptionWire{} }
func (reconnectTestSubscription) subscriptionChannel() string        { return "test" }
func (reconnectTestSubscription) deliverRaw(json.RawMessage)         {}
func (reconnectTestSubscription) isDone() bool                       { return false }
