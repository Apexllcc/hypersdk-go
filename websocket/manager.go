package websocket

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
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

type terminalSubscription interface{ terminate(error) }

type connectionManager struct {
	client       *Client
	wake         chan struct{}
	done         chan struct{}
	stopped      chan struct{}
	closeOnce    sync.Once
	write        sync.Mutex
	admissionMu  sync.Mutex
	admissionSeq uint64
	admissions   map[string]wireAdmission
	ctx          context.Context
	cancel       context.CancelFunc
}

type wireAdmission struct {
	token       uint64
	wantPresent bool
	fingerprint string
	cancel      context.CancelCauseFunc
}

var errStaleWireAdmission = errors.New("stale websocket wire admission")

type pendingSubscriptionAck struct {
	method   string
	identity string
	wire     subscriptionWire
	deadline time.Time
}

type activeWireSubscription struct {
	wire    subscriptionWire
	members map[string]managedSubscription
	acked   bool
}

func newConnectionManager(client *Client) *connectionManager {
	ctx, cancel := context.WithCancel(context.Background())
	manager := &connectionManager{client: client, wake: make(chan struct{}, 1), done: make(chan struct{}), stopped: make(chan struct{}), admissions: make(map[string]wireAdmission), ctx: ctx, cancel: cancel}
	go func() {
		defer close(manager.stopped)
		manager.run()
	}()
	return manager
}

func (m *connectionManager) notify() {
	select {
	case m.wake <- struct{}{}:
	default:
	}
}

// close stops the manager and waits for it to exit. A Dialer must honor its
// context; this makes Client.Close deterministic with respect to in-flight
// and future dials without permitting a post-close connection attempt.
func (m *connectionManager) close() {
	m.closeOnce.Do(func() {
		close(m.done)
		m.cancel()
	})
	<-m.stopped
}

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
			// A close can race with the initial snapshot. Recheck after draining
			// so its wake is not consumed as a stale registration notification.
			if len(m.snapshot()) > 0 {
				return true
			}
			continue
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
	ctx, cancel := context.WithCancel(m.ctx)
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
	ctx, cancel := context.WithCancel(m.ctx)
	defer cancel()
	if err := connection.SetReadDeadline(time.Now().Add(m.client.config.PongWait)); err != nil {
		m.reportAll(err)
		return
	}
	subscribed := make(map[string]*activeWireSubscription)
	pending := make(map[string]pendingSubscriptionAck)
	if !m.syncSubscriptions(ctx, connection, subscribed, pending) {
		return
	}
	read := make(chan readResult, 1)
	readDone := make(chan struct{})
	defer close(readDone)
	go readLoop(connection, read, readDone)
	stopHeartbeat, heartbeatErrors := startHeartbeat(ctx, func(writeCtx context.Context, message any) error {
		_, err := m.writeJSON(writeCtx, connection, message, "", false, nil)
		return err
	}, m.client.config)
	defer stopHeartbeat()
	ackTimer := time.NewTimer(time.Hour)
	if !ackTimer.Stop() {
		<-ackTimer.C
	}
	defer ackTimer.Stop()
	for {
		ackDeadline := resetSubscriptionAckTimer(ackTimer, pending)
		select {
		case <-m.done:
			return
		case <-m.wake:
			if !m.syncSubscriptions(ctx, connection, subscribed, pending) {
				return
			}
			if len(m.snapshot()) == 0 && len(pending) == 0 {
				return
			}
		case err := <-heartbeatErrors:
			if err != nil && ctx.Err() == nil {
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
			if err := m.dispatch(result.data, subscribed, pending); err != nil {
				return
			}
			if len(m.snapshot()) == 0 && len(pending) == 0 {
				return
			}
		case <-ackDeadline:
			m.failExpiredAcknowledgement(pending)
			return
		}
	}
}

