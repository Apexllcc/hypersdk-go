package signing

import (
	"encoding/hex"
	"encoding/json"
	"fmt"
	"math/big"
	"strings"

	"github.com/Apexllcc/hypersdk-go/signer"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/common/math"
	"github.com/ethereum/go-ethereum/signer/core/apitypes"
)

const defaultUserActionSignatureChainID uint64 = 0x66eee

// UserSignedAction is a human-readable EIP-712 action accepted by the
// Exchange endpoint. It is deliberately sealed so every supported action has
// an exact protocol schema instead of exposing untyped signing maps.
type UserSignedAction interface {
	ActionNonce() uint64
	OmitOuterVaultAddress() bool
	userSignedTypedData(isMainnet bool) (string, []apitypes.Type, apitypes.TypedDataMessage, error)
	userSignedWire(isMainnet bool) (any, error)
}

// ComputeUserActionDigest constructs the final EIP-712 digest for a
// user-signed action. Hyperliquid's official SDK uses 0x66eee as its default
// signature chain ID while hyperliquidChain separates Mainnet and Testnet.
func ComputeUserActionDigest(action UserSignedAction, isMainnet bool) (signer.Digest, error) {
	if action == nil {
		return signer.Digest{}, fmt.Errorf("user-signed action is required")
	}
	primaryType, fields, message, err := action.userSignedTypedData(isMainnet)
	if err != nil {
		return signer.Digest{}, err
	}
	return userActionTypedDataDigest(primaryType, fields, message)
}

func userActionTypedDataDigest(primaryType string, fields []apitypes.Type, message apitypes.TypedDataMessage) (signer.Digest, error) {
	data := apitypes.TypedData{
		Types: apitypes.Types{
			"EIP712Domain": {
				{Name: "name", Type: "string"},
				{Name: "version", Type: "string"},
				{Name: "chainId", Type: "uint256"},
				{Name: "verifyingContract", Type: "address"},
			},
			primaryType: fields,
		},
		PrimaryType: primaryType,
		Domain: apitypes.TypedDataDomain{
			Name:              "HyperliquidSignTransaction",
			Version:           "1",
			ChainId:           math.NewHexOrDecimal256(int64(defaultUserActionSignatureChainID)),
			VerifyingContract: "0x0000000000000000000000000000000000000000",
		},
		Message: message,
	}
	raw, _, err := apitypes.TypedDataAndHash(data)
	if err != nil {
		return signer.Digest{}, fmt.Errorf("hash user-signed typed data: %w", err)
	}
	var digest signer.Digest
	copy(digest[:], raw)
	return digest, nil
}

// MarshalUserSignedAction returns the exact action JSON for the selected
// Hyperliquid environment. User-signed action bodies never contain
// expiresAfter; outer vault routing is selected by the Exchange client.
func MarshalUserSignedAction(action UserSignedAction, isMainnet bool) (json.RawMessage, error) {
	wire, err := UserSignedWire(action, isMainnet)
	if err != nil {
		return nil, err
	}
	raw, err := json.Marshal(wire)
	if err != nil {
		return nil, fmt.Errorf("marshal user-signed action: %w", err)
	}
	return raw, nil
}

// UserSignedWire returns the concrete canonical wire value for a supported
// user-signed action. It is exported for multi-sig wrapper construction; most
// callers should use MarshalUserSignedAction instead.
func UserSignedWire(action UserSignedAction, isMainnet bool) (any, error) {
	if action == nil {
		return nil, fmt.Errorf("user-signed action is required")
	}
	return action.userSignedWire(isMainnet)
}

