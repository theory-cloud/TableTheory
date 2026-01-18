package encryption

import (
	"encoding/base64"
	"fmt"

	"github.com/aws/aws-sdk-go-v2/service/dynamodb/types"
)

func marshalAVJSON(av types.AttributeValue) (avJSON, error) {
	if out, handled, err := marshalScalarAVJSON(av); handled {
		return out, err
	} else if err != nil {
		return avJSON{}, err
	}

	if out, handled, err := marshalCollectionAVJSON(av); handled {
		return out, err
	} else if err != nil {
		return avJSON{}, err
	}

	return avJSON{}, fmt.Errorf("unsupported attribute value type: %T", av)
}

func marshalScalarAVJSON(av types.AttributeValue) (avJSON, bool, error) {
	switch v := av.(type) {
	case *types.AttributeValueMemberS:
		s := v.Value
		return avJSON{Type: "S", S: &s}, true, nil
	case *types.AttributeValueMemberN:
		n := v.Value
		return avJSON{Type: "N", N: &n}, true, nil
	case *types.AttributeValueMemberB:
		encoded := base64.StdEncoding.EncodeToString(v.Value)
		return avJSON{Type: "B", B: &encoded}, true, nil
	case *types.AttributeValueMemberBOOL:
		val := v.Value
		return avJSON{Type: "BOOL", BOOL: &val}, true, nil
	case *types.AttributeValueMemberNULL:
		return avJSON{Type: "NULL", NULL: true}, true, nil
	default:
		return avJSON{}, false, nil
	}
}

func marshalCollectionAVJSON(av types.AttributeValue) (avJSON, bool, error) {
	switch v := av.(type) {
	case *types.AttributeValueMemberL:
		list := make([]avJSON, len(v.Value))
		for i := range v.Value {
			elem, err := marshalAVJSON(v.Value[i])
			if err != nil {
				return avJSON{}, true, err
			}
			list[i] = elem
		}
		return avJSON{Type: "L", L: list}, true, nil
	case *types.AttributeValueMemberM:
		m := make(map[string]avJSON, len(v.Value))
		for key, val := range v.Value {
			encoded, err := marshalAVJSON(val)
			if err != nil {
				return avJSON{}, true, err
			}
			m[key] = encoded
		}
		return avJSON{Type: "M", M: m}, true, nil
	case *types.AttributeValueMemberSS:
		return avJSON{Type: "SS", SS: append([]string(nil), v.Value...)}, true, nil
	case *types.AttributeValueMemberNS:
		return avJSON{Type: "NS", NS: append([]string(nil), v.Value...)}, true, nil
	case *types.AttributeValueMemberBS:
		encoded := make([]string, len(v.Value))
		for i := range v.Value {
			encoded[i] = base64.StdEncoding.EncodeToString(v.Value[i])
		}
		return avJSON{Type: "BS", BS: encoded}, true, nil
	default:
		return avJSON{}, false, nil
	}
}

func unmarshalAVJSON(enc avJSON) (types.AttributeValue, error) {
	if out, handled, err := unmarshalScalarAVJSON(enc); handled {
		return out, err
	} else if err != nil {
		return nil, err
	}

	if out, handled, err := unmarshalCollectionAVJSON(enc); handled {
		return out, err
	} else if err != nil {
		return nil, err
	}

	return nil, fmt.Errorf("unsupported encoded attribute value type: %s", enc.Type)
}

func unmarshalScalarAVJSON(enc avJSON) (types.AttributeValue, bool, error) {
	switch enc.Type {
	case "S":
		if enc.S == nil {
			return &types.AttributeValueMemberS{Value: ""}, true, nil
		}
		return &types.AttributeValueMemberS{Value: *enc.S}, true, nil
	case "N":
		if enc.N == nil {
			return &types.AttributeValueMemberN{Value: "0"}, true, nil
		}
		return &types.AttributeValueMemberN{Value: *enc.N}, true, nil
	case "B":
		if enc.B == nil {
			return &types.AttributeValueMemberB{Value: nil}, true, nil
		}
		decoded, err := base64.StdEncoding.DecodeString(*enc.B)
		if err != nil {
			return nil, true, fmt.Errorf("failed to decode binary: %w", err)
		}
		return &types.AttributeValueMemberB{Value: decoded}, true, nil
	case "BOOL":
		val := false
		if enc.BOOL != nil {
			val = *enc.BOOL
		}
		return &types.AttributeValueMemberBOOL{Value: val}, true, nil
	case "NULL":
		return &types.AttributeValueMemberNULL{Value: true}, true, nil
	default:
		return nil, false, nil
	}
}

func unmarshalCollectionAVJSON(enc avJSON) (types.AttributeValue, bool, error) {
	switch enc.Type {
	case "L":
		list := make([]types.AttributeValue, len(enc.L))
		for i := range enc.L {
			elem, err := unmarshalAVJSON(enc.L[i])
			if err != nil {
				return nil, true, err
			}
			list[i] = elem
		}
		return &types.AttributeValueMemberL{Value: list}, true, nil
	case "M":
		m := make(map[string]types.AttributeValue, len(enc.M))
		for key, val := range enc.M {
			decoded, err := unmarshalAVJSON(val)
			if err != nil {
				return nil, true, err
			}
			m[key] = decoded
		}
		return &types.AttributeValueMemberM{Value: m}, true, nil
	case "SS":
		return &types.AttributeValueMemberSS{Value: append([]string(nil), enc.SS...)}, true, nil
	case "NS":
		return &types.AttributeValueMemberNS{Value: append([]string(nil), enc.NS...)}, true, nil
	case "BS":
		decoded := make([][]byte, len(enc.BS))
		for i := range enc.BS {
			b, err := base64.StdEncoding.DecodeString(enc.BS[i])
			if err != nil {
				return nil, true, fmt.Errorf("failed to decode binary set: %w", err)
			}
			decoded[i] = b
		}
		return &types.AttributeValueMemberBS{Value: decoded}, true, nil
	default:
		return nil, false, nil
	}
}
