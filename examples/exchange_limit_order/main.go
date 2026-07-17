// Command exchange_limit_order shows how to construct a precision-safe BTC
// limit order for Testnet. It is intentionally a dry run: it never creates a
// signer and never submits an Exchange action.
//
// To turn a reviewed request into a Testnet submission, follow the Exchange
// section in README.md and set both HL_TESTNET_TRADE=1 and
// HL_TESTNET_PRIVATE_KEY in your own controlled environment.
package main

import (
	"fmt"
	"os"

	"github.com/Apexllcc/hypersdk-go/exchange"
	"github.com/shopspring/decimal"
)

func main() {
	order := exchange.OrderRequest{
		Coin:       "BTC",
		IsBuy:      true,
		Price:      decimal.RequireFromString("10000"),
		Size:       decimal.RequireFromString("0.001"),
		ReduceOnly: false,
		Type:       exchange.LimitOrder{TimeInForce: exchange.TIFGTC},
	}

	if os.Getenv("HL_TESTNET_TRADE") == "1" {
		fmt.Println("HL_TESTNET_TRADE is acknowledged; this example remains a dry run.")
	}
	fmt.Printf("review this Testnet order before submission: %+v\n", order)
}
