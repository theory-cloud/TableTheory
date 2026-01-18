package marshal

import (
	"fmt"
	"reflect"
	"sync"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go-v2/service/dynamodb/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/theory-cloud/tabletheory/pkg/model"
)

// Test structures
type SimpleStruct struct {
	ID     string  `dynamodb:"id"`
	Name   string  `dynamodb:"name"`
	Age    int     `dynamodb:"age"`
	Score  float64 `dynamodb:"score"`
	Active bool    `dynamodb:"active"`
}

type ComplexStruct struct {
	CreatedAt     time.Time         `dynamodb:"created_at,createdAt"`
	UpdatedAt     time.Time         `dynamodb:"updated_at,updatedAt"`
	TTL           time.Time         `dynamodb:"ttl,ttl"`
	Attributes    map[string]string `dynamodb:"attributes"`
	OptionalField *string           `dynamodb:"optional,omitempty"`
	ID            string            `dynamodb:"id"`
	Tags          []string          `dynamodb:"tags"`
	StringSet     []string          `dynamodb:"string_set,set"`
	Version       int64             `dynamodb:"version,version"`
}

type PointerStruct struct {
	StringPtr  *string  `dynamodb:"string_ptr"`
	IntPtr     *int     `dynamodb:"int_ptr"`
	Float64Ptr *float64 `dynamodb:"float64_ptr"`
	BoolPtr    *bool    `dynamodb:"bool_ptr"`
}

type OmitEmptyStruct struct {
	MapOE    map[string]string `dynamodb:"map_oe,omitempty"`
	Required string            `dynamodb:"required"`
	Optional string            `dynamodb:"optional,omitempty"`
	SliceOE  []string          `dynamodb:"slice_oe,omitempty"`
	Number   int               `dynamodb:"number,omitempty"`
	Float    float64           `dynamodb:"float,omitempty"`
}

type AllTypesStruct struct {
	Time     time.Time         `dynamodb:"time"`
	StrMap   map[string]string `dynamodb:"str_map"`
	String   string            `dynamodb:"string"`
	StrSlice []string          `dynamodb:"str_slice"`
	Int      int               `dynamodb:"int"`
	Int64    int64             `dynamodb:"int64"`
	Float64  float64           `dynamodb:"float64"`
	Bool     bool              `dynamodb:"bool"`
}

type VersionedStruct struct {
	ID      string `dynamodb:"id"`
	Version int64  `dynamodb:"version,version"`
}

// Helper function to create field metadata
func createFieldMetadata(structType reflect.Type, name, dbName string, typ reflect.Type, opts ...func(*model.FieldMetadata)) *model.FieldMetadata {
	if structType.Kind() == reflect.Ptr {
		structType = structType.Elem()
	}
	if structType.Kind() != reflect.Struct {
		panic(fmt.Sprintf("createFieldMetadata requires struct type, got %s", structType.Kind()))
	}

	field, ok := structType.FieldByName(name)
	if !ok {
		panic(fmt.Sprintf("field %s not found in %s", name, structType.Name()))
	}

	indexPath := append([]int(nil), field.Index...)
	fm := &model.FieldMetadata{
		Name:      name,
		DBName:    dbName,
		Index:     indexPath[len(indexPath)-1],
		IndexPath: indexPath,
		Type:      typ,
	}
	for _, opt := range opts {
		opt(fm)
	}
	return fm
}

// Helper options for field metadata
func withCreatedAt() func(*model.FieldMetadata) {
	return func(fm *model.FieldMetadata) { fm.IsCreatedAt = true }
}

func withUpdatedAt() func(*model.FieldMetadata) {
	return func(fm *model.FieldMetadata) { fm.IsUpdatedAt = true }
}

func withVersion() func(*model.FieldMetadata) {
	return func(fm *model.FieldMetadata) { fm.IsVersion = true }
}

func withTTL() func(*model.FieldMetadata) {
	return func(fm *model.FieldMetadata) { fm.IsTTL = true }
}

func withSet() func(*model.FieldMetadata) {
	return func(fm *model.FieldMetadata) { fm.IsSet = true }
}

