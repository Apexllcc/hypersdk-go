package exchange

import (
	"bytes"
	"encoding/json"
	"fmt"
)

// ActionResponse is the Exchange response envelope. Response.Data is a
// discriminated union selected from Response.Type; unknown variants are
// retained safely as UnknownActionResponseData for forward compatibility.
type ActionResponse struct {
	Status   string               `json:"status"`
	Response ActionResponseBody   `json:"response"`
	Error    *ActionResponseError `json:"-"`
}

// ActionResponseBody contains a protocol response variant.
type ActionResponseBody struct {
	Type ActionResponseType
	Data ActionResponseData
}

// ActionResponseType identifies a documented Exchange response variant.
type ActionResponseType string

const (
	ActionResponseDefault    ActionResponseType = "default"
	ActionResponseOrder      ActionResponseType = "order"
	ActionResponseCancel     ActionResponseType = "cancel"
	ActionResponseTWAPOrder  ActionResponseType = "twapOrder"
	ActionResponseTWAPCancel ActionResponseType = "twapCancel"
)

// ActionResponseData is implemented by every typed Exchange response body.
// It is sealed so callers can exhaustively type-switch on SDK-defined values.
type ActionResponseData interface{ actionResponseData() }

// DefaultActionResponseData is returned by actions whose documented response
// type is "default".
type DefaultActionResponseData struct{}

func (DefaultActionResponseData) actionResponseData() {}

// OrderResponseData is returned for order placement and contains one status
// per submitted order.
type OrderResponseData struct {
	Statuses []OrderStatus `json:"statuses"`
}

func (OrderResponseData) actionResponseData() {}

// RestingOrder identifies an accepted resting order.
type RestingOrder struct {
	OID   uint64  `json:"oid"`
	Cloid *string `json:"cloid,omitempty"`
}

// FilledOrder identifies an immediately filled order. Price and size remain
// decimal strings to avoid precision loss.
type FilledOrder struct {
	TotalSize    string  `json:"totalSz"`
	AveragePrice string  `json:"avgPx"`
	OID          uint64  `json:"oid"`
	Cloid        *string `json:"cloid,omitempty"`
}

// OrderStatus is a discriminated order result. Exactly one documented field is
// normally populated; Error indicates a per-order rejection.
type OrderStatus struct {
	Resting *RestingOrder `json:"resting,omitempty"`
	Filled  *FilledOrder  `json:"filled,omitempty"`
	Error   *string       `json:"error,omitempty"`
}

// CancelResponseData is returned by cancel and cancel-by-cloid actions.
type CancelResponseData struct {
	Statuses []CancelStatus `json:"statuses"`
}

func (CancelResponseData) actionResponseData() {}

// CancelStatus is either a success string or a per-order error.
type CancelStatus struct {
	Success *string
	Error   *string
}

func (s *CancelStatus) UnmarshalJSON(raw []byte) error {
	var success string
	if err := json.Unmarshal(raw, &success); err == nil {
		s.Success = &success
		s.Error = nil
		return nil
	}
	var object struct {
		Error *string `json:"error"`
	}
	if err := json.Unmarshal(raw, &object); err != nil {
		return fmt.Errorf("decode cancel status: %w", err)
	}
	if object.Error == nil {
		return fmt.Errorf("decode cancel status: expected success string or error object")
	}
	s.Success = nil
	s.Error = object.Error
	return nil
}

// TWAPOrderResponseData is returned after a TWAP placement attempt.
type TWAPOrderResponseData struct {
	Status TWAPOrderStatus `json:"status"`
}

func (TWAPOrderResponseData) actionResponseData() {}

// TWAPOrderStatus is either a newly running TWAP identifier or an action error.
type TWAPOrderStatus struct {
	Running *TWAPRunningStatus `json:"running,omitempty"`
	Error   *string            `json:"error,omitempty"`
}

// TWAPRunningStatus contains the protocol TWAP identifier.
type TWAPRunningStatus struct {
	TWAPID uint64 `json:"twapId"`
}

// TWAPCancelResponseData is returned after a TWAP cancel attempt.
type TWAPCancelResponseData struct {
	Status TWAPCancelStatus `json:"status"`
}

func (TWAPCancelResponseData) actionResponseData() {}

// TWAPCancelStatus is either a success string or a protocol error.
type TWAPCancelStatus struct {
	Success *string
	Error   *string
}