func (m *connectionManager) syncSubscriptions(ctx context.Context, connection *websocket.Conn, subscribed map[string]*activeWireSubscription, pending map[string]pendingSubscriptionAck) bool {
	current := make(map[string]*activeWireSubscription)
	for _, subscription := range m.snapshot() {
		identity := serverSubscriptionIdentity(subscription.subscriptionWire().Subscription)
		group := current[identity]
		if group == nil {
			group = &activeWireSubscription{wire: subscription.subscriptionWire(), members: make(map[string]managedSubscription)}
			current[identity] = group
		}
		group.members[subscription.subscriptionKey()] = subscription
	}
	for _, group := range current {
		if len(group.members) > 1 {
			group.wire = canonicalSubscriptionWire(group.wire)
		}
	}
	for identity, active := range subscribed {
		group := current[identity]
		if group != nil {
			for key, subscription := range group.members {
				if existing, exists := active.members[key]; exists && existing == subscription {
					continue
				}
				m.stateSubscription(subscription, SubscriptionStateConnected, nil)
				if active.acked {
					m.stateSubscription(subscription, SubscriptionStateSubscribed, nil)
				}
			}
			active.members = group.members
			continue
		}
		wire := active.wire
		wire.Method = "unsubscribe"
		wrote, err := m.writeJSON(ctx, connection, wire, identity, false, func() bool { return !m.hasSubscriptionIdentity(identity) })
		if err != nil {
			if ctx.Err() == nil {
				m.reportAll(err)
			}
			return false
		}
		if !wrote {
			continue
		}
		delete(pending, pendingAckKey("subscribe", identity))
		pending[pendingAckKey("unsubscribe", identity)] = pendingSubscriptionAck{method: "unsubscribe", identity: identity, wire: wire, deadline: time.Now().Add(m.client.config.SubscriptionAckTimeout)}
		delete(subscribed, identity)
	}
	for identity, group := range current {
		if _, ok := subscribed[identity]; ok {
			continue
		}
		for _, subscription := range group.members {
			m.stateSubscription(subscription, SubscriptionStateConnected, nil)
		}
		wrote, err := m.writeJSON(ctx, connection, group.wire, identity, true, func() bool { return m.hasSubscriptionIdentity(identity) })
		if err != nil {
			if ctx.Err() == nil {
				m.reportAll(err)
			}
			return false
		}
		if !wrote {
			continue
		}
		subscribed[identity] = group
		pending[pendingAckKey("subscribe", identity)] = pendingSubscriptionAck{method: "subscribe", identity: identity, wire: group.wire, deadline: time.Now().Add(m.client.config.SubscriptionAckTimeout)}
	}
	return true
}

func (m *connectionManager) writeJSON(ctx context.Context, connection *websocket.Conn, message any, identity string, wantPresent bool, current func() bool) (bool, error) {
	waitCtx := ctx
	if current != nil {
		var cancel context.CancelCauseFunc
		waitCtx, cancel = context.WithCancelCause(ctx)
		wire, _ := message.(subscriptionWire)
		fingerprint := subscriptionWireFingerprint(wire)
		unregister := m.registerWireAdmission(identity, wantPresent, fingerprint, current, cancel)
		defer func() {
			unregister()
			cancel(nil)
		}()
	}
	if err := m.client.messages.Wait(waitCtx); err != nil {
		if errors.Is(context.Cause(waitCtx), errStaleWireAdmission) {
			return false, nil
		}
		return false, err
	}
	if current != nil && !current() {
		return false, nil
	}
	m.write.Lock()
	defer m.write.Unlock()
	if err := ctx.Err(); err != nil {
		return false, err
	}
	if current != nil && !current() {
		return false, nil
	}
	return true, connection.WriteJSON(message)
}

func (m *connectionManager) registerWireAdmission(identity string, wantPresent bool, fingerprint string, current func() bool, cancel context.CancelCauseFunc) func() {
	m.admissionMu.Lock()
	m.admissionSeq++
	token := m.admissionSeq
	m.admissions[identity] = wireAdmission{token: token, wantPresent: wantPresent, fingerprint: fingerprint, cancel: cancel}
	m.admissionMu.Unlock()
	if !current() {
		cancel(errStaleWireAdmission)
	} else if wantPresent {
		_, currentFingerprint := m.client.subscriptionIdentityState(identity)
		if currentFingerprint != fingerprint {
			cancel(errStaleWireAdmission)
		}
	}
	return func() {
		m.admissionMu.Lock()
		if admission, ok := m.admissions[identity]; ok && admission.token == token {
			delete(m.admissions, identity)
		}
		m.admissionMu.Unlock()
	}
}

func (m *connectionManager) registryChanged(identity string, present bool, fingerprint string) {
	m.admissionMu.Lock()
	admission, ok := m.admissions[identity]
	m.admissionMu.Unlock()
	if ok && (admission.wantPresent != present || (present && admission.wantPresent && admission.fingerprint != fingerprint)) {
		admission.cancel(errStaleWireAdmission)
	}
	m.notify()
}

func (m *connectionManager) hasSubscriptionIdentity(identity string) bool {
	for _, subscription := range m.snapshot() {
		if serverSubscriptionIdentity(subscription.subscriptionWire().Subscription) == identity {
			return true
		}
	}
	return false
}

