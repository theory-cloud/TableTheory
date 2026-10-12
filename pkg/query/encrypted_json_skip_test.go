package query

import (
	"testing"

	"github.com/aws/aws-sdk-go-v2/service/dynamodb/types"
	"github.com/stretchr/testify/require"

	customerrors "github.com/theory-cloud/tabletheory/v4/pkg/errors"
)

type encryptedJSONSkippedRecord struct {
	ID            string `theorydb:"pk" json:"id"`
	WebhookSecret string `theorydb:"encrypted" json:"-"`
}

func TestUnmarshalItemEncryptedFieldWithJSONDashFailsClosed(t *testing.T) {
	envelope := &types.AttributeValueMemberM{Value: map[string]types.AttributeValue{
		"v":     &types.AttributeValueMemberN{Value: "1"},
		"edk":   &types.AttributeValueMemberB{Value: []byte("edk")},
		"nonce": &types.AttributeValueMemberB{Value: []byte("nonce")},
		"ct":    &types.AttributeValueMemberB{Value: []byte("ct")},
	}}

	var out encryptedJSONSkippedRecord
	err := UnmarshalItem(map[string]types.AttributeValue{
		"id":            &types.AttributeValueMemberS{Value: "1"},
		"webhookSecret": envelope,
	}, &out)
	require.Error(t, err)
	require.ErrorIs(t, err, customerrors.ErrEncryptionNotConfigured)

	// Authorized-success control: a plaintext value on the same field decodes.
	var plaintext encryptedJSONSkippedRecord
	require.NoError(t, UnmarshalItem(map[string]types.AttributeValue{
		"id":            &types.AttributeValueMemberS{Value: "1"},
		"webhookSecret": &types.AttributeValueMemberS{Value: "plain"},
	}, &plaintext))
	require.Equal(t, "plain", plaintext.WebhookSecret)
}

func TestUnmarshalItemNonEncryptedJSONDashStillSkipped(t *testing.T) {
	type record struct {
		Hidden string `json:"-"`
		Name   string `json:"name"`
	}

	var out record
	require.NoError(t, UnmarshalItem(map[string]types.AttributeValue{
		"hidden": &types.AttributeValueMemberS{Value: "should-not-set"},
		"name":   &types.AttributeValueMemberS{Value: "visible"},
	}, &out))
	require.Empty(t, out.Hidden)
	require.Equal(t, "visible", out.Name)
}
