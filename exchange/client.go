// Package exchange implements signed Hyperliquid Exchange actions.
package exchange

import (
	"fmt"
	"time"

	"github.com/Apexllcc/hypersdk-go/asset"
	"github.com/Apexllcc/hypersdk-go/nonce"
	"github.com/Apexllcc/hypersdk-go/signer"
	"github.com/Apexllcc/hypersdk-go/transport"
	"github.com/ethereum/go-ethereum/common"
)

// Client owns protocol construction, not private key material.
type Client struct {
	baseURL   string
	network   string
	transport transport.HTTPTransport
	request   transport.RequestTransport
	timeout   time.Duration
	signer    signer.DigestSigner
	nonce     nonce.Manager
	assets    asset.Resolver
	userAgent string
	submit    submitConfig
}

// SetRequestTransport selects a non-HTTP API request transport for signed
// action submissions. It is intended for construction-time injection. The
// caller's transport must not retry actions after a network failure.
func (c *Client) SetRequestTransport(request transport.RequestTransport) {
	c.request = request
}

type submitConfig struct {
	vaultAddress *common.Address
	expiresAfter *uint64
}

// SubmitOption configures optional protocol fields included in every L1
// action submitted by this client. Construct a separate Client when different
// actions require different vaults or expiry windows.
type SubmitOption func(*submitConfig) error

// WithVaultAddress signs and submits L1 actions on behalf of this vault or
// subaccount. The signer remains the master/API wallet.
func WithVaultAddress(address common.Address) SubmitOption {
	return func(c *submitConfig) error {
		if address == (common.Address{}) {
			return fmt.Errorf("vault address is required")
		}
		c.vaultAddress = &address
		return nil
	}
}

// WithExpiresAfter rejects actions at the protocol layer after this Unix
// millisecond timestamp. It applies only to L1 actions, never user-signed
// actions.
func WithExpiresAfter(timestamp uint64) SubmitOption {
	return func(c *submitConfig) error {
		if timestamp == 0 {
			return fmt.Errorf("expiresAfter must be positive")
		}
		c.expiresAfter = &timestamp
		return nil
	}
}

// NewClient creates an Exchange client without optional submission settings.
// It is normally called by the root client.
func NewClient(baseURL, network string, t transport.HTTPTransport, timeout time.Duration, s signer.DigestSigner, n nonce.Manager, a asset.Resolver, userAgent string) *Client {
	client, _ := NewClientWithOptions(baseURL, network, t, timeout, s, n, a, userAgent)
	return client
}

// NewClientWithOptions creates an Exchange client with validated protocol
// submission settings.
func NewClientWithOptions(baseURL, network string, t transport.HTTPTransport, timeout time.Duration, s signer.DigestSigner, n nonce.Manager, a asset.Resolver, userAgent string, options ...SubmitOption) (*Client, error) {
	if t == nil {
		t = transport.NewDefaultHTTPTransport(nil)
	}
	if timeout <= 0 {
		timeout = 5 * time.Second
	}
	config := submitConfig{}
	for _, option := range options {
		if option == nil {
			return nil, fmt.Errorf("nil submit option")
		}
		if err := option(&config); err != nil {
			return nil, err
		}
	}
	return &Client{baseURL: baseURL, network: network, transport: t, timeout: timeout, signer: s, nonce: n, assets: a, userAgent: userAgent, submit: config}, nil
}
