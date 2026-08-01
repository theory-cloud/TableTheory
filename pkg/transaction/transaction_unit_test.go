package transaction

import (
	"bytes"
	"context"
	stderrs "errors"
	"fmt"
	"io"
	"net/http"
	"testing"
	"time"

	_ "unsafe"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb/types"
	"github.com/aws/smithy-go"
	"github.com/stretchr/testify/require"

	theorydberrors "github.com/theory-cloud/tabletheory/v3/pkg/errors"
	"github.com/theory-cloud/tabletheory/v3/pkg/model"
	"github.com/theory-cloud/tabletheory/v3/pkg/session"
	pkgTypes "github.com/theory-cloud/tabletheory/v3/pkg/types"
)

//go:linkname sessionConfigLoadFunc github.com/theory-cloud/tabletheory/v3/pkg/session.configLoadFunc
var sessionConfigLoadFunc func(context.Context, ...func(*config.LoadOptions) error) (aws.Config, error)

func stubSessionConfigLoad(t *testing.T, fn func(context.Context, ...func(*config.LoadOptions) error) (aws.Config, error)) {
	t.Helper()

	original := sessionConfigLoadFunc
	sessionConfigLoadFunc = fn

	t.Cleanup(func() {
		sessionConfigLoadFunc = original
	})
}

type stubHTTPClient struct {
	responses map[string]string
	calls     *int
}

func (c stubHTTPClient) Do(req *http.Request) (*http.Response, error) {
	if c.calls != nil {
		*c.calls++
	}
	target := req.Header.Get("X-Amz-Target")
	if req.Body != nil {
		if _, err := io.Copy(io.Discard, req.Body); err != nil {
			return nil, err
		}
		if err := req.Body.Close(); err != nil {
			return nil, err
		}
	}

	body := c.responses[target]
	if body == "" {
		body = "{}"
	}

	status := http.StatusOK
	return &http.Response{
		StatusCode:    status,
		Status:        fmt.Sprintf("%d %s", status, http.StatusText(status)),
		Header:        http.Header{"Content-Type": []string{"application/x-amz-json-1.0"}},
		ContentLength: int64(len(body)),
		Body:          io.NopCloser(bytes.NewReader([]byte(body))),
		Request:       req,
	}, nil
}

func minimalAWSConfig(httpClient aws.HTTPClient) aws.Config {
	cfg := aws.Config{
		Region:      "us-east-1",
		Credentials: credentials.NewStaticCredentialsProvider("test", "secret", "token"),
		Retryer: func() aws.Retryer {
			return aws.NopRetryer{}
		},
		HTTPClient: httpClient,
	}
	return cfg
}

type unitUser struct {
	UpdatedAt time.Time `theorydb:"updated_at"`
	ID        string    `theorydb:"pk"`
	Email     string
	Version   int `theorydb:"version"`
}

type unitCreatedAtOnly struct {
	CreatedAt time.Time `theorydb:"created_at,attr:createdAt" json:"createdAt"`
	ID        string    `theorydb:"pk,attr:id" json:"id"`
}

func (unitCreatedAtOnly) TableName() string {
	return "created_at_only_unit"
}

func (unitUser) TableName() string {
	return "users_unit"
}

