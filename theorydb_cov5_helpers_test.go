package theorydb

import (
	"errors"
	"reflect"
	"testing"
	"time"

	ddbTypes "github.com/aws/aws-sdk-go-v2/service/dynamodb/types"
	"github.com/stretchr/testify/require"

	"github.com/theory-cloud/tabletheory/pkg/marshal"
	"github.com/theory-cloud/tabletheory/pkg/model"
	pkgTypes "github.com/theory-cloud/tabletheory/pkg/types"
)

type cov5StringConverter struct{}

func (cov5StringConverter) ToAttributeValue(value any) (ddbTypes.AttributeValue, error) {
	s, ok := value.(string)
	if !ok {
		return nil, errors.New("expected string")
	}
	return &ddbTypes.AttributeValueMemberS{Value: s}, nil
}

func (cov5StringConverter) FromAttributeValue(av ddbTypes.AttributeValue, target any) error {
	member, ok := av.(*ddbTypes.AttributeValueMemberS)
	if !ok {
		return errors.New("expected string attribute")
	}
	dst, ok := target.(*string)
	if !ok {
		return errors.New("expected *string target")
	}
	*dst = member.Value
	return nil
}

func TestDB_RegisterTypeConverter_ValidatesInputs_COV5(t *testing.T) {
	converter := pkgTypes.NewConverter()
	db := &DB{
		converter: converter,
		marshaler: marshal.New(converter),
	}

	require.Error(t, db.RegisterTypeConverter(nil, cov5StringConverter{}))
	require.Error(t, db.RegisterTypeConverter(reflect.TypeOf(""), nil))

	require.NoError(t, db.RegisterTypeConverter(reflect.TypeOf(""), cov5StringConverter{}))
	require.True(t, db.converter.HasCustomConverter(reflect.TypeOf("")))
}

func TestQueryExecutor_unmarshalItem_MapDestinationAndTypeErrors_COV5(t *testing.T) {
	executor := &queryExecutor{db: &DB{converter: pkgTypes.NewConverter()}}

	var emptyDest map[string]any
	require.NoError(t, executor.unmarshalItem(map[string]ddbTypes.AttributeValue{}, &emptyDest))
	require.NotNil(t, emptyDest)

	item := map[string]ddbTypes.AttributeValue{
		"id": &ddbTypes.AttributeValueMemberS{Value: "u1"},
		"n":  &ddbTypes.AttributeValueMemberN{Value: "3"},
	}

	var dest map[string]any
	require.NoError(t, executor.unmarshalItem(item, &dest))
	require.NotNil(t, dest)
	require.Equal(t, "u1", dest["id"])
	require.EqualValues(t, 3, dest["n"])

	require.Error(t, executor.unmarshalItem(item, dest))

	var invalidKeyDest map[int]any
	require.Error(t, executor.unmarshalItem(item, &invalidKeyDest))

	var invalid int
	require.Error(t, executor.unmarshalItem(item, &invalid))
}

func TestQueryExecutor_checkLambdaTimeout_CoversDeadlineBranches_COV5(t *testing.T) {
	executor := &queryExecutor{db: &DB{}}
	require.NoError(t, executor.checkLambdaTimeout())

	executor = &queryExecutor{db: &DB{lambdaDeadline: time.Now().Add(-time.Second)}}
	require.Error(t, executor.checkLambdaTimeout())

	executor = &queryExecutor{db: &DB{lambdaDeadline: time.Now().Add(200 * time.Millisecond), lambdaTimeoutBuffer: time.Second}}
	require.Error(t, executor.checkLambdaTimeout())

	executor = &queryExecutor{db: &DB{lambdaDeadline: time.Now().Add(500 * time.Millisecond), lambdaTimeoutBuffer: 10 * time.Millisecond}}
	require.NoError(t, executor.checkLambdaTimeout())
}

