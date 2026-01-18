package dms

import (
	"reflect"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/theory-cloud/tabletheory/pkg/model"
	"github.com/theory-cloud/tabletheory/pkg/naming"
)

func TestParseDocumentAndFindModel(t *testing.T) {
	t.Parallel()

	raw := []byte(`
dms_version: "0.1"
namespace: "theorydb.test"
models:
  - name: "Demo"
    table: { name: "tbl" }
    keys:
      partition: { attribute: "PK", type: "S" }
      sort: { attribute: "SK", type: "S" }
    attributes:
      - attribute: "PK"
        type: "S"
        required: true
        roles: ["pk"]
      - attribute: "SK"
        type: "S"
        required: true
        roles: ["sk"]
`)

	doc, err := ParseDocument(raw)
	require.NoError(t, err)

	m, ok := FindModel(doc, "Demo")
	require.True(t, ok)
	require.Equal(t, "Demo", m.Name)
	require.Equal(t, "tbl", m.Table.Name)
	require.Equal(t, "PK", m.Keys.Partition.Attribute)
	require.NotNil(t, m.Keys.Sort)
	require.Equal(t, "SK", m.Keys.Sort.Attribute)
}

func TestFindModelNilDocument(t *testing.T) {
	t.Parallel()

	_, ok := FindModel(nil, "Demo")
	require.False(t, ok)
}

func TestParseDocumentRejectsUnsupportedVersion(t *testing.T) {
	t.Parallel()

	_, err := ParseDocument([]byte(`dms_version: "9.9"\nmodels: []\n`))
	require.Error(t, err)
}

func TestParseDocumentRejectsNonJSONValues(t *testing.T) {
	t.Parallel()

	_, err := ParseDocument([]byte(`
dms_version: "0.1"
models:
  - name: "Demo"
    table: { name: "tbl" }
    keys:
      partition: { attribute: "PK", type: "S" }
    attributes:
      - attribute: "PK"
        type: "S"
        required: true
        roles: ["pk"]
timestamp: 2025-01-01T00:00:00Z
`))
	require.Error(t, err)
}

func TestNormalizeJSONCompatible_MapAnyAny(t *testing.T) {
	t.Parallel()

	got, err := normalizeJSONCompatible(map[any]any{
		"outer": map[any]any{
			"inner": []any{
				map[any]any{"x": "y"},
			},
		},
	}, "dms")
	require.NoError(t, err)

	m, ok := got.(map[string]any)
	require.True(t, ok)
	outerAny, ok := m["outer"]
	require.True(t, ok)
	outer, ok := outerAny.(map[string]any)
	require.True(t, ok)
	innerAny, ok := outer["inner"]
	require.True(t, ok)
	inner, ok := innerAny.([]any)
	require.True(t, ok)
	require.Len(t, inner, 1)
	firstAny := inner[0]
	first, ok := firstAny.(map[string]any)
	require.True(t, ok)
	require.Equal(t, "y", first["x"])
}

func TestNormalizeJSONCompatible_MapAnyAnyRejectsNonStringKey(t *testing.T) {
	t.Parallel()

	_, err := normalizeJSONCompatible(map[any]any{1: "x"}, "dms")
	require.Error(t, err)
}

