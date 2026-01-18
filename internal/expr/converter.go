package expr

import (
	"fmt"
	"reflect"
	"strconv"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go-v2/service/dynamodb/types"
)

// Marshaler interface for custom marshaling
type Marshaler interface {
	MarshalDynamoDBAttributeValue() (types.AttributeValue, error)
}

// Unmarshaler interface for custom unmarshaling
type Unmarshaler interface {
	UnmarshalDynamoDBAttributeValue(av types.AttributeValue) error
}

// ConvertToAttributeValue converts a Go value to a DynamoDB AttributeValue
func ConvertToAttributeValue(value any) (types.AttributeValue, error) {
	if value == nil {
		return &types.AttributeValueMemberNULL{Value: true}, nil
	}

	// Check for custom marshaler
	if marshaler, ok := value.(Marshaler); ok {
		return marshaler.MarshalDynamoDBAttributeValue()
	}

	v := reflect.ValueOf(value)

	// Handle pointers
	if v.Kind() == reflect.Ptr {
		if v.IsNil() {
			return &types.AttributeValueMemberNULL{Value: true}, nil
		}
		return ConvertToAttributeValue(v.Elem().Interface())
	}

	switch v.Kind() {
	case reflect.String:
		return &types.AttributeValueMemberS{Value: v.String()}, nil

	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		return &types.AttributeValueMemberN{Value: fmt.Sprintf("%d", v.Int())}, nil

	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		return &types.AttributeValueMemberN{Value: fmt.Sprintf("%d", v.Uint())}, nil

	case reflect.Float32, reflect.Float64:
		return &types.AttributeValueMemberN{Value: fmt.Sprintf("%g", v.Float())}, nil

	case reflect.Bool:
		return &types.AttributeValueMemberBOOL{Value: v.Bool()}, nil

	case reflect.Slice:
		return convertSliceToAttributeValue(v)

	case reflect.Map:
		return convertMapToAttributeValue(v)

	case reflect.Struct:
		// Special handling for time.Time
		if t, ok := value.(time.Time); ok {
			return &types.AttributeValueMemberS{Value: t.Format(time.RFC3339Nano)}, nil
		}

		return convertStructToAttributeValue(v)

	default:
		return nil, fmt.Errorf("unsupported type: %v", v.Type())
	}
}

func convertSliceToAttributeValue(v reflect.Value) (types.AttributeValue, error) {
	// Handle []byte as binary
	if v.Type().Elem().Kind() == reflect.Uint8 {
		return &types.AttributeValueMemberB{Value: v.Bytes()}, nil
	}

	// Handle other slices as lists
	list := make([]types.AttributeValue, v.Len())
	for i := 0; i < v.Len(); i++ {
		item, err := ConvertToAttributeValue(v.Index(i).Interface())
		if err != nil {
			return nil, err
		}
		list[i] = item
	}
	return &types.AttributeValueMemberL{Value: list}, nil
}

func convertMapToAttributeValue(v reflect.Value) (types.AttributeValue, error) {
	// Handle map[string]any as M type
	if v.Type().Key().Kind() != reflect.String {
		return nil, fmt.Errorf("unsupported map type: %v", v.Type())
	}

	m := make(map[string]types.AttributeValue, v.Len())
	for _, key := range v.MapKeys() {
		val, err := ConvertToAttributeValue(v.MapIndex(key).Interface())
		if err != nil {
			return nil, err
		}
		m[key.String()] = val
	}
	return &types.AttributeValueMemberM{Value: m}, nil
}

