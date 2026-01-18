// Package marshal provides optimized marshaling for DynamoDB
package marshal

import (
	"fmt"
	"reflect"
	"strconv"
	"strings"
	"sync"
	"time"
	"unsafe"

	"github.com/aws/aws-sdk-go-v2/service/dynamodb/types"

	"github.com/theory-cloud/tabletheory/pkg/model"
	"github.com/theory-cloud/tabletheory/pkg/naming"
	pkgTypes "github.com/theory-cloud/tabletheory/pkg/types"
)

// Marshaler provides high-performance marshaling to DynamoDB AttributeValues
type Marshaler struct {
	converter        *pkgTypes.Converter
	now              func() time.Time
	cache            sync.Map
	mu               sync.Mutex
	namingConvention naming.Convention
}

var timeType = reflect.TypeOf(time.Time{})

// structMarshaler contains cached information for marshaling a specific struct type
type structMarshaler struct {
	fields []fieldMarshaler
	// Pre-calculated number of non-omitempty fields for better allocation
	minFields int
}

// fieldMarshaler contains cached information for marshaling a struct field
type fieldMarshaler struct {
	typ         reflect.Type
	marshalFunc func(unsafe.Pointer) (types.AttributeValue, error)
	dbName      string
	index       int
	offset      uintptr
	omitEmpty   bool
	isSet       bool
	isCreatedAt bool
	isUpdatedAt bool
	isVersion   bool
	isTTL       bool
}

func fieldOffsetForIndexPath(root reflect.Type, indexPath []int) uintptr {
	var offset uintptr
	t := root
	for _, idx := range indexPath {
		field := t.Field(idx)
		offset += field.Offset
		t = field.Type
	}
	return offset
}

// New creates a new optimized marshaler. If a converter is provided it will
// be consulted for custom type conversions during marshaling.
func New(converter *pkgTypes.Converter) *Marshaler {
	return &Marshaler{
		converter: converter,
		now:       time.Now,
	}
}

// ClearCache removes all cached struct marshalers. This is useful when new
// custom converters are registered and previously compiled functions need to
// be rebuilt.
func (m *Marshaler) ClearCache() {
	if m == nil {
		return
	}

	m.cache.Range(func(key, _ any) bool {
		m.cache.Delete(key)
		return true
	})
}

func derefStructValue(model any) (reflect.Value, error) {
	v := reflect.ValueOf(model)
	if v.Kind() == reflect.Ptr {
		if v.IsNil() {
			return reflect.Value{}, fmt.Errorf("cannot marshal nil pointer")
		}
		v = v.Elem()
	}

	if v.Kind() != reflect.Struct {
		return reflect.Value{}, fmt.Errorf("model must be a struct or pointer to struct")
	}

	return v, nil
}

// MarshalItem marshals a model to DynamoDB AttributeValues using cached reflection
func (m *Marshaler) MarshalItem(model any, metadata *model.Metadata) (map[string]types.AttributeValue, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	v, err := derefStructValue(model)
	if err != nil {
		return nil, err
	}

	// Set naming convention from metadata for nested struct marshaling
	if metadata != nil {
		m.namingConvention = metadata.NamingConvention
	}

	sm := m.getOrBuildStructMarshaler(v.Type(), metadata)

	// Pre-allocate result map with estimated size
	result := make(map[string]types.AttributeValue, sm.minFields)

	ptr := structUnsafePointer(v)
	nowFn := m.now
	if nowFn == nil {
		nowFn = time.Now
	}
	nowStr := nowTimestampIfNeeded(sm.fields, nowFn)

	if err := m.marshalStructFields(ptr, sm.fields, result, nowStr); err != nil {
		return nil, err
	}

	return result, nil
}

func (m *Marshaler) getOrBuildStructMarshaler(typ reflect.Type, metadata *model.Metadata) *structMarshaler {
	cached, ok := m.cache.Load(typ)
	if !ok {
		sm := m.buildStructMarshaler(typ, metadata)
		cached, _ = m.cache.LoadOrStore(typ, sm)
	}

	sm, ok := cached.(*structMarshaler)
	if ok && sm != nil {
		return sm
	}

	sm = m.buildStructMarshaler(typ, metadata)
	m.cache.Store(typ, sm)
	return sm
}

