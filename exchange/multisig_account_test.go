package exchange_test

import (
	"context"
	"encoding/json"
	"math/big"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/Apexllcc/hyperliquid-go-sdk/asset"
	"github.com/Apexllcc/hyperliquid-go-sdk/exchange"
	"github.com/Apexllcc/hyperliquid-go-sdk/nonce"
	"github.com/Apexllcc/hyperliquid-go-sdk/signer"
	"github.com/Apexllcc/hyperliquid-go-sdk/signing"
	"github.com/ethereum/go-ethereum/common"
	"github.com/shopspring/decimal"
)

func TestMultiSigL1CollectsCanonicalAuthorizedSignatures(t *testing.T) {
	leader := testMultiSigSigner(t, "0123456789012345678901234567890123456789012345678901234567890123")
	second := testMultiSigSigner(t, "1123456789012345678901234567890123456789012345678901234567890123")
	multiSigUser := common.HexToAddress("0x1111111111111111111111111111111111111111")
	requests := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests++
		var payload struct {
			Action struct {
				Type       string `json:"type"`
				Signatures []struct {
					R string `json:"r"`
					S string `json:"s"`
					V uint8  `json:"v"`
				} `json:"signatures"`
				Payload struct {
					MultiSigUser string `json:"multiSigUser"`
					OuterSigner  string `json:"outerSigner"`
					Action       struct {
						Type string `json:"type"`
					} `json:"action"`
				} `json:"payload"`
			} `json:"action"`
		}
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			t.Fatal(err)
		}
		if payload.Action.Type != "multiSig" || payload.Action.Payload.Action.Type != "noop" {
			t.Fatalf("action=%+v", payload.Action)
		}
		if payload.Action.Payload.MultiSigUser != strings.ToLower(multiSigUser.Hex()) || payload.Action.Payload.OuterSigner != strings.ToLower(leader.Address().Hex()) {
			t.Fatalf("payload=%+v", payload.Action.Payload)
		}
		if len(payload.Action.Signatures) != 2 {
			t.Fatalf("signature count=%d", len(payload.Action.Signatures))
		}
		for _, signature := range payload.Action.Signatures {
			if signature.R == "" || signature.S == "" || (signature.V != 27 && signature.V != 28) {
				t.Fatalf("non-canonical compact signature=%+v", signature)
			}
		}
		_, _ = w.Write([]byte(`{"status":"ok","response":{"type":"default","data":{}}}`))
	}))
	defer server.Close()
	c := exchange.NewClient(server.URL, "mainnet", nil, 0, leader, nonce.NewMonotonicManager(nil), asset.NewStaticResolver(nil), "test")
	config := exchange.MultiSigConfig{
		MultiSigUser:    multiSigUser,
		Leader:          leader,
		AuthorizedUsers: []common.Address{leader.Address(), second.Address()},
		Signers:         []signer.DigestSigner{second, leader},
		Threshold:       2,
	}
	if _, err := c.SubmitMultiSigL1(context.Background(), config, signing.NoopAction{}); err != nil {
		t.Fatal(err)
	}
	if requests != 1 {
		t.Fatalf("requests=%d", requests)
	}
}

func TestMultiSigRejectsDuplicateUnauthorizedAndUnderThresholdSignersBeforeSubmission(t *testing.T) {
	leader := testMultiSigSigner(t, "0123456789012345678901234567890123456789012345678901234567890123")
	second := testMultiSigSigner(t, "1123456789012345678901234567890123456789012345678901234567890123")
	third := testMultiSigSigner(t, "2123456789012345678901234567890123456789012345678901234567890123")
	requests := 0
	server := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) { requests++ }))
	defer server.Close()
	c := exchange.NewClient(server.URL, "mainnet", nil, 0, leader, nonce.NewMonotonicManager(nil), asset.NewStaticResolver(nil), "test")
	base := exchange.MultiSigConfig{MultiSigUser: common.HexToAddress("0x1111111111111111111111111111111111111111"), Leader: leader, AuthorizedUsers: []common.Address{leader.Address(), second.Address()}, Signers: []signer.DigestSigner{leader, second}, Threshold: 2}
	cases := []exchange.MultiSigConfig{
		func() exchange.MultiSigConfig { x := base; x.Signers = []signer.DigestSigner{leader, leader}; return x }(),
		func() exchange.MultiSigConfig { x := base; x.Signers = []signer.DigestSigner{leader, third}; return x }(),
		func() exchange.MultiSigConfig { x := base; x.Signers = []signer.DigestSigner{leader}; return x }(),
	}
	for _, config := range cases {
		if _, err := c.SubmitMultiSigL1(context.Background(), config, signing.NoopAction{}); err == nil {
			t.Fatal("expected local multi-sig validation failure")
		}
	}
	if requests != 0 {
		t.Fatalf("invalid multi-sig requests=%d", requests)
	}
}

