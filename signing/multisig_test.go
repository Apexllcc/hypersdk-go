package signing_test

import (
	"context"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"testing"

	"github.com/Apexllcc/hypersdk-go/signer"
	"github.com/Apexllcc/hypersdk-go/signing"
	"github.com/ethereum/go-ethereum/common"
	"github.com/vmihailenco/msgpack/v5"
)

func TestCompactSignatureMessagePackUsesCanonicalLowercaseKeys(t *testing.T) {
	raw, err := msgpack.Marshal(signing.CompactSignature{R: "0x1", S: "0x2", V: 27})
	if err != nil {
		t.Fatal(err)
	}
	const want = "83a172a3307831a173a3307832a1761b"
	if got := hex.EncodeToString(raw); got != want {
		t.Fatalf("compact signature MessagePack = %s, want %s", got, want)
	}
}

func TestMultiSigL1AndEnvelopeFixedVector(t *testing.T) {
	user := common.HexToAddress("0x1111111111111111111111111111111111111111")
	leader := common.HexToAddress("0x14791697260E4c9A71f18484C9f997B308e59325")
	nonce := uint64(1_700_000_000_123)
	inner, err := signing.ComputeMultiSigL1PayloadDigest(signing.NoopAction{}, user, leader, nonce, nil, nil, true)
	if err != nil {
		t.Fatal(err)
	}
	envelope := signing.MultiSigEnvelopeAction{SignatureChainID: "0x66eee", Signatures: []signing.CompactSignature{{R: "0x1", S: "0x2", V: 27}}, Payload: signing.MultiSigPayload{MultiSigUser: "0x1111111111111111111111111111111111111111", OuterSigner: "0x14791697260e4c9a71f18484c9f997b308e59325", Action: signing.NoopAction{}}}
	outer, err := signing.ComputeMultiSigEnvelopeDigest(envelope, nonce, nil, nil, true)
	if err != nil {
		t.Fatal(err)
	}
	if got := hex.EncodeToString(inner[:]); got != "73f9b7d4360f6d544e4f6328f9575c248c0a1d52b5512a0756be890869e74f07" {
		t.Fatalf("inner digest=%s", got)
	}
	if got := hex.EncodeToString(outer[:]); got != "62ac8525e06243d9c7c08054aa81a66493603b006ce0422e2069ae7911d9ac8b" {
		t.Fatalf("outer digest=%s", got)
	}
	local, err := signer.NewLocalPrivateKeySignerFromHex("0123456789012345678901234567890123456789012345678901234567890123")
	if err != nil {
		t.Fatal(err)
	}
	innerSignature, err := local.SignDigest(context.Background(), inner)
	if err != nil {
		t.Fatal(err)
	}
	outerSignature, err := local.SignDigest(context.Background(), outer)
	if err != nil {
		t.Fatal(err)
	}
	if got := hex.EncodeToString(innerSignature.R[:]) + hex.EncodeToString(innerSignature.S[:]) + fmt.Sprint(innerSignature.V); got != "3a02f67799785fe016898877fdfc03e706b2537ca8e8277f8f312952f95d59441adc227fa773504760fb0fb28fc580974c57b9185d4084e760c112e5d71f3e7c0" {
		t.Fatalf("inner R/S/V=%s", got)
	}
	if got := hex.EncodeToString(outerSignature.R[:]) + hex.EncodeToString(outerSignature.S[:]) + fmt.Sprint(outerSignature.V); got != "23585417e5c4bd1e20b23270e6ff847d5ddd2d13efdd0924c8d470d39ca770d277c826dcaf678d5f59edbf33d2e9100491f0ec7a2b76050e464d9d52d08b6d711" {
		t.Fatalf("outer R/S/V=%s", got)
	}
}

func TestMultiSigUserSetAbstractionUsesCompactPayloadWire(t *testing.T) {
	action := signing.UserSetAbstractionAction{User: common.HexToAddress("0xAa000000000000000000000000000000000000aA"), Abstraction: "unifiedAccount", Nonce: 7}
	payload, err := signing.MultiSigUserPayloadWire(action, true)
	if err != nil {
		t.Fatal(err)
	}
	raw, err := json.Marshal(payload)
	if err != nil {
		t.Fatal(err)
	}
	var parsed struct {
		Abstraction string `json:"abstraction"`
		User        string `json:"user"`
	}
	if err := json.Unmarshal(raw, &parsed); err != nil {
		t.Fatal(err)
	}
	if parsed.Abstraction != "u" || parsed.User != "0xaa000000000000000000000000000000000000aa" {
		t.Fatalf("payload=%+v", parsed)
	}
}

func TestMultiSigUserDexAbstractionUsesLowercasePayloadAddress(t *testing.T) {
	action := signing.UserDexAbstractionAction{User: common.HexToAddress("0xAa000000000000000000000000000000000000aA"), Enabled: true, Nonce: 7}
	payload, err := signing.MultiSigUserPayloadWire(action, true)
	if err != nil {
		t.Fatal(err)
	}
	raw, err := json.Marshal(payload)
	if err != nil {
		t.Fatal(err)
	}
	var parsed struct {
		User string `json:"user"`
	}
	if err := json.Unmarshal(raw, &parsed); err != nil {
		t.Fatal(err)
	}
	if parsed.User != "0xaa000000000000000000000000000000000000aa" {
		t.Fatalf("payload user=%q", parsed.User)
	}
}

func TestMultiSigUserSignedFixedVector(t *testing.T) {
	user := common.HexToAddress("0x1111111111111111111111111111111111111111")
	leader := common.HexToAddress("0x14791697260E4c9A71f18484C9f997B308e59325")
	action := signing.ConvertToMultiSigUserAction{Signers: nil, Nonce: 1_700_000_000_123}
	digest, err := signing.ComputeMultiSigUserActionDigest(action, user, leader, true)
	if err != nil {
		t.Fatal(err)
	}
	local, err := signer.NewLocalPrivateKeySignerFromHex("0123456789012345678901234567890123456789012345678901234567890123")
	if err != nil {
		t.Fatal(err)
	}
	signature, err := local.SignDigest(context.Background(), digest)
	if err != nil {
		t.Fatal(err)
	}
	if err := signer.Verify(digest, signature, local.Address()); err != nil {
		t.Fatal(err)
	}
	if got := hex.EncodeToString(digest[:]) + hex.EncodeToString(signature.R[:]) + hex.EncodeToString(signature.S[:]) + fmt.Sprint(signature.V); got != "408f32ae2eb7a4e9f708b3506477a0c9c3776dc7139a7aeb53f2f9a1b6f3d43ee79adc27b4b5dfa084e10ceadae05e44129edd13be35ef989795660baa0c7c4f5f347b684d2ea0aa5e472636da57c1aca4cf79ebe399cee7c1211eeaf3d74a0f0" {
		t.Fatalf("multi-sig user digest/R/S/V=%s", got)
	}
}
