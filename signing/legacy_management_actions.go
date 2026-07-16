package signing

import (
	"encoding/json"

	"github.com/vmihailenco/msgpack/v5"
)

// SetReferrerAction is the official L1 action exposed by the Python SDK for
// associating an account with a referral code.
type SetReferrerAction struct{ Code string }

func (a SetReferrerAction) MarshalMsgpack() ([]byte, error) {
	return marshalMap(func(e *msgpack.Encoder) error {
		if err := e.EncodeMapLen(2); err != nil {
			return err
		}
		if err := e.EncodeString("type"); err != nil {
			return err
		}
		if err := e.EncodeString("setReferrer"); err != nil {
			return err
		}
		if err := e.EncodeString("code"); err != nil {
			return err
		}
		return e.EncodeString(a.Code)
	})
}
func (a SetReferrerAction) MarshalJSON() ([]byte, error) {
	return json.Marshal(struct {
		Type string `json:"type"`
		Code string `json:"code"`
	}{"setReferrer", a.Code})
}

// CreateSubAccountAction creates a named subaccount through an L1 action.
type CreateSubAccountAction struct{ Name string }

func (a CreateSubAccountAction) MarshalMsgpack() ([]byte, error) {
	return marshalMap(func(e *msgpack.Encoder) error {
		if err := e.EncodeMapLen(2); err != nil {
			return err
		}
		if err := e.EncodeString("type"); err != nil {
			return err
		}
		if err := e.EncodeString("createSubAccount"); err != nil {
			return err
		}
		if err := e.EncodeString("name"); err != nil {
			return err
		}
		return e.EncodeString(a.Name)
	})
}
func (a CreateSubAccountAction) MarshalJSON() ([]byte, error) {
	return json.Marshal(struct {
		Type string `json:"type"`
		Name string `json:"name"`
	}{"createSubAccount", a.Name})
}
