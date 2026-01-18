package query

import (
	"reflect"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go-v2/service/dynamodb/types"
	"github.com/stretchr/testify/require"

	"github.com/theory-cloud/tabletheory/internal/expr"
	"github.com/theory-cloud/tabletheory/pkg/core"
)

func TestQuery_addPrimaryKeyCondition_ErrorsAndCompositeKeys_COV6(t *testing.T) {
	q := &Query{}
	q.addPrimaryKeyCondition("attribute_exists")
	require.Error(t, q.checkBuilderError())

	q = &Query{metadata: cov5Metadata{primaryKey: core.KeySchema{PartitionKey: ""}}}
	q.addPrimaryKeyCondition("attribute_exists")
	require.Error(t, q.checkBuilderError())

	q = &Query{metadata: cov5Metadata{primaryKey: core.KeySchema{PartitionKey: "pk", SortKey: "sk"}}}
	q.addPrimaryKeyCondition("attribute_exists")
	require.Len(t, q.writeConditions, 2, "composite keys include sort key existence check")
	require.Equal(t, "attribute_exists", q.writeConditions[0].Operator)
	require.Equal(t, "attribute_exists", q.writeConditions[1].Operator)
}

func TestQuery_resolveAttributeName_UsesMetaNameWhenNoDynamoDBName_COV6(t *testing.T) {
	q := &Query{metadata: cov5Metadata{
		attributes: map[string]*core.AttributeMetadata{
			"Field": {Name: "GoName", DynamoDBName: ""},
		},
	}}

	require.Equal(t, "GoName", q.resolveAttributeName("Field"))
	require.Equal(t, "", q.resolveAttributeName(""))
}

func TestQuery_resolveConditionNames_CoversMetadataLookups_COV6(t *testing.T) {
	meta := cov5Metadata{
		attributes: map[string]*core.AttributeMetadata{
			"go":   {Name: "GoResolved", DynamoDBName: "attrResolved"},
			"attr": {Name: "OtherGo", DynamoDBName: "otherAttr"},
			"only": {Name: "OnlyName"},
		},
	}
	q := &Query{metadata: meta}

	goName, attrName := q.resolveConditionNames("go", "attr")
	require.Equal(t, "GoResolved", goName)
	require.Equal(t, "attrResolved", attrName)

	goName, attrName = q.resolveConditionNames("missing", "attr")
	require.Equal(t, "OtherGo", goName)
	require.Equal(t, "otherAttr", attrName)

	goName, attrName = q.resolveConditionNames("only", "")
	require.Equal(t, "OnlyName", goName)
	require.Equal(t, "OnlyName", attrName)
}

func TestQuery_addDefaultCondition_ErrorBranches_COV6(t *testing.T) {
	builder := expr.NewBuilder()

	q := &Query{}
	require.Error(t, q.addDefaultCondition(builder))

	q = &Query{metadata: cov5Metadata{primaryKey: core.KeySchema{PartitionKey: ""}}}
	require.Error(t, q.addDefaultCondition(builder))

	q = &Query{metadata: cov5Metadata{primaryKey: core.KeySchema{PartitionKey: "pk"}}}
	require.NoError(t, q.addDefaultCondition(expr.NewBuilder()))
}

func TestQuery_addWhereConditions_MetadataRequiredAndSkipKeys_COV6(t *testing.T) {
	q := &Query{}
	_, err := q.addWhereConditions(expr.NewBuilder(), false)
	require.Error(t, err)

	q = &Query{
		metadata: cov5Metadata{primaryKey: core.KeySchema{PartitionKey: "pk"}},
		conditions: []Condition{
			{Field: "pk", Operator: "=", Value: "p1"},
			{Field: "other", Operator: "=", Value: "x"},
		},
	}

	builder := expr.NewBuilder()
	added, err := q.addWhereConditions(builder, true)
	require.NoError(t, err)
	require.True(t, added)

	components := builder.Build()
	names := make([]string, 0, len(components.ExpressionAttributeNames))
	for _, v := range components.ExpressionAttributeNames {
		names = append(names, v)
	}
	require.Contains(t, names, "other")
	require.NotContains(t, names, "pk")
}

func TestExtractAndValidateKeyValues_ErrorBranches_COV6(t *testing.T) {
	pk := core.KeySchema{PartitionKey: "pk", SortKey: "sk"}

	_, err := extractKeyValues(pk, []Condition{{Field: "pk", Operator: "<", Value: "p1"}})
	require.Error(t, err)

	_, err = extractKeyValues(pk, []Condition{{Field: "pk", Operator: "=", Value: "p1"}})
	require.NoError(t, err)

	require.Error(t, validateKeyValues(pk, map[string]any{"sk": "s1"}, "op"))
	require.Error(t, validateKeyValues(pk, map[string]any{"pk": "p1"}, "op"))
	require.NoError(t, validateKeyValues(pk, map[string]any{"pk": "p1", "sk": "s1"}, "op"))
}

func TestShouldSkipUpdateField_TagRules_COV6(t *testing.T) {
	type item struct {
		CreatedAt time.Time `theorydb:"created_at"`
		Ignored   string    `theorydb:"-"`
		PK        string    `theorydb:"pk"`
		SK        string    `theorydb:"sk"`
		Value     string    `theorydb:"attr:value"`
	}

	typ := reflect.TypeOf(item{})
	primaryKey := core.KeySchema{PartitionKey: "PK", SortKey: "SK"}

	for i := 0; i < typ.NumField(); i++ {
		field := typ.Field(i)
		tag := field.Tag.Get("theorydb")
		skip := shouldSkipUpdateField(field, tag, primaryKey)
		switch field.Name {
		case "Value":
			require.False(t, skip)
		default:
			require.True(t, skip)
		}
	}
}

func TestQuery_EncodeDecodeCursor_ErrorBranches_COV6(t *testing.T) {
	q := &Query{index: "idx", orderBy: OrderBy{Order: "DESC"}}

	_, err := q.decodeCursor("not-base64")
	require.Error(t, err)

	lastKeyAny := map[string]any{
		"LastEvaluatedKey": map[string]types.AttributeValue{"pk": &types.AttributeValueMemberS{Value: "p1"}},
	}
	require.NotEmpty(t, q.encodeCursor(lastKeyAny))

	type unsupportedAV struct{ types.AttributeValue }
	lastKeyBad := map[string]types.AttributeValue{"pk": &unsupportedAV{}}
	require.Equal(t, "", q.encodeCursor(lastKeyBad))
}

func TestQuery_WithConditionExpression_RejectsEmpty_COV6(t *testing.T) {
	q := &Query{}
	q.WithConditionExpression("   ", nil)
	require.Error(t, q.checkBuilderError())
}