func TestValidateDocument_Errors(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name string
		doc  *Document
		want string
	}{
		{
			name: "nil_document",
			doc:  nil,
			want: "DMS document is nil",
		},
		{
			name: "missing_models",
			doc:  &Document{DMSVersion: "0.1"},
			want: "must include models",
		},
		{
			name: "model_missing_name",
			doc:  &Document{DMSVersion: "0.1", Models: []Model{{}}},
			want: "model missing name",
		},
		{
			name: "model_missing_table",
			doc:  &Document{DMSVersion: "0.1", Models: []Model{{Name: "A"}}},
			want: "missing table.name",
		},
		{
			name: "model_missing_partition_key",
			doc: &Document{
				DMSVersion: "0.1",
				Models: []Model{{
					Name:  "A",
					Table: Table{Name: "t"},
					Keys:  Keys{Partition: KeyAttribute{}},
					Attributes: []Attribute{
						{Attribute: "PK", Type: "S"},
					},
				}},
			},
			want: "missing keys.partition",
		},
		{
			name: "model_invalid_sort_key",
			doc: &Document{
				DMSVersion: "0.1",
				Models: []Model{{
					Name:  "A",
					Table: Table{Name: "t"},
					Keys: Keys{
						Sort:      &KeyAttribute{},
						Partition: KeyAttribute{Attribute: "PK", Type: "S"},
					},
					Attributes: []Attribute{
						{Attribute: "PK", Type: "S"},
					},
				}},
			},
			want: "invalid keys.sort",
		},
		{
			name: "duplicate_attribute",
			doc: &Document{
				DMSVersion: "0.1",
				Models: []Model{{
					Name:  "A",
					Table: Table{Name: "t"},
					Keys:  Keys{Partition: KeyAttribute{Attribute: "PK", Type: "S"}},
					Attributes: []Attribute{
						{Attribute: "PK", Type: "S"},
						{Attribute: "PK", Type: "S"},
					},
				}},
			},
			want: "duplicate attribute",
		},
		{
			name: "required_and_optional",
			doc: &Document{
				DMSVersion: "0.1",
				Models: []Model{{
					Name:  "A",
					Table: Table{Name: "t"},
					Keys:  Keys{Partition: KeyAttribute{Attribute: "PK", Type: "S"}},
					Attributes: []Attribute{
						{Attribute: "PK", Type: "S", Required: true, Optional: true},
					},
				}},
			},
			want: "cannot be both required and optional",
		},
		{
			name: "missing_partition_key_attribute",
			doc: &Document{
				DMSVersion: "0.1",
				Models: []Model{{
					Name:  "A",
					Table: Table{Name: "t"},
					Keys:  Keys{Partition: KeyAttribute{Attribute: "PK", Type: "S"}},
					Attributes: []Attribute{
						{Attribute: "SK", Type: "S"},
					},
				}},
			},
			want: "missing partition key attribute",
		},
		{
			name: "missing_sort_key_attribute",
			doc: &Document{
				DMSVersion: "0.1",
				Models: []Model{{
					Name:  "A",
					Table: Table{Name: "t"},
					Keys: Keys{
						Sort:      &KeyAttribute{Attribute: "SK", Type: "S"},
						Partition: KeyAttribute{Attribute: "PK", Type: "S"},
					},
					Attributes: []Attribute{
						{Attribute: "PK", Type: "S"},
					},
				}},
			},
			want: "missing sort key attribute",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			err := validateDocument(tc.doc)
			require.Error(t, err)
			require.Contains(t, err.Error(), tc.want)
		})
	}
}

type demoModel struct {
	PK     string `theorydb:"pk,attr:PK"`
	SK     string `theorydb:"sk,attr:SK"`
	Value  string `theorydb:"attr:value,omitempty"`
	Secret string `theorydb:"encrypted,attr:secret,omitempty"`
}

func (demoModel) TableName() string { return "tbl" }

func TestFromMetadataAndEquivalence(t *testing.T) {
	t.Parallel()

	doc, err := ParseDocument([]byte(`
dms_version: "0.1"
models:
  - name: "demoModel"
    table: { name: "ignored" }
    keys:
      partition: { attribute: "PK", type: "S" }
      sort: { attribute: "SK", type: "S" }
    attributes:
      - attribute: "PK"
        type: "S"
        required: true
        roles: ["pk"]
      - attribute: "SK"
        type: "S"
        required: true
        roles: ["sk"]
      - attribute: "value"
        type: "S"
        optional: true
        omit_empty: true
      - attribute: "secret"
        type: "S"
        optional: true
        omit_empty: true
        encryption: { v: 1 }
`))
	require.NoError(t, err)

	want, ok := FindModel(doc, "demoModel")
	require.True(t, ok)

	reg := model.NewRegistry()
	require.NoError(t, reg.Register(demoModel{}))
	meta, err := reg.GetMetadata(demoModel{})
	require.NoError(t, err)

	got, err := FromMetadata(meta)
	require.NoError(t, err)

	require.NoError(t, AssertModelsEquivalent(got, *want, CompareOptions{IgnoreTableName: true}))
}

