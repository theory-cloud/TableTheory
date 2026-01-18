// Package marshal provides safe marshaling for DynamoDB without unsafe operations
package marshal

import (
	"fmt"
	"reflect"
	"strconv"
	"sync"
	"time"

	"github.com/aws/aws-sdk-go-v2/service/dynamodb/types"

	"github.com/theory-cloud/tabletheory/pkg/model"
	pkgTypes "github.com/theory-cloud/tabletheory/pkg/types"
)

// SafeMarshaler provides memory-safe marshaling implementation without unsafe operations
// This is the default marshaler and should be used in production environments
type SafeMarshaler struct {
	converter *pkgTypes.Converter
	now       func() time.Time

	// Cache for reflection metadata to optimize performance
	cache sync.Map // map[reflect.Type]*safeStructMarshaler
}

// safeStructMarshaler contains cached reflection information for a struct type
type safeStructMarshaler struct {
	fields    []safeFieldMarshaler
	minFields int // Pre-calculated number of non-omitempty fields for better allocation
}

// safeFieldMarshaler contains cached information for marshaling a struct field
type safeFieldMarshaler struct {
	typ         reflect.Type
	dbName      string
	fieldIndex  []int
	omitEmpty   bool
	isSet       bool
	isCreatedAt bool
	isUpdatedAt bool
	isVersion   bool
	isTTL       bool
}

// NewSafeMarshaler creates a new safe marshaler (recommended for production)
func NewSafeMarshaler() *SafeMarshaler {
	return &SafeMarshaler{now: time.Now}
}

// NewSafeMarshalerWithConverter creates a safe marshaler that consults the provided converter
// for registered custom type conversions.
func NewSafeMarshalerWithConverter(converter *pkgTypes.Converter) *SafeMarshaler {
	return &SafeMarshaler{converter: converter, now: time.Now}
}

// MarshalItem safely marshals a model to DynamoDB AttributeValues using only reflection
// This implementation prioritizes security over performance but is still highly optimized
func (m *SafeMarshaler) MarshalItem(model any, metadata *model.Metadata) (map[string]types.AttributeValue, error) {
	v, err := derefStructValue(model)
	if err != nil {
		return nil, err
	}

	sm := m.getOrBuildSafeStructMarshaler(v.Type(), metadata)
	nowFn := m.now
	if nowFn == nil {
		nowFn = time.Now
	}
	nowStr := nowTimestampIfSafeNeeded(sm.fields, nowFn)

	return m.marshalSafeStructFields(v, sm.fields, sm.minFields, nowStr)
}

func (m *SafeMarshaler) getOrBuildSafeStructMarshaler(typ reflect.Type, metadata *model.Metadata) *safeStructMarshaler {
	cached, ok := m.cache.Load(typ)
	if !ok {
		sm := m.buildSafeStructMarshaler(typ, metadata)
		cached, _ = m.cache.LoadOrStore(typ, sm)
	}

	sm, ok := cached.(*safeStructMarshaler)
	if ok && sm != nil {
		return sm
	}

	m.cache.Delete(typ)
	sm = m.buildSafeStructMarshaler(typ, metadata)
	m.cache.Store(typ, sm)
	return sm
}

func nowTimestampIfSafeNeeded(fields []safeFieldMarshaler, now func() time.Time) string {
	for _, fm := range fields {
		if fm.isCreatedAt || fm.isUpdatedAt {
			return now().Format(time.RFC3339Nano)
		}
	}
	return ""
}

func (m *SafeMarshaler) marshalSafeStructFields(
	v reflect.Value,
	fields []safeFieldMarshaler,
	minFields int,
	nowStr string,
) (map[string]types.AttributeValue, error) {
	result := make(map[string]types.AttributeValue, minFields)

	for i := range fields {
		fm := &fields[i]

		if fm.isCreatedAt || fm.isUpdatedAt {
			result[fm.dbName] = &types.AttributeValueMemberS{Value: nowStr}
			continue
		}

		field := v.FieldByIndex(fm.fieldIndex)
		if fm.isVersion {
			version, err := versionNumberFromValue(field)
			if err != nil {
				return nil, fmt.Errorf("field %s: %w", fm.dbName, err)
			}
			result[fm.dbName] = marshalVersionNumber(version)
			continue
		}

		av, err := m.marshalValue(field, fm)
		if err != nil {
			return nil, fmt.Errorf("field %s: %w", fm.dbName, err)
		}

		if _, isNull := av.(*types.AttributeValueMemberNULL); isNull && fm.omitEmpty {
			continue
		}

		result[fm.dbName] = av
	}

	return result, nil
}

