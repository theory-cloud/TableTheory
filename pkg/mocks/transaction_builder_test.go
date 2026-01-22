package mocks_test

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"

	"github.com/theory-cloud/tabletheory/pkg/core"
	"github.com/theory-cloud/tabletheory/pkg/mocks"
)

func TestMockTransactionBuilderImplementsInterface(t *testing.T) {
	var _ core.TransactionBuilder = (*mocks.MockTransactionBuilder)(nil)
}

func TestMockExtendedDB_TransactWrite_AutoRunsCallback(t *testing.T) {
	ctx := context.Background()
	db := mocks.NewMockExtendedDBStrict()

	tx := new(mocks.MockTransactionBuilder)
	db.TransactWriteBuilder = tx

	db.On("TransactWrite", ctx, mock.Anything).Return(nil).Once()
	tx.On("WithContext", ctx).Return(tx).Once()
	tx.On("UpdateWithBuilder", mock.Anything, mock.Anything, mock.Anything).Return(tx).Once()
	tx.On("Execute").Return(nil).Once()

	var callbackCalls int
	var updateFnCalls int

	err := db.TransactWrite(ctx, func(tb core.TransactionBuilder) error {
		callbackCalls++
		tb.WithContext(ctx)
		tb.UpdateWithBuilder(&struct{}{}, func(core.UpdateBuilder) error {
			updateFnCalls++
			return nil
		})
		return nil
	})

	assert.NoError(t, err)
	assert.Equal(t, 1, callbackCalls)
	assert.Equal(t, 1, updateFnCalls)

	db.AssertExpectations(t)
	tx.AssertExpectations(t)
}

func TestMockExtendedDB_TransactWrite_DoesNotDoubleRunCallback(t *testing.T) {
	ctx := context.Background()
	db := mocks.NewMockExtendedDBStrict()
	tx := new(mocks.MockTransactionBuilder)

	db.On("TransactWrite", ctx, mock.Anything).Run(func(args mock.Arguments) {
		fn := args.Get(1).(func(core.TransactionBuilder) error)
		_ = fn(tx)
	}).Return(nil).Once()

	var callbackCalls int
	err := db.TransactWrite(ctx, func(core.TransactionBuilder) error {
		callbackCalls++
		return nil
	})

	assert.NoError(t, err)
	assert.Equal(t, 1, callbackCalls)
	db.AssertExpectations(t)
}

func TestMockDB_Transaction_AutoRunsCallback(t *testing.T) {
	db := new(mocks.MockDB)
	db.On("Transaction", mock.Anything).Return(nil).Once()

	var calls int
	err := db.Transaction(func(*core.Tx) error {
		calls++
		return nil
	})

	assert.NoError(t, err)
	assert.Equal(t, 1, calls)
	db.AssertExpectations(t)
}

func TestMockDB_Transaction_DoesNotDoubleRunCallback(t *testing.T) {
	db := new(mocks.MockDB)
	db.On("Transaction", mock.Anything).Run(func(args mock.Arguments) {
		fn := args.Get(0).(func(*core.Tx) error)
		_ = fn(&core.Tx{})
	}).Return(nil).Once()

	var calls int
	err := db.Transaction(func(*core.Tx) error {
		calls++
		return nil
	})

	assert.NoError(t, err)
	assert.Equal(t, 1, calls)
	db.AssertExpectations(t)
}

func TestMockTransactionBuilder_DefaultBehavior_ReturnsSelf(t *testing.T) {
	ctx := context.Background()
	tx := new(mocks.MockTransactionBuilder)
	model := &struct{}{}
	cond := core.TransactCondition{Kind: core.TransactConditionKindPrimaryKeyExists}

	assert.Same(t, tx, tx.Put(model, cond))
	assert.Same(t, tx, tx.Create(model, cond))
	assert.Same(t, tx, tx.Update(model, []string{"a"}, cond))
	assert.Same(t, tx, tx.UpdateWithBuilder(model, nil, cond))
	assert.Same(t, tx, tx.Delete(model, cond))
	assert.Same(t, tx, tx.ConditionCheck(model, cond))
	assert.Same(t, tx, tx.WithContext(ctx))
	assert.NoError(t, tx.Execute())
	assert.NoError(t, tx.ExecuteWithContext(ctx))
}

