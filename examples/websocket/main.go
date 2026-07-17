// Command websocket subscribes to Hyperliquid's public all-mids stream.
//
// The example needs no key and never sends an Exchange action. Cancel it with
// Ctrl-C after it receives the first market update.
package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"syscall"

	hyperliquid "github.com/Apexllcc/hypersdk-go"
	"github.com/Apexllcc/hypersdk-go/websocket"
)

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	client, err := hyperliquid.NewClient(hyperliquid.WithTestnet())
	if err != nil {
		panic(err)
	}
	defer func() { _ = client.WebSocket.Close() }()

	subscription, err := client.WebSocket.SubscribeAllMids(ctx, websocket.AllMidsRequest{})
	if err != nil {
		panic(err)
	}
	defer func() { _ = subscription.Close() }()

	select {
	case event := <-subscription.Events():
		fmt.Printf("testnet mids: %v\n", event.Mids)
	case err := <-subscription.Errors():
		panic(err)
	case <-ctx.Done():
		fmt.Println("stopped")
	}
}
