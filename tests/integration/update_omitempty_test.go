package integration

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/theory-cloud/tabletheory/tests"
)

type UpdateOmitEmptyItem struct {
	Attributes           map[string]string `theorydb:"attr:attributes,omitempty"`
	ID                   string            `theorydb:"pk"`
	SK                   string            `theorydb:"sk"`
	EncryptedPaymentData string            `theorydb:"attr:encryptedPaymentData"`
	UpdateTimestamp      string            `theorydb:"attr:updateTimestamp"`
	ProcessorTokens      []string          `theorydb:"attr:processorTokens,omitempty"`
}

func (UpdateOmitEmptyItem) TableName() string { return "UpdateOmitEmptyItems" }

func TestUpdate_OmitEmptyDoesNotOverwriteEmptyCollections(t *testing.T) {
	tests.RequireDynamoDBLocal(t)

	testCtx := InitTestDB(t)
	testCtx.CreateTableIfNotExists(t, &UpdateOmitEmptyItem{})

	original := &UpdateOmitEmptyItem{
		ID:                   "pmt#1",
		SK:                   "token#1",
		ProcessorTokens:      []string{"tok_123"},
		Attributes:           map[string]string{"stripe": "tok_123"},
		EncryptedPaymentData: "enc_v1",
		UpdateTimestamp:      "ts_v1",
	}

	err := testCtx.DB.Model(original).Create()
	require.NoError(t, err)

	update := &UpdateOmitEmptyItem{
		ID:                   original.ID,
		SK:                   original.SK,
		ProcessorTokens:      []string{},          // empty-but-non-nil
		Attributes:           map[string]string{}, // empty-but-non-nil
		EncryptedPaymentData: "enc_v2",
		UpdateTimestamp:      "ts_v2",
	}

	err = testCtx.DB.Model(update).
		Where("ID", "=", original.ID).
		Where("SK", "=", original.SK).
		Update()
	require.NoError(t, err)

	var got UpdateOmitEmptyItem
	err = testCtx.DB.Model(&UpdateOmitEmptyItem{}).
		Where("ID", "=", original.ID).
		Where("SK", "=", original.SK).
		First(&got)
	require.NoError(t, err)

	assert.Equal(t, "enc_v2", got.EncryptedPaymentData)
	assert.Equal(t, "ts_v2", got.UpdateTimestamp)
	assert.Equal(t, []string{"tok_123"}, got.ProcessorTokens)
	assert.Equal(t, map[string]string{"stripe": "tok_123"}, got.Attributes)
}