func TestTransaction_OperationsAndCommit(t *testing.T) {
	httpClient := stubHTTPClient{
		responses: map[string]string{
			"DynamoDB_20120810.TransactWriteItems": `{}`,
			"DynamoDB_20120810.TransactGetItems":   `{"Responses":[{"Item":{"id":{"S":"user-1"},"email":{"S":"test@example.com"}}}]}`,
		},
	}

	stubSessionConfigLoad(t, func(context.Context, ...func(*config.LoadOptions) error) (aws.Config, error) {
		return minimalAWSConfig(httpClient), nil
	})

	sess, err := session.NewSession(&session.Config{Region: "us-east-1"})
	require.NoError(t, err)

	registry := model.NewRegistry()
	require.NoError(t, registry.Register(&unitUser{}))
	converter := pkgTypes.NewConverter()

	tx := NewTransaction(sess, registry, converter)

	ctx := context.Background()
	require.Same(t, tx, tx.WithContext(ctx))
	require.Equal(t, ctx, tx.ctx)

	user := &unitUser{
		ID:      "user-1",
		Email:   "test@example.com",
		Version: 1,
	}

	require.NoError(t, tx.Create(user))
	require.NoError(t, tx.Update(user))
	require.NoError(t, tx.Delete(user))
	require.NoError(t, tx.Get(user, &unitUser{}))

	require.NoError(t, tx.Commit())
	require.NotEmpty(t, tx.results)
	require.Contains(t, tx.results, "0")
	require.NotNil(t, tx.results["0"])

	require.NoError(t, tx.Rollback())
	require.Nil(t, tx.writes)
	require.Nil(t, tx.reads)
	require.NotNil(t, tx.results)
	require.Empty(t, tx.results)
}

func TestTransaction_UpdateErrorPoisonsCommit(t *testing.T) {
	requestCount := 0
	httpClient := stubHTTPClient{
		responses: map[string]string{
			"DynamoDB_20120810.TransactWriteItems": `{}`,
		},
		calls: &requestCount,
	}

	stubSessionConfigLoad(t, func(context.Context, ...func(*config.LoadOptions) error) (aws.Config, error) {
		return minimalAWSConfig(httpClient), nil
	})

	sess, err := session.NewSession(&session.Config{Region: "us-east-1"})
	require.NoError(t, err)
	registry := model.NewRegistry()
	require.NoError(t, registry.Register(&unitUser{}))
	require.NoError(t, registry.Register(&unitCreatedAtOnly{}))

	tx := NewTransaction(sess, registry, pkgTypes.NewConverter())
	require.NoError(t, tx.Create(&unitUser{ID: "queued-before-update-error"}))
	updateErr := tx.Update(&unitCreatedAtOnly{ID: "rejected-update"})
	require.EqualError(t, updateErr, "no non-key fields to update")
	queuedBeforeRetry := len(tx.writes)
	require.Same(t, updateErr, tx.Update(&unitUser{ID: "blocked-after-update-error", Email: "blocked@example.com"}))
	require.Len(t, tx.writes, queuedBeforeRetry, "a poisoned transaction must not queue a second operation")

	require.EqualError(t, tx.Commit(), "no non-key fields to update")
	require.Zero(t, requestCount, "a poisoned transaction must not submit earlier queued writes")

	require.NoError(t, tx.Rollback())
	require.NoError(t, tx.Update(&unitUser{ID: "accepted-after-rollback", Email: "accepted@example.com"}))
	require.NoError(t, tx.Commit(), "rollback must clear the stored transaction error")
	require.Equal(t, 1, requestCount, "a post-rollback update must submit normally")
}

func TestTransaction_CreateAndDeleteErrorsPoisonCommit(t *testing.T) {
	tests := []struct {
		name      string
		operation func(*Transaction) error
	}{
		{
			name: "create",
			operation: func(tx *Transaction) error {
				return tx.Create(&struct{ ID string }{ID: "not-registered"})
			},
		},
		{
			name: "delete",
			operation: func(tx *Transaction) error {
				return tx.Delete(&writePolicyTransactionEvent{PK: "release#svc", SK: "event#1"})
			},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			requestCount := 0
			httpClient := stubHTTPClient{
				responses: map[string]string{
					"DynamoDB_20120810.TransactWriteItems": `{}`,
				},
				calls: &requestCount,
			}

			stubSessionConfigLoad(t, func(context.Context, ...func(*config.LoadOptions) error) (aws.Config, error) {
				return minimalAWSConfig(httpClient), nil
			})

			sess, err := session.NewSession(&session.Config{Region: "us-east-1"})
			require.NoError(t, err)
			registry := model.NewRegistry()
			require.NoError(t, registry.Register(&unitUser{}))
			require.NoError(t, registry.Register(&writePolicyTransactionEvent{}))

			tx := NewTransaction(sess, registry, pkgTypes.NewConverter())
			require.NoError(t, tx.Create(&unitUser{ID: "queued-before-write-error"}))
			operationErr := test.operation(tx)
			require.Error(t, operationErr)

			require.Same(t, operationErr, tx.Commit(), "commit must return the stored first error")
			require.Zero(t, requestCount, "a poisoned transaction must not submit earlier queued writes")
		})
	}
}

