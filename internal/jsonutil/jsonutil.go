// Package jsonutil contains shared JSON decoding helpers.
package jsonutil

import (
	"encoding/json"
	"fmt"
)

func Decode(data []byte, target any) error {
	if len(data) == 0 {
		return fmt.Errorf("empty JSON response")
	}
	return json.Unmarshal(data, target)
}
