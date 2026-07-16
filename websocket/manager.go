package websocket

import (
	"context"
	"encoding/json"
	"errors"
	"sync"
	"time"

	"github.com/gorilla/websocket"
)

// managedSubscription is implemented by subscriptions registered with the
// client's shared WebSocket connection.
type managedSubscription interface {
	Subscription
	subscriptionKey() string
	subscriptionWire() subscriptionWire
	subscriptionChannel() string
	deliverRaw(json.RawMessage)
	isDone() bool
}

type connectionManager struct {
	client *Client
	wake   chan struct{}
	done   chan struct{}
	write  sync.Mutex
}

func newConnectionManager(client *Client) *connectionManager {
	manager := &connectionManager{client: client, wake: make(chan struct{}, 1), done: make(chan struct{})}
	go manager.run()
	return manager
}

func (m *connectionManager) notify() {
	select {
	case m.wake <- struct{}{}:
	default:
	}
}

func (m *connectionManager) close() { close(m.done) }

func (m *connectionManager) run() {
	reconnectAttempt := 0
	for {
		if !m.waitForSubscriptions() {
			return
		}
		m.stateAll(SubscriptionStateConnecting, nil)
		connection, err := m.dial()
		if err != nil {
			m.reportAll(err)
			m.stateAll(SubscriptionStateReconnecting, nil)
			waitResult := m.waitReconnect(reconnectAttempt)
			if waitResult == reconnectWaitStopped {
				return
			}
			if waitResult == reconnectWaitElapsed {
				reconnectAttempt++
			}
			continue
		}
		// A successful WebSocket dial ends a consecutive failure streak. A later
		// disconnect starts again from the configured initial delay instead of
		// inheriting a stale backoff from an unrelated earlier outage.
		reconnectAttempt = 0
		m.serve(connection)
		_ = connection.Close()
		m.stateAll(SubscriptionStateReconnecting, nil)
		waitResult := m.waitReconnect(reconnectAttempt)
		if waitResult == reconnectWaitStopped {
			return
		}
		if waitResult == reconnectWaitElapsed {
			reconnectAttempt++
		}
	}
}

func (m *connectionManager) waitForSubscriptions() bool {
	for {
		if m.isClosed() {
			return false
		}
		if len(m.snapshot()) > 0 {
			// A subscription can be registered before this goroutine observes
			// it, leaving its notification buffered. The snapshot already
			// contains the latest registration state, so retaining that old wake
			// would spuriously skip the first reconnect delay after a dial error.
			m.drainWake()
			return true
		}
		select {
		case <-m.done:
			return false
		case <-m.wake:
		}
	}
}

func (m *connectionManager) drainWake() {
	for {
		select {
		case <-m.wake:
		default:
			return
		}
	}
}

func (m *connectionManager) dial() (*websocket.Conn, error) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go func() {
		for {
			select {
			case <-m.done:
				cancel()
				return
			case <-ctx.Done():
				return
			case <-m.wake:
				if len(m.snapshot()) == 0 {
					cancel()
					return
				}
			}
		}
	}()
	return m.client.dial(ctx)
}

func (m *connectionManager) serve(connection *websocket.Conn) {
	if err := connection.SetReadDeadline(time.Now().Add(m.client.config.PongWait)); err != nil {
		m.reportAll(err)
		return
	}
	subscribed := make(map[string]subscriptionWire)
	if !m.syncSubscriptions(connection, subscribed) {
		return
	}
	read := make(chan readResult, 1)
	readDone := make(chan struct{})
	defer close(readDone)
	go readLoop(connection, read, readDone)
	stopHeartbeat, heartbeatErrors := startHeartbeat(func(message any) error { return m.writeJSON(connection, message) }, m.client.config)
	defer stopHeartbeat()
	for {
		select {
		case <-m.done:
			return
		case <-m.wake:
			if len(m.snapshot()) == 0 || !m.syncSubscriptions(connection, subscribed) {
				return
			}
		case err := <-heartbeatErrors:
			if err != nil {
				m.reportAll(err)
			}
			return
		case result := <-read:
			if result.err != nil {
				if len(m.snapshot()) > 0 {
					m.reportAll(result.err)
				}
				return
			}
			_ = connection.SetReadDeadline(time.Now().Add(m.client.config.PongWait))
			m.dispatch(result.data)
		}
	}
}

