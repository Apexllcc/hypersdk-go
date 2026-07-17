// Package explorer implements Hyperliquid's read-only explorer RPC API.
//
// Hyperliquid's current public API documentation does not describe the
// explorer HTTP request schemas. The request methods in this package are
// therefore compatibility implementations verified against the public
// https://rpc.hyperliquid.xyz/explorer endpoint and cross-checked with
// nktkas/hyperliquid. They do not sign, submit, or retry trading operations.
package explorer

import (
	"bytes"
	"context"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/Apexllcc/hyperliquid-go-sdk/internal/hlerr"
	"github.com/Apexllcc/hyperliquid-go-sdk/transport"
	"github.com/Apexllcc/hyperliquid-go-sdk/websocket"
)

// Transaction is a read-only explorer transaction. Explorer action payloads
// are an open protocol union and preserve their action-specific JSON exactly.
type Transaction = websocket.ExplorerTransaction

// Action is the open action union contained by a Transaction.
type Action = websocket.ExplorerAction

// Block is one read-only explorer block summary.
type Block = websocket.ExplorerBlock

// BlockDetailsResponse is returned by the explorer blockDetails request.
type BlockDetailsResponse struct {
	Type         string `json:"type"`
	BlockDetails struct {
		Block
		Transactions []Transaction `json:"txs"`
	} `json:"blockDetails"`
}

// TxDetailsResponse is returned by the explorer txDetails request.
type TxDetailsResponse struct {
	Type        string      `json:"type"`
	Transaction Transaction `json:"tx"`
}

// UserDetailsResponse is returned by the explorer userDetails request.
type UserDetailsResponse struct {
	Type         string        `json:"type"`
	Transactions []Transaction `json:"txs"`
}

// ExplorerBlockEvent is the batch emitted by an explorerBlock subscription.
type ExplorerBlockEvent = []Block

// ExplorerTxsEvent is the batch emitted by an explorerTxs subscription.
type ExplorerTxsEvent = []Transaction

// ExplorerBlockSubscription is a resilient explorer RPC block stream.
type ExplorerBlockSubscription = websocket.ExplorerBlockSubscription

// ExplorerTxsSubscription is a resilient explorer RPC transaction stream.
type ExplorerTxsSubscription = websocket.ExplorerTxsSubscription

// Client calls Hyperliquid's read-only explorer RPC endpoint. The optional
// WebSocket client must point to the corresponding RPC WebSocket URL and is
// used only for explorerBlock and explorerTxs subscriptions.
type Client struct {
	baseURL            string
	transport          transport.HTTPTransport
	request            transport.RequestTransport
	timeout            time.Duration
	userAgent          string
	subscriptions      *websocket.Client
	subscriptionURL    string
	subscriptionConfig websocket.Config
	subscriptionMu     sync.Mutex
	subscriptionOwned  bool
}

// NewClient creates an Explorer client. The optional subscription client is
// intentionally separate from request transport: official API WebSocket post
// requests do not support the Explorer request kind.
func NewClient(baseURL string, t transport.HTTPTransport, timeout time.Duration, userAgent string, subscriptions ...*websocket.Client) *Client {
	if t == nil {
		t = transport.NewDefaultHTTPTransport(nil)
	}
	var stream *websocket.Client
	if len(subscriptions) > 0 {
		stream = subscriptions[0]
	}
	return &Client{baseURL: baseURL, transport: t, timeout: timeout, userAgent: userAgent, subscriptions: stream}
}

// NewClientWithWebSocket creates an Explorer client with a lazily constructed
// RPC subscription connection. The connection is not opened until ExplorerBlock
// or ExplorerTxs is called, so merely constructing a root Client does not add a
// background connection or goroutine for Explorer.
func NewClientWithWebSocket(baseURL string, t transport.HTTPTransport, timeout time.Duration, userAgent, websocketURL string, config websocket.Config) *Client {
	client := NewClient(baseURL, t, timeout, userAgent)
	client.subscriptionURL, client.subscriptionConfig = websocketURL, config
	return client
}

// SetRequestTransport selects a replacement read-only Explorer request path.
// It is intended for construction-time injection; callers must not mutate a
// client while it is in use.
func (c *Client) SetRequestTransport(request transport.RequestTransport) { c.request = request }

// BlockDetails returns the block at height. Explorer RPC does not expose a
// height-zero block, so zero is rejected before any request is sent.
func (c *Client) BlockDetails(ctx context.Context, height uint64) (BlockDetailsResponse, error) {
	if height == 0 {
		return BlockDetailsResponse{}, fmt.Errorf("invalid height")
	}
	var response BlockDetailsResponse
	err := c.call(ctx, map[string]any{"type": "blockDetails", "height": height}, &response)
	return response, err
}

// TxDetails returns one transaction selected by an exact 32-byte hash.
func (c *Client) TxDetails(ctx context.Context, hash string) (TxDetailsResponse, error) {
	if err := validateHex(hash, 32, "hash"); err != nil {
		return TxDetailsResponse{}, err
	}
	var response TxDetailsResponse
	err := c.call(ctx, map[string]any{"type": "txDetails", "hash": hash}, &response)
	return response, err
}

