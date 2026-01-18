package testing

import (
	"errors"

	"github.com/stretchr/testify/mock"

	"github.com/theory-cloud/tabletheory/pkg/core"
)

func mustUpdateBuilder(v any) core.UpdateBuilder {
	if v == nil {
		return nil
	}
	builder, ok := v.(core.UpdateBuilder)
	if !ok {
		panic("unexpected type: expected core.UpdateBuilder")
	}
	return builder
}

// CommonScenarios provides pre-built test scenarios
type CommonScenarios struct {
	db *TestDB
}

// NewCommonScenarios creates common test scenarios
func NewCommonScenarios(db *TestDB) *CommonScenarios {
	return &CommonScenarios{db: db}
}

// SetupCRUD sets up expectations for basic CRUD operations
func (s *CommonScenarios) SetupCRUD(model interface{}) {
	// Allow any number of Model calls
	s.db.MockDB.On("Model", mock.AnythingOfType(getTypeString(model))).
		Return(s.db.MockQuery).Maybe()

	// Setup common CRUD operations
	s.db.MockQuery.On("Create").Return(nil).Maybe()
	s.db.MockQuery.On("First", mock.AnythingOfType(getTypeString(model))).Return(nil).Maybe()
	s.db.MockQuery.On("Update", mock.Anything).Return(nil).Maybe()
	s.db.MockQuery.On("Delete").Return(nil).Maybe()
}

// SetupPagination sets up expectations for paginated queries
func (s *CommonScenarios) SetupPagination(pageSize int) {
	s.db.MockQuery.On("Limit", pageSize).Return(s.db.MockQuery).Maybe()
	s.db.MockQuery.On("Cursor", mock.Anything).Return(s.db.MockQuery).Maybe()
	s.db.MockQuery.On("AllPaginated", mock.Anything).
		Return(&core.PaginatedResult{
			Items:      []interface{}{},
			NextCursor: "next-cursor",
			Count:      pageSize,
			HasMore:    true,
		}, nil).Maybe()
}

// SetupMultiTenant sets up expectations for multi-tenant queries
func (s *CommonScenarios) SetupMultiTenant(tenantID string) {
	// Tenant isolation typically adds a where condition
	s.db.MockQuery.On("Where", "tenant_id", "=", tenantID).
		Return(s.db.MockQuery).Maybe()

	// Or it might use a filter
	s.db.MockQuery.On("Filter", "tenant_id", "=", tenantID).
		Return(s.db.MockQuery).Maybe()
}

// SetupBatchOperations sets up expectations for batch operations
func (s *CommonScenarios) SetupBatchOperations() {
	s.db.MockQuery.On("BatchCreate", mock.Anything).Return(nil).Maybe()
	s.db.MockQuery.On("BatchGet", mock.Anything, mock.Anything).Return(nil).Maybe()
	s.db.MockQuery.On("BatchDelete", mock.Anything).Return(nil).Maybe()
	s.db.MockQuery.On("BatchWrite", mock.Anything, mock.Anything).Return(nil).Maybe()
}

// SetupComplexQuery sets up expectations for complex queries with multiple conditions
func (s *CommonScenarios) SetupComplexQuery() {
	// Allow chaining of query methods
	s.db.MockQuery.On("Where", mock.Anything, mock.Anything, mock.Anything).
		Return(s.db.MockQuery).Maybe()
	s.db.MockQuery.On("Filter", mock.Anything, mock.Anything, mock.Anything).
		Return(s.db.MockQuery).Maybe()
	s.db.MockQuery.On("OrderBy", mock.Anything, mock.Anything).
		Return(s.db.MockQuery).Maybe()
	s.db.MockQuery.On("Limit", mock.Anything).
		Return(s.db.MockQuery).Maybe()
	s.db.MockQuery.On("Index", mock.Anything).
		Return(s.db.MockQuery).Maybe()
}

// SetupErrorScenarios sets up common error scenarios
func (s *CommonScenarios) SetupErrorScenarios(errorMap map[string]error) {
	for operation, err := range errorMap {
		switch operation {
		case "create":
			s.db.MockQuery.On("Create").Return(err).Maybe()
		case "find":
			s.db.MockQuery.On("First", mock.Anything).Return(err).Maybe()
		case "update":
			s.db.MockQuery.On("Update", mock.Anything).Return(err).Maybe()
		case "delete":
			s.db.MockQuery.On("Delete").Return(err).Maybe()
		case "all":
			s.db.MockQuery.On("All", mock.Anything).Return(err).Maybe()
		case "count":
			s.db.MockQuery.On("Count").Return(int64(0), err).Maybe()
		}
	}
}

// SetupTransactionScenario sets up expectations for transactional operations
func (s *CommonScenarios) SetupTransactionScenario(success bool) {
	if success {
		s.db.MockDB.On("Transaction", mock.AnythingOfType("func(*core.Tx) error")).
			Run(func(args mock.Arguments) {
				fn, ok := args.Get(0).(func(*core.Tx) error)
				if !ok {
					panic("unexpected type: expected func(*core.Tx) error")
				}
				// Execute the transaction function with a mock transaction
				if err := fn(&core.Tx{}); err != nil {
					panic(err)
				}
			}).Return(nil).Maybe()
	} else {
		s.db.MockDB.On("Transaction", mock.AnythingOfType("func(*core.Tx) error")).
			Return(errors.New("transaction failed")).Maybe()
	}
}

