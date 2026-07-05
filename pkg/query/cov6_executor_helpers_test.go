package query

import (
	"context"
	"errors"
	"reflect"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb/types"
	"github.com/stretchr/testify/require"

	"github.com/theory-cloud/tabletheory/pkg/core"
	theorydbErrors "github.com/theory-cloud/tabletheory/pkg/errors"
)

func TestApplyCompiledQueryReadFields_COV6(t *testing.T) {
	limitValue := int32(5)
	consistentValue := true

	compiled := &core.CompiledQuery{
		IndexName:                 "idx",
		FilterExpression:          "f",
		ProjectionExpression:      "p",
		ExpressionAttributeNames:  map[string]string{"#n": "name"},
		ExpressionAttributeValues: map[string]types.AttributeValue{":v": &types.AttributeValueMemberS{Value: "x"}},
		Limit:                     &limitValue,
		ExclusiveStartKey:         map[string]types.AttributeValue{"pk": &types.AttributeValueMemberS{Value: "p1"}},
		ConsistentRead:            &consistentValue,
	}

	var (
		indexName                 *string
		filterExpression          *string
		projectionExpression      *string
		expressionAttributeNames  map[string]string
		expressionAttributeValues map[string]types.AttributeValue
		limit                     *int32
		exclusiveStartKey         map[string]types.AttributeValue
		consistentRead            *bool
	)

	applyCompiledQueryReadFields(
		compiled,
		&indexName,
		&filterExpression,
		&projectionExpression,
		&expressionAttributeNames,
		&expressionAttributeValues,
		&limit,
		&exclusiveStartKey,
		&consistentRead,
	)

	require.NotNil(t, indexName)
	require.Equal(t, "idx", *indexName)
	require.NotNil(t, filterExpression)
	require.Equal(t, "f", *filterExpression)
	require.NotNil(t, projectionExpression)
	require.Equal(t, "p", *projectionExpression)
	require.NotEmpty(t, expressionAttributeNames)
	require.NotEmpty(t, expressionAttributeValues)
	require.NotNil(t, limit)
	require.Equal(t, int32(5), *limit)
	require.NotEmpty(t, exclusiveStartKey)
	require.NotNil(t, consistentRead)
	require.True(t, *consistentRead)
}

func TestBuildDynamoInputs_AssignOptionalFields_COV6(t *testing.T) {
	segment := int32(1)
	totalSegments := int32(4)
	scanForward := false

	compiled := &core.CompiledQuery{
		TableName:              "tbl",
		KeyConditionExpression: "pk = :pk",
		ScanIndexForward:       &scanForward,
		Segment:                &segment,
		TotalSegments:          &totalSegments,
	}

	queryInput := buildDynamoQueryInput(compiled)
	require.NotNil(t, queryInput.KeyConditionExpression)
	require.Equal(t, "pk = :pk", *queryInput.KeyConditionExpression)
	require.NotNil(t, queryInput.ScanIndexForward)
	require.False(t, *queryInput.ScanIndexForward)

	scanInput := buildDynamoScanInput(compiled)
	require.NotNil(t, scanInput.Segment)
	require.Equal(t, int32(1), *scanInput.Segment)
	require.NotNil(t, scanInput.TotalSegments)
	require.Equal(t, int32(4), *scanInput.TotalSegments)
}

func TestCalculateBatchRetryDelay_Basics_COV6(t *testing.T) {
	require.Zero(t, calculateBatchRetryDelay(nil, 0))

	policy := &core.RetryPolicy{
		InitialDelay:  0,
		BackoffFactor: 2,
		MaxDelay:      125 * time.Millisecond,
		Jitter:        0,
		MaxRetries:    3,
	}

	delay0 := calculateBatchRetryDelay(policy, 0)
	require.Equal(t, 50*time.Millisecond, delay0)

	delay2 := calculateBatchRetryDelay(policy, 2)
	require.Equal(t, 125*time.Millisecond, delay2, "clamps to max delay")
}

func TestCalculateBatchRetryDelay_JitterRange_COV6(t *testing.T) {
	base := 100 * time.Millisecond
	policy := &core.RetryPolicy{
		InitialDelay:  base,
		BackoffFactor: 1,
		MaxDelay:      0,
		Jitter:        0.5,
		MaxRetries:    1,
	}

	delay := calculateBatchRetryDelay(policy, 0)
	require.GreaterOrEqual(t, delay, time.Duration(float64(base)*0.5))
	require.LessOrEqual(t, delay, time.Duration(float64(base)*1.5))
}

