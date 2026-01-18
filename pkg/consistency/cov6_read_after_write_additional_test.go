package consistency

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/theory-cloud/tabletheory/pkg/core"
	"github.com/theory-cloud/tabletheory/pkg/mocks"
)

type cov6ConsistencyDB struct {
	query core.Query
}

func (d cov6ConsistencyDB) Model(any) core.Query                      { return d.query }
func (d cov6ConsistencyDB) Transaction(func(tx *core.Tx) error) error { return nil }
func (d cov6ConsistencyDB) Migrate() error                            { return nil }
func (d cov6ConsistencyDB) AutoMigrate(...any) error                  { return nil }
func (d cov6ConsistencyDB) Close() error                              { return nil }
func (d cov6ConsistencyDB) WithContext(context.Context) core.DB       { return d }

func TestConsistentQueryBuilder_First_RetryBranches_COV6(t *testing.T) {
	t.Run("uses verification helper when VerifyFunc provided", func(t *testing.T) {
		mockQuery := new(mocks.MockQuery)
		mockQuery.On("First", mock.Anything).Return(nil).Once()

		helper := NewReadAfterWriteHelper(cov6ConsistencyDB{query: mockQuery})
		builder := helper.QueryAfterWrite(&struct{}{}, &QueryAfterWriteOptions{
			RetryConfig: &RetryConfig{MaxRetries: 0, InitialDelay: 0},
			VerifyFunc:  func(any) bool { return true },
		})

		var out struct{}
		require.NoError(t, builder.First(&out))
		mockQuery.AssertNotCalled(t, "WithRetry", mock.Anything, mock.Anything)
		mockQuery.AssertExpectations(t)
	})

	t.Run("uses WithRetry when VerifyFunc missing", func(t *testing.T) {
		mockQuery := new(mocks.MockQuery)
		mockQuery.On("WithRetry", 1, time.Duration(0)).Return(mockQuery).Once()
		mockQuery.On("First", mock.Anything).Return(nil).Once()

		helper := NewReadAfterWriteHelper(cov6ConsistencyDB{query: mockQuery})
		builder := helper.QueryAfterWrite(&struct{}{}, &QueryAfterWriteOptions{
			RetryConfig: &RetryConfig{MaxRetries: 1, InitialDelay: 0},
		})

		var out struct{}
		require.NoError(t, builder.First(&out))
		mockQuery.AssertExpectations(t)
	})
}

func TestConsistentQueryBuilder_All_RetryBranches_COV6(t *testing.T) {
	mockQuery := new(mocks.MockQuery)
	mockQuery.On("WithRetry", 2, time.Duration(0)).Return(mockQuery).Once()
	mockQuery.On("All", mock.Anything).Return(nil).Once()

	helper := NewReadAfterWriteHelper(cov6ConsistencyDB{query: mockQuery})
	builder := helper.QueryAfterWrite(&struct{}{}, &QueryAfterWriteOptions{
		RetryConfig: &RetryConfig{MaxRetries: 2, InitialDelay: 0},
	})

	var out []struct{}
	require.NoError(t, builder.All(&out))
	mockQuery.AssertExpectations(t)
}

func TestReadAfterWriteHelper_CreateAndUpdate_WithVerification_COV6(t *testing.T) {
	type item struct {
		Name string
	}

	t.Run("create verifies and waits", func(t *testing.T) {
		mockQuery := new(mocks.MockQuery)
		mockQuery.On("Create").Return(nil).Once()
		mockQuery.On("ConsistentRead").Return(mockQuery).Once()
		mockQuery.On("First", mock.Anything).Return(nil).Once()

		helper := NewReadAfterWriteHelper(cov6ConsistencyDB{query: mockQuery})
		model := &item{Name: "old"}
		require.NoError(t, helper.CreateWithConsistency(model, &WriteOptions{
			VerifyWrite:           true,
			WaitForGSIPropagation: time.Nanosecond,
		}))

		mockQuery.AssertExpectations(t)
	})

	t.Run("update copies verified data back", func(t *testing.T) {
		mockQuery := new(mocks.MockQuery)
		mockQuery.On("Update", []string{"Name"}).Return(nil).Once()
		mockQuery.On("ConsistentRead").Return(mockQuery).Once()
		mockQuery.On("First", mock.Anything).Run(func(args mock.Arguments) {
			dest, ok := args.Get(0).(*item)
			require.True(t, ok)
			dest.Name = "new"
		}).Return(nil).Once()

		helper := NewReadAfterWriteHelper(cov6ConsistencyDB{query: mockQuery})
		model := &item{Name: "old"}
		require.NoError(t, helper.UpdateWithConsistency(model, []string{"Name"}, &WriteOptions{
			VerifyWrite:           true,
			WaitForGSIPropagation: time.Nanosecond,
		}))
		require.Equal(t, "new", model.Name)

		mockQuery.AssertExpectations(t)
	})
}

func TestWriteAndReadPattern_CreateAndQueryGSI_FallsBack_COV6(t *testing.T) {
	type item struct {
		PK string
	}

	mockQuery := new(mocks.MockQuery)
	mockQuery.On("Create").Return(nil).Once()
	mockQuery.On("Index", "gsi1").Return(mockQuery).Once()
	mockQuery.On("Where", "GSI1PK", "=", "gsi-key").Return(mockQuery).Once()
	mockQuery.On("WithRetry", 5, 100*time.Millisecond).Return(mockQuery).Once()

	mockQuery.On("First", mock.Anything).Return(errors.New("not ready")).Once()
	mockQuery.On("ConsistentRead").Return(mockQuery).Once()
	mockQuery.On("Where", "PK", "=", "p1").Return(mockQuery).Once()
	mockQuery.On("First", mock.Anything).Return(nil).Once()

	pattern := NewWriteAndReadPattern(cov6ConsistencyDB{query: mockQuery})

	src := &item{PK: "p1"}
	dest := &item{}
	require.NoError(t, pattern.CreateAndQueryGSI(src, "gsi1", "GSI1PK", "gsi-key", dest))

	mockQuery.AssertExpectations(t)
}