func TestMockTransactionBuilder_ExpectedCalls_AreRoutedThroughTestify(t *testing.T) {
	ctx := context.Background()
	tx := new(mocks.MockTransactionBuilder)
	model := &struct{}{}
	cond := core.TransactCondition{Kind: core.TransactConditionKindPrimaryKeyExists}

	tx.On("Put", model, []core.TransactCondition{cond}).Return(tx).Once()
	tx.On("Create", model, []core.TransactCondition{cond}).Return(tx).Once()
	tx.On("Update", model, []string{"a"}, []core.TransactCondition{cond}).Return(tx).Once()
	tx.On("UpdateWithBuilder", model, mock.Anything, []core.TransactCondition{cond}).Return(tx).Once()
	tx.On("Delete", model, []core.TransactCondition{cond}).Return(tx).Once()
	tx.On("ConditionCheck", model, []core.TransactCondition{cond}).Return(tx).Once()
	tx.On("WithContext", ctx).Return(tx).Once()
	tx.On("ExecuteWithContext", ctx).Return(nil).Once()

	assert.Same(t, tx, tx.Put(model, cond))
	assert.Same(t, tx, tx.Create(model, cond))
	assert.Same(t, tx, tx.Update(model, []string{"a"}, cond))
	assert.Same(t, tx, tx.UpdateWithBuilder(model, func(core.UpdateBuilder) error { return nil }, cond))
	assert.Same(t, tx, tx.Delete(model, cond))
	assert.Same(t, tx, tx.ConditionCheck(model, cond))
	assert.Same(t, tx, tx.WithContext(ctx))
	assert.NoError(t, tx.ExecuteWithContext(ctx))

	tx.AssertExpectations(t)
}

func TestMockTransactionBuilder_UpdateWithBuilder_RunsUpdateFnOnExecute_AndCoversNoopUpdateBuilder(t *testing.T) {
	tx := new(mocks.MockTransactionBuilder)
	model := &struct{}{}

	var calls int
	tx.UpdateWithBuilder(model, func(ub core.UpdateBuilder) error {
		calls++
		ub.Set("a", 1)
		ub.SetIfNotExists("b", 2, 0)
		ub.Add("c", 3)
		ub.Increment("d")
		ub.Decrement("e")
		ub.Remove("f")
		ub.Delete("g", 1)
		ub.AppendToList("h", []string{"x"})
		ub.PrependToList("i", []string{"y"})
		ub.RemoveFromListAt("j", 0)
		ub.SetListElement("k", 1, "z")
		ub.Condition("l", "=", 1)
		ub.OrCondition("m", "=", 2)
		ub.ConditionExists("n")
		ub.ConditionNotExists("o")
		ub.ConditionVersion(1)
		ub.ReturnValues("ALL_NEW")
		_ = ub.Execute()
		_ = ub.ExecuteWithResult(&struct{}{})
		return nil
	})

	assert.NoError(t, tx.Execute())
	assert.Equal(t, 1, calls)
	assert.NoError(t, tx.Execute())
	assert.Equal(t, 1, calls)
}

func TestMockTransactionBuilder_ExecuteWithContext_RunsPendingUpdateFns(t *testing.T) {
	tx := new(mocks.MockTransactionBuilder)
	model := &struct{}{}
	ctx := context.Background()

	var calls int
	tx.UpdateWithBuilder(model, func(core.UpdateBuilder) error {
		calls++
		return nil
	})

	assert.NoError(t, tx.ExecuteWithContext(ctx))
	assert.Equal(t, 1, calls)
}