func withOmitEmpty() func(*model.FieldMetadata) {
	return func(fm *model.FieldMetadata) { fm.OmitEmpty = true }
}

// Helper to create metadata
func createMetadata(fields ...*model.FieldMetadata) *model.Metadata {
	metadata := &model.Metadata{
		Fields:         make(map[string]*model.FieldMetadata),
		FieldsByDBName: make(map[string]*model.FieldMetadata),
	}

	for _, f := range fields {
		metadata.Fields[f.Name] = f
		metadata.FieldsByDBName[f.DBName] = f
	}

	return metadata
}

func requireAVS(t testing.TB, av types.AttributeValue) *types.AttributeValueMemberS {
	t.Helper()
	member, ok := av.(*types.AttributeValueMemberS)
	require.True(t, ok, "expected *types.AttributeValueMemberS, got %T", av)
	return member
}

func requireAVN(t testing.TB, av types.AttributeValue) *types.AttributeValueMemberN {
	t.Helper()
	member, ok := av.(*types.AttributeValueMemberN)
	require.True(t, ok, "expected *types.AttributeValueMemberN, got %T", av)
	return member
}

func requireAVBOOL(t testing.TB, av types.AttributeValue) *types.AttributeValueMemberBOOL {
	t.Helper()
	member, ok := av.(*types.AttributeValueMemberBOOL)
	require.True(t, ok, "expected *types.AttributeValueMemberBOOL, got %T", av)
	return member
}

func requireAVL(t testing.TB, av types.AttributeValue) *types.AttributeValueMemberL {
	t.Helper()
	member, ok := av.(*types.AttributeValueMemberL)
	require.True(t, ok, "expected *types.AttributeValueMemberL, got %T", av)
	return member
}

func requireAVM(t testing.TB, av types.AttributeValue) *types.AttributeValueMemberM {
	t.Helper()
	member, ok := av.(*types.AttributeValueMemberM)
	require.True(t, ok, "expected *types.AttributeValueMemberM, got %T", av)
	return member
}

func requireAVSS(t testing.TB, av types.AttributeValue) *types.AttributeValueMemberSS {
	t.Helper()
	member, ok := av.(*types.AttributeValueMemberSS)
	require.True(t, ok, "expected *types.AttributeValueMemberSS, got %T", av)
	return member
}

func requireAVNULL(t testing.TB, av types.AttributeValue) *types.AttributeValueMemberNULL {
	t.Helper()
	member, ok := av.(*types.AttributeValueMemberNULL)
	require.True(t, ok, "expected *types.AttributeValueMemberNULL, got %T", av)
	return member
}

func TestNew(t *testing.T) {
	marshaler := New(nil)
	assert.NotNil(t, marshaler)
}