type pkOnlyModel struct {
	PK   string   `theorydb:"pk,attr:PK"`
	Tags []string `theorydb:"attr:tags,set,omitempty"`
	Flag bool     `theorydb:"attr:flag,omitempty"`
}

func (pkOnlyModel) TableName() string { return "tbl" }

func TestFromMetadata_KindAndSetMapping(t *testing.T) {
	t.Parallel()

	reg := model.NewRegistry()
	require.NoError(t, reg.Register(pkOnlyModel{}))
	meta, err := reg.GetMetadata(pkOnlyModel{})
	require.NoError(t, err)

	got, err := FromMetadata(meta)
	require.NoError(t, err)

	require.Nil(t, got.Keys.Sort)

	attrByName := make(map[string]Attribute, len(got.Attributes))
	for _, a := range got.Attributes {
		attrByName[a.Attribute] = a
	}

	require.Equal(t, "S", attrByName["PK"].Type)
	require.Equal(t, "BOOL", attrByName["flag"].Type)
	require.True(t, attrByName["flag"].OmitEmpty)
	require.Equal(t, "SS", attrByName["tags"].Type)
}

func TestAttributeTypeFromField_SetMappingsAndErrors(t *testing.T) {
	t.Parallel()

	tp, err := attributeTypeFromField(reflect.TypeOf([]string{}), true, nil)
	require.NoError(t, err)
	require.Equal(t, "SS", tp)

	tp, err = attributeTypeFromField(reflect.TypeOf([]int{}), true, nil)
	require.NoError(t, err)
	require.Equal(t, "NS", tp)

	tp, err = attributeTypeFromField(reflect.TypeOf([][]byte{}), true, nil)
	require.NoError(t, err)
	require.Equal(t, "BS", tp)

	_, err = attributeTypeFromField(reflect.TypeOf([]struct{}{}), true, nil)
	require.Error(t, err)

	_, err = attributeTypeFromField(reflect.TypeOf(map[string]int{}), true, nil)
	require.Error(t, err)
}

func TestAttributeTypeFromField_TagsAndKinds(t *testing.T) {
	t.Parallel()

	tp, err := attributeTypeFromField(reflect.TypeOf(map[string]int{}), false, map[string]string{"json": "true"})
	require.NoError(t, err)
	require.Equal(t, "S", tp)

	tp, err = attributeTypeFromField(reflect.TypeOf(""), false, map[string]string{"binary": "true"})
	require.NoError(t, err)
	require.Equal(t, "B", tp)

	tp, err = attributeTypeFromField(reflect.TypeOf(true), false, nil)
	require.NoError(t, err)
	require.Equal(t, "BOOL", tp)

	tp, err = attributeTypeFromField(reflect.TypeOf(123), false, nil)
	require.NoError(t, err)
	require.Equal(t, "N", tp)

	tp, err = attributeTypeFromField(reflect.TypeOf([]string{}), false, nil)
	require.NoError(t, err)
	require.Equal(t, "L", tp)

	tp, err = attributeTypeFromField(reflect.TypeOf(struct{}{}), false, nil)
	require.NoError(t, err)
	require.Equal(t, "M", tp)

	tp, err = attributeTypeFromField(reflect.TypeOf((chan int)(nil)), false, nil)
	require.NoError(t, err)
	require.Equal(t, "S", tp)
}

type jsonBinaryTagModel struct {
	PK      string            `theorydb:"pk,attr:PK"`
	Payload map[string]string `theorydb:"json,attr:payload,omitempty"`
	Blob    string            `theorydb:"binary,attr:blob,omitempty"`
}

func (jsonBinaryTagModel) TableName() string { return "tbl" }

