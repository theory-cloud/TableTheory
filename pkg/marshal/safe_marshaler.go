// Package marshal provides safe marshaling for DynamoDB without unsafe operations
package marshal

import (
	"fmt"
	"reflect"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/aws/aws-sdk-go-v2/service/dynamodb/types"

	"github.com/theory-cloud/tabletheory/v2/internal/anonymous"
	"github.com/theory-cloud/tabletheory/v2/internal/expr"
	"github.com/theory-cloud/tabletheory/v2/internal/fieldcodec"
	"github.com/theory-cloud/tabletheory/v2/pkg/model"
	"github.com/theory-cloud/tabletheory/v2/pkg/naming"
	pkgTypes "github.com/theory-cloud/tabletheory/v2/pkg/types"
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
	typ              reflect.Type
	dbName           string
	fieldIndex       []int
	namingConvention naming.Convention
	omitEmpty        bool
	inheritNaming    bool
	isJSON           bool
	isSet            bool
	isCreatedAt      bool
	isUpdatedAt      bool
	isVersion        bool
	isTTL            bool
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

		field, ok := safeStructFieldByIndexPath(typ, fieldMeta.IndexPath)
		if !ok {
			continue
		}

		fm := safeFieldMarshaler{
			fieldIndex:       fieldMeta.IndexPath,
			dbName:           fieldMeta.DBName,
			typ:              field.Type,
			namingConvention: metadata.NamingConvention,
			omitEmpty:        fieldMeta.OmitEmpty,
			inheritNaming:    true,
			isJSON:           fieldcodec.HasJSONTag(fieldMeta.Tags),
			isSet:            fieldMeta.IsSet,
			isCreatedAt:      fieldMeta.IsCreatedAt,
			isUpdatedAt:      fieldMeta.IsUpdatedAt,
			isVersion:        fieldMeta.IsVersion,
			isTTL:            fieldMeta.IsTTL,
		}

		sm.fields = append(sm.fields, fm)
	}

	return sm
}

