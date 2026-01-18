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
	"github.com/stretchr/testify/require"

	theorydberrors "github.com/theory-cloud/tabletheory/pkg/errors"
	"github.com/theory-cloud/tabletheory/pkg/model"
	"github.com/theory-cloud/tabletheory/pkg/session"
	pkgTypes "github.com/theory-cloud/tabletheory/pkg/types"
)

//go:linkname sessionConfigLoadFunc github.com/theory-cloud/tabletheory/pkg/session.configLoadFunc
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
}

func (c stubHTTPClient) Do(req *http.Request) (*http.Response, error) {
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
	require.Nil(t, tx.results)
}

func TestTransaction_handleTransactionError(t *testing.T) {
	tx := &Transaction{}

	require.NoError(t, tx.handleTransactionError(nil))

	err := tx.handleTransactionError(stderrs.New("prefix ConditionalCheckFailed suffix"))
	require.ErrorIs(t, err, theorydberrors.ErrConditionFailed)

	err = tx.handleTransactionError(stderrs.New("prefix TransactionCanceled suffix"))
	require.Error(t, err)
	require.Contains(t, err.Error(), "transaction canceled")

	err = tx.handleTransactionError(stderrs.New("prefix ValidationException suffix"))
	require.Error(t, err)
	require.Contains(t, err.Error(), "validation error")

	other := stderrs.New("something else")
	require.ErrorIs(t, tx.handleTransactionError(other), other)
}
