package integration

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/theory-cloud/tabletheory/v5"
	"github.com/theory-cloud/tabletheory/v5/pkg/core"
)

// v6LambdaPayment is a minimal model for the LambdaDB interface test.
type v6LambdaPayment struct {
	CreatedAt time.Time `dynamodb:"created_at"`
	ID        string    `theorydb:"pk" json:"id"`
	Status    string    `json:"status"`
	Amount    int64     `json:"amount"`
}

func (v6LambdaPayment) TableName() string { return "v6_lambda_payments_integration" }

// TestLambdaInitHandleSatisfiesCoreInterfacesAgainstDynamoDBLocal proves
// scenario (1) end to end: a real LambdaInit handle, asserted to
// core.ExtendedDB, runs a TransactWrite and a read against DynamoDB Local.
//
// LambdaInit builds its client from the ambient AWS environment, so the test
// points the DynamoDB SDK at DynamoDB Local through AWS_ENDPOINT_URL_DYNAMODB
// and dummy static credentials.
func TestLambdaInitHandleSatisfiesCoreInterfacesAgainstDynamoDBLocal(t *testing.T) {
	InitTestDB(t)

	t.Setenv("AWS_REGION", "us-east-1")
	t.Setenv("AWS_ACCESS_KEY_ID", "dummy")
	t.Setenv("AWS_SECRET_ACCESS_KEY", "dummy")
	t.Setenv("AWS_EC2_METADATA_DISABLED", "true")
	t.Setenv("AWS_ENDPOINT_URL_DYNAMODB", "http://localhost:8000")

	ldb, err := tabletheory.LambdaInit(&v6LambdaPayment{})
	require.NoError(t, err)
	require.NotNil(t, ldb)

	// Runtime assertion: the shape that failed at Lambda cold start.
	handle, ok := any(ldb).(core.ExtendedDB)
	require.True(t, ok, "LambdaInit handle %T must satisfy core.ExtendedDB", ldb)

	ctx, cancel := context.WithDeadline(context.Background(), time.Now().Add(30*time.Second))
	t.Cleanup(cancel)

	// Provision the table through the interface-typed handle.
	require.NoError(t, handle.EnsureTable(&v6LambdaPayment{}))

	// Derive the deadline-scoped handle through the interface.
	invocationDB := handle.WithLambdaTimeout(ctx)
	require.IsType(t, &tabletheory.LambdaDB{}, invocationDB)

	// TransactWrite through the interface-typed handle.
	require.NoError(t, handle.TransactWrite(ctx, func(tb core.TransactionBuilder) error {
		tb.Put(&v6LambdaPayment{
			ID:        "pay_1",
			Status:    "pending",
			Amount:    100,
			CreatedAt: time.Now().UTC().Truncate(time.Millisecond),
		})
		return nil
	}))

	// Read it back through the derived core.DB handle.
	var stored v6LambdaPayment
	require.NoError(t, invocationDB.Model(&v6LambdaPayment{}).Where("ID", "=", "pay_1").First(&stored))
	require.Equal(t, "pending", stored.Status)
	require.Equal(t, int64(100), stored.Amount)
}
