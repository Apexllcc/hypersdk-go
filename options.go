package hyperliquid

import (
	"fmt"
	"net/http"
	"net/url"
	"time"

	"github.com/Apexllcc/hypersdk-go/asset"
	"github.com/Apexllcc/hypersdk-go/nonce"
	"github.com/Apexllcc/hypersdk-go/signer"
	"github.com/Apexllcc/hypersdk-go/transport"
	"github.com/Apexllcc/hypersdk-go/websocket"
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

// WithExplorerBaseURL replaces the read-only explorer RPC HTTP endpoint.
func WithExplorerBaseURL(rawURL string) Option {
	return setURL("explorer", rawURL, func(c *config, v string) { c.endpoints.Explorer = v })
}
func WithWebSocketURL(rawURL string) Option {
	return setURL("websocket", rawURL, func(c *config, v string) { c.endpoints.WebSocket = v })
}

// WithExplorerWebSocketURL replaces the explorer RPC subscription endpoint.
// It does not affect Info or Exchange WebSocket post requests.
func WithExplorerWebSocketURL(rawURL string) Option {
	return setURL("explorer websocket", rawURL, func(c *config, v string) { c.endpoints.ExplorerWebSocket = v })
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
// The transport remains caller-owned: Client.Close never closes it, so it may
// safely be shared by multiple SDK clients. WebSocket post transports support
// only Info and Action requests; Explorer is intentionally excluded by the
// Hyperliquid protocol. Exchange actions are still submitted exactly once by
// the Exchange client.
func WithRequestTransport(t transport.RequestTransport) Option {
	return func(c *config) error {
		if t == nil {
			return fmt.Errorf("invalid request transport: nil")
		}
		c.request = t
		return nil
	}
}

// WithExplorerRequestTransport replaces only read-only Explorer HTTP requests.
// The official API WebSocket post protocol does not accept Explorer requests,
// so it is deliberately not selected by WithRequestTransport.
func WithExplorerRequestTransport(t transport.RequestTransport) Option {
	return func(c *config) error {
		if t == nil {
			return fmt.Errorf("invalid explorer request transport: nil")
		}
		c.explorerRequest = t
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

// WithRateLimitPolicy applies a weighted HTTP attempt limiter to all root
// clients. The policy is caller-supplied, while the limiter uses the official
// 1200-weight-per-minute budget. It never retries Exchange actions.
func WithRateLimitPolicy(policy transport.WeightPolicy) Option {
	return WithRateLimitPolicyAndLimiter(policy, transport.NewWeightLimiter(transport.OfficialRateLimitBudget, time.Minute))
}

// WithRateLimitPolicyAndLimiter applies policy through caller-supplied
// admission state. Share one limiter across root clients that use the same IP
// budget. The limiter remains caller-owned.
func WithRateLimitPolicyAndLimiter(policy transport.WeightPolicy, limiter transport.AdmissionLimiter) Option {
	return func(c *config) error {
		if policy == nil {
			return fmt.Errorf("invalid rate limit policy: nil")
		}
		if limiter == nil {
			return fmt.Errorf("invalid rate limit limiter: nil")
		}
		c.middleware = append(c.middleware, transport.WeightedRateLimitWithLimiter(policy, limiter))
		return nil
	}
}

// WithOfficialRateLimit applies Hyperliquid's documented weighted REST policy.
func WithOfficialRateLimit() Option { return WithRateLimitPolicy(transport.OfficialWeightPolicy()) }

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
