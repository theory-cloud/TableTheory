package keycontract

import (
	"fmt"
	"math"
	"strconv"
	"strings"
)

// Evaluate finds and evaluates a named derived key in a contract.
func Evaluate(contract *Contract, name string, input map[string]any) (string, error) {
	key, ok := FindDerivedKey(contract, name)
	if !ok {
		return "", fmt.Errorf("derived key not found: %s", name)
	}
	return EvaluateDerivedKey(*key, input)
}

// EvaluateDerivedKey evaluates one derived-key template. Output bytes are solely
// determined by the contract, input scalar values, and ordered transforms.
func EvaluateDerivedKey(key DerivedKey, input map[string]any) (string, error) {
	if err := validateDerivedKey(key); err != nil {
		return "", err
	}
	if input == nil {
		input = map[string]any{}
	}

	parts := make([]string, 0, len(key.Segments))
	for i, segment := range key.Segments {
		value, omit, err := evaluateSegment(key.Name, i, segment, input)
		if err != nil {
			return "", err
		}
		if omit {
			continue
		}
		parts = append(parts, segment.Prefix+value+segment.Suffix)
	}
	return strings.Join(parts, key.Join), nil
}

// VerifyFixtures evaluates all embedded fixtures and returns the first mismatch.
func VerifyFixtures(contract *Contract) error {
	if contract == nil {
		return fmt.Errorf("tabletheory model contract is nil")
	}
	for _, key := range contract.DerivedKeys {
		for _, fixture := range key.Fixtures {
			got, err := EvaluateDerivedKey(key, fixture.Input)
			if err != nil {
				return fmt.Errorf("derived key %s fixture %s: %w", key.Name, fixture.Name, err)
			}
			if got != fixture.Expect {
				return fmt.Errorf("derived key %s fixture %s: want %q got %q", key.Name, fixture.Name, fixture.Expect, got)
			}
		}
	}
	return nil
}

func evaluateSegment(keyName string, index int, segment Segment, input map[string]any) (value string, omit bool, err error) {
	label := segmentLabel(segment, index)

	value, present, err := resolveSegmentValue(segment, input)
	if err != nil {
		return "", false, fmt.Errorf("derived key %s segment %s: %w", keyName, label, err)
	}
	if shouldOmitMissingSegment(present, segment) {
		return "", true, nil
	}

	value, err = applyTransforms(value, segment.Transforms)
	if err != nil {
		return "", false, fmt.Errorf("derived key %s segment %s: %w", keyName, label, err)
	}
	value = applyDefault(value, segment)

	if shouldOmitSegment(value, segment) || shouldOmitEmptyOptional(value, segment) {
		return "", true, nil
	}
	return value, false, nil
}

func segmentLabel(segment Segment, index int) string {
	if segment.Name != "" {
		return segment.Name
	}
	return fmt.Sprintf("#%d", index+1)
}

func shouldOmitMissingSegment(present bool, segment Segment) bool {
	return !present && segment.Optional && segment.Default == nil
}

func shouldOmitEmptyOptional(value string, segment Segment) bool {
	return value == "" && segment.Optional && segment.Default == nil
}

func applyDefault(value string, segment Segment) string {
	if value == "" && segment.Default != nil {
		return *segment.Default
	}
	return value
}

func applyTransforms(value string, transforms []string) (string, error) {
	for _, transform := range transforms {
		switch transform {
		case TransformTrim:
			value = trimContractWhitespace(value)
		case TransformWildcardEmpty:
			if value == "" {
				value = "*"
			}
		default:
			return "", fmt.Errorf("unsupported transform %q", transform)
		}
	}
	return value, nil
}

func resolveSegmentValue(segment Segment, input map[string]any) (string, bool, error) {
	switch {
	case segment.Literal != nil:
		return *segment.Literal, true, nil
	case segment.Value.Literal != nil:
		return *segment.Value.Literal, true, nil
	case segment.Value.Input != "":
		value, ok := input[segment.Value.Input]
		if !ok {
			if segment.Default != nil || segment.Optional {
				return "", false, nil
			}
			return "", false, fmt.Errorf("missing required input %q", segment.Value.Input)
		}
		out, err := scalarToString(value)
		if err != nil {
			return "", true, fmt.Errorf("input %q: %w", segment.Value.Input, err)
		}
		return out, true, nil
	default:
		return "", false, fmt.Errorf("missing value source")
	}
}

