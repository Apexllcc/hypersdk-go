// Package info implements Hyperliquid's unsigned Info HTTP API.
package info

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/Apexllcc/hyperliquid-go-sdk/internal/hlerr"
	"github.com/Apexllcc/hyperliquid-go-sdk/transport"
)

// Client calls an Info endpoint.
type Client struct {
	baseURL   string
	transport transport.HTTPTransport
	timeout   time.Duration
	userAgent string
}

// NewClient creates an Info client. It is normally constructed by hyperliquid.NewClient.
func NewClient(baseURL string, t transport.HTTPTransport, timeout time.Duration, userAgent string) *Client {
	return &Client{baseURL, t, timeout, userAgent}
}
func (c *Client) call(ctx context.Context, request any, target any) error {
	ctx, cancel := context.WithTimeout(ctx, c.timeout)
	defer cancel()
	body, err := json.Marshal(request)
	if err != nil {
		return err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL, bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("User-Agent", c.userAgent)
	resp, err := c.transport.Do(ctx, req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	raw, err := io.ReadAll(resp.Body)
	if err != nil {
		return err
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		apiErr := &hlerr.APIError{StatusCode: resp.StatusCode, Body: raw, Message: string(raw)}
		var structured struct {
			Code    string `json:"code"`
			Message string `json:"message"`
		}
		if json.Unmarshal(raw, &structured) == nil {
			apiErr.Code, apiErr.Message = structured.Code, structured.Message
		}
		return apiErr
	}
	if len(raw) == 0 {
		return fmt.Errorf("%w: empty response", hlerr.ErrUnexpectedResponse)
	}
	if err := json.Unmarshal(raw, target); err != nil {
		return fmt.Errorf("%w: %w", hlerr.ErrUnexpectedResponse, err)
	}
	return nil
}

// Raw performs an explicit advanced Info request.
func (c *Client) Raw(ctx context.Context, request any, response any) error {
	return c.call(ctx, request, response)
}
