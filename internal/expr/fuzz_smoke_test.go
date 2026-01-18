package expr

import (
	"encoding/binary"
	"math"
	"testing"
)

func FuzzExprBuilder_NoPanics(f *testing.F) {
	f.Add("Field", uint8(0), []byte("value"))
	f.Add("UserCreatedAt", uint8(7), []byte{0, 1, 2, 3, 4, 5, 6, 7})
	f.Add("_f", uint8(11), []byte{0xff, 0x00, 0x00, 0x00})

	f.Fuzz(func(t *testing.T, rawField string, mode uint8, raw []byte) {
		field := sanitizeFieldNameForFuzz(rawField)
		value := fuzzValueForExpr(raw, mode)

		builder := NewBuilder()
		operator := fuzzOperator(mode)

		switch operator {
		case "BETWEEN":
			value = []any{fuzzValueForExpr(raw, mode), fuzzValueForExpr(raw, mode+1)}
		case "IN":
			value = []any{fuzzValueForExpr(raw, mode), fuzzValueForExpr(raw, mode+1), fuzzValueForExpr(raw, mode+2)}
		case "ATTRIBUTE_EXISTS", "ATTRIBUTE_NOT_EXISTS":
			value = nil
		}

		if err := builder.AddKeyCondition(field, operator, value); err != nil {
			_ = err
		}
		if err := builder.AddFilterCondition("AND", field, operator, value); err != nil {
			_ = err
		}
		if err := builder.AddConditionExpression(field, operator, value); err != nil {
			_ = err
		}
		if err := builder.AddUpdateSet(field, value); err != nil {
			_ = err
		}
		if err := builder.AddUpdateAdd(field, value); err != nil {
			_ = err
		}

		_, _ = builder.Build(), builder.Clone().Build()

		av, err := ConvertToAttributeValueSecure(value)
		if err == nil && av == nil {
			t.Fatalf("expected non-nil AttributeValue when err=nil")
		}
	})
}

func sanitizeFieldNameForFuzz(field string) string {
	const maxLen = 32
	if field == "" {
		return "F"
	}

	out := make([]byte, 0, maxLen)
	out = append(out, 'F') // avoid standalone keywords like "select"

	for i := 0; i < len(field) && len(out) < maxLen; i++ {
		b := field[i]
		switch {
		case b >= 'a' && b <= 'z':
			out = append(out, b)
		case b >= 'A' && b <= 'Z':
			out = append(out, b)
		case b >= '0' && b <= '9':
			out = append(out, b)
		case b == '_':
			out = append(out, b)
		default:
			out = append(out, '_')
		}
	}

	return string(out)
}

func fuzzOperator(mode uint8) string {
	operators := []string{
		"=",
		"<>",
		"<",
		"<=",
		">",
		">=",
		"BETWEEN",
		"IN",
		"BEGINS_WITH",
		"CONTAINS",
		"ATTRIBUTE_EXISTS",
		"ATTRIBUTE_NOT_EXISTS",
	}
	return operators[int(mode)%len(operators)]
}

func fuzzValueForExpr(raw []byte, mode uint8) any {
	const maxBytes = 512
	if len(raw) > maxBytes {
		raw = raw[:maxBytes]
	}

	switch mode % 10 {
	case 0:
		return nil
	case 1:
		return string(raw)
	case 2:
		b := make([]byte, len(raw))
		copy(b, raw)
		return b
	case 3:
		u := binary.LittleEndian.Uint32(padTo4(raw))
		magnitude := int64(u & 0x7fffffff)
		if u&0x80000000 != 0 {
			return -magnitude
		}
		return magnitude
	case 4:
		return binary.LittleEndian.Uint64(padTo8(raw))
	case 5:
		return math.Float64frombits(binary.LittleEndian.Uint64(padTo8(raw)))
	case 6:
		return mode%2 == 0
	case 7:
		return []string{string(raw), string(raw)}
	case 8:
		return map[string]any{"k": string(raw), "n": int64(len(raw))}
	default:
		return map[int]string{int(mode): string(raw)}
	}
}

func padTo8(b []byte) []byte {
	var out [8]byte
	copy(out[:], b)
	return out[:]
}

func padTo4(b []byte) []byte {
	var out [4]byte
	copy(out[:], b)
	return out[:]
}
