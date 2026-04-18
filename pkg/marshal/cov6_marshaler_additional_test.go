package marshal

import (
	"errors"
	"reflect"
	"testing"
	"time"
	"unsafe"

	"github.com/aws/aws-sdk-go-v2/service/dynamodb/types"
	"github.com/stretchr/testify/require"

	"github.com/theory-cloud/tabletheory/pkg/model"
	pkgTypes "github.com/theory-cloud/tabletheory/pkg/types"
)

func TestMarshalStructComplex_TimeZeroAndNonZero_COV6(t *testing.T) {
	m := New(nil)

	av, err := m.marshalStructComplex(reflect.ValueOf(time.Time{}))
	require.NoError(t, err)
	_, ok := av.(*types.AttributeValueMemberNULL)
	require.True(t, ok)

	now := time.Date(2024, 1, 2, 3, 4, 5, 0, time.UTC)
	av, err = m.marshalStructComplex(reflect.ValueOf(now))
	require.NoError(t, err)
	avs := requireAVS(t, av)
	require.Equal(t, now.Format(time.RFC3339Nano), avs.Value)
}

func TestMarshalStructAsMap_JSONTagsAndOmitEmpty_COV6(t *testing.T) {
	type nested struct {
		Dash    string `json:"-"`
		Skip    string `json:"skip,omitempty"`
		Visible string `json:"visible"`
	}

	m := New(nil)

	av, err := m.marshalStructAsMap(reflect.ValueOf(nested{
		Dash:    "dash-value",
		Skip:    "",
		Visible: "seen",
	}))
	require.NoError(t, err)

	mmap := requireAVM(t, av).Value
	require.Equal(t, "seen", requireAVS(t, mmap["visible"]).Value)
	require.NotContains(t, mmap, "skip")
	require.Equal(t, "dash-value", requireAVS(t, mmap["dash"]).Value)
}

func TestMarshalStructAsMap_PromotedAnonymousEmbedsLegacyShape_COV6(t *testing.T) {
	type BaseObject struct {
		ID   string
		Type string
		To   []string
	}

	//nolint:govet // Field order mirrors the anonymous-embed contract fixture under test.
	type activity struct {
		BaseObject
		Actor  string
		Object string
	}

	m := New(nil)

	av, err := m.marshalStructAsMap(reflect.ValueOf(activity{
		BaseObject: BaseObject{
			ID:   "activity-1",
			Type: "Create",
			To:   []string{"acct:one", "acct:two"},
		},
		Actor:  "acct:actor",
		Object: "note-1",
	}))
	require.NoError(t, err)

	activityAV := requireAVM(t, av).Value
	require.Contains(t, activityAV, "baseObject")
	require.Contains(t, activityAV, "actor")
	require.Contains(t, activityAV, "object")
	require.NotContains(t, activityAV, "id")
	require.NotContains(t, activityAV, "type")
	require.NotContains(t, activityAV, "to")

	baseObjectAV := requireAVM(t, activityAV["baseObject"]).Value
	require.Equal(t, "activity-1", requireAVS(t, baseObjectAV["id"]).Value)
	require.Equal(t, "Create", requireAVS(t, baseObjectAV["type"]).Value)
	require.ElementsMatch(t, []string{"acct:one", "acct:two"}, attributeValueStringList(t, baseObjectAV["to"]))
}

func TestMarshalStructAsMap_PromotedAnonymousEmbedsFlatShape_COV6(t *testing.T) {
	type BaseObject struct {
		ID   string
		Type string
		To   []string
	}

	//nolint:govet // Field order mirrors the anonymous-embed contract fixture under test.
	type activity struct {
		BaseObject
		Actor  string
		Object string
	}

	m := New(pkgTypes.NewConverter().WithFlatAnonymousEmbedEncoding())

	av, err := m.marshalStructAsMap(reflect.ValueOf(activity{
		BaseObject: BaseObject{
			ID:   "activity-1",
			Type: "Create",
			To:   []string{"acct:one", "acct:two"},
		},
		Actor:  "acct:actor",
		Object: "note-1",
	}))
	require.NoError(t, err)

	activityAV := requireAVM(t, av).Value
	require.Contains(t, activityAV, "id")
	require.Contains(t, activityAV, "type")
	require.Contains(t, activityAV, "to")
	require.Contains(t, activityAV, "actor")
	require.Contains(t, activityAV, "object")
	require.NotContains(t, activityAV, "baseObject")

	require.Equal(t, "activity-1", requireAVS(t, activityAV["id"]).Value)
	require.Equal(t, "Create", requireAVS(t, activityAV["type"]).Value)
	require.ElementsMatch(t, []string{"acct:one", "acct:two"}, attributeValueStringList(t, activityAV["to"]))
}

