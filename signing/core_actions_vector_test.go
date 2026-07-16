package signing_test

import (
	"context"
	"encoding/hex"
	"testing"

	"github.com/Apexllcc/hyperliquid-go-sdk/signer"
	"github.com/Apexllcc/hyperliquid-go-sdk/signing"
	"github.com/ethereum/go-ethereum/common"
)

// These L1 fixtures were generated with Hyperliquid's official Python SDK
// action_hash, l1_payload, and eth_account signing implementation.
func TestCoreL1ActionVectorsMatchOfficialPythonSDK(t *testing.T) {
	const nonce = uint64(1_700_000_000_000)
	cases := []struct {
		name, actionBytes, connection, digest, r, s string
		v                                           uint8
		action                                      any
	}{
		{"update leverage", "84a474797065ae7570646174654c65766572616765a5617373657400a7697343726f7373c3a86c6576657261676505", "4a2f0def3cccafc69efda4409db55f1144941a44fb0deecd2687ecd80db99c2e", "e8c85aa61b09b5af31623825a905dea6ef756f2e925b3bf19dd431c38927c2f1", "59856938ebc8b58127a0c2bfc4bbc88f81d5faec69beefdf05beeae649054a3f", "124a9e3dcb25662880e64a23bcb8accbb2e88e8986903fa39f8847b44a96c26e", 0, signing.UpdateLeverageAction{Asset: 0, IsCross: true, Leverage: 5}},
		{"update isolated margin", "84a474797065b475706461746549736f6c617465644d617267696ea5617373657400a56973427579c2a46e746c69d2ffeced30", "77d0966d55ddd00d0fb8e933e7204615eff2d41efba619ee0e4b6b1ddab84a6f", "d06b0b10f043a83052108307c418b1b47f91663b0fae2122702430ce2dcf2824", "fac82679adc8ebf29f858588f1cfa7992c569c32ff97d709da57732af910fd04", "3fa4febd8e7e874f942b0a1427dbae93f24b11841bfa0964ab48538774193752", 1, signing.UpdateIsolatedMarginAction{Asset: 0, IsBuy: false, NTLI: -1_250_000}},
		{"top up isolated only margin", "83a474797065b7746f70557049736f6c617465644f6e6c794d617267696ea5617373657400a86c65766572616765a3322e35", "8d346d7234e9b47413eb31a325841aa77debf0a747643327e7d4d6c56b826165", "2fe9d1d35beab862b601e0fa2eea9b05855388f048a68002c691b23a7dab07a1", "e4cfe0abc973d30cb9e783e444ce63ae1701d96a125866249189c1906843e548", "00dbc2166f7737c6815998b88ab97aaf73cd41e26541339fac380219614f60d7", 1, signing.TopUpIsolatedOnlyMarginAction{Asset: 0, Leverage: "2.5"}},
		{"reserve request weight", "82a474797065b47265736572766552657175657374576569676874a67765696768740a", "c52c2d4aaf97e879278e9fba20ea7903fd6b949c3b3463e81c4873aca3a2841e", "86cff0e1d39b2317807e001fe5dd9e9d8c0a5a92ff7ec28d6e23c26d66d8a414", "253d20ee6d467b6ba1046abb0545624697aef893a065d601aa0c2c4c0b3319b8", "41ed5806f0840a17aac102a4f796ae4cd60953a7f6aaf517e899e645ded8ac6f", 0, signing.ReserveRequestWeightAction{Weight: 10}},
		{"noop", "81a474797065a46e6f6f70", "ef5dcef9775ebb2c5a6553314e66a6a57bd7e9b2319a869a8b17f08fa48bdcaf", "7ed74d02e0b55db498130e0462e3fa5856adf83d577a9157c8da3453fc03be6d", "a094d7afdcaffc2e2643f31df05a7594de12880e6558e17021d863c868a06972", "2ec74c7efddc03c04b04a660effe31f52ce3cddd2e0b80c68486ec772ca42ce2", 1, signing.NoopAction{}},
	}
	local, err := signer.NewLocalPrivateKeySignerFromHex("0123456789012345678901234567890123456789012345678901234567890123")
	if err != nil {
		t.Fatal(err)
	}
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
				t.Fatalf("connection ID=%s", got)
			}
			digest, err := signing.ComputeL1ActionDigest(tc.action, nonce, nil, nil, true)
			if err != nil {
				t.Fatal(err)
			}
			assertSignedVector(t, local, digest, tc.digest, tc.r, tc.s, tc.v)
		})
	}
}