func TestMarshalItem_SimpleTypes(t *testing.T) {
	marshaler := New(nil)

	tests := []struct {
		input    interface{}
		metadata *model.Metadata
		expected map[string]types.AttributeValue
		name     string
	}{
		{
			name: "simple struct with all fields",
			input: SimpleStruct{
				ID:     "test-id",
				Name:   "Test Name",
				Age:    30,
				Score:  98.5,
				Active: true,
			},
			metadata: createMetadata(
				createFieldMetadata(reflect.TypeOf(SimpleStruct{}), "ID", "id", reflect.TypeOf("")),
				createFieldMetadata(reflect.TypeOf(SimpleStruct{}), "Name", "name", reflect.TypeOf("")),
				createFieldMetadata(reflect.TypeOf(SimpleStruct{}), "Age", "age", reflect.TypeOf(0)),
				createFieldMetadata(reflect.TypeOf(SimpleStruct{}), "Score", "score", reflect.TypeOf(0.0)),
				createFieldMetadata(reflect.TypeOf(SimpleStruct{}), "Active", "active", reflect.TypeOf(false)),
			),
			expected: map[string]types.AttributeValue{
				"id":     &types.AttributeValueMemberS{Value: "test-id"},
				"name":   &types.AttributeValueMemberS{Value: "Test Name"},
				"age":    &types.AttributeValueMemberN{Value: "30"},
				"score":  &types.AttributeValueMemberN{Value: "98.5"},
				"active": &types.AttributeValueMemberBOOL{Value: true},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := marshaler.MarshalItem(tt.input, tt.metadata)
			require.NoError(t, err)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestMarshalItem_ComplexTypes(t *testing.T) {
	marshaler := New(nil)

	now := time.Now()
	optional := "optional-value"

	input := ComplexStruct{
		ID:            "complex-id",
		Tags:          []string{"tag1", "tag2", "tag3"},
		Attributes:    map[string]string{"key1": "value1", "key2": "value2"},
		CreatedAt:     now,
		UpdatedAt:     now,
		Version:       1,
		TTL:           now.Add(24 * time.Hour),
		OptionalField: &optional,
		StringSet:     []string{"set1", "set2"},
	}

	metadata := createMetadata(
		createFieldMetadata(reflect.TypeOf(ComplexStruct{}), "ID", "id", reflect.TypeOf("")),
		createFieldMetadata(reflect.TypeOf(ComplexStruct{}), "Tags", "tags", reflect.TypeOf([]string{})),
		createFieldMetadata(reflect.TypeOf(ComplexStruct{}), "Attributes", "attributes", reflect.TypeOf(map[string]string{})),
		createFieldMetadata(reflect.TypeOf(ComplexStruct{}), "CreatedAt", "created_at", reflect.TypeOf(time.Time{}), withCreatedAt()),
		createFieldMetadata(reflect.TypeOf(ComplexStruct{}), "UpdatedAt", "updated_at", reflect.TypeOf(time.Time{}), withUpdatedAt()),
		createFieldMetadata(reflect.TypeOf(ComplexStruct{}), "Version", "version", reflect.TypeOf(int64(0)), withVersion()),
		createFieldMetadata(reflect.TypeOf(ComplexStruct{}), "TTL", "ttl", reflect.TypeOf(time.Time{}), withTTL()),
		createFieldMetadata(reflect.TypeOf(ComplexStruct{}), "OptionalField", "optional", reflect.TypeOf(&optional), withOmitEmpty()),
		createFieldMetadata(reflect.TypeOf(ComplexStruct{}), "StringSet", "string_set", reflect.TypeOf([]string{}), withSet()),
	)

	result, err := marshaler.MarshalItem(input, metadata)
	require.NoError(t, err)

	// Check regular fields
	assert.Equal(t, "complex-id", requireAVS(t, result["id"]).Value)

	// Check list
	tagsList := requireAVL(t, result["tags"]).Value
	assert.Len(t, tagsList, 3)
	assert.Equal(t, "tag1", requireAVS(t, tagsList[0]).Value)

	// Check map
	attrMap := requireAVM(t, result["attributes"]).Value
	assert.Len(t, attrMap, 2)
	assert.Equal(t, "value1", requireAVS(t, attrMap["key1"]).Value)

	// Check timestamps (should be current time)
	createdAt := requireAVS(t, result["created_at"]).Value
	updatedAt := requireAVS(t, result["updated_at"]).Value
	assert.NotEmpty(t, createdAt)
	assert.NotEmpty(t, updatedAt)

	// Check version
	assert.Equal(t, "1", requireAVN(t, result["version"]).Value)

	// Check TTL (should be Unix timestamp)
	ttl := requireAVN(t, result["ttl"]).Value
	assert.NotEmpty(t, ttl)

	// Check optional field
	assert.Equal(t, "optional-value", requireAVS(t, result["optional"]).Value)

	// Check string set
	stringSet := requireAVSS(t, result["string_set"]).Value
	assert.ElementsMatch(t, []string{"set1", "set2"}, stringSet)
}

func TestMarshalerFactory_WithNowFuncOverridesLifecycleTimestamps(t *testing.T) {
	fixed := time.Date(2026, 1, 16, 0, 0, 0, 0, time.UTC)

	factory := NewMarshalerFactory(DefaultConfig()).WithNowFunc(func() time.Time { return fixed })
	marshaler, err := factory.NewMarshaler()
	require.NoError(t, err)

	input := ComplexStruct{
		ID:         "complex-id",
		Tags:       []string{"tag1"},
		Attributes: map[string]string{"key1": "value1"},
	}

	metadata := createMetadata(
		createFieldMetadata(reflect.TypeOf(ComplexStruct{}), "ID", "id", reflect.TypeOf("")),
		createFieldMetadata(reflect.TypeOf(ComplexStruct{}), "Tags", "tags", reflect.TypeOf([]string{})),
		createFieldMetadata(reflect.TypeOf(ComplexStruct{}), "Attributes", "attributes", reflect.TypeOf(map[string]string{})),
		createFieldMetadata(reflect.TypeOf(ComplexStruct{}), "CreatedAt", "created_at", reflect.TypeOf(time.Time{}), withCreatedAt()),
		createFieldMetadata(reflect.TypeOf(ComplexStruct{}), "UpdatedAt", "updated_at", reflect.TypeOf(time.Time{}), withUpdatedAt()),
		createFieldMetadata(reflect.TypeOf(ComplexStruct{}), "Version", "version", reflect.TypeOf(int64(0)), withVersion()),
		createFieldMetadata(reflect.TypeOf(ComplexStruct{}), "TTL", "ttl", reflect.TypeOf(time.Time{}), withTTL()),
		createFieldMetadata(reflect.TypeOf(ComplexStruct{}), "OptionalField", "optional", reflect.TypeOf((*string)(nil)), withOmitEmpty()),
		createFieldMetadata(reflect.TypeOf(ComplexStruct{}), "StringSet", "string_set", reflect.TypeOf([]string{}), withSet()),
	)

	result, err := marshaler.MarshalItem(input, metadata)
	require.NoError(t, err)

	want := fixed.Format(time.RFC3339Nano)
	require.Equal(t, want, requireAVS(t, result["created_at"]).Value)
	require.Equal(t, want, requireAVS(t, result["updated_at"]).Value)
}

func TestMarshalItem_PointerTypes(t *testing.T) {
	marshaler := New(nil)

	// Test with non-nil pointers
	str := "test-string"
	num := 42
	flt := 3.14
	bl := true

	input := PointerStruct{
		StringPtr:  &str,
		IntPtr:     &num,
		Float64Ptr: &flt,
		BoolPtr:    &bl,
	}

	metadata := createMetadata(
		createFieldMetadata(reflect.TypeOf(PointerStruct{}), "StringPtr", "string_ptr", reflect.TypeOf(&str)),
		createFieldMetadata(reflect.TypeOf(PointerStruct{}), "IntPtr", "int_ptr", reflect.TypeOf(&num)),
		createFieldMetadata(reflect.TypeOf(PointerStruct{}), "Float64Ptr", "float64_ptr", reflect.TypeOf(&flt)),
		createFieldMetadata(reflect.TypeOf(PointerStruct{}), "BoolPtr", "bool_ptr", reflect.TypeOf(&bl)),
	)

	result, err := marshaler.MarshalItem(input, metadata)
	require.NoError(t, err)

	assert.Equal(t, "test-string", requireAVS(t, result["string_ptr"]).Value)
	assert.Equal(t, "42", requireAVN(t, result["int_ptr"]).Value)
	assert.Equal(t, "3.14", requireAVN(t, result["float64_ptr"]).Value)
	assert.Equal(t, true, requireAVBOOL(t, result["bool_ptr"]).Value)

	// Test with nil pointers
	input2 := PointerStruct{}

	result2, err := marshaler.MarshalItem(input2, metadata)
	require.NoError(t, err)

	for _, key := range []string{"string_ptr", "int_ptr", "float64_ptr", "bool_ptr"} {
		nullMember := requireAVNULL(t, result2[key])
		assert.True(t, nullMember.Value)
	}
}

func TestMarshalItem_OmitEmpty(t *testing.T) {
	marshaler := New(nil)

	// Test with empty values
	input := OmitEmptyStruct{
		Required: "required-value",
		// All other fields are zero values
	}

	metadata := createMetadata(
		createFieldMetadata(reflect.TypeOf(OmitEmptyStruct{}), "Required", "required", reflect.TypeOf("")),
		createFieldMetadata(reflect.TypeOf(OmitEmptyStruct{}), "Optional", "optional", reflect.TypeOf(""), withOmitEmpty()),
		createFieldMetadata(reflect.TypeOf(OmitEmptyStruct{}), "Number", "number", reflect.TypeOf(0), withOmitEmpty()),
		createFieldMetadata(reflect.TypeOf(OmitEmptyStruct{}), "Float", "float", reflect.TypeOf(0.0), withOmitEmpty()),
		createFieldMetadata(reflect.TypeOf(OmitEmptyStruct{}), "SliceOE", "slice_oe", reflect.TypeOf([]string{}), withOmitEmpty()),
		createFieldMetadata(reflect.TypeOf(OmitEmptyStruct{}), "MapOE", "map_oe", reflect.TypeOf(map[string]string{}), withOmitEmpty()),
	)

	result, err := marshaler.MarshalItem(input, metadata)
	require.NoError(t, err)

	// Required field should be present
	assert.Equal(t, "required-value", requireAVS(t, result["required"]).Value)

	// OmitEmpty fields should not be present
	assert.Len(t, result, 1) // Only required field should be in result
}

func TestMarshalItem_Errors(t *testing.T) {
	marshaler := New(nil)

	tests := []struct {
		name     string
		input    interface{}
		metadata *model.Metadata
		wantErr  string
	}{
		{
			name:     "nil pointer",
			input:    (*SimpleStruct)(nil),
			metadata: &model.Metadata{},
			wantErr:  "cannot marshal nil pointer",
		},
		{
			name:     "non-struct type",
			input:    "not-a-struct",
			metadata: &model.Metadata{},
			wantErr:  "model must be a struct or pointer to struct",
		},
		{
			name:     "non-struct pointer",
			input:    new(string),
			metadata: &model.Metadata{},
			wantErr:  "model must be a struct or pointer to struct",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := marshaler.MarshalItem(tt.input, tt.metadata)
			require.Error(t, err)
			assert.Contains(t, err.Error(), tt.wantErr)
		})
	}
}

func TestMarshalItem_AllTypesSupport(t *testing.T) {
	marshaler := New(nil)

	now := time.Now()
	input := AllTypesStruct{
		String:   "test",
		Int:      42,
		Int64:    int64(9223372036854775807),
		Float64:  3.14159,
		Bool:     true,
		Time:     now,
		StrSlice: []string{"a", "b", "c"},
		StrMap:   map[string]string{"key": "value"},
	}

	metadata := createMetadata(
		createFieldMetadata(reflect.TypeOf(AllTypesStruct{}), "String", "string", reflect.TypeOf("")),
		createFieldMetadata(reflect.TypeOf(AllTypesStruct{}), "Int", "int", reflect.TypeOf(0)),
		createFieldMetadata(reflect.TypeOf(AllTypesStruct{}), "Int64", "int64", reflect.TypeOf(int64(0))),
		createFieldMetadata(reflect.TypeOf(AllTypesStruct{}), "Float64", "float64", reflect.TypeOf(0.0)),
		createFieldMetadata(reflect.TypeOf(AllTypesStruct{}), "Bool", "bool", reflect.TypeOf(false)),
		createFieldMetadata(reflect.TypeOf(AllTypesStruct{}), "Time", "time", reflect.TypeOf(time.Time{})),
		createFieldMetadata(reflect.TypeOf(AllTypesStruct{}), "StrSlice", "str_slice", reflect.TypeOf([]string{})),
		createFieldMetadata(reflect.TypeOf(AllTypesStruct{}), "StrMap", "str_map", reflect.TypeOf(map[string]string{})),
	)

	result, err := marshaler.MarshalItem(input, metadata)
	require.NoError(t, err)

	assert.Equal(t, "test", requireAVS(t, result["string"]).Value)
	assert.Equal(t, "42", requireAVN(t, result["int"]).Value)
	assert.Equal(t, "9223372036854775807", requireAVN(t, result["int64"]).Value)
	assert.Equal(t, "3.14159", requireAVN(t, result["float64"]).Value)
	assert.Equal(t, true, requireAVBOOL(t, result["bool"]).Value)
	assert.Equal(t, now.Format(time.RFC3339Nano), requireAVS(t, result["time"]).Value)

	// Check slice
	sliceVal := requireAVL(t, result["str_slice"]).Value
	assert.Len(t, sliceVal, 3)
	assert.Equal(t, "a", requireAVS(t, sliceVal[0]).Value)

	// Check map
	mapVal := requireAVM(t, result["str_map"]).Value
	assert.Len(t, mapVal, 1)
	assert.Equal(t, "value", requireAVS(t, mapVal["key"]).Value)
}

func TestMarshalItem_VersionField(t *testing.T) {
	marshaler := New(nil)

	// Test with zero version
	input1 := VersionedStruct{ID: "test-id", Version: 0}
	metadata := createMetadata(
		createFieldMetadata(reflect.TypeOf(VersionedStruct{}), "ID", "id", reflect.TypeOf("")),
		createFieldMetadata(reflect.TypeOf(VersionedStruct{}), "Version", "version", reflect.TypeOf(int64(0)), withVersion()),
	)

	result1, err := marshaler.MarshalItem(input1, metadata)
	require.NoError(t, err)
	assert.Equal(t, "0", requireAVN(t, result1["version"]).Value)

	// Test with non-zero version
	input2 := VersionedStruct{ID: "test-id", Version: 5}
	result2, err := marshaler.MarshalItem(input2, metadata)
	require.NoError(t, err)
	assert.Equal(t, "5", requireAVN(t, result2["version"]).Value)
}

func TestMarshalItem_ConcurrentAccess(t *testing.T) {
	marshaler := New(nil)

	metadata := createMetadata(
		createFieldMetadata(reflect.TypeOf(SimpleStruct{}), "ID", "id", reflect.TypeOf("")),
		createFieldMetadata(reflect.TypeOf(SimpleStruct{}), "Name", "name", reflect.TypeOf("")),
		createFieldMetadata(reflect.TypeOf(SimpleStruct{}), "Age", "age", reflect.TypeOf(0)),
		createFieldMetadata(reflect.TypeOf(SimpleStruct{}), "Score", "score", reflect.TypeOf(0.0)),
		createFieldMetadata(reflect.TypeOf(SimpleStruct{}), "Active", "active", reflect.TypeOf(false)),
	)

	var wg sync.WaitGroup
	errors := make(chan error, 100)

	// Run 100 concurrent marshal operations
	for i := 0; i < 100; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			input := SimpleStruct{
				ID:     fmt.Sprintf("id-%d", id),
				Name:   fmt.Sprintf("name-%d", id),
				Age:    id,
				Score:  float64(id) * 1.5,
				Active: id%2 == 0,
			}
			_, err := marshaler.MarshalItem(input, metadata)
			if err != nil {
				errors <- err
			}
		}(i)
	}

	wg.Wait()
	close(errors)

	// Check no errors occurred
	for err := range errors {
		t.Errorf("Concurrent marshal error: %v", err)
	}
}

func TestMarshalItem_CacheReuse(t *testing.T) {
	marshaler := New(nil)

	metadata := createMetadata(
		createFieldMetadata(reflect.TypeOf(SimpleStruct{}), "ID", "id", reflect.TypeOf("")),
		createFieldMetadata(reflect.TypeOf(SimpleStruct{}), "Name", "name", reflect.TypeOf("")),
	)

	// First marshal should populate cache
	input1 := SimpleStruct{ID: "1", Name: "First"}
	_, err := marshaler.MarshalItem(input1, metadata)
	require.NoError(t, err)

	// Second marshal should use cached marshaler
	input2 := SimpleStruct{ID: "2", Name: "Second"}
	_, err = marshaler.MarshalItem(input2, metadata)
	require.NoError(t, err)

	// Verify cache was used (we can't directly test this, but ensure no errors)
	assert.NoError(t, err)
}

func TestMarshalComplexValue_EdgeCases(t *testing.T) {
	marshaler := New(nil)

	// Test nil slice
	var nilSlice []string
	v1 := reflect.ValueOf(nilSlice)
	result1, err := marshaler.marshalComplexValue(v1)
	require.NoError(t, err)
	assert.IsType(t, &types.AttributeValueMemberNULL{}, result1)

	// Test nil map
	var nilMap map[string]string
	v2 := reflect.ValueOf(nilMap)
	result2, err := marshaler.marshalComplexValue(v2)
	require.NoError(t, err)
	assert.IsType(t, &types.AttributeValueMemberNULL{}, result2)

	// Test empty slice
	emptySlice := []string{}
	v3 := reflect.ValueOf(emptySlice)
	result3, err := marshaler.marshalComplexValue(v3)
	require.NoError(t, err)
	list := requireAVL(t, result3).Value
	assert.Len(t, list, 0)

	// Test empty map
	emptyMap := map[string]string{}
	v4 := reflect.ValueOf(emptyMap)
	result4, err := marshaler.marshalComplexValue(v4)
	require.NoError(t, err)
	mapVal := requireAVM(t, result4).Value
	assert.Len(t, mapVal, 0)
}

func TestMarshalValue_AllNumericTypes(t *testing.T) {
	marshaler := New(nil)

	tests := []struct {
		name     string
		value    interface{}
		expected string
	}{
		{"int8", int8(127), "127"},
		{"int16", int16(32767), "32767"},
		{"int32", int32(2147483647), "2147483647"},
		{"uint", uint(42), "42"},
		{"uint8", uint8(255), "255"},
		{"uint16", uint16(65535), "65535"},
		{"uint32", uint32(4294967295), "4294967295"},
		{"uint64", uint64(18446744073709551615), "18446744073709551615"},
		{"float32", float32(3.14), "3.14"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			v := reflect.ValueOf(tt.value)
			result, err := marshaler.marshalValue(v)
			require.NoError(t, err)
			assert.Equal(t, tt.expected, requireAVN(t, result).Value)
		})
	}
}