func structUnsafePointer(v reflect.Value) unsafe.Pointer {
	if v.CanAddr() {
		return unsafe.Pointer(v.UnsafeAddr()) // #nosec G103 -- performance-critical marshaling uses verified field offsets
	}

	vcopy := reflect.New(v.Type()).Elem()
	vcopy.Set(v)
	return unsafe.Pointer(vcopy.UnsafeAddr()) // #nosec G103 -- performance-critical marshaling uses verified field offsets
}

func nowTimestampIfNeeded(fields []fieldMarshaler, now func() time.Time) string {
	for _, fm := range fields {
		if fm.isCreatedAt || fm.isUpdatedAt {
			return now().Format(time.RFC3339Nano)
		}
	}
	return ""
}

func (m *Marshaler) marshalStructFields(ptr unsafe.Pointer, fields []fieldMarshaler, result map[string]types.AttributeValue, nowStr string) error {
	for _, fm := range fields {
		if fm.isCreatedAt || fm.isUpdatedAt {
			result[fm.dbName] = &types.AttributeValueMemberS{Value: nowStr}
			continue
		}

		if fm.isVersion {
			fieldPtr := unsafe.Add(ptr, fm.offset)
			fieldValue := reflect.NewAt(fm.typ, fieldPtr).Elem()
			version, err := versionNumberFromValue(fieldValue)
			if err != nil {
				return fmt.Errorf("field %s: %w", fm.dbName, err)
			}
			result[fm.dbName] = marshalVersionNumber(version)
			continue
		}

		if fm.marshalFunc == nil {
			continue
		}

		fieldPtr := unsafe.Add(ptr, fm.offset)
		av, err := fm.marshalFunc(fieldPtr)
		if err != nil {
			return fmt.Errorf("field %s: %w", fm.dbName, err)
		}

		if _, isNull := av.(*types.AttributeValueMemberNULL); isNull && fm.omitEmpty {
			continue
		}

		result[fm.dbName] = av
	}

	return nil
}

func marshalVersionNumber(val int64) types.AttributeValue {
	if val == 0 {
		return &types.AttributeValueMemberN{Value: "0"}
	}
	return &types.AttributeValueMemberN{Value: strconv.FormatInt(val, 10)}
}

// buildStructMarshaler builds a cached marshaler for a struct type
func (m *Marshaler) buildStructMarshaler(typ reflect.Type, metadata *model.Metadata) *structMarshaler {
	sm := &structMarshaler{
		fields:    make([]fieldMarshaler, 0, len(metadata.Fields)),
		minFields: 0,
	}

	for _, fieldMeta := range metadata.Fields {
		field := typ.FieldByIndex(fieldMeta.IndexPath)

		// Count non-omitempty fields
		if !fieldMeta.OmitEmpty || fieldMeta.IsCreatedAt || fieldMeta.IsUpdatedAt || fieldMeta.IsVersion {
			sm.minFields++
		}

		fm := fieldMarshaler{
			index:       fieldMeta.Index,
			dbName:      fieldMeta.DBName,
			offset:      fieldOffsetForIndexPath(typ, fieldMeta.IndexPath),
			typ:         field.Type,
			omitEmpty:   fieldMeta.OmitEmpty,
			isSet:       fieldMeta.IsSet,
			isCreatedAt: fieldMeta.IsCreatedAt,
			isUpdatedAt: fieldMeta.IsUpdatedAt,
			isVersion:   fieldMeta.IsVersion,
			isTTL:       fieldMeta.IsTTL,
		}

		// Prefer registered custom converters when available so callers can
		// override marshaling behavior for specific types.
		if m.hasCustomConverter(field.Type) {
			fm.marshalFunc = m.buildCustomConverterMarshalFunc(field.Type)
			sm.fields = append(sm.fields, fm)
			continue
		}

		// Build type-specific marshal function
		fm.marshalFunc = m.buildMarshalFunc(field.Type, fieldMeta)

		sm.fields = append(sm.fields, fm)
	}

	return sm
}

// buildMarshalFunc builds a type-specific marshal function
func (m *Marshaler) buildMarshalFunc(typ reflect.Type, fieldMeta *model.FieldMetadata) func(unsafe.Pointer) (types.AttributeValue, error) {
	if typ.Kind() == reflect.Ptr {
		return m.buildPointerMarshalFunc(typ, fieldMeta)
	}

	if scalarFunc := buildScalarMarshalFunc(typ, fieldMeta); scalarFunc != nil {
		return scalarFunc
	}

	switch typ.Kind() {
	case reflect.Struct:
		return m.buildStructMarshalFunc(typ, fieldMeta)
	case reflect.Slice:
		return m.buildSliceMarshalFunc(typ, fieldMeta)
	case reflect.Map:
		return m.buildMapMarshalFunc(typ, fieldMeta)
	default:
		// Fall back to reflection for complex types
		return m.buildReflectMarshalFunc(typ, fieldMeta)
	}
}

