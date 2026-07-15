package websocket

import "fmt"

// l2BookKey is a stable registry key for a semantic subscription request.
func l2BookKey(request L2BookRequest) string {
	return fmt.Sprintf("l2Book:%s:%v:%v", request.Coin, request.NSigFigs, request.Mantissa)
}

func allMidsKey(request AllMidsRequest) string { return fmt.Sprintf("allMids:%s", request.DEX) }
func tradesKey(request TradesRequest) string   { return fmt.Sprintf("trades:%s", request.Coin) }
func candleKey(request CandleRequest) string {
	return fmt.Sprintf("candle:%s:%s", request.Coin, request.Interval)
}
func bboKey(request BBORequest) string { return fmt.Sprintf("bbo:%s", request.Coin) }