func (m *connectionManager) dispatch(data []byte, subscribed map[string]*activeWireSubscription, pending map[string]pendingSubscriptionAck) error {
	var envelope struct {
		Channel string          `json:"channel"`
		Data    json.RawMessage `json:"data"`
	}
	if err := json.Unmarshal(data, &envelope); err == nil && envelope.Channel != "" {
		if envelope.Channel == "subscriptionResponse" {
			return m.acknowledgeSubscription(envelope.Data, subscribed, pending)
		}
		if envelope.Channel == "error" && len(pending) > 0 {
			m.rejectPendingSubscriptions(envelope.Data, subscribed, pending)
			return nil
		}
		m.dispatchChannel(envelope.Channel, envelope.Data)
		return nil
	}
	// The explorer RPC (unlike api.hyperliquid.xyz/ws) sends its two live
	// streams as raw arrays. Classify only the documented structural shapes so
	// unrelated malformed frames remain visible as subscription errors.
	if channel, ok := explorerRawChannel(data); ok {
		m.dispatchChannel(channel, json.RawMessage(data))
		return nil
	}
	m.reportAll(errors.New("unexpected websocket message"))
	return nil
}

func (m *connectionManager) rejectPendingSubscriptions(data json.RawMessage, subscribed map[string]*activeWireSubscription, pending map[string]pendingSubscriptionAck) {
	message := string(data)
	var response struct {
		Error        json.RawMessage `json:"error"`
		Message      string          `json:"message"`
		Method       string          `json:"method"`
		Subscription map[string]any  `json:"subscription"`
		Request      struct {
			Method       string         `json:"method"`
			Subscription map[string]any `json:"subscription"`
		} `json:"request"`
	}
	if err := json.Unmarshal(data, &response); err == nil {
		if response.Message != "" {
			message = response.Message
		} else if len(response.Error) > 0 {
			if err := json.Unmarshal(response.Error, &message); err != nil {
				message = string(response.Error)
			}
		}
		method, subscription := response.Request.Method, response.Request.Subscription
		if method == "" {
			method, subscription = response.Method, response.Subscription
		}
		if method != "" && subscription != nil {
			if key, request, ok := matchingPendingAcknowledgement(method, subscription, pending); ok {
				m.rejectPendingAcknowledgement(key, request, subscribed, pending, fmt.Errorf("%w: %s", ErrSubscriptionRejected, message))
			}
			return
		}
	} else {
		if json.Unmarshal(data, &message) == nil {
			if method, subscription, ok := embeddedSubscriptionRequest(message); ok {
				if key, request, matched := matchingErrorAcknowledgement(method, subscription, pending); matched {
					m.rejectPendingAcknowledgement(key, request, subscribed, pending, fmt.Errorf("%w: %s", ErrSubscriptionRejected, message))
				}
				return
			}
		}
	}
	err := fmt.Errorf("%w: %s", ErrSubscriptionRejected, message)
	identities := make(map[string]struct{})
	for _, request := range pending {
		if request.method == "subscribe" {
			identities[request.identity] = struct{}{}
		}
	}
	for identity := range identities {
		m.rejectWireSubscription(identity, subscribed, pending, err)
	}
}

func (m *connectionManager) rejectPendingAcknowledgement(key string, request pendingSubscriptionAck, subscribed map[string]*activeWireSubscription, pending map[string]pendingSubscriptionAck, err error) {
	delete(pending, key)
	if request.method == "subscribe" {
		m.rejectWireSubscription(request.identity, subscribed, pending, err)
	}
}

func embeddedSubscriptionRequest(message string) (string, map[string]any, bool) {
	for offset := 0; offset < len(message); {
		start := strings.IndexByte(message[offset:], '{')
		if start < 0 {
			break
		}
		start += offset
		var object map[string]any
		if err := json.NewDecoder(strings.NewReader(message[start:])).Decode(&object); err == nil {
			method, _ := object["method"].(string)
			if subscription, ok := object["subscription"].(map[string]any); method != "" && ok {
				return method, subscription, true
			}
			if _, subscriptionOnly := object["type"].(string); subscriptionOnly {
				return "", object, true
			}
		}
		offset = start + 1
	}
	return "", nil, false
}

func matchingErrorAcknowledgement(method string, subscription map[string]any, pending map[string]pendingSubscriptionAck) (string, pendingSubscriptionAck, bool) {
	if method != "" {
		return matchingPendingAcknowledgement(method, subscription, pending)
	}
	var matchedKey string
	var matched pendingSubscriptionAck
	found := false
	for key, request := range pending {
		if _, ok := subscriptionMatchScore(request.wire.Subscription, subscription); !ok {
			continue
		}
		if found {
			return "", pendingSubscriptionAck{}, false
		}
		matchedKey, matched, found = key, request, true
	}
	return matchedKey, matched, found
}