// SetupScanScenario sets up expectations for scan operations
func (s *CommonScenarios) SetupScanScenario() {
	s.db.MockQuery.On("Scan", mock.Anything).Return(nil).Maybe()
	s.db.MockQuery.On("ParallelScan", mock.Anything, mock.Anything).
		Return(s.db.MockQuery).Maybe()
	s.db.MockQuery.On("ScanAllSegments", mock.Anything, mock.Anything).
		Return(nil).Maybe()
}

// SetupIndexQuery sets up expectations for index-based queries
func (s *CommonScenarios) SetupIndexQuery(indexName string) {
	s.db.MockQuery.On("Index", indexName).Return(s.db.MockQuery).Maybe()
	s.db.MockQuery.On("Where", mock.Anything, mock.Anything, mock.Anything).
		Return(s.db.MockQuery).Maybe()
	s.db.MockQuery.On("All", mock.Anything).Return(nil).Maybe()
}

// SetupUpdateBuilder sets up expectations for complex update operations
func (s *CommonScenarios) SetupUpdateBuilder() {
	mockBuilder := new(MockUpdateBuilder)

	// Setup fluent interface
	mockBuilder.On("Set", mock.Anything, mock.Anything).Return(mockBuilder).Maybe()
	mockBuilder.On("Add", mock.Anything, mock.Anything).Return(mockBuilder).Maybe()
	mockBuilder.On("Remove", mock.Anything).Return(mockBuilder).Maybe()
	mockBuilder.On("Condition", mock.Anything, mock.Anything, mock.Anything).
		Return(mockBuilder).Maybe()
	mockBuilder.On("Execute").Return(nil).Maybe()

	s.db.MockQuery.On("UpdateBuilder").Return(mockBuilder).Maybe()
}

// MockUpdateBuilder is a mock implementation of UpdateBuilder
type MockUpdateBuilder struct {
	mock.Mock
}

func (m *MockUpdateBuilder) Set(field string, value any) core.UpdateBuilder {
	args := m.Called(field, value)
	return mustUpdateBuilder(args.Get(0))
}

func (m *MockUpdateBuilder) SetIfNotExists(field string, value any, defaultValue any) core.UpdateBuilder {
	args := m.Called(field, value, defaultValue)
	return mustUpdateBuilder(args.Get(0))
}

func (m *MockUpdateBuilder) Add(field string, value any) core.UpdateBuilder {
	args := m.Called(field, value)
	return mustUpdateBuilder(args.Get(0))
}

func (m *MockUpdateBuilder) Increment(field string) core.UpdateBuilder {
	args := m.Called(field)
	return mustUpdateBuilder(args.Get(0))
}

func (m *MockUpdateBuilder) Decrement(field string) core.UpdateBuilder {
	args := m.Called(field)
	return mustUpdateBuilder(args.Get(0))
}

func (m *MockUpdateBuilder) Remove(field string) core.UpdateBuilder {
	args := m.Called(field)
	return mustUpdateBuilder(args.Get(0))
}

func (m *MockUpdateBuilder) Delete(field string, value any) core.UpdateBuilder {
	args := m.Called(field, value)
	return mustUpdateBuilder(args.Get(0))
}

func (m *MockUpdateBuilder) AppendToList(field string, values any) core.UpdateBuilder {
	args := m.Called(field, values)
	return mustUpdateBuilder(args.Get(0))
}

func (m *MockUpdateBuilder) PrependToList(field string, values any) core.UpdateBuilder {
	args := m.Called(field, values)
	return mustUpdateBuilder(args.Get(0))
}

func (m *MockUpdateBuilder) RemoveFromListAt(field string, index int) core.UpdateBuilder {
	args := m.Called(field, index)
	return mustUpdateBuilder(args.Get(0))
}

func (m *MockUpdateBuilder) SetListElement(field string, index int, value any) core.UpdateBuilder {
	args := m.Called(field, index, value)
	return mustUpdateBuilder(args.Get(0))
}

func (m *MockUpdateBuilder) Condition(field string, operator string, value any) core.UpdateBuilder {
	args := m.Called(field, operator, value)
	return mustUpdateBuilder(args.Get(0))
}

func (m *MockUpdateBuilder) OrCondition(field string, operator string, value any) core.UpdateBuilder {
	args := m.Called(field, operator, value)
	return mustUpdateBuilder(args.Get(0))
}

func (m *MockUpdateBuilder) ConditionExists(field string) core.UpdateBuilder {
	args := m.Called(field)
	return mustUpdateBuilder(args.Get(0))
}

func (m *MockUpdateBuilder) ConditionNotExists(field string) core.UpdateBuilder {
	args := m.Called(field)
	return mustUpdateBuilder(args.Get(0))
}

func (m *MockUpdateBuilder) ConditionVersion(currentVersion int64) core.UpdateBuilder {
	args := m.Called(currentVersion)
	return mustUpdateBuilder(args.Get(0))
}

func (m *MockUpdateBuilder) ReturnValues(option string) core.UpdateBuilder {
	args := m.Called(option)
	return mustUpdateBuilder(args.Get(0))
}

func (m *MockUpdateBuilder) Execute() error {
	args := m.Called()
	return args.Error(0)
}

func (m *MockUpdateBuilder) ExecuteWithResult(result any) error {
	args := m.Called(result)
	return args.Error(0)
}
