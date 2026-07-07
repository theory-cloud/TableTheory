package transaction

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/theory-cloud/tabletheory/v2/pkg/core"
	customerrors "github.com/theory-cloud/tabletheory/v2/pkg/errors"
	"github.com/theory-cloud/tabletheory/v2/pkg/model"
	"github.com/theory-cloud/tabletheory/v2/pkg/session"
	pkgTypes "github.com/theory-cloud/tabletheory/v2/pkg/types"
)

// Test models
type User struct {
	CreatedAt time.Time `theorydb:"created_at"`
	UpdatedAt time.Time `theorydb:"updated_at"`
	ID        string    `theorydb:"pk"`
	Email     string
	Name      string
	Balance   float64
	Version   int `theorydb:"version"`
}

type Account struct {
	UpdatedAt   time.Time `theorydb:"updated_at"`
	AccountID   string    `theorydb:"pk"`
	UserID      string    `theorydb:"sk"`
	AccountType string
	Balance     float64
	Version     int `theorydb:"version"`
}

type Order struct {
	CreatedAt  time.Time `theorydb:"created_at"`
	OrderID    string    `theorydb:"pk"`
	CustomerID string
	Status     string
	Total      float64
}

type SecretRecord struct {
	ID     string `theorydb:"pk"`
	Secret string `theorydb:"encrypted"`
}

type mockTransactGetClient struct {
	input *dynamodb.TransactGetItemsInput
	out   *dynamodb.TransactGetItemsOutput
	err   error
}

func (m *mockTransactGetClient) TransactGetItems(ctx context.Context, params *dynamodb.TransactGetItemsInput, optFns ...func(*dynamodb.Options)) (*dynamodb.TransactGetItemsOutput, error) {
	_ = ctx
	_ = optFns
	m.input = params
	return m.out, m.err
}

func setupTest(t *testing.T) (*Transaction, *model.Registry) {
	// Skip if no test endpoint is set
	if testing.Short() {
		t.Skip("Skipping integration test")
	}

	// Create test session
	sess, err := session.NewSession(&session.Config{
		Region:   "us-east-1",
		Endpoint: "http://localhost:8000",
	})
	require.NoError(t, err)

	// Create registry and register models
	registry := model.NewRegistry()
	err = registry.Register(&User{})
	require.NoError(t, err)
	err = registry.Register(&Account{})
	require.NoError(t, err)
	err = registry.Register(&Order{})
	require.NoError(t, err)

	// Create converter
	converter := pkgTypes.NewConverter()

	// Create transaction
	tx := NewTransaction(sess, registry, converter)
	return tx, registry
}

func TestTransactionCreate(t *testing.T) {
	tx, _ := setupTest(t)

	t.Run("AddCreateToTransaction", func(t *testing.T) {
		user := &User{
			ID:      "user-1",
			Email:   "test@example.com",
			Name:    "Test User",
			Balance: 100.0,
		}

		err := tx.Create(user)
		assert.NoError(t, err)
		assert.Len(t, tx.writes, 1)

		// Check the write item
		writeItem := tx.writes[0]
		assert.NotNil(t, writeItem.Put)
		assert.Equal(t, "Users", *writeItem.Put.TableName)
		assert.NotNil(t, writeItem.Put.ConditionExpression)
		assert.Contains(t, *writeItem.Put.ConditionExpression, "attribute_not_exists")
	})

	t.Run("MultipleCreates", func(t *testing.T) {
		tx = &Transaction{
			session:   tx.session,
			registry:  tx.registry,
			converter: tx.converter,
			writes:    make([]types.TransactWriteItem, 0),
		}

		user1 := &User{ID: "user-1", Name: "User 1"}
		user2 := &User{ID: "user-2", Name: "User 2"}
		order := &Order{OrderID: "order-1", CustomerID: "user-1", Total: 50.0}

		err := tx.Create(user1)
		assert.NoError(t, err)
		err = tx.Create(user2)
		assert.NoError(t, err)
		err = tx.Create(order)
		assert.NoError(t, err)

		assert.Len(t, tx.writes, 3)
	})
}

