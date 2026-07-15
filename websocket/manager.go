package websocket

import (
	"context"
	"encoding/json"
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
	for {
		if !m.waitForSubscriptions() {
			return
		}
		m.stateAll(SubscriptionStateConnecting, nil)
		connection, err := m.dial()
		if err != nil {
			m.reportAll(err)
			m.stateAll(SubscriptionStateReconnecting, nil)
			if !m.waitReconnect() {
				return
			}
			continue
		}
		m.serve(connection)
		_ = connection.Close()
		m.stateAll(SubscriptionStateReconnecting, nil)
		if !m.waitReconnect() {
			return
		}
	}
}

func (m *connectionManager) waitForSubscriptions() bool {
	for {
		if len(m.snapshot()) > 0 {
			return true
		}
		select {
		case <-m.done:
			return false
		case <-m.wake:
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
	if err := json.Unmarshal(data, &envelope); err != nil {
		m.reportAll(err)
		return
	}
	for _, subscription := range m.snapshot() {
		if subscription.subscriptionChannel() == envelope.Channel {
			subscription.deliverRaw(envelope.Data)
		}
	}
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

func (m *connectionManager) waitReconnect() bool {
	if len(m.snapshot()) == 0 {
		return m.waitForSubscriptions()
	}
	timer := time.NewTimer(m.client.config.ReconnectDelay)
	defer timer.Stop()
	select {
	case <-m.done:
		return false
	case <-m.wake:
		return true
	case <-timer.C:
		return true
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
