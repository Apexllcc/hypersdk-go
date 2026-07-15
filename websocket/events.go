package websocket

import "encoding/json"

// L2BookRequest identifies one L2 book stream.
type L2BookRequest struct {
	Coin     string `json:"coin"`
	NSigFigs *int   `json:"nSigFigs,omitempty"`
	Mantissa *int   `json:"mantissa,omitempty"`
}

// L2BookEvent is one L2 book event. Levels retain their protocol JSON until a
// shared order-book DTO is introduced.
type L2BookEvent struct {
	Coin   string          `json:"coin"`
	Time   int64           `json:"time"`
	Levels json.RawMessage `json:"levels"`
}
