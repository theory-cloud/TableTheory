package query

import (
	"errors"
	"strings"
	"sync"
	"testing"

	"github.com/aws/aws-sdk-go-v2/service/dynamodb/types"
	"github.com/stretchr/testify/require"

	"github.com/theory-cloud/tabletheory/v3/pkg/core"
	theorydbErrors "github.com/theory-cloud/tabletheory/v3/pkg/errors"
	"github.com/theory-cloud/tabletheory/v3/pkg/model"
)

type ordinaryCreateItem struct {
	PK      string `json:"pk" theorydb:"pk,attr:pk"`
	Payload string `json:"payload" theorydb:"attr:payload"`
}

func (ordinaryCreateItem) TableName() string { return "ordinary_create_items" }

type conditionalCreateExecutor struct {
	mu      sync.Mutex
	exists  bool
	payload types.AttributeValue
}

func (e *conditionalCreateExecutor) ExecuteQuery(*core.CompiledQuery, any) error { return nil }
func (e *conditionalCreateExecutor) ExecuteScan(*core.CompiledQuery, any) error  { return nil }
func (e *conditionalCreateExecutor) ExecuteUpdateItem(*core.CompiledQuery, map[string]types.AttributeValue) error {
	return nil
}
func (e *conditionalCreateExecutor) ExecuteDeleteItem(*core.CompiledQuery, map[string]types.AttributeValue) error {
	return nil
}
func (e *conditionalCreateExecutor) ExecuteBatchWriteItem(string, []types.WriteRequest) (*core.BatchWriteResult, error) {
	return &core.BatchWriteResult{}, nil
}

func (e *conditionalCreateExecutor) ExecutePutItem(input *core.CompiledQuery, item map[string]types.AttributeValue) error {
	e.mu.Lock()
	defer e.mu.Unlock()

	if e.exists && strings.Contains(input.ConditionExpression, "attribute_not_exists") {
		return theorydbErrors.ErrConditionFailed
	}
	e.exists = true
	e.payload = item["payload"]
	return nil
}

func newOrdinaryCreateQuery(t *testing.T, item *ordinaryCreateItem, executor *conditionalCreateExecutor) *Query {
	t.Helper()
	registry := model.NewRegistry()
	require.NoError(t, registry.Register(item))
	metadata, err := registry.GetMetadata(item)
	require.NoError(t, err)
	return New(item, writePolicyQueryMetadata{meta: metadata}, executor)
}

func TestCreateConcurrentCallerChosenKeyAllowsExactlyOneWriter(t *testing.T) {
	executor := &conditionalCreateExecutor{}
	items := []*ordinaryCreateItem{
		{PK: "account#caller-chosen", Payload: "first"},
		{PK: "account#caller-chosen", Payload: "second"},
	}

	start := make(chan struct{})
	results := make(chan error, len(items))
	var ready sync.WaitGroup
	ready.Add(len(items))
	for _, item := range items {
		query := newOrdinaryCreateQuery(t, item, executor)
		go func() {
			ready.Done()
			<-start
			results <- query.Create()
		}()
	}
	ready.Wait()
	close(start)

	var successes int
	var conditionFailures int
	for range items {
		err := <-results
		switch {
		case err == nil:
			successes++
		case errors.Is(err, theorydbErrors.ErrConditionFailed):
			conditionFailures++
		default:
			require.NoError(t, err)
		}
	}

	require.Equal(t, 1, successes)
	require.Equal(t, 1, conditionFailures)
	require.NotNil(t, executor.payload)
}

func TestCreateOrUpdateRemainsIntentionalOverwriteOptOut(t *testing.T) {
	executor := &conditionalCreateExecutor{}

	first := newOrdinaryCreateQuery(t, &ordinaryCreateItem{PK: "account#1", Payload: "first"}, executor)
	require.NoError(t, first.CreateOrUpdate())

	second := newOrdinaryCreateQuery(t, &ordinaryCreateItem{PK: "account#1", Payload: "second"}, executor)
	require.NoError(t, second.CreateOrUpdate())
}