func TestTransactionUpdate(t *testing.T) {
	tx, _ := setupTest(t)

	t.Run("AddUpdateToTransaction", func(t *testing.T) {
		user := &User{
			ID:      "user-1",
			Email:   "updated@example.com",
			Name:    "Updated User",
			Balance: 150.0,
			Version: 1,
		}

		err := tx.Update(user)
		assert.NoError(t, err)
		assert.Len(t, tx.writes, 1)

		// Check the write item
		writeItem := tx.writes[0]
		assert.NotNil(t, writeItem.Update)
		assert.Equal(t, "Users", *writeItem.Update.TableName)
		assert.NotNil(t, writeItem.Update.UpdateExpression)
		assert.Contains(t, *writeItem.Update.UpdateExpression, "SET")

		// Should have version condition
		assert.NotNil(t, writeItem.Update.ConditionExpression)
		assert.Contains(t, *writeItem.Update.ConditionExpression, "#ver = :currentVer")
	})

	t.Run("UpdateWithoutVersion", func(t *testing.T) {
		tx = &Transaction{
			session:   tx.session,
			registry:  tx.registry,
			converter: tx.converter,
			writes:    make([]types.TransactWriteItem, 0),
		}

		order := &Order{
			OrderID:    "order-1",
			CustomerID: "user-1",
			Status:     "SHIPPED",
		}

		err := tx.Update(order)
		assert.NoError(t, err)
		assert.Len(t, tx.writes, 1)

		// Should not have version condition
		writeItem := tx.writes[0]
		assert.NotNil(t, writeItem.Update)
		assert.Empty(t, writeItem.Update.ConditionExpression)
	})
}

func TestTransactionDelete(t *testing.T) {
	tx, _ := setupTest(t)

	t.Run("AddDeleteToTransaction", func(t *testing.T) {
		user := &User{
			ID:      "user-1",
			Version: 2,
		}

		err := tx.Delete(user)
		assert.NoError(t, err)
		assert.Len(t, tx.writes, 1)

		// Check the write item
		writeItem := tx.writes[0]
		assert.NotNil(t, writeItem.Delete)
		assert.Equal(t, "Users", *writeItem.Delete.TableName)

		// Should have version condition
		assert.NotNil(t, writeItem.Delete.ConditionExpression)
		assert.Contains(t, *writeItem.Delete.ConditionExpression, "#ver = :ver")
	})

	t.Run("DeleteWithoutVersion", func(t *testing.T) {
		tx = &Transaction{
			session:   tx.session,
			registry:  tx.registry,
			converter: tx.converter,
			writes:    make([]types.TransactWriteItem, 0),
		}

		order := &Order{
			OrderID: "order-1",
		}

		err := tx.Delete(order)
		assert.NoError(t, err)
		assert.Len(t, tx.writes, 1)

		// Should not have condition
		writeItem := tx.writes[0]
		assert.NotNil(t, writeItem.Delete)
		assert.Nil(t, writeItem.Delete.ConditionExpression)
	})
}

func TestTransactionGet(t *testing.T) {
	tx, _ := setupTest(t)

	t.Run("AddGetToTransaction", func(t *testing.T) {
		user := &User{ID: "user-1"}
		var result User

		err := tx.Get(user, &result)
		assert.NoError(t, err)
		assert.Len(t, tx.reads, 1)

		// Check the read item
		readItem := tx.reads[0]
		assert.NotNil(t, readItem.Get)
		assert.Equal(t, "Users", *readItem.Get.TableName)
		assert.Contains(t, readItem.Get.Key, "id")
	})

	t.Run("MultipleGets", func(t *testing.T) {
		tx = &Transaction{
			session:   tx.session,
			registry:  tx.registry,
			converter: tx.converter,
			reads:     make([]types.TransactGetItem, 0),
		}

		user := &User{ID: "user-1"}
		order := &Order{OrderID: "order-1"}
		var userResult User
		var orderResult Order

		err := tx.Get(user, &userResult)
		assert.NoError(t, err)
		err = tx.Get(order, &orderResult)
		assert.NoError(t, err)

		assert.Len(t, tx.reads, 2)
	})
}

