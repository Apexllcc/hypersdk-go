package websocket

import (
	"context"
	"errors"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"runtime"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	gorilla "github.com/gorilla/websocket"
)

type task5Round2WriteMode uint8

const (
	task5Round2BlockWrite task5Round2WriteMode = iota
	task5Round2FailWrite
)

type task5Round2ControlledConn struct {
	net.Conn
	mode task5Round2WriteMode

	armed atomic.Bool

	mu       sync.Mutex
	deadline time.Time
	cancel   context.CancelFunc
	failErr  error

	entered         chan struct{}
	release         chan struct{}
	closed          chan struct{}
	deadlineSet     chan struct{}
	deadlineCleared chan struct{}
	enterOnce       sync.Once
	releaseOnce     sync.Once
	closeOnce       sync.Once
	deadlineSetOnce sync.Once
	deadlineClrOnce sync.Once
}

func newTask5Round2ControlledConn(connection net.Conn, mode task5Round2WriteMode, failErr error) *task5Round2ControlledConn {
	return &task5Round2ControlledConn{
		Conn: connection, mode: mode, failErr: failErr,
		entered: make(chan struct{}), release: make(chan struct{}), closed: make(chan struct{}),
		deadlineSet: make(chan struct{}), deadlineCleared: make(chan struct{}),
	}
}

func (c *task5Round2ControlledConn) Write(payload []byte) (int, error) {
	if !c.armed.Load() {
		return c.Conn.Write(payload)
	}
	c.enterOnce.Do(func() { close(c.entered) })
	c.mu.Lock()
	cancel, deadline := c.cancel, c.deadline
	c.mu.Unlock()
	if cancel != nil {
		cancel()
	}
	if c.mode == task5Round2FailWrite {
		return 0, c.failErr
	}
	var deadlineC <-chan time.Time
	var timer *time.Timer
	if !deadline.IsZero() {
		delay := time.Until(deadline)
		if delay < 0 {
			delay = 0
		}
		timer = time.NewTimer(delay)
		deadlineC = timer.C
		defer timer.Stop()
	}
	select {
	case <-c.release:
		return 0, errors.New("test released blocked websocket write")
	case <-c.closed:
		return 0, net.ErrClosed
	case <-deadlineC:
		return 0, os.ErrDeadlineExceeded
	}
}

func (c *task5Round2ControlledConn) SetWriteDeadline(deadline time.Time) error {
	if !c.armed.Load() {
		return c.Conn.SetWriteDeadline(deadline)
	}
	c.mu.Lock()
	c.deadline = deadline
	c.mu.Unlock()
	if deadline.IsZero() {
		c.deadlineClrOnce.Do(func() { close(c.deadlineCleared) })
	} else {
		c.deadlineSetOnce.Do(func() { close(c.deadlineSet) })
	}
	return nil
}

func (c *task5Round2ControlledConn) Close() error {
	c.closeOnce.Do(func() { close(c.closed) })
	return c.Conn.Close()
}

func (c *task5Round2ControlledConn) releaseWrite() {
	c.releaseOnce.Do(func() { close(c.release) })
}

func (c *task5Round2ControlledConn) setCancel(cancel context.CancelFunc) {
	c.mu.Lock()
	c.cancel = cancel
	c.mu.Unlock()
}

type task5Round2Dialer struct {
	mode    task5Round2WriteMode
	failErr error
	ready   chan *task5Round2ControlledConn
	dials   atomic.Int32
}

func newTask5Round2Dialer(mode task5Round2WriteMode, failErr error) *task5Round2Dialer {
	return &task5Round2Dialer{mode: mode, failErr: failErr, ready: make(chan *task5Round2ControlledConn, 1)}
}

func (d *task5Round2Dialer) DialContext(ctx context.Context, url string) (*gorilla.Conn, error) {
	attempt := d.dials.Add(1)
	dialer := *gorilla.DefaultDialer
	var controlled *task5Round2ControlledConn
	dialer.NetDialContext = func(ctx context.Context, network, address string) (net.Conn, error) {
		connection, err := (&net.Dialer{}).DialContext(ctx, network, address)
		if err != nil || attempt != 1 {
			return connection, err
		}
		controlled = newTask5Round2ControlledConn(connection, d.mode, d.failErr)
		return controlled, nil
	}
	connection, _, err := dialer.DialContext(ctx, url, nil)
	if err != nil {
		return nil, err
	}
	if controlled != nil {
		controlled.armed.Store(true)
		d.ready <- controlled
	}
	return connection, nil
}

func TestClientCloseInterruptsBlockedSubscriptionWrite(t *testing.T) {
	upgrader := gorilla.Upgrader{}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		connection, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			return
		}
		defer func() { _ = connection.Close() }()
		_, _, _ = connection.ReadMessage()
	}))
	defer server.Close()

	dialer := newTask5Round2Dialer(task5Round2BlockWrite, nil)
	client := NewClient("ws"+strings.TrimPrefix(server.URL, "http"), Config{
		Dialer:                 dialer,
		PingInterval:           time.Hour,
		SubscriptionAckTimeout: time.Hour,
	})
	_, err := client.SubscribeTrades(context.Background(), TradesRequest{Coin: "BLOCKED"})
	if err != nil {
		t.Fatal(err)
	}
	connection := <-dialer.ready
	select {
	case <-connection.entered:
	case <-time.After(time.Second):
		t.Fatal("subscription write did not block")
	}

	closed := make(chan error, 1)
	go func() { closed <- client.Close() }()
	select {
	case err := <-closed:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(150 * time.Millisecond):
		connection.releaseWrite()
		<-closed
		t.Fatal("Client.Close waited for a blocked subscription write")
	}
	select {
	case <-connection.closed:
	default:
		t.Fatal("Client.Close did not close the active subscription connection")
	}
}