func BenchmarkMarshalItem_Simple(b *testing.B) {
	marshaler := New(nil)

	input := SimpleStruct{
		ID:     "bench-id",
		Name:   "Benchmark Test",
		Age:    30,
		Score:  98.5,
		Active: true,
	}

	metadata := createMetadata(
		createFieldMetadata(reflect.TypeOf(SimpleStruct{}), "ID", "id", reflect.TypeOf("")),
		createFieldMetadata(reflect.TypeOf(SimpleStruct{}), "Name", "name", reflect.TypeOf("")),
		createFieldMetadata(reflect.TypeOf(SimpleStruct{}), "Age", "age", reflect.TypeOf(0)),
		createFieldMetadata(reflect.TypeOf(SimpleStruct{}), "Score", "score", reflect.TypeOf(0.0)),
		createFieldMetadata(reflect.TypeOf(SimpleStruct{}), "Active", "active", reflect.TypeOf(false)),
	)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if _, err := marshaler.MarshalItem(input, metadata); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkMarshalItem_Complex(b *testing.B) {
	marshaler := New(nil)

	optional := "optional"
	input := ComplexStruct{
		ID:            "bench-id",
		Tags:          []string{"tag1", "tag2", "tag3"},
		Attributes:    map[string]string{"key1": "value1", "key2": "value2"},
		Version:       1,
		TTL:           time.Now().Add(24 * time.Hour),
		OptionalField: &optional,
		StringSet:     []string{"set1", "set2"},
	}

	metadata := createMetadata(
		createFieldMetadata(reflect.TypeOf(ComplexStruct{}), "ID", "id", reflect.TypeOf("")),
		createFieldMetadata(reflect.TypeOf(ComplexStruct{}), "Tags", "tags", reflect.TypeOf([]string{})),
		createFieldMetadata(reflect.TypeOf(ComplexStruct{}), "Attributes", "attributes", reflect.TypeOf(map[string]string{})),
		createFieldMetadata(reflect.TypeOf(ComplexStruct{}), "CreatedAt", "created_at", reflect.TypeOf(time.Time{}), withCreatedAt()),
		createFieldMetadata(reflect.TypeOf(ComplexStruct{}), "UpdatedAt", "updated_at", reflect.TypeOf(time.Time{}), withUpdatedAt()),
		createFieldMetadata(reflect.TypeOf(ComplexStruct{}), "Version", "version", reflect.TypeOf(int64(0)), withVersion()),
		createFieldMetadata(reflect.TypeOf(ComplexStruct{}), "TTL", "ttl", reflect.TypeOf(time.Time{}), withTTL()),
		createFieldMetadata(reflect.TypeOf(ComplexStruct{}), "OptionalField", "optional", reflect.TypeOf(&optional), withOmitEmpty()),
		createFieldMetadata(reflect.TypeOf(ComplexStruct{}), "StringSet", "string_set", reflect.TypeOf([]string{}), withSet()),
	)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if _, err := marshaler.MarshalItem(input, metadata); err != nil {
			b.Fatal(err)
		}
	}
}

