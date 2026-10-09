package transaction

import (
	"context"
	"testing"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb/types"
	"github.com/stretchr/testify/require"

	"github.com/theory-cloud/tabletheory/v4/pkg/model"
	"github.com/theory-cloud/tabletheory/v4/pkg/session"
	pkgTypes "github.com/theory-cloud/tabletheory/v4/pkg/types"
)

// PromotedVersionBase carries the optimistic-lock version inside an embedded
// struct, so the version field's metadata index path spans more than one element.
type PromotedVersionBase struct {
	Note    int `theorydb:"attr:note,omitempty" json:"note,omitempty"`
	Version int `theorydb:"version,attr:version" json:"version"`
}

// promotedVersionRecord deliberately places a numeric field at the outer struct
// position equal to the last element of the version field's index path. A
// positional read therefore silently returns Counter instead of Version.
type promotedVersionRecord struct {
	PromotedVersionBase
	Counter int `theorydb:"attr:counter" json:"counter"`
	ID      int `theorydb:"pk,attr:id" json:"id"`
	Sort    int `theorydb:"sk,attr:sort" json:"sort"`
}

func newPromotedVersionTransaction(t *testing.T) *Transaction {
	t.Helper()

	stubSessionConfigLoad(t, func(context.Context, ...func(*config.LoadOptions) error) (aws.Config, error) {
		return minimalAWSConfig(stubHTTPClient{}), nil
	})

	sess, err := session.NewSession(&session.Config{Region: "us-east-1"})
	require.NoError(t, err)

	registry := model.NewRegistry()
	require.NoError(t, registry.Register(&promotedVersionRecord{}))

	metadata, err := registry.GetMetadata(&promotedVersionRecord{})
	require.NoError(t, err)
	require.NotNil(t, metadata.VersionField)
	require.Len(t, metadata.VersionField.IndexPath, 2,
		"the regression is only meaningful while the version field is promoted from an embedded struct")

	return NewTransaction(sess, registry, pkgTypes.NewConverter())
}

func TestTransaction_DeleteReadsPromotedVersionField(t *testing.T) {
	tx := newPromotedVersionTransaction(t)

	require.NoError(t, tx.Delete(&promotedVersionRecord{
		PromotedVersionBase: PromotedVersionBase{Version: 7},
		Counter:             999,
		ID:                  1,
		Sort:                2,
	}))

	require.Len(t, tx.writes, 1)
	deleteItem := tx.writes[0].Delete
	require.NotNil(t, deleteItem)
	require.Equal(t, "#ver = :ver", aws.ToString(deleteItem.ConditionExpression))

	versionValue, ok := deleteItem.ExpressionAttributeValues[":ver"].(*types.AttributeValueMemberN)
	require.True(t, ok)
	require.Equal(t, "7", versionValue.Value,
		"the delete condition must read the promoted version field, not the outer field at the same positional index")
}

func TestTransaction_UpdateReadsPromotedVersionField(t *testing.T) {
	tx := newPromotedVersionTransaction(t)

	require.NoError(t, tx.Update(&promotedVersionRecord{
		PromotedVersionBase: PromotedVersionBase{Version: 7},
		Counter:             999,
		ID:                  1,
		Sort:                2,
	}))

	require.Len(t, tx.writes, 1)
	updateItem := tx.writes[0].Update
	require.NotNil(t, updateItem)

	versionValue, ok := updateItem.ExpressionAttributeValues[":currentVer"].(*types.AttributeValueMemberN)
	require.True(t, ok)
	require.Equal(t, "7", versionValue.Value)
}
