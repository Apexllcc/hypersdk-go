// Package hyperliquid provides a precision-safe, transport-pluggable client for
// Hyperliquid's Info, Exchange, and WebSocket APIs.
package hyperliquid

import (
	"errors"
	"fmt"

	"github.com/Apexllcc/hypersdk-go/asset"
	"github.com/Apexllcc/hypersdk-go/exchange"
	"github.com/Apexllcc/hypersdk-go/explorer"
	"github.com/Apexllcc/hypersdk-go/info"
	"github.com/Apexllcc/hypersdk-go/websocket"
)

// Client groups the independently usable Info, Exchange, and WebSocket APIs.
type Client struct {
	Info      *info.Client
	Exchange  *exchange.Client
	Explorer  *explorer.Client
	WebSocket *websocket.Client
}

// Close releases network resources owned by the root client. It closes the
// public WebSocket client and any lazily created Explorer subscription client.
// It deliberately does not close injected HTTP transports, nonce managers,
// asset resolvers, or DigestSigners because those dependencies remain owned by
// the caller and can be shared by other clients.
//
// Close is idempotent. All closable root-owned resources are attempted even if
// one close operation reports an error.
func (c *Client) Close() error {
	if c == nil {
		return nil
	}
	var errs []error
	if c.WebSocket != nil {
		errs = append(errs, c.WebSocket.Close())
	}
	if c.Explorer != nil {
		errs = append(errs, c.Explorer.Close())
	}
	return errors.Join(errs...)
}

// NewClient creates an Info-only capable client. A signer is required only by
// Exchange methods that submit signed actions.
func NewClient(options ...Option) (*Client, error) {
	c := defaultConfig()
	for _, option := range options {
		if option == nil {
			return nil, fmt.Errorf("nil client option")
		}
		if err := option(&c); err != nil {
			return nil, err
		}
	}
	for i := len(c.middleware) - 1; i >= 0; i-- {
		c.http = c.middleware[i](c.http)
	}
	infoClient := info.NewClient(c.endpoints.Info, c.http, c.infoTimeout, c.userAgent, c.infoRetry)
	if c.request != nil {
		infoClient.SetRequestTransport(c.request)
	}
	resolver := c.asset
	if resolver == nil {
		metaOptions := make([]asset.MetaResolverOption, 0, 1)
		if c.network == Testnet {
			metaOptions = append(metaOptions, asset.WithOutcomeMetadata())
		}
		metaResolver, err := asset.NewMetaResolver(infoClient, metaOptions...)
		if err != nil {
			return nil, err
		}
		resolver = metaResolver
	}
	exchangeOptions := make([]exchange.SubmitOption, 0, 2)
	if c.vaultAddress != nil {
		exchangeOptions = append(exchangeOptions, exchange.WithVaultAddress(*c.vaultAddress))
	}
	if c.expiresAfter != nil {
		exchangeOptions = append(exchangeOptions, exchange.WithExpiresAfter(*c.expiresAfter))
	}
	exchangeClient, err := exchange.NewClientWithOptions(c.endpoints.Exchange, string(c.network), c.http, c.exchangeTimeout, c.signer, c.nonce, resolver, c.userAgent, exchangeOptions...)
	if err != nil {
		return nil, err
	}
	if c.request != nil {
		exchangeClient.SetRequestTransport(c.request)
	}
	explorerClient := explorer.NewClientWithWebSocket(c.endpoints.Explorer, c.http, c.infoTimeout, c.userAgent, c.endpoints.ExplorerWebSocket, c.websocket)
	if c.explorerRequest != nil {
		explorerClient.SetRequestTransport(c.explorerRequest)
	}
	return &Client{Info: infoClient, Exchange: exchangeClient, Explorer: explorerClient, WebSocket: websocket.NewClient(c.endpoints.WebSocket, c.websocket)}, nil
}
