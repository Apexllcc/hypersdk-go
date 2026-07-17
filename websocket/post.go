package websocket

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/Apexllcc/hyperliquid-go-sdk/transport"
	"github.com/gorilla/websocket"
)

// PostError is the protocol error returned in a WebSocket post response.
// Hyperliquid encodes the corresponding HTTP status and message as a string.
type PostError struct {
	Message string
	status  int
}

func (e *PostError) Error() string { return e.Message }

// StatusCode returns the HTTP-equivalent status embedded by Hyperliquid in a
// WebSocket post error, or zero when the server supplied no recognizable code.
func (e *PostError) StatusCode() int { return e.status }

func newPostError(message string) *PostError {
	fields := strings.Fields(message)
	status := 0
	if len(fields) > 0 {
		if value, err := strconv.Atoi(fields[0]); err == nil && value >= 100 && value <= 599 {
			status = value
		}
	}
	return &PostError{Message: message, status: status}
}

type postResponse struct {
	payload json.RawMessage
	err     error
}

type postPending struct {
	kind   transport.RequestKind
	result chan postResponse
}

// postManager owns one reusable WebSocket connection for request/response
// traffic. It is deliberately separate from the subscription connection: a
// slow stream consumer must never delay a signed action write. A request is
// never replayed after a disconnect, because its execution may be unknown.
type postManager struct {
	client  *Client
	mu      sync.Mutex
	write   chan struct{}
	done    chan struct{}
	conn    *websocket.Conn
	closed  bool
	nextID  atomic.Uint64
	pending map[uint64]postPending
	ctx     context.Context
	cancel  context.CancelFunc
}

func newPostManager(client *Client) *postManager {
	ctx, cancel := context.WithCancel(context.Background())
	manager := &postManager{client: client, write: make(chan struct{}, 1), done: make(chan struct{}), pending: make(map[uint64]postPending), ctx: ctx, cancel: cancel}
	manager.write <- struct{}{}
	return manager
}

func (m *postManager) close() {
	m.mu.Lock()
	if m.closed {
		m.mu.Unlock()
		return
	}
	m.closed = true
	close(m.done)
	m.cancel()
	connection := m.conn
	m.conn = nil
	pending := m.pending
	m.pending = make(map[uint64]postPending)
	m.mu.Unlock()
	if connection != nil {
		_ = connection.Close()
	}
	for _, request := range pending {
		request.result <- postResponse{err: ErrWebSocketClosed}
	}
}

func (m *postManager) request(ctx context.Context, kind transport.RequestKind, payload any, target any) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if kind != transport.RequestInfo && kind != transport.RequestAction {
		return fmt.Errorf("%w: %q", ErrUnsupportedPostRequest, kind)
	}
	requestCtx, cancel := context.WithCancel(ctx)
	stopManagerCancel := context.AfterFunc(m.ctx, cancel)
	defer func() {
		stopManagerCancel()
		cancel()
	}()
	release, err := m.client.postGate.Acquire(requestCtx)
	if err != nil {
		return m.contextError(ctx, err)
	}
	defer release()
	id := m.nextID.Add(1)
	result := make(chan postResponse, 1)
	m.mu.Lock()
	if m.closed {
		m.mu.Unlock()
		return ErrWebSocketClosed
	}
	m.pending[id] = postPending{kind: kind, result: result}
	m.mu.Unlock()
	connection, err := m.connection(requestCtx)
	if err != nil {
		m.remove(id)
		return m.contextError(ctx, err)
	}
	message := struct {
		Method  string `json:"method"`
		ID      uint64 `json:"id"`
		Request struct {
			Type    transport.RequestKind `json:"type"`
			Payload any                   `json:"payload"`
		} `json:"request"`
	}{Method: "post", ID: id}
	message.Request.Type, message.Request.Payload = kind, payload
	if err := m.lockWrite(requestCtx); err != nil {
		m.remove(id)
		return m.contextError(ctx, err)
	}
	if err := requestCtx.Err(); err != nil {
		m.unlockWrite()
		m.remove(id)
		return m.contextError(ctx, err)
	}
	if err := m.client.messages.Wait(requestCtx); err != nil {
		m.unlockWrite()
		m.remove(id)
		return m.contextError(ctx, err)
	}
	// gorilla/websocket has no context-aware WriteJSON. Once this request owns
	// the token, cancellation must close the connection to unblock its write.
	// The atomic gate makes a cancellation observed after WriteJSON returns a
	// no-op, avoiding a late cancellation disconnecting other pending requests.
	var writing atomic.Bool
	writing.Store(true)
	stopCancel := context.AfterFunc(requestCtx, func() {
		if writing.CompareAndSwap(true, false) {
			m.disconnect(connection, requestCtx.Err())
		}
	})
	if deadline, ok := requestCtx.Deadline(); ok {
		err = connection.SetWriteDeadline(deadline)
	}
	if err == nil {
		err = connection.WriteJSON(message)
	}
	// A deadline belongs to one serialized write only. Leaving it in place
	// would make a later independent request fail after the former context's
	// timeout.
	if clearErr := connection.SetWriteDeadline(time.Time{}); err == nil {
		err = clearErr
	}
	writing.CompareAndSwap(true, false)
	stopCancel()
	m.unlockWrite()
	if err != nil {
		if requestCtx.Err() != nil {
			return m.contextError(ctx, requestCtx.Err())
		}
		m.remove(id)
		m.disconnect(connection, err)
		return err
	}
	select {
	case outcome := <-result:
		if outcome.err != nil {
			return outcome.err
		}
		if target == nil {
			return nil
		}
		if err := json.Unmarshal(outcome.payload, target); err != nil {
			return fmt.Errorf("%w: post response: %w", ErrUnexpectedPostResponse, err)
		}
		return nil
	case <-requestCtx.Done():
		m.remove(id)
		return m.contextError(ctx, requestCtx.Err())
	}
}