// These fixtures were independently generated with the current official
// hyperliquid-python-sdk action_hash/l1_payload code and eth-account. The
// action-byte and connection-ID values are also checked against that Python
// implementation so a MsgPack map-order regression cannot silently pass.
func TestCompatibilityL1ActionVectorsMatchOfficialPythonSDK(t *testing.T) {
	const nonce = uint64(1_700_000_000_123)
	expires := uint64(1_700_000_000_999)
	cases := []struct {
		name, actionBytes, connection, digest, r, s string
		v                                           uint8
		action                                      any
		expires                                     *uint64
	}{
		// Generated with hyperliquid-python-sdk Exchange.c_signer_inner("jailSelf")
		// and its action_hash/sign_l1_action implementation.
		{"validator signer jail", "82a474797065ad435369676e6572416374696f6ea86a61696c53656c66c0", "146d5b72034aa4f8c125a8c47a48e7eb7085628d2ccdb7a968774083ae70916d", "ddbc3849d92dfe2fb691175465e25a82090ccc9a1999b91fa4a1c346a75017a9", "db20b18edb68fcacd0a98889721ee46574fb24740152f1ed237cde90f39eda43", "698ae927246149665b9d2537cac153262359913b9990910f9bc5bfe65f7f43c9", 1, signing.CSignerAction{Variant: signing.CSignerJailSelf{}}, &expires},
		{"validator signer unjail", "82a474797065ad435369676e6572416374696f6eaa756e6a61696c53656c66c0", "5222acb3360295857cb8a426aea1c46a2fb2de66580fa69db48e0ecfd8bf9364", "d947df21bba30fa00da174562817fe150321ee375201c5d788f8accc04ba218d", "4a19ab01735041ede9c779641cea9751dc866f11811c1bae38adeb74dcab31bb", "339cb0d832cbed7239f0c32adffe7a9f523110e72ddd8d6cd03018ce698add74", 0, signing.CSignerAction{Variant: signing.CSignerUnjailSelf{}}, &expires},
		{"evm user modify", "82a474797065ad65766d557365724d6f64696679ae7573696e67426967426c6f636b73c3", "88f68512ff2b497e32cbee1f8d4f933bb79efb845a753a67a0123aae2aaf239e", "3750ee50d29e0871732f55806abb8d0ae7b88b4d5a6615bfdefa62bd67ee0962", "5f96047d4056a7b26be49950dc0c8d2a3698d0ed76b14dfc3b34fb4daef37919", "0c34fc915498962dc8e2d881f76a9729efb76974e26bc3138a4e284d2e2564e8", 0, signing.EVMUserModifyAction{UsingBigBlocks: true}, nil},
		{"gossip priority bid", "84a474797065b1676f737369705072696f72697479426964a6736c6f74496400a26970a7312e322e332e34a66d6178476173ce05f5e100", "1d11236f151a0135eac14b8b4ae94d44d872616af556b6eb03d18e7abf92e844", "26a40f94effdd0b6999374ca8989dbfecc51ff2ac5b9eea1f0f1307962f914c6", "3522a4f1db5b1373eff7adeb30264f103ee942c90f4178f2ee9ddd272e6e46a9", "4e98367da1b26d6b2840d0635eecefee2829953bc652605b0f570a352b6f7992", 0, signing.GossipPriorityBidAction{SlotID: 0, IP: "1.2.3.4", MaxGas: 100_000_000}, nil},
		{"validator unregister", "82a474797065b04356616c696461746f72416374696f6eaa756e7265676973746572c0", "304e6c3168fe24a2ac3643fcac716369aff1aa4cc158da19b4ed092b058bf30f", "34ac4c4103bde539a21cdb0a4c6f7a833c79519cbe4abde0932499dbb2aee940", "ecb0591c5feab1633313b6d57fc7354cad1d997fe4901cc58e9c220dc961e66c", "6b83f82e0d01113517767d9d3f4c7cc7502c0c9cf9e3f04e3933b45ce304abcb", 0, signing.CValidatorAction{Variant: signing.CValidatorUnregister{}}, &expires},
		{"validator register", "82a474797065b04356616c696461746f72416374696f6ea8726567697374657283a770726f66696c6586a76e6f64655f697081a24970a7312e322e332e34a46e616d65a976616c696461746f72ab6465736372697074696f6eab6465736372697074696f6eb464656c65676174696f6e735f64697361626c6564c2ae636f6d6d697373696f6e5f6270730aa67369676e6572d92a307832323232323232323232323232323232323232323232323232323232323232323232323232323232a8756e6a61696c6564c3ab696e697469616c5f776569ce05f5e100", "0e290f18b6f24489fd5eb6f36dde5a87eb29d210c582515fd5299f16b94c525f", "22da93f0aa81cdeb0083a2d4c46b008930933b276af66baf9866e58b8add33b8", "3b089ffd5271e7ae296e6c67676a203428ba270dce3828beee324f4126606a3d", "555882566027715e47472c9b5265d12f90e041b73436c3234c38708658e986bb", 0, signing.CValidatorAction{Variant: signing.CValidatorRegister{Profile: signing.CValidatorProfile{NodeIP: "1.2.3.4", Name: "validator", Description: "description", CommissionBPS: 10, Signer: common.HexToAddress("0x2222222222222222222222222222222222222222")}, Unjailed: true, InitialWei: 100_000_000}}, nil},
		{"validator change profile", "82a474797065b04356616c696461746f72416374696f6ead6368616e676550726f66696c6587a76e6f64655f6970c0a46e616d65c0ab6465736372697074696f6ec0a8756e6a61696c6564c3b364697361626c655f64656c65676174696f6e73c0ae636f6d6d697373696f6e5f627073c0a67369676e6572c0", "968694573f1f851940f5f88a01a372c6e9ec5c6e5e563e31027916377d1b40c2", "6388d6b1652b9946768c039e30607d095141a73d7c1ecbc80c5b2dbc9d4aa57b", "e878be67cf39d2e0a2aa8e959bb322ad9297dc5908fa66714af29f1e2ba5afab", "2767d377cce1e756b06bc7ef2ee00fd8ba9073d8ceaaa9074096ec79a5352d23", 0, signing.CValidatorAction{Variant: signing.CValidatorChangeProfile{Unjailed: true}}, nil},
		{"finalize EVM create", "83a474797065b366696e616c697a6545766d436f6e7472616374a5746f6b656eccc8a5696e70757481a663726561746581a56e6f6e636500", "04f6627b3ef4706857bd66de0e9403712e169301d256a386837b41d5951a9ada", "4a6d0a14979fee7cecedc41cc201a02481d1da73cfca69144f7dc37e91a342fd", "4fcb7127812eb1040b19a137b9d0177ab5e160373233167b472783a5a9c577ba", "3430b988f3226d54b98774da85b9df222b7bce653797490be643c2ccbe1ba183", 1, signing.FinalizeEVMContractAction{Token: 200, Input: signing.FinalizeEVMCreate{Nonce: 0}}, nil},
		{"finalize EVM first storage slot", "83a474797065b366696e616c697a6545766d436f6e7472616374a5746f6b656eccc8a5696e707574b0666972737453746f72616765536c6f74", "4dd7a9b9534978f7846dd3c27980e7e7bb87f2a51b132b9f733c537d78f6511b", "751ef3a95858d6e6dc34f180f9000c99c4c2b06f6fb2af7190b8c2f4102ee540", "d8ea1e0d4a90b211ee1f233ebd53d815c8f169cee909cf02e436c7ebbf6a678f", "204203f7e6fad80221d98bffda15521cd16f3473a31a56ec9be7f97ffc333e98", 1, signing.FinalizeEVMContractAction{Token: 200, Input: signing.FinalizeEVMFirstStorageSlot{}}, nil},
		{"finalize EVM custom storage slot", "83a474797065b366696e616c697a6545766d436f6e7472616374a5746f6b656eccc8a5696e707574b1637573746f6d53746f72616765536c6f74", "6ca5378cfcf4053e77f4882620bfe2478087516df9cb56237ffd35ee8ab4118d", "5d8d25335c2590e63507c40e7b8696b579467b0c7b9011e4e941ce91805ef033", "7be96dd96dd9feb051c2af9526c143eae8d60fa734776b151070e70daec9dff0", "70e51a14d68820621a0922d607f52e2973af21529d0d67072c5b7127fdc96e7d", 0, signing.FinalizeEVMContractAction{Token: 200, Input: signing.FinalizeEVMCustomStorageSlot{}}, nil},
	}
	local, err := signer.NewLocalPrivateKeySignerFromHex("0123456789012345678901234567890123456789012345678901234567890123")
	if err != nil {
		t.Fatal(err)
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			components, err := signing.L1ActionComponents(tc.action, nonce, nil, tc.expires)
			if err != nil {
				t.Fatal(err)
			}
			if got := hex.EncodeToString(components.ActionBytes); got != tc.actionBytes {
				t.Fatalf("action bytes=%s", got)
			}
			if got := hex.EncodeToString(components.ConnectionID[:]); got != tc.connection {
				t.Fatalf("connection ID=%s", got)
			}
			digest, err := signing.ComputeL1ActionDigest(tc.action, nonce, nil, tc.expires, true)
			if err != nil {
				t.Fatal(err)
			}
			assertSignedVector(t, local, digest, tc.digest, tc.r, tc.s, tc.v)
		})
	}
}

