package signer_test

import (
	"context"
	"encoding/hex"
	"testing"

	"github.com/Apexllcc/hypersdk-go/signer"
	"github.com/ethereum/go-ethereum/common"
)

func TestLocalPrivateKeySignerMatchesOfficialVectorAndRecoversAddress(t *testing.T) {
	t.Parallel()
	s, err := signer.NewLocalPrivateKeySignerFromHex("0123456789012345678901234567890123456789012345678901234567890123")
	if err != nil {
		t.Fatal(err)
	}
	var digest signer.Digest
	raw, _ := hex.DecodeString("0112d3197cf21279614ce6780ef32a056438bc5c5e4e6404a9be01e5658a01d8")
	copy(digest[:], raw)
	sig, err := s.SignDigest(context.Background(), digest)
	if err != nil {
		t.Fatal(err)
	}
	if got := hex.EncodeToString(sig.R[:]); got != "c301caf5befeaa397f775e09671cc9bc6665ed3a11ae72f849be31eef4eaffde" {
		t.Fatalf("R=%s", got)
	}
	if got := hex.EncodeToString(sig.S[:]); got != "71847b09f9a87edba4c5402ce8997d78ebc29ac01dc4bf02eee12a7a36b3e155" {
		t.Fatalf("S=%s", got)
	}
	if sig.V != 0 {
		t.Fatalf("V=%d", sig.V)
	}
	if s.Address() != common.HexToAddress("0x14791697260E4c9A71f18484C9f997B308e59325") {
		t.Fatalf("address=%s", s.Address())
	}
	if err := signer.Verify(digest, sig, s.Address()); err != nil {
		t.Fatal(err)
	}
}
