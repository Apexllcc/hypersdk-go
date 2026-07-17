package websocket

import (
	"encoding/json"
	"reflect"
	"strings"
)

func serverSubscriptionIdentity(subscription map[string]any) string {
	normalized := make(map[string]any, len(subscription))
	kind, _ := subscription["type"].(string)
	for key, value := range subscription {
		if key == "user" {
			if user, ok := value.(string); ok {
				normalized[key] = strings.ToLower(user)
				continue
			}
		}
		if serverDefaultField(kind, key, value) {
			continue
		}
		normalized[key] = value
	}
	encoded, _ := json.Marshal(normalized)
	return string(encoded)
}

func subscriptionMatchScore(request, response map[string]any) (int, bool) {
	kind, _ := request["type"].(string)
	score := 0
	for key, expected := range request {
		actual, exists := normalizedResponseField(kind, key, response)
		if !exists {
			if serverDefaultField(kind, key, expected) {
				continue
			}
			return 0, false
		}
		if !subscriptionFieldEqual(key, expected, actual) {
			return 0, false
		}
		if !serverDefaultField(kind, key, expected) {
			score++
		}
	}
	return score, true
}

func normalizedResponseField(kind, key string, response map[string]any) (any, bool) {
	if actual, exists := response[key]; exists {
		return actual, true
	}
	if kind == "spotState" && key == "isPortfolioMargin" {
		ignored, exists := response["ignorePortfolioMargin"].(bool)
		if exists {
			return !ignored, true
		}
	}
	return nil, false
}

func subscriptionFieldEqual(key string, expected, actual any) bool {
	if key == "user" {
		expectedUser, expectedOK := expected.(string)
		actualUser, actualOK := actual.(string)
		return expectedOK && actualOK && strings.EqualFold(expectedUser, actualUser)
	}
	if reflect.DeepEqual(expected, actual) {
		return true
	}
	expectedNumber, expectedOK := protocolNumber(expected)
	actualNumber, actualOK := protocolNumber(actual)
	return expectedOK && actualOK && expectedNumber == actualNumber
}

func protocolNumber(value any) (float64, bool) {
	switch number := value.(type) {
	case int:
		return float64(number), true
	case int8:
		return float64(number), true
	case int16:
		return float64(number), true
	case int32:
		return float64(number), true
	case int64:
		return float64(number), true
	case uint:
		return float64(number), true
	case uint8:
		return float64(number), true
	case uint16:
		return float64(number), true
	case uint32:
		return float64(number), true
	case uint64:
		return float64(number), true
	case float32:
		return float64(number), true
	case float64:
		return number, true
	case json.Number:
		parsed, err := number.Float64()
		return parsed, err == nil
	default:
		return 0, false
	}
}

func serverDefaultField(kind, key string, value any) bool {
	if value == nil {
		return (kind == "l2Book" && (key == "nSigFigs" || key == "mantissa")) ||
			(kind == "spotState" && (key == "isPortfolioMargin" || key == "ignorePortfolioMargin"))
	}
	flag, ok := value.(bool)
	if !ok || flag {
		return false
	}
	switch kind {
	case "l2Book":
		return key == "fast"
	case "userFills":
		return key == "aggregateByTime"
	case "spotState":
		return key == "isPortfolioMargin" || key == "ignorePortfolioMargin"
	default:
		return false
	}
}
