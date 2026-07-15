package signing_test

import (
	"encoding/hex"
	"testing"

	"github.com/Apexllcc/hyperliquid-go-sdk/signing"
)

func TestTWAPActionMessagePackVector(t *testing.T) {
	action := signing.TWAPOrderAction{TWAP: signing.TWAPWire{
		Asset:      0,
		IsBuy:      true,
		Size:       "1.2",
		ReduceOnly: false,
		Minutes:    30,
		Randomize:  true,
	}}
	components, err := signing.L1ActionComponents(action, 1_700_000_000_000, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	const want = "82a474797065a9747761704f72646572a47477617086a16100a162c3a173a3312e32a172c2a16d1ea174c3"
	if got := hex.EncodeToString(components.ActionBytes); got != want {
		t.Fatalf("action msgpack = %s, want %s", got, want)
	}
	digest, err := signing.ComputeL1ActionDigest(action, 1_700_000_000_000, nil, nil, true)
	if err != nil {
		t.Fatal(err)
	}
	const wantDigest = "53d8d1ddb04d56edba441cf302efca325214776b217be8383b2d3e2d1e2bebb3"
	if got := hex.EncodeToString(digest[:]); got != wantDigest {
		t.Fatalf("L1 digest = %s, want %s", got, wantDigest)
	}
}

func TestTWAPCancelActionMessagePackVector(t *testing.T) {
	action := signing.TWAPCancelAction{Asset: 7, TWAPID: 77_738_308}
	components, err := signing.L1ActionComponents(action, 1_700_000_000_000, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	const want = "83a474797065aa7477617043616e63656ca16107a174ce04a23144"
	if got := hex.EncodeToString(components.ActionBytes); got != want {
		t.Fatalf("action msgpack = %s, want %s", got, want)
	}
}