func TestCompiledConditionReferencesVersion_TokenBoundaries_THE2551(t *testing.T) {
	tests := []struct {
		name        string
		compiled    *core.CompiledQuery
		versionAttr string
		want        bool
	}{
		{
			name:        "nil query",
			compiled:    nil,
			versionAttr: "version",
			want:        false,
		},
		{
			name:        "bare default version token",
			compiled:    &core.CompiledQuery{ConditionExpression: "attribute_exists(version) AND version >= :v"},
			versionAttr: "",
			want:        true,
		},
		{
			name: "placeholder maps to version attribute",
			compiled: &core.CompiledQuery{
				ConditionExpression:      "#version = :expected",
				ExpressionAttributeNames: map[string]string{"#version": "recordVersion"},
			},
			versionAttr: "recordVersion",
			want:        true,
		},
		{
			name:        "substring inside longer identifier is not a version token",
			compiled:    &core.CompiledQuery{ConditionExpression: "preview_version = :v OR versioned = :next"},
			versionAttr: "version",
			want:        false,
		},
		{
			name:        "case insensitive token match",
			compiled:    &core.CompiledQuery{ConditionExpression: "Version BETWEEN :lo AND :hi"},
			versionAttr: "version",
			want:        true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			require.Equal(t, tt.want, compiledConditionReferencesVersion(tt.compiled, tt.versionAttr))
		})
	}
}

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

func TestBuildKeysAndAttributes_COV6(t *testing.T) {
	keys := []map[string]types.AttributeValue{
		{"PK": &types.AttributeValueMemberS{Value: "USER#1"}},
	}

	kaa := buildKeysAndAttributes(&CompiledBatchGet{
		TableName:                "tbl",
		Keys:                     keys,
		ProjectionExpression:     "#pk, #sk",
		ExpressionAttributeNames: map[string]string{"#pk": "PK", "#sk": "SK"},
		ConsistentRead:           true,
	})

	require.Equal(t, keys, kaa.Keys)
	require.NotNil(t, kaa.ProjectionExpression)
	require.Equal(t, "#pk, #sk", *kaa.ProjectionExpression)
	require.Equal(t, map[string]string{"#pk": "PK", "#sk": "SK"}, kaa.ExpressionAttributeNames)
	require.NotNil(t, kaa.ConsistentRead)
	require.True(t, *kaa.ConsistentRead)

	kaa = buildKeysAndAttributes(&CompiledBatchGet{
		TableName: "tbl",
		Keys:      keys,
	})
	require.Nil(t, kaa.ProjectionExpression)
	require.Nil(t, kaa.ExpressionAttributeNames)
	require.Nil(t, kaa.ConsistentRead)
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

func TestIsConditionalCheckFailed_COV6(t *testing.T) {
	require.False(t, isConditionalCheckFailed(nil))
	require.True(t, isConditionalCheckFailed(&types.ConditionalCheckFailedException{}))
	require.False(t, isConditionalCheckFailed(errors.New("prefix ConditionalCheckFailed suffix")))
	require.False(t, isConditionalCheckFailed(errors.New("something else")))
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

func TestQueryAndScanPager_Fetch_ErrorBranches_COV6(t *testing.T) {
	ctx := context.Background()

	mockClient := &MockDynamoDBClient{
		QueryFunc: func(context.Context, *dynamodb.QueryInput, ...func(*dynamodb.Options)) (*dynamodb.QueryOutput, error) {
			return nil, errors.New("query failed")
		},
		ScanFunc: func(context.Context, *dynamodb.ScanInput, ...func(*dynamodb.Options)) (*dynamodb.ScanOutput, error) {
			return nil, errors.New("scan failed")
		},
	}

	qp := queryPager{client: mockClient, ctx: ctx, input: &dynamodb.QueryInput{TableName: aws.String("tbl")}}
	_, _, err := qp.fetch(map[string]types.AttributeValue{"pk": &types.AttributeValueMemberS{Value: "p1"}})
	require.Error(t, err)

	sp := scanPager{client: mockClient, ctx: ctx, input: &dynamodb.ScanInput{TableName: aws.String("tbl")}}
	_, _, err = sp.fetch(nil)
	require.Error(t, err)
}

func TestMainExecutor_ExecuteQueryAndScan_NilInput_COV6(t *testing.T) {
	executor := NewExecutor(&MockDynamoDBClient{}, context.Background())

	require.Error(t, executor.ExecuteQuery(nil, &[]map[string]types.AttributeValue{}))
	require.Error(t, executor.ExecuteScan(nil, &[]map[string]types.AttributeValue{}))
}
