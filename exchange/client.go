// Package exchange implements signed Hyperliquid Exchange actions.
package exchange

import (
	"time"

	"github.com/Apexllcc/hyperliquid-go-sdk/asset"
	"github.com/Apexllcc/hyperliquid-go-sdk/nonce"
	"github.com/Apexllcc/hyperliquid-go-sdk/signer"
	"github.com/Apexllcc/hyperliquid-go-sdk/transport"
)

// Client owns protocol construction, not private key material.
type Client struct {
	baseURL   string
	network   string
	transport transport.HTTPTransport
	timeout   time.Duration
	signer    signer.DigestSigner
	nonce     nonce.Manager
	assets    asset.Resolver
	userAgent string
}

// NewClient creates an Exchange client. It is normally called by the root client.
func NewClient(baseURL, network string, t transport.HTTPTransport, timeout time.Duration, s signer.DigestSigner, n nonce.Manager, a asset.Resolver, userAgent string) *Client {
	if t == nil {
		t = transport.NewDefaultHTTPTransport(nil)
	}
	if timeout <= 0 {
		timeout = 5 * time.Second
	}
	return &Client{baseURL: baseURL, network: network, transport: t, timeout: timeout, signer: s, nonce: n, assets: a, userAgent: userAgent}
}
