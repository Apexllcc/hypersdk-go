//go:build integration && testnet

package integration

import (
	"errors"
	"fmt"
	"strings"
	"testing"

	"github.com/Apexllcc/hypersdk-go/exchange"
)

func TestRequireAcceptedCancels(t *testing.T) {
	success := "success"
	rejected := "order not found"
	unexpected := "queued"
	accepted := func(statuses ...exchange.CancelStatus) exchange.ActionResponse {
		return exchange.ActionResponse{Response: exchange.ActionResponseBody{
			Type: exchange.ActionResponseCancel,
			Data: exchange.CancelResponseData{Statuses: statuses},
		}}
	}
	tests := []struct {
		name     string
		response exchange.ActionResponse
		expected int
		wantErr  bool
		wantText string
	}{
		{
			name:     "two accepted cancels",
			response: accepted(exchange.CancelStatus{Success: &success}, exchange.CancelStatus{Success: &success}),
			expected: 2,
		},
		{
			name:     "status count mismatch",
			response: accepted(exchange.CancelStatus{Success: &success}),
			expected: 2,
			wantErr:  true,
		},
		{
			name:     "per order rejection",
			response: accepted(exchange.CancelStatus{Error: &rejected}),
			expected: 1,
			wantErr:  true,
			wantText: "cancel 0 rejected: order not found",
		},
		{
			name:     "unexpected success value",
			response: accepted(exchange.CancelStatus{Success: &unexpected}),
			expected: 1,
			wantErr:  true,
			wantText: "cancel 0 returned unexpected success value \"queued\"",
		},
		{
			name: "wrong response type",
			response: exchange.ActionResponse{Response: exchange.ActionResponseBody{
				Type: exchange.ActionResponseDefault,
			}},
			expected: 1,
			wantErr:  true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := requireAcceptedCancels(tt.response, tt.expected)
			if (err != nil) != tt.wantErr {
				t.Fatalf("requireAcceptedCancels() error = %v, wantErr %t", err, tt.wantErr)
			}
			if tt.wantText != "" && (err == nil || err.Error() != tt.wantText) {
				t.Fatalf("requireAcceptedCancels() error = %v, want %q", err, tt.wantText)
			}
		})
	}
}

func TestCancellationOutcome(t *testing.T) {
	definitive := &exchange.ActionResponseError{Status: "err", Message: "invalid cancel"}
	transport := errors.New("connection reset")
	confirmation := errors.New("order remains open")
	tests := []struct {
		name             string
		cancelErr        error
		confirmationErr  error
		wantNil          bool
		wantCancelErr    bool
		wantConfirmation bool
	}{
		{
			name:          "definitive rejection despite canceled order",
			cancelErr:     definitive,
			wantCancelErr: true,
		},
		{
			name:             "definitive rejection and order still open",
			cancelErr:        definitive,
			confirmationErr:  confirmation,
			wantCancelErr:    true,
			wantConfirmation: true,
		},
		{
			name:      "transport outcome reconciled by canceled order",
			cancelErr: transport,
			wantNil:   true,
		},
		{
			name:             "transport outcome and order still open",
			cancelErr:        transport,
			confirmationErr:  confirmation,
			wantCancelErr:    true,
			wantConfirmation: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := cancellationOutcome(tt.cancelErr, tt.confirmationErr)
			if (err == nil) != tt.wantNil {
				t.Fatalf("cancellationOutcome() error = %v, wantNil %t", err, tt.wantNil)
			}
			if tt.wantCancelErr && !errors.Is(err, tt.cancelErr) {
				t.Fatalf("cancellationOutcome() error = %v, does not retain cancel error", err)
			}
			if tt.wantConfirmation && (err == nil || !strings.Contains(err.Error(), tt.confirmationErr.Error())) {
				t.Fatalf("cancellationOutcome() error = %v, does not retain confirmation diagnostic", err)
			}
		})
	}
}

func TestIsDefinitiveCancelRejection(t *testing.T) {
	tests := []struct {
		name string
		err  error
		want bool
	}{
		{
			name: "exchange protocol rejection",
			err:  &exchange.ActionResponseError{Status: "err", Message: "invalid cancel"},
			want: true,
		},
		{
			name: "validated cancel response rejection",
			err:  fmt.Errorf("cancel: %w", &cancelResponseValidationError{err: errors.New("cancel 0 rejected")}),
			want: true,
		},
		{
			name: "transport error",
			err:  errors.New("connection reset"),
			want: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := isDefinitiveCancelRejection(tt.err); got != tt.want {
				t.Fatalf("isDefinitiveCancelRejection(%v) = %t, want %t", tt.err, got, tt.want)
			}
		})
	}
}
