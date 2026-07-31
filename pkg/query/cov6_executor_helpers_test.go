package query

import (
	"errors"
	"reflect"
	"testing"

	"github.com/aws/aws-sdk-go-v2/service/dynamodb/types"
	"github.com/stretchr/testify/require"

	"github.com/theory-cloud/tabletheory/v3/pkg/core"
	theorydbErrors "github.com/theory-cloud/tabletheory/v3/pkg/errors"
)

type the2551RetryReadItem struct {
	ID string
}

type the2551RetryReadExecutor struct {
	batches [][]the2551RetryReadItem
	errs    []error
	calls   int
}

func (e *the2551RetryReadExecutor) ExecuteQuery(input *core.CompiledQuery, dest any) error {
	return e.ExecuteScan(input, dest)
}

func (e *the2551RetryReadExecutor) ExecuteScan(_ *core.CompiledQuery, dest any) error {
	call := e.calls
	e.calls++

	if call < len(e.errs) && e.errs[call] != nil {
		return e.errs[call]
	}
	if call >= len(e.batches) {
		return nil
	}

	reflect.ValueOf(dest).Elem().Set(reflect.ValueOf(e.batches[call]))
	return nil
}

func TestQueryRetryReadsRetryEmptyResults_THE2551(t *testing.T) {
	exec := &the2551RetryReadExecutor{
		batches: [][]the2551RetryReadItem{
			{},
			{},
			{{ID: "ready"}},
		},
	}
	q := New(&the2551RetryReadItem{}, cov5Metadata{
		table:      "tbl",
		primaryKey: core.KeySchema{PartitionKey: "id"},
	}, exec)

	var out the2551RetryReadItem
	err := q.WithRetry(2, 0).First(&out)
	require.NoError(t, err)
	require.Equal(t, "ready", out.ID)
	require.Equal(t, 3, exec.calls)
}

func TestQueryAllWithRetryClearsStaleDestination_THE2551(t *testing.T) {
	exec := &the2551RetryReadExecutor{
		batches: [][]the2551RetryReadItem{
			{},
			{{ID: "fresh"}},
		},
	}
	q := New(&the2551RetryReadItem{}, cov5Metadata{
		table:      "tbl",
		primaryKey: core.KeySchema{PartitionKey: "id"},
	}, exec)

	out := []the2551RetryReadItem{{ID: "stale"}}
	err := q.WithRetry(1, 0).All(&out)
	require.NoError(t, err)
	require.Equal(t, []the2551RetryReadItem{{ID: "fresh"}}, out)
	require.Equal(t, 2, exec.calls)
}

func TestQueryRetryReadsReturnTerminalErrors_THE2551(t *testing.T) {
	t.Run("first returns not found after retry budget", func(t *testing.T) {
		exec := &the2551RetryReadExecutor{
			batches: [][]the2551RetryReadItem{{}},
		}
		q := New(&the2551RetryReadItem{}, cov5Metadata{
			table:      "tbl",
			primaryKey: core.KeySchema{PartitionKey: "id"},
		}, exec)

		var out the2551RetryReadItem
		err := q.WithRetry(0, 0).First(&out)
		require.ErrorIs(t, err, theorydbErrors.ErrItemNotFound)
		require.Equal(t, 1, exec.calls)
	})

	t.Run("all returns last executor error after retry budget", func(t *testing.T) {
		expected := errors.New("read failed")
		exec := &the2551RetryReadExecutor{
			errs: []error{errors.New("transient"), expected},
		}
		q := New(&the2551RetryReadItem{}, cov5Metadata{
			table:      "tbl",
			primaryKey: core.KeySchema{PartitionKey: "id"},
		}, exec)

		var out []the2551RetryReadItem
		err := q.WithRetry(1, 0).All(&out)
		require.ErrorIs(t, err, expected)
		require.Equal(t, 2, exec.calls)
	})
}

func TestUnmarshalJSONString_ErrorBranches_COV6(t *testing.T) {
	t.Run("rejects non-addressable value", func(t *testing.T) {
		dest := reflect.ValueOf(map[string]string{})
		require.False(t, dest.CanAddr())
		require.Error(t, unmarshalJSONString(`{"a":"b"}`, dest))
	})

	t.Run("propagates json errors", func(t *testing.T) {
		var dest map[string]string
		destValue := reflect.ValueOf(&dest).Elem()
		require.True(t, destValue.CanAddr())
		require.Error(t, unmarshalJSONString(`{not-json`, destValue))
	})
}