// buildSafeStructMarshaler builds a cached marshaler for a struct type using safe reflection
func (m *SafeMarshaler) buildSafeStructMarshaler(typ reflect.Type, metadata *model.Metadata) *safeStructMarshaler {
	sm := &safeStructMarshaler{
		fields:    make([]safeFieldMarshaler, 0, len(metadata.Fields)),
		minFields: 0,
	}

	for _, fieldMeta := range metadata.Fields {
		// Count non-omitempty fields for better allocation
		if !fieldMeta.OmitEmpty || fieldMeta.IsCreatedAt || fieldMeta.IsUpdatedAt || fieldMeta.IsVersion {
			sm.minFields++
		}

		// Get field information safely
		field, ok := typ.FieldByName(fieldMeta.Name)
		if !ok {
			// Try by index if name lookup fails
			if fieldMeta.Index < typ.NumField() {
				field = typ.Field(fieldMeta.Index)
			} else {
				continue // Skip invalid fields
			}
		}

		fm := safeFieldMarshaler{
			fieldIndex:  fieldMeta.IndexPath,
			dbName:      fieldMeta.DBName,
			typ:         field.Type,
			omitEmpty:   fieldMeta.OmitEmpty,
			isSet:       fieldMeta.IsSet,
			isCreatedAt: fieldMeta.IsCreatedAt,
			isUpdatedAt: fieldMeta.IsUpdatedAt,
			isVersion:   fieldMeta.IsVersion,
			isTTL:       fieldMeta.IsTTL,
		}

		sm.fields = append(sm.fields, fm)
	}

	return sm
}

// marshalValue safely marshals a reflect.Value to AttributeValue
func (m *SafeMarshaler) marshalValue(v reflect.Value, fieldMeta *safeFieldMarshaler) (types.AttributeValue, error) {
	original := v
	v, isNil := derefOptionalPointer(v)
	if isNil {
		return &types.AttributeValueMemberNULL{Value: true}, nil
	}

	if fieldMeta.omitEmpty && v.IsZero() {
		return &types.AttributeValueMemberNULL{Value: true}, nil
	}

	if av, ok, err := m.marshalTimeValue(v, fieldMeta); ok {
		return av, err
	}

	if av, ok, err := m.marshalCustomValue(original); ok {
		return av, err
	}

	return m.marshalValueByKind(v, fieldMeta)
}

func derefOptionalPointer(v reflect.Value) (reflect.Value, bool) {
	if v.Kind() != reflect.Ptr {
		return v, false
	}
	if v.IsNil() {
		return reflect.Value{}, true
	}
	return v.Elem(), false
}

func (m *SafeMarshaler) marshalCustomValue(original reflect.Value) (types.AttributeValue, bool, error) {
	if m.converter == nil || !original.IsValid() {
		return nil, false, nil
	}
	if !m.converter.HasCustomConverter(original.Type()) {
		return nil, false, nil
	}

	av, err := m.converter.ToAttributeValue(original.Interface())
	if err != nil {
		return nil, true, err
	}
	return av, true, nil
}

func (m *SafeMarshaler) marshalTimeValue(v reflect.Value, fieldMeta *safeFieldMarshaler) (types.AttributeValue, bool, error) {
	if v.Type() != timeType {
		return nil, false, nil
	}

	t, ok := v.Interface().(time.Time)
	if !ok {
		return nil, true, fmt.Errorf("expected time.Time, got %T", v.Interface())
	}
	if fieldMeta.isTTL {
		return &types.AttributeValueMemberN{Value: strconv.FormatInt(t.Unix(), 10)}, true, nil
	}
	return &types.AttributeValueMemberS{Value: t.Format(time.RFC3339Nano)}, true, nil
}

func (m *SafeMarshaler) marshalValueByKind(v reflect.Value, fieldMeta *safeFieldMarshaler) (types.AttributeValue, error) {
	switch v.Kind() {
	case reflect.String:
		return &types.AttributeValueMemberS{Value: v.String()}, nil
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		return &types.AttributeValueMemberN{Value: strconv.FormatInt(v.Int(), 10)}, nil
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		return &types.AttributeValueMemberN{Value: strconv.FormatUint(v.Uint(), 10)}, nil
	case reflect.Float32, reflect.Float64:
		return &types.AttributeValueMemberN{Value: strconv.FormatFloat(v.Float(), 'f', -1, 64)}, nil
	case reflect.Bool:
		return &types.AttributeValueMemberBOOL{Value: v.Bool()}, nil
	case reflect.Struct:
		return m.marshalStructValue(v, fieldMeta)
	case reflect.Slice:
		return m.marshalSliceValue(v, fieldMeta)
	case reflect.Map:
		return m.marshalMapValue(v)
	case reflect.Interface:
		return m.marshalInterfaceValue(v, fieldMeta)
	default:
		return nil, fmt.Errorf("unsupported type: %v", v.Kind())
	}
}

