package theorydb

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/theory-cloud/tabletheory/v3/pkg/core"
	"github.com/theory-cloud/tabletheory/v3/pkg/session"
)

type transactGetUser struct {
	ID    string `theorydb:"pk"`
	Email string
}

func TestDBTransactGetExtension(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "DynamoDB_20120810.TransactGetItems", r.Header.Get("X-Amz-Target"))
		w.Header().Set("Content-Type", "application/x-amz-json-1.0")
		_, err := w.Write([]byte(`{"Responses":[{"Item":{"id":{"S":"user-1"},"email":{"S":"one@example.test"}}}]}`))
		require.NoError(t, err)
	}))
	defer server.Close()

	db, err := New(session.Config{
		Region:   "us-east-1",
		Endpoint: server.URL,
		AWSConfigOptions: []func(*config.LoadOptions) error{
			config.WithRegion("us-east-1"),
			config.WithCredentialsProvider(credentials.NewStaticCredentialsProvider("dummy", "dummy", "")),
		},
	})
	require.NoError(t, err)

	rawDB, ok := db.(*DB)
	require.True(t, ok)
	rawDB.ctx = nil
	getter, ok := db.(core.TransactGetter)
	require.True(t, ok)

	dest := &transactGetUser{}
	var nilCtx context.Context
	results, err := getter.TransactGet(nilCtx, []core.TransactGetRequest{
		{Model: &transactGetUser{}, Key: "user-1", Dest: dest},
	})
	require.NoError(t, err)
	require.Len(t, results, 1)
	assert.True(t, results[0].Found)
	assert.Equal(t, "user-1", dest.ID)
	assert.Equal(t, "one@example.test", dest.Email)
}
