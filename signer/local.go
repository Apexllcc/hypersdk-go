package signer

import (
	"context"
	"crypto/ecdsa"
	"fmt"
	"sync"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/crypto"
)

// LocalPrivateKeySigner is the optional development/test signer. Its private
// key is never exposed by this package.
type LocalPrivateKeySigner struct {
	mu      sync.RWMutex
	key     *ecdsa.PrivateKey
	address common.Address
}

func NewLocalPrivateKeySigner(key *ecdsa.PrivateKey) (*LocalPrivateKeySigner, error) {
	if key == nil {
		return nil, fmt.Errorf("private key is nil")
	}
	return &LocalPrivateKeySigner{key: key, address: crypto.PubkeyToAddress(key.PublicKey)}, nil
}
func NewLocalPrivateKeySignerFromHex(hexKey string) (*LocalPrivateKeySigner, error) {
	key, err := crypto.HexToECDSA(trim0x(hexKey))
	if err != nil {
		return nil, fmt.Errorf("parse private key: %w", err)
	}
	return NewLocalPrivateKeySigner(key)
}
func trim0x(s string) string {
	if len(s) >= 2 && (s[:2] == "0x" || s[:2] == "0X") {
		return s[2:]
	}
	return s
}
func (s *LocalPrivateKeySigner) Address() common.Address { return s.address }
func (s *LocalPrivateKeySigner) SignDigest(ctx context.Context, digest Digest) (Signature, error) {
	if err := ctx.Err(); err != nil {
		return Signature{}, err
	}
	s.mu.RLock()
	key := s.key
	s.mu.RUnlock()
	if key == nil {
		return Signature{}, fmt.Errorf("local signer is closed")
	}
	raw, err := crypto.Sign(digest[:], key)
	if err != nil {
		return Signature{}, err
	}
	var sig Signature
	copy(sig.R[:], raw[:32])
	copy(sig.S[:], raw[32:64])
	sig.V = raw[64]
	return sig, nil
}

// Close releases the signer's direct key reference. Go cannot guarantee that
// every private-key copy held by the runtime has been erased.
func (s *LocalPrivateKeySigner) Close() error {
	s.mu.Lock()
	s.key = nil
	s.mu.Unlock()
	return nil
}
