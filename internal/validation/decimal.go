// Package validation provides non-exported protocol input checks.
package validation

import (
	"fmt"
	"github.com/shopspring/decimal"
)

func Positive(field string, value decimal.Decimal) error {
	if !value.IsPositive() {
		return fmt.Errorf("%s must be positive", field)
	}
	return nil
}