// Additional tests for edge cases and special scenarios
func TestMarshalItem_SpecialStringSetHandling(t *testing.T) {
	marshaler := New(nil)

	// Test empty string set with omitempty
	type StringSetStruct struct {
		ID   string   `dynamodb:"id"`
		Tags []string `dynamodb:"tags,set,omitempty"`
	}

	input := StringSetStruct{
		ID:   "test-id",
		Tags: []string{}, // Empty set
	}

	metadata := createMetadata(
		createFieldMetadata(reflect.TypeOf(StringSetStruct{}), "ID", "id", reflect.TypeOf("")),
		createFieldMetadata(reflect.TypeOf(StringSetStruct{}), "Tags", "tags", reflect.TypeOf([]string{}), withSet(), withOmitEmpty()),
	)

	result, err := marshaler.MarshalItem(input, metadata)
	require.NoError(t, err)

	// Empty set with omitempty should not be in result
	_, exists := result["tags"]
	assert.False(t, exists)
}

func TestMarshalItem_EmptySetEncodesNullWithoutOmitEmpty(t *testing.T) {
	marshaler := New(nil)

	type StringSetStruct struct {
		ID   string   `dynamodb:"id"`
		Tags []string `dynamodb:"tags,set"`
	}

	input := StringSetStruct{
		ID:   "test-id",
		Tags: []string{},
	}

	metadata := createMetadata(
		createFieldMetadata(reflect.TypeOf(StringSetStruct{}), "ID", "id", reflect.TypeOf("")),
		createFieldMetadata(reflect.TypeOf(StringSetStruct{}), "Tags", "tags", reflect.TypeOf([]string{}), withSet()),
	)

	result, err := marshaler.MarshalItem(input, metadata)
	require.NoError(t, err)

	av, ok := result["tags"]
	require.True(t, ok, "expected tags field")
	_, isNull := av.(*types.AttributeValueMemberNULL)
	require.True(t, isNull, "expected NULL for empty set, got %T", av)
}

func TestMarshalItem_DeepNestedStructures(t *testing.T) {
	marshaler := New(nil)

	type NestedMap struct {
		DeepMap map[string]map[string]string `dynamodb:"deep_map"`
		ID      string                       `dynamodb:"id"`
	}

	input := NestedMap{
		ID: "nested-id",
		DeepMap: map[string]map[string]string{
			"level1": {
				"key1": "value1",
				"key2": "value2",
			},
		},
	}

	metadata := createMetadata(
		createFieldMetadata(reflect.TypeOf(NestedMap{}), "ID", "id", reflect.TypeOf("")),
		createFieldMetadata(reflect.TypeOf(NestedMap{}), "DeepMap", "deep_map", reflect.TypeOf(map[string]map[string]string{})),
	)

	_, err := marshaler.MarshalItem(input, metadata)
	require.NoError(t, err)
}
