// Package hyperliquid provides a precision-safe, transport-pluggable client for
// Hyperliquid's Info, Exchange, and WebSocket APIs.
package hyperliquid

import (
	"fmt"

	"github.com/Apexllcc/hyperliquid-go-sdk/asset"
	"github.com/Apexllcc/hyperliquid-go-sdk/exchange"
	"github.com/Apexllcc/hyperliquid-go-sdk/info"
	"github.com/Apexllcc/hyperliquid-go-sdk/websocket"
)

// Client groups the independently usable Info, Exchange, and WebSocket APIs.
type Client struct {
	Info      *info.Client
	Exchange  *exchange.Client
	WebSocket *websocket.Client
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
	resolver := c.asset
	if resolver == nil {
		metaResolver, err := asset.NewMetaResolver(infoClient)
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
	return &Client{Info: infoClient, Exchange: exchangeClient, WebSocket: websocket.NewClient(c.endpoints.WebSocket, c.websocket)}, nil
}
