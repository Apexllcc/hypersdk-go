package hyperliquid

import (
	"time"

	"github.com/Apexllcc/hypersdk-go/asset"
	"github.com/Apexllcc/hypersdk-go/nonce"
	"github.com/Apexllcc/hypersdk-go/signer"
	"github.com/Apexllcc/hypersdk-go/transport"
	"github.com/Apexllcc/hypersdk-go/websocket"
	"github.com/ethereum/go-ethereum/common"
)

type config struct {
	network                      Network
	endpoints                    Endpoints
	signer                       signer.DigestSigner
	nonce                        nonce.Manager
	asset                        asset.Resolver
	http                         transport.HTTPTransport
	request                      transport.RequestTransport
	explorerRequest              transport.RequestTransport
	infoTimeout, exchangeTimeout time.Duration
	userAgent                    string
	websocket                    websocket.Config
	infoRetry                    transport.RetryPolicy
	middleware                   []transport.Middleware
	vaultAddress                 *common.Address
	expiresAfter                 *uint64
}

func defaultConfig() config {
	e, _ := endpointsFor(Mainnet)
	return config{network: Mainnet, endpoints: e, nonce: nonce.NewMonotonicManager(nil), http: transport.NewDefaultHTTPTransport(nil), infoTimeout: 5 * time.Second, exchangeTimeout: 5 * time.Second, userAgent: "hypersdk-go", infoRetry: transport.DefaultRetryPolicy()}
}
