// Package types contains shared, protocol-safe public domain types.
package types

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
)

// Cloid is Hyperliquid's 128-bit client order identifier.
type Cloid [16]byte

// NewCloid returns a cryptographically random CLOID.
func NewCloid() (Cloid, error) {
	var c Cloid
	if _, err := rand.Read(c[:]); err != nil {
		return Cloid{}, fmt.Errorf("generate cloid: %w", err)
	}
	return c, nil
}

// ParseCloid parses exactly 16 bytes of 0x-prefixed hexadecimal text.
func ParseCloid(raw string) (Cloid, error) {
	if len(raw) != 34 || raw[:2] != "0x" {
		return Cloid{}, fmt.Errorf("invalid cloid")
	}
	decoded, err := hex.DecodeString(raw[2:])
	if err != nil {
		return Cloid{}, fmt.Errorf("invalid cloid: %w", err)
	}
	var c Cloid
	copy(c[:], decoded)
	return c, nil
}
func (c Cloid) String() string               { return "0x" + hex.EncodeToString(c[:]) }
func (c Cloid) MarshalJSON() ([]byte, error) { return json.Marshal(c.String()) }
func (c *Cloid) UnmarshalJSON(data []byte) error {
	var raw string
	if err := json.Unmarshal(data, &raw); err != nil {
		return err
	}
	parsed, err := ParseCloid(raw)
	if err != nil {
		return err
	}
	*c = parsed
	return nil
}
