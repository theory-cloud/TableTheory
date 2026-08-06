package transaction

import (
	"testing"

	"github.com/aws/aws-sdk-go-v2/service/dynamodb/types"
	"github.com/stretchr/testify/require"

	"github.com/theory-cloud/tabletheory/v3/pkg/core"
	"github.com/theory-cloud/tabletheory/v3/pkg/model"
	pkgTypes "github.com/theory-cloud/tabletheory/v3/pkg/types"
)

// THE-2833: DynamoDB ExpressionAttributeValues are bound parameters, never
// interpolated into the expression string, so values containing SQL comment
// tokens (e.g. base64url WebAuthn credential IDs containing "--") must pass
// validation and flow through the transaction builder.
func TestBuilderBoundValueRegression_SQLCommentTokensAccepted(t *testing.T) {
	const credentialID = "m7WUcUKcPcWDKEsh_lGCqi3Imqt2jIODoBBdlXs2Ady4Ukj2EFY1Yn--fNuPDvHB"

	registry := model.NewRegistry()
	require.NoError(t, registry.Register(&User{}))
	builder := NewBuilder(nil, registry, pkgTypes.NewConverter())
	mockClient := newMockTransactClient(t, nil)
	builder.client = mockClient

	user := &User{ID: "user-webauthn", Name: credentialID, Email: credentialID}
	err := builder.
		Update(user, []string{"Name", "Email"},
			core.TransactCondition{Field: "Email", Operator: "=", Value: credentialID}).
		Execute()
	require.NoError(t, err)

	require.Len(t, mockClient.inputs, 1)
	require.Len(t, mockClient.inputs[0].TransactItems, 1)
	update := mockClient.inputs[0].TransactItems[0].Update
	require.NotNil(t, update)

	// The value must be bound via ExpressionAttributeValues, never
	// interpolated into the expression strings.
	require.NotNil(t, update.UpdateExpression)
	require.NotContains(t, *update.UpdateExpression, credentialID)
	if update.ConditionExpression != nil {
		require.NotContains(t, *update.ConditionExpression, credentialID)
	}

	bound := 0
	for _, av := range update.ExpressionAttributeValues {
		if s, ok := av.(*types.AttributeValueMemberS); ok && s.Value == credentialID {
			bound++
		}
	}
	require.Equal(t, 3, bound, "two SET values and one condition value must be bound")
}

// Attribute NAMES are interpolated into expressions and must stay guarded.
func TestBuilderBoundValueRegression_DangerousFieldNameRejected(t *testing.T) {
	registry := model.NewRegistry()
	require.NoError(t, registry.Register(&User{}))
	builder := NewBuilder(nil, registry, pkgTypes.NewConverter())
	builder.client = newMockTransactClient(t, nil)

	user := &User{ID: "user-1", Name: "safe"}
	err := builder.
		Update(user, []string{"Name"},
			core.TransactCondition{Field: "email'; DROP TABLE users; --", Operator: "=", Value: "x"}).
		Execute()
	require.Error(t, err)
	require.Contains(t, err.Error(), "security validation failed: InjectionAttempt")
}
