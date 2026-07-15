package info

// AllMidsRequest requests all mid prices.
type AllMidsRequest struct {
	DEX  string `json:"dex,omitempty"`
	Type string `json:"type"`
}

// MetaRequest requests perpetual metadata.
type MetaRequest struct {
	Type string `json:"type"`
	DEX  string `json:"dex,omitempty"`
}

// L2BookRequest requests a market order book.
type L2BookRequest struct {
	Type     string `json:"type"`
	Coin     string `json:"coin"`
	NSigFigs *int   `json:"nSigFigs,omitempty"`
	Mantissa *int   `json:"mantissa,omitempty"`
}

// CandleSnapshotRequest requests historical candles in milliseconds.
type CandleSnapshotRequest struct {
	Type string        `json:"type"`
	Req  CandleRequest `json:"req"`
}

// CandleRequest is the nested official candleSnapshot request payload.
type CandleRequest struct {
	Coin      string `json:"coin"`
	Interval  string `json:"interval"`
	StartTime int64  `json:"startTime"`
	EndTime   *int64 `json:"endTime,omitempty"`
}

// ClearinghouseStateRequest requests a perpetual account state.
type ClearinghouseStateRequest struct {
	Type string `json:"type"`
	User string `json:"user"`
	DEX  string `json:"dex,omitempty"`
}

// SpotClearinghouseStateRequest requests a spot account state.
type SpotClearinghouseStateRequest struct {
	Type string `json:"type"`
	User string `json:"user"`
}