// MultiSigUserPayloadWire returns the exact action representation embedded in
// a multiSig payload. userSetAbstraction is the one documented exception: its
// inner EIP-712 signatures use the human-readable value, while the outer
// payload uses HyperCore's compact wire enum.
func MultiSigUserPayloadWire(action UserSignedAction, isMainnet bool) (any, error) {
	if abstraction, ok := action.(UserSetAbstractionAction); ok {
		_, err := abstraction.userSignedWire(isMainnet)
		if err != nil {
			return nil, err
		}
		compact := map[string]string{"disabled": "i", "unifiedAccount": "u", "portfolioMargin": "p"}[abstraction.Abstraction]
		if compact == "" {
			return nil, fmt.Errorf("unsupported user abstraction %q", abstraction.Abstraction)
		}
		return struct {
			Type             string `json:"type"`
			HyperliquidChain string `json:"hyperliquidChain"`
			SignatureChainID string `json:"signatureChainId"`
			User             string `json:"user"`
			Abstraction      string `json:"abstraction"`
			Nonce            uint64 `json:"nonce"`
		}{"userSetAbstraction", userActionNetwork(isMainnet), "0x66eee", strings.ToLower(abstraction.User.Hex()), compact, abstraction.Nonce}, nil
	}
	if abstraction, ok := action.(UserDexAbstractionAction); ok {
		if _, _, _, err := abstraction.userSignedTypedData(isMainnet); err != nil {
			return nil, err
		}
		return struct {
			Type             string `json:"type"`
			HyperliquidChain string `json:"hyperliquidChain"`
			SignatureChainID string `json:"signatureChainId"`
			User             string `json:"user"`
			Enabled          bool   `json:"enabled"`
			Nonce            uint64 `json:"nonce"`
		}{"userDexAbstraction", userActionNetwork(isMainnet), "0x66eee", strings.ToLower(abstraction.User.Hex()), abstraction.Enabled, abstraction.Nonce}, nil
	}
	return UserSignedWire(action, isMainnet)
}

// USDSendAction transfers Core USDC to a destination account.
// Amount is a canonical decimal string to retain the exact signed value.
type USDSendAction struct {
	Destination common.Address
	Amount      string
	Time        uint64
}

func (a USDSendAction) ActionNonce() uint64       { return a.Time }
func (USDSendAction) OmitOuterVaultAddress() bool { return false }

// SpotSendAction transfers a Core spot token to a destination account.
type SpotSendAction struct {
	Destination common.Address
	Token       string
	Amount      string
	Time        uint64
}

func (a SpotSendAction) ActionNonce() uint64       { return a.Time }
func (SpotSendAction) OmitOuterVaultAddress() bool { return false }

func (a SpotSendAction) userSignedTypedData(isMainnet bool) (string, []apitypes.Type, apitypes.TypedDataMessage, error) {
	if err := a.validate(); err != nil {
		return "", nil, nil, err
	}
	return "HyperliquidTransaction:SpotSend", spotSendFields(), apitypes.TypedDataMessage{"hyperliquidChain": userActionNetwork(isMainnet), "destination": a.Destination.Hex(), "token": a.Token, "amount": a.Amount, "time": new(big.Int).SetUint64(a.Time)}, nil
}
func (a SpotSendAction) userSignedWire(isMainnet bool) (any, error) {
	if err := a.validate(); err != nil {
		return nil, err
	}
	return struct {
		Type             string `json:"type"`
		HyperliquidChain string `json:"hyperliquidChain"`
		SignatureChainID string `json:"signatureChainId"`
		Destination      string `json:"destination"`
		Token            string `json:"token"`
		Amount           string `json:"amount"`
		Time             uint64 `json:"time"`
	}{"spotSend", userActionNetwork(isMainnet), "0x66eee", a.Destination.Hex(), a.Token, a.Amount, a.Time}, nil
}
func (a SpotSendAction) validate() error {
	if a.Destination == (common.Address{}) || a.Token == "" || a.Amount == "" || a.Time == 0 {
		return fmt.Errorf("spot send requires destination, token, amount, and time")
	}
	return nil
}

// WithdrawFromBridgeAction starts an EVM bridge withdrawal.
type WithdrawFromBridgeAction struct {
	Destination common.Address
	Amount      string
	Time        uint64
}