func TestTransactionMixed(t *testing.T) {
	tx, _ := setupTest(t)

	t.Run("MixedOperations", func(t *testing.T) {
		// Add various operations
		createUser := &User{ID: "user-new", Name: "New User", Balance: 100}
		updateUser := &User{ID: "user-1", Balance: 200, Version: 1}
		deleteOrder := &Order{OrderID: "order-old"}
		getUser := &User{ID: "user-2"}
		var getUserResult User

		err := tx.Create(createUser)
		assert.NoError(t, err)
		err = tx.Update(updateUser)
		assert.NoError(t, err)
		err = tx.Delete(deleteOrder)
		assert.NoError(t, err)
		err = tx.Get(getUser, &getUserResult)
		assert.NoError(t, err)

		assert.Len(t, tx.writes, 3)
		assert.Len(t, tx.reads, 1)
	})
}

func TestTransactionRollback(t *testing.T) {
	tx, _ := setupTest(t)

	t.Run("RollbackClearsOperations", func(t *testing.T) {
		// Add some operations
		user := &User{ID: "user-1", Name: "Test"}
		err := tx.Create(user)
		assert.NoError(t, err)

		var result User
		err = tx.Get(user, &result)
		assert.NoError(t, err)

		assert.Len(t, tx.writes, 1)
		assert.Len(t, tx.reads, 1)

		// Rollback
		err = tx.Rollback()
		assert.NoError(t, err)

		// Operations should be cleared
		assert.Nil(t, tx.writes)
		assert.Nil(t, tx.reads)
		assert.Nil(t, tx.results)
	})
}

func TestExtractPrimaryKey(t *testing.T) {
	tx, registry := setupTest(t)

	t.Run("SimpleKey", func(t *testing.T) {
		user := &User{ID: "user-1"}
		metadata, err := registry.GetMetadata(user)
		require.NoError(t, err)

		key, err := tx.extractPrimaryKey(user, metadata)
		assert.NoError(t, err)
		assert.Len(t, key, 1)
		assert.Contains(t, key, "id")
	})

	t.Run("CompositeKey", func(t *testing.T) {
		account := &Account{
			AccountID: "acc-1",
			UserID:    "user-1",
		}
		metadata, err := registry.GetMetadata(account)
		require.NoError(t, err)

		key, err := tx.extractPrimaryKey(account, metadata)
		assert.NoError(t, err)
		assert.Len(t, key, 2)
		assert.Contains(t, key, "accountID")
		assert.Contains(t, key, "userID")
	})

	t.Run("MissingPartitionKey", func(t *testing.T) {
		user := &User{} // ID not set
		metadata, err := registry.GetMetadata(user)
		require.NoError(t, err)

		_, err = tx.extractPrimaryKey(user, metadata)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "partition key")
	})

	t.Run("MissingSortKey", func(t *testing.T) {
		account := &Account{
			AccountID: "acc-1",
			// UserID not set
		}
		metadata, err := registry.GetMetadata(account)
		require.NoError(t, err)

		_, err = tx.extractPrimaryKey(account, metadata)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "sort key")
	})
}

func TestMarshalItem(t *testing.T) {
	tx, registry := setupTest(t)

	t.Run("FullItem", func(t *testing.T) {
		user := &User{
			ID:        "user-1",
			Email:     "test@example.com",
			Name:      "Test User",
			Balance:   100.50,
			Version:   1,
			CreatedAt: time.Now(),
			UpdatedAt: time.Now(),
		}

		metadata, err := registry.GetMetadata(user)
		require.NoError(t, err)

		item, err := tx.marshalItem(user, metadata)
		assert.NoError(t, err)
		assert.Contains(t, item, "id")
		assert.Contains(t, item, "email")
		assert.Contains(t, item, "name")
		assert.Contains(t, item, "balance")
		assert.Contains(t, item, "version")
		assert.Contains(t, item, "createdAt")
		assert.Contains(t, item, "updatedAt")
	})

	t.Run("OmitEmpty", func(t *testing.T) {
		user := &User{
			ID:   "user-1",
			Name: "Test User",
			// Email and Balance are zero values
		}

		metadata, err := registry.GetMetadata(user)
		require.NoError(t, err)

		// Simulate omitempty on Email field
		if emailField, exists := metadata.Fields["Email"]; exists {
			emailField.OmitEmpty = true
		}

		item, err := tx.marshalItem(user, metadata)
		assert.NoError(t, err)
		assert.Contains(t, item, "id")
		assert.Contains(t, item, "name")
		// Balance should still be included (0 is valid)
		assert.Contains(t, item, "balance")
	})
}

