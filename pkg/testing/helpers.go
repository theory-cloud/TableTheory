package testing

import (
	"github.com/stretchr/testify/mock"

	"github.com/theory-cloud/tabletheory/pkg/core"
	"github.com/theory-cloud/tabletheory/pkg/errors"
	"github.com/theory-cloud/tabletheory/pkg/mocks"
)

// TestDB provides a fluent interface for setting up mock expectations
type TestDB struct {
	MockDB    *mocks.MockDB
	MockQuery *mocks.MockQuery
}

// NewTestDB creates a new test database with common setup
func NewTestDB() *TestDB {
	mockDB := new(mocks.MockDB)
	mockQuery := new(mocks.MockQuery)

	// Common setup that most tests will need
	mockDB.On("Model", mock.Anything).Return(mockQuery).Maybe()
	mockDB.On("WithContext", mock.Anything).Return(mockDB).Maybe()

	return &TestDB{
		MockDB:    mockDB,
		MockQuery: mockQuery,
	}
}

// ExpectModel sets up expectations for Model calls
func (t *TestDB) ExpectModel(model interface{}) *TestDB {
	t.MockDB.On("Model", model).Return(t.MockQuery).Once()
	return t
}

// ExpectCreate sets up expectations for a create operation
func (t *TestDB) ExpectCreate() *TestDB {
	t.MockQuery.On("Create").Return(nil).Once()
	return t
}

// ExpectCreateError sets up expectations for a failed create
func (t *TestDB) ExpectCreateError(err error) *TestDB {
	t.MockQuery.On("Create").Return(err).Once()
	return t
}

// ExpectFind sets up expectations for finding a record
func (t *TestDB) ExpectFind(result interface{}) *TestDB {
	t.MockQuery.On("First", mock.AnythingOfType(getTypeString(result))).
		Run(func(args mock.Arguments) {
			dest := args.Get(0)
			copyValue(dest, result)
		}).Return(nil).Once()
	return t
}

// ExpectNotFound sets up expectations for a not found error
func (t *TestDB) ExpectNotFound() *TestDB {
	t.MockQuery.On("First", mock.Anything).
		Return(errors.ErrItemNotFound).Once()
	return t
}

// ExpectWhere sets up expectations for where conditions
func (t *TestDB) ExpectWhere(field string, op string, value interface{}) *TestDB {
	t.MockQuery.On("Where", field, op, value).Return(t.MockQuery).Once()
	return t
}

// ExpectUpdate sets up expectations for update operations
func (t *TestDB) ExpectUpdate(fields ...string) *TestDB {
	t.MockQuery.On("Update", fields).Return(nil).Once()
	return t
}

// ExpectUpdateError sets up expectations for failed update
func (t *TestDB) ExpectUpdateError(err error, fields ...string) *TestDB {
	t.MockQuery.On("Update", fields).Return(err).Once()
	return t
}

// ExpectDelete sets up expectations for delete operations
func (t *TestDB) ExpectDelete() *TestDB {
	t.MockQuery.On("Delete").Return(nil).Once()
	return t
}

// ExpectDeleteError sets up expectations for failed delete
func (t *TestDB) ExpectDeleteError(err error) *TestDB {
	t.MockQuery.On("Delete").Return(err).Once()
	return t
}

// ExpectAll sets up expectations for retrieving all records
func (t *TestDB) ExpectAll(results interface{}) *TestDB {
	t.MockQuery.On("All", mock.AnythingOfType(getTypeString(results))).
		Run(func(args mock.Arguments) {
			dest := args.Get(0)
			copyValue(dest, results)
		}).Return(nil).Once()
	return t
}

// ExpectCount sets up expectations for count operations
func (t *TestDB) ExpectCount(count int64) *TestDB {
	t.MockQuery.On("Count").Return(count, nil).Once()
	return t
}

// ExpectTransaction sets up transaction expectations
func (t *TestDB) ExpectTransaction(setupFunc func(tx *core.Tx)) *TestDB {
	t.MockDB.On("Transaction", mock.AnythingOfType("func(*core.Tx) error")).
		Run(func(args mock.Arguments) {
			txFn, ok := args.Get(0).(func(*core.Tx) error)
			if !ok {
				panic("unexpected Transaction callback type")
			}

			// Create a mock transaction
			mockTx := &core.Tx{}

			// Let the test setup the transaction expectations
			if setupFunc != nil {
				setupFunc(mockTx)
			}

			// Execute the transaction function
			if err := txFn(mockTx); err != nil {
				panic(err)
			}
		}).Return(nil).Once()
	return t
}

