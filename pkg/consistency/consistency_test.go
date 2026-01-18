package consistency

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/theory-cloud/tabletheory/pkg/mocks"
)

func TestRetryWithVerification(t *testing.T) {
	tests := []struct {
		setupMock     func(q *mocks.MockQuery)
		verifyFunc    func(any) bool
		name          string
		expectedCalls int
		expectedError bool
	}{
		{
			name: "Success on first attempt",
			setupMock: func(q *mocks.MockQuery) {
				q.On("First", mock.Anything).Return(nil).Once()
			},
			verifyFunc: func(result any) bool {
				return true
			},
			expectedError: false,
			expectedCalls: 1,
		},
		{
			name: "Success after retries",
			setupMock: func(q *mocks.MockQuery) {
				// Fail twice, then succeed
				q.On("First", mock.Anything).Return(errors.New("not found")).Twice()
				q.On("First", mock.Anything).Return(nil).Once()
			},
			verifyFunc: func(result any) bool {
				return true
			},
			expectedError: false,
			expectedCalls: 3,
		},
		{
			name: "Verification fails after max retries",
			setupMock: func(q *mocks.MockQuery) {
				q.On("First", mock.Anything).Return(nil).Times(3)
			},
			verifyFunc: func(result any) bool {
				return false
			},
			expectedError: true,
			expectedCalls: 3,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mockQuery := new(mocks.MockQuery)
			tt.setupMock(mockQuery)

			config := &RetryConfig{
				MaxRetries:    2,
				InitialDelay:  1 * time.Millisecond,
				MaxDelay:      10 * time.Millisecond,
				BackoffFactor: 2.0,
			}

			var result string
			err := RetryWithVerification(context.Background(), mockQuery, &result, tt.verifyFunc, config)

			if tt.expectedError && err == nil {
				t.Errorf("expected error but got none")
			}
			if !tt.expectedError && err != nil {
				t.Errorf("unexpected error: %v", err)
			}

			mockQuery.AssertNumberOfCalls(t, "First", tt.expectedCalls)
		})
	}
}

func TestWithRetry(t *testing.T) {
	tests := []struct {
		config        *RetryConfig
		name          string
		expectedDelay time.Duration
	}{
		{
			name:          "Default config",
			config:        nil,
			expectedDelay: 100 * time.Millisecond,
		},
		{
			name: "Custom config",
			config: &RetryConfig{
				MaxRetries:    3,
				InitialDelay:  50 * time.Millisecond,
				MaxDelay:      1 * time.Second,
				BackoffFactor: 1.5,
			},
			expectedDelay: 50 * time.Millisecond,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mockQuery := new(mocks.MockQuery)
			retryable := WithRetry(mockQuery, tt.config)

			if retryable.query != mockQuery {
				t.Errorf("expected query to be wrapped")
			}

			config := retryable.config
			if tt.config == nil {
				if config.InitialDelay != 100*time.Millisecond {
					t.Errorf("expected default initial delay")
				}
			} else {
				if config.InitialDelay != tt.expectedDelay {
					t.Errorf("expected custom initial delay")
				}
			}
		})
	}
}

// Test helper types
type TestUser struct {
	PK    string `theorydb:"pk"`
	SK    string `theorydb:"sk"`
	Email string `theorydb:"index:email-index,pk"`
	Name  string
}

