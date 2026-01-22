package mocks_test

import (
	"context"
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