func TestTransactionBuilderHappyPath(t *testing.T) {
	registry := model.NewRegistry()
	require.NoError(t, registry.Register(&User{}))
	require.NoError(t, registry.Register(&Order{}))

	converter := pkgTypes.NewConverter()
	builder := NewBuilder(nil, registry, converter)
	mockClient := newMockTransactClient(t, nil)
	builder.client = mockClient

	user := &User{ID: "user-xyz", Email: "demo@example.com", Name: "Demo"}
	update := &User{ID: "user-xyz", Balance: 42}
	order := &Order{OrderID: "order-99", Status: "pending"}

	err := builder.
		Create(user, core.TransactCondition{Field: "Email", Operator: "<>", Value: ""}).
		Update(update, []string{"Balance"}, core.TransactCondition{Field: "Balance", Operator: ">=", Value: 0}).
		Delete(order).
		ConditionCheck(order, core.TransactCondition{Field: "Status", Operator: "=", Value: "pending"}).
		Execute()

	require.NoError(t, err)
	require.Equal(t, 1, mockClient.callCount)
	require.Len(t, mockClient.inputs[0].TransactItems, 4)

	put := mockClient.inputs[0].TransactItems[0].Put
	require.NotNil(t, put)
	assert.Contains(t, aws.ToString(put.ConditionExpression), "attribute_not_exists")
}

func TestTransactionBuilderConditionFailure(t *testing.T) {
	registry := model.NewRegistry()
	require.NoError(t, registry.Register(&User{}))
	converter := pkgTypes.NewConverter()
	builder := NewBuilder(nil, registry, converter)

	cancel := &types.TransactionCanceledException{
		CancellationReasons: []types.CancellationReason{
			{
				Code:    aws.String("ConditionalCheckFailed"),
				Message: aws.String("duplicate user"),
			},
		},
	}

	builder.client = newMockTransactClient(t, cancel)

	err := builder.Create(&User{ID: "dupe"}).Execute()
	require.Error(t, err)
	assert.True(t, errors.Is(err, customerrors.ErrConditionFailed))

	var txErr *customerrors.TransactionError
	require.ErrorAs(t, err, &txErr)
	assert.Equal(t, "Create", txErr.Operation)
}

func TestTransactionBuilderRetriesOnConflict(t *testing.T) {
	registry := model.NewRegistry()
	require.NoError(t, registry.Register(&User{}))
	converter := pkgTypes.NewConverter()

	conflict := &types.TransactionCanceledException{
		CancellationReasons: []types.CancellationReason{
			{Code: aws.String("TransactionConflict")},
		},
	}

	builder := NewBuilder(nil, registry, converter)
	builder.client = newMockTransactClient(t, conflict, nil)

	err := builder.Put(&User{ID: "retry-user"}).Execute()
	require.NoError(t, err)
	mockClient, ok := builder.client.(*mockTransactClient)
	require.True(t, ok)
	assert.Equal(t, 2, mockClient.callCount)
}

func TestTransactionBuilderOperationLimit(t *testing.T) {
	registry := model.NewRegistry()
	require.NoError(t, registry.Register(&User{}))
	converter := pkgTypes.NewConverter()

	builder := NewBuilder(nil, registry, converter)
	for i := 0; i < maxTransactOperations; i++ {
		id := fmt.Sprintf("user-%d", i)
		builder.Put(&User{ID: id})
	}
	builder.Put(&User{ID: "overflow"})

	err := builder.Execute()
	require.Error(t, err)
	assert.ErrorIs(t, err, customerrors.ErrInvalidOperator)
	assert.Contains(t, err.Error(), "100")
}

