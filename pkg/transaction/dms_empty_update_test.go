package transaction

import (
	"testing"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb/types"
	"github.com/stretchr/testify/require"

	"github.com/theory-cloud/tabletheory/v3/pkg/core"
	"github.com/theory-cloud/tabletheory/v3/pkg/model"
	"github.com/theory-cloud/tabletheory/v3/pkg/session"
	pkgTypes "github.com/theory-cloud/tabletheory/v3/pkg/types"
)

type transactionalExplicitEmptyRecord struct {
	PK       string `theorydb:"pk,attr:PK" json:"PK"`
	SK       string `theorydb:"sk,attr:SK" json:"SK"`
	Nickname string `theorydb:"attr:nickname,omitempty" json:"nickname,omitempty"`
}

func (transactionalExplicitEmptyRecord) TableName() string {
	return "transactional_explicit_empty_records"
}

func TestBuilderUpdateExplicitEmptyOmitEmptyRemovesAttribute(t *testing.T) {
	registry := model.NewRegistry()
	require.NoError(t, registry.Register(&transactionalExplicitEmptyRecord{}))

	builder := NewBuilder(&session.Session{}, registry, pkgTypes.NewConverter())
	builder.Update(
		&transactionalExplicitEmptyRecord{
			PK: "USER#explicit-empty",
			SK: "PROFILE",
		},
		[]string{"Nickname"},
	)

	items, err := builder.materializeOperations()
	require.NoError(t, err)
	require.Len(t, items, 1)
	require.NotNil(t, items[0].Update)

	update := items[0].Update
	require.Contains(t, aws.ToString(update.UpdateExpression), "REMOVE")
	require.NotContains(t, aws.ToString(update.UpdateExpression), "SET")
	require.Contains(t, attributeNames(update.ExpressionAttributeNames), "nickname")
	require.Empty(t, update.ExpressionAttributeValues)
}

type sparseTransactionAddress struct {
	Line string `theorydb:"attr:line" json:"line"`
}

type sparseTransactionRecord struct {
	PK      string                   `theorydb:"pk,attr:PK" json:"PK"`
	SK      string                   `theorydb:"sk,attr:SK" json:"SK"`
	Status  string                   `theorydb:"attr:status,omitempty" json:"status,omitempty"`
	Address sparseTransactionAddress `theorydb:"attr:address,omitempty" json:"address,omitempty"`
	Values  [2]int                   `theorydb:"attr:values,omitempty" json:"values,omitempty"`
}

func (sparseTransactionRecord) TableName() string {
	return "sparse_transaction_records"
}

func TestTransactionUpdateSkipsZeroStructuredOmitEmptyFields(t *testing.T) {
	registry := model.NewRegistry()
	require.NoError(t, registry.Register(&sparseTransactionRecord{}))

	tx := NewTransaction(&session.Session{}, registry, pkgTypes.NewConverter())
	require.NoError(t, tx.Update(&sparseTransactionRecord{
		PK:     "ACCOUNT#sparse",
		SK:     "PROFILE",
		Status: "active",
	}))
	require.Len(t, tx.writes, 1)
	require.NotNil(t, tx.writes[0].Update)

	update := tx.writes[0].Update
	updatedAttributes := attributeNames(update.ExpressionAttributeNames)
	require.Contains(t, updatedAttributes, "status")
	require.NotContains(t, updatedAttributes, "address",
		"skipping the zero address preserves the persisted address")
	require.NotContains(t, updatedAttributes, "values",
		"skipping the zero fixed array preserves the persisted values")
	require.Len(t, update.ExpressionAttributeValues, 1)
}

func attributeNames(names map[string]string) []string {
	attributes := make([]string, 0, len(names))
	for _, attribute := range names {
		attributes = append(attributes, attribute)
	}
	return attributes
}

type fixedArrayTransactGetRecord struct {
	PK   string   `theorydb:"pk,attr:PK" json:"PK"`
	Hash [32]byte `theorydb:"attr:hash" json:"hash"`
}

func (fixedArrayTransactGetRecord) TableName() string {
	return "fixed_array_transact_get_records"
}

func TestCollectTransactGetResultsRoundTripsFixedArray(t *testing.T) {
	registry := model.NewRegistry()
	require.NoError(t, registry.Register(&fixedArrayTransactGetRecord{}))
	metadata, err := registry.GetMetadata(&fixedArrayTransactGetRecord{})
	require.NoError(t, err)

	hash := [32]byte{0, 1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15,
		16, 17, 18, 19, 20, 21, 22, 23, 24, 25, 26, 27, 28, 29, 30, 31}
	converter := pkgTypes.NewConverter()
	hashValue, err := converter.ToAttributeValue(hash)
	require.NoError(t, err)
	require.IsType(t, &types.AttributeValueMemberL{}, hashValue)

	var out fixedArrayTransactGetRecord
	results, err := collectTransactGetResults(
		t.Context(),
		&session.Session{},
		[]core.TransactGetRequest{{Model: &fixedArrayTransactGetRecord{}, Dest: &out}},
		[]*model.Metadata{metadata},
		[]types.ItemResponse{{Item: map[string]types.AttributeValue{
			"PK":   &types.AttributeValueMemberS{Value: "HASH#transact-get"},
			"hash": hashValue,
		}}},
	)
	require.NoError(t, err)
	require.Len(t, results, 1)
	require.True(t, results[0].Found)
	require.Equal(t, "HASH#transact-get", out.PK)
	require.Equal(t, hash, out.Hash)
}
