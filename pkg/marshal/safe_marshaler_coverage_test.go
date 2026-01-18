package marshal

import (
	"reflect"
	"strconv"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/theory-cloud/tabletheory/pkg/model"
)

func TestSafeMarshaler_MarshalItem_CoversCorePaths(t *testing.T) {
	type nested struct {
		Field string
	}

	type safeItem struct {
		Optional  *string
		Nested    nested
		Data      map[string]int
		CreatedAt time.Time
		UpdatedAt time.Time
		TTL       time.Time
		ID        string
		Tags      []string
		StringSet []string
		Version   int64
	}

	now := time.Now().UTC().Truncate(time.Second)
	optional := "present"
	item := safeItem{
		CreatedAt: now.Add(-time.Hour),
		UpdatedAt: now.Add(-time.Minute),
		TTL:       now.Add(time.Hour),
		ID:        "id-1",
		Tags:      []string{"a", "b"},
		StringSet: []string{"x", "y"},
		Optional:  &optional,
		Version:   2,
		Nested:    nested{Field: "value"},
		Data:      map[string]int{"k": 1},
	}

	metadata := &model.Metadata{
		Fields:         make(map[string]*model.FieldMetadata),
		FieldsByDBName: make(map[string]*model.FieldMetadata),
	}

	registerField := func(name, dbName string, typ reflect.Type, indexPath []int, opts ...func(*model.FieldMetadata)) {
		fm := &model.FieldMetadata{
			Name:      name,
			DBName:    dbName,
			Type:      typ,
			Index:     indexPath[len(indexPath)-1],
			IndexPath: append([]int(nil), indexPath...),
		}
		for _, opt := range opts {
			opt(fm)
		}
		metadata.Fields[name] = fm
		metadata.FieldsByDBName[dbName] = fm
	}

	typ := reflect.TypeOf(item)
	fieldIndex := func(name string) []int {
		f, ok := typ.FieldByName(name)
		require.True(t, ok)
		return f.Index
	}

	registerField("CreatedAt", "created_at", reflect.TypeOf(time.Time{}), fieldIndex("CreatedAt"), withCreatedAt())
	registerField("UpdatedAt", "updated_at", reflect.TypeOf(time.Time{}), fieldIndex("UpdatedAt"), withUpdatedAt())
	registerField("TTL", "ttl", reflect.TypeOf(time.Time{}), fieldIndex("TTL"), withTTL())
	registerField("ID", "id", reflect.TypeOf(""), fieldIndex("ID"))
	registerField("Tags", "tags", reflect.TypeOf([]string{}), fieldIndex("Tags"))
	registerField("StringSet", "string_set", reflect.TypeOf([]string{}), fieldIndex("StringSet"), withSet())
	registerField("Optional", "optional", reflect.TypeOf((*string)(nil)), fieldIndex("Optional"), withOmitEmpty())
	registerField("Version", "version", reflect.TypeOf(int64(0)), fieldIndex("Version"), withVersion())
	registerField("Nested", "nested", reflect.TypeOf(nested{}), fieldIndex("Nested"))
	registerField("Data", "data", reflect.TypeOf(map[string]int{}), fieldIndex("Data"))

	metadata.PrimaryKey = &model.KeySchema{
		PartitionKey: metadata.Fields["ID"],
	}

	marshaler := NewSafeMarshaler()

	// Run twice to cover the cache path.
	for i := 0; i < 2; i++ {
		out, err := marshaler.MarshalItem(item, metadata)
		require.NoError(t, err)

		require.NotEmpty(t, requireAVS(t, out["created_at"]).Value)
		require.NotEmpty(t, requireAVS(t, out["updated_at"]).Value)

		require.Equal(t, item.ID, requireAVS(t, out["id"]).Value)
		require.Equal(t, "2", requireAVN(t, out["version"]).Value)
		require.Equal(t, item.TTL.Unix(), mustParseInt64(t, requireAVN(t, out["ttl"]).Value))
		require.ElementsMatch(t, item.StringSet, requireAVSS(t, out["string_set"]).Value)
		require.NotNil(t, out["data"])
		require.NotNil(t, out["nested"])
		require.NotNil(t, out["optional"])
	}
}

func TestSafeMarshaler_MarshalItem_OmitEmptySkipsNull(t *testing.T) {
	type item struct {
		Optional *string
		ID       string
	}

	typ := reflect.TypeOf(item{})
	metadata := &model.Metadata{
		Fields:         make(map[string]*model.FieldMetadata),
		FieldsByDBName: make(map[string]*model.FieldMetadata),
		PrimaryKey: &model.KeySchema{
			PartitionKey: &model.FieldMetadata{Name: "ID", DBName: "id"},
		},
	}

	idField, ok := typ.FieldByName("ID")
	require.True(t, ok)
	optionalField, ok := typ.FieldByName("Optional")
	require.True(t, ok)

	metadata.Fields["ID"] = &model.FieldMetadata{Name: "ID", DBName: "id", Type: reflect.TypeOf(""), Index: idField.Index[0], IndexPath: idField.Index}
	metadata.FieldsByDBName["id"] = metadata.Fields["ID"]
	metadata.Fields["Optional"] = &model.FieldMetadata{Name: "Optional", DBName: "optional", Type: reflect.TypeOf((*string)(nil)), Index: optionalField.Index[0], IndexPath: optionalField.Index, OmitEmpty: true}
	metadata.FieldsByDBName["optional"] = metadata.Fields["Optional"]

	marshaler := NewSafeMarshaler()

	out, err := marshaler.MarshalItem(item{ID: "id-1"}, metadata)
	require.NoError(t, err)
	require.Contains(t, out, "id")
	require.NotContains(t, out, "optional")
}

func TestSafeMarshaler_MarshalItem_UnsupportedTypeErrors(t *testing.T) {
	type bad struct {
		BadChan chan int
		ID      string
	}

	typ := reflect.TypeOf(bad{})
	metadata := &model.Metadata{
		Fields:         make(map[string]*model.FieldMetadata),
		FieldsByDBName: make(map[string]*model.FieldMetadata),
	}

	idField, ok := typ.FieldByName("ID")
	require.True(t, ok)
	badChanField, ok := typ.FieldByName("BadChan")
	require.True(t, ok)

	metadata.Fields["ID"] = &model.FieldMetadata{Name: "ID", DBName: "id", Type: reflect.TypeOf(""), Index: idField.Index[0], IndexPath: idField.Index}
	metadata.FieldsByDBName["id"] = metadata.Fields["ID"]
	metadata.Fields["BadChan"] = &model.FieldMetadata{Name: "BadChan", DBName: "bad", Type: reflect.TypeOf((chan int)(nil)), Index: badChanField.Index[0], IndexPath: badChanField.Index}
	metadata.FieldsByDBName["bad"] = metadata.Fields["BadChan"]
	metadata.PrimaryKey = &model.KeySchema{PartitionKey: metadata.Fields["ID"]}

	marshaler := NewSafeMarshaler()
	_, err := marshaler.MarshalItem(bad{ID: "id-1"}, metadata)
	require.Error(t, err)
}

func mustParseInt64(t *testing.T, s string) int64 {
	t.Helper()
	v, err := strconv.ParseInt(s, 10, 64)
	require.NoError(t, err)
	return v
}
