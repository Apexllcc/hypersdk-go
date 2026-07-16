package signing

import (
	"encoding/json"

	"github.com/vmihailenco/msgpack/v5"
)

// L1Action is a closed set of SDK-defined actions with deterministic
// MessagePack and JSON wire encodings. It deliberately prevents arbitrary
// maps and caller-defined structs from crossing an L1 signing boundary.
type L1Action interface {
	json.Marshaler
	msgpack.Marshaler
	canonicalL1Action()
}

func (CancelAction) canonicalL1Action()                    {}
func (OrderAction) canonicalL1Action()                     {}
func (CancelByCloidAction) canonicalL1Action()             {}
func (ScheduleCancelAction) canonicalL1Action()            {}
func (TWAPOrderAction) canonicalL1Action()                 {}
func (TWAPCancelAction) canonicalL1Action()                {}
func (SubaccountTransferAction) canonicalL1Action()        {}
func (SubaccountSpotTransferAction) canonicalL1Action()    {}
func (VaultTransferAction) canonicalL1Action()             {}
func (BatchModifyAction) canonicalL1Action()               {}
func (UpdateLeverageAction) canonicalL1Action()            {}
func (UpdateIsolatedMarginAction) canonicalL1Action()      {}
func (TopUpIsolatedOnlyMarginAction) canonicalL1Action()   {}
func (ReserveRequestWeightAction) canonicalL1Action()      {}
func (NoopAction) canonicalL1Action()                      {}
func (AgentEnableDexAbstractionAction) canonicalL1Action() {}
func (AgentSetAbstractionAction) canonicalL1Action()       {}
func (AgentSendAssetAction) canonicalL1Action()            {}
func (AuthorizeAQAV2RoleAction) canonicalL1Action()        {}
func (HIP3LiquidatorTransferAction) canonicalL1Action()    {}
func (UserOutcomeAction) canonicalL1Action()               {}
func (ValidatorL1StreamAction) canonicalL1Action()         {}
func (ClaimRewardsAction) canonicalL1Action()              {}
func (SetReferrerAction) canonicalL1Action()               {}
func (CreateSubAccountAction) canonicalL1Action()          {}
func (CreateVaultAction) canonicalL1Action()               {}
func (VaultModifyAction) canonicalL1Action()               {}
func (VaultDistributeAction) canonicalL1Action()           {}
func (SubAccountModifyAction) canonicalL1Action()          {}
func (SetDisplayNameAction) canonicalL1Action()            {}
func (PerpDeployAction) canonicalL1Action()                {}
func (SpotDeployAction) canonicalL1Action()                {}
func (EVMUserModifyAction) canonicalL1Action()             {}
func (GossipPriorityBidAction) canonicalL1Action()         {}
func (CValidatorAction) canonicalL1Action()                {}
func (CSignerAction) canonicalL1Action()                   {}
func (FinalizeEVMContractAction) canonicalL1Action()       {}
