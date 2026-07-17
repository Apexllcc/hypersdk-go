package websocket

func newL2SubscriptionWire(request L2BookRequest) subscriptionWire {
	fields := map[string]any{"coin": request.Coin}
	if request.NSigFigs != nil {
		fields["nSigFigs"] = *request.NSigFigs
	}
	if request.Mantissa != nil {
		fields["mantissa"] = *request.Mantissa
	}
	if request.Fast != nil {
		fields["fast"] = *request.Fast
	}
	wire := newSubscriptionWire("l2Book", fields)
	return wire
}