func (m *Marshaler) buildPointerMarshalFunc(typ reflect.Type, fieldMeta *model.FieldMetadata) func(unsafe.Pointer) (types.AttributeValue, error) {
	elemFunc := m.buildMarshalFunc(typ.Elem(), fieldMeta)
	return func(ptr unsafe.Pointer) (types.AttributeValue, error) {
		p := *(*unsafe.Pointer)(ptr)
		if p == nil {
			return &types.AttributeValueMemberNULL{Value: true}, nil
		}
		return elemFunc(p)
	}
}

func buildScalarMarshalFunc(typ reflect.Type, fieldMeta *model.FieldMetadata) func(unsafe.Pointer) (types.AttributeValue, error) {
	switch typ.Kind() {
	case reflect.String:
		return func(ptr unsafe.Pointer) (types.AttributeValue, error) {
			s := *(*string)(ptr)
			if s == "" && fieldMeta.OmitEmpty {
				return &types.AttributeValueMemberNULL{Value: true}, nil
			}
			return &types.AttributeValueMemberS{Value: s}, nil
		}
	case reflect.Int:
		return func(ptr unsafe.Pointer) (types.AttributeValue, error) {
			i := *(*int)(ptr)
			if i == 0 && fieldMeta.OmitEmpty {
				return &types.AttributeValueMemberNULL{Value: true}, nil
			}
			return &types.AttributeValueMemberN{Value: strconv.Itoa(i)}, nil
		}
	case reflect.Int64:
		return func(ptr unsafe.Pointer) (types.AttributeValue, error) {
			i := *(*int64)(ptr)
			if i == 0 && fieldMeta.OmitEmpty {
				return &types.AttributeValueMemberNULL{Value: true}, nil
			}
			return &types.AttributeValueMemberN{Value: strconv.FormatInt(i, 10)}, nil
		}
	case reflect.Float64:
		return func(ptr unsafe.Pointer) (types.AttributeValue, error) {
			f := *(*float64)(ptr)
			if f == 0 && fieldMeta.OmitEmpty {
				return &types.AttributeValueMemberNULL{Value: true}, nil
			}
			return &types.AttributeValueMemberN{Value: strconv.FormatFloat(f, 'f', -1, 64)}, nil
		}
	case reflect.Bool:
		return func(ptr unsafe.Pointer) (types.AttributeValue, error) {
			b := *(*bool)(ptr)
			return &types.AttributeValueMemberBOOL{Value: b}, nil
		}
	default:
		return nil
	}
}

func (m *Marshaler) buildStructMarshalFunc(typ reflect.Type, fieldMeta *model.FieldMetadata) func(unsafe.Pointer) (types.AttributeValue, error) {
	if typ == timeType {
		return func(ptr unsafe.Pointer) (types.AttributeValue, error) {
			t := *(*time.Time)(ptr)
			if t.IsZero() && fieldMeta.OmitEmpty {
				return &types.AttributeValueMemberNULL{Value: true}, nil
			}
			if fieldMeta.IsTTL {
				return &types.AttributeValueMemberN{Value: strconv.FormatInt(t.Unix(), 10)}, nil
			}
			return &types.AttributeValueMemberS{Value: t.Format(time.RFC3339Nano)}, nil
		}
	}

	return m.buildReflectMarshalFunc(typ, fieldMeta)
}

func (m *Marshaler) buildSliceMarshalFunc(typ reflect.Type, fieldMeta *model.FieldMetadata) func(unsafe.Pointer) (types.AttributeValue, error) {
	if typ.Elem().Kind() == reflect.String {
		if fieldMeta.IsSet {
			return func(ptr unsafe.Pointer) (types.AttributeValue, error) {
				slice := (*[]string)(ptr)
				if len(*slice) == 0 {
					return &types.AttributeValueMemberNULL{Value: true}, nil
				}
				return &types.AttributeValueMemberSS{Value: *slice}, nil
			}
		}

		return func(ptr unsafe.Pointer) (types.AttributeValue, error) {
			slice := *(*[]string)(ptr)
			if len(slice) == 0 && fieldMeta.OmitEmpty {
				return &types.AttributeValueMemberNULL{Value: true}, nil
			}

			list := make([]types.AttributeValue, len(slice))
			for i, s := range slice {
				list[i] = &types.AttributeValueMemberS{Value: s}
			}
			return &types.AttributeValueMemberL{Value: list}, nil
		}
	}

	return m.buildReflectMarshalFunc(typ, fieldMeta)
}