func TestErrorBatchGetBuilder_FluentMethodsReturnSelf_COV5(t *testing.T) {
	errBoom := errors.New("boom")
	b := &errorBatchGetBuilder{err: errBoom}

	require.Same(t, b, b.Keys(nil))
	require.Same(t, b, b.ChunkSize(1))
	require.Same(t, b, b.ConsistentRead())
	require.Same(t, b, b.Parallel(2))
	require.Same(t, b, b.WithRetry(nil))
	require.Same(t, b, b.Select("ID"))
	require.Same(t, b, b.OnProgress(nil))
	require.Same(t, b, b.OnError(nil))
	require.ErrorIs(t, b.Execute(nil), errBoom)
}

func TestMetadataAdapter_CoversPrimaryKeyIndexesAndAttributes_COV5(t *testing.T) {
	pk := &model.FieldMetadata{
		Name:   "PK",
		DBName: "pk",
		Type:   reflect.TypeOf(""),
		Tags: map[string]string{
			"tag": "value",
		},
	}
	sk := &model.FieldMetadata{
		Name:   "SK",
		DBName: "sk",
		Type:   reflect.TypeOf(""),
	}

	meta := &model.Metadata{
		TableName: "tbl",
		PrimaryKey: &model.KeySchema{
			PartitionKey: pk,
			SortKey:      sk,
		},
		Fields: map[string]*model.FieldMetadata{
			"PK": pk,
			"SK": sk,
		},
		FieldsByDBName: map[string]*model.FieldMetadata{
			"pk": pk,
			"sk": sk,
		},
		Indexes: []model.IndexSchema{
			{
				Name:         "byGSI",
				Type:         model.GlobalSecondaryIndex,
				PartitionKey: pk,
				SortKey:      sk,
			},
			{
				Name:         "byPK",
				Type:         model.LocalSecondaryIndex,
				PartitionKey: pk,
			},
		},
		VersionField: &model.FieldMetadata{
			Name:   "Version",
			DBName: "ver",
		},
	}

	adapter := &metadataAdapter{metadata: meta}
	require.Equal(t, "tbl", adapter.TableName())
	require.Same(t, meta, adapter.RawMetadata())

	keySchema := adapter.PrimaryKey()
	require.Equal(t, "PK", keySchema.PartitionKey)
	require.Equal(t, "SK", keySchema.SortKey)

	indexes := adapter.Indexes()
	require.Len(t, indexes, 2)
	require.Equal(t, "byGSI", indexes[0].Name)
	require.Equal(t, "PK", indexes[0].PartitionKey)
	require.Equal(t, "SK", indexes[0].SortKey)
	require.Equal(t, "byPK", indexes[1].Name)
	require.Equal(t, "PK", indexes[1].PartitionKey)
	require.Empty(t, indexes[1].SortKey)

	attr := adapter.AttributeMetadata("PK")
	require.NotNil(t, attr)
	require.Equal(t, "PK", attr.Name)
	require.Equal(t, "pk", attr.DynamoDBName)
	require.Equal(t, "tag", func() string {
		for k := range attr.Tags {
			return k
		}
		return ""
	}())

	attr = adapter.AttributeMetadata("pk")
	require.NotNil(t, attr)
	require.Nil(t, adapter.AttributeMetadata("missing"))

	require.Equal(t, "ver", adapter.VersionFieldName())
	meta.VersionField.DBName = ""
	require.Equal(t, "Version", adapter.VersionFieldName())
	meta.VersionField = nil
	require.Empty(t, adapter.VersionFieldName())

	emptyAdapter := &metadataAdapter{metadata: &model.Metadata{TableName: "tbl"}}
	emptySchema := emptyAdapter.PrimaryKey()
	require.Empty(t, emptySchema.PartitionKey)
	require.Empty(t, emptySchema.SortKey)
	require.Nil(t, emptyAdapter.AttributeMetadata("pk"))

	nilAdapter := &metadataAdapter{}
	require.Nil(t, nilAdapter.RawMetadata())
	require.Empty(t, nilAdapter.VersionFieldName())
}