func (a WithdrawFromBridgeAction) ActionNonce() uint64       { return a.Time }
func (WithdrawFromBridgeAction) OmitOuterVaultAddress() bool { return false }
func (a WithdrawFromBridgeAction) userSignedTypedData(isMainnet bool) (string, []apitypes.Type, apitypes.TypedDataMessage, error) {
	if err := a.validate(); err != nil {
		return "", nil, nil, err
	}
	return "HyperliquidTransaction:Withdraw", usdSendFields(), apitypes.TypedDataMessage{"hyperliquidChain": userActionNetwork(isMainnet), "destination": a.Destination.Hex(), "amount": a.Amount, "time": new(big.Int).SetUint64(a.Time)}, nil
}
func (a WithdrawFromBridgeAction) userSignedWire(isMainnet bool) (any, error) {
	if err := a.validate(); err != nil {
		return nil, err
	}
	return struct {
		Type             string `json:"type"`
		HyperliquidChain string `json:"hyperliquidChain"`
		SignatureChainID string `json:"signatureChainId"`
		Destination      string `json:"destination"`
		Amount           string `json:"amount"`
		Time             uint64 `json:"time"`
	}{"withdraw3", userActionNetwork(isMainnet), "0x66eee", a.Destination.Hex(), a.Amount, a.Time}, nil
}
func (a WithdrawFromBridgeAction) validate() error {
	if a.Destination == (common.Address{}) || a.Amount == "" || a.Time == 0 {
		return fmt.Errorf("withdrawal requires destination, amount, and time")
	}
	return nil
}

// USDClassTransferAction moves Core USDC between the spot and perp classes.
type USDClassTransferAction struct {
	Amount string
	ToPerp bool
	Nonce  uint64
}

func (a USDClassTransferAction) ActionNonce() uint64       { return a.Nonce }
func (USDClassTransferAction) OmitOuterVaultAddress() bool { return true }
func (a USDClassTransferAction) userSignedTypedData(isMainnet bool) (string, []apitypes.Type, apitypes.TypedDataMessage, error) {
	if err := a.validate(); err != nil {
		return "", nil, nil, err
	}
	return "HyperliquidTransaction:UsdClassTransfer", []apitypes.Type{{Name: "hyperliquidChain", Type: "string"}, {Name: "amount", Type: "string"}, {Name: "toPerp", Type: "bool"}, {Name: "nonce", Type: "uint64"}}, apitypes.TypedDataMessage{"hyperliquidChain": userActionNetwork(isMainnet), "amount": a.Amount, "toPerp": a.ToPerp, "nonce": new(big.Int).SetUint64(a.Nonce)}, nil
}
func (a USDClassTransferAction) userSignedWire(isMainnet bool) (any, error) {
	if err := a.validate(); err != nil {
		return nil, err
	}
	return struct {
		Type             string `json:"type"`
		HyperliquidChain string `json:"hyperliquidChain"`
		SignatureChainID string `json:"signatureChainId"`
		Amount           string `json:"amount"`
		ToPerp           bool   `json:"toPerp"`
		Nonce            uint64 `json:"nonce"`
	}{"usdClassTransfer", userActionNetwork(isMainnet), "0x66eee", a.Amount, a.ToPerp, a.Nonce}, nil
}
func (a USDClassTransferAction) validate() error {
	if a.Amount == "" || a.Nonce == 0 {
		return fmt.Errorf("USD class transfer requires amount and nonce")
	}
	return nil
}

// SendAssetAction moves a token between HyperCore DEX namespaces.
type SendAssetAction struct {
	Destination                              common.Address
	SourceDEX, DestinationDEX, Token, Amount string
	FromSubAccount                           *common.Address
	Nonce                                    uint64
}

