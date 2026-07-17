package signing_test

import (
	"encoding/hex"
	"github.com/Apexllcc/hypersdk-go/signing"
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
		{"subaccount transfer", signing.SubaccountTransferAction{SubaccountUser: "0x1111111111111111111111111111111111111111", IsDeposit: true, USD: 123}, "84a474797065b27375624163636f756e745472616e73666572ae7375624163636f756e7455736572d92a307831313131313131313131313131313131313131313131313131313131313131313131313131313131a969734465706f736974c3a37573647b", "0d7769178c7d15a1cbc7d8eaa8f3916476ed54dd135fee3564e8d59b0d017960", "8967322e29bd052db552221d69f0da15fac48806e59f8a270160b3cc14bcfae9"},
		{"subaccount spot transfer", signing.SubaccountSpotTransferAction{SubaccountUser: "0x1111111111111111111111111111111111111111", IsDeposit: false, Token: "PURR:0xc4bf3f870c0e9465323c0b6ed28096c2", Amount: "0.01"}, "85a474797065b67375624163636f756e7453706f745472616e73666572ae7375624163636f756e7455736572d92a307831313131313131313131313131313131313131313131313131313131313131313131313131313131a969734465706f736974c2a5746f6b656ed927505552523a30786334626633663837306330653934363533323363306236656432383039366332a6616d6f756e74a4302e3031", "99eddd3e5c3a4954e7b45752e2ff0de347615f24b19ca22b63202f2904a4c087", "c5ea54992b3e596144da798ae33b39739749672e5230bc3973af6789d254c2ea"},
		{"vault transfer", signing.VaultTransferAction{VaultAddress: "0x2222222222222222222222222222222222222222", IsDeposit: true, USD: 456}, "84a474797065ad7661756c745472616e73666572ac7661756c7441646472657373d92a307832323232323232323232323232323232323232323232323232323232323232323232323232323232a969734465706f736974c3a3757364cd01c8", "024188c7e7c4fa82c4fcafe1290867c7ea2db20ef0b3203120a2f032a9574596", "1185168af4b619c534dd7fa66b9e1e2bc9826f7647f0ec80dc81a23b738d5371"},
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
