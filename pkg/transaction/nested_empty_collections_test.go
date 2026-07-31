package transaction

import (
	"testing"

	"github.com/aws/aws-sdk-go-v2/service/dynamodb/types"
	"github.com/stretchr/testify/require"

	"github.com/theory-cloud/tabletheory/v3/pkg/model"
	pkgTypes "github.com/theory-cloud/tabletheory/v3/pkg/types"
)

type nestedCollectionPayload struct {
	EmptyMap    map[string]string `theorydb:"attr:emptyMap" json:"emptyMap"`
	OmittedMap  map[string]string `theorydb:"attr:omittedMap,omitempty" json:"omittedMap,omitempty"`
	EmptyList   []string          `theorydb:"attr:emptyList" json:"emptyList"`
	OmittedList []string          `theorydb:"attr:omittedList,omitempty" json:"omittedList,omitempty"`
}

type nestedCollectionRecord struct {
	PK      string                  `theorydb:"pk,attr:PK" json:"PK"`
	SK      string                  `theorydb:"sk,attr:SK" json:"SK"`
	Payload nestedCollectionPayload `theorydb:"attr:payload" json:"payload"`
}

type topLevelEmptyCollectionRecord struct {
	PK        string            `theorydb:"pk,attr:PK" json:"PK"`
	SK        string            `theorydb:"sk,attr:SK" json:"SK"`
	EmptyMap  map[string]string `theorydb:"attr:emptyMap,omitempty" json:"emptyMap,omitempty"`
	EmptyList []string          `theorydb:"attr:emptyList,omitempty" json:"emptyList,omitempty"`
}

func TestTransactionCreatePreservesNestedEmptyCollectionsWithoutOmitEmpty(t *testing.T) {
	registry := model.NewRegistry()
	require.NoError(t, registry.Register(&nestedCollectionRecord{}))

	builder := NewBuilder(nil, registry, pkgTypes.NewConverter())
	mockClient := newMockTransactClient(t, nil)
	builder.client = mockClient

	record := &nestedCollectionRecord{
		PK: "RECORD#empty-collections",
		SK: "PAYLOAD",
		Payload: nestedCollectionPayload{
			EmptyList:   []string{},
			EmptyMap:    map[string]string{},
			OmittedList: []string{},
			OmittedMap:  map[string]string{},
		},
	}

	require.NoError(t, builder.Create(record).Execute())
	require.Len(t, mockClient.inputs, 1)
	require.Len(t, mockClient.inputs[0].TransactItems, 1)

	put := mockClient.inputs[0].TransactItems[0].Put
	require.NotNil(t, put)

	payload, ok := put.Item["payload"].(*types.AttributeValueMemberM)
	require.True(t, ok, "payload should be a DynamoDB map")

	emptyList, ok := payload.Value["emptyList"].(*types.AttributeValueMemberL)
	require.True(t, ok, "non-omitempty empty list should be present")
	require.Empty(t, emptyList.Value)

	emptyMap, ok := payload.Value["emptyMap"].(*types.AttributeValueMemberM)
	require.True(t, ok, "non-omitempty empty map should be present")
	require.Empty(t, emptyMap.Value)

	require.NotContains(t, payload.Value, "omittedList")
	require.NotContains(t, payload.Value, "omittedMap")
}

func TestTransactionCreateOmitsTopLevelEmptyCollections(t *testing.T) {
	registry := model.NewRegistry()
	require.NoError(t, registry.Register(&topLevelEmptyCollectionRecord{}))

	builder := NewBuilder(nil, registry, pkgTypes.NewConverter())
	mockClient := newMockTransactClient(t, nil)
	builder.client = mockClient

	record := &topLevelEmptyCollectionRecord{
		PK:        "RECORD#top-level-empty-collections",
		SK:        "PAYLOAD",
		EmptyList: []string{},
		EmptyMap:  map[string]string{},
	}

	require.NoError(t, builder.Create(record).Execute())
	put := mockClient.inputs[0].TransactItems[0].Put
	require.NotNil(t, put)
	require.NotContains(t, put.Item, "emptyList")
	require.NotContains(t, put.Item, "emptyMap")
}