func (a SendAssetAction) ActionNonce() uint64       { return a.Nonce }
func (SendAssetAction) OmitOuterVaultAddress() bool { return true }
func (a SendAssetAction) userSignedTypedData(isMainnet bool) (string, []apitypes.Type, apitypes.TypedDataMessage, error) {
	if err := a.validate(); err != nil {
		return "", nil, nil, err
	}
	return "HyperliquidTransaction:SendAsset", []apitypes.Type{{Name: "hyperliquidChain", Type: "string"}, {Name: "destination", Type: "string"}, {Name: "sourceDex", Type: "string"}, {Name: "destinationDex", Type: "string"}, {Name: "token", Type: "string"}, {Name: "amount", Type: "string"}, {Name: "fromSubAccount", Type: "string"}, {Name: "nonce", Type: "uint64"}}, apitypes.TypedDataMessage{"hyperliquidChain": userActionNetwork(isMainnet), "destination": a.Destination.Hex(), "sourceDex": a.SourceDEX, "destinationDex": a.DestinationDEX, "token": a.Token, "amount": a.Amount, "fromSubAccount": a.fromSubAccount(), "nonce": new(big.Int).SetUint64(a.Nonce)}, nil
}
func (a SendAssetAction) userSignedWire(isMainnet bool) (any, error) {
	if err := a.validate(); err != nil {
		return nil, err
	}
	return struct {
		Type             string `json:"type"`
		HyperliquidChain string `json:"hyperliquidChain"`
		SignatureChainID string `json:"signatureChainId"`
		Destination      string `json:"destination"`
		SourceDEX        string `json:"sourceDex"`
		DestinationDEX   string `json:"destinationDex"`
		Token            string `json:"token"`
		Amount           string `json:"amount"`
		FromSubAccount   string `json:"fromSubAccount"`
		Nonce            uint64 `json:"nonce"`
	}{"sendAsset", userActionNetwork(isMainnet), "0x66eee", a.Destination.Hex(), a.SourceDEX, a.DestinationDEX, a.Token, a.Amount, a.fromSubAccount(), a.Nonce}, nil
}
func (a SendAssetAction) fromSubAccount() string {
	if a.FromSubAccount == nil {
		return ""
	}
	return a.FromSubAccount.Hex()
}
func (a SendAssetAction) validate() error {
	if a.Destination == (common.Address{}) || a.Token == "" || a.Amount == "" || a.Nonce == 0 {
		return fmt.Errorf("send asset requires destination, token, amount, and nonce")
	}
	return nil
}

// ApproveAgentAction authorizes an API/agent wallet. A nil AgentName produces
// the official empty-string EIP-712 field while omitting agentName from JSON.
type ApproveAgentAction struct {
	AgentAddress common.Address
	AgentName    *string
	Nonce        uint64
}

func (a ApproveAgentAction) ActionNonce() uint64       { return a.Nonce }
func (ApproveAgentAction) OmitOuterVaultAddress() bool { return false }
func (a ApproveAgentAction) userSignedTypedData(isMainnet bool) (string, []apitypes.Type, apitypes.TypedDataMessage, error) {
	if err := a.validate(); err != nil {
		return "", nil, nil, err
	}
	return "HyperliquidTransaction:ApproveAgent", []apitypes.Type{{Name: "hyperliquidChain", Type: "string"}, {Name: "agentAddress", Type: "address"}, {Name: "agentName", Type: "string"}, {Name: "nonce", Type: "uint64"}}, apitypes.TypedDataMessage{"hyperliquidChain": userActionNetwork(isMainnet), "agentAddress": a.AgentAddress.Hex(), "agentName": a.agentName(), "nonce": new(big.Int).SetUint64(a.Nonce)}, nil
}
func (a ApproveAgentAction) userSignedWire(isMainnet bool) (any, error) {
	if err := a.validate(); err != nil {
		return nil, err
	}
	return struct {
		Type             string  `json:"type"`
		HyperliquidChain string  `json:"hyperliquidChain"`
		SignatureChainID string  `json:"signatureChainId"`
		AgentAddress     string  `json:"agentAddress"`
		AgentName        *string `json:"agentName,omitempty"`
		Nonce            uint64  `json:"nonce"`
	}{"approveAgent", userActionNetwork(isMainnet), "0x66eee", a.AgentAddress.Hex(), a.AgentName, a.Nonce}, nil
}
func (a ApproveAgentAction) agentName() string {
	if a.AgentName == nil {
		return ""
	}
	return *a.AgentName
}
func (a ApproveAgentAction) validate() error {
	if a.AgentAddress == (common.Address{}) || a.Nonce == 0 {
		return fmt.Errorf("approve agent requires address and nonce")
	}
	return nil
}

// ApproveBuilderFeeAction authorizes a builder fee ceiling expressed as a
// protocol percent string, for example "0.001%".
type ApproveBuilderFeeAction struct {
	Builder    common.Address
	MaxFeeRate string
	Nonce      uint64
}

