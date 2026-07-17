package signing_test

import (
	"context"
	"encoding/hex"
	"testing"

	"github.com/Apexllcc/hypersdk-go/signer"
	"github.com/Apexllcc/hypersdk-go/signing"
	"github.com/ethereum/go-ethereum/common"
)

func TestL1ActionVectorMatchesOfficialPythonSDK(t *testing.T) {
	t.Parallel()
	action := signing.CancelAction{Cancels: []signing.CancelWire{{Asset: 0, OID: 12345}}}
	components, err := signing.L1ActionComponents(action, 1700000000000, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	if got := hex.EncodeToString(components.ActionBytes); got != "82a474797065a663616e63656ca763616e63656c739182a16100a16fcd3039" {
		t.Fatalf("action bytes = %s", got)
	}
	if got := hex.EncodeToString(components.ConnectionID[:]); got != "a1cb188de2cd5e1f2684211f96d83f1b05541001e55eb2e98192f4d87623d36b" {
		t.Fatalf("connection ID = %s", got)
	}
	digest, err := signing.ComputeL1ActionDigest(action, 1700000000000, nil, nil, true)
	if err != nil {
		t.Fatal(err)
	}
	if got := hex.EncodeToString(digest[:]); got != "0112d3197cf21279614ce6780ef32a056438bc5c5e4e6404a9be01e5658a01d8" {
		t.Fatalf("digest = %s", got)
	}
}

func TestL1ActionVectorWithVaultExpiryAndTestnetMatchesOfficialPythonSDK(t *testing.T) {
	t.Parallel()
	action := signing.OrderAction{Orders: []signing.OrderWire{{Asset: 0, IsBuy: true, Price: "60000", Size: "0.001", Type: signing.LimitOrderType{TIF: "Gtc"}}}, Grouping: "na"}
	vault := common.HexToAddress("0x1111111111111111111111111111111111111111")
	expires := uint64(1700001234567)
	components, err := signing.L1ActionComponents(action, 1700000000001, &vault, &expires)
	if err != nil {
		t.Fatal(err)
	}
	if got := hex.EncodeToString(components.ActionBytes); got != "83a474797065a56f72646572a66f72646572739186a16100a162c3a170a53630303030a173a5302e303031a172c2a17481a56c696d697481a3746966a3477463a867726f7570696e67a26e61" {
		t.Fatalf("action bytes = %s", got)
	}
	if got := hex.EncodeToString(components.ConnectionID[:]); got != "819649ab003af3992bcd6163f31d0b84d567501e1ced44574560a527ba16ab6e" {
		t.Fatalf("connection ID = %s", got)
	}
	digest, err := signing.ComputeL1ActionDigest(action, 1700000000001, &vault, &expires, false)
	if err != nil {
		t.Fatal(err)
	}
	if got := hex.EncodeToString(digest[:]); got != "f1fb5803c0af51e3bd9ff5f0266f985b19fc4e2dc237dd93a06fecca0a0c2dfa" {
		t.Fatalf("digest = %s", got)
	}
}

func TestTriggerBuilderL1ActionVectorMatchesOfficialPythonSDK(t *testing.T) {
	t.Parallel()
	action := signing.OrderAction{
		Orders: []signing.OrderWire{{
			Asset: 0, IsBuy: false, Price: "58000", Size: "0.1",
			Type: signing.TriggerOrderType{IsMarket: true, TriggerPx: "59000", TPSL: "sl"},
		}},
		Grouping: "na",
		Builder:  &signing.BuilderWire{Address: "0x2222222222222222222222222222222222222222", Fee: 10},
	}
	vault := common.HexToAddress("0x1111111111111111111111111111111111111111")
	expires := uint64(1700001234567)
	components, err := signing.L1ActionComponents(action, 1700000000002, &vault, &expires)
	if err != nil {
		t.Fatal(err)
	}
	if got := hex.EncodeToString(components.ActionBytes); got != "84a474797065a56f72646572a66f72646572739186a16100a162c2a170a53538303030a173a3302e31a172c2a17481a77472696767657283a869734d61726b6574c3a9747269676765725078a53539303030a47470736ca2736ca867726f7570696e67a26e61a76275696c64657282a162d92a307832323232323232323232323232323232323232323232323232323232323232323232323232323232a1660a" {
		t.Fatalf("action bytes = %s", got)
	}
	if got := hex.EncodeToString(components.ConnectionID[:]); got != "8aac90034523c74e4dc86b1f21f5bc5d21a6985bb96415c8d3550577d31d1968" {
		t.Fatalf("connection ID = %s", got)
	}
	digest, err := signing.ComputeL1ActionDigest(action, 1700000000002, &vault, &expires, false)
	if err != nil {
		t.Fatal(err)
	}
	if got := hex.EncodeToString(digest[:]); got != "cb740d58f3413c389c7edee731f7951d6bbbe14673b55be9cad7e95f2953f0d9" {
		t.Fatalf("digest = %s", got)
	}
	local, err := signer.NewLocalPrivateKeySignerFromHex("0123456789012345678901234567890123456789012345678901234567890123")
	if err != nil {
		t.Fatal(err)
	}
	signature, err := local.SignDigest(context.Background(), digest)
	if err != nil {
		t.Fatal(err)
	}
	if got := hex.EncodeToString(signature.R[:]); got != "0a9a1eca0bd553eda94053863d5b041f9cfacc55643462047a535c6c467d26b1" {
		t.Fatalf("R = %s", got)
	}
	if got := hex.EncodeToString(signature.S[:]); got != "18906c96893df88f340bdb160902261fd498cda07f23cec124f2284b7704cf06" {
		t.Fatalf("S = %s", got)
	}
	if signature.V != 0 {
		t.Fatalf("V = %d", signature.V)
	}
	if err := signer.Verify(digest, signature, local.Address()); err != nil {
		t.Fatalf("recover signer address: %v", err)
	}
}
