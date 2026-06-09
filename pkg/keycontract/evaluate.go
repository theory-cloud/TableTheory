package keycontract

import (
	"fmt"
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
			value = strings.TrimSpace(value)
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
		return strconv.FormatFloat(float64(v), 'f', -1, 32), nil
	case float64:
		return strconv.FormatFloat(v, 'f', -1, 64), nil
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
