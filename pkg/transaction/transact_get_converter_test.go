package transaction

import (
	"errors"
	"reflect"
	"strings"
	"testing"

	"github.com/aws/aws-sdk-go-v2/service/dynamodb/types"
	"github.com/stretchr/testify/require"

	"github.com/theory-cloud/tabletheory/v4/pkg/core"
	"github.com/theory-cloud/tabletheory/v4/pkg/model"
	"github.com/theory-cloud/tabletheory/v4/pkg/session"
	pkgTypes "github.com/theory-cloud/tabletheory/v4/pkg/types"
)

type validatedID string

type uppercaseIDConverter struct {
	calls *int
	err   error
}

func (c uppercaseIDConverter) ToAttributeValue(value any) (types.AttributeValue, error) {
	if c.err != nil {
		return nil, c.err
	}
	id, ok := value.(validatedID)
	if !ok {
		return nil, errors.New("unexpected converter input")
	}
	return &types.AttributeValueMemberS{Value: string(id)}, nil
}

func (c uppercaseIDConverter) FromAttributeValue(av types.AttributeValue, target any) error {
	if c.calls != nil {
		*c.calls++
	}
	if c.err != nil {
		return c.err
	}
	s, ok := av.(*types.AttributeValueMemberS)
	if !ok {
		return errors.New("expected string attribute value")
	}
	targetPtr, ok := target.(*validatedID)
	if !ok {
		return errors.New("unexpected converter target")
	}
	*targetPtr = validatedID(strings.ToUpper(s.Value))
	return nil
}

type convertedRecord struct {
	PK string      `theorydb:"pk" json:"PK"`
	ID validatedID `json:"id"`
}

func registerConvertedRecord(t *testing.T) *model.Metadata {
	t.Helper()
	registry := model.NewRegistry()
	require.NoError(t, registry.Register(&convertedRecord{}))
	meta, err := registry.GetMetadata(&convertedRecord{})
	require.NoError(t, err)
	return meta
}

func TestCollectTransactGetResultsInvokesRegisteredConverter(t *testing.T) {
	meta := registerConvertedRecord(t)

	converter := pkgTypes.NewConverter()
	calls := 0
	converter.RegisterConverter(reflect.TypeOf(validatedID("")), uppercaseIDConverter{calls: &calls})

	var out convertedRecord
	results, err := collectTransactGetResults(
		t.Context(),
		&session.Session{},
		converter,
		[]core.TransactGetRequest{{Model: &convertedRecord{}, Dest: &out}},
		[]*model.Metadata{meta},
		[]types.ItemResponse{{Item: map[string]types.AttributeValue{
			meta.Fields["PK"].DBName: &types.AttributeValueMemberS{Value: "PK#1"},
			meta.Fields["ID"].DBName: &types.AttributeValueMemberS{Value: "abc"},
		}}},
	)
	require.NoError(t, err)
	require.Len(t, results, 1)
	require.True(t, results[0].Found)

	require.Equal(t, 1, calls, "the registered converter must run on the TransactGet response")
	require.Equal(t, validatedID("ABC"), out.ID)
	require.Equal(t, "PK#1", out.PK)
}

func TestCollectTransactGetResultsPropagatesConverterError(t *testing.T) {
	meta := registerConvertedRecord(t)

	converter := pkgTypes.NewConverter()
	want := errors.New("rejected value")
	converter.RegisterConverter(reflect.TypeOf(validatedID("")), uppercaseIDConverter{err: want})

	var out convertedRecord
	_, err := collectTransactGetResults(
		t.Context(),
		&session.Session{},
		converter,
		[]core.TransactGetRequest{{Model: &convertedRecord{}, Dest: &out}},
		[]*model.Metadata{meta},
		[]types.ItemResponse{{Item: map[string]types.AttributeValue{
			meta.Fields["PK"].DBName: &types.AttributeValueMemberS{Value: "PK#1"},
			meta.Fields["ID"].DBName: &types.AttributeValueMemberS{Value: "abc"},
		}}},
	)
	require.ErrorIs(t, err, want)
}
