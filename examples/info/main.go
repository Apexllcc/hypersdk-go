package main

import (
	"context"
	"fmt"

	hyperliquid "github.com/Apexllcc/hypersdk-go"
)

func main() {
	client, err := hyperliquid.NewClient(hyperliquid.WithTestnet())
	if err != nil {
		panic(err)
	}
	mids, err := client.Info.AllMids(context.Background())
	if err != nil {
		panic(err)
	}
	fmt.Println(mids)
}
