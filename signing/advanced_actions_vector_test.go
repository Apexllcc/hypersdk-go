package signing_test

import (
	"encoding/hex"
	"testing"

	"github.com/Apexllcc/hyperliquid-go-sdk/signer"
	"github.com/Apexllcc/hyperliquid-go-sdk/signing"
	"github.com/ethereum/go-ethereum/common"
)

// These vectors were generated independently with the official Python SDK's
// EIP-712 schemas and eth_account using the same fixed private key as the
// other signing fixtures.
func TestAdvancedUserActionVectorsMatchOfficialSchemas(t *testing.T) {
	const nonce = uint64(1_700_000_000_000)
	cases := []struct {
		name, digest, r, s string
		v                  uint8
		action             signing.UserSignedAction
	}{
		{"user DEX abstraction", "cb42fe03bc7e2ab6bfcffc29f185f14389ade041c5c4b055b2dca2d691e2128a", "f3807b2b5899681169d919594a0079205c189cbeb16425b05f8135f0489d5f79", "095e3874db9b65cce2f5f5704f31f4161ca42d526809ea61643ac4224a0dd7ef", 1, signing.UserDexAbstractionAction{User: common.HexToAddress("0x2222222222222222222222222222222222222222"), Enabled: true, Nonce: nonce}},
		{"user set abstraction", "8399767f98048bda03faf6fb8b3ffaa05909cba6fd50ffd4094ef33dcc7c2683", "23e702c3503e3a843fb0bcf42f34c33239bec71a28a3f28019d681401cad3dbe", "03f0a2d651e9a248fa567821ceb1b7e1dd5928df402170d7c33d7b94f400c1d3", 1, signing.UserSetAbstractionAction{User: common.HexToAddress("0x2222222222222222222222222222222222222222"), Abstraction: "unifiedAccount", Nonce: nonce}},
	}
	local := advancedFixtureSigner(t)
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			digest, err := signing.ComputeUserActionDigest(tc.action, true)
			if err != nil {
				t.Fatal(err)
			}
			assertSignedVector(t, local, digest, tc.digest, tc.r, tc.s, tc.v)
		})
	}
}

// These L1 vectors pin MsgPack bytes, connection IDs, Agent digests, R/S/V,
// and recovered address for the advanced L1 action family.
func TestAdvancedL1ActionVectorsMatchOfficialSchemas(t *testing.T) {
	const nonce = uint64(1_700_000_000_000)
	cases := []struct {
		name, actionBytes, connection, digest, r, s string
		v                                           uint8
		action                                      any
	}{
		{"agent enable DEX abstraction", "81a474797065b96167656e74456e61626c654465784162737472616374696f6e", "22fcc93b98d265d62f3ffe5ec83f14d24a85bc3ed0c003615bf57aa0243101f1", "d63c88ca2fe10d557a7c385061780d2d0c3ec4a80dd0d249f4d84e1ee8a8639e", "6f4c06bd5aacfbfb98a877bf9058c8483ded16c8051e9e71ecce38e83963b551", "454486824bc1e47e295b895f8e46f8a359149a0a71bd76e507faab59f86b6cc2", 1, signing.AgentEnableDexAbstractionAction{}},
		{"agent set abstraction", "82a474797065b36167656e745365744162737472616374696f6eab6162737472616374696f6ea170", "0ada403c8671a0609f286ae38ab44dad087a517eb737f31d814bc65a521e44bd", "db3417cbe90b87372907f6aa6d686e63f409f875999e77e80196de621542b323", "9f6efb03084eb5beee5df42b6ee0617515841737d19555196b9343c90e1297e8", "7fda70d915f3ab2912aa712503d4cb8ba6e652b857576503e596be6eaf415380", 1, signing.AgentSetAbstractionAction{Abstraction: "p"}},
		{"validator L1 stream", "82a474797065b176616c696461746f724c3153747265616dac7269736b4672656552617465a4302e3034", "5212e09bfe86d6bd5d9697fc8decae329a087a2dba8cd154fc587d2e33fa478b", "7d0ace164a6f05883e26fd6e3443922a32edbeb89598ff036b45d69528706a44", "7535adfe28ccb116b3a76c610d61098dffb2571716b0c9ad5d154c2ce2dd8920", "6bd3cace8ea9cc002a841b37dc9626569707b24578f097d30a3c46092154f742", 0, signing.ValidatorL1StreamAction{RiskFreeRate: "0.04"}},
		{"claim rewards", "81a474797065ac636c61696d52657761726473", "51aaf1b5200df7fd6fe4d7b9695b49a19d5fedb54caccfd6005949da3fbdd50a", "a47814c26a1380037fca6cae838eecfec7a7e336e81d7271d12a7f3dc1d092dd", "f7a4d8dcf32317c538466c0f698fe57afdd22aa459e6df17a2c649d54f7e6058", "454fcc7fdcf5f5765e7b3038e2ec62b6465107f8567af0ec75be1206ac756a79", 1, signing.ClaimRewardsAction{}},
	}
	local := advancedFixtureSigner(t)
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			components, err := signing.L1ActionComponents(tc.action, nonce, nil, nil)
			if err != nil {
				t.Fatal(err)
			}
			if got := hex.EncodeToString(components.ActionBytes); got != tc.actionBytes {
				t.Fatalf("action bytes=%s", got)
			}
			if got := hex.EncodeToString(components.ConnectionID[:]); got != tc.connection {
				t.Fatalf("connection=%s", got)
			}
			digest, err := signing.ComputeL1ActionDigest(tc.action, nonce, nil, nil, true)
			if err != nil {
				t.Fatal(err)
			}
			assertSignedVector(t, local, digest, tc.digest, tc.r, tc.s, tc.v)
		})
	}
}

func advancedFixtureSigner(t *testing.T) *signer.LocalPrivateKeySigner {
	t.Helper()
	local, err := signer.NewLocalPrivateKeySignerFromHex("0123456789012345678901234567890123456789012345678901234567890123")
	if err != nil {
		t.Fatal(err)
	}
	return local
}
