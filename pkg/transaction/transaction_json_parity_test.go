package transaction

import (
	"encoding/json"
	"testing"

	"github.com/aws/aws-sdk-go-v2/service/dynamodb/types"
	"github.com/stretchr/testify/require"

	"github.com/theory-cloud/tabletheory/v3/pkg/model"
	"github.com/theory-cloud/tabletheory/v3/pkg/query"
	"github.com/theory-cloud/tabletheory/v3/pkg/session"
	pkgTypes "github.com/theory-cloud/tabletheory/v3/pkg/types"
)

type transactionalJSONRecord struct {
	PK      string          `theorydb:"pk,attr:PK" json:"PK"`
	Payload json.RawMessage `theorydb:"json,attr:payload" json:"payload"`
	Legacy  json.RawMessage `theorydb:"attr:legacy" json:"legacy"`
}

func (transactionalJSONRecord) TableName() string {
	return "transactional_json_records"
}

func TestBuilderUpdateMatchesQueryJSONNormalization(t *testing.T) {
	record := &transactionalJSONRecord{
		PK:      "RECORD#json",
		Payload: json.RawMessage(`{"count":4}`),
		Legacy:  json.RawMessage(`{"count":4}`),
	}
	registry := model.NewRegistry()
	require.NoError(t, registry.Register(record))
	metadata, err := registry.GetMetadata(record)
	require.NoError(t, err)

	capture := &capturingUpdateExecutor{}
	q := query.New(record, adaptMetadata(metadata), capture)
	q.Where("PK", "=", record.PK)
	require.NoError(t, q.Update("Payload", "Legacy"))
	require.NotNil(t, capture.compiled)

	builder := NewBuilder(&session.Session{}, registry, pkgTypes.NewConverter())
	builder.Update(record, []string{"Payload", "Legacy"})
	items, err := builder.materializeOperations()
	require.NoError(t, err)
	require.Len(t, items, 1)
	update := items[0].Update
	require.NotNil(t, update)

	require.Equal(t, capture.compiled.ExpressionAttributeNames, update.ExpressionAttributeNames)
	require.Equal(t, capture.compiled.ExpressionAttributeValues, update.ExpressionAttributeValues)
	require.Contains(t, update.ExpressionAttributeValues, ":v1")
	require.Contains(t, update.ExpressionAttributeValues, ":v2")
	require.Equal(t, &types.AttributeValueMemberS{Value: `{"count":4}`}, update.ExpressionAttributeValues[":v1"])
	require.Equal(t, &types.AttributeValueMemberB{Value: []byte(`{"count":4}`)}, update.ExpressionAttributeValues[":v2"])
}
