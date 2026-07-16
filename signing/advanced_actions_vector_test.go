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
		{"agent send asset", "88a474797065ae6167656e7453656e644173736574ab64657374696e6174696f6ed92a307832323232323232323232323232323232323232323232323232323232323232323232323232323232a9736f75726365446578a0ae64657374696e6174696f6e446578a473706f74a5746f6b656ea8555344433a307831a6616d6f756e74a4312e3235ae66726f6d5375624163636f756e74a0a56e6f6e6365cf0000018bcfe56800", "da9befd255b8119052c695b87d6330c5bbadc03a632830bd3445d9495ca69dfb", "6f8d648aef30ae47a49027fea465f7400519d88a01a2352943c824e91000c54b", "25a2e9ae55570fcb15cfdfc53e69c95e26797b5c1ca2dc491f2550f0604957f7", "4650ee122bc83004a97909deccdc548bee9be7db3804ba5746a795758ec7cb2e", 1, signing.AgentSendAssetAction{Destination: "0x2222222222222222222222222222222222222222", SourceDEX: "", DestinationDEX: "spot", Token: "USDC:0x1", Amount: "1.25", FromSubAccount: "", Nonce: nonce}},
		{"authorize AQA v2 role", "83a474797065b2617574686f72697a654171617632526f6c65a5746f6b656e00a4726f6c65a9746563686e6963616c", "64c58a3d0f9cf20b00126b1833e0aec30b94103c7e2d0bc87aacf790251fda85", "e370aa32e49a01af9958ff3787fe5b05b17fa8a5a91b7b48225aecf0aaa2e979", "3adf47e3f26db1bc771cb0c6533e8b914429bafbd878153fd86d7c32546e94a3", "21c8dccd40a906b356b970c82029ac0ce129d1220b6bd10a4b592d3c242cf684", 1, signing.AuthorizeAQAV2RoleAction{Token: 0, Role: "technical"}},
		{"HIP-3 liquidator transfer", "84a474797065b6686970334c697175696461746f725472616e73666572a3646578a378797aa36e746cce3b9aca00a969734465706f736974c3", "b45ea4ee980a8208c40f3f59d26757b3b955caa60acdc39199639d9e5198aae2", "052628656980ccf16ad213db29412a9ceb36a25d25a688d3bc5fcbc9c3ef7c22", "bf8ae983d982802404c70f4feec4f28aea9f75d904d143cb1bfcd7e8902884fe", "4b2605a574a3c0859eea9aaa9f0173a41602476edff17845567d33747741968f", 0, signing.HIP3LiquidatorTransferAction{DEX: "xyz", NTL: 1_000_000_000, IsDeposit: true}},
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

func TestUserOutcomeL1ActionVectorsMatchOfficialSchemas(t *testing.T) {
	const nonce = uint64(1_700_000_000_000)
	amount := "2.5"
	cases := []struct {
		name, actionBytes, connection, digest, r, s string
		v                                           uint8
		action                                      signing.UserOutcomeAction
	}{
		{"split", "82a474797065ab757365724f7574636f6d65ac73706c69744f7574636f6d6582a76f7574636f6d6507a6616d6f756e74a4312e3235", "48033a5c877c841a8f4d2468e2ff0bf298f25dd3e30e2f46f2bffe56b384a9cc", "7be9bdb13577a9b849760cb52e8ca6e395c22998805b842f377ae1d0e71dfa0d", "a748bdd69aa8d33249ccdf47101d9c007a6f764dae6478ba469b473c074324ec", "073181ff3882ab12c96dc6bb5b6f8937d5acd4d3a6719c46394aa33c473b5223", 0, signing.UserOutcomeAction{SplitOutcome: &signing.SplitOutcome{Outcome: 7, Amount: "1.25"}}},
		{"merge outcome all", "82a474797065ab757365724f7574636f6d65ac6d657267654f7574636f6d6582a76f7574636f6d6507a6616d6f756e74c0", "1c90918a16c7e81caca87bc7dc2dcaf980beebb9d0b481cf73156fa9a4e75738", "495c0a7f2dd706efea682f43ed2f238e7b1a54c4a4cea9b28d0fd47360c248ee", "39e411ef28f359648768bd56c0baff120a44fb8a13c75f51abd9ee5b8fcd6fc0", "2b826a5b9193ce232752d0f0f51c25a8050d773457121834e18c421b254f711a", 1, signing.UserOutcomeAction{MergeOutcome: &signing.MergeOutcome{Outcome: 7, Amount: nil}}},
		{"merge question", "82a474797065ab757365724f7574636f6d65ad6d657267655175657374696f6e82a87175657374696f6e09a6616d6f756e74a3322e35", "914a3a82b367858115523a4be905cb9446b7832dca9986a3c9c0ac932930d77e", "3d60a96f2ed16e966e04e43a5721e6af13f8077f320e578433e25043e79d8337", "762e90a57577c804cddbc6b02fa7cde3c48dad1ce009cb99310d9d70d1f1b563", "6aa5065a4c42835806c14af5df3a9567a83fbbc786b3778337351d004e1c7440", 1, signing.UserOutcomeAction{MergeQuestion: &signing.MergeQuestion{Question: 9, Amount: &amount}}},
		{"negate", "82a474797065ab757365724f7574636f6d65ad6e65676174654f7574636f6d6583a87175657374696f6e09a76f7574636f6d6507a6616d6f756e74a133", "3e49d1d1e9c015b447871a0ffc8df87118d71cb6b46ca716090427987ca6353c", "86cc300aad5fe80a559666aba94ce0faab30f29d86b9c1d247188f0f9a7f4b79", "efddd03d14f6d0bad0bf367cee2badbaf38652e450b781eaa3743e772d0c19b8", "3002c168c5b08b88b4a5afe64a9ac15cc21291e5ece4bf626379243bc77bc71d", 0, signing.UserOutcomeAction{NegateOutcome: &signing.NegateOutcome{Question: 9, Outcome: 7, Amount: "3"}}},
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
