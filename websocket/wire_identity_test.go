package websocket

import "testing"

func TestSubscriptionMatchScoreOnlyNormalizesNumericJSONTypes(t *testing.T) {
	request := map[string]any{"type": "l2Book", "coin": "BTC", "nSigFigs": 5}
	if _, ok := subscriptionMatchScore(request, map[string]any{"type": "l2Book", "coin": "BTC", "nSigFigs": "5"}); ok {
		t.Fatal("string nSigFigs matched numeric request")
	}
	if _, ok := subscriptionMatchScore(request, map[string]any{"type": "l2Book", "coin": "BTC", "nSigFigs": float64(5)}); !ok {
		t.Fatal("JSON float64 nSigFigs did not match numeric request")
	}
}