func convertStructToAttributeValue(v reflect.Value) (types.AttributeValue, error) {
	// General struct marshaling
	// Note: We no longer automatically JSON-serialize structs just because they have json tags.
	// JSON serialization should only happen when explicitly requested via theorydb:"json" tag,
	// which is handled at the field level during marshaling, not here in the converter.
	m := make(map[string]types.AttributeValue)
	t := v.Type()

	for i := 0; i < v.NumField(); i++ {
		field := t.Field(i)
		if !field.IsExported() {
			continue
		}

		fieldName, theorydbTag, jsonTag, ok := marshalFieldNameAndTags(field)
		if !ok {
			continue
		}

		fieldValue := v.Field(i)
		if shouldOmitEmptyField(fieldValue, theorydbTag, jsonTag) {
			continue
		}

		av, err := ConvertToAttributeValue(fieldValue.Interface())
		if err != nil {
			return nil, fmt.Errorf("failed to convert field %s: %w", field.Name, err)
		}
		m[fieldName] = av
	}

	return &types.AttributeValueMemberM{Value: m}, nil
}

func marshalFieldNameAndTags(field reflect.StructField) (string, string, string, bool) {
	theorydbTag := field.Tag.Get("theorydb")
	jsonTag := field.Tag.Get("json")
	if theorydbTag == "-" || jsonTag == "-" {
		return "", "", "", false
	}

	fieldName := field.Name
	if theorydbTag != "" {
		fieldName = fieldNameFromTheorydbTag(fieldName, theorydbTag)
	} else if jsonTag != "" {
		fieldName = fieldNameFromJSONTag(fieldName, jsonTag)
	}

	return fieldName, theorydbTag, jsonTag, true
}

func fieldNameFromTheorydbTag(defaultName string, tag string) string {
	fieldName := defaultName

	parts := strings.Split(tag, ",")
	if len(parts) > 0 && parts[0] != "" {
		firstPart := parts[0]
		if !strings.Contains(firstPart, ":") && !isPureModifierTag(firstPart) {
			fieldName = firstPart
		}
	}

	if attrName := parseAttrTag(tag); attrName != "" {
		fieldName = attrName
	}

	return fieldName
}

func fieldNameFromJSONTag(defaultName string, jsonTag string) string {
	parts := strings.Split(jsonTag, ",")
	if len(parts) > 0 && parts[0] != "" {
		return parts[0]
	}
	return defaultName
}

func shouldOmitEmptyField(fieldValue reflect.Value, theorydbTag string, jsonTag string) bool {
	if !hasOmitEmpty(theorydbTag) && !strings.Contains(jsonTag, "omitempty") {
		return false
	}
	return isZeroValue(fieldValue)
}

// ConvertFromAttributeValue converts a DynamoDB AttributeValue to a Go value
func ConvertFromAttributeValue(av types.AttributeValue, target any) error {
	targetValue := reflect.ValueOf(target)
	if targetValue.Kind() != reflect.Ptr || targetValue.IsNil() {
		return fmt.Errorf("target must be a non-nil pointer")
	}

	// Check for custom unmarshaler
	if unmarshaler, ok := target.(Unmarshaler); ok {
		return unmarshaler.UnmarshalDynamoDBAttributeValue(av)
	}

	targetElem := targetValue.Elem()
	return unmarshalAttributeValue(av, targetElem)
}

// unmarshalAttributeValue unmarshals an AttributeValue into a reflect.Value
func unmarshalAttributeValue(av types.AttributeValue, v reflect.Value) error {
	if isEmptyInterfaceValue(v) {
		return unmarshalIntoEmptyInterface(av, v)
	}
	if v.Kind() == reflect.Ptr {
		return unmarshalIntoPointer(av, v)
	}
	return unmarshalAttributeValueNonPtr(av, v)
}

func isEmptyInterfaceValue(v reflect.Value) bool {
	return v.Kind() == reflect.Interface && v.Type().NumMethod() == 0
}

func unmarshalIntoEmptyInterface(av types.AttributeValue, v reflect.Value) error {
	val, err := attributeValueToInterface(av)
	if err != nil {
		return err
	}
	v.Set(reflect.ValueOf(val))
	return nil
}

