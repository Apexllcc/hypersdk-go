// Package signer defines digest-only signing and local signature verification.
package signer

import (
	"context"
	"errors"
	"fmt"
	"math/big"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/crypto"
)

var (
	ErrSignerRequired        = errors.New("digest signer is required")
	ErrInvalidDigest         = errors.New("invalid digest")
	ErrInvalidSignature      = errors.New("invalid signature")
	ErrInvalidRecoveryID     = errors.New("invalid recovery id")
	ErrHighSSignature        = errors.New("signature has non-canonical high-s value")
	ErrSignatureRecovery     = errors.New("signature recovery failed")
	ErrSignerAddressMismatch = errors.New("signer address mismatch")
)

// Digest is a final, unprefixed 32-byte signing digest.
type Digest [32]byte

// Signature is a canonical secp256k1 signature with recovery ID 0 or 1.
type Signature struct {
	R [32]byte
	S [32]byte
	V uint8
}

// DigestSigner signs only a final digest. Implementations may be local or
// external; this module deliberately defines no remote signing protocol.
type DigestSigner interface {
	Address() common.Address
	SignDigest(context.Context, Digest) (Signature, error)
}

// NormalizeRecoveryID normalizes Ethereum's legacy 27/28 values to 0/1.
func NormalizeRecoveryID(v uint8) (uint8, error) {
	switch v {
	case 0, 1:
		return v, nil
	case 27, 28:
		return v - 27, nil
	default:
		return 0, fmt.Errorf("%w: %d", ErrInvalidRecoveryID, v)
	}
}

// Bytes returns R || S || V with a normalized recovery ID.
func (s Signature) Bytes() ([]byte, error) {
	v, err := NormalizeRecoveryID(s.V)
	if err != nil {
		return nil, err
	}
	b := make([]byte, 65)
	copy(b[:32], s.R[:])
	copy(b[32:64], s.S[:])
	b[64] = v
	return b, nil
}

// Verify validates a canonical signature and proves that it recovers expected.
func Verify(digest Digest, signature Signature, expected common.Address) error {
	b, err := signature.Bytes()
	if err != nil {
		return err
	}
	r, s := new(big.Int).SetBytes(signature.R[:]), new(big.Int).SetBytes(signature.S[:])
	if !crypto.ValidateSignatureValues(b[64], r, s, true) {
		if s.Cmp(new(big.Int).Rsh(crypto.S256().Params().N, 1)) > 0 {
			return ErrHighSSignature
		}
		return ErrInvalidSignature
	}
	pub, err := crypto.SigToPub(digest[:], b)
	if err != nil {
		return fmt.Errorf("%w: %v", ErrSignatureRecovery, err)
	}
	if got := crypto.PubkeyToAddress(*pub); got != expected {
		return fmt.Errorf("%w: expected %s, got %s", ErrSignerAddressMismatch, expected, got)
	}
	return nil
}