func TestFromMetadata_JsonAndBinaryTags(t *testing.T) {
	t.Parallel()

	reg := model.NewRegistry()
	require.NoError(t, reg.Register(jsonBinaryTagModel{}))
	meta, err := reg.GetMetadata(jsonBinaryTagModel{})
	require.NoError(t, err)

	got, err := FromMetadata(meta)
	require.NoError(t, err)

	attrByName := make(map[string]Attribute, len(got.Attributes))
	for _, a := range got.Attributes {
		attrByName[a.Attribute] = a
	}

	require.True(t, attrByName["payload"].JSON)
	require.Equal(t, "S", attrByName["payload"].Type)

	require.True(t, attrByName["blob"].Binary)
	require.Equal(t, "B", attrByName["blob"].Type)
}

func TestParseDocumentRejectsInvalidJsonBinaryModifiers(t *testing.T) {
	t.Parallel()

	_, err := ParseDocument([]byte(`
dms_version: "0.1"
models:
  - name: "Demo"
    table: { name: "tbl" }
    keys:
      partition: { attribute: "PK", type: "S" }
    attributes:
      - attribute: "PK"
        type: "S"
        required: true
        roles: ["pk"]
      - attribute: "bad_json"
        type: "N"
        optional: true
        json: true
`))
	require.Error(t, err)
}

func TestAssertModelsEquivalent_ReturnsDiff(t *testing.T) {
	t.Parallel()

	got := Model{
		Name: "Demo",
		Table: Table{
			Name: "tbl",
		},
		Naming: Naming{Convention: "camelCase"},
		Keys: Keys{
			Partition: KeyAttribute{
				Attribute: "PK",
				Type:      "S",
			},
		},
		Attributes: []Attribute{
			{Attribute: "PK", Type: "S", Required: true},
		},
	}
	want := got
	want.Naming.Convention = "snake_case"

	err := AssertModelsEquivalent(got, want, CompareOptions{})
	require.Error(t, err)
	require.Contains(t, err.Error(), "models not equivalent")
}

type indexKeyTypeModel struct {
	GSI1PK string `theorydb:"attr:gsi1PK,index:gsi-keytype,pk"`
	SK     []byte `theorydb:"sk,attr:SK"`
	PK     int64  `theorydb:"pk,attr:PK"`
	GSI1SK int    `theorydb:"attr:gsi1SK,index:gsi-keytype,sk"`
}

func (indexKeyTypeModel) TableName() string { return "tbl" }

func TestFromMetadata_IndexAndKeyTypes(t *testing.T) {
	t.Parallel()

	reg := model.NewRegistry()
	require.NoError(t, reg.Register(indexKeyTypeModel{}))
	meta, err := reg.GetMetadata(indexKeyTypeModel{})
	require.NoError(t, err)

	meta.NamingConvention = naming.SnakeCase
	require.NotEmpty(t, meta.Indexes)
	meta.Indexes[0].ProjectionType = ""

	got, err := FromMetadata(meta)
	require.NoError(t, err)

	require.Equal(t, "snake_case", got.Naming.Convention)
	require.Equal(t, "N", got.Keys.Partition.Type)
	require.NotNil(t, got.Keys.Sort)
	require.Equal(t, "B", got.Keys.Sort.Type)

	require.Len(t, got.Indexes, 1)
	require.Equal(t, "gsi-keytype", got.Indexes[0].Name)
	require.Equal(t, "GSI", got.Indexes[0].Type)
	require.Equal(t, "ALL", got.Indexes[0].Projection.Type)
	require.Equal(t, "gsi1PK", got.Indexes[0].Partition.Attribute)
	require.NotNil(t, got.Indexes[0].Sort)
	require.Equal(t, "gsi1SK", got.Indexes[0].Sort.Attribute)
}

func TestScalarKeyTypeFromField(t *testing.T) {
	t.Parallel()

	require.Equal(t, "S", scalarKeyTypeFromField(reflect.TypeOf("")))
	require.Equal(t, "N", scalarKeyTypeFromField(reflect.TypeOf(int64(0))))
	require.Equal(t, "B", scalarKeyTypeFromField(reflect.TypeOf([]byte{})))
	require.Equal(t, "S", scalarKeyTypeFromField(reflect.TypeOf(struct{}{})))
}