func TestTransaction_RollbackThenGetCommitReusesResultsMap(t *testing.T) {
	requestCount := 0
	httpClient := stubHTTPClient{
		responses: map[string]string{
			"DynamoDB_20120810.TransactGetItems": `{"Responses":[{"Item":{"id":{"S":"user-1"}}}]}`,
		},
		calls: &requestCount,
	}

	stubSessionConfigLoad(t, func(context.Context, ...func(*config.LoadOptions) error) (aws.Config, error) {
		return minimalAWSConfig(httpClient), nil
	})

	sess, err := session.NewSession(&session.Config{Region: "us-east-1"})
	require.NoError(t, err)
	registry := model.NewRegistry()
	require.NoError(t, registry.Register(&unitUser{}))
	require.NoError(t, registry.Register(&unitCreatedAtOnly{}))

	tx := NewTransaction(sess, registry, pkgTypes.NewConverter())
	require.EqualError(t, tx.Update(&unitCreatedAtOnly{ID: "poison-before-rollback"}), "no non-key fields to update")
	require.NoError(t, tx.Rollback())
	require.NoError(t, tx.Get(&unitUser{ID: "user-1"}, &unitUser{}))

	var commitErr error
	require.NotPanics(t, func() {
		commitErr = tx.Commit()
	})
	require.NoError(t, commitErr)
	require.Equal(t, 1, requestCount)
	require.Contains(t, tx.results, "0")
}

func TestTransaction_handleTransactionError(t *testing.T) {
	tx := &Transaction{}

	require.NoError(t, tx.handleTransactionError(nil))

	err := tx.handleTransactionError(&types.ConditionalCheckFailedException{})
	require.ErrorIs(t, err, theorydberrors.ErrConditionFailed)

	err = tx.handleTransactionError(&types.TransactionCanceledException{
		CancellationReasons: []types.CancellationReason{{Code: aws.String("ConditionalCheckFailed")}},
	})
	require.ErrorIs(t, err, theorydberrors.ErrConditionFailed)

	err = tx.handleTransactionError(&types.TransactionCanceledException{
		CancellationReasons: []types.CancellationReason{{Code: aws.String("TransactionConflict")}},
	})
	require.ErrorIs(t, err, theorydberrors.ErrTransactionConflict)
	require.ErrorIs(t, err, theorydberrors.ErrTransactionFailed)

	err = tx.handleTransactionError(&types.TransactionCanceledException{
		CancellationReasons: []types.CancellationReason{{Code: aws.String("ThrottlingError")}},
	})
	require.ErrorIs(t, err, theorydberrors.ErrThrottled)

	err = tx.handleTransactionError(transactionAPIError("TransactionCanceledException"))
	require.Error(t, err)
	require.Contains(t, err.Error(), "transaction canceled")

	err = tx.handleTransactionError(transactionAPIError("ValidationException"))
	require.Error(t, err)
	require.Contains(t, err.Error(), "validation error")

	err = tx.handleTransactionError(transactionAPIError("ProvisionedThroughputExceededException"))
	require.ErrorIs(t, err, theorydberrors.ErrThrottled)

	plain := stderrs.New("prefix ConditionalCheckFailed suffix")
	require.ErrorIs(t, tx.handleTransactionError(plain), plain)

	other := stderrs.New("something else")
	require.ErrorIs(t, tx.handleTransactionError(other), other)
}

func transactionAPIError(code string) error {
	return &smithy.GenericAPIError{Code: code, Message: "test"}
}
