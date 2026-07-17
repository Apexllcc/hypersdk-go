package hyperliquid

import (
	"fmt"
	"github.com/Apexllcc/hypersdk-go/internal/hlerr"
)

var ErrInvalidNetwork = hlerr.ErrInvalidNetwork
var ErrUnexpectedResponse = hlerr.ErrUnexpectedResponse

type APIError = hlerr.APIError

// ValidationError identifies an invalid public API input.
type ValidationError struct {
	Field   string
	Message string
}

func (e *ValidationError) Error() string { return fmt.Sprintf("invalid %s: %s", e.Field, e.Message) }