func TestAttributeValueToInterface_DefaultBranch_COV6(t *testing.T) {
	type unsupportedAV struct{ types.AttributeValue }

	_, err := attributeValueToInterface(&unsupportedAV{})
	require.Error(t, err)
}

func TestParseAttributeName_TrimsAndHandlesEmpty_COV6(t *testing.T) {
	require.Equal(t, "", parseAttributeName(""))
	require.Equal(t, "name", parseAttributeName(" name ,omitempty"))
}

func TestEncryptedTagAndEnvelopeHelpers_COV6(t *testing.T) {
	type model struct {
		Secret string `theorydb:"encrypted,attr:secret"`
		Plain  string `theorydb:"attr:plain"`
		Empty  string
	}

	modelType := reflect.TypeOf(model{})
	require.True(t, fieldHasEncryptedTag(modelType.Field(0)))
	require.False(t, fieldHasEncryptedTag(modelType.Field(1)))
	require.False(t, fieldHasEncryptedTag(modelType.Field(2)))

	validEnvelope := &types.AttributeValueMemberM{Value: map[string]types.AttributeValue{
		"v":     &types.AttributeValueMemberN{Value: "1"},
		"edk":   &types.AttributeValueMemberB{Value: []byte("edk")},
		"nonce": &types.AttributeValueMemberB{Value: []byte("nonce")},
		"ct":    &types.AttributeValueMemberB{Value: []byte("ciphertext")},
	}}
	require.True(t, looksLikeEncryptedEnvelope(validEnvelope))

	require.False(t, looksLikeEncryptedEnvelope(&types.AttributeValueMemberS{Value: "plaintext"}))
	require.False(t, looksLikeEncryptedEnvelope(&types.AttributeValueMemberM{Value: map[string]types.AttributeValue{
		"v":   &types.AttributeValueMemberN{Value: "1"},
		"edk": &types.AttributeValueMemberB{Value: []byte("edk")},
		"ct":  &types.AttributeValueMemberB{Value: []byte("ciphertext")},
	}}))
}

func TestUnmarshalItems_PointerSliceAndErrorBranches_COV6(t *testing.T) {
	type model struct {
		ID    string `dynamodb:"id"`
		Count int    `dynamodb:"count"`
	}

	items := []map[string]types.AttributeValue{
		{"id": &types.AttributeValueMemberS{Value: "1"}},
	}

	var out []*model
	require.NoError(t, UnmarshalItems(items, &out))
	require.Len(t, out, 1)
	require.NotNil(t, out[0])
	require.Equal(t, "1", out[0].ID)

	var nonPointerDest []model
	require.EqualError(t, UnmarshalItems(items, nonPointerDest), "destination must be a pointer")

	err := UnmarshalItems([]map[string]types.AttributeValue{
		{
			"id":    &types.AttributeValueMemberS{Value: "1"},
			"count": &types.AttributeValueMemberS{Value: "not-a-number"},
		},
	}, &[]model{})
	require.Error(t, err)
}

func TestUnmarshalNumberAttribute_CoversNumericKinds_COV6(t *testing.T) {
	t.Run("uint", func(t *testing.T) {
		var out uint32
		require.NoError(t, unmarshalNumberAttribute("42", reflect.ValueOf(&out).Elem()))
		require.Equal(t, uint32(42), out)
	})

	t.Run("float", func(t *testing.T) {
		var out float64
		require.NoError(t, unmarshalNumberAttribute("3.14", reflect.ValueOf(&out).Elem()))
		require.InEpsilon(t, 3.14, out, 0.0001)
	})

	t.Run("invalid number returns error", func(t *testing.T) {
		var out int
		require.Error(t, unmarshalNumberAttribute("not-a-number", reflect.ValueOf(&out).Elem()))
	})
}

func TestUnmarshalNumberSetAndBinaryAttribute_ErrorBranches_COV6(t *testing.T) {
	var out string
	require.Error(t, unmarshalNumberSetAttribute([]string{"1"}, reflect.ValueOf(&out).Elem()))
	require.Error(t, unmarshalBinaryAttribute([]byte("x"), reflect.ValueOf(&out).Elem()))
}

func TestAttributeValueListAndMapToInterface_ErrorPropagation_COV6(t *testing.T) {
	type unsupportedAV struct{ types.AttributeValue }

	_, err := attributeValueListToInterface([]types.AttributeValue{&unsupportedAV{}})
	require.Error(t, err)

	_, err = attributeValueMapToInterface(map[string]types.AttributeValue{"a": &unsupportedAV{}})
	require.Error(t, err)
}
