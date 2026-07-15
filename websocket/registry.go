package websocket

import "fmt"

// l2BookKey is a stable registry key for a semantic subscription request.
func l2BookKey(request L2BookRequest) string {
	return fmt.Sprintf("l2Book:%s:%v:%v", request.Coin, request.NSigFigs, request.Mantissa)
}