func unmarshalIntoPointer(av types.AttributeValue, v reflect.Value) error {
	if av == nil || isNullAttributeValue(av) {
		v.Set(reflect.Zero(v.Type()))
		return nil
	}

	// Create new value if pointer is nil
	if v.IsNil() {
		v.Set(reflect.New(v.Type().Elem()))
	}
	return unmarshalAttributeValue(av, v.Elem())
}

func unmarshalAttributeValueNonPtr(av types.AttributeValue, v reflect.Value) error {
	switch av := av.(type) {
	case *types.AttributeValueMemberS:
		return unmarshalString(av.Value, v)

	case *types.AttributeValueMemberN:
		return unmarshalNumber(av.Value, v)

	case *types.AttributeValueMemberB:
		return unmarshalBinary(av.Value, v)

	case *types.AttributeValueMemberBOOL:
		return unmarshalBool(av.Value, v)

	case *types.AttributeValueMemberNULL:
		v.Set(reflect.Zero(v.Type()))
		return nil

	case *types.AttributeValueMemberL:
		return unmarshalList(av.Value, v)

	case *types.AttributeValueMemberM:
		return unmarshalMap(av.Value, v)

	case *types.AttributeValueMemberSS:
		return unmarshalStringSet(av.Value, v)

	case *types.AttributeValueMemberNS:
		return unmarshalNumberSet(av.Value, v)

	case *types.AttributeValueMemberBS:
		return unmarshalBinarySet(av.Value, v)

	default:
		return fmt.Errorf("unknown AttributeValue type: %T", av)
	}
}

// unmarshalString unmarshals a string value
func unmarshalString(s string, v reflect.Value) error {
	switch v.Kind() {
	case reflect.String:
		v.SetString(s)
		return nil

	case reflect.Struct:
		// Special handling for time.Time
		if v.Type() == reflect.TypeOf(time.Time{}) {
			t, err := time.Parse(time.RFC3339Nano, s)
			if err != nil {
				// Try other common formats
				t, err = time.Parse(time.RFC3339, s)
				if err != nil {
					return fmt.Errorf("failed to parse time: %w", err)
				}
			}
			v.Set(reflect.ValueOf(t))
			return nil
		}

		return fmt.Errorf("cannot unmarshal string into %v", v.Type())

	default:
		return fmt.Errorf("cannot unmarshal string into %v", v.Type())
	}
}

// unmarshalNumber unmarshals a number value
func unmarshalNumber(n string, v reflect.Value) error {
	switch v.Kind() {
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		i, err := strconv.ParseInt(n, 10, 64)
		if err != nil {
			return err
		}
		v.SetInt(i)
		return nil

	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		u, err := strconv.ParseUint(n, 10, 64)
		if err != nil {
			return err
		}
		v.SetUint(u)
		return nil

	case reflect.Float32, reflect.Float64:
		f, err := strconv.ParseFloat(n, 64)
		if err != nil {
			return err
		}
		v.SetFloat(f)
		return nil

	default:
		return fmt.Errorf("cannot unmarshal number into %v", v.Type())
	}
}

// unmarshalBinary unmarshals binary data
func unmarshalBinary(b []byte, v reflect.Value) error {
	if v.Kind() == reflect.Slice && v.Type().Elem().Kind() == reflect.Uint8 {
		v.SetBytes(b)
		return nil
	}
	return fmt.Errorf("cannot unmarshal binary into %v", v.Type())
}

// unmarshalBool unmarshals a boolean value
func unmarshalBool(b bool, v reflect.Value) error {
	if v.Kind() == reflect.Bool {
		v.SetBool(b)
		return nil
	}
	return fmt.Errorf("cannot unmarshal bool into %v", v.Type())
}

// unmarshalList unmarshals a list of AttributeValues
func unmarshalList(list []types.AttributeValue, v reflect.Value) error {
	if v.Kind() != reflect.Slice {
		return fmt.Errorf("cannot unmarshal list into %v", v.Type())
	}

	// Create new slice
	slice := reflect.MakeSlice(v.Type(), len(list), len(list))

	// Unmarshal each element
	for i, item := range list {
		if err := unmarshalAttributeValue(item, slice.Index(i)); err != nil {
			return fmt.Errorf("failed to unmarshal list item %d: %w", i, err)
		}
	}

	v.Set(slice)
	return nil
}

