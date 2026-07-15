package types_test

import (
	"encoding/json"
	"testing"

	"github.com/Apexllcc/hyperliquid-go-sdk/types"
)

func TestCloidRoundTrip(t *testing.T) {
	t.Parallel()
	c, err := types.ParseCloid("0x1234567890abcdef1234567890abcdef")
	if err != nil {
		t.Fatal(err)
	}
	if got := c.String(); got != "0x1234567890abcdef1234567890abcdef" {
		t.Fatalf("got %s", got)
	}
	encoded, err := json.Marshal(c)
	if err != nil {
		t.Fatal(err)
	}
	var decoded types.Cloid
	if err := json.Unmarshal(encoded, &decoded); err != nil {
		t.Fatal(err)
	}
	if decoded != c {
		t.Fatal("JSON round trip changed cloid")
	}
}
