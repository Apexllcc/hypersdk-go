// Package info implements Hyperliquid's unsigned Info HTTP API.
package info

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
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
	request   transport.RequestTransport
	timeout   time.Duration
	userAgent string
	retry     transport.RetryPolicy
}

// NewClient creates an Info client. It is normally constructed by hyperliquid.NewClient.
func NewClient(baseURL string, t transport.HTTPTransport, timeout time.Duration, userAgent string, policies ...transport.RetryPolicy) *Client {
	policy := transport.DefaultRetryPolicy()
	if len(policies) > 0 {
		policy = policies[0]
	}
	return &Client{baseURL: baseURL, transport: t, timeout: timeout, userAgent: userAgent, retry: policy}
}

// SetRequestTransport selects a non-HTTP API request transport, such as the
// WebSocket post transport. It is intended for construction-time injection;
// callers must not mutate a client while it is in use.
func (c *Client) SetRequestTransport(request transport.RequestTransport) {
	c.request = request
}
func (c *Client) call(ctx context.Context, request any, target any) error {
	ctx, cancel := context.WithTimeout(ctx, c.timeout)
	defer cancel()
	if c.request != nil {
		policy := c.retry
		if policy.MaxAttempts <= 0 {
			policy = transport.DefaultRetryPolicy()
		}
		for attempt := 0; attempt < policy.MaxAttempts; attempt++ {
			err := c.request.Request(ctx, transport.RequestInfo, request, target)
			if err == nil || attempt == policy.MaxAttempts-1 || !retryableRequestError(err) {
				return err
			}
			timer := time.NewTimer(policy.Delay(attempt))
			select {
			case <-ctx.Done():
				timer.Stop()
				return ctx.Err()
			case <-timer.C:
			}
		}
		return nil
	}
	body, err := json.Marshal(request)
	if err != nil {
		return err
	}
	var resp *http.Response
	requestID := fmt.Sprintf("hl-%d", time.Now().UnixNano())
	policy := c.retry
	if policy.MaxAttempts <= 0 {
		policy = transport.DefaultRetryPolicy()
	}
	for attempt := 0; attempt < policy.MaxAttempts; attempt++ {
		req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL, bytes.NewReader(body))
		if err != nil {
			return err
		}
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("User-Agent", c.userAgent)
		req.Header.Set("X-Request-ID", requestID)
		resp, err = c.transport.Do(ctx, req)
		if err != nil {
			if resp != nil && resp.Body != nil {
				_ = resp.Body.Close()
			}
			return err
		}
		if resp == nil || resp.Body == nil {
			return fmt.Errorf("%w: nil HTTP response", hlerr.ErrUnexpectedResponse)
		}
		retryable := err == nil && (resp.StatusCode == 429 || resp.StatusCode == 502 || resp.StatusCode == 503 || resp.StatusCode == 504)
		if !retryable || attempt == policy.MaxAttempts-1 {
			break
		}
		delay := policy.Delay(attempt)
		if retryAfter, ok := policy.RetryAfterDelay(resp.Header, time.Now()); ok {
			delay = retryAfter
		}
		resp.Body.Close()
		timer := time.NewTimer(delay)
		select {
		case <-ctx.Done():
			timer.Stop()
			return ctx.Err()
		case <-timer.C:
		}
	}
	defer resp.Body.Close()
	raw, err := io.ReadAll(resp.Body)
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

type statusCodedError interface{ StatusCode() int }

func retryableRequestError(err error) bool {
	var statusError statusCodedError
	if !errors.As(err, &statusError) {
		return false
	}
	switch statusError.StatusCode() {
	case http.StatusTooManyRequests, http.StatusBadGateway, http.StatusServiceUnavailable, http.StatusGatewayTimeout:
		return true
	default:
		return false
	}
}

// Raw performs an explicit advanced Info request.
func (c *Client) Raw(ctx context.Context, request any, response any) error {
	return c.call(ctx, request, response)
}
