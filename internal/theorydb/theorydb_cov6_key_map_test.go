package theorydb

import (
	"testing"

	"github.com/aws/aws-sdk-go-v2/service/dynamodb/types"
	"github.com/stretchr/testify/require"

	"github.com/theory-cloud/tabletheory/pkg/core"
	queryPkg "github.com/theory-cloud/tabletheory/pkg/query"
)

type recordingBatchExecutor struct {
	lastBatchGet *queryPkg.CompiledBatchGet
}

func (e *recordingBatchExecutor) ExecuteQuery(_ *core.CompiledQuery, _ any) error { return nil }
func (e *recordingBatchExecutor) ExecuteScan(_ *core.CompiledQuery, _ any) error  { return nil }

func (e *recordingBatchExecutor) ExecuteBatchGet(input *queryPkg.CompiledBatchGet, _ *core.BatchGetOptions) ([]map[string]types.AttributeValue, error) {
	e.lastBatchGet = input
	return nil, nil
}

func (e *recordingBatchExecutor) ExecuteBatchWrite(_ *queryPkg.CompiledBatchWrite) error { return nil }

func TestQuery_BatchGet_KeyConversion_COV6(t *testing.T) {
	db := newBareDB()
	require.NoError(t, db.registry.Register(&cov4RootItem{}))

	meta, err := db.registry.GetMetadata(&cov4RootItem{})
	require.NoError(t, err)

	exec := &recordingBatchExecutor{}
	q := queryPkg.New(&cov4RootItem{}, &metadataAdapter{metadata: meta}, exec).
		WithConverter(db.converter).
		WithMarshaler(db.marshaler)

	var nilPtr *cov4RootItem
	var out []cov4RootItem
	require.NoError(t, q.BatchGet([]any{
		cov4RootItem{ID: "u1"},
		&cov4RootItem{ID: "u2"},
		"u3",
		nilPtr,
	}, &out))

	require.NotNil(t, exec.lastBatchGet)
	require.Len(t, exec.lastBatchGet.Keys, 4)

	id1, ok := exec.lastBatchGet.Keys[0]["id"].(*types.AttributeValueMemberS)
	require.True(t, ok)
	require.Equal(t, "u1", id1.Value)

	id2, ok := exec.lastBatchGet.Keys[1]["id"].(*types.AttributeValueMemberS)
	require.True(t, ok)
	require.Equal(t, "u2", id2.Value)

	id3, ok := exec.lastBatchGet.Keys[2]["id"].(*types.AttributeValueMemberS)
	require.True(t, ok)
	require.Equal(t, "u3", id3.Value)

	_, ok = exec.lastBatchGet.Keys[3]["id"].(*types.AttributeValueMemberNULL)
	require.True(t, ok)
}
