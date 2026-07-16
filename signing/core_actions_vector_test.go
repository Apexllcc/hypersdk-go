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