func (m *Marshaler) buildMapMarshalFunc(typ reflect.Type, fieldMeta *model.FieldMetadata) func(unsafe.Pointer) (types.AttributeValue, error) {
	if typ.Key().Kind() == reflect.String && typ.Elem().Kind() == reflect.String {
		return func(ptr unsafe.Pointer) (types.AttributeValue, error) {
			v := reflect.NewAt(typ, ptr).Elem()
			if v.IsNil() && fieldMeta.OmitEmpty {
				return &types.AttributeValueMemberNULL{Value: true}, nil
			}

			avMap := make(map[string]types.AttributeValue, v.Len())
			for _, key := range v.MapKeys() {
				keyStr := key.String()
				val := v.MapIndex(key).String()
				avMap[keyStr] = &types.AttributeValueMemberS{Value: val}
			}
			return &types.AttributeValueMemberM{Value: avMap}, nil
		}
	}

	return m.buildReflectMarshalFunc(typ, fieldMeta)
}

// buildReflectMarshalFunc builds a reflection-based marshal function for complex types
func (m *Marshaler) buildReflectMarshalFunc(typ reflect.Type, fieldMeta *model.FieldMetadata) func(unsafe.Pointer) (types.AttributeValue, error) {
	return func(ptr unsafe.Pointer) (types.AttributeValue, error) {
		// Convert unsafe pointer back to reflect.Value
		v := reflect.NewAt(typ, ptr).Elem()

		if fieldMeta.OmitEmpty && v.IsZero() {
			return &types.AttributeValueMemberNULL{Value: true}, nil
		}

		// Handle complex types with optimized paths
		return m.marshalComplexValue(v)
	}
}

// marshalComplexValue handles complex types that can't use unsafe optimizations
func (m *Marshaler) marshalComplexValue(v reflect.Value) (types.AttributeValue, error) {
	if av, ok, err := m.marshalUsingCustomConverter(v); ok {
		return av, err
	}

	switch v.Kind() {
	case reflect.Slice:
		return m.marshalSliceComplex(v)

	case reflect.Map:
		return m.marshalMapComplex(v)

	case reflect.Struct:
		return m.marshalStructComplex(v)

	default:
		// For other types, use basic marshaling
		return m.marshalValue(v)
	}
}

func (m *Marshaler) marshalSliceComplex(v reflect.Value) (types.AttributeValue, error) {
	if v.IsNil() {
		return &types.AttributeValueMemberNULL{Value: true}, nil
	}

	list := make([]types.AttributeValue, v.Len())
	for i := 0; i < v.Len(); i++ {
		elem := v.Index(i)
		av, err := m.marshalValue(elem)
		if err != nil {
			return nil, fmt.Errorf("slice index %d: %w", i, err)
		}
		list[i] = av
	}
	return &types.AttributeValueMemberL{Value: list}, nil
}

func (m *Marshaler) marshalMapComplex(v reflect.Value) (types.AttributeValue, error) {
	if v.IsNil() {
		return &types.AttributeValueMemberNULL{Value: true}, nil
	}

	avMap := make(map[string]types.AttributeValue, v.Len())
	for _, key := range v.MapKeys() {
		keyStr := fmt.Sprintf("%v", key.Interface())
		val := v.MapIndex(key)
		av, err := m.marshalValue(val)
		if err != nil {
			return nil, fmt.Errorf("map key %s: %w", keyStr, err)
		}
		avMap[keyStr] = av
	}
	return &types.AttributeValueMemberM{Value: avMap}, nil
}

func (m *Marshaler) marshalStructComplex(v reflect.Value) (types.AttributeValue, error) {
	if v.Type() == timeType {
		t, ok := v.Interface().(time.Time)
		if !ok {
			return nil, fmt.Errorf("expected time.Time, got %T", v.Interface())
		}
		if t.IsZero() {
			return &types.AttributeValueMemberNULL{Value: true}, nil
		}
		return &types.AttributeValueMemberS{Value: t.Format(time.RFC3339Nano)}, nil
	}

	return m.marshalStructAsMap(v)
}

