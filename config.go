package hyperliquid

import (
	"time"

	"github.com/Apexllcc/hyperliquid-go-sdk/asset"
	"github.com/Apexllcc/hyperliquid-go-sdk/nonce"
	"github.com/Apexllcc/hyperliquid-go-sdk/signer"
	"github.com/Apexllcc/hyperliquid-go-sdk/transport"
	"github.com/Apexllcc/hyperliquid-go-sdk/websocket"
)

type config struct {
	network                      Network
	endpoints                    Endpoints
	signer                       signer.DigestSigner
	nonce                        nonce.Manager
	asset                        asset.Resolver
	http                         transport.HTTPTransport
	infoTimeout, exchangeTimeout time.Duration
	userAgent                    string
	websocket                    websocket.Config
	infoRetry                    transport.RetryPolicy
}

func defaultConfig() config {
	e, _ := endpointsFor(Mainnet)
	return config{network: Mainnet, endpoints: e, nonce: nonce.NewMonotonicManager(nil), http: transport.NewDefaultHTTPTransport(nil), infoTimeout: 5 * time.Second, exchangeTimeout: 5 * time.Second, userAgent: "hyperliquid-go-sdk", infoRetry: transport.DefaultRetryPolicy()}
}
