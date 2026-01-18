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

type encryptedTagGatingModel struct {
	PK     string `theorydb:"pk" json:"pk"`
	SK     string `theorydb:"sk" json:"sk"`
	Secret string `theorydb:"encrypted" json:"secret"`
}

func (encryptedTagGatingModel) TableName() string {
	return "EncryptedTagGatingModels"
}

func TestEncryptedTagFailsClosedWithoutKMSKeyARN(t *testing.T) {
	httpClient := newCapturingHTTPClient(nil)
	stubSessionConfigLoad(t, func(context.Context, ...func(*config.LoadOptions) error) (aws.Config, error) {
		return minimalAWSConfig(httpClient), nil
	})

	dbAny, err := New(session.Config{Region: "us-east-1"})
	require.NoError(t, err)

	db := mustDB(t, dbAny)

	t.Run("CreateOrUpdate", func(t *testing.T) {
		err := db.Model(&encryptedTagGatingModel{
			PK:     "pk1",
			SK:     "sk1",
			Secret: "top-secret",
		}).CreateOrUpdate()
		require.ErrorIs(t, err, customerrors.ErrEncryptionNotConfigured)
	})

	t.Run("Update", func(t *testing.T) {
		err := db.Model(&encryptedTagGatingModel{
			PK:     "pk2",
			SK:     "sk2",
			Secret: "top-secret",
		}).Update("Secret")
		require.ErrorIs(t, err, customerrors.ErrEncryptionNotConfigured)
	})

	t.Run("UpdateBuilder", func(t *testing.T) {
		builder := db.Model(&encryptedTagGatingModel{}).
			Where("PK", "=", "pk3").
			Where("SK", "=", "sk3").
			UpdateBuilder()

		err := builder.Set("Secret", "top-secret").Execute()
		require.ErrorIs(t, err, customerrors.ErrEncryptionNotConfigured)
	})
}
