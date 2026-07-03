package theorydb

import (
	"errors"
	"testing"

	"github.com/aws/aws-sdk-go-v2/service/dynamodb/types"
	"github.com/stretchr/testify/require"

	"github.com/theory-cloud/tabletheory/pkg/core"
	theorydbErrors "github.com/theory-cloud/tabletheory/pkg/errors"
	"github.com/theory-cloud/tabletheory/pkg/model"
)

func TestQueryExecutorConditionFailedErrorDistinguishesVersionConflict(t *testing.T) {
	qe := &queryExecutor{metadata: &model.Metadata{
		VersionField: &model.FieldMetadata{DBName: "revision"},
	}}

	versionQuery := &core.CompiledQuery{
		ConditionExpression: "#ver = :expected",
		ExpressionAttributeNames: map[string]string{
			"#ver": "revision",
		},
		ExpressionAttributeValues: map[string]types.AttributeValue{
			":expected": &types.AttributeValueMemberN{Value: "1"},
		},
	}

	err := qe.conditionFailedError(versionQuery)
	require.True(t, errors.Is(err, theorydbErrors.ErrVersionConflict))
	require.True(t, errors.Is(err, theorydbErrors.ErrConditionFailed))

	statusQuery := &core.CompiledQuery{
		ConditionExpression: "#status = :active",
		ExpressionAttributeNames: map[string]string{
			"#status": "status",
		},
		ExpressionAttributeValues: map[string]types.AttributeValue{
			":active": &types.AttributeValueMemberS{Value: "active"},
		},
	}

	err = qe.conditionFailedError(statusQuery)
	require.True(t, errors.Is(err, theorydbErrors.ErrConditionFailed))
	require.False(t, errors.Is(err, theorydbErrors.ErrVersionConflict))
}
