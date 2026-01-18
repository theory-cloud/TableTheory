package mocks

import (
	"github.com/stretchr/testify/mock"

	"github.com/theory-cloud/tabletheory/pkg/core"
)

// MockUpdateBuilder is a mock implementation of the core.UpdateBuilder interface.
// It can be used for unit testing code that uses TableTheory's update builder pattern.
//
// Example usage:
//
//	mockUpdateBuilder := new(mocks.MockUpdateBuilder)
//	mockUpdateBuilder.On("Set", "status", "active").Return(mockUpdateBuilder)
//	mockUpdateBuilder.On("Execute").Return(nil)
type MockUpdateBuilder struct {
	mock.Mock
}

// Set updates a field to a new value
func (m *MockUpdateBuilder) Set(field string, value any) core.UpdateBuilder {
	args := m.Called(field, value)
	return args.Get(0).(core.UpdateBuilder)
}

// SetIfNotExists sets a field value only if it doesn't already exist
func (m *MockUpdateBuilder) SetIfNotExists(field string, value any, defaultValue any) core.UpdateBuilder {
	args := m.Called(field, value, defaultValue)
	return args.Get(0).(core.UpdateBuilder)
}

// Add performs atomic addition (for numbers) or adds to a set
func (m *MockUpdateBuilder) Add(field string, value any) core.UpdateBuilder {
	args := m.Called(field, value)
	return args.Get(0).(core.UpdateBuilder)
}

// Increment increments a numeric field by 1
func (m *MockUpdateBuilder) Increment(field string) core.UpdateBuilder {
	args := m.Called(field)
	return args.Get(0).(core.UpdateBuilder)
}

// Decrement decrements a numeric field by 1
func (m *MockUpdateBuilder) Decrement(field string) core.UpdateBuilder {
	args := m.Called(field)
	return args.Get(0).(core.UpdateBuilder)
}

// Remove removes an attribute from the item
func (m *MockUpdateBuilder) Remove(field string) core.UpdateBuilder {
	args := m.Called(field)
	return args.Get(0).(core.UpdateBuilder)
}

// Delete removes values from a set
func (m *MockUpdateBuilder) Delete(field string, value any) core.UpdateBuilder {
	args := m.Called(field, value)
	return args.Get(0).(core.UpdateBuilder)
}

// AppendToList appends values to the end of a list
func (m *MockUpdateBuilder) AppendToList(field string, values any) core.UpdateBuilder {
	args := m.Called(field, values)
	return args.Get(0).(core.UpdateBuilder)
}

// PrependToList prepends values to the beginning of a list
func (m *MockUpdateBuilder) PrependToList(field string, values any) core.UpdateBuilder {
	args := m.Called(field, values)
	return args.Get(0).(core.UpdateBuilder)
}

// RemoveFromListAt removes an element at a specific index from a list
func (m *MockUpdateBuilder) RemoveFromListAt(field string, index int) core.UpdateBuilder {
	args := m.Called(field, index)
	return args.Get(0).(core.UpdateBuilder)
}

// SetListElement sets a specific element in a list
func (m *MockUpdateBuilder) SetListElement(field string, index int, value any) core.UpdateBuilder {
	args := m.Called(field, index, value)
	return args.Get(0).(core.UpdateBuilder)
}

// Condition adds a condition that must be met for the update to succeed
func (m *MockUpdateBuilder) Condition(field string, operator string, value any) core.UpdateBuilder {
	m.Called(field, operator, value)
	return m
}

// OrCondition adds a condition with OR logic
func (m *MockUpdateBuilder) OrCondition(field string, operator string, value any) core.UpdateBuilder {
	m.Called(field, operator, value)
	return m
}

// ConditionExists adds a condition that the field must exist
func (m *MockUpdateBuilder) ConditionExists(field string) core.UpdateBuilder {
	args := m.Called(field)
	return args.Get(0).(core.UpdateBuilder)
}

// ConditionNotExists adds a condition that the field must not exist
func (m *MockUpdateBuilder) ConditionNotExists(field string) core.UpdateBuilder {
	args := m.Called(field)
	return args.Get(0).(core.UpdateBuilder)
}

// ConditionVersion adds optimistic locking based on version
func (m *MockUpdateBuilder) ConditionVersion(currentVersion int64) core.UpdateBuilder {
	args := m.Called(currentVersion)
	return args.Get(0).(core.UpdateBuilder)
}

// ReturnValues specifies what values to return after the update
func (m *MockUpdateBuilder) ReturnValues(option string) core.UpdateBuilder {
	args := m.Called(option)
	return args.Get(0).(core.UpdateBuilder)
}

// Execute performs the update operation
func (m *MockUpdateBuilder) Execute() error {
	args := m.Called()
	return args.Error(0)
}

// ExecuteWithResult performs the update and returns the result
func (m *MockUpdateBuilder) ExecuteWithResult(result any) error {
	args := m.Called(result)
	return args.Error(0)
}
