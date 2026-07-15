package hyperliquid

import (
	"fmt"
	"github.com/Apexllcc/hyperliquid-go-sdk/internal/hlerr"
)

// Network identifies a Hyperliquid deployment.
type Network string

const (
	// Mainnet is the production Hyperliquid network.
	Mainnet Network = "mainnet"
	// Testnet is Hyperliquid's test network.
	Testnet Network = "testnet"
)

// Endpoints contains all network-specific endpoints. Keeping these together
// prevents signing and transport settings from silently crossing networks.
type Endpoints struct {
	Info      string
	Exchange  string
	WebSocket string
}

func endpointsFor(network Network) (Endpoints, error) {
	switch network {
	case Mainnet:
		return Endpoints{"https://api.hyperliquid.xyz/info", "https://api.hyperliquid.xyz/exchange", "wss://api.hyperliquid.xyz/ws"}, nil
	case Testnet:
		return Endpoints{"https://api.hyperliquid-testnet.xyz/info", "https://api.hyperliquid-testnet.xyz/exchange", "wss://api.hyperliquid-testnet.xyz/ws"}, nil
	default:
		return Endpoints{}, fmt.Errorf("%w: %q", hlerr.ErrInvalidNetwork, network)
	}
}
