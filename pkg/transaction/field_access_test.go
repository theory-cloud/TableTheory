package transaction

import (
	"context"
	"reflect"
	"testing"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb/types"
	"github.com/stretchr/testify/require"

	"github.com/theory-cloud/tabletheory/v4/pkg/core"
	"github.com/theory-cloud/tabletheory/v4/pkg/model"
	"github.com/theory-cloud/tabletheory/v4/pkg/session"
	pkgTypes "github.com/theory-cloud/tabletheory/v4/pkg/types"
)

// PromotedFieldBase carries every metadata-bearing field inside an embedded
// struct, so each field's index path is longer than one element. The outer struct
// also holds one extra field, which is what a positional read would pick up.
type PromotedFieldBase struct {
	Note    int `theorydb:"attr:note" json:"note"`
	Version int `theorydb:"version,attr:version" json:"version"`
	ID      int `theorydb:"pk,attr:id" json:"id"`
	Sort    int `theorydb:"sk,attr:sort" json:"sort"`
}

type promotedFieldRecord struct {
	PromotedFieldBase
	Value int `theorydb:"attr:value" json:"value"`
}

func newPromotedFieldRegistry(t *testing.T) *model.Registry {
	t.Helper()

	registry := model.NewRegistry()
	require.NoError(t, registry.Register(&promotedFieldRecord{}))
	return registry
}

func newPromotedFieldSession(t *testing.T) *session.Session {
	t.Helper()

	stubSessionConfigLoad(t, func(context.Context, ...func(*config.LoadOptions) error) (aws.Config, error) {
		return minimalAWSConfig(stubHTTPClient{}), nil
	})

	sess, err := session.NewSession(&session.Config{Region: "us-east-1"})
	require.NoError(t, err)
	return sess
}

func newPromotedFieldRecord() *promotedFieldRecord {
	return &promotedFieldRecord{
		PromotedFieldBase: PromotedFieldBase{
			Note:    42,
			Version: 4,
			ID:      7,
			Sort:    8,
		},
		Value: 99,
	}
}

func TestFieldByIndexPath_GuardsDivergentMetadata(t *testing.T) {
	type record struct{ Value string }

	tests := []struct {
		name      string
		model     reflect.Value
		indexPath []int
	}{
		{name: "empty path", model: reflect.ValueOf(record{}), indexPath: nil},
		{name: "out of range", model: reflect.ValueOf(record{}), indexPath: []int{7}},
		{name: "negative index", model: reflect.ValueOf(record{}), indexPath: []int{-1}},
		{name: "path through a non-struct field", model: reflect.ValueOf(record{}), indexPath: []int{0, 0}},
		{name: "invalid model value", model: reflect.Value{}, indexPath: []int{0}},
		{name: "non-struct model value", model: reflect.ValueOf("not a struct"), indexPath: []int{0}},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			var (
				field reflect.Value
				err   error
			)
			require.NotPanics(t, func() {
				field, err = fieldByIndexPath(test.model, test.indexPath)
			})
			require.Error(t, err)
			require.False(t, field.IsValid())
		})
	}
}

func TestFieldByIndexPath_ResolvesPromotedFields(t *testing.T) {
	value := reflect.ValueOf(*newPromotedFieldRecord())

	registry := model.NewRegistry()
	require.NoError(t, registry.Register(&promotedFieldRecord{}))
	metadata, err := registry.GetMetadata(&promotedFieldRecord{})
	require.NoError(t, err)

	pkValue, err := fieldByIndexPath(value, metadata.PrimaryKey.PartitionKey.IndexPath)
	require.NoError(t, err)
	require.Equal(t, 7, pkValue.Interface())

	versionValue, err := fieldByIndexPath(value, metadata.VersionField.IndexPath)
	require.NoError(t, err)
	require.Equal(t, 4, versionValue.Interface())
}