func (a ApproveBuilderFeeAction) ActionNonce() uint64       { return a.Nonce }
func (ApproveBuilderFeeAction) OmitOuterVaultAddress() bool { return false }
func (a ApproveBuilderFeeAction) userSignedTypedData(isMainnet bool) (string, []apitypes.Type, apitypes.TypedDataMessage, error) {
	if err := a.validate(); err != nil {
		return "", nil, nil, err
	}
	return "HyperliquidTransaction:ApproveBuilderFee", []apitypes.Type{{Name: "hyperliquidChain", Type: "string"}, {Name: "maxFeeRate", Type: "string"}, {Name: "builder", Type: "address"}, {Name: "nonce", Type: "uint64"}}, apitypes.TypedDataMessage{"hyperliquidChain": userActionNetwork(isMainnet), "maxFeeRate": a.MaxFeeRate, "builder": a.Builder.Hex(), "nonce": new(big.Int).SetUint64(a.Nonce)}, nil
}
func (a ApproveBuilderFeeAction) userSignedWire(isMainnet bool) (any, error) {
	if err := a.validate(); err != nil {
		return nil, err
	}
	return struct {
		Type             string `json:"type"`
		HyperliquidChain string `json:"hyperliquidChain"`
		SignatureChainID string `json:"signatureChainId"`
		MaxFeeRate       string `json:"maxFeeRate"`
		Builder          string `json:"builder"`
		Nonce            uint64 `json:"nonce"`
	}{"approveBuilderFee", userActionNetwork(isMainnet), "0x66eee", a.MaxFeeRate, a.Builder.Hex(), a.Nonce}, nil
}
func (a ApproveBuilderFeeAction) validate() error {
	if a.Builder == (common.Address{}) || a.MaxFeeRate == "" || a.Nonce == 0 {
		return fmt.Errorf("approve builder fee requires builder, max fee rate, and nonce")
	}
	return nil
}

func usdSendFields() []apitypes.Type {
	return []apitypes.Type{{Name: "hyperliquidChain", Type: "string"}, {Name: "destination", Type: "string"}, {Name: "amount", Type: "string"}, {Name: "time", Type: "uint64"}}
}
func spotSendFields() []apitypes.Type {
	return []apitypes.Type{{Name: "hyperliquidChain", Type: "string"}, {Name: "destination", Type: "string"}, {Name: "token", Type: "string"}, {Name: "amount", Type: "string"}, {Name: "time", Type: "uint64"}}
}

func (a USDSendAction) userSignedTypedData(isMainnet bool) (string, []apitypes.Type, apitypes.TypedDataMessage, error) {
	if err := a.validate(); err != nil {
		return "", nil, nil, err
	}
	return "HyperliquidTransaction:UsdSend", []apitypes.Type{
			{Name: "hyperliquidChain", Type: "string"},
			{Name: "destination", Type: "string"},
			{Name: "amount", Type: "string"},
			{Name: "time", Type: "uint64"},
		}, apitypes.TypedDataMessage{
			"hyperliquidChain": userActionNetwork(isMainnet),
			"destination":      a.Destination.Hex(),
			"amount":           a.Amount,
			"time":             new(big.Int).SetUint64(a.Time),
		}, nil
}

func (a USDSendAction) userSignedWire(isMainnet bool) (any, error) {
	if err := a.validate(); err != nil {
		return nil, err
	}
	return struct {
		Type             string `json:"type"`
		HyperliquidChain string `json:"hyperliquidChain"`
		SignatureChainID string `json:"signatureChainId"`
		Destination      string `json:"destination"`
		Amount           string `json:"amount"`
		Time             uint64 `json:"time"`
	}{"usdSend", userActionNetwork(isMainnet), "0x66eee", a.Destination.Hex(), a.Amount, a.Time}, nil
}

func (a USDSendAction) validate() error {
	if a.Destination == (common.Address{}) {
		return fmt.Errorf("USD send destination is required")
	}
	if a.Amount == "" {
		return fmt.Errorf("USD send amount is required")
	}
	if a.Time == 0 {
		return fmt.Errorf("USD send time is required")
	}
	return nil
}

func userActionNetwork(isMainnet bool) string {
	if isMainnet {
		return "Mainnet"
	}
	return "Testnet"
}

// SendToEVMWithDataAction transfers a Core asset to an EVM contract and
// carries opaque calldata. Data is raw bytes; its wire representation is
// canonical 0x-prefixed hexadecimal.
type SendToEVMWithDataAction struct {
	Token                string
	Amount               string
	SourceDEX            string
	DestinationRecipient string
	AddressEncoding      string
	DestinationChainID   uint32
	GasLimit             uint64
	Data                 []byte
	Nonce                uint64
}

