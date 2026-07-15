// Package hlerr contains error definitions shared by leaf packages.
package hlerr

import (
	"errors"
	"fmt"
)

var ErrInvalidNetwork = errors.New("invalid hyperliquid network")
var ErrUnexpectedResponse = errors.New("unexpected API response")

type APIError struct {
	StatusCode int
	Code       string
	Message    string
	Body       []byte
}

func (e *APIError) Error() string {
	if e.Code != "" {
		return fmt.Sprintf("hyperliquid API error (%d, %s): %s", e.StatusCode, e.Code, e.Message)
	}
	return fmt.Sprintf("hyperliquid API error (%d): %s", e.StatusCode, e.Message)
}