func TestTransactGetBuildsItemsAndCollectsResponses(t *testing.T) {
	registry := model.NewRegistry()
	converter := pkgTypes.NewConverter()

	requests := []core.TransactGetRequest{
		{
			Model:      &User{},
			Key:        map[string]any{"ID": "user-1"},
			Projection: []string{"ID", "Email"},
			Dest:       &User{},
		},
		{
			Model: &User{},
			Key:   core.NewKeyPair("user-2"),
			Dest:  &User{},
		},
	}

	items, metas, err := buildTransactGetItems(registry, converter, requests)
	require.NoError(t, err)
	require.Len(t, items, 2)
	require.Len(t, metas, 2)
	require.NotNil(t, items[0].Get)
	assert.Equal(t, "Users", aws.ToString(items[0].Get.TableName))
	assert.Equal(t, "#p0, #p1", aws.ToString(items[0].Get.ProjectionExpression))
	assert.Equal(t, map[string]string{"#p0": "id", "#p1": "email"}, items[0].Get.ExpressionAttributeNames)
	assert.Equal(t, &types.AttributeValueMemberS{Value: "user-1"}, items[0].Get.Key["id"])

	results, err := collectTransactGetResults(
		context.Background(),
		nil,
		requests,
		metas,
		[]types.ItemResponse{
			{Item: map[string]types.AttributeValue{
				"id":      &types.AttributeValueMemberS{Value: "user-1"},
				"email":   &types.AttributeValueMemberS{Value: "one@example.test"},
				"version": &types.AttributeValueMemberN{Value: "7"},
			}},
		},
	)
	require.NoError(t, err)
	require.Len(t, results, 2)
	assert.True(t, results[0].Found)
	assert.False(t, results[1].Found)
	dest, ok := requests[0].Dest.(*User)
	require.True(t, ok)
	assert.Equal(t, "user-1", dest.ID)
	assert.Equal(t, "one@example.test", dest.Email)
	assert.Equal(t, 7, dest.Version)
}

func TestTransactGetWithClientExecutesRequest(t *testing.T) {
	registry := model.NewRegistry()
	converter := pkgTypes.NewConverter()
	requests := []core.TransactGetRequest{
		{Model: &User{}, Key: "user-1", Dest: &User{}},
	}
	items, metas, err := buildTransactGetItems(registry, converter, requests)
	require.NoError(t, err)

	client := &mockTransactGetClient{
		out: &dynamodb.TransactGetItemsOutput{
			Responses: []types.ItemResponse{
				{Item: map[string]types.AttributeValue{
					"id":    &types.AttributeValueMemberS{Value: "user-1"},
					"email": &types.AttributeValueMemberS{Value: "one@example.test"},
				}},
			},
		},
	}

	results, err := transactGetWithClient(context.Background(), client, nil, requests, metas, items)
	require.NoError(t, err)
	require.NotNil(t, client.input)
	assert.Len(t, client.input.TransactItems, 1)
	require.Len(t, results, 1)
	assert.True(t, results[0].Found)
}

func TestTransactGetExecutesAgainstDynamoClient(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "DynamoDB_20120810.TransactGetItems", r.Header.Get("X-Amz-Target"))
		w.Header().Set("Content-Type", "application/x-amz-json-1.0")
		_, err := w.Write([]byte(`{"Responses":[{"Item":{"id":{"S":"user-1"},"email":{"S":"one@example.test"}}}]}`))
		require.NoError(t, err)
	}))
	defer server.Close()

	sess, err := session.NewSession(&session.Config{
		Region:   "us-east-1",
		Endpoint: server.URL,
		AWSConfigOptions: []func(*config.LoadOptions) error{
			config.WithRegion("us-east-1"),
			config.WithCredentialsProvider(credentials.NewStaticCredentialsProvider("dummy", "dummy", "")),
		},
	})
	require.NoError(t, err)

	dest := &User{}
	results, err := TransactGet(
		context.Background(),
		sess,
		model.NewRegistry(),
		pkgTypes.NewConverter(),
		[]core.TransactGetRequest{{Model: &User{}, Key: "user-1", Dest: dest}},
	)
	require.NoError(t, err)
	require.Len(t, results, 1)
	assert.True(t, results[0].Found)
	assert.Equal(t, "user-1", dest.ID)
	assert.Equal(t, "one@example.test", dest.Email)
}

func TestTransactGetWithClientPropagatesErrors(t *testing.T) {
	want := errors.New("transact get failed")
	_, err := transactGetWithClient(
		context.Background(),
		&mockTransactGetClient{err: want},
		nil,
		nil,
		nil,
		nil,
	)
	assert.ErrorIs(t, err, want)
}

