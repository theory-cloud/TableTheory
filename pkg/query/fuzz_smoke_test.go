package query

import (
	"encoding/binary"
	"math"
	"testing"
)

func FuzzQuery_BuildConditionExpressions_NoPanics(f *testing.F) {
	f.Add("Field", uint8(0), []byte("value"), "(:x = :y)")
	f.Add("A", uint8(7), []byte{0, 1, 2, 3, 4, 5, 6, 7}, "")

	f.Fuzz(func(t *testing.T, rawField string, mode uint8, raw []byte, rawExpr string) {
		q := &Query{}

		field := sanitizeFieldNameForFuzz(rawField)
		operator := fuzzOperator(mode)
		value := fuzzValueForQuery(raw, mode)
		rawExpr = truncateFuzzString(rawExpr, 512)

		switch operator {
		case "BETWEEN":
			value = []any{fuzzValueForQuery(raw, mode), fuzzValueForQuery(raw, mode+1)}
		case "IN":
			value = []any{fuzzValueForQuery(raw, mode), fuzzValueForQuery(raw, mode+1), fuzzValueForQuery(raw, mode+2)}
		case "ATTRIBUTE_EXISTS", "ATTRIBUTE_NOT_EXISTS":
			value = nil
		}

		q.writeConditions = []Condition{{
			Field:    field,
			Operator: operator,
			Value:    value,
		}}

		if rawExpr != "" {
			q.rawConditionExpressions = []conditionExpression{{
				Expression: rawExpr,
				Values:     map[string]any{":x": fuzzValueForQuery(raw, mode), ":y": fuzzValueForQuery(raw, mode+1)},
			}}
		}

		_, _, _, err := q.buildConditionExpression(nil, false, false, false)
		if err != nil {
			_ = err
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

func fuzzValueForQuery(raw []byte, mode uint8) any {
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

func truncateFuzzString(s string, max int) string {
	if max <= 0 || len(s) <= max {
		return s
	}
	return string(append([]byte(nil), s[:max]...))
}