func TestMultiSigL1TypeBoundaryRejectsMapAndBareStruct(t *testing.T) {
	for _, action := range []any{
		map[string]any{"type": "noop"},
		struct {
			Type string `json:"type"`
		}{Type: "noop"},
	} {
		if _, ok := action.(signing.L1Action); ok {
			t.Fatalf("non-canonical action %T satisfied signing.L1Action", action)
		}
	}
	var _ signing.L1Action = signing.NoopAction{}
}

func TestAccountManagementActionsUseCanonicalL1AndUserSignedPaths(t *testing.T) {
	local := testMultiSigSigner(t, "0123456789012345678901234567890123456789012345678901234567890123")
	vaultTarget := common.HexToAddress("0xAa000000000000000000000000000000000000aA")
	seen := map[string]struct{}{}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var payload struct {
			Action struct {
				Type         string `json:"type"`
				Signers      string `json:"signers"`
				USD          uint64 `json:"usd"`
				VaultAddress string `json:"vaultAddress"`
			} `json:"action"`
		}
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			t.Fatal(err)
		}
		seen[payload.Action.Type] = struct{}{}
		if payload.Action.Type == "convertToMultiSigUser" && payload.Action.Signers != `{"authorizedUsers":["0x14791697260e4c9a71f18484c9f997b308e59325"],"threshold":1}` {
			t.Fatalf("signers=%s", payload.Action.Signers)
		}
		if payload.Action.Type == "vaultDistribute" && payload.Action.USD != 0 {
			t.Fatalf("vault close USD=%d", payload.Action.USD)
		}
		if (payload.Action.Type == "vaultModify" || payload.Action.Type == "vaultDistribute") && payload.Action.VaultAddress != strings.ToLower(vaultTarget.Hex()) {
			t.Fatalf("non-canonical vault address=%q", payload.Action.VaultAddress)
		}
		_, _ = w.Write([]byte(`{"status":"ok","response":{"type":"default","data":{}}}`))
	}))
	defer server.Close()
	c := exchange.NewClient(server.URL, "mainnet", nil, 0, local, nonce.NewMonotonicManager(nil), asset.NewStaticResolver(nil), "test")
	ctx := context.Background()
	if _, err := c.ConvertToMultiSigUser(ctx, &signing.MultiSigSignerSet{AuthorizedUsers: []common.Address{local.Address()}, Threshold: 1}); err != nil {
		t.Fatal(err)
	}
	if _, err := c.CreateVault(ctx, exchange.CreateVaultRequest{Name: "vault", Description: "a vault description", InitialUSD: decimal.RequireFromString("100")}); err != nil {
		t.Fatal(err)
	}
	allow := true
	if _, err := c.ModifyVault(ctx, exchange.VaultModifyRequest{VaultAddress: vaultTarget, AllowDeposits: &allow}); err != nil {
		t.Fatal(err)
	}
	if _, err := c.DistributeVault(ctx, exchange.VaultDistributionRequest{VaultAddress: vaultTarget, USD: decimal.Zero}); err != nil {
		t.Fatal(err)
	}
	if _, err := c.ModifySubAccount(ctx, exchange.SubAccountModifyRequest{SubAccountUser: common.HexToAddress("0x2222222222222222222222222222222222222222"), Name: "desk"}); err != nil {
		t.Fatal(err)
	}
	if _, err := c.SetDisplayName(ctx, "alice"); err != nil {
		t.Fatal(err)
	}
	for _, kind := range []string{"convertToMultiSigUser", "createVault", "vaultModify", "vaultDistribute", "subAccountModify", "setDisplayName"} {
		if _, ok := seen[kind]; !ok {
			t.Errorf("did not submit %s", kind)
		}
	}
}

func TestMultiSigUserSignedActionUsesCompactInnerSignatures(t *testing.T) {
	leader := testMultiSigSigner(t, "0123456789012345678901234567890123456789012345678901234567890123")
	second := testMultiSigSigner(t, "1123456789012345678901234567890123456789012345678901234567890123")
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var payload struct {
			Action struct {
				Type    string `json:"type"`
				Payload struct {
					Action struct {
						Type string `json:"type"`
					} `json:"action"`
				} `json:"payload"`
			} `json:"action"`
			VaultAddress *common.Address `json:"vaultAddress"`
			ExpiresAfter *uint64         `json:"expiresAfter"`
		}
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			t.Fatal(err)
		}
		if payload.Action.Type != "multiSig" || payload.Action.Payload.Action.Type != "convertToMultiSigUser" || payload.ExpiresAfter == nil || *payload.ExpiresAfter != 1_700_000_100_000 || payload.VaultAddress == nil {
			t.Fatalf("payload=%+v", payload)
		}
		_, _ = w.Write([]byte(`{"status":"ok","response":{"type":"default","data":{}}}`))
	}))
	defer server.Close()
	vault := common.HexToAddress("0xaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa")
	c, err := exchange.NewClientWithOptions(server.URL, "testnet", nil, 0, leader, nonce.NewMonotonicManager(nil), asset.NewStaticResolver(nil), "test", exchange.WithVaultAddress(vault), exchange.WithExpiresAfter(1_700_000_100_000))
	if err != nil {
		t.Fatal(err)
	}
	config := exchange.MultiSigConfig{MultiSigUser: common.HexToAddress("0x1111111111111111111111111111111111111111"), Leader: leader, AuthorizedUsers: []common.Address{leader.Address(), second.Address()}, Signers: []signer.DigestSigner{leader, second}, Threshold: 2}
	action := signing.ConvertToMultiSigUserAction{Signers: nil, Nonce: 1_700_000_000_123}
	if _, err := c.SubmitMultiSigUserAction(context.Background(), config, action); err != nil {
		t.Fatal(err)
	}
}

