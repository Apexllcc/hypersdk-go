package signing_test

import (
	"encoding/hex"
	"github.com/Apexllcc/hyperliquid-go-sdk/signing"
	"testing"
)

func TestAdditionalL1ActionVectorsMatchOfficialPythonSDK(t *testing.T) {
	t.Parallel()
	time := uint64(1700000100000)
	cases := []struct {
		name                      string
		action                    any
		bytes, connection, digest string
	}{
		{"cancel by cloid", signing.CancelByCloidAction{Cancels: []signing.CancelByCloidWire{{Asset: 0, Cloid: "0x1234567890abcdef1234567890abcdef"}}}, "82a474797065ad63616e63656c4279436c6f6964a763616e63656c739182a5617373657400a5636c6f6964d92230783132333435363738393061626364656631323334353637383930616263646566", "435c79a1720d068d43b842b0e6b5386afe432bf4d9eb2950dd4c07b67cef7322", "ae1645a334e7c872cafb1d36bdd8a173b7a128f618836b07cbbb4b4753e62ef5"},
		{"schedule cancel", signing.ScheduleCancelAction{Time: &time}, "82a474797065ae7363686564756c6543616e63656ca474696d65cf0000018bcfe6eea0", "e7bb4cc34ad17f264166d3d3cc2033c60bbc0e50cac381ce85777a040595ebff", "2935cc068a9aea54c615a2a8dd29ac20c24f475f0ab6b3c259fafec376fe1afd"},
		{"batch modify", signing.BatchModifyAction{Modifies: []signing.ModifyWire{{OID: 123, Order: signing.OrderWire{Asset: 0, IsBuy: true, Price: "60000", Size: "0.001", Type: signing.LimitOrderType{TIF: "Gtc"}}}}}, "82a474797065ab62617463684d6f64696679a86d6f6469666965739182a36f69647ba56f7264657286a16100a162c3a170a53630303030a173a5302e303031a172c2a17481a56c696d697481a3746966a3477463", "d1c88eb530532ae4cdfdc6f18fada0df2b47e235d979339a14e36110fc7ea9b7", "fb233fe499bc41bdc7e3077ed4415b4227e7eeb21ceda0c2f2d4182b1fe3771b"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			components, err := signing.L1ActionComponents(tc.action, 1700000000000, nil, nil)
			if err != nil {
				t.Fatal(err)
			}
			if got := hex.EncodeToString(components.ActionBytes); got != tc.bytes {
				t.Fatalf("bytes=%s", got)
			}
			if got := hex.EncodeToString(components.ConnectionID[:]); got != tc.connection {
				t.Fatalf("connection=%s", got)
			}
			digest, err := signing.ComputeL1ActionDigest(tc.action, 1700000000000, nil, nil, true)
			if err != nil {
				t.Fatal(err)
			}
			if got := hex.EncodeToString(digest[:]); got != tc.digest {
				t.Fatalf("digest=%s", got)
			}
		})
	}
}
