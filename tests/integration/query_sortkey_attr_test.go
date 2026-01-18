package integration

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

type notificationRecord struct {
	CreatedAt time.Time `theorydb:"attr:createdAt"`
	PK        string    `theorydb:"pk,attr:PK"`
	SK        string    `theorydb:"sk,attr:SK"`
	Type      string    `theorydb:"attr:type"`
	Message   string    `theorydb:"attr:message"`
}

func (notificationRecord) TableName() string {
	return "IntegrationNotificationsAttr"
}

func TestQueryWithCustomAttributeSortKeyBeginsWith(t *testing.T) {
	testCtx := InitTestDB(t)

	testCtx.CreateTableIfNotExists(t, &notificationRecord{})

	now := time.Now()
	fixtures := []notificationRecord{
		{PK: "USER#admin", SK: "NOTIF#2024-03", Type: "system", Message: "Third notice", CreatedAt: now.Add(-3 * time.Minute)},
		{PK: "USER#admin", SK: "NOTIF#2024-02", Type: "system", Message: "Second notice", CreatedAt: now.Add(-2 * time.Minute)},
		{PK: "USER#admin", SK: "NOTIF#2024-01", Type: "system", Message: "First notice", CreatedAt: now.Add(-1 * time.Minute)},
		{PK: "USER#guest", SK: "NOTIF#2024-99", Type: "system", Message: "Other user", CreatedAt: now},
	}

	for _, record := range fixtures {
		item := record // capture range variable
		require.NoError(t, testCtx.DB.Model(&item).Create())
	}

	var rows []notificationRecord
	err := testCtx.DB.Model(&notificationRecord{}).
		Where("PK", "=", "USER#admin").
		Where("SK", "begins_with", "NOTIF#").
		OrderBy("SK", "DESC").
		Limit(6).
		All(&rows)

	require.NoError(t, err, "expected DynamoDB query to succeed when using begins_with on a custom attribute sort key")
	require.Len(t, rows, 3)
	require.Equal(t, "NOTIF#2024-03", rows[0].SK)
	require.Equal(t, "NOTIF#2024-02", rows[1].SK)
	require.Equal(t, "NOTIF#2024-01", rows[2].SK)

	count, err := testCtx.DB.Model(&notificationRecord{}).
		Where("PK", "=", "USER#admin").
		Where("SK", "begins_with", "NOTIF#").
		Count()

	require.NoError(t, err, "expected Count() path to treat begins_with as a key condition")
	require.Equal(t, int64(3), count)
}