func (m *connectionManager) rejectWireSubscription(identity string, subscribed map[string]*activeWireSubscription, pending map[string]pendingSubscriptionAck, err error) {
	delete(pending, pendingAckKey("subscribe", identity))
	active := subscribed[identity]
	delete(subscribed, identity)
	members := m.client.detachSubscriptionIdentity(identity)
	if active != nil {
		detachedKeys := make(map[string]struct{}, len(members))
		for _, subscription := range members {
			detachedKeys[subscription.subscriptionKey()] = struct{}{}
		}
		for key, subscription := range active.members {
			if _, alreadyDetached := detachedKeys[key]; alreadyDetached {
				continue
			}
			members = append(members, subscription)
		}
	}
	for _, subscription := range members {
		if terminal, ok := subscription.(terminalSubscription); ok {
			terminal.terminate(err)
			continue
		}
		m.reportSubscription(subscription, err)
		_ = subscription.Close()
	}
}

func pendingAckKey(method, identity string) string {
	return method + ":" + identity
}

func (m *connectionManager) acknowledgeSubscription(data json.RawMessage, subscribed map[string]*activeWireSubscription, pending map[string]pendingSubscriptionAck) error {
	var response struct {
		Method       string          `json:"method"`
		Subscription map[string]any  `json:"subscription"`
		Error        json.RawMessage `json:"error"`
	}
	if err := json.Unmarshal(data, &response); err != nil || response.Method == "" || response.Subscription == nil {
		err := fmt.Errorf("%w: malformed subscriptionResponse", ErrSubscriptionRejected)
		m.reportAll(err)
		return err
	}
	key, request, ok := matchingPendingAcknowledgement(response.Method, response.Subscription, pending)
	if !ok {
		return nil
	}
	delete(pending, key)
	if len(response.Error) > 0 && string(response.Error) != "null" {
		var message string
		if err := json.Unmarshal(response.Error, &message); err != nil {
			message = string(response.Error)
		}
		err := fmt.Errorf("%w: %s", ErrSubscriptionRejected, message)
		m.rejectWireSubscription(request.identity, subscribed, pending, err)
		return nil
	}
	if request.method == "subscribe" {
		if active := subscribed[request.identity]; active != nil {
			active.acked = true
			for _, subscription := range active.members {
				m.stateSubscription(subscription, SubscriptionStateSubscribed, nil)
			}
		}
	}
	return nil
}

func matchingPendingAcknowledgement(method string, subscription map[string]any, pending map[string]pendingSubscriptionAck) (string, pendingSubscriptionAck, bool) {
	bestScore := -1
	var bestKey string
	var best pendingSubscriptionAck
	for key, request := range pending {
		if request.method != method {
			continue
		}
		score, matches := subscriptionMatchScore(request.wire.Subscription, subscription)
		if matches && score > bestScore {
			bestScore, bestKey, best = score, key, request
		}
	}
	return bestKey, best, bestScore >= 0
}

func resetSubscriptionAckTimer(timer *time.Timer, pending map[string]pendingSubscriptionAck) <-chan time.Time {
	if !timer.Stop() {
		select {
		case <-timer.C:
		default:
		}
	}
	var earliest time.Time
	for _, request := range pending {
		if earliest.IsZero() || request.deadline.Before(earliest) {
			earliest = request.deadline
		}
	}
	if earliest.IsZero() {
		return nil
	}
	delay := time.Until(earliest)
	if delay < 0 {
		delay = 0
	}
	timer.Reset(delay)
	return timer.C
}

func (m *connectionManager) failExpiredAcknowledgement(pending map[string]pendingSubscriptionAck) {
	now := time.Now()
	for key, request := range pending {
		if request.deadline.After(now) {
			continue
		}
		delete(pending, key)
		if request.method == "subscribe" {
			for _, subscription := range m.subscriptionsForIdentity(request.identity) {
				m.reportSubscription(subscription, ErrSubscriptionAckTimeout)
			}
		}
	}
}

func (m *connectionManager) subscriptionsForIdentity(identity string) []managedSubscription {
	var subscriptions []managedSubscription
	for _, subscription := range m.snapshot() {
		if serverSubscriptionIdentity(subscription.subscriptionWire().Subscription) == identity {
			subscriptions = append(subscriptions, subscription)
		}
	}
	return subscriptions
}

func (m *connectionManager) stateSubscription(subscription managedSubscription, state SubscriptionState, err error) {
	if subscription == nil || subscription.isDone() {
		return
	}
	if stateful, ok := subscription.(statefulSubscription); ok {
		stateful.stateChange(state, err)
	}
}

func (m *connectionManager) reportSubscription(subscription managedSubscription, err error) {
	if subscription == nil || subscription.isDone() {
		return
	}
	if reporter, ok := subscription.(interface{ report(error) }); ok {
		reporter.report(err)
	}
	if stateful, ok := subscription.(statefulSubscription); ok {
		stateful.stateChange(SubscriptionStateError, err)
	}
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