// marshalValue safely marshals a reflect.Value to AttributeValue
func (m *SafeMarshaler) marshalValue(v reflect.Value, fieldMeta *safeFieldMarshaler) (types.AttributeValue, error) {
	original := v
	if fieldMeta != nil && fieldMeta.omitEmpty && (!v.IsValid() || v.IsZero()) {
		return &types.AttributeValueMemberNULL{Value: true}, nil
	}

	v, isNil := derefOptionalPointer(v)
	if isNil {
		return &types.AttributeValueMemberNULL{Value: true}, nil
	}

	if fieldMeta != nil && fieldMeta.isJSON {
		normalized, err := fieldcodec.NormalizeJSONReflectValue(fieldMeta.typ, v)
		if err != nil {
			return nil, err
		}
		return expr.ConvertToAttributeValueWithOptions(normalized, expr.ConvertOptions{
			FlatAnonymousEmbedEncoding: anonymous.RequestsFlatEncoding(m.converter),
		})
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
		return m.marshalMapValue(v, fieldMeta)
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

	return m.marshalStruct(v, fieldMeta)
}

func (m *SafeMarshaler) marshalSliceValue(v reflect.Value, fieldMeta *safeFieldMarshaler) (types.AttributeValue, error) {
	if v.IsNil() {
		return &types.AttributeValueMemberNULL{Value: true}, nil
	}

	isSet := fieldMeta != nil && fieldMeta.isSet
	if isSet {
		return marshalSetSliceReflect(v)
	}

	if isByteSliceType(v.Type()) {
		return &types.AttributeValueMemberB{Value: append([]byte(nil), v.Bytes()...)}, nil
	}

	return m.marshalSlice(v, fieldMeta)
}

func (m *SafeMarshaler) marshalMapValue(v reflect.Value, fieldMeta *safeFieldMarshaler) (types.AttributeValue, error) {
	if v.IsNil() {
		return &types.AttributeValueMemberNULL{Value: true}, nil
	}
	return m.marshalMap(v, fieldMeta)
}

func (m *SafeMarshaler) marshalInterfaceValue(v reflect.Value, fieldMeta *safeFieldMarshaler) (types.AttributeValue, error) {
	if v.IsNil() {
		return &types.AttributeValueMemberNULL{Value: true}, nil
	}
	childMeta := nestedFieldContext(fieldMeta)
	return m.marshalValue(v.Elem(), &childMeta)
}

// marshalSlice safely marshals a slice
func (m *SafeMarshaler) marshalSlice(v reflect.Value, fieldMeta *safeFieldMarshaler) (types.AttributeValue, error) {
	list := make([]types.AttributeValue, v.Len())
	childMeta := nestedFieldContext(fieldMeta)
	for i := 0; i < v.Len(); i++ {
		elem, err := m.marshalValue(v.Index(i), &childMeta)
		if err != nil {
			return nil, fmt.Errorf("slice index %d: %w", i, err)
		}
		list[i] = elem
	}
	return &types.AttributeValueMemberL{Value: list}, nil
}

func isByteSliceType(typ reflect.Type) bool {
	return typ.Kind() == reflect.Slice && typ.Elem().Kind() == reflect.Uint8
}

func marshalSetSliceReflect(v reflect.Value) (types.AttributeValue, error) {
	if v.IsNil() || v.Len() == 0 {
		return &types.AttributeValueMemberNULL{Value: true}, nil
	}

	elemType := v.Type().Elem()
	switch elemType.Kind() {
	case reflect.String:
		values := make([]string, v.Len())
		for i := 0; i < v.Len(); i++ {
			values[i] = v.Index(i).String()
		}
		return &types.AttributeValueMemberSS{Value: values}, nil
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64, reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64, reflect.Float32, reflect.Float64:
		values := make([]string, v.Len())
		for i := 0; i < v.Len(); i++ {
			values[i] = formatSetNumberValue(v.Index(i))
		}
		return &types.AttributeValueMemberNS{Value: values}, nil
	case reflect.Slice:
		if elemType.Elem().Kind() == reflect.Uint8 {
			values := make([][]byte, v.Len())
			for i := 0; i < v.Len(); i++ {
				values[i] = append([]byte(nil), v.Index(i).Bytes()...)
			}
			return &types.AttributeValueMemberBS{Value: values}, nil
		}
	}

	return nil, fmt.Errorf("unsupported set element type: %s", elemType)
}

func formatSetNumberValue(v reflect.Value) string {
	switch v.Kind() {
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		return strconv.FormatInt(v.Int(), 10)
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		return strconv.FormatUint(v.Uint(), 10)
	case reflect.Float32:
		return strconv.FormatFloat(v.Float(), 'f', -1, 32)
	case reflect.Float64:
		return strconv.FormatFloat(v.Float(), 'f', -1, 64)
	default:
		return fmt.Sprintf("%v", v.Interface())
	}
}

// marshalMap safely marshals a map
func (m *SafeMarshaler) marshalMap(v reflect.Value, fieldMeta *safeFieldMarshaler) (types.AttributeValue, error) {
	avMap := make(map[string]types.AttributeValue, v.Len())
	childMeta := nestedFieldContext(fieldMeta)
	for _, key := range v.MapKeys() {
		keyStr := fmt.Sprintf("%v", key.Interface())
		val, err := m.marshalValue(v.MapIndex(key), &childMeta)
		if err != nil {
			return nil, fmt.Errorf("map key %s: %w", keyStr, err)
		}
		avMap[keyStr] = val
	}
	return &types.AttributeValueMemberM{Value: avMap}, nil
}

// marshalStruct safely marshals a struct as a map
func (m *SafeMarshaler) marshalStruct(v reflect.Value, fieldMeta *safeFieldMarshaler) (types.AttributeValue, error) {
	typ := v.Type()
	convention := resolveNestedNamingConvention(typ, fieldMeta)
	flattenAnonymousEmbeds := anonymous.RequestsFlatEncoding(m.converter)
	fieldPlans, err := buildMarshalVisibleFieldPlans(typ, m.converter)
	if err != nil {
		return nil, err
	}

	structMap := make(map[string]types.AttributeValue, len(fieldPlans))
	childMeta := safeFieldMarshaler{
		namingConvention: convention,
		inheritNaming:    true,
	}

	for _, fieldPlan := range fieldPlans {
		fieldValue := v.FieldByIndex(fieldPlan.IndexPath)
		// Skip zero values for omitempty behavior
		if fieldValue.IsZero() {
			continue
		}

		av, err := m.marshalValue(fieldValue, &childMeta)
		if err != nil {
			return nil, fmt.Errorf("struct field %s: %w", fieldPlan.Field.Name, err)
		}

		attrName, skip := resolveNestedFieldName(fieldPlan.Field, convention)
		if skip {
			continue
		}
		containerNames, skip := anonymous.MarshalContainerNamesForField(typ, fieldPlan.IndexPath, func(field reflect.StructField) (string, bool) {
			return resolveNestedFieldName(field, convention)
		}, flattenAnonymousEmbeds)
		if skip {
			continue
		}
		if err := anonymous.SetMarshaledAttributeValue(structMap, containerNames, attrName, av, flattenAnonymousEmbeds); err != nil {
			return nil, fmt.Errorf("struct field %s: %w", fieldPlan.Field.Name, err)
		}
	}
	return &types.AttributeValueMemberM{Value: structMap}, nil
}

func safeStructFieldByIndexPath(typ reflect.Type, indexPath []int) (field reflect.StructField, ok bool) {
	defer func() {
		if recover() != nil {
			field = reflect.StructField{}
			ok = false
		}
	}()

	if len(indexPath) == 0 {
		return reflect.StructField{}, false
	}

	return typ.FieldByIndex(indexPath), true
}

func nestedFieldContext(fieldMeta *safeFieldMarshaler) safeFieldMarshaler {
	if fieldMeta == nil {
		return safeFieldMarshaler{}
	}

	return safeFieldMarshaler{
		namingConvention: fieldMeta.namingConvention,
		inheritNaming:    fieldMeta.inheritNaming,
	}
}

func resolveNestedNamingConvention(typ reflect.Type, fieldMeta *safeFieldMarshaler) naming.Convention {
	if convention, ok := explicitNestedNamingConvention(typ); ok {
		return convention
	}

	if fieldMeta != nil && fieldMeta.inheritNaming {
		return fieldMeta.namingConvention
	}

	return naming.CamelCase
}

func explicitNestedNamingConvention(typ reflect.Type) (naming.Convention, bool) {
	for i := 0; i < typ.NumField(); i++ {
		tag := typ.Field(i).Tag.Get("theorydb")
		if tag == "" {
			continue
		}

		parts := strings.Split(tag, ",")
		for _, part := range parts {
			part = strings.TrimSpace(part)
			if !strings.HasPrefix(part, "naming:") {
				continue
			}

			switch strings.TrimPrefix(part, "naming:") {
			case "snake_case":
				return naming.SnakeCase, true
			case "camel_case", "camelCase":
				return naming.CamelCase, true
			case "pascal_case", "pascalCase":
				return naming.PascalCase, true
			case "dynamorm", "legacy_dynamorm", "legacyDynamORM":
				return naming.DynamORM, true
			}
		}
	}

	return naming.CamelCase, false
}

func resolveNestedFieldName(field reflect.StructField, convention naming.Convention) (string, bool) {
	theorydbTag := field.Tag.Get("theorydb")
	jsonTag := field.Tag.Get("json")
	if theorydbTag == "-" || jsonTag == "-" {
		return "", true
	}

	if theorydbTag != "" {
		return naming.ResolveAttrNameWithConvention(field, convention)
	}

	if jsonTag != "" {
		return safeJSONTagName(jsonTag), false
	}

	return naming.ConvertAttrName(field.Name, convention), false
}

func safeJSONTagName(tag string) string {
	commaIdx := strings.IndexByte(tag, ',')
	if commaIdx > 0 {
		return tag[:commaIdx]
	}
	return tag
}