func (m *connectionManager) syncSubscriptions(connection *websocket.Conn, subscribed map[string]subscriptionWire) bool {
	current := make(map[string]managedSubscription)
	for _, subscription := range m.snapshot() {
		current[subscription.subscriptionKey()] = subscription
	}
	for key, wire := range subscribed {
		if _, ok := current[key]; ok {
			continue
		}
		wire.Method = "unsubscribe"
		if err := m.writeJSON(connection, wire); err != nil {
			m.reportAll(err)
			return false
		}
		delete(subscribed, key)
	}
	for key, subscription := range current {
		if _, ok := subscribed[key]; ok {
			continue
		}
		if stateful, ok := subscription.(statefulSubscription); ok {
			stateful.stateChange(SubscriptionStateConnected, nil)
		}
		if err := m.writeJSON(connection, subscription.subscriptionWire()); err != nil {
			m.reportAll(err)
			return false
		}
		subscribed[key] = subscription.subscriptionWire()
		if stateful, ok := subscription.(statefulSubscription); ok {
			stateful.stateChange(SubscriptionStateSubscribed, nil)
		}
	}
	return true
}

func (m *connectionManager) writeJSON(connection *websocket.Conn, message any) error {
	m.write.Lock()
	defer m.write.Unlock()
	return connection.WriteJSON(message)
}

func (m *connectionManager) dispatch(data []byte) {
	var envelope struct {
		Channel string          `json:"channel"`
		Data    json.RawMessage `json:"data"`
	}
	if err := json.Unmarshal(data, &envelope); err == nil && envelope.Channel != "" {
		m.dispatchChannel(envelope.Channel, envelope.Data)
		return
	}
	// The explorer RPC (unlike api.hyperliquid.xyz/ws) sends its two live
	// streams as raw arrays. Classify only the documented structural shapes so
	// unrelated malformed frames remain visible as subscription errors.
	if channel, ok := explorerRawChannel(data); ok {
		m.dispatchChannel(channel, json.RawMessage(data))
		return
	}
	m.reportAll(errors.New("unexpected websocket message"))
}

func (m *connectionManager) dispatchChannel(channel string, data json.RawMessage) {
	for _, subscription := range m.snapshot() {
		if subscription.subscriptionChannel() == channel {
			subscription.deliverRaw(data)
		}
	}
}

func explorerRawChannel(data []byte) (string, bool) {
	var entries []map[string]json.RawMessage
	if err := json.Unmarshal(data, &entries); err != nil || len(entries) == 0 {
		return "", false
	}
	first := entries[0]
	if first["blockTime"] != nil && first["height"] != nil && first["numTxs"] != nil && first["proposer"] != nil && first["hash"] != nil {
		return "explorerBlock_", true
	}
	if first["action"] != nil && first["block"] != nil && first["error"] != nil && first["hash"] != nil && first["time"] != nil && first["user"] != nil {
		return "explorerTxs_", true
	}
	return "", false
}

func (m *connectionManager) snapshot() []managedSubscription {
	m.client.mu.Lock()
	defer m.client.mu.Unlock()
	subscriptions := make([]managedSubscription, 0, len(m.client.subs))
	for _, subscription := range m.client.subs {
		subscriptions = append(subscriptions, subscription)
	}
	return subscriptions
}

func (m *connectionManager) reportAll(err error) {
	for _, subscription := range m.snapshot() {
		if reporter, ok := subscription.(interface{ report(error) }); ok {
			reporter.report(err)
		}
		if stateful, ok := subscription.(statefulSubscription); ok {
			stateful.stateChange(SubscriptionStateError, err)
		}
	}
}

func (m *connectionManager) stateAll(state SubscriptionState, err error) {
	for _, subscription := range m.snapshot() {
		if stateful, ok := subscription.(statefulSubscription); ok {
			stateful.stateChange(state, err)
		}
	}
}

type reconnectWaitResult uint8

const (
	reconnectWaitStopped reconnectWaitResult = iota
	reconnectWaitElapsed
	reconnectWaitWoken
)

func (m *connectionManager) waitReconnect(attempt int) reconnectWaitResult {
	if m.isClosed() {
		return reconnectWaitStopped
	}
	if len(m.snapshot()) == 0 {
		if m.waitForSubscriptions() {
			return reconnectWaitWoken
		}
		return reconnectWaitStopped
	}
	delay := m.client.config.ReconnectPolicy.Delay(attempt)
	timer := time.NewTimer(delay)
	defer timer.Stop()
	var result reconnectWaitResult
	select {
	case <-m.done:
		result = reconnectWaitStopped
	case <-m.wake:
		result = reconnectWaitWoken
	case <-timer.C:
		result = reconnectWaitElapsed
	}
	if m.isClosed() {
		return reconnectWaitStopped
	}
	return result
}

func (m *connectionManager) isClosed() bool {
	select {
	case <-m.done:
		return true
	default:
		return false
	}
}

type readResult struct {
	data []byte
	err  error
}

func readLoop(connection *websocket.Conn, results chan<- readResult, done <-chan struct{}) {
	for {
		_, data, err := connection.ReadMessage()
		select {
		case results <- readResult{data: data, err: err}:
		case <-done:
			return
		}
		if err != nil {
			return
		}
	}
}