// ExpectTransactionError sets up expectations for a failed transaction
func (t *TestDB) ExpectTransactionError(err error) *TestDB {
	t.MockDB.On("Transaction", mock.AnythingOfType("func(*core.Tx) error")).
		Return(err).Once()
	return t
}

// ExpectLimit sets up expectations for limit operations
func (t *TestDB) ExpectLimit(limit int) *TestDB {
	t.MockQuery.On("Limit", limit).Return(t.MockQuery).Once()
	return t
}

// ExpectOffset sets up expectations for offset operations
func (t *TestDB) ExpectOffset(offset int) *TestDB {
	t.MockQuery.On("Offset", offset).Return(t.MockQuery).Once()
	return t
}

// ExpectOrderBy sets up expectations for ordering
func (t *TestDB) ExpectOrderBy(field string, order string) *TestDB {
	t.MockQuery.On("OrderBy", field, order).Return(t.MockQuery).Once()
	return t
}

// ExpectIndex sets up expectations for index usage
func (t *TestDB) ExpectIndex(indexName string) *TestDB {
	t.MockQuery.On("Index", indexName).Return(t.MockQuery).Once()
	return t
}

// ExpectBatchGet sets up expectations for batch get operations
func (t *TestDB) ExpectBatchGet(keys []interface{}, results interface{}) *TestDB {
	t.MockQuery.On("BatchGet", keys, mock.AnythingOfType(getTypeString(results))).
		Run(func(args mock.Arguments) {
			dest := args.Get(1)
			copyValue(dest, results)
		}).Return(nil).Once()
	return t
}

// ExpectBatchCreate sets up expectations for batch create operations
func (t *TestDB) ExpectBatchCreate(items interface{}) *TestDB {
	t.MockQuery.On("BatchCreate", items).Return(nil).Once()
	return t
}

// ExpectBatchDelete sets up expectations for batch delete operations
func (t *TestDB) ExpectBatchDelete(keys []interface{}) *TestDB {
	t.MockQuery.On("BatchDelete", keys).Return(nil).Once()
	return t
}

// AssertExpectations asserts that all expectations were met
func (t *TestDB) AssertExpectations(testing mock.TestingT) {
	t.MockDB.AssertExpectations(testing)
	t.MockQuery.AssertExpectations(testing)
}

// Reset clears all expectations
func (t *TestDB) Reset() {
	t.MockDB.ExpectedCalls = nil
	t.MockQuery.ExpectedCalls = nil
}

// QueryChain helps build complex query expectations
type QueryChain struct {
	testDB *TestDB
	calls  []mockCall
}

type mockCall struct {
	method string
	args   []interface{}
}

// NewQueryChain creates a new query chain builder
func (t *TestDB) NewQueryChain() *QueryChain {
	return &QueryChain{
		testDB: t,
		calls:  make([]mockCall, 0),
	}
}

// Where adds a where condition to the chain
func (q *QueryChain) Where(field string, op string, value interface{}) *QueryChain {
	q.calls = append(q.calls, mockCall{"Where", []interface{}{field, op, value}})
	return q
}

// Limit adds a limit to the chain
func (q *QueryChain) Limit(limit int) *QueryChain {
	q.calls = append(q.calls, mockCall{"Limit", []interface{}{limit}})
	return q
}

// OrderBy adds ordering to the chain
func (q *QueryChain) OrderBy(field string, order string) *QueryChain {
	q.calls = append(q.calls, mockCall{"OrderBy", []interface{}{field, order}})
	return q
}

// ExpectAll finalizes the chain with an All expectation
func (q *QueryChain) ExpectAll(results interface{}) *TestDB {
	// Set up all the chained calls
	for _, call := range q.calls {
		q.testDB.MockQuery.On(call.method, call.args...).Return(q.testDB.MockQuery).Once()
	}

	// Set up the final All call
	q.testDB.ExpectAll(results)

	return q.testDB
}

// ExpectFirst finalizes the chain with a First expectation
func (q *QueryChain) ExpectFirst(result interface{}) *TestDB {
	// Set up all the chained calls
	for _, call := range q.calls {
		q.testDB.MockQuery.On(call.method, call.args...).Return(q.testDB.MockQuery).Once()
	}

	// Set up the final First call
	q.testDB.ExpectFind(result)

	return q.testDB
}
