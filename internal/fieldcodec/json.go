package fieldcodec

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"reflect"
	"strconv"
	"strings"

	"github.com/aws/aws-sdk-go-v2/service/dynamodb/types"
)

// HasJSONTag reports whether parsed field metadata includes the json modifier.
func HasJSONTag(tags map[string]string) bool {
	if len(tags) == 0 {
		return false
	}
	_, ok := tags["json"]
	return ok
}

// HasJSONModifier reports whether a raw theorydb tag includes the json modifier.
func HasJSONModifier(tag string) bool {
	return HasModifier(tag, "json")
}

// HasModifier reports whether a raw comma-separated tag contains an exact
// standalone modifier token. Substring matches are deliberately rejected
// because modifiers such as omitempty can control destructive update actions.
func HasModifier(tag, modifier string) bool {
	if modifier == "" {
		return false
	}
	for _, part := range strings.Split(tag, ",") {
		if strings.TrimSpace(part) == modifier {
			return true
		}
	}
	return false
}

// HasKeyRoleModifier reports whether a theorydb tag declares the requested
// key role either as a standalone token or in the legacy gsi:Name:pk /
// lsi:Name:sk form. Attribute names containing the same substring do not
// match.
func HasKeyRoleModifier(tag, role string) bool {
	if role != "pk" && role != "sk" {
		return false
	}
	if HasModifier(tag, role) {
		return true
	}
	for _, part := range strings.Split(tag, ",") {
		token := strings.TrimSpace(part)
		if (strings.HasPrefix(token, "gsi:") || strings.HasPrefix(token, "lsi:")) &&
			strings.HasSuffix(token, ":"+role) {
			return true
		}
	}
	return false
}

// NormalizeJSONFieldValue converts a json-tagged field value into the canonical
// Go shape used by DynamoDB conversion. Structured fields are normalized into
// generic JSON-compatible maps/slices/scalars; string-like fields retain text.
func NormalizeJSONFieldValue(fieldType reflect.Type, value any) (any, error) {
	if isNilLike(value) {
		return nil, nil
	}

	if isJSONStringCarrierType(fieldType) {
		return normalizeJSONStringCarrierValue(value)
	}

	switch v := value.(type) {
	case string:
		if fieldType == nil {
			return v, nil
		}
		return decodeJSONTextToGeneric([]byte(v))
	case []byte:
		if fieldType == nil {
			return string(v), nil
		}
		return decodeJSONTextToGeneric(v)
	case json.RawMessage:
		if fieldType == nil {
			return string(v), nil
		}
		return decodeJSONTextToGeneric([]byte(v))
	default:
		return normalizeStructuredJSONValue(value)
	}
}

// NormalizeJSONReflectValue mirrors NormalizeJSONFieldValue for reflect-based
// callers handling struct fields.
func NormalizeJSONReflectValue(fieldType reflect.Type, value reflect.Value) (any, error) {
	if !value.IsValid() {
		return nil, nil
	}

	for value.Kind() == reflect.Interface || value.Kind() == reflect.Ptr {
		if value.IsNil() {
			return nil, nil
		}
		value = value.Elem()
	}

	return NormalizeJSONFieldValue(fieldType, value.Interface())
}

// UnmarshalJSONFieldValue decodes a json-tagged DynamoDB attribute value into a
// field destination. Structured fields accept both native document values and
// legacy JSON strings; string-like fields accept both raw strings and native
// documents re-encoded as JSON text.
func UnmarshalJSONFieldValue(av types.AttributeValue, dest reflect.Value, fallback func() error) error {
	if !dest.IsValid() {
		return fmt.Errorf("destination is invalid")
	}

	if isNullAttributeValue(av) {
		dest.Set(reflect.Zero(dest.Type()))
		return nil
	}

	if dest.Kind() == reflect.Ptr {
		if dest.IsNil() {
			dest.Set(reflect.New(dest.Type().Elem()))
		}
		return UnmarshalJSONFieldValue(av, dest.Elem(), fallback)
	}

	if isJSONStringCarrierType(dest.Type()) {
		return setJSONStringCarrierValue(av, dest)
	}

	if stringAV, ok := av.(*types.AttributeValueMemberS); ok {
		if err := decodeJSONTextIntoValue([]byte(stringAV.Value), dest); err == nil {
			return nil
		}
	}

	if fallback != nil {
		return fallback()
	}

	return fmt.Errorf("cannot unmarshal json field into %s", dest.Type())
}