func TestTransactGetBuildsValidationErrors(t *testing.T) {
	registry := model.NewRegistry()
	converter := pkgTypes.NewConverter()

	_, _, err := buildTransactGetItems(registry, converter, []core.TransactGetRequest{{Key: "missing-model"}})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "model cannot be nil")

	_, _, err = buildTransactGetItems(registry, converter, []core.TransactGetRequest{{Model: &Account{}, Key: "pk-only"}})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "composite key requires")
}

func TestTransactGetValidatesInputs(t *testing.T) {
	registry := model.NewRegistry()
	converter := pkgTypes.NewConverter()

	_, err := TransactGet(context.Background(), nil, registry, converter, nil)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "at least one")

	requests := make([]core.TransactGetRequest, maxTransactGetItems+1)
	for i := range requests {
		requests[i] = core.TransactGetRequest{Model: &User{}, Key: "x"}
	}
	_, err = TransactGet(context.Background(), nil, registry, converter, requests)
	require.Error(t, err)
	assert.ErrorIs(t, err, customerrors.ErrInvalidOperator)

	_, err = TransactGet(context.Background(), nil, registry, converter, []core.TransactGetRequest{
		{Model: &User{}, Key: "x"},
	})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "session is not configured")

	_, err = TransactGet(context.Background(), &session.Session{}, nil, converter, []core.TransactGetRequest{
		{Model: &User{}, Key: "x"},
	})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "registry is not configured")

	_, err = TransactGet(context.Background(), &session.Session{}, registry, nil, []core.TransactGetRequest{
		{Model: &User{}, Key: "x"},
	})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "DynamoDB client is nil")

	_, err = TransactGet(context.Background(), &session.Session{}, registry, nil, []core.TransactGetRequest{
		{Model: &Account{}, Key: "pk-only"},
	})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "composite key requires")
}

func TestTransactGetKeyAndProjectionVariants(t *testing.T) {
	registry := model.NewRegistry()
	require.NoError(t, registry.Register(&User{}))
	require.NoError(t, registry.Register(&Account{}))
	converter := pkgTypes.NewConverter()

	userMeta, err := registry.GetMetadata(&User{})
	require.NoError(t, err)
	userKey, err := buildTransactGetKey(userMeta, converter, "user-primitive")
	require.NoError(t, err)
	assert.Equal(t, &types.AttributeValueMemberS{Value: "user-primitive"}, userKey["id"])

	accountMeta, err := registry.GetMetadata(&Account{})
	require.NoError(t, err)
	accountKey, err := buildTransactGetKey(
		accountMeta,
		converter,
		Account{AccountID: "acct-1", UserID: "user-1"},
	)
	require.NoError(t, err)
	assert.Equal(t, &types.AttributeValueMemberS{Value: "acct-1"}, accountKey["accountID"])
	assert.Equal(t, &types.AttributeValueMemberS{Value: "user-1"}, accountKey["userID"])

	accountKey, err = buildTransactGetKey(
		accountMeta,
		converter,
		map[string]any{"accountID": "acct-2", "userID": "user-2"},
	)
	require.NoError(t, err)
	assert.Equal(t, &types.AttributeValueMemberS{Value: "acct-2"}, accountKey["accountID"])
	assert.Equal(t, &types.AttributeValueMemberS{Value: "user-2"}, accountKey["userID"])

	_, err = resolveProjectionField(userMeta, "email")
	require.NoError(t, err)
	_, err = resolveProjectionField(userMeta, "missing")
	require.Error(t, err)

	value, ok := valueFromKeyMap(map[string]any{"ACCOUNTID": "acct-upper"}, accountMeta.PrimaryKey.PartitionKey)
	require.True(t, ok)
	assert.Equal(t, "acct-upper", value)
	_, ok = valueFromKeyMap(map[string]any{"other": "value"}, accountMeta.PrimaryKey.PartitionKey)
	assert.False(t, ok)

	_, err = keyFromValues(accountMeta, converter, "acct", nil, false)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "sort key")

	_, err = buildTransactGetKey(userMeta, converter, nil)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "key cannot be nil")

	var nilUser *User
	_, err = buildTransactGetKey(userMeta, converter, nilUser)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "pointer cannot be nil")
}

