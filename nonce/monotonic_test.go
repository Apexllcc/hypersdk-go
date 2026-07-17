package nonce_test

import (
	"context"
	"testing"
	"time"

	"github.com/Apexllcc/hypersdk-go/nonce"
	"github.com/ethereum/go-ethereum/common"
)

func TestMonotonicManagerAdvancesWhenClockDoesNot(t *testing.T) {
	t.Parallel()
	m := nonce.NewMonotonicManager(func() time.Time { return time.UnixMilli(100) })
	address := common.HexToAddress("0x1")
	first, err := m.Next(context.Background(), address)
	if err != nil {
		t.Fatal(err)
	}
	second, err := m.Next(context.Background(), address)
	if err != nil {
		t.Fatal(err)
	}
	if first != 100 || second != 101 {
		t.Fatalf("got %d, %d", first, second)
	}
}