func (a SendToEVMWithDataAction) ActionNonce() uint64       { return a.Nonce }
func (SendToEVMWithDataAction) OmitOuterVaultAddress() bool { return true }

func (a SendToEVMWithDataAction) userSignedTypedData(isMainnet bool) (string, []apitypes.Type, apitypes.TypedDataMessage, error) {
	if err := a.validate(); err != nil {
		return "", nil, nil, err
	}
	return "HyperliquidTransaction:SendToEvmWithData", []apitypes.Type{
			{Name: "hyperliquidChain", Type: "string"},
			{Name: "token", Type: "string"},
			{Name: "amount", Type: "string"},
			{Name: "sourceDex", Type: "string"},
			{Name: "destinationRecipient", Type: "string"},
			{Name: "addressEncoding", Type: "string"},
			{Name: "destinationChainId", Type: "uint32"},
			{Name: "gasLimit", Type: "uint64"},
			{Name: "data", Type: "bytes"},
			{Name: "nonce", Type: "uint64"},
		}, apitypes.TypedDataMessage{
			"hyperliquidChain":     userActionNetwork(isMainnet),
			"token":                a.Token,
			"amount":               a.Amount,
			"sourceDex":            a.SourceDEX,
			"destinationRecipient": a.DestinationRecipient,
			"addressEncoding":      a.AddressEncoding,
			"destinationChainId":   new(big.Int).SetUint64(uint64(a.DestinationChainID)),
			"gasLimit":             new(big.Int).SetUint64(a.GasLimit),
			"data":                 a.dataHex(),
			"nonce":                new(big.Int).SetUint64(a.Nonce),
		}, nil
}

func (a SendToEVMWithDataAction) userSignedWire(isMainnet bool) (any, error) {
	if err := a.validate(); err != nil {
		return nil, err
	}
	return struct {
		Type                 string `json:"type"`
		HyperliquidChain     string `json:"hyperliquidChain"`
		SignatureChainID     string `json:"signatureChainId"`
		Token                string `json:"token"`
		Amount               string `json:"amount"`
		SourceDEX            string `json:"sourceDex"`
		DestinationRecipient string `json:"destinationRecipient"`
		AddressEncoding      string `json:"addressEncoding"`
		DestinationChainID   uint32 `json:"destinationChainId"`
		GasLimit             uint64 `json:"gasLimit"`
		Data                 string `json:"data"`
		Nonce                uint64 `json:"nonce"`
	}{"sendToEvmWithData", userActionNetwork(isMainnet), "0x66eee", a.Token, a.Amount, a.SourceDEX, a.DestinationRecipient, a.AddressEncoding, a.DestinationChainID, a.GasLimit, a.dataHex(), a.Nonce}, nil
}

func (a SendToEVMWithDataAction) dataHex() string { return "0x" + hex.EncodeToString(a.Data) }

func (a SendToEVMWithDataAction) validate() error {
	if a.Token == "" || a.Amount == "" || a.DestinationRecipient == "" || a.Nonce == 0 {
		return fmt.Errorf("send to EVM with data requires token, amount, recipient, and nonce")
	}
	if a.AddressEncoding != "hex" && a.AddressEncoding != "base58" {
		return fmt.Errorf("unsupported EVM destination address encoding")
	}
	return nil
}

// CDepositAction moves native token from the Core spot account into staking.
type CDepositAction struct {
	Wei   uint64
	Nonce uint64
}

func (a CDepositAction) ActionNonce() uint64       { return a.Nonce }
func (CDepositAction) OmitOuterVaultAddress() bool { return true }
func (a CDepositAction) userSignedTypedData(isMainnet bool) (string, []apitypes.Type, apitypes.TypedDataMessage, error) {
	if err := a.validate(); err != nil {
		return "", nil, nil, err
	}
	return "HyperliquidTransaction:CDeposit", stakingFields(), apitypes.TypedDataMessage{"hyperliquidChain": userActionNetwork(isMainnet), "wei": new(big.Int).SetUint64(a.Wei), "nonce": new(big.Int).SetUint64(a.Nonce)}, nil
}
func (a CDepositAction) userSignedWire(isMainnet bool) (any, error) {
	if err := a.validate(); err != nil {
		return nil, err
	}
	return stakingWire{"cDeposit", userActionNetwork(isMainnet), "0x66eee", a.Wei, a.Nonce}, nil
}
func (a CDepositAction) validate() error {
	if a.Wei == 0 || a.Nonce == 0 {
		return fmt.Errorf("staking deposit requires positive wei and nonce")
	}
	return nil
}