func TestGossipPriorityBidRejectsBelowDocumentedMinimumAuctionPrice(t *testing.T) {
	_, err := (signing.GossipPriorityBidAction{
		SlotID: 0,
		IP:     "1.2.3.4",
		MaxGas: 9_999_999, // 0.1 HYPE is 10,000,000 wei.
	}).MarshalMsgpack()
	if err == nil {
		t.Fatal("expected a bid below the documented 0.1 HYPE minimum to be rejected")
	}
}

// These newer user-action fixtures use the official EIP-712 field definitions
// documented by Hyperliquid. The current official Python SDK does not expose
// constructors for cDeposit, cWithdraw, or sendToEvmWithData, so their values
// were independently generated with eth_account and the documented schemas.
func TestCoreUserActionVectorsMatchDocumentedEIP712Schemas(t *testing.T) {
	const nonce = uint64(1_700_000_000_000)
	cases := []struct {
		name, digest, r, s string
		v                  uint8
		action             signing.UserSignedAction
	}{
		{"send to EVM with data", "bf422169e0bd726a0d26f2158a757ac67088d1990d7ed1661d1a2898528bfb70", "252a6dc103a76bc592391b7df2c9388c910f6d35cc3568f337f5856120c73fdc", "58ecc69b63ce97cc2cd54480814c0e7d1733e952f3cc3504e3a845b38bb68105", 1, signing.SendToEVMWithDataAction{Token: "USDC", Amount: "1.25", DestinationRecipient: "0x2222222222222222222222222222222222222222", AddressEncoding: "hex", DestinationChainID: 42161, GasLimit: 200000, Data: []byte{1, 2}, Nonce: nonce}},
		{"c deposit", "ae33421c367d1a2aa2cd0c1d0a88675b0d85ebc5133df4038fc3b242e5763253", "009e05d03871ee070fba04df5287eee8fd0de5ed259809bc179565b3bbd949a4", "5b961f85ff278a5e473269f91a2020b0c129545feb80528438c06631186dccba", 0, signing.CDepositAction{Wei: 100_000_000, Nonce: nonce}},
		{"c withdraw", "0f3731764f8fe4519a0588c6fcc81fe0c2c3daa02b9da087904a6493967fcac4", "e75751e34d0a6f080b76cb43f96ea9ea85b33a346ff773672e98f2925f27a4c4", "1ca097754433e19821f250c86d76bf486884edc70b7198797e9b6fd506b62827", 1, signing.CWithdrawAction{Wei: 100_000_000, Nonce: nonce}},
		{"token delegate", "6b1673522ae205bbd66c00c51d24a9a70037d2b5b3be2f38bf74d547a8106d1c", "cef6252ce69b63b24f4a21c83a86e951b71e749d51a18aa1af3dc1271c29c586", "5987a755fb36ea8635b3dc3b9957986127ad5079c14a48e674caca3440f0a7d6", 0, signing.TokenDelegateAction{Validator: common.HexToAddress("0x3333333333333333333333333333333333333333"), Wei: 100_000_000, Nonce: nonce}},
	}
	local, err := signer.NewLocalPrivateKeySignerFromHex("0123456789012345678901234567890123456789012345678901234567890123")
	if err != nil {
		t.Fatal(err)
	}
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

func assertSignedVector(t *testing.T, local *signer.LocalPrivateKeySigner, digest signer.Digest, wantDigest, wantR, wantS string, wantV uint8) {
	t.Helper()
	if got := hex.EncodeToString(digest[:]); got != wantDigest {
		t.Fatalf("digest=%s", got)
	}
	signature, err := local.SignDigest(context.Background(), digest)
	if err != nil {
		t.Fatal(err)
	}
	if got := hex.EncodeToString(signature.R[:]); got != wantR {
		t.Fatalf("R=%s", got)
	}
	if got := hex.EncodeToString(signature.S[:]); got != wantS {
		t.Fatalf("S=%s", got)
	}
	if signature.V != wantV {
		t.Fatalf("V=%d", signature.V)
	}
	if err := signer.Verify(digest, signature, local.Address()); err != nil {
		t.Fatalf("recover signer address: %v", err)
	}
}
