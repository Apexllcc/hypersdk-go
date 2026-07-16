package hyperliquid

import (
	"fmt"
	"net/http"
	"net/url"
	"time"

	"github.com/Apexllcc/hyperliquid-go-sdk/asset"
	"github.com/Apexllcc/hyperliquid-go-sdk/nonce"
	"github.com/Apexllcc/hyperliquid-go-sdk/signer"
	"github.com/Apexllcc/hyperliquid-go-sdk/transport"
	"github.com/Apexllcc/hyperliquid-go-sdk/websocket"
	"github.com/ethereum/go-ethereum/common"
)

// Option configures a Client.
type Option func(*config) error

func WithMainnet() Option { return WithNetwork(Mainnet) }
func WithTestnet() Option { return WithNetwork(Testnet) }
func WithNetwork(network Network) Option {
	return func(c *config) error {
		e, err := endpointsFor(network)
		if err != nil {
			return err
		}
		c.network, c.endpoints = network, e
		return nil
	}
}
func WithInfoBaseURL(rawURL string) Option {
	return setURL("info", rawURL, func(c *config, v string) { c.endpoints.Info = v })
}
func WithExchangeBaseURL(rawURL string) Option {
	return setURL("exchange", rawURL, func(c *config, v string) { c.endpoints.Exchange = v })
}
func WithWebSocketURL(rawURL string) Option {
	return setURL("websocket", rawURL, func(c *config, v string) { c.endpoints.WebSocket = v })
}
func WithDigestSigner(s signer.DigestSigner) Option {
	return func(c *config) error {
		if s == nil {
			return fmt.Errorf("invalid signer: nil")
		}
		c.signer = s
		return nil
	}
}

// WithVaultAddress signs L1 Exchange actions on behalf of a vault or
// subaccount. The configured DigestSigner remains the master/API wallet.
func WithVaultAddress(address common.Address) Option {
	return func(c *config) error {
		if address == (common.Address{}) {
			return fmt.Errorf("invalid vault address")
		}
		c.vaultAddress = &address
		return nil
	}
}

// WithExpiresAfter sets the optional L1 action expiry in Unix milliseconds.
// It is deliberately not used for user-signed actions.
func WithExpiresAfter(timestamp uint64) Option {
	return func(c *config) error {
		if timestamp == 0 {
			return fmt.Errorf("invalid expiresAfter")
		}
		c.expiresAfter = &timestamp
		return nil
	}
}
func WithNonceManager(m nonce.Manager) Option {
	return func(c *config) error {
		if m == nil {
			return fmt.Errorf("invalid nonce manager: nil")
		}
		c.nonce = m
		return nil
	}
}
func WithAssetResolver(r asset.Resolver) Option {
	return func(c *config) error {
		if r == nil {
			return fmt.Errorf("invalid asset resolver: nil")
		}
		c.asset = r
		return nil
	}
}
func WithHTTPTransport(t transport.HTTPTransport) Option {
	return func(c *config) error {
		if t == nil {
			return fmt.Errorf("invalid HTTP transport: nil")
		}
		c.http = t
		return nil
	}
}

// WithRequestTransport replaces the API request path for Info and Exchange.
// WebSocket post transports support only Info and Action requests; Explorer is
// intentionally excluded by the Hyperliquid protocol. Exchange actions are
// still submitted exactly once by the Exchange client.
func WithRequestTransport(t transport.RequestTransport) Option {
	return func(c *config) error {
		if t == nil {
			return fmt.Errorf("invalid request transport: nil")
		}
		c.request = t
		return nil
	}
}
func WithHTTPClient(client *http.Client) Option {
	return func(c *config) error {
		if client == nil {
			return fmt.Errorf("invalid HTTP client: nil")
		}
		c.http = transport.NewDefaultHTTPTransport(client)
		return nil
	}
}
func WithHTTPTimeout(timeout time.Duration) Option {
	return func(c *config) error {
		if timeout <= 0 {
			return fmt.Errorf("invalid HTTP timeout")
		}
		c.infoTimeout, c.exchangeTimeout = timeout, timeout
		return nil
	}
}
func WithInfoTimeout(timeout time.Duration) Option {
	return func(c *config) error {
		if timeout <= 0 {
			return fmt.Errorf("invalid info timeout")
		}
		c.infoTimeout = timeout
		return nil
	}
}
func WithInfoRetryPolicy(policy transport.RetryPolicy) Option {
	return func(c *config) error { c.infoRetry = policy; return nil }
}
func WithExchangeTimeout(timeout time.Duration) Option {
	return func(c *config) error {
		if timeout <= 0 {
			return fmt.Errorf("invalid exchange timeout")
		}
		c.exchangeTimeout = timeout
		return nil
	}
}
func WithUserAgent(userAgent string) Option {
	return func(c *config) error {
		if userAgent == "" {
			return fmt.Errorf("invalid user agent")
		}
		c.userAgent = userAgent
		return nil
	}
}

// WithWebSocketConfig configures bounded delivery and reconnect behavior.
func WithWebSocketConfig(wsConfig websocket.Config) Option {
	return func(c *config) error { c.websocket = wsConfig; return nil }
}
func WithMiddleware(middleware ...transport.Middleware) Option {
	return func(c *config) error {
		for _, item := range middleware {
			if item == nil {
				return fmt.Errorf("nil middleware")
			}
		}
		c.middleware = append(c.middleware, middleware...)
		return nil
	}
}
func setURL(name, rawURL string, apply func(*config, string)) Option {
	return func(c *config) error {
		u, err := url.ParseRequestURI(rawURL)
		if err != nil || u.Scheme == "" || u.Host == "" {
			return fmt.Errorf("invalid %s URL: %q", name, rawURL)
		}
		apply(c, rawURL)
		return nil
	}
}