// CWithdrawAction moves native token from staking back to the Core spot account.
type CWithdrawAction struct {
	Wei   uint64
	Nonce uint64
}

func (a CWithdrawAction) ActionNonce() uint64       { return a.Nonce }
func (CWithdrawAction) OmitOuterVaultAddress() bool { return true }
func (a CWithdrawAction) userSignedTypedData(isMainnet bool) (string, []apitypes.Type, apitypes.TypedDataMessage, error) {
	if err := a.validate(); err != nil {
		return "", nil, nil, err
	}
	return "HyperliquidTransaction:CWithdraw", stakingFields(), apitypes.TypedDataMessage{"hyperliquidChain": userActionNetwork(isMainnet), "wei": new(big.Int).SetUint64(a.Wei), "nonce": new(big.Int).SetUint64(a.Nonce)}, nil
}
func (a CWithdrawAction) userSignedWire(isMainnet bool) (any, error) {
	if err := a.validate(); err != nil {
		return nil, err
	}
	return stakingWire{"cWithdraw", userActionNetwork(isMainnet), "0x66eee", a.Wei, a.Nonce}, nil
}
func (a CWithdrawAction) validate() error {
	if a.Wei == 0 || a.Nonce == 0 {
		return fmt.Errorf("staking withdrawal requires positive wei and nonce")
	}
	return nil
}

type stakingWire struct {
	Type             string `json:"type"`
	HyperliquidChain string `json:"hyperliquidChain"`
	SignatureChainID string `json:"signatureChainId"`
	Wei              uint64 `json:"wei"`
	Nonce            uint64 `json:"nonce"`
}

func stakingFields() []apitypes.Type {
	return []apitypes.Type{{Name: "hyperliquidChain", Type: "string"}, {Name: "wei", Type: "uint64"}, {Name: "nonce", Type: "uint64"}}
}

// TokenDelegateAction delegates or undelegates native staking balance.
type TokenDelegateAction struct {
	Validator    common.Address
	Wei          uint64
	IsUndelegate bool
	Nonce        uint64
}

func (a TokenDelegateAction) ActionNonce() uint64       { return a.Nonce }
func (TokenDelegateAction) OmitOuterVaultAddress() bool { return true }
func (a TokenDelegateAction) userSignedTypedData(isMainnet bool) (string, []apitypes.Type, apitypes.TypedDataMessage, error) {
	if err := a.validate(); err != nil {
		return "", nil, nil, err
	}
	return "HyperliquidTransaction:TokenDelegate", []apitypes.Type{{Name: "hyperliquidChain", Type: "string"}, {Name: "validator", Type: "address"}, {Name: "wei", Type: "uint64"}, {Name: "isUndelegate", Type: "bool"}, {Name: "nonce", Type: "uint64"}}, apitypes.TypedDataMessage{"hyperliquidChain": userActionNetwork(isMainnet), "validator": a.Validator.Hex(), "wei": new(big.Int).SetUint64(a.Wei), "isUndelegate": a.IsUndelegate, "nonce": new(big.Int).SetUint64(a.Nonce)}, nil
}
func (a TokenDelegateAction) userSignedWire(isMainnet bool) (any, error) {
	if err := a.validate(); err != nil {
		return nil, err
	}
	return struct {
		Type             string `json:"type"`
		HyperliquidChain string `json:"hyperliquidChain"`
		SignatureChainID string `json:"signatureChainId"`
		Validator        string `json:"validator"`
		Wei              uint64 `json:"wei"`
		IsUndelegate     bool   `json:"isUndelegate"`
		Nonce            uint64 `json:"nonce"`
	}{"tokenDelegate", userActionNetwork(isMainnet), "0x66eee", a.Validator.Hex(), a.Wei, a.IsUndelegate, a.Nonce}, nil
}
func (a TokenDelegateAction) validate() error {
	if a.Validator == (common.Address{}) || a.Wei == 0 || a.Nonce == 0 {
		return fmt.Errorf("token delegate requires validator, positive wei, and nonce")
	}
	return nil
}