func TestMockTransactionBuilder_Execute_ReturnsExpectedCallError(t *testing.T) {
	tx := new(mocks.MockTransactionBuilder)
	expectedErr := errors.New("execute failed")

	tx.On("Execute").Return(expectedErr).Once()
	assert.ErrorIs(t, tx.Execute(), expectedErr)
	tx.AssertExpectations(t)
}

func TestMockTransactionBuilder_ExecuteWithBuilder_ErrorIsPropagated(t *testing.T) {
	tx := new(mocks.MockTransactionBuilder)
	expectedErr := errors.New("update failed")

	tx.UpdateWithBuilder(&struct{}{}, func(core.UpdateBuilder) error {
		return expectedErr
	})

	assert.ErrorIs(t, tx.Execute(), expectedErr)
}

func TestMockTransactionBuilder_ExecuteWithContext_ReturnsExpectedCallError(t *testing.T) {
	tx := new(mocks.MockTransactionBuilder)
	ctx := context.Background()
	expectedErr := errors.New("execute-with-context failed")

	tx.On("ExecuteWithContext", ctx).Return(expectedErr).Once()
	assert.ErrorIs(t, tx.ExecuteWithContext(ctx), expectedErr)
	tx.AssertExpectations(t)
}

func TestMockTransactionBuilder_UpdateWithBuilder_NilFn_UsesExpectedCallWhenConfigured(t *testing.T) {
	tx := new(mocks.MockTransactionBuilder)
	tx.On("UpdateWithBuilder", mock.Anything, mock.Anything, mock.Anything).Return(tx).Once()

	assert.Same(t, tx, tx.UpdateWithBuilder(&struct{}{}, nil))
	tx.AssertExpectations(t)
}

func TestMockTransactionBuilder_UsesProvidedUpdateBuilder(t *testing.T) {
	tx := new(mocks.MockTransactionBuilder)
	tx.UpdateBuilder = new(mocks.MockUpdateBuilder)

	var calls int
	tx.UpdateWithBuilder(&struct{}{}, func(core.UpdateBuilder) error {
		calls++
		return nil
	})

	assert.NoError(t, tx.Execute())
	assert.Equal(t, 1, calls)
}

func TestMockExtendedDB_DescribeTable_ReturnsNilWhenValueIsNil(t *testing.T) {
	db := mocks.NewMockExtendedDBStrict()
	expectedErr := errors.New("not found")

	db.On("DescribeTable", mock.Anything).Return(nil, expectedErr).Once()
	got, err := db.DescribeTable(&struct{}{})

	assert.Nil(t, got)
	assert.ErrorIs(t, err, expectedErr)
	db.AssertExpectations(t)
}

func TestMockExtendedDB_Transact_PanicsOnUnexpectedReturnType(t *testing.T) {
	db := mocks.NewMockExtendedDBStrict()
	db.On("Transact").Return("not-a-builder").Once()

	assert.Panics(t, func() { _ = db.Transact() })
	db.AssertExpectations(t)
}

func TestMockExtendedDB_TransactWrite_ReturnsCallbackError(t *testing.T) {
	ctx := context.Background()
	db := mocks.NewMockExtendedDBStrict()
	db.TransactWriteBuilder = new(mocks.MockTransactionBuilder)

	db.On("TransactWrite", ctx, mock.Anything).Return(nil).Once()

	expectedErr := errors.New("tx failed")
	err := db.TransactWrite(ctx, func(core.TransactionBuilder) error { return expectedErr })

	assert.ErrorIs(t, err, expectedErr)
	db.AssertExpectations(t)
}

func TestMockDB_Transaction_ReturnsExpectationError(t *testing.T) {
	db := new(mocks.MockDB)
	expectedErr := errors.New("tx failed")

	db.On("Transaction", mock.Anything).Return(expectedErr).Once()
	err := db.Transaction(func(*core.Tx) error { return nil })

	assert.ErrorIs(t, err, expectedErr)
	db.AssertExpectations(t)
}
