package websocket

type l2SubscriptionWire struct {
	Method       string `json:"method"`
	Subscription struct {
		Type     string `json:"type"`
		Coin     string `json:"coin"`
		NSigFigs *int   `json:"nSigFigs,omitempty"`
		Mantissa *int   `json:"mantissa,omitempty"`
	} `json:"subscription"`
}

func newL2SubscriptionWire(request L2BookRequest) subscriptionWire {
	fields := map[string]any{"coin": request.Coin}
	if request.NSigFigs != nil {
		fields["nSigFigs"] = *request.NSigFigs
	}
	if request.Mantissa != nil {
		fields["mantissa"] = *request.Mantissa
	}
	wire := newSubscriptionWire("l2Book", fields)
	return wire
}