// unmarshalMap unmarshals a map of AttributeValues
func unmarshalMap(m map[string]types.AttributeValue, v reflect.Value) error {
	switch v.Kind() {
	case reflect.Map:
		return unmarshalMapIntoMap(m, v)

	case reflect.Struct:
		return unmarshalMapIntoStruct(m, v)

	default:
		return fmt.Errorf("cannot unmarshal map into %v", v.Type())
	}
}

func unmarshalMapIntoMap(m map[string]types.AttributeValue, v reflect.Value) error {
	// Ensure map is string-keyed
	if v.Type().Key().Kind() != reflect.String {
		return fmt.Errorf("map must have string keys")
	}

	// Create new map if nil
	if v.IsNil() {
		v.Set(reflect.MakeMap(v.Type()))
	}

	// Unmarshal each value
	for key, value := range m {
		mapValue := reflect.New(v.Type().Elem()).Elem()
		if err := unmarshalAttributeValue(value, mapValue); err != nil {
			return fmt.Errorf("failed to unmarshal map value for key %s: %w", key, err)
		}
		v.SetMapIndex(reflect.ValueOf(key), mapValue)
	}
	return nil
}

func unmarshalMapIntoStruct(m map[string]types.AttributeValue, v reflect.Value) error {
	t := v.Type()
	for i := 0; i < v.NumField(); i++ {
		field := t.Field(i)
		if !field.IsExported() {
			continue
		}

		fieldName := unmarshalFieldName(field)
		if fieldName == "" {
			continue
		}

		av, ok := m[fieldName]
		if !ok {
			continue
		}
		if err := unmarshalAttributeValue(av, v.Field(i)); err != nil {
			return fmt.Errorf("failed to unmarshal field %s: %w", field.Name, err)
		}
	}
	return nil
}

func unmarshalFieldName(field reflect.StructField) string {
	fieldName := field.Name

	tag := field.Tag.Get("theorydb")
	jsonTag := field.Tag.Get("json")

	if tag != "" && tag != "-" {
		return fieldNameFromTheorydbTag(fieldName, tag)
	}
	if jsonTag != "" && jsonTag != "-" {
		return fieldNameFromJSONTag(fieldName, jsonTag)
	}
	return fieldName
}

// unmarshalStringSet unmarshals a string set
func unmarshalStringSet(ss []string, v reflect.Value) error {
	if v.Kind() != reflect.Slice || v.Type().Elem().Kind() != reflect.String {
		return fmt.Errorf("cannot unmarshal string set into %v", v.Type())
	}

	slice := reflect.MakeSlice(v.Type(), len(ss), len(ss))
	for i, s := range ss {
		slice.Index(i).SetString(s)
	}
	v.Set(slice)
	return nil
}

// unmarshalNumberSet unmarshals a number set
func unmarshalNumberSet(ns []string, v reflect.Value) error {
	if v.Kind() != reflect.Slice {
		return fmt.Errorf("cannot unmarshal number set into %v", v.Type())
	}

	slice := reflect.MakeSlice(v.Type(), len(ns), len(ns))
	for i, n := range ns {
		if err := unmarshalNumber(n, slice.Index(i)); err != nil {
			return fmt.Errorf("failed to unmarshal number set item %d: %w", i, err)
		}
	}
	v.Set(slice)
	return nil
}

// unmarshalBinarySet unmarshals a binary set
func unmarshalBinarySet(bs [][]byte, v reflect.Value) error {
	if v.Kind() != reflect.Slice || v.Type().Elem().Kind() != reflect.Slice {
		return fmt.Errorf("cannot unmarshal binary set into %v", v.Type())
	}

	slice := reflect.MakeSlice(v.Type(), len(bs), len(bs))
	for i, b := range bs {
		slice.Index(i).SetBytes(b)
	}
	v.Set(slice)
	return nil
}