func normalizeJSONStringCarrierValue(value any) (any, error) {
	switch v := value.(type) {
	case string:
		return v, nil
	case []byte:
		return string(v), nil
	case json.RawMessage:
		return string(v), nil
	default:
		data, err := json.Marshal(value)
		if err != nil {
			return nil, fmt.Errorf("marshal json field: %w", err)
		}
		return string(data), nil
	}
}

func normalizeStructuredJSONValue(value any) (any, error) {
	data, err := json.Marshal(value)
	if err != nil {
		return nil, fmt.Errorf("marshal json field: %w", err)
	}
	return decodeJSONTextToGeneric(data)
}

func decodeJSONTextToGeneric(data []byte) (any, error) {
	data = bytes.TrimSpace(data)
	if len(data) == 0 {
		return nil, fmt.Errorf("json text cannot be empty")
	}

	var out any
	dec := json.NewDecoder(bytes.NewReader(data))
	dec.UseNumber()
	if err := dec.Decode(&out); err != nil {
		return nil, fmt.Errorf("decode json field: %w", err)
	}
	if err := ensureJSONEOF(dec); err != nil {
		return nil, err
	}

	return normalizeDecodedJSONNumbers(out), nil
}

func decodeJSONTextIntoValue(data []byte, dest reflect.Value) error {
	data = bytes.TrimSpace(data)
	if len(data) == 0 {
		return fmt.Errorf("json text cannot be empty")
	}

	if isDynamicJSONType(dest.Type()) {
		value, err := decodeJSONTextToGeneric(data)
		if err != nil {
			return err
		}
		return assignDynamicJSONValue(dest, value)
	}

	tmp := reflect.New(dest.Type())
	dec := json.NewDecoder(bytes.NewReader(data))
	dec.UseNumber()
	if err := dec.Decode(tmp.Interface()); err != nil {
		return fmt.Errorf("decode json field: %w", err)
	}
	if err := ensureJSONEOF(dec); err != nil {
		return err
	}

	dest.Set(tmp.Elem())
	return nil
}

func ensureJSONEOF(dec *json.Decoder) error {
	var extra any
	if err := dec.Decode(&extra); err != io.EOF {
		if err == nil {
			return fmt.Errorf("decode json field: trailing data")
		}
		return fmt.Errorf("decode json field: trailing data: %w", err)
	}
	return nil
}

func assignDynamicJSONValue(dest reflect.Value, value any) error {
	if value == nil {
		dest.Set(reflect.Zero(dest.Type()))
		return nil
	}

	converted := reflect.ValueOf(value)
	if converted.Type().AssignableTo(dest.Type()) {
		dest.Set(converted)
		return nil
	}
	if converted.Type().ConvertibleTo(dest.Type()) {
		dest.Set(converted.Convert(dest.Type()))
		return nil
	}

	return fmt.Errorf("cannot assign %T to %s", value, dest.Type())
}

func isJSONStringCarrierType(typ reflect.Type) bool {
	typ = derefType(typ)
	if typ == nil {
		return false
	}

	if typ.Kind() == reflect.String {
		return true
	}

	return typ.Kind() == reflect.Slice && typ.Elem().Kind() == reflect.Uint8
}

func isDynamicJSONType(typ reflect.Type) bool {
	typ = derefType(typ)
	if typ == nil {
		return false
	}

	switch typ.Kind() {
	case reflect.Interface:
		return typ.NumMethod() == 0
	case reflect.Map:
		return typ.Key().Kind() == reflect.String &&
			typ.Elem().Kind() == reflect.Interface &&
			typ.Elem().NumMethod() == 0
	case reflect.Slice:
		return typ.Elem().Kind() == reflect.Interface &&
			typ.Elem().NumMethod() == 0
	default:
		return false
	}
}

func derefType(typ reflect.Type) reflect.Type {
	for typ != nil && typ.Kind() == reflect.Ptr {
		typ = typ.Elem()
	}
	return typ
}

func isNilLike(value any) bool {
	if value == nil {
		return true
	}

	v := reflect.ValueOf(value)
	switch v.Kind() {
	case reflect.Chan, reflect.Func, reflect.Interface, reflect.Map, reflect.Ptr, reflect.Slice:
		return v.IsNil()
	default:
		return false
	}
}