func (m *postManager) contextError(caller context.Context, fallback error) error {
	if err := caller.Err(); err != nil {
		return err
	}
	m.mu.Lock()
	closed := m.closed
	m.mu.Unlock()
	if closed {
		return ErrWebSocketClosed
	}
	return fallback
}

func (m *postManager) connection(ctx context.Context) (*websocket.Conn, error) {
	m.mu.Lock()
	if m.closed {
		m.mu.Unlock()
		return nil, ErrWebSocketClosed
	}
	if m.conn != nil {
		connection := m.conn
		m.mu.Unlock()
		return connection, nil
	}
	m.mu.Unlock()
	// Serializing connection creation prevents concurrent request callers from
	// opening redundant sockets while preserving context cancellation.
	if err := m.lockWrite(ctx); err != nil {
		return nil, err
	}
	defer m.unlockWrite()
	m.mu.Lock()
	if m.closed {
		m.mu.Unlock()
		return nil, ErrWebSocketClosed
	}
	if m.conn != nil {
		connection := m.conn
		m.mu.Unlock()
		return connection, nil
	}
	m.mu.Unlock()
	connection, err := m.client.dial(ctx)
	if err != nil {
		return nil, err
	}
	m.mu.Lock()
	if m.closed {
		m.mu.Unlock()
		_ = connection.Close()
		return nil, ErrWebSocketClosed
	}
	m.conn = connection
	m.mu.Unlock()
	go m.read(connection)
	return connection, nil
}

func (m *postManager) lockWrite(ctx context.Context) error {
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-m.write:
		return nil
	}
}

func (m *postManager) unlockWrite() { m.write <- struct{}{} }

func (m *postManager) read(connection *websocket.Conn) {
	for {
		_, raw, err := connection.ReadMessage()
		if err != nil {
			m.disconnect(connection, err)
			return
		}
		var envelope struct {
			Channel string `json:"channel"`
			Data    struct {
				ID       uint64 `json:"id"`
				Response struct {
					Type    transport.RequestKind `json:"type"`
					Payload json.RawMessage       `json:"payload"`
				} `json:"response"`
			} `json:"data"`
		}
		if err := json.Unmarshal(raw, &envelope); err != nil || envelope.Channel != "post" {
			continue
		}
		m.mu.Lock()
		request, ok := m.pending[envelope.Data.ID]
		delete(m.pending, envelope.Data.ID)
		m.mu.Unlock()
		if !ok {
			continue
		}
		if envelope.Data.Response.Type == "error" {
			var message string
			if err := json.Unmarshal(envelope.Data.Response.Payload, &message); err != nil {
				request.result <- postResponse{err: fmt.Errorf("%w: invalid error payload", ErrUnexpectedPostResponse)}
			} else {
				request.result <- postResponse{err: newPostError(message)}
			}
			continue
		}
		if envelope.Data.Response.Type != request.kind {
			request.result <- postResponse{err: fmt.Errorf("%w: request %s received %s", ErrUnexpectedPostResponse, request.kind, envelope.Data.Response.Type)}
			continue
		}
		payload := envelope.Data.Response.Payload
		// The official WebSocket Info response adds an inner {type, data}
		// envelope that is absent from the HTTP response. Action payloads are
		// already the HTTP-equivalent response body.
		if envelope.Data.Response.Type == transport.RequestInfo {
			var infoEnvelope struct {
				Data json.RawMessage `json:"data"`
			}
			if err := json.Unmarshal(payload, &infoEnvelope); err != nil || len(infoEnvelope.Data) == 0 {
				request.result <- postResponse{err: fmt.Errorf("%w: invalid info payload", ErrUnexpectedPostResponse)}
				continue
			}
			payload = infoEnvelope.Data
		}
		request.result <- postResponse{payload: payload}
	}
}

func (m *postManager) remove(id uint64) {
	m.mu.Lock()
	delete(m.pending, id)
	m.mu.Unlock()
}

func (m *postManager) disconnect(connection *websocket.Conn, err error) {
	m.mu.Lock()
	if m.conn != connection {
		m.mu.Unlock()
		return
	}
	m.conn = nil
	pending := m.pending
	m.pending = make(map[uint64]postPending)
	m.mu.Unlock()
	_ = connection.Close()
	for _, request := range pending {
		select {
		case request.result <- postResponse{err: err}:
		default:
		}
	}
}

// Request implements transport.RequestTransport using the official WebSocket
// post envelope. It shares one request connection across concurrent callers.
func (c *Client) Request(ctx context.Context, kind transport.RequestKind, payload any, response any) error {
	if c == nil {
		return errors.New("nil websocket client")
	}
	return c.posts.request(ctx, kind, payload, response)
}

// PostInfo performs a strongly typed Hyperliquid Info request over WebSocket.
func (c *Client) PostInfo(ctx context.Context, payload any, response any) error {
	return c.Request(ctx, transport.RequestInfo, payload, response)
}

// PostAction performs a strongly typed signed Exchange action over WebSocket.
// It never retries an action after a network failure.
func (c *Client) PostAction(ctx context.Context, payload any, response any) error {
	return c.Request(ctx, transport.RequestAction, payload, response)
}
