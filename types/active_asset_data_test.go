package types_test

import (
	"encoding/json"
	"testing"

	"github.com/Apexllcc/hypersdk-go/types"
)

func TestActiveAssetDataResponseDecodesOfficialPayload(t *testing.T) {
	const payload = `{
		"user":"0xabc",
		"coin":"BTC",
		"leverage":{"type":"cross","value":3,"rawUsd":"1.25"},
		"maxTradeSzs":["2.5","3.5"],
		"availableToTrade":["4.5","5.5"],
		"markPx":"60000.25"
	}`

	var got types.ActiveAssetDataResponse
	if err := json.Unmarshal([]byte(payload), &got); err != nil {
		t.Fatalf("unmarshal active asset data: %v", err)
	}
	if got.User != "0xabc" || got.Coin != "BTC" {
		t.Fatalf("identity = (%q, %q), want (0xabc, BTC)", got.User, got.Coin)
	}
	if got.Leverage.Type != "cross" || got.Leverage.Value != 3 || got.Leverage.RawUsd == nil || got.Leverage.RawUsd.String() != "1.25" {
		t.Fatalf("leverage = %#v, want cross 3 / 1.25", got.Leverage)
	}
	if got.MaxTradeSizes[0].String() != "2.5" || got.AvailableToTrade[1].String() != "5.5" || got.MarkPx.String() != "60000.25" {
		t.Fatalf("economic fields = %#v, want exact decimal values", got)
	}
}
