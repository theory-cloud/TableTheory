package typed_test

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/theory-cloud/tabletheory/pkg/core"
	theorydbErrors "github.com/theory-cloud/tabletheory/pkg/errors"
	"github.com/theory-cloud/tabletheory/pkg/mocks"
	"github.com/theory-cloud/tabletheory/pkg/query"
	"github.com/theory-cloud/tabletheory/pkg/typed"
)

type typedUser struct {
	PK   string `theorydb:"pk" json:"PK"`
	SK   string `theorydb:"sk" json:"SK"`
	Name string `json:"name"`
}

type typedOrder struct {
	PK string `theorydb:"pk" json:"PK"`
}

func TestTypedQueryFirstAllAndOperators(t *testing.T) {
	db := new(mocks.MockDB)
	q := new(mocks.MockQuery)
	db.On("Model", mock.Anything).Return(q)
	q.On("Where", "SK", string(query.OpBeginsWith), "USER#").Return(q)
	q.On("First", mock.Anything).Run(func(args mock.Arguments) {
		dest, ok := args.Get(0).(*typedUser)
		require.True(t, ok)
		*dest = typedUser{PK: "TENANT#1", SK: "USER#1", Name: "Ada"}
	}).Return(nil)
	q.On("All", mock.Anything).Run(func(args mock.Arguments) {
		dest, ok := args.Get(0).(*[]typedUser)
		require.True(t, ok)
		*dest = []typedUser{{PK: "TENANT#1", SK: "USER#1", Name: "Ada"}}
	}).Return(nil)

	users := typed.ModelOf[typedUser](db)
	first, err := users.Query().Where("SK", query.OpBeginsWith, "USER#").First()
	require.NoError(t, err)
	require.Equal(t, "Ada", first.Name)

	all, err := users.Query().All()
	require.NoError(t, err)
	require.Equal(t, []typedUser{{PK: "TENANT#1", SK: "USER#1", Name: "Ada"}}, all)

	db.AssertExpectations(t)
	q.AssertExpectations(t)
}

func TestTypedKeysStayBoundToModelType(t *testing.T) {
	userKey := typed.NewKey[typedUser]("TENANT#1", "USER#1")
	require.Equal(t, core.KeyPair{PartitionKey: "TENANT#1", SortKey: "USER#1"}, userKey.Core())

	// This assignment is intentionally absent because it would not compile:
	// var _ typed.Key[typedUser] = typed.NewKey[typedOrder]("ORDER#1")
	_ = typed.NewKey[typedOrder]("ORDER#1")
}

func TestTypedBatchGetUsesTypedKeyInput(t *testing.T) {
	db := new(mocks.MockDB)
	q := new(mocks.MockQuery)
	db.On("Model", mock.Anything).Return(q)
	q.On("BatchGet", []any{core.NewKeyPair("TENANT#1", "USER#1")}, mock.Anything).Run(func(args mock.Arguments) {
		dest, ok := args.Get(1).(*[]typedUser)
		require.True(t, ok)
		*dest = []typedUser{{PK: "TENANT#1", SK: "USER#1", Name: "Ada"}}
	}).Return(nil)

	users := typed.ModelOf[typedUser](db)
	got, err := users.BatchGet([]typed.Key[typedUser]{users.Key("TENANT#1", "USER#1")})
	require.NoError(t, err)
	require.Len(t, got, 1)
	require.Equal(t, "Ada", got[0].Name)

	db.AssertExpectations(t)
	q.AssertExpectations(t)
}

func TestTypedModelCRUDDelegatesToBoundQuery(t *testing.T) {
	db := new(mocks.MockDB)
	q := new(mocks.MockQuery)
	item := &typedUser{PK: "TENANT#1", SK: "USER#1", Name: "Ada"}

	db.On("Model", item).Return(q).Times(4)
	q.On("Create").Return(nil).Once()
	q.On("CreateOrUpdate").Return(nil).Once()
	q.On("Update", []string{"Name"}).Return(nil).Once()
	q.On("Delete").Return(nil).Once()

	users := typed.ModelOf[typedUser](db)
	require.NoError(t, users.Create(item))
	require.NoError(t, users.CreateOrUpdate(item))
	require.NoError(t, users.Update(item, "Name"))
	require.NoError(t, users.Delete(item))

	db.AssertExpectations(t)
	q.AssertExpectations(t)
}

func TestTypedQueryBuilderDelegatesAndExposesUntyped(t *testing.T) {
	db := new(mocks.MockDB)
	q := new(mocks.MockQuery)

	db.On("Model", mock.Anything).Return(q)
	q.On("Where", "SK", string(query.OpBetween), []any{"USER#1", "USER#9"}).Return(q)
	q.On("Filter", "Name", string(query.OpContains), "Ada").Return(q)
	q.On("Index", "gsi1").Return(q)
	q.On("Limit", 2).Return(q)
	q.On("Count").Return(int64(7), nil)

	users := typed.ModelOf[typedUser](db)
	built := users.Query().
		Between("SK", "USER#1", "USER#9").
		Filter("Name", query.OpContains, "Ada").
		Index("gsi1").
		Limit(2)

	require.Same(t, q, built.Untyped())
	count, err := built.Count()
	require.NoError(t, err)
	require.Equal(t, int64(7), count)

	db.AssertExpectations(t)
	q.AssertExpectations(t)
}

