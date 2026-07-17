package exchange_test

import (
	"encoding/json"
	"testing"

	"github.com/Apexllcc/hypersdk-go/exchange"
)

func TestActionResponseDecodesDocumentedVariants(t *testing.T) {
	cases := []struct {
		name  string
		body  string
		check func(*testing.T, exchange.ActionResponseData)
	}{
		{
			name: "filled order preserves decimal strings",
			body: `{"status":"ok","response":{"type":"order","data":{"statuses":[{"filled":{"totalSz":"0.02","avgPx":"1891.4","oid":77747314}}]}}}`,
			check: func(t *testing.T, data exchange.ActionResponseData) {
				t.Helper()
				got, ok := data.(exchange.OrderResponseData)
				if !ok || len(got.Statuses) != 1 || got.Statuses[0].Filled == nil || got.Statuses[0].Filled.AveragePrice != "1891.4" || got.Statuses[0].Filled.TotalSize != "0.02" {
					t.Fatalf("order data = %#v", data)
				}
			},
		},
		{
			name: "cancel error",
			body: `{"status":"ok","response":{"type":"cancel","data":{"statuses":[{"error":"already canceled"}]}}}`,
			check: func(t *testing.T, data exchange.ActionResponseData) {
				t.Helper()
				got, ok := data.(exchange.CancelResponseData)
				if !ok || len(got.Statuses) != 1 || got.Statuses[0].Error == nil || *got.Statuses[0].Error != "already canceled" {
					t.Fatalf("cancel data = %#v", data)
				}
			},
		},
		{
			name: "batch modify returns order status",
			body: `{"status":"ok","response":{"type":"order","data":{"statuses":[{"resting":{"oid":7}}]}}}`,
			check: func(t *testing.T, data exchange.ActionResponseData) {
				t.Helper()
				got, ok := data.(exchange.OrderResponseData)
				if !ok || len(got.Statuses) != 1 || got.Statuses[0].Resting == nil || got.Statuses[0].Resting.OID != 7 {
					t.Fatalf("batch modify data = %#v", data)
				}
			},
		},
		{
			name: "default response without data",
			body: `{"status":"ok","response":{"type":"default"}}`,
			check: func(t *testing.T, data exchange.ActionResponseData) {
				t.Helper()
				if _, ok := data.(exchange.DefaultActionResponseData); !ok {
					t.Fatalf("default data = %#v", data)
				}
			},
		},
		{
			name: "create vault response",
			body: `{"status":"ok","response":{"type":"createVault","data":"0x1111111111111111111111111111111111111111"}}`,
			check: func(t *testing.T, data exchange.ActionResponseData) {
				t.Helper()
				if got, ok := data.(exchange.CreateVaultResponseData); !ok || string(got) != "0x1111111111111111111111111111111111111111" {
					t.Fatalf("create vault data = %#v", data)
				}
			},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var response exchange.ActionResponse
			if err := json.Unmarshal([]byte(tc.body), &response); err != nil {
				t.Fatal(err)
			}
			tc.check(t, response.Response.Data)
		})
	}
}

func TestActionResponsePreservesNonObjectProtocolError(t *testing.T) {
	var response exchange.ActionResponse
	if err := json.Unmarshal([]byte(`{"status":"err","response":"invalid action"}`), &response); err != nil {
		t.Fatal(err)
	}
	if response.Error == nil || response.Error.Message != "invalid action" {
		t.Fatalf("protocol error = %#v", response.Error)
	}
}