func TestReadAfterWriteHelper(t *testing.T) {
	t.Run("CreateWithConsistency", func(t *testing.T) {
		mockDB := new(mocks.MockDB)
		mockQuery := new(mocks.MockQuery)

		user := &TestUser{
			PK:    "USER#123",
			SK:    "PROFILE",
			Email: "test@example.com",
			Name:  "Test User",
		}

		// Setup expectations
		mockDB.On("Model", user).Return(mockQuery)
		mockQuery.On("Create").Return(nil)

		// Test with verify write
		mockDB.On("Model", user).Return(mockQuery)
		mockQuery.On("ConsistentRead").Return(mockQuery)
		mockQuery.On("First", mock.Anything).Return(nil)

		helper := NewReadAfterWriteHelper(mockDB)
		err := helper.CreateWithConsistency(user, &WriteOptions{
			VerifyWrite:           true,
			WaitForGSIPropagation: 10 * time.Millisecond,
		})

		if err != nil {
			t.Errorf("unexpected error: %v", err)
		}

		mockDB.AssertExpectations(t)
		mockQuery.AssertExpectations(t)
	})

	t.Run("UpdateWithConsistency", func(t *testing.T) {
		mockDB := new(mocks.MockDB)
		mockQuery := new(mocks.MockQuery)

		user := &TestUser{
			PK:    "USER#123",
			SK:    "PROFILE",
			Email: "test@example.com",
			Name:  "Initial Name",
		}

		mockDB.On("Model", user).Return(mockQuery).Once()
		mockQuery.On("Update", []string{"Name"}).Return(nil).Once()

		mockDB.On("Model", user).Return(mockQuery).Once()
		mockQuery.On("ConsistentRead").Return(mockQuery).Once()
		mockQuery.On("First", mock.Anything).Run(func(args mock.Arguments) {
			dest, ok := args.Get(0).(*TestUser)
			require.True(t, ok)
			dest.PK = user.PK
			dest.SK = user.SK
			dest.Email = user.Email
			dest.Name = "Verified Name"
		}).Return(nil).Once()

		helper := NewReadAfterWriteHelper(mockDB)
		err := helper.UpdateWithConsistency(user, []string{"Name"}, &WriteOptions{
			VerifyWrite: true,
		})
		require.NoError(t, err)
		require.Equal(t, "Verified Name", user.Name)

		mockDB.AssertExpectations(t)
		mockQuery.AssertExpectations(t)
	})

	t.Run("QueryAfterWrite with main table", func(t *testing.T) {
		mockDB := new(mocks.MockDB)
		mockQuery := new(mocks.MockQuery)

		user := &TestUser{}
		var result TestUser

		// Setup expectations
		mockDB.On("Model", user).Return(mockQuery)
		mockQuery.On("Where", "Email", "=", "test@example.com").Return(mockQuery)
		mockQuery.On("ConsistentRead").Return(mockQuery)
		mockQuery.On("First", &result).Return(nil)

		helper := NewReadAfterWriteHelper(mockDB)
		err := helper.QueryAfterWrite(user, &QueryAfterWriteOptions{
			UseMainTable: true,
		}).Where("Email", "=", "test@example.com").First(&result)

		if err != nil {
			t.Errorf("unexpected error: %v", err)
		}

		mockDB.AssertExpectations(t)
		mockQuery.AssertExpectations(t)
	})

	t.Run("QueryAfterWrite Index and All", func(t *testing.T) {
		mockDB := new(mocks.MockDB)
		mockQuery := new(mocks.MockQuery)

		user := &TestUser{}
		var results []TestUser

		// Index should be ignored when UseMainTable is enabled.
		mockDB.On("Model", user).Return(mockQuery).Once()
		mockQuery.On("ConsistentRead").Return(mockQuery).Once()
		mockQuery.On("All", &results).Return(nil).Once()

		helper := NewReadAfterWriteHelper(mockDB)
		err := helper.QueryAfterWrite(user, &QueryAfterWriteOptions{
			UseMainTable: true,
		}).Index("email-index").All(&results)
		require.NoError(t, err)

		mockQuery.AssertNotCalled(t, "Index", mock.Anything)
		mockDB.AssertExpectations(t)
		mockQuery.AssertExpectations(t)
	})

	t.Run("QueryAfterWrite All with retry config", func(t *testing.T) {
		mockDB := new(mocks.MockDB)
		mockQuery := new(mocks.MockQuery)

		user := &TestUser{}
		var results []TestUser

		config := &RetryConfig{
			MaxRetries:   2,
			InitialDelay: 0,
		}

		mockDB.On("Model", user).Return(mockQuery).Once()
		mockQuery.On("Index", "email-index").Return(mockQuery).Once()
		mockQuery.On("WithRetry", config.MaxRetries, config.InitialDelay).Return(mockQuery).Once()
		mockQuery.On("All", &results).Return(nil).Once()

		helper := NewReadAfterWriteHelper(mockDB)
		err := helper.QueryAfterWrite(user, &QueryAfterWriteOptions{
			RetryConfig: config,
		}).Index("email-index").All(&results)
		require.NoError(t, err)

		mockDB.AssertExpectations(t)
		mockQuery.AssertExpectations(t)
	})
}

