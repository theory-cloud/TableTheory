package mocks

import (
	"context"

	"github.com/stretchr/testify/mock"

	"github.com/theory-cloud/tabletheory/pkg/core"
)

type noopUpdateBuilder struct{}

func (n *noopUpdateBuilder) Set(_ string, _ any) core.UpdateBuilder                   { return n }
func (n *noopUpdateBuilder) SetIfNotExists(_ string, _ any, _ any) core.UpdateBuilder { return n }
func (n *noopUpdateBuilder) Add(_ string, _ any) core.UpdateBuilder                   { return n }
func (n *noopUpdateBuilder) Increment(_ string) core.UpdateBuilder                    { return n }
func (n *noopUpdateBuilder) Decrement(_ string) core.UpdateBuilder                    { return n }
func (n *noopUpdateBuilder) Remove(_ string) core.UpdateBuilder                       { return n }
func (n *noopUpdateBuilder) Delete(_ string, _ any) core.UpdateBuilder                { return n }
func (n *noopUpdateBuilder) AppendToList(_ string, _ any) core.UpdateBuilder          { return n }
func (n *noopUpdateBuilder) PrependToList(_ string, _ any) core.UpdateBuilder         { return n }
func (n *noopUpdateBuilder) RemoveFromListAt(_ string, _ int) core.UpdateBuilder      { return n }
func (n *noopUpdateBuilder) SetListElement(_ string, _ int, _ any) core.UpdateBuilder { return n }
func (n *noopUpdateBuilder) Condition(_ string, _ string, _ any) core.UpdateBuilder   { return n }
func (n *noopUpdateBuilder) OrCondition(_ string, _ string, _ any) core.UpdateBuilder { return n }
func (n *noopUpdateBuilder) ConditionExists(_ string) core.UpdateBuilder              { return n }
func (n *noopUpdateBuilder) ConditionNotExists(_ string) core.UpdateBuilder           { return n }
func (n *noopUpdateBuilder) ConditionVersion(_ int64) core.UpdateBuilder              { return n }
func (n *noopUpdateBuilder) ReturnValues(_ string) core.UpdateBuilder                 { return n }
func (n *noopUpdateBuilder) Execute() error                                           { return nil }
func (n *noopUpdateBuilder) ExecuteWithResult(_ any) error                            { return nil }

// MockTransactionBuilder is a mock implementation of the core.TransactionBuilder interface.
//
// It supports the fluent transaction DSL used by ExtendedDB.TransactWrite and can optionally
// execute UpdateWithBuilder callbacks when Execute/ExecuteWithContext is invoked.
type MockTransactionBuilder struct {
	mock.Mock

	// UpdateBuilder is used when executing UpdateWithBuilder callbacks during Execute/ExecuteWithContext.
	// If nil, a no-op implementation is used.
	UpdateBuilder core.UpdateBuilder

	pendingUpdateFns []func(core.UpdateBuilder) error
}

var _ core.TransactionBuilder = (*MockTransactionBuilder)(nil)

func (m *MockTransactionBuilder) hasExpectedCall(method string) bool {
	for _, call := range m.ExpectedCalls {
		if call.Method == method {
			return true
		}
	}
	return false
}

// Put adds a put (upsert) operation.
func (m *MockTransactionBuilder) Put(model any, conditions ...core.TransactCondition) core.TransactionBuilder {
	if m.hasExpectedCall("Put") {
		args := m.Called(model, conditions)
		return mustTransactionBuilder(args.Get(0))
	}
	return m
}

// Create adds a put operation guarded by attribute_not_exists on the primary key.
func (m *MockTransactionBuilder) Create(model any, conditions ...core.TransactCondition) core.TransactionBuilder {
	if m.hasExpectedCall("Create") {
		args := m.Called(model, conditions)
		return mustTransactionBuilder(args.Get(0))
	}
	return m
}

// Update updates selected fields on the provided model.
func (m *MockTransactionBuilder) Update(model any, fields []string, conditions ...core.TransactCondition) core.TransactionBuilder {
	if m.hasExpectedCall("Update") {
		args := m.Called(model, fields, conditions)
		return mustTransactionBuilder(args.Get(0))
	}
	return m
}

// UpdateWithBuilder allows complex expression-based updates.
func (m *MockTransactionBuilder) UpdateWithBuilder(model any, updateFn func(core.UpdateBuilder) error, conditions ...core.TransactCondition) core.TransactionBuilder {
	if updateFn != nil {
		ran := false
		var runErr error
		wrapped := func(ub core.UpdateBuilder) error {
			if ran {
				return runErr
			}
			ran = true
			runErr = updateFn(ub)
			return runErr
		}
		m.pendingUpdateFns = append(m.pendingUpdateFns, wrapped)

		if m.hasExpectedCall("UpdateWithBuilder") {
			args := m.Called(model, wrapped, conditions)
			return mustTransactionBuilder(args.Get(0))
		}
		return m
	}

	if m.hasExpectedCall("UpdateWithBuilder") {
		args := m.Called(model, updateFn, conditions)
		return mustTransactionBuilder(args.Get(0))
	}
	return m
}

// Delete removes the provided model by primary key.
func (m *MockTransactionBuilder) Delete(model any, conditions ...core.TransactCondition) core.TransactionBuilder {
	if m.hasExpectedCall("Delete") {
		args := m.Called(model, conditions)
		return mustTransactionBuilder(args.Get(0))
	}
	return m
}

// ConditionCheck adds a pure condition check without mutating data.
func (m *MockTransactionBuilder) ConditionCheck(model any, conditions ...core.TransactCondition) core.TransactionBuilder {
	if m.hasExpectedCall("ConditionCheck") {
		args := m.Called(model, conditions)
		return mustTransactionBuilder(args.Get(0))
	}
	return m
}

// WithContext sets the context used for DynamoDB calls.
func (m *MockTransactionBuilder) WithContext(ctx context.Context) core.TransactionBuilder {
	if m.hasExpectedCall("WithContext") {
		args := m.Called(ctx)
		return mustTransactionBuilder(args.Get(0))
	}
	return m
}

// Execute commits the transaction using the currently configured context.
func (m *MockTransactionBuilder) Execute() error {
	if m.hasExpectedCall("Execute") {
		args := m.Called()
		if err := args.Error(0); err != nil {
			return err
		}
	}
	return m.runPendingUpdateFns()
}

// ExecuteWithContext commits the transaction with an explicit context override.
func (m *MockTransactionBuilder) ExecuteWithContext(ctx context.Context) error {
	if m.hasExpectedCall("ExecuteWithContext") {
		args := m.Called(ctx)
		if err := args.Error(0); err != nil {
			return err
		}
	}
	return m.runPendingUpdateFns()
}

func (m *MockTransactionBuilder) runPendingUpdateFns() error {
	ub := m.UpdateBuilder
	if ub == nil {
		ub = &noopUpdateBuilder{}
	}

	for _, fn := range m.pendingUpdateFns {
		if fn == nil {
			continue
		}
		if err := fn(ub); err != nil {
			return err
		}
	}

	m.pendingUpdateFns = nil
	return nil
}