func shouldOmitSegment(value string, segment Segment) bool {
	if segment.OmitWhen.Empty && value == "" {
		return true
	}
	if segment.OmitWhen.Default && segment.Default != nil && value == *segment.Default {
		return true
	}
	for _, candidate := range segment.OmitWhen.Values {
		if value == candidate {
			return true
		}
	}
	return false
}

func scalarToString(value any) (string, error) {
	switch v := value.(type) {
	case nil:
		return "", nil
	case string:
		return v, nil
	case bool:
		return strconv.FormatBool(v), nil
	case float32:
		return canonicalFloatToString(float64(v), 32)
	case float64:
		return canonicalFloatToString(v, 64)
	}
	if out, ok := integerScalarToString(value); ok {
		return out, nil
	}
	return "", fmt.Errorf("unsupported non-scalar value %T", value)
}

func integerScalarToString(value any) (string, bool) {
	switch v := value.(type) {
	case int:
		return strconv.FormatInt(int64(v), 10), true
	case int8:
		return strconv.FormatInt(int64(v), 10), true
	case int16:
		return strconv.FormatInt(int64(v), 10), true
	case int32:
		return strconv.FormatInt(int64(v), 10), true
	case int64:
		return strconv.FormatInt(v, 10), true
	case uint:
		return strconv.FormatUint(uint64(v), 10), true
	case uint8:
		return strconv.FormatUint(uint64(v), 10), true
	case uint16:
		return strconv.FormatUint(uint64(v), 10), true
	case uint32:
		return strconv.FormatUint(uint64(v), 10), true
	case uint64:
		return strconv.FormatUint(v, 10), true
	default:
		return "", false
	}
}

func trimContractWhitespace(value string) string {
	return strings.TrimFunc(value, isContractTrimWhitespace)
}

func isContractTrimWhitespace(r rune) bool {
	switch r {
	case '\u0009', // CHARACTER TABULATION
		'\u000A', // LINE FEED
		'\u000B', // LINE TABULATION
		'\u000C', // FORM FEED
		'\u000D', // CARRIAGE RETURN
		'\u0020', // SPACE
		'\u0085', // NEXT LINE
		'\u00A0', // NO-BREAK SPACE
		'\u1680', // OGHAM SPACE MARK
		'\u2000', // EN QUAD
		'\u2001', // EM QUAD
		'\u2002', // EN SPACE
		'\u2003', // EM SPACE
		'\u2004', // THREE-PER-EM SPACE
		'\u2005', // FOUR-PER-EM SPACE
		'\u2006', // SIX-PER-EM SPACE
		'\u2007', // FIGURE SPACE
		'\u2008', // PUNCTUATION SPACE
		'\u2009', // THIN SPACE
		'\u200A', // HAIR SPACE
		'\u2028', // LINE SEPARATOR
		'\u2029', // PARAGRAPH SEPARATOR
		'\u202F', // NARROW NO-BREAK SPACE
		'\u205F', // MEDIUM MATHEMATICAL SPACE
		'\u3000', // IDEOGRAPHIC SPACE
		'\uFEFF': // ZERO WIDTH NO-BREAK SPACE / BOM
		return true
	default:
		return false
	}
}

func canonicalFloatToString(value float64, bitSize int) (string, error) {
	if math.IsNaN(value) || math.IsInf(value, 0) {
		return "", fmt.Errorf("must be a finite number")
	}
	if value == 0 {
		return "0", nil
	}
	return expandExponentDecimal(strconv.FormatFloat(value, 'g', -1, bitSize)), nil
}

func expandExponentDecimal(value string) string {
	expIndex := strings.IndexAny(value, "eE")
	if expIndex < 0 {
		return value
	}

	mantissa := value[:expIndex]
	exponent, err := strconv.Atoi(value[expIndex+1:])
	if err != nil {
		return value
	}

	sign := ""
	if strings.HasPrefix(mantissa, "-") || strings.HasPrefix(mantissa, "+") {
		sign = mantissa[:1]
		mantissa = mantissa[1:]
	}

	intPart, fracPart, ok := strings.Cut(mantissa, ".")
	if !ok {
		fracPart = ""
	}
	digits := intPart + fracPart
	decimalIndex := len(intPart) + exponent

	switch {
	case decimalIndex <= 0:
		return sign + "0." + strings.Repeat("0", -decimalIndex) + digits
	case decimalIndex >= len(digits):
		return sign + digits + strings.Repeat("0", decimalIndex-len(digits))
	default:
		return sign + digits[:decimalIndex] + "." + digits[decimalIndex:]
	}
}