func TestRetryableQuery_FirstAndAll(t *testing.T) {
	t.Run("First retries until success", func(t *testing.T) {
		mockQuery := new(mocks.MockQuery)
		mockQuery.On("First", mock.Anything).Return(errors.New("not ready")).Twice()
		mockQuery.On("First", mock.Anything).Return(nil).Once()

		config := &RetryConfig{
			MaxRetries:    2,
			InitialDelay:  0,
			MaxDelay:      0,
			BackoffFactor: 2.0,
			RetryCondition: func(_ any, err error) bool {
				return err != nil
			},
		}

		var result string
		err := WithRetry(mockQuery, config).First(&result)
		require.NoError(t, err)
		mockQuery.AssertNumberOfCalls(t, "First", 3)
	})

	t.Run("First returns wrapped error after retries", func(t *testing.T) {
		mockQuery := new(mocks.MockQuery)
		mockQuery.On("First", mock.Anything).Return(errors.New("boom")).Times(3)

		config := &RetryConfig{
			MaxRetries:    2,
			InitialDelay:  0,
			MaxDelay:      0,
			BackoffFactor: 2.0,
			RetryCondition: func(_ any, err error) bool {
				return err != nil
			},
		}

		var result string
		err := WithRetry(mockQuery, config).First(&result)
		require.Error(t, err)
		require.Contains(t, err.Error(), "query failed after 2 retries")
	})

	t.Run("All errors when retry condition never satisfied", func(t *testing.T) {
		mockQuery := new(mocks.MockQuery)
		mockQuery.On("All", mock.Anything).Return(nil).Once()

		config := &RetryConfig{
			MaxRetries:    0,
			InitialDelay:  0,
			MaxDelay:      0,
			BackoffFactor: 1.0,
			RetryCondition: func(_ any, _ error) bool {
				return true
			},
		}

		var results []string
		err := WithRetry(mockQuery, config).All(&results)
		require.Error(t, err)
		require.Contains(t, err.Error(), "query returned no results")
	})
}

func TestWriteAndReadPattern(t *testing.T) {
	t.Run("CreateAndQueryGSI", func(t *testing.T) {
		mockDB := new(mocks.MockDB)
		mockQuery := new(mocks.MockQuery)

		user := &TestUser{
			PK:    "USER#123",
			SK:    "PROFILE",
			Email: "test@example.com",
			Name:  "Test User",
		}
		var result TestUser

		// Setup create expectation
		mockDB.On("Model", user).Return(mockQuery).Once()
		mockQuery.On("Create").Return(nil).Once()

		// Setup GSI query with retry
		mockDB.On("Model", &result).Return(mockQuery).Once()
		mockQuery.On("Index", "email-index").Return(mockQuery).Once()
		mockQuery.On("Where", "Email", "=", "test@example.com").Return(mockQuery).Once()
		mockQuery.On("WithRetry", 5, 100*time.Millisecond).Return(mockQuery).Once()
		mockQuery.On("First", &result).Return(nil).Once()

		pattern := NewWriteAndReadPattern(mockDB)
		err := pattern.CreateAndQueryGSI(user, "email-index", "Email", "test@example.com", &result)

		if err != nil {
			t.Errorf("unexpected error: %v", err)
		}

		mockDB.AssertExpectations(t)
		mockQuery.AssertExpectations(t)
	})

	t.Run("UpdateAndVerify", func(t *testing.T) {
		mockDB := new(mocks.MockDB)
		mockQuery := new(mocks.MockQuery)

		user := &TestUser{
			PK:    "USER#123",
			SK:    "PROFILE",
			Email: "test@example.com",
			Name:  "Updated Name",
		}

		// Setup update expectation
		mockDB.On("Model", user).Return(mockQuery).Once()
		mockQuery.On("Update", []string{"Name"}).Return(nil).Once()

		// Setup verification expectation
		mockDB.On("Model", user).Return(mockQuery).Once()
		mockQuery.On("ConsistentRead").Return(mockQuery).Once()
		mockQuery.On("First", mock.Anything).Return(nil).Once()

		pattern := NewWriteAndReadPattern(mockDB)
		err := pattern.UpdateAndVerify(user, []string{"Name"})

		if err != nil {
			t.Errorf("unexpected error: %v", err)
		}

		mockDB.AssertExpectations(t)
		mockQuery.AssertExpectations(t)
	})
}

func TestBestPractices(t *testing.T) {
	bp := &BestPractices{}

	if bp.ForGSIQuery() != StrategyRetryWithBackoff {
		t.Errorf("expected retry with backoff for GSI queries")
	}

	if bp.ForCriticalReads() != StrategyStrongConsistency {
		t.Errorf("expected strong consistency for critical reads")
	}

	if bp.ForHighThroughput() != StrategyDelayedRead {
		t.Errorf("expected delayed read for high throughput")
	}
}

func TestRecommendedConfigs(t *testing.T) {
	config := RecommendedRetryConfig()
	if config.MaxRetries != 5 {
		t.Errorf("expected 5 max retries")
	}
	if config.InitialDelay != 100*time.Millisecond {
		t.Errorf("expected 100ms initial delay")
	}

	delay := RecommendedGSIPropagationDelay()
	if delay != 500*time.Millisecond {
		t.Errorf("expected 500ms GSI propagation delay")
	}
}