// Helper functions

func isNullAttributeValue(av types.AttributeValue) bool {
	if nullAV, ok := av.(*types.AttributeValueMemberNULL); ok {
		return nullAV.Value
	}
	return false
}

func parseAttrTag(tag string) string {
	// Parse "attr:name" from tag
	parts := strings.Split(tag, ",")
	for _, part := range parts {
		part = strings.TrimSpace(part)
		if strings.HasPrefix(part, "attr:") {
			return strings.TrimPrefix(part, "attr:")
		}
	}
	return ""
}

func hasOmitEmpty(tag string) bool {
	return strings.Contains(tag, "omitempty")
}

func isZeroValue(v reflect.Value) bool {
	switch v.Kind() {
	case reflect.Array, reflect.Map, reflect.Slice, reflect.String:
		return v.Len() == 0
	case reflect.Bool:
		return !v.Bool()
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		return v.Int() == 0
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		return v.Uint() == 0
	case reflect.Float32, reflect.Float64:
		return v.Float() == 0
	case reflect.Interface, reflect.Ptr:
		return v.IsNil()
	}
	return false
}

func isPureModifierTag(tag string) bool {
	// These are tags that are ONLY modifiers and never field names
	modifiers := []string{"pk", "sk", "version", "ttl", "set", "omitempty", "binary", "json", "encrypted"}
	for _, mod := range modifiers {
		if tag == mod {
			return true
		}
	}
	return false
}

// attributeValueToInterface converts an AttributeValue to a native Go type
func attributeValueToInterface(av types.AttributeValue) (any, error) {
	switch av := av.(type) {
	case *types.AttributeValueMemberS:
		return av.Value, nil

	case *types.AttributeValueMemberN:
		return parseNumberString(av.Value)

	case *types.AttributeValueMemberB:
		return av.Value, nil

	case *types.AttributeValueMemberBOOL:
		return av.Value, nil

	case *types.AttributeValueMemberNULL:
		return nil, nil

	case *types.AttributeValueMemberL:
		return attributeValueListToInterface(av.Value)

	case *types.AttributeValueMemberM:
		return attributeValueMapToInterface(av.Value)

	case *types.AttributeValueMemberSS:
		return av.Value, nil

	case *types.AttributeValueMemberNS:
		return attributeValueNumberSetToInterface(av.Value)

	case *types.AttributeValueMemberBS:
		// Convert binary set to slice of []byte
		return av.Value, nil

	default:
		return nil, fmt.Errorf("unknown AttributeValue type: %T", av)
	}
}

func parseNumberString(value string) (any, error) {
	if i, err := strconv.ParseInt(value, 10, 64); err == nil {
		return i, nil
	}
	if f, err := strconv.ParseFloat(value, 64); err == nil {
		return f, nil
	}
	return nil, fmt.Errorf("cannot parse number: %s", value)
}

func attributeValueListToInterface(list []types.AttributeValue) ([]any, error) {
	result := make([]any, len(list))
	for i, item := range list {
		val, err := attributeValueToInterface(item)
		if err != nil {
			return nil, fmt.Errorf("failed to convert list item %d: %w", i, err)
		}
		result[i] = val
	}
	return result, nil
}

func attributeValueMapToInterface(m map[string]types.AttributeValue) (map[string]any, error) {
	result := make(map[string]any, len(m))
	for k, v := range m {
		val, err := attributeValueToInterface(v)
		if err != nil {
			return nil, fmt.Errorf("failed to convert map value for key %s: %w", k, err)
		}
		result[k] = val
	}
	return result, nil
}

func attributeValueNumberSetToInterface(values []string) ([]any, error) {
	nums := make([]any, len(values))
	for i, value := range values {
		num, err := parseNumberString(value)
		if err != nil {
			return nil, fmt.Errorf("cannot parse number in set: %s", value)
		}
		nums[i] = num
	}
	return nums, nil
}
