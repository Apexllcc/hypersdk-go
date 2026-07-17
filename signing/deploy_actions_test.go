package signing_test

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"testing"

	"github.com/Apexllcc/hypersdk-go/signer"
	"github.com/Apexllcc/hypersdk-go/signing"
	"github.com/ethereum/go-ethereum/common"
)

func TestDeployActionFixedVectorsMatchOfficialMessagePackRules(t *testing.T) {
	a := signing.PerpDeployAction{Variant: signing.RegisterAsset2{
		MaxGas: uint64ptr(0), DEX: "xyz",
		AssetRequest: signing.RegisterAssetRequest2{Coin: "xyz:BTC", SzDecimals: 5, OraclePx: "123.45", MarginTableID: 1, MarginMode: signing.MarginModeNoCross},
		Schema:       &signing.PerpDexSchemaInput{FullName: "XYZ Perps", CollateralToken: 0},
	}}
	c, err := signing.L1ActionComponents(a, 1_700_000_000_000, nil, uint64ptr(1_700_000_100_000))
	if err != nil {
		t.Fatal(err)
	}
	d, err := signing.ComputeL1ActionDigest(a, 1_700_000_000_000, nil, uint64ptr(1_700_000_100_000), true)
	if err != nil {
		t.Fatal(err)
	}
	local, err := signer.NewLocalPrivateKeySignerFromHex("0123456789012345678901234567890123456789012345678901234567890123")
	if err != nil {
		t.Fatal(err)
	}
	s, err := local.SignDigest(context.Background(), d)
	if err != nil {
		t.Fatal(err)
	}
	if got := hex.EncodeToString(c.ActionBytes); got != "82a474797065aa706572704465706c6f79ae726567697374657241737365743284a66d617847617300ac61737365745265717565737485a4636f696ea778797a3a425443aa737a446563696d616c7305a86f7261636c655078a63132332e3435ad6d617267696e5461626c65496401aa6d617267696e4d6f6465a76e6f43726f7373a3646578a378797aa6736368656d6183a866756c6c4e616d65a958595a205065727073af636f6c6c61746572616c546f6b656e00ad6f7261636c6555706461746572c0" {
		t.Fatalf("perp action bytes = %s", got)
	}
	if got := hex.EncodeToString(c.ConnectionID[:]); got != "9df0beba57b7bb19484763e81c42ef7409dd41427f95b99103bd2140e7801f95" {
		t.Fatalf("perp connection ID = %s", got)
	}
	if got := hex.EncodeToString(d[:]); got != "6ddbf8d207ea902daba4ad7c5f65533e21f32b6fb28a8a238efd5301ed3b63b1" {
		t.Fatalf("perp digest = %s", got)
	}
	if got := hex.EncodeToString(s.R[:]); got != "a43b1825292408d19d71c69002d3e552b34fb0e3ef034ad094564706d8e45b5f" {
		t.Fatalf("perp R = %s", got)
	}
	if got := hex.EncodeToString(s.S[:]); got != "7348a6991328f76fb7ac887734fa213487183e6430eed2f0dd6fa27cb21eff57" {
		t.Fatalf("perp S = %s", got)
	}
	if s.V != 1 {
		t.Fatalf("perp V = %d", s.V)
	}
	if err := signer.Verify(d, s, local.Address()); err != nil {
		t.Fatalf("perp recovered address: %v", err)
	}

	b := signing.SpotDeployAction{Variant: signing.UserGenesis{Token: 1000, UserAndWei: []signing.AddressWei{{User: "0x1111111111111111111111111111111111111111", Wei: "1000000"}}, ExistingTokenAndWei: []signing.TokenWei{{Token: 0, Wei: "2000000"}}}}
	c, err = signing.L1ActionComponents(b, 1_700_000_000_001, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	d, err = signing.ComputeL1ActionDigest(b, 1_700_000_000_001, nil, nil, false)
	if err != nil {
		t.Fatal(err)
	}
	s, err = local.SignDigest(context.Background(), d)
	if err != nil {
		t.Fatal(err)
	}
	if got := hex.EncodeToString(c.ActionBytes); got != "82a474797065aa73706f744465706c6f79ab7573657247656e6573697383a5746f6b656ecd03e8aa75736572416e645765699192d92a307831313131313131313131313131313131313131313131313131313131313131313131313131313131a731303030303030b36578697374696e67546f6b656e416e64576569919200a732303030303030" {
		t.Fatalf("spot action bytes = %s", got)
	}
	if got := hex.EncodeToString(c.ConnectionID[:]); got != "490176deb72fb292a9214514ef08e917ef83da53b5e2b7bc9e1e70823f093c79" {
		t.Fatalf("spot connection ID = %s", got)
	}
	if got := hex.EncodeToString(d[:]); got != "b10aac45f7c67ead4a0aaa520edaa411f3a8153c85c12e47cdccf99ebb629c94" {
		t.Fatalf("spot digest = %s", got)
	}
	if got := hex.EncodeToString(s.R[:]); got != "3f9812c564273ec1ead0345678999c9878972057848652f6b76c43c1fc1e1f14" {
		t.Fatalf("spot R = %s", got)
	}
	if got := hex.EncodeToString(s.S[:]); got != "7847128e99cff920d07077ca472b6885114f5ffbba944e293b63b0fd0ebce9e6" {
		t.Fatalf("spot S = %s", got)
	}
	if s.V != 1 {
		t.Fatalf("spot V = %d", s.V)
	}
	if err := signer.Verify(d, s, local.Address()); err != nil {
		t.Fatalf("spot recovered address: %v", err)
	}
}

func TestEveryOfficialDeployVariantHasOneTypedWireKey(t *testing.T) {
	perps := []signing.PerpDeployVariant{
		signing.RegisterAsset{DEX: "abc", AssetRequest: signing.RegisterAssetRequest{Coin: "abc:A", SzDecimals: 1, OraclePx: "1", MarginTableID: 1}},
		signing.RegisterAsset2{DEX: "abc", AssetRequest: signing.RegisterAssetRequest2{Coin: "abc:A", SzDecimals: 1, OraclePx: "1", MarginTableID: 1, MarginMode: signing.MarginModeNormal}},
		signing.SetOracle{DEX: "abc", OraclePxs: []signing.StringPair{{Key: "abc:A", Value: "1"}}, ExternalPerpPxs: []signing.StringPair{{Key: "abc:A", Value: "1"}}},
		signing.SetFundingMultipliers{Values: []signing.StringPair{{Key: "abc:A", Value: "1"}}},
		signing.SetFundingInterestRates{Values: []signing.StringPair{{Key: "abc:A", Value: "0.01"}}},
		signing.HaltTrading{Coin: "abc:A", IsHalted: true},
		signing.SetMarginTableIDs{Values: []signing.StringUintPair{{Key: "abc:A", Value: 1}}},
		signing.InsertMarginTable{DEX: "abc", MarginTable: signing.RawMarginTable{Description: "standard", MarginTiers: []signing.RawMarginTier{{LowerBound: 0, MaxLeverage: 50}}}},
		signing.SetFeeRecipient{DEX: "abc", FeeRecipient: "0x1111111111111111111111111111111111111111"},
		signing.SetOpenInterestCaps{Values: []signing.StringUintPair{{Key: "abc:A", Value: 1000000}}},
		signing.SetSubDeployers{DEX: "abc", SubDeployers: []signing.SubDeployerInput{{Variant: "setOracle", User: "0x1111111111111111111111111111111111111111", Allowed: true}}},
		signing.SetMarginModes{Values: []signing.CoinMarginMode{{Coin: "abc:A", Mode: signing.MarginModeNoCross}}},
		signing.SetFeeScale{DEX: "abc", Scale: "1"},
		signing.SetGrowthModes{Values: []signing.StringBoolPair{{Key: "abc:A", Value: true}}},
		signing.SetPerpAnnotation{Coin: "abc:A", Category: "test", Description: "desc", Keywords: []string{"a"}},
		signing.DisableDEX("abc"),
	}
	spots := []signing.SpotDeployVariant{
		signing.RegisterToken2{Spec: signing.TokenSpec{Name: "TOK", SzDecimals: 2, WeiDecimals: 8}, MaxGas: 1},
		signing.UserGenesis{Token: 1, UserAndWei: []signing.AddressWei{{User: "0x1111111111111111111111111111111111111111", Wei: "1"}}},
		signing.Genesis{Token: 1, MaxSupply: "1"},
		signing.RegisterSpot{Tokens: [2]uint64{1, 0}},
		signing.RegisterHyperliquidity{Spot: 1, StartPx: "1", OrderSz: "1", NOrders: 1},
		signing.SetDeployerTradingFeeShare{Token: 1, Share: "1%"},
		signing.EnableQuoteToken{Token: 1}, signing.DisableQuoteToken{Token: 1}, signing.EnableAlignedQuoteToken{Token: 1},
		signing.RequestEVMContract{Token: 1, Address: "0x1111111111111111111111111111111111111111", EVMExtraWeiDecimals: 13},
		signing.EnableFreezePrivilege{Token: 1}, signing.FreezeUser{Token: 1, User: "0x1111111111111111111111111111111111111111", Freeze: true}, signing.RevokeFreezePrivilege{Token: 1},
	}
	local, err := signer.NewLocalPrivateKeySignerFromHex("0123456789012345678901234567890123456789012345678901234567890123")
	if err != nil {
		t.Fatal(err)
	}
	h := sha256.New()
	nonce := uint64(20_000)
	for _, variant := range perps {
		nonce = assertFixedDeployVector(t, h, local, signing.PerpDeployAction{Variant: variant}, "perpDeploy", nonce)
	}
	for _, variant := range spots {
		nonce = assertFixedDeployVector(t, h, local, signing.SpotDeployAction{Variant: variant}, "spotDeploy", nonce)
	}
	if got := hex.EncodeToString(h.Sum(nil)); got != "ff1879aefd6cc473953c21a6619ca4f1f6599344a07fe2d92fdeb44253cb82d6" {
		t.Fatalf("all-variant fixed vector checksum = %s", got)
	}
}

func assertFixedDeployVector(t *testing.T, h interface{ Write([]byte) (int, error) }, local signer.DigestSigner, action any, wantType string, nonce uint64) uint64 {
	t.Helper()
	assertOneDeployKey(t, action, wantType)
	components, err := signing.L1ActionComponents(action, nonce, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	digest, err := signing.ComputeL1ActionDigest(action, nonce, nil, nil, true)
	if err != nil {
		t.Fatal(err)
	}
	sig, err := local.SignDigest(context.Background(), digest)
	if err != nil {
		t.Fatal(err)
	}
	if err := signer.Verify(digest, sig, local.Address()); err != nil {
		t.Fatalf("vector recovery: %v", err)
	}
	// One immutable checksum covers every byte of every per-variant action,
	// connection ID, digest, R/S/V, and recovered signer verification above.
	_, _ = h.Write(components.ActionBytes)
	_, _ = h.Write(components.ConnectionID[:])
	_, _ = h.Write(digest[:])
	_, _ = h.Write(sig.R[:])
	_, _ = h.Write(sig.S[:])
	_, _ = h.Write([]byte{sig.V})
	return nonce + 1
}

func TestDeployActionsRejectUnsafeOrAmbiguousInput(t *testing.T) {
	cases := []any{
		signing.PerpDeployAction{},
		signing.PerpDeployAction{Variant: signing.SetOracle{DEX: "abc", OraclePxs: []signing.StringPair{{Key: "b", Value: "1"}, {Key: "a", Value: "1"}}}},
		signing.PerpDeployAction{Variant: signing.SetFeeRecipient{DEX: "abc", FeeRecipient: "0x111111111111111111111111111111111111111g"}},
		signing.SpotDeployAction{},
		signing.SpotDeployAction{Variant: signing.FreezeUser{Token: 1, User: "not-an-address"}},
		signing.SpotDeployAction{Variant: signing.SetDeployerTradingFeeShare{Token: 1, Share: "101%"}},
	}
	for _, action := range cases {
		if _, err := signing.L1ActionComponents(action, 1, nil, nil); err == nil {
			t.Errorf("expected action validation error for %#v", action)
		}
	}
}

func TestDeployAddressInputsAreCanonicalizedBeforeSigning(t *testing.T) {
	checksum := "0x14791697260E4c9A71f18484C9f997B308e59325"
	action := signing.SpotDeployAction{Variant: signing.RequestEVMContract{Token: 1, Address: checksum, EVMExtraWeiDecimals: 1}}
	raw, err := json.Marshal(action)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Contains(raw, []byte("0x14791697260e4c9a71f18484c9f997b308e59325")) {
		t.Fatalf("checksum address was not normalized: %s", raw)
	}
	if _, err := signing.L1ActionComponents(action, 1, nil, nil); err != nil {
		t.Fatalf("normalized address should sign: %v", err)
	}
}

func TestDeployL1DigestExcludesOuterVaultButIncludesExpiry(t *testing.T) {
	action := signing.PerpDeployAction{Variant: signing.SetFeeScale{DEX: "abc", Scale: "1"}}
	expires := uint64(1_700_020_000_000)
	vault := common.HexToAddress("0x1111111111111111111111111111111111111111")
	withoutVault, err := signing.ComputeL1ActionDigest(action, 1_700_010_000_000, nil, &expires, true)
	if err != nil {
		t.Fatal(err)
	}
	withVault, err := signing.ComputeL1ActionDigest(action, 1_700_010_000_000, &vault, &expires, true)
	if err != nil {
		t.Fatal(err)
	}
	if withoutVault == withVault {
		t.Fatal("deployment digest must not silently include an outer vault marker")
	}
	if got := hex.EncodeToString(withoutVault[:]); got != "b753941111dc47169c29ff133bfedb9aaa44579cf51adc0565029d86e4ae1e6b" {
		t.Fatalf("nil-vault expiry digest = %s", got)
	}
}

func TestDeployAdditionalFixedVectors(t *testing.T) {
	items := []struct {
		action                          any
		bytes, connection, digest, r, s string
		v                               uint8
	}{
		{
			signing.PerpDeployAction{Variant: signing.RegisterAsset{DEX: "abc", AssetRequest: signing.RegisterAssetRequest{Coin: "abc:A", SzDecimals: 1, OraclePx: "1", MarginTableID: 1}}},
			"82a474797065aa706572704465706c6f79ad7265676973746572417373657484a66d6178476173c0ac61737365745265717565737485a4636f696ea56162633a41aa737a446563696d616c7301a86f7261636c655078a131ad6d617267696e5461626c65496401ac6f6e6c7949736f6c61746564c2a3646578a3616263a6736368656d61c0", "f7088d37fe0f04083822970d0b295650d15570bf44de01baf01182668ab23ed9", "5222096f483ddb7e05b2ce8acb7893666d5f5fd8cf6622f3589741fa221368d8", "03d086f9cc7d77ea455316cd4132eac3311c5ecfd3ca7daba3e3be744d047f8b", "70c83fdc66984a363b13b7a71e8c842a267477fe14725a5c9ee5227ecdd272ba", 0,
		}, {
			signing.PerpDeployAction{Variant: signing.SetOracle{DEX: "abc", OraclePxs: []signing.StringPair{{Key: "abc:A", Value: "1"}}, MarkPxs: [][]signing.StringPair{{{Key: "abc:A", Value: "1"}}}, ExternalPerpPxs: []signing.StringPair{{Key: "abc:A", Value: "1"}}}},
			"82a474797065aa706572704465706c6f79a97365744f7261636c6584a3646578a3616263a96f7261636c655078739192a56162633a41a131a76d61726b507873919192a56162633a41a131af65787465726e616c506572705078739192a56162633a41a131", "cbe28716935bd47a59b8e3466ddfed0a6b33a713501cc50de152286e9257a555", "0f18be20aed82c9e9693c7b19bbd4232bbe72aa3ed878fa072dc1661e2443288", "45e1ec31857c44f297f38a28239359fd1922f94384f15f45b57f7ffc59221d67", "060f3b68c038397e394433535247476546dfc8871744e07774c43e821f6ec3e9", 1,
		}, {
			signing.SpotDeployAction{Variant: signing.Genesis{Token: 1, MaxSupply: "100"}},
			"82a474797065aa73706f744465706c6f79a767656e6573697382a5746f6b656e01a96d6178537570706c79a3313030", "090b62d826485328203de2fafd3fd5a4bb7e8f45d689ccc7ab9c4b8f9ca294bb", "0581f29eb1d0cb2bcf28face95a040318dbd9ad1661185b1e1d6741ee6439120", "d62d78c769d6aebf9e71ffc571979f7f6e6d2204cc05e6050e0e493006665e37", "2a5224ed69ba293422cd6724d0c970e7d94bd1baf9bc6254bc243ba0fd66b9bf", 1,
		}, {
			signing.SpotDeployAction{Variant: signing.RegisterHyperliquidity{Spot: 1, StartPx: "1.5", OrderSz: "2", NOrders: 3, NSeededLevels: uint64ptr(1)}},
			"82a474797065aa73706f744465706c6f79b6726567697374657248797065726c697175696469747985a473706f7401a773746172745078a3312e35a76f72646572537aa132a76e4f726465727303ad6e5365656465644c6576656c7301", "da11072e43bbcffea4e76faa19bfc98625fef874a101b524b3cb0c7e7053fd71", "4b8144f7f0649e66fa3e020fce1fc6895034239510ed28b99a56997393d72bae", "7b956ab92d29afecc3f93cf32ebe78a537ee92303da9db32014ba9516dc16b1c", "4e6ac85cdcd2fa2d6ddb168a074e3e13e46061d7ed089781d3fb0f04fd9605b0", 1,
		}, {
			signing.SpotDeployAction{Variant: signing.RequestEVMContract{Token: 1, Address: "0x1111111111111111111111111111111111111111", EVMExtraWeiDecimals: -2}},
			"82a474797065aa73706f744465706c6f79b27265717565737445766d436f6e747261637483a5746f6b656e01a761646472657373d92a307831313131313131313131313131313131313131313131313131313131313131313131313131313131b365766d4578747261576569446563696d616c73fe", "9ab19537f2862681cde4136226b1351cae97c71226887fa453ac6ce4dca39708", "fe6c669fb45e26bb2b09f04067820ff683281fd04b2d37dd71242e663c967689", "f74c5db6f05c0ab55088c18575efa26cad666dc37d7bdc65e82cec597a1c67f1", "461a0b695fd5201e12ad1f13264b3d777db4f54e2b1ccdbc6554848fcf9190bc", 0,
		}, {
			signing.PerpDeployAction{Variant: signing.DisableDEX("abc")},
			"82a474797065aa706572704465706c6f79aa64697361626c65446578a3616263", "4fe809298199b1f8ee541a221481c283c5756147d2ec99b83fe465bd9ce4634c", "b2dd5d4bcaee9332deb23956d35896699c04e2d121da095273599f02d88dcb21", "865004e1ba2c5c6ebf48c5b77ff7fb4bf987c11a20b313c10f95d6affb373ed0", "5d2dc6cd165b70b63fb4b9385556f72f7e88f1bd62d3977b0936bd48904fd247", 0,
		}, {
			signing.SpotDeployAction{Variant: signing.DisableQuoteToken{Token: 1}},
			"82a474797065aa73706f744465706c6f79b164697361626c6551756f7465546f6b656e81a5746f6b656e01", "f05c80dfd1257b412e85d55d2d8478484d568a35b6221b11fb6706f3324bee69", "4836ede70157c8719f8bd741bcfe02f4fbd90b73e228285a782b4acb8f532b9a", "caac36b26c6d135bdc0dfe054ae66c6185b7453d2bdd6429a31ef74bc4b61e36", "3ec61b440b39c4491720ddca99784588cd024bef28b6103b6fd760567926d06e", 1,
		},
	}
	local, err := signer.NewLocalPrivateKeySignerFromHex("0123456789012345678901234567890123456789012345678901234567890123")
	if err != nil {
		t.Fatal(err)
	}
	for i, item := range items {
		c, err := signing.L1ActionComponents(item.action, uint64(100+i), nil, nil)
		if err != nil {
			t.Fatal(err)
		}
		d, err := signing.ComputeL1ActionDigest(item.action, uint64(100+i), nil, nil, true)
		if err != nil {
			t.Fatal(err)
		}
		s, err := local.SignDigest(context.Background(), d)
		if err != nil {
			t.Fatal(err)
		}
		if got := hex.EncodeToString(c.ActionBytes); got != item.bytes {
			t.Fatalf("%d action bytes = %s", i, got)
		}
		if got := hex.EncodeToString(c.ConnectionID[:]); got != item.connection {
			t.Fatalf("%d connection = %s", i, got)
		}
		if got := hex.EncodeToString(d[:]); got != item.digest {
			t.Fatalf("%d digest = %s", i, got)
		}
		if got := hex.EncodeToString(s.R[:]); got != item.r {
			t.Fatalf("%d R = %s", i, got)
		}
		if got := hex.EncodeToString(s.S[:]); got != item.s {
			t.Fatalf("%d S = %s", i, got)
		}
		if s.V != item.v {
			t.Fatalf("%d V = %d", i, s.V)
		}
		if err := signer.Verify(d, s, local.Address()); err != nil {
			t.Fatalf("%d recovered address: %v", i, err)
		}
	}
}

func assertOneDeployKey(t *testing.T, action any, wantType string) {
	t.Helper()
	if _, err := signing.L1ActionComponents(action, 1, nil, nil); err != nil {
		t.Fatalf("wire %T: %v", action, err)
	}
	raw, err := json.Marshal(action)
	if err != nil {
		t.Fatalf("JSON %T: %v", action, err)
	}
	var decoded map[string]json.RawMessage
	if err := json.Unmarshal(raw, &decoded); err != nil {
		t.Fatal(err)
	}
	if got := string(decoded["type"]); got != `"`+wantType+`"` {
		t.Fatalf("type = %s", got)
	}
	if len(decoded) != 2 {
		t.Fatalf("deploy union must have exactly type plus variant, got %s", raw)
	}
}
func uint64ptr(v uint64) *uint64 { return &v }