func setJSONStringCarrierValue(av types.AttributeValue, dest reflect.Value) error {
	if dest.Kind() == reflect.Ptr {
		if isNullAttributeValue(av) {
			dest.Set(reflect.Zero(dest.Type()))
			return nil
		}
		if dest.IsNil() {
			dest.Set(reflect.New(dest.Type().Elem()))
		}
		return setJSONStringCarrierValue(av, dest.Elem())
	}

	var data []byte
	switch v := av.(type) {
	case *types.AttributeValueMemberS:
		data = []byte(v.Value)
	default:
		jsonBytes, err := attributeValueToJSONBytes(av)
		if err != nil {
			return err
		}
		data = jsonBytes
	}

	switch {
	case dest.Kind() == reflect.String:
		dest.SetString(string(data))
		return nil
	case dest.Kind() == reflect.Slice && dest.Type().Elem().Kind() == reflect.Uint8:
		dest.Set(reflect.ValueOf(append([]byte(nil), data...)).Convert(dest.Type()))
		return nil
	default:
		return fmt.Errorf("unsupported json string destination %s", dest.Type())
	}
}

func attributeValueToJSONBytes(av types.AttributeValue) ([]byte, error) {
	value, err := attributeValueToJSONCompatible(av)
	if err != nil {
		return nil, err
	}

	data, err := json.Marshal(value)
	if err != nil {
		return nil, fmt.Errorf("marshal attribute value json: %w", err)
	}
	return data, nil
}

func attributeValueToJSONCompatible(av types.AttributeValue) (any, error) {
	switch v := av.(type) {
	case *types.AttributeValueMemberS:
		return v.Value, nil
	case *types.AttributeValueMemberN:
		return normalizeDecodedJSONNumbers(json.Number(v.Value)), nil
	case *types.AttributeValueMemberBOOL:
		return v.Value, nil
	case *types.AttributeValueMemberNULL:
		return nil, nil
	case *types.AttributeValueMemberL:
		return attributeValueListToJSONCompatible(v.Value)
	case *types.AttributeValueMemberM:
		return attributeValueMapToJSONCompatible(v.Value)
	case *types.AttributeValueMemberSS:
		return append([]string(nil), v.Value...), nil
	case *types.AttributeValueMemberNS:
		return attributeValueNumberSetToJSONCompatible(v.Value), nil
	case *types.AttributeValueMemberBS:
		return attributeValueBinarySetToJSONCompatible(v.Value), nil
	case *types.AttributeValueMemberB:
		return append([]byte(nil), v.Value...), nil
	default:
		return nil, fmt.Errorf("unsupported attribute value type %T", av)
	}
}

func attributeValueListToJSONCompatible(values []types.AttributeValue) ([]any, error) {
	out := make([]any, len(values))
	for i, item := range values {
		converted, err := attributeValueToJSONCompatible(item)
		if err != nil {
			return nil, err
		}
		out[i] = converted
	}
	return out, nil
}

func attributeValueMapToJSONCompatible(values map[string]types.AttributeValue) (map[string]any, error) {
	out := make(map[string]any, len(values))
	for key, item := range values {
		converted, err := attributeValueToJSONCompatible(item)
		if err != nil {
			return nil, err
		}
		out[key] = converted
	}
	return out, nil
}

func attributeValueNumberSetToJSONCompatible(values []string) []any {
	out := make([]any, len(values))
	for i, item := range values {
		out[i] = normalizeDecodedJSONNumbers(json.Number(item))
	}
	return out
}

func attributeValueBinarySetToJSONCompatible(values [][]byte) [][]byte {
	out := make([][]byte, len(values))
	for i, item := range values {
		out[i] = append([]byte(nil), item...)
	}
	return out
}

func normalizeDecodedJSONNumbers(value any) any {
	switch v := value.(type) {
	case map[string]any:
		for key, item := range v {
			v[key] = normalizeDecodedJSONNumbers(item)
		}
		return v
	case []any:
		for i, item := range v {
			v[i] = normalizeDecodedJSONNumbers(item)
		}
		return v
	case json.Number:
		s := v.String()
		if strings.ContainsAny(s, ".eE") {
			if f, err := v.Float64(); err == nil {
				return f
			}
			return s
		}
		if i, err := v.Int64(); err == nil {
			return i
		}
		if u, err := strconv.ParseUint(s, 10, 64); err == nil {
			return u
		}
		if f, err := v.Float64(); err == nil {
			return f
		}
		return s
	default:
		return value
	}
}

func isNullAttributeValue(av types.AttributeValue) bool {
	if av == nil {
		return true
	}
	nullAV, ok := av.(*types.AttributeValueMemberNULL)
	return ok && nullAV.Value
}
