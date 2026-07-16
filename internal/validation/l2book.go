package validation

import "fmt"

// L2BookAggregation validates Hyperliquid's documented L2 aggregation
// parameters. mantissa is available only with five significant figures.
func L2BookAggregation(nSigFigs, mantissa *int) error {
	if nSigFigs != nil && *nSigFigs != 2 && *nSigFigs != 3 && *nSigFigs != 4 && *nSigFigs != 5 {
		return fmt.Errorf("nSigFigs must be 2, 3, 4, or 5")
	}
	if mantissa == nil {
		return nil
	}
	if nSigFigs == nil || *nSigFigs != 5 {
		return fmt.Errorf("mantissa requires nSigFigs=5")
	}
	if *mantissa != 1 && *mantissa != 2 && *mantissa != 5 {
		return fmt.Errorf("mantissa must be 1, 2, or 5")
	}
	return nil
}

// CandleInterval validates the complete set of candle periods supported by
// Hyperliquid's Info and WebSocket APIs.
func CandleInterval(interval string) error {
	switch interval {
	case "1m", "3m", "5m", "15m", "30m", "1h", "2h", "4h", "8h", "12h", "1d", "3d", "1w", "1M":
		return nil
	default:
		return fmt.Errorf("unsupported candle interval %q", interval)
	}
}