func TestMultiSigAllowsAPITxLeaderOnlyWithAuthorizedOwner(t *testing.T) {
	owner := testMultiSigSigner(t, "0123456789012345678901234567890123456789012345678901234567890123")
	second := testMultiSigSigner(t, "1123456789012345678901234567890123456789012345678901234567890123")
	apiLeader := testMultiSigSigner(t, "2123456789012345678901234567890123456789012345678901234567890123")
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"status":"ok","response":{"type":"default","data":{}}}`))
	}))
	defer server.Close()
	c := exchange.NewClient(server.URL, "mainnet", nil, 0, owner, nonce.NewMonotonicManager(nil), asset.NewStaticResolver(nil), "test")
	ownerAddress := owner.Address()
	config := exchange.MultiSigConfig{MultiSigUser: common.HexToAddress("0x1111111111111111111111111111111111111111"), Leader: apiLeader, LeaderOwner: &ownerAddress, AuthorizedUsers: []common.Address{owner.Address(), second.Address()}, Signers: []signer.DigestSigner{owner, second}, Threshold: 2}
	if _, err := c.SubmitMultiSigL1(context.Background(), config, signing.NoopAction{}); err != nil {
		t.Fatal(err)
	}
	config.LeaderOwner = nil
	if _, err := c.SubmitMultiSigL1(context.Background(), config, signing.NoopAction{}); err == nil {
		t.Fatal("API leader without an authorized owner was accepted")
	}
}

func TestMultiSigBlocksInvalidSignerContributionsBeforeSubmission(t *testing.T) {
	leader := testMultiSigSigner(t, "0123456789012345678901234567890123456789012345678901234567890123")
	second := testMultiSigSigner(t, "1123456789012345678901234567890123456789012345678901234567890123")
	requests := 0
	server := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) { requests++ }))
	defer server.Close()
	c := exchange.NewClient(server.URL, "mainnet", nil, 0, leader, nonce.NewMonotonicManager(nil), asset.NewStaticResolver(nil), "test")
	base := exchange.MultiSigConfig{MultiSigUser: common.HexToAddress("0x1111111111111111111111111111111111111111"), Leader: leader, AuthorizedUsers: []common.Address{leader.Address(), second.Address()}, Threshold: 2}
	for _, contribution := range []signer.DigestSigner{
		addressMismatchSigner{DigestSigner: leader, address: second.Address()},
		highSSigner{DigestSigner: second},
	} {
		config := base
		config.Signers = []signer.DigestSigner{leader, contribution}
		if _, err := c.SubmitMultiSigL1(context.Background(), config, signing.NoopAction{}); err == nil {
			t.Fatal("expected invalid multi-sig contribution to fail")
		}
	}
	if requests != 0 {
		t.Fatalf("invalid signature requests=%d", requests)
	}
}

type addressMismatchSigner struct {
	signer.DigestSigner
	address common.Address
}

func (s addressMismatchSigner) Address() common.Address { return s.address }

type highSSigner struct{ signer.DigestSigner }

func (s highSSigner) SignDigest(ctx context.Context, digest signer.Digest) (signer.Signature, error) {
	signature, err := s.DigestSigner.SignDigest(ctx, digest)
	if err != nil {
		return signer.Signature{}, err
	}
	high := new(big.Int).Sub(signerCurveOrder(), new(big.Int).SetBytes(signature.S[:]))
	high.FillBytes(signature.S[:])
	return signature, nil
}

func signerCurveOrder() *big.Int {
	value, ok := new(big.Int).SetString("fffffffffffffffffffffffffffffffebaaedce6af48a03bbfd25e8cd0364141", 16)
	if !ok {
		panic("invalid secp256k1 order")
	}
	return value
}

func testMultiSigSigner(t *testing.T, key string) *signer.LocalPrivateKeySigner {
	t.Helper()
	s, err := signer.NewLocalPrivateKeySignerFromHex(key)
	if err != nil {
		t.Fatal(err)
	}
	return s
}