// UserDetails returns the explorer transactions associated with an Ethereum
// account address.
func (c *Client) UserDetails(ctx context.Context, user string) (UserDetailsResponse, error) {
	if err := validateHex(user, 20, "user"); err != nil {
		return UserDetailsResponse{}, err
	}
	var response UserDetailsResponse
	err := c.call(ctx, map[string]any{"type": "userDetails", "user": user}, &response)
	return response, err
}

// ExplorerBlock subscribes to raw-array block batches from the RPC WebSocket.
func (c *Client) ExplorerBlock(ctx context.Context) (*ExplorerBlockSubscription, error) {
	subscriptions, err := c.subscriptionClient()
	if err != nil {
		return nil, err
	}
	if subscriptions == nil {
		return nil, fmt.Errorf("explorer websocket subscriptions are not configured")
	}
	return subscriptions.SubscribeExplorerBlock(ctx)
}

// ExplorerTxs subscribes to raw-array transaction batches from the RPC
// WebSocket.
func (c *Client) ExplorerTxs(ctx context.Context) (*ExplorerTxsSubscription, error) {
	subscriptions, err := c.subscriptionClient()
	if err != nil {
		return nil, err
	}
	if subscriptions == nil {
		return nil, fmt.Errorf("explorer websocket subscriptions are not configured")
	}
	return subscriptions.SubscribeExplorerTxs(ctx)
}

// Close releases the lazily created explorer RPC subscription client. It is
// idempotent and does not affect the independent Info, Exchange, or market
// WebSocket clients.
func (c *Client) Close() error {
	c.subscriptionMu.Lock()
	subscriptions := c.subscriptions
	owned := c.subscriptionOwned
	c.subscriptions = nil
	c.subscriptionOwned = false
	c.subscriptionMu.Unlock()
	if subscriptions == nil || !owned {
		return nil
	}
	return subscriptions.Close()
}

func (c *Client) subscriptionClient() (*websocket.Client, error) {
	c.subscriptionMu.Lock()
	defer c.subscriptionMu.Unlock()
	if c.subscriptions != nil {
		return c.subscriptions, nil
	}
	if c.subscriptionURL == "" {
		return nil, nil
	}
	c.subscriptions = websocket.NewClient(c.subscriptionURL, c.subscriptionConfig)
	c.subscriptionOwned = true
	return c.subscriptions, nil
}

func (c *Client) call(ctx context.Context, request any, target any) error {
	if c.timeout > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, c.timeout)
		defer cancel()
	}
	if c.request != nil {
		return c.request.Request(ctx, transport.RequestExplorer, request, target)
	}
	body, err := json.Marshal(request)
	if err != nil {
		return err
	}
	requestContext := transport.ContextWithRequestMetadata(ctx, transport.RequestExplorer, request)
	req, err := http.NewRequestWithContext(requestContext, http.MethodPost, c.baseURL, bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("User-Agent", c.userAgent)
	response, err := c.transport.Do(requestContext, req)
	if err != nil {
		if response != nil && response.Body != nil {
			_ = response.Body.Close()
		}
		return err
	}
	if response == nil || response.Body == nil {
		return fmt.Errorf("%w: nil HTTP response", hlerr.ErrUnexpectedResponse)
	}
	defer func() { _ = response.Body.Close() }()
	raw, err := io.ReadAll(response.Body)
	if err != nil {
		return err
	}
	if response.StatusCode < http.StatusOK || response.StatusCode >= http.StatusMultipleChoices {
		return apiError(response.StatusCode, raw)
	}
	if len(raw) == 0 {
		return fmt.Errorf("%w: empty response", hlerr.ErrUnexpectedResponse)
	}
	var protocolError struct {
		Type    string `json:"type"`
		Message string `json:"message"`
	}
	if err := json.Unmarshal(raw, &protocolError); err != nil {
		return fmt.Errorf("%w: %w", hlerr.ErrUnexpectedResponse, err)
	}
	if protocolError.Type == "error" {
		return &hlerr.APIError{StatusCode: response.StatusCode, Message: protocolError.Message, Body: raw}
	}
	if err := json.Unmarshal(raw, target); err != nil {
		return fmt.Errorf("%w: %w", hlerr.ErrUnexpectedResponse, err)
	}
	return nil
}

func apiError(status int, body []byte) *hlerr.APIError {
	apiErr := &hlerr.APIError{StatusCode: status, Body: body, Message: string(body)}
	var structured struct {
		Code    string `json:"code"`
		Message string `json:"message"`
	}
	if json.Unmarshal(body, &structured) == nil {
		apiErr.Code, apiErr.Message = structured.Code, structured.Message
	}
	return apiErr
}

func validateHex(value string, bytesLen int, field string) error {
	if len(value) != 2+bytesLen*2 || !strings.HasPrefix(value, "0x") {
		return fmt.Errorf("invalid %s", field)
	}
	if _, err := hex.DecodeString(value[2:]); err != nil {
		return fmt.Errorf("invalid %s: %w", field, err)
	}
	return nil
}