func (m *SafeMarshaler) marshalStructValue(v reflect.Value, fieldMeta *safeFieldMarshaler) (types.AttributeValue, error) {
	if v.Type() == timeType {
		t, ok := v.Interface().(time.Time)
		if !ok {
			return nil, fmt.Errorf("expected time.Time, got %T", v.Interface())
		}
		if fieldMeta.isTTL {
			return &types.AttributeValueMemberN{Value: strconv.FormatInt(t.Unix(), 10)}, nil
		}
		return &types.AttributeValueMemberS{Value: t.Format(time.RFC3339Nano)}, nil
	}

	return m.marshalStruct(v)
}

func (m *SafeMarshaler) marshalSliceValue(v reflect.Value, fieldMeta *safeFieldMarshaler) (types.AttributeValue, error) {
	if v.IsNil() {
		return &types.AttributeValueMemberNULL{Value: true}, nil
	}

	if v.Type().Elem().Kind() == reflect.String && fieldMeta.isSet {
		if v.Len() == 0 {
			return &types.AttributeValueMemberNULL{Value: true}, nil
		}
		values := make([]string, v.Len())
		for i := 0; i < v.Len(); i++ {
			values[i] = v.Index(i).String()
		}
		return &types.AttributeValueMemberSS{Value: values}, nil
	}

	return m.marshalSlice(v)
}

func (m *SafeMarshaler) marshalMapValue(v reflect.Value) (types.AttributeValue, error) {
	if v.IsNil() {
		return &types.AttributeValueMemberNULL{Value: true}, nil
	}
	return m.marshalMap(v)
}

func (m *SafeMarshaler) marshalInterfaceValue(v reflect.Value, fieldMeta *safeFieldMarshaler) (types.AttributeValue, error) {
	if v.IsNil() {
		return &types.AttributeValueMemberNULL{Value: true}, nil
	}
	return m.marshalValue(v.Elem(), fieldMeta)
}

// marshalSlice safely marshals a slice
func (m *SafeMarshaler) marshalSlice(v reflect.Value) (types.AttributeValue, error) {
	list := make([]types.AttributeValue, v.Len())
	for i := 0; i < v.Len(); i++ {
		elem, err := m.marshalValue(v.Index(i), &safeFieldMarshaler{})
		if err != nil {
			return nil, fmt.Errorf("slice index %d: %w", i, err)
		}
		list[i] = elem
	}
	return &types.AttributeValueMemberL{Value: list}, nil
}

// marshalMap safely marshals a map
func (m *SafeMarshaler) marshalMap(v reflect.Value) (types.AttributeValue, error) {
	avMap := make(map[string]types.AttributeValue, v.Len())
	for _, key := range v.MapKeys() {
		keyStr := fmt.Sprintf("%v", key.Interface())
		val, err := m.marshalValue(v.MapIndex(key), &safeFieldMarshaler{})
		if err != nil {
			return nil, fmt.Errorf("map key %s: %w", keyStr, err)
		}
		avMap[keyStr] = val
	}
	return &types.AttributeValueMemberM{Value: avMap}, nil
}

// marshalStruct safely marshals a struct as a map
func (m *SafeMarshaler) marshalStruct(v reflect.Value) (types.AttributeValue, error) {
	structMap := make(map[string]types.AttributeValue)
	typ := v.Type()

	for i := 0; i < v.NumField(); i++ {
		field := typ.Field(i)
		// Skip unexported fields for security
		if !field.IsExported() {
			continue
		}

		fieldValue := v.Field(i)
		// Skip zero values for omitempty behavior
		if fieldValue.IsZero() {
			continue
		}

		av, err := m.marshalValue(fieldValue, &safeFieldMarshaler{})
		if err != nil {
			return nil, fmt.Errorf("struct field %s: %w", field.Name, err)
		}

		structMap[field.Name] = av
	}
	return &types.AttributeValueMemberM{Value: structMap}, nil
}