func TestTypedGetMissingErrorAndNilHelper(t *testing.T) {
	db := new(mocks.MockDB)
	q := new(mocks.MockQuery)

	db.On("Model", mock.Anything).Return(q).Times(2)
	q.On("BatchGet", []any{core.NewKeyPair("TENANT#1", "USER#404")}, mock.Anything).Return(nil).Times(2)

	users := typed.ModelOf[typedUser](db)
	key := users.Key("TENANT#1", "USER#404")

	_, err := users.Get(key)
	require.ErrorIs(t, err, theorydbErrors.ErrItemNotFound)

	got, err := users.GetOrNil(key)
	require.NoError(t, err)
	require.Nil(t, got)

	db.AssertExpectations(t)
	q.AssertExpectations(t)
}

func TestTypedGetAndDeleteKeyDelegateThroughTypedKeys(t *testing.T) {
	db := new(mocks.MockDB)
	q := new(mocks.MockQuery)
	key := core.NewKeyPair("TENANT#1", "USER#1")

	db.On("Model", mock.Anything).Return(q).Times(2)
	q.On("BatchGet", []any{key}, mock.Anything).Run(func(args mock.Arguments) {
		dest, ok := args.Get(1).(*[]typedUser)
		require.True(t, ok)
		*dest = []typedUser{{PK: "TENANT#1", SK: "USER#1", Name: "Ada"}}
	}).Return(nil).Once()
	q.On("BatchDelete", []any{key}).Return(nil).Once()

	users := typed.ModelOf[typedUser](db)
	got, err := users.Get(users.Key("TENANT#1", "USER#1"))
	require.NoError(t, err)
	require.Equal(t, "Ada", got.Name)
	require.NoError(t, users.DeleteKey(users.Key("TENANT#1", "USER#1")))

	db.AssertExpectations(t)
	q.AssertExpectations(t)
}

func TestTypedUnboundQueryWriteMethodsFailEarly(t *testing.T) {
	var q typed.Query[typedUser]

	require.ErrorContains(t, q.Create(), "not bound")
	require.ErrorContains(t, q.CreateOrUpdate(), "not bound")
	require.ErrorContains(t, q.Update("Name"), "not bound")
	require.ErrorContains(t, q.Delete(), "not bound")
	require.Nil(t, q.Untyped())
}

func TestTypedQueryFirstOrNilSuccessAndMissing(t *testing.T) {
	db := new(mocks.MockDB)
	q := new(mocks.MockQuery)

	db.On("Model", mock.Anything).Return(q).Times(2)
	q.On("First", mock.Anything).Run(func(args mock.Arguments) {
		dest, ok := args.Get(0).(*typedUser)
		require.True(t, ok)
		*dest = typedUser{PK: "TENANT#1", SK: "USER#1", Name: "Ada"}
	}).Return(nil).Once()
	q.On("First", mock.Anything).Return(theorydbErrors.ErrItemNotFound).Once()

	users := typed.ModelOf[typedUser](db)
	found, err := users.Query().FirstOrNil()
	require.NoError(t, err)
	require.NotNil(t, found)
	require.Equal(t, "Ada", found.Name)

	missing, err := users.Query().FirstOrNil()
	require.NoError(t, err)
	require.Nil(t, missing)

	db.AssertExpectations(t)
	q.AssertExpectations(t)
}

func TestTypedQueryAndGetReturnUnderlyingErrors(t *testing.T) {
	db := new(mocks.MockDB)
	q := new(mocks.MockQuery)
	boom := errors.New("boom")

	db.On("Model", mock.Anything).Return(q).Times(4)
	q.On("First", mock.Anything).Return(boom).Once()
	q.On("All", mock.Anything).Return(boom).Once()
	q.On("BatchGet", []any{core.NewKeyPair("TENANT#1", "USER#1")}, mock.Anything).Return(boom).Times(2)

	users := typed.ModelOf[typedUser](db)
	_, err := users.Query().First()
	require.ErrorIs(t, err, boom)

	_, err = users.Query().All()
	require.ErrorIs(t, err, boom)

	_, err = users.Get(users.Key("TENANT#1", "USER#1"))
	require.ErrorIs(t, err, boom)

	_, err = users.GetOrNil(users.Key("TENANT#1", "USER#1"))
	require.ErrorIs(t, err, boom)

	db.AssertExpectations(t)
	q.AssertExpectations(t)
}