func TestBlockedSubscriptionWriteDeadlineAllowsMutationAndReconnect(t *testing.T) {
	upgrader := gorilla.Upgrader{}
	var generations atomic.Int32
	secondGenerationWires := make(chan string, 2)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		connection, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			return
		}
		defer func() { _ = connection.Close() }()
		generation := generations.Add(1)
		for {
			var request subscriptionWire
			if err := connection.ReadJSON(&request); err != nil {
				return
			}
			if generation == 1 {
				continue
			}
			coin, _ := request.Subscription["coin"].(string)
			secondGenerationWires <- coin
			if err := connection.WriteJSON(map[string]any{
				"channel": "subscriptionResponse",
				"data": map[string]any{
					"method":       request.Method,
					"subscription": request.Subscription,
				},
			}); err != nil {
				return
			}
		}
	}))
	defer server.Close()

	dialer := newTask5Round2Dialer(task5Round2BlockWrite, nil)
	client := NewClient("ws"+strings.TrimPrefix(server.URL, "http"), Config{
		Dialer:                 dialer,
		PingInterval:           time.Hour,
		ReconnectDelay:         time.Millisecond,
		SubscriptionAckTimeout: 30 * time.Millisecond,
	})
	defer func() { _ = client.Close() }()
	first, err := client.SubscribeTrades(context.Background(), TradesRequest{Coin: "FIRST"})
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = first.Close() }()
	connection := <-dialer.ready
	select {
	case <-connection.entered:
	case <-time.After(time.Second):
		t.Fatal("subscription write did not block")
	}

	type subscribeResult struct {
		subscription *TradesSubscription
		err          error
	}
	mutated := make(chan subscribeResult, 1)
	go func() {
		subscription, err := client.SubscribeTrades(context.Background(), TradesRequest{Coin: "SECOND"})
		mutated <- subscribeResult{subscription: subscription, err: err}
	}()
	var second *TradesSubscription
	select {
	case result := <-mutated:
		if result.err != nil {
			t.Fatal(result.err)
		}
		second = result.subscription
	case <-time.After(time.Second):
		connection.releaseWrite()
		t.Fatal("registry mutation remained blocked behind subscription network I/O")
	}
	defer func() { _ = second.Close() }()

	select {
	case <-connection.deadlineSet:
	default:
		t.Fatal("subscription write did not install a bounded write deadline")
	}
	select {
	case <-connection.deadlineCleared:
	default:
		t.Fatal("subscription write did not clear its write deadline")
	}
	want := map[string]bool{"FIRST": false, "SECOND": false}
	for range 2 {
		select {
		case coin := <-secondGenerationWires:
			if _, ok := want[coin]; !ok {
				t.Fatalf("unexpected second-generation subscription %q", coin)
			}
			want[coin] = true
		case <-time.After(time.Second):
			t.Fatal("live subscriptions did not reconnect after write timeout")
		}
	}
	for coin, seen := range want {
		if !seen {
			t.Fatalf("second generation missed %s", coin)
		}
	}
}

func TestPostWriteErrorWithConcurrentContextCancelAlwaysCleansPendingAndDisconnects(t *testing.T) {
	upgrader := gorilla.Upgrader{}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		connection, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			return
		}
		defer func() { _ = connection.Close() }()
		for {
			if _, _, err := connection.ReadMessage(); err != nil {
				return
			}
		}
	}))
	defer server.Close()

	wantWriteErr := errors.New("forced websocket write failure")
	dialer := newTask5Round2Dialer(task5Round2FailWrite, wantWriteErr)
	client := NewClient("ws"+strings.TrimPrefix(server.URL, "http"), Config{Dialer: dialer})
	defer func() { _ = client.Close() }()
	badConnection, err := client.posts.connection(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	controlled := <-dialer.ready
	ctx, cancel := context.WithCancel(context.Background())
	controlled.setCancel(cancel)
	previousProcs := runtime.GOMAXPROCS(1)
	err = client.PostInfo(ctx, map[string]string{"type": "race"}, &map[string]any{})
	runtime.GOMAXPROCS(previousProcs)
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("POST error = %v, want context cancellation", err)
	}

	client.posts.mu.Lock()
	pendingCount := len(client.posts.pending)
	retained := client.posts.conn
	client.posts.mu.Unlock()
	if pendingCount != 0 {
		t.Fatalf("POST write error retained %d pending request(s)", pendingCount)
	}
	if retained != nil {
		t.Fatal("POST write error retained the corrupted connection")
	}
	select {
	case <-controlled.closed:
	default:
		t.Fatal("POST write error did not close the corrupted connection")
	}
	replacement, err := client.posts.connection(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if replacement == badConnection {
		t.Fatal("POST connection was not replaced after write failure")
	}
	if got := dialer.dials.Load(); got != 2 {
		t.Fatalf("POST dial count = %d, want 2", got)
	}
}

var _ Dialer = (*task5Round2Dialer)(nil)
