package theorydb

import (
	"context"
	"testing"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/stretchr/testify/require"

	customerrors "github.com/theory-cloud/tabletheory/pkg/errors"
	"github.com/theory-cloud/tabletheory/pkg/session"
)

func TestEncryptedTag_RejectsEncryptedFieldConditions(t *testing.T) {
	httpClient := newCapturingHTTPClient(nil)
	stubSessionConfigLoad(t, func(context.Context, ...func(*config.LoadOptions) error) (aws.Config, error) {
		return minimalAWSConfig(httpClient), nil
	})

	dbAny, err := New(session.Config{
		Region:    "us-east-1",
		KMSKeyARN: "arn:aws:kms:us-east-1:111111111111:key/test",
	})
	require.NoError(t, err)

	db := mustDB(t, dbAny)

	t.Run("Where", func(t *testing.T) {
		var out []encryptedTagGatingModel
		err := db.Model(&encryptedTagGatingModel{}).
			Where("Secret", "=", "top-secret").
			All(&out)
		require.ErrorIs(t, err, customerrors.ErrEncryptedFieldNotQueryable)
	})

	t.Run("Filter", func(t *testing.T) {
		var out []encryptedTagGatingModel
		err := db.Model(&encryptedTagGatingModel{}).
			Filter("Secret", "=", "top-secret").
			All(&out)
		require.ErrorIs(t, err, customerrors.ErrEncryptedFieldNotQueryable)
	})

	t.Run("WithCondition", func(t *testing.T) {
		err := db.Model(&encryptedTagGatingModel{
			PK: "pk1",
			SK: "sk1",
		}).
			WithCondition("Secret", "=", "top-secret").
			CreateOrUpdate()
		require.ErrorIs(t, err, customerrors.ErrEncryptedFieldNotQueryable)
	})

	t.Run("UpdateBuilder Condition", func(t *testing.T) {
		builder := db.Model(&encryptedTagGatingModel{}).
			Where("PK", "=", "pk2").
			Where("SK", "=", "sk2").
			UpdateBuilder()

		err := builder.
			Condition("Secret", "=", "top-secret").
			Set("Secret", "builder-secret").
			Execute()
		require.ErrorIs(t, err, customerrors.ErrEncryptedFieldNotQueryable)
	})
}
