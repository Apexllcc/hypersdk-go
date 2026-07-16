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
	Info              string
	Exchange          string
	WebSocket         string
	Explorer          string
	ExplorerWebSocket string
}

func endpointsFor(network Network) (Endpoints, error) {
	switch network {
	case Mainnet:
		return Endpoints{Info: "https://api.hyperliquid.xyz/info", Exchange: "https://api.hyperliquid.xyz/exchange", WebSocket: "wss://api.hyperliquid.xyz/ws", Explorer: "https://rpc.hyperliquid.xyz/explorer", ExplorerWebSocket: "wss://rpc.hyperliquid.xyz/ws"}, nil
	case Testnet:
		return Endpoints{Info: "https://api.hyperliquid-testnet.xyz/info", Exchange: "https://api.hyperliquid-testnet.xyz/exchange", WebSocket: "wss://api.hyperliquid-testnet.xyz/ws", Explorer: "https://rpc.hyperliquid-testnet.xyz/explorer", ExplorerWebSocket: "wss://rpc.hyperliquid-testnet.xyz/ws"}, nil
	default:
		return Endpoints{}, fmt.Errorf("%w: %q", hlerr.ErrInvalidNetwork, network)
	}
}
