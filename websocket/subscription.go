package websocket

import (
	"context"
	"encoding/json"
	"errors"
	"github.com/gorilla/websocket"
	"sync"
	"time"
)

// Subscription is the common lifecycle surface for all future stream types.
type Subscription interface {
	Errors() <-chan error
	Close() error
}

// L2BookSubscription delivers L2 book events and reconnect errors.
type L2BookSubscription struct {
	events  chan L2BookEvent
	errors  chan error
	done    chan struct{}
	once    sync.Once
	client  *Client
	key     string
	request L2BookRequest
	connMu  sync.Mutex
	conn    *websocket.Conn
}

func (c *Client) SubscribeL2Book(ctx context.Context, request L2BookRequest) (*L2BookSubscription, error) {
	if request.Coin == "" {
		return nil, errors.New("coin is required")
	}
	c.mu.Lock()
	if c.closed {
		c.mu.Unlock()
		return nil, ErrWebSocketClosed
	}
	key := l2BookKey(request)
	if existing := c.subs[key]; existing != nil {
		c.mu.Unlock()
		return existing, nil
	}
	s := &L2BookSubscription{events: make(chan L2BookEvent, c.config.EventBuffer), errors: make(chan error, 1), done: make(chan struct{}), client: c, key: key, request: request}
	c.subs[key] = s
	c.mu.Unlock()
	go s.run(ctx)
	go func() {
		select {
		case <-ctx.Done():
			_ = s.Close()
		case <-s.done:
		}
	}()
	return s, nil
}
func (s *L2BookSubscription) Events() <-chan L2BookEvent { return s.events }
func (s *L2BookSubscription) Errors() <-chan error       { return s.errors }
func (s *L2BookSubscription) Close() error {
	s.once.Do(func() { close(s.done); s.closeConn(); s.client.remove(s.key, s) })
	return nil
}
func (s *L2BookSubscription) run(ctx context.Context) {
	defer close(s.events)
	defer close(s.errors)
	for {
		select {
		case <-ctx.Done():
			return
		case <-s.done:
			return
		default:
		}
		connection, err := dial(ctx, s.client.url)
		if err != nil {
			s.report(err)
			if !waitForReconnect(ctx, s.done, s.client.config.ReconnectDelay) {
				return
			}
			continue
		}
		s.setConn(connection)
		if err := connection.SetReadDeadline(time.Now().Add(s.client.config.PongWait)); err != nil {
			_ = connection.Close()
			return
		}
		connection.SetPongHandler(func(string) error { return connection.SetReadDeadline(time.Now().Add(s.client.config.PongWait)) })
		if err := connection.WriteJSON(newL2SubscriptionWire(s.request)); err != nil {
			s.clearConn(connection)
			_ = connection.Close()
			s.report(err)
			if !waitForReconnect(ctx, s.done, s.client.config.ReconnectDelay) {
				return
			}
			continue
		}
		stopHeartbeat := startHeartbeat(connection, s.client.config)
		for {
			_, data, err := connection.ReadMessage()
			if err != nil {
				stopHeartbeat()
				s.clearConn(connection)
				_ = connection.Close()
				s.report(err)
				break
			}
			_ = connection.SetReadDeadline(time.Now().Add(s.client.config.PongWait))
			var envelope struct {
				Channel string      `json:"channel"`
				Data    L2BookEvent `json:"data"`
			}
			if err := json.Unmarshal(data, &envelope); err != nil {
				s.report(err)
				continue
			}
			if envelope.Channel != "l2Book" {
				continue
			}
			select {
			case s.events <- envelope.Data:
			case <-ctx.Done():
				stopHeartbeat()
				s.clearConn(connection)
				_ = connection.Close()
				return
			case <-s.done:
				stopHeartbeat()
				s.clearConn(connection)
				_ = connection.Close()
				return
			}
		}
		if !waitForReconnect(ctx, s.done, s.client.config.ReconnectDelay) {
			return
		}
	}
}
func (s *L2BookSubscription) setConn(connection *websocket.Conn) {
	s.connMu.Lock()
	s.conn = connection
	s.connMu.Unlock()
}
func (s *L2BookSubscription) clearConn(connection *websocket.Conn) {
	s.connMu.Lock()
	if s.conn == connection {
		s.conn = nil
	}
	s.connMu.Unlock()
}
func (s *L2BookSubscription) closeConn() {
	s.connMu.Lock()
	connection := s.conn
	s.conn = nil
	s.connMu.Unlock()
	if connection != nil {
		_ = connection.Close()
	}
}
func (s *L2BookSubscription) report(err error) {
	select {
	case s.errors <- err:
	default:
	}
}
