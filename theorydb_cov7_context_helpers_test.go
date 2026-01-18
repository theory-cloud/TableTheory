package theorydb

import (
	"context"
	"reflect"
	"testing"

	"github.com/stretchr/testify/require"

	theorydbErrors "github.com/theory-cloud/tabletheory/pkg/errors"
	"github.com/theory-cloud/tabletheory/pkg/model"
	"github.com/theory-cloud/tabletheory/pkg/session"
)

type cov7ContextKey string

const cov7SourceKey cov7ContextKey = "source"

func TestQueryExecutor_SetContext_And_CtxOrBackground_COV7(t *testing.T) {
	dbCtx := context.WithValue(context.Background(), cov7SourceKey, "db")
	executor := &queryExecutor{db: &DB{ctx: dbCtx}}

	require.Equal(t, "db", executor.ctxOrBackground().Value(cov7SourceKey))

	executor.SetContext(nil) //nolint:staticcheck // Intentionally exercising SetContext nil fallback.
	require.Nil(t, executor.ctxOrBackground().Value(cov7SourceKey))

	customCtx := context.WithValue(context.Background(), cov7SourceKey, "custom")
	executor.SetContext(customCtx)
	require.Equal(t, "custom", executor.ctxOrBackground().Value(cov7SourceKey))
}

func TestQueryExecutor_session_And_failClosedIfEncrypted_COV7(t *testing.T) {
	var nilExecutor *queryExecutor
	require.Nil(t, nilExecutor.session())
	require.NoError(t, nilExecutor.failClosedIfEncrypted())

	executor := &queryExecutor{db: &DB{}}
	require.Nil(t, executor.session())

	executor.db.session = &session.Session{}
	require.NotNil(t, executor.session())

	type testModel struct{}
	metaEncrypted := &model.Metadata{
		Type: reflect.TypeOf(testModel{}),
		Fields: map[string]*model.FieldMetadata{
			"Secret": {DBName: "secret", IsEncrypted: true},
		},
	}

	executor.metadata = metaEncrypted
	require.ErrorIs(t, executor.failClosedIfEncrypted(), theorydbErrors.ErrEncryptionNotConfigured)

	executor.metadata = &model.Metadata{
		Type:   reflect.TypeOf(testModel{}),
		Fields: map[string]*model.FieldMetadata{},
	}
	require.NoError(t, executor.failClosedIfEncrypted())
}

func TestDerefNonNilPointer_Errors_COV7(t *testing.T) {
	_, err := derefNonNilPointer(nil)
	require.Error(t, err)

	var dest *int
	_, err = derefNonNilPointer(dest)
	require.Error(t, err)
}

func TestQueryExecutor_ctxOrBackground_DefaultsToBackground_COV7(t *testing.T) {
	executor := &queryExecutor{}
	require.Nil(t, executor.ctxOrBackground().Value(cov7SourceKey))
}
