package numutil

import (
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"reflect"
	"strconv"
)

// JSONNumberType is the reflect.Type of encoding/json.Number. Marshaling code
// recognizes it and emits a DynamoDB N value so a decoded number that cannot be
// represented exactly by int64 or float64 survives a rewrite unchanged.
var JSONNumberType = reflect.TypeOf(json.Number(""))

// ParseNumber converts a DynamoDB numeric string into a Go value that preserves
// the exact persisted value. Integers that fit int64 become int64; decimals that
// a float64 reproduces exactly become float64; every other valid number becomes
// json.Number so no precision is lost. Malformed input returns an error.
func ParseNumber(value string) (any, error) {
	if intValue, err := strconv.ParseInt(value, 10, 64); err == nil {
		return intValue, nil
	}

	floatValue, floatErr := strconv.ParseFloat(value, 64)
	if floatErr == nil {
		if !math.IsInf(floatValue, 0) && !math.IsNaN(floatValue) &&
			strconv.FormatFloat(floatValue, 'f', -1, 64) == value {
			return floatValue, nil
		}
		return json.Number(value), nil
	}

	var numErr *strconv.NumError
	if errors.As(floatErr, &numErr) && errors.Is(numErr.Err, strconv.ErrRange) {
		// Syntactically valid but outside float64's finite range: keep the text.
		return json.Number(value), nil
	}

	return nil, fmt.Errorf("invalid number: %s", value)
}

// ParseNumberSet converts DynamoDB number-set elements with the same exactness
// guarantees as ParseNumber.
func ParseNumberSet(values []string) ([]any, error) {
	out := make([]any, len(values))
	for i, value := range values {
		converted, err := ParseNumber(value)
		if err != nil {
			return nil, fmt.Errorf("index %d: %w", i, err)
		}
		out[i] = converted
	}
	return out, nil
}
