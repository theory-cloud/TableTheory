package query

import (
	"fmt"
	"testing"

	"github.com/aws/aws-sdk-go-v2/service/dynamodb/types"
	"github.com/stretchr/testify/require"

	"github.com/theory-cloud/tabletheory/v3/pkg/model"
)

type uintVersionQueryRecord struct {
	PK      string `theorydb:"pk,attr:PK" json:"PK"`
	SK      string `theorydb:"sk,attr:SK" json:"SK"`
	Value   string `theorydb:"attr:value" json:"value"`
	Version uint64 `theorydb:"version,attr:version" json:"version"`
}

func (uintVersionQueryRecord) TableName() string {
	return "uint_version_query_records"
}

func TestQueryUpdateSupportsUint64Versions(t *testing.T) {
	registry := model.NewRegistry()
	require.NoError(t, registry.Register(&uintVersionQueryRecord{}))
	metadata, err := registry.GetMetadata(&uintVersionQueryRecord{})
	require.NoError(t, err)

	for _, test := range []struct {
		name    string
		version uint64
	}{
		{name: "zero", version: 0},
		{name: "non-zero", version: 7},
	} {
		t.Run(test.name, func(t *testing.T) {
			executor := &writePolicyRecordingExecutor{}
			q := New(&uintVersionQueryRecord{
				PK:      "USER#uint-version",
				SK:      "PROFILE",
				Value:   "changed",
				Version: test.version,
			}, writePolicyQueryMetadata{meta: metadata}, executor)

			require.NoError(t, q.Update("Value"))
			require.NotNil(t, executor.compiled)
			require.Contains(t, executor.compiled.ConditionExpression, versionNamePlaceholder(t, executor.compiled.ExpressionAttributeNames))
			require.Contains(t, executor.compiled.UpdateExpression, versionNamePlaceholder(t, executor.compiled.ExpressionAttributeNames))
			require.ElementsMatch(t, []string{fmt.Sprint(test.version), "1"}, numberAttributeValues(executor.compiled.ExpressionAttributeValues))
		})
	}
}

func TestQueryDeleteSupportsUint64Version(t *testing.T) {
	registry := model.NewRegistry()
	require.NoError(t, registry.Register(&uintVersionQueryRecord{}))
	metadata, err := registry.GetMetadata(&uintVersionQueryRecord{})
	require.NoError(t, err)

	executor := &writePolicyRecordingExecutor{}
	q := New(&uintVersionQueryRecord{
		PK:      "USER#uint-version-delete",
		SK:      "PROFILE",
		Version: 7,
	}, writePolicyQueryMetadata{meta: metadata}, executor)

	require.NoError(t, q.Delete())
	require.NotNil(t, executor.compiled)
	require.Contains(t, executor.compiled.ConditionExpression, versionNamePlaceholder(t, executor.compiled.ExpressionAttributeNames))
	require.Equal(t, []string{"7"}, numberAttributeValues(executor.compiled.ExpressionAttributeValues))
}

func versionNamePlaceholder(t *testing.T, names map[string]string) string {
	t.Helper()
	for placeholder, name := range names {
		if name == "version" {
			return placeholder
		}
	}
	require.FailNow(t, "version expression attribute name was not compiled")
	return ""
}

func numberAttributeValues(values map[string]types.AttributeValue) []string {
	numbers := make([]string, 0, len(values))
	for _, value := range values {
		if number, ok := value.(*types.AttributeValueMemberN); ok {
			numbers = append(numbers, number.Value)
		}
	}
	return numbers
}
