// Package transport contains replaceable HTTP transport primitives.
package transport

import (
	"context"
	"net/http"
)

// HTTPTransport executes an HTTP request while preserving its context.
type HTTPTransport interface {
	Do(context.Context, *http.Request) (*http.Response, error)
}

// Middleware wraps an HTTP transport.
type Middleware func(HTTPTransport) HTTPTransport
type defaultHTTPTransport struct{ client *http.Client }

// NewDefaultHTTPTransport adapts net/http to HTTPTransport.
func NewDefaultHTTPTransport(client *http.Client) HTTPTransport {
	if client == nil {
		client = &http.Client{}
	}
	return defaultHTTPTransport{client}
}
func (t defaultHTTPTransport) Do(ctx context.Context, req *http.Request) (*http.Response, error) {
	return t.client.Do(req.WithContext(ctx))
}