type cov6CustomType struct {
	Value string
}

type cov6GoodConverter struct{}

func (cov6GoodConverter) ToAttributeValue(any) (types.AttributeValue, error) {
	return &types.AttributeValueMemberS{Value: "ok"}, nil
}

func (cov6GoodConverter) FromAttributeValue(types.AttributeValue, any) error { return nil }

type cov6ErrorConverter struct{}

func (cov6ErrorConverter) ToAttributeValue(any) (types.AttributeValue, error) {
	return nil, errors.New("boom")
}

func (cov6ErrorConverter) FromAttributeValue(types.AttributeValue, any) error { return nil }

func TestMarshalUsingCustomConverter_NilAndErrors_COV6(t *testing.T) {
	var nilMarshaler *Marshaler
	av, ok, err := nilMarshaler.marshalUsingCustomConverter(reflect.ValueOf("x"))
	require.Nil(t, av)
	require.False(t, ok)
	require.NoError(t, err)

	m := New(nil)
	av, ok, err = m.marshalUsingCustomConverter(reflect.ValueOf("x"))
	require.Nil(t, av)
	require.False(t, ok)
	require.NoError(t, err)

	converter := pkgTypes.NewConverter()
	converter.RegisterConverter(reflect.TypeOf(cov6CustomType{}), cov6ErrorConverter{})
	m = New(converter)

	av, ok, err = m.marshalUsingCustomConverter(reflect.ValueOf(cov6CustomType{Value: "x"}))
	require.Nil(t, av)
	require.False(t, ok)
	require.Error(t, err)

	converter = pkgTypes.NewConverter()
	converter.RegisterConverter(reflect.TypeOf(cov6CustomType{}), cov6GoodConverter{})
	m = New(converter)

	av, ok, err = m.marshalUsingCustomConverter(reflect.ValueOf(cov6CustomType{Value: "x"}))
	require.NoError(t, err)
	require.True(t, ok)
	require.Equal(t, "ok", requireAVS(t, av).Value)
}

func TestMarshalPointerAndInterface_CustomConverter_COV6(t *testing.T) {
	converter := pkgTypes.NewConverter()
	converter.RegisterConverter(reflect.TypeOf(cov6CustomType{}), cov6GoodConverter{})

	m := New(converter)

	value := cov6CustomType{Value: "x"}
	ptrValue := reflect.ValueOf(&value)
	av, err := m.marshalPointerValue(ptrValue)
	require.NoError(t, err)
	require.Equal(t, "ok", requireAVS(t, av).Value)

	var iface any = cov6CustomType{Value: "y"}
	ifaceValue := reflect.ValueOf(&iface).Elem()
	av, err = m.marshalInterfaceValue(ifaceValue)
	require.NoError(t, err)
	require.Equal(t, "ok", requireAVS(t, av).Value)

	var nilIface any
	nilIfaceValue := reflect.ValueOf(&nilIface).Elem()
	av, err = m.marshalInterfaceValue(nilIfaceValue)
	require.NoError(t, err)
	_, ok := av.(*types.AttributeValueMemberNULL)
	require.True(t, ok)
}

func TestBuildCustomConverterMarshalFunc_ErrorsWhenConverterMissing_COV6(t *testing.T) {
	m := New(nil)
	fn := m.buildCustomConverterMarshalFunc(reflect.TypeOf(cov6CustomType{}))

	value := cov6CustomType{Value: "x"}
	_, err := fn(unsafe.Pointer(&value)) //nolint:gosec // pointer used only for unit testing marshaling paths
	require.Error(t, err)
	require.Contains(t, err.Error(), "no converter configured")
}

func TestGetOrBuildStructMarshaler_RebuildsBadCacheEntry_COV6(t *testing.T) {
	type item struct {
		A string
	}

	meta := &model.Metadata{
		Fields: make(map[string]*model.FieldMetadata),
	}

	m := New(nil)
	typ := reflect.TypeOf(item{})

	m.cache.Store(typ, "bad")
	sm := m.getOrBuildStructMarshaler(typ, meta)
	require.NotNil(t, sm)
}

func TestMarshalValue_ReturnsErrorsForUnsupportedKinds_COV6(t *testing.T) {
	m := New(nil)

	av, err := m.marshalValue(reflect.Value{})
	require.NoError(t, err)
	_, ok := av.(*types.AttributeValueMemberNULL)
	require.True(t, ok)

	_, err = m.marshalValue(reflect.ValueOf(make(chan int)))
	require.Error(t, err)
}

func attributeValueStringList(t *testing.T, av types.AttributeValue) []string {
	t.Helper()

	list := requireAVL(t, av)
	values := make([]string, len(list.Value))
	for i, item := range list.Value {
		values[i] = requireAVS(t, item).Value
	}
	return values
}