func (m *Marshaler) marshalStructAsMap(v reflect.Value) (types.AttributeValue, error) {
	structMap := make(map[string]types.AttributeValue, v.NumField())
	typ := v.Type()

	for i := 0; i < v.NumField(); i++ {
		field := typ.Field(i)
		if field.PkgPath != "" {
			continue
		}

		fieldValue := v.Field(i)

		jsonTag := field.Tag.Get("json")
		hasOmitEmpty := jsonTag != "" && strings.Contains(jsonTag, "omitempty")
		if hasOmitEmpty && fieldValue.IsZero() {
			continue
		}

		av, err := m.marshalValue(fieldValue)
		if err != nil {
			return nil, fmt.Errorf("struct field %s: %w", field.Name, err)
		}

		fieldName := naming.ConvertAttrName(field.Name, m.namingConvention)
		if jsonTag != "" && jsonTag != "-" {
			fieldName = jsonTagName(jsonTag)
		}

		structMap[fieldName] = av
	}

	return &types.AttributeValueMemberM{Value: structMap}, nil
}

func jsonTagName(tag string) string {
	commaIdx := strings.IndexByte(tag, ',')
	if commaIdx > 0 {
		return tag[:commaIdx]
	}
	return tag
}

// marshalValue marshals a single reflect.Value
func (m *Marshaler) marshalValue(v reflect.Value) (types.AttributeValue, error) {
	if !v.IsValid() {
		return &types.AttributeValueMemberNULL{Value: true}, nil
	}

	if v.Kind() == reflect.Ptr {
		return m.marshalPointerValue(v)
	}

	if av, ok, err := m.marshalUsingCustomConverter(v); ok {
		return av, err
	}

	switch v.Kind() {
	case reflect.String:
		return &types.AttributeValueMemberS{Value: v.String()}, nil
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		return &types.AttributeValueMemberN{Value: strconv.FormatInt(v.Int(), 10)}, nil
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		return &types.AttributeValueMemberN{Value: strconv.FormatUint(v.Uint(), 10)}, nil
	case reflect.Float32, reflect.Float64:
		return marshalFloatNumber(v), nil
	case reflect.Bool:
		return &types.AttributeValueMemberBOOL{Value: v.Bool()}, nil
	case reflect.Struct, reflect.Slice, reflect.Map:
		return m.marshalComplexValue(v)
	case reflect.Interface:
		return m.marshalInterfaceValue(v)
	default:
		// For unsupported types, return an error instead of recursing
		return nil, fmt.Errorf("unsupported type: %v", v.Kind())
	}
}

func (m *Marshaler) marshalPointerValue(v reflect.Value) (types.AttributeValue, error) {
	if v.IsNil() {
		return &types.AttributeValueMemberNULL{Value: true}, nil
	}
	if av, ok, err := m.marshalUsingCustomConverter(v); ok {
		return av, err
	}
	return m.marshalValue(v.Elem())
}

func (m *Marshaler) marshalInterfaceValue(v reflect.Value) (types.AttributeValue, error) {
	if v.IsNil() {
		return &types.AttributeValueMemberNULL{Value: true}, nil
	}

	elem := v.Elem()
	if av, ok, err := m.marshalUsingCustomConverter(elem); ok {
		return av, err
	}

	return m.marshalValue(elem)
}

func marshalFloatNumber(v reflect.Value) types.AttributeValue {
	bitSize := 64
	if v.Kind() == reflect.Float32 {
		bitSize = 32
	}
	return &types.AttributeValueMemberN{Value: strconv.FormatFloat(v.Float(), 'f', -1, bitSize)}
}

func (m *Marshaler) buildCustomConverterMarshalFunc(typ reflect.Type) func(unsafe.Pointer) (types.AttributeValue, error) {
	return func(ptr unsafe.Pointer) (types.AttributeValue, error) {
		if m.converter == nil {
			return nil, fmt.Errorf("no converter configured for type %s", typ)
		}
		value := reflect.NewAt(typ, ptr).Elem()
		return m.converter.ToAttributeValue(value.Interface())
	}
}

func (m *Marshaler) hasCustomConverter(typ reflect.Type) bool {
	if m == nil || m.converter == nil {
		return false
	}
	return m.converter.HasCustomConverter(typ)
}

func (m *Marshaler) marshalUsingCustomConverter(v reflect.Value) (types.AttributeValue, bool, error) {
	if m == nil || m.converter == nil {
		return nil, false, nil
	}
	if !m.converter.HasCustomConverter(v.Type()) {
		return nil, false, nil
	}
	av, err := m.converter.ToAttributeValue(v.Interface())
	if err != nil {
		return nil, false, err
	}
	return av, true, nil
}