func TestTransactGetEncryptedMetadataFailsClosed(t *testing.T) {
	registry := model.NewRegistry()
	require.NoError(t, registry.Register(&SecretRecord{}))
	metadata, err := registry.GetMetadata(&SecretRecord{})
	require.NoError(t, err)

	err = decryptTransactGetItem(context.Background(), nil, metadata, map[string]types.AttributeValue{
		"secret": &types.AttributeValueMemberS{Value: "ciphertext"},
	})
	require.Error(t, err)
	assert.ErrorIs(t, err, customerrors.ErrEncryptionNotConfigured)
}

func TestTransactGetDecryptsFailClosedEnvelopeErrors(t *testing.T) {
	registry := model.NewRegistry()
	require.NoError(t, registry.Register(&SecretRecord{}))
	metadata, err := registry.GetMetadata(&SecretRecord{})
	require.NoError(t, err)

	sess, err := session.NewSession(&session.Config{
		Region:    "us-east-1",
		KMSKeyARN: "arn:aws:kms:us-east-1:111111111111:key/test",
		AWSConfigOptions: []func(*config.LoadOptions) error{
			config.WithRegion("us-east-1"),
			config.WithCredentialsProvider(credentials.NewStaticCredentialsProvider("dummy", "dummy", "")),
		},
	})
	require.NoError(t, err)

	err = decryptTransactGetItem(context.Background(), sess, metadata, map[string]types.AttributeValue{
		"id": &types.AttributeValueMemberS{Value: "record-1"},
	})
	require.NoError(t, err)

	err = decryptTransactGetItem(context.Background(), sess, metadata, map[string]types.AttributeValue{
		"secret": &types.AttributeValueMemberS{Value: "not-an-envelope"},
	})
	require.Error(t, err)
	assert.ErrorIs(t, err, customerrors.ErrInvalidEncryptedEnvelope)
}

func TestTransactionBuilderMissingKey(t *testing.T) {
	registry := model.NewRegistry()
	require.NoError(t, registry.Register(&User{}))
	converter := pkgTypes.NewConverter()

	builder := NewBuilder(nil, registry, converter)
	builder.client = newMockTransactClient(t, nil)

	builder.Delete(&User{})
	err := builder.Execute()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "partition key")
}

func TestTransactionBuilderUpdateWithBuilder(t *testing.T) {
	registry := model.NewRegistry()
	require.NoError(t, registry.Register(&User{}))
	converter := pkgTypes.NewConverter()

	builder := NewBuilder(nil, registry, converter)
	mockClient := newMockTransactClient(t, nil)
	builder.client = mockClient

	user := &User{ID: "builder-user", Balance: 10}
	err := builder.UpdateWithBuilder(user, func(ub core.UpdateBuilder) error {
		ub.Increment("Balance")
		return nil
	}, core.TransactCondition{Field: "Balance", Operator: ">=", Value: 0}).Execute()

	require.NoError(t, err)
	require.Equal(t, 1, mockClient.callCount)
	update := mockClient.inputs[0].TransactItems[0].Update
	require.NotNil(t, update)
	assert.Contains(t, aws.ToString(update.UpdateExpression), "ADD")
	assert.NotEmpty(t, aws.ToString(update.ConditionExpression))

	foundBalance := false
	for _, attr := range update.ExpressionAttributeNames {
		if attr == "balance" {
			foundBalance = true
			break
		}
	}
	assert.True(t, foundBalance, "condition should reference balance attribute")
}

type mockTransactClient struct {
	t         *testing.T
	responses []error
	inputs    []*dynamodb.TransactWriteItemsInput
	callCount int
}

func newMockTransactClient(t *testing.T, responses ...error) *mockTransactClient {
	return &mockTransactClient{
		t:         t,
		responses: responses,
	}
}

func (m *mockTransactClient) TransactWriteItems(ctx context.Context, params *dynamodb.TransactWriteItemsInput, optFns ...func(*dynamodb.Options)) (*dynamodb.TransactWriteItemsOutput, error) {
	m.callCount++
	m.inputs = append(m.inputs, params)

	if len(m.responses) >= m.callCount {
		if err := m.responses[m.callCount-1]; err != nil {
			return nil, err
		}
	}

	return &dynamodb.TransactWriteItemsOutput{}, nil
}