func (s *TWAPCancelStatus) UnmarshalJSON(raw []byte) error {
	var success string
	if err := json.Unmarshal(raw, &success); err == nil {
		s.Success = &success
		s.Error = nil
		return nil
	}
	var object struct {
		Error *string `json:"error"`
	}
	if err := json.Unmarshal(raw, &object); err != nil {
		return fmt.Errorf("decode TWAP cancel status: %w", err)
	}
	if object.Error == nil {
		return fmt.Errorf("decode TWAP cancel status: expected success string or error object")
	}
	s.Success = nil
	s.Error = object.Error
	return nil
}

// UnknownActionResponseData retains an unknown response body without exposing
// mutable JSON bytes. RawJSON returns a defensive copy for diagnostics.
type UnknownActionResponseData struct{ raw []byte }

func (UnknownActionResponseData) actionResponseData() {}

func (d UnknownActionResponseData) RawJSON() []byte { return bytes.Clone(d.raw) }

// ActionResponseError is a protocol-level Exchange rejection returned with an
// HTTP 200 response and a non-ok status. It also implements error so callers
// cannot accidentally treat a rejected action as a successful submission.
type ActionResponseError struct {
	Status  string
	Message string
	raw     []byte
}

func (e *ActionResponseError) Error() string {
	if e.Message != "" {
		return fmt.Sprintf("Exchange action %s: %s", e.Status, e.Message)
	}
	return fmt.Sprintf("Exchange action %s", e.Status)
}

// RawJSON returns a defensive copy of a non-string protocol error payload.
func (e *ActionResponseError) RawJSON() []byte { return bytes.Clone(e.raw) }

func (r *ActionResponse) UnmarshalJSON(raw []byte) error {
	var envelope struct {
		Status   string          `json:"status"`
		Response json.RawMessage `json:"response"`
	}
	if err := json.Unmarshal(raw, &envelope); err != nil {
		return err
	}
	r.Status = envelope.Status
	r.Response = ActionResponseBody{}
	r.Error = nil
	if len(envelope.Response) == 0 || bytes.Equal(envelope.Response, []byte("null")) {
		return nil
	}
	if envelope.Status != "ok" {
		r.Error = decodeActionResponseError(envelope.Status, envelope.Response)
		return nil
	}
	var wire struct {
		Type ActionResponseType `json:"type"`
		Data json.RawMessage    `json:"data"`
	}
	if err := json.Unmarshal(envelope.Response, &wire); err != nil {
		// Some protocol errors use a non-object response. Preserve it instead of
		// converting a successful HTTP response into a decoder failure.
		r.Response.Data = UnknownActionResponseData{raw: bytes.Clone(envelope.Response)}
		return nil
	}
	r.Response.Type = wire.Type
	data, err := decodeActionResponseData(wire.Type, wire.Data)
	if err != nil {
		return err
	}
	r.Response.Data = data
	return nil
}

func decodeActionResponseError(status string, raw json.RawMessage) *ActionResponseError {
	var message string
	if err := json.Unmarshal(raw, &message); err == nil {
		return &ActionResponseError{Status: status, Message: message}
	}
	return &ActionResponseError{Status: status, Message: string(raw), raw: bytes.Clone(raw)}
}

func decodeActionResponseData(kind ActionResponseType, raw json.RawMessage) (ActionResponseData, error) {
	switch kind {
	case ActionResponseDefault:
		return DefaultActionResponseData{}, nil
	case ActionResponseOrder:
		var output OrderResponseData
		if err := json.Unmarshal(raw, &output); err != nil {
			return nil, fmt.Errorf("decode Exchange %q response: %w", kind, err)
		}
		return output, nil
	case ActionResponseCancel:
		var output CancelResponseData
		if err := json.Unmarshal(raw, &output); err != nil {
			return nil, fmt.Errorf("decode Exchange %q response: %w", kind, err)
		}
		return output, nil
	case ActionResponseTWAPOrder:
		var output TWAPOrderResponseData
		if err := json.Unmarshal(raw, &output); err != nil {
			return nil, fmt.Errorf("decode Exchange %q response: %w", kind, err)
		}
		return output, nil
	case ActionResponseTWAPCancel:
		var output TWAPCancelResponseData
		if err := json.Unmarshal(raw, &output); err != nil {
			return nil, fmt.Errorf("decode Exchange %q response: %w", kind, err)
		}
		return output, nil
	default:
		return UnknownActionResponseData{raw: bytes.Clone(raw)}, nil
	}
}
