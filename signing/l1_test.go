package signing_test

import (
	"encoding/hex"
	"testing"

	"github.com/Apexllcc/hyperliquid-go-sdk/signing"
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
