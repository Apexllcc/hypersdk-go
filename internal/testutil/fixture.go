// Package testutil supports package-level protocol fixtures.
package testutil

import "testing"

func Must[T any](t testing.TB, value T, err error) T {
	t.Helper()
	if err != nil {
		t.Fatal(err)
	}
	return value
}