func TestTransaction_PromotedKeyFieldsResolveThroughIndexPath(t *testing.T) {
	registry := newPromotedFieldRegistry(t)
	tx := NewTransaction(newPromotedFieldSession(t), registry, pkgTypes.NewConverter())

	require.NoError(t, tx.Delete(newPromotedFieldRecord()))
	require.Len(t, tx.writes, 1)

	deleteItem := tx.writes[0].Delete
	require.NotNil(t, deleteItem)
	require.Equal(t, "7", attributeValueString(t, deleteItem.Key["id"]),
		"the delete key must read the promoted partition key, not the outer field at the same position")
	require.Equal(t, "8", attributeValueString(t, deleteItem.Key["sort"]))
	require.Equal(t, "#ver = :ver", aws.ToString(deleteItem.ConditionExpression))
	require.Equal(t, "4", attributeValueString(t, deleteItem.ExpressionAttributeValues[":ver"]))
}

func TestTransaction_CreateMarshalsPromotedFields(t *testing.T) {
	registry := newPromotedFieldRegistry(t)
	tx := NewTransaction(newPromotedFieldSession(t), registry, pkgTypes.NewConverter())

	require.NotPanics(t, func() {
		require.NoError(t, tx.Create(newPromotedFieldRecord()))
	})
	require.Len(t, tx.writes, 1)

	put := tx.writes[0].Put
	require.NotNil(t, put)
	require.Equal(t, "7", attributeValueString(t, put.Item["id"]))
	require.Equal(t, "8", attributeValueString(t, put.Item["sort"]))
	require.Equal(t, "42", attributeValueString(t, put.Item["note"]))
	require.Equal(t, "99", attributeValueString(t, put.Item["value"]))
}

func TestBuilder_PromotedFieldsResolveThroughIndexPath(t *testing.T) {
	registry := newPromotedFieldRegistry(t)

	t.Run("field update", func(t *testing.T) {
		builder := NewBuilder(&session.Session{}, registry, pkgTypes.NewConverter())
		builder.Update(newPromotedFieldRecord(), []string{"Note"})

		items, err := builder.materializeOperations()
		require.NoError(t, err)
		require.Len(t, items, 1)

		update := items[0].Update
		require.NotNil(t, update)
		require.Len(t, update.ExpressionAttributeNames, 1)
		require.Len(t, update.ExpressionAttributeValues, 1)

		nameRef, valueRef := "", ""
		for name, attrName := range update.ExpressionAttributeNames {
			require.Equal(t, "note", attrName)
			nameRef = name
		}
		for value, av := range update.ExpressionAttributeValues {
			require.Equal(t, "42", attributeValueString(t, av))
			valueRef = value
		}
		require.Equal(t, "SET "+nameRef+" = "+valueRef, aws.ToString(update.UpdateExpression))
	})

	t.Run("builder update keys", func(t *testing.T) {
		builder := NewBuilder(&session.Session{}, registry, pkgTypes.NewConverter())
		builder.UpdateWithBuilder(newPromotedFieldRecord(), func(ub core.UpdateBuilder) error {
			ub.Set("Value", 100)
			return nil
		})

		items, err := builder.materializeOperations()
		require.NoError(t, err)
		require.Len(t, items, 1)
		require.Equal(t, "7", attributeValueString(t, items[0].Update.Key["id"]))
		require.Equal(t, "8", attributeValueString(t, items[0].Update.Key["sort"]))
	})

	t.Run("put", func(t *testing.T) {
		builder := NewBuilder(&session.Session{}, registry, pkgTypes.NewConverter())
		builder.Put(newPromotedFieldRecord())

		items, err := builder.materializeOperations()
		require.NoError(t, err)
		require.Len(t, items, 1)
		require.Equal(t, "7", attributeValueString(t, items[0].Put.Item["id"]))
		require.Equal(t, "8", attributeValueString(t, items[0].Put.Item["sort"]))
	})
}

func attributeValueString(t *testing.T, av types.AttributeValue) string {
	t.Helper()

	switch value := av.(type) {
	case *types.AttributeValueMemberS:
		return value.Value
	case *types.AttributeValueMemberN:
		return value.Value
	default:
		require.Failf(t, "unexpected attribute value", "got %T", av)
		return ""
	}
}
