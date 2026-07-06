package fakedb_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb/types"
	"github.com/stretchr/testify/require"

	"github.com/theory-cloud/tabletheory"
	theorydberrors "github.com/theory-cloud/tabletheory/pkg/errors"
	"github.com/theory-cloud/tabletheory/pkg/session"
	"github.com/theory-cloud/tabletheory/pkg/testing/fakedb"
)

type fakeUser struct {
	CreatedAt time.Time `theorydb:"created_at" json:"createdAt"`
	UpdatedAt time.Time `theorydb:"updated_at" json:"updatedAt"`
	PK        string    `theorydb:"pk" json:"PK"`
	SK        string    `theorydb:"sk" json:"SK"`
	EmailHash string    `json:"emailHash"`
	Nickname  string    `json:"nickname,omitempty"`
	Version   int64     `theorydb:"version" json:"version"`
	TTL       int64     `theorydb:"ttl,omitempty" json:"ttl,omitempty"`
}

func (fakeUser) TableName() string { return "fake_users" }

func TestNewWithClientUsesStatefulFake(t *testing.T) {
	now := time.Date(2026, 7, 4, 0, 0, 0, 0, time.UTC)
	fake := fakedb.New()
	db, err := tabletheory.NewWithClient(session.Config{
		Region: "us-east-1",
		Now:    func() time.Time { return now },
	}, fake)
	require.NoError(t, err)

	require.NoError(t, db.CreateTable(&fakeUser{}))
	require.NoError(t, db.Model(&fakeUser{
		PK:        "USER#1",
		SK:        "PROFILE",
		EmailHash: "test@example",
		Nickname:  "one",
		TTL:       1_700_000_000,
	}).Create())

	var queried []fakeUser
	err = db.Model(&fakeUser{}).
		Where("PK", "=", "USER#1").
		Where("SK", "begins_with", "PRO").
		Filter("emailHash", "=", "test@example").
		All(&queried)
	require.NoError(t, err)
	require.Len(t, queried, 1)
	require.Equal(t, "one", queried[0].Nickname)
	require.Equal(t, int64(1_700_000_000), queried[0].TTL)
	require.Equal(t, now, queried[0].CreatedAt)
	require.Equal(t, int64(0), queried[0].Version)

	require.NoError(t, db.Model(&fakeUser{
		PK:       "USER#1",
		SK:       "PROFILE",
		Nickname: "two",
		Version:  0,
	}).Update("nickname"))

	err = db.Model(&fakeUser{
		PK:       "USER#1",
		SK:       "PROFILE",
		Nickname: "stale",
		Version:  0,
	}).Update("nickname")
	require.Error(t, err)
	require.True(t, errors.Is(err, theorydberrors.ErrVersionConflict) || errors.Is(err, theorydberrors.ErrConditionFailed))

	var got fakeUser
	require.NoError(t, db.Model(&fakeUser{}).Where("PK", "=", "USER#1").Where("SK", "=", "PROFILE").First(&got))
	require.Equal(t, "two", got.Nickname)
	require.Equal(t, int64(1), got.Version)
}

func TestNewWithClientRejectsNilClient(t *testing.T) {
	_, err := tabletheory.NewWithClient(session.Config{}, nil)
	require.ErrorContains(t, err, "DynamoDB client is nil")
}

func TestFakeSupportsAdminBatchScanAndTransactions(t *testing.T) {
	ctx := context.Background()
	fake := fakedb.New()
	tableName := "stateful_fake"

	_, err := fake.CreateTable(ctx, &dynamodb.CreateTableInput{
		TableName: aws.String(tableName),
		KeySchema: []types.KeySchemaElement{
			{AttributeName: aws.String("PK"), KeyType: types.KeyTypeHash},
			{AttributeName: aws.String("SK"), KeyType: types.KeyTypeRange},
		},
		GlobalSecondaryIndexes: []types.GlobalSecondaryIndex{
			{
				IndexName: aws.String("gsi1"),
				KeySchema: []types.KeySchemaElement{
					{AttributeName: aws.String("GPK"), KeyType: types.KeyTypeHash},
					{AttributeName: aws.String("GSK"), KeyType: types.KeyTypeRange},
				},
			},
		},
	})
	require.NoError(t, err)

	_, err = fake.CreateTable(ctx, &dynamodb.CreateTableInput{TableName: aws.String(tableName)})
	require.ErrorAs(t, err, new(*types.ResourceInUseException))

	_, err = fake.UpdateTimeToLive(ctx, &dynamodb.UpdateTimeToLiveInput{
		TableName: aws.String(tableName),
		TimeToLiveSpecification: &types.TimeToLiveSpecification{
			AttributeName: aws.String("ttl"),
			Enabled:       aws.Bool(true),
		},
	})
	require.NoError(t, err)

	backup, err := fake.CreateBackup(ctx, &dynamodb.CreateBackupInput{
		TableName:  aws.String(tableName),
		BackupName: aws.String("unit"),
	})
	require.NoError(t, err)
	require.Equal(t, types.BackupStatusAvailable, backup.BackupDetails.BackupStatus)

	_, err = fake.UpdateTable(ctx, &dynamodb.UpdateTableInput{
		TableName: aws.String(tableName),
		GlobalSecondaryIndexUpdates: []types.GlobalSecondaryIndexUpdate{
			{
				Create: &types.CreateGlobalSecondaryIndexAction{
					IndexName: aws.String("gsi2"),
					KeySchema: []types.KeySchemaElement{
						{AttributeName: aws.String("AltPK"), KeyType: types.KeyTypeHash},
					},
				},
			},
		},
	})
	require.NoError(t, err)

	require.NoError(t, fake.Seed(tableName,
		fakeItem("USER#1", "A", "one", "10", "blue", "G#1", "001"),
		fakeItem("USER#1", "B", "two", "20", "green", "G#1", "002"),
		fakeItem("USER#2", "A", "three", "30", "blue", "G#2", "001"),
	))
	require.Len(t, fake.Items(tableName), 3)

	tables, err := fake.ListTables(ctx, &dynamodb.ListTablesInput{Limit: aws.Int32(1)})
	require.NoError(t, err)
	require.Equal(t, []string{tableName}, tables.TableNames)

	got, err := fake.GetItem(ctx, &dynamodb.GetItemInput{
		TableName:            aws.String(tableName),
		Key:                  fakeKey("USER#1", "A"),
		ProjectionExpression: aws.String("PK, #n"),
		ExpressionAttributeNames: map[string]string{
			"#n": "name",
		},
	})
	require.NoError(t, err)
	require.Equal(t, avS("one"), got.Item["name"])
	require.Nil(t, got.Item["score"])

	scanned, err := fake.Scan(ctx, &dynamodb.ScanInput{
		TableName:        aws.String(tableName),
		FilterExpression: aws.String("contains(#tags, :tag) OR #score BETWEEN :lo AND :hi"),
		ExpressionAttributeNames: map[string]string{
			"#score": "score",
			"#tags":  "tags",
		},
		ExpressionAttributeValues: map[string]types.AttributeValue{
			":hi":  avN("25"),
			":lo":  avN("15"),
			":tag": avS("blue"),
		},
		Limit: aws.Int32(2),
	})
	require.NoError(t, err)
	require.Len(t, scanned.Items, 2)
	require.NotEmpty(t, scanned.LastEvaluatedKey)

	counted, err := fake.Scan(ctx, &dynamodb.ScanInput{
		TableName: aws.String(tableName),
		Select:    types.SelectCount,
	})
	require.NoError(t, err)
	require.Empty(t, counted.Items)
	require.Equal(t, int32(3), counted.ScannedCount)

	gsiPage, err := fake.Query(ctx, &dynamodb.QueryInput{
		TableName:              aws.String(tableName),
		IndexName:              aws.String("gsi1"),
		KeyConditionExpression: aws.String("#gpk = :gpk AND #gsk >= :min"),
		ExpressionAttributeNames: map[string]string{
			"#gpk": "GPK",
			"#gsk": "GSK",
		},
		ExpressionAttributeValues: map[string]types.AttributeValue{
			":gpk": avS("G#1"),
			":min": avS("001"),
		},
		ScanIndexForward: aws.Bool(false),
		Limit:            aws.Int32(1),
	})
	require.NoError(t, err)
	require.Equal(t, avS("B"), gsiPage.Items[0]["SK"])
	require.NotEmpty(t, gsiPage.LastEvaluatedKey)

	nextGSIPage, err := fake.Query(ctx, &dynamodb.QueryInput{
		TableName:              aws.String(tableName),
		IndexName:              aws.String("gsi1"),
		KeyConditionExpression: aws.String("#gpk = :gpk AND #gsk >= :min"),
		ExpressionAttributeNames: map[string]string{
			"#gpk": "GPK",
			"#gsk": "GSK",
		},
		ExpressionAttributeValues: map[string]types.AttributeValue{
			":gpk": avS("G#1"),
			":min": avS("001"),
		},
		ExclusiveStartKey: gsiPage.LastEvaluatedKey,
		ScanIndexForward:  aws.Bool(false),
	})
	require.NoError(t, err)
	require.Equal(t, avS("A"), nextGSIPage.Items[0]["SK"])

	_, err = fake.BatchWriteItem(ctx, &dynamodb.BatchWriteItemInput{
		RequestItems: map[string][]types.WriteRequest{
			tableName: {
				{PutRequest: &types.PutRequest{Item: fakeItem("USER#1", "C", "four", "40", "red", "G#1", "003")}},
				{DeleteRequest: &types.DeleteRequest{Key: fakeKey("USER#1", "B")}},
			},
		},
	})
	require.NoError(t, err)

	batch, err := fake.BatchGetItem(ctx, &dynamodb.BatchGetItemInput{
		RequestItems: map[string]types.KeysAndAttributes{
			tableName: {
				Keys: []map[string]types.AttributeValue{
					fakeKey("USER#1", "A"),
					fakeKey("USER#1", "C"),
				},
			},
		},
	})
	require.NoError(t, err)
	require.Len(t, batch.Responses[tableName], 2)

	updated, err := fake.UpdateItem(ctx, &dynamodb.UpdateItemInput{
		TableName: aws.String(tableName),
		Key:       fakeKey("USER#1", "C"),
		UpdateExpression: aws.String(
			"SET #note = if_not_exists(#note, :note), #score = #score + :inc REMOVE #tags DELETE #unused :unused",
		),
		ExpressionAttributeNames: map[string]string{
			"#note":   "note",
			"#score":  "score",
			"#tags":   "tags",
			"#unused": "unused",
		},
		ExpressionAttributeValues: map[string]types.AttributeValue{
			":inc":    avN("2"),
			":note":   avS("seeded"),
			":unused": avS("unused"),
		},
		ReturnValues: types.ReturnValueAllNew,
	})
	require.NoError(t, err)
	require.Equal(t, avN("42"), updated.Attributes["score"])
	require.Equal(t, avS("seeded"), updated.Attributes["note"])
	require.Nil(t, updated.Attributes["tags"])

	transactRead, err := fake.TransactGetItems(ctx, &dynamodb.TransactGetItemsInput{
		TransactItems: []types.TransactGetItem{
			{Get: &types.Get{TableName: aws.String(tableName), Key: fakeKey("USER#1", "C")}},
			{Get: &types.Get{TableName: aws.String(tableName), Key: fakeKey("MISSING", "A")}},
		},
	})
	require.NoError(t, err)
	require.Len(t, transactRead.Responses, 2)
	require.NotNil(t, transactRead.Responses[0].Item)
	require.Nil(t, transactRead.Responses[1].Item)

	_, err = fake.TransactWriteItems(ctx, &dynamodb.TransactWriteItemsInput{
		TransactItems: []types.TransactWriteItem{
			{Put: &types.Put{TableName: aws.String(tableName), Item: fakeItem("TX#1", "A", "tx", "1", "tx", "G#9", "001")}},
			{ConditionCheck: &types.ConditionCheck{
				TableName:           aws.String(tableName),
				Key:                 fakeKey("USER#1", "C"),
				ConditionExpression: aws.String("attribute_exists(PK)"),
			}},
			{Update: &types.Update{
				TableName:                 aws.String(tableName),
				Key:                       fakeKey("TX#1", "A"),
				UpdateExpression:          aws.String("ADD #score :inc"),
				ExpressionAttributeNames:  map[string]string{"#score": "score"},
				ExpressionAttributeValues: map[string]types.AttributeValue{":inc": avN("1")},
			}},
			{Delete: &types.Delete{TableName: aws.String(tableName), Key: fakeKey("USER#2", "A")}},
		},
	})
	require.NoError(t, err)

	_, err = fake.TransactWriteItems(ctx, &dynamodb.TransactWriteItemsInput{
		TransactItems: []types.TransactWriteItem{
			{ConditionCheck: &types.ConditionCheck{
				TableName:           aws.String(tableName),
				Key:                 fakeKey("USER#1", "C"),
				ConditionExpression: aws.String("attribute_not_exists(PK)"),
			}},
		},
	})
	require.ErrorAs(t, err, new(*types.TransactionCanceledException))

	_, err = fake.DeleteItem(ctx, &dynamodb.DeleteItemInput{
		TableName:           aws.String(tableName),
		Key:                 fakeKey("USER#1", "A"),
		ConditionExpression: aws.String("attribute_exists(PK)"),
	})
	require.NoError(t, err)

	_, err = fake.DeleteTable(ctx, &dynamodb.DeleteTableInput{TableName: aws.String(tableName)})
	require.NoError(t, err)
	_, err = fake.DescribeTable(ctx, &dynamodb.DescribeTableInput{TableName: aws.String(tableName)})
	require.ErrorAs(t, err, new(*types.ResourceNotFoundException))

	fake.Reset()
	require.Empty(t, fake.Items(tableName))
}

func fakeItem(pk, sk, name, score, tag, gpk, gsk string) map[string]types.AttributeValue {
	return map[string]types.AttributeValue{
		"PK":    avS(pk),
		"SK":    avS(sk),
		"GPK":   avS(gpk),
		"GSK":   avS(gsk),
		"name":  avS(name),
		"score": avN(score),
		"tags":  &types.AttributeValueMemberSS{Value: []string{tag}},
		"ttl":   avN("1700000000"),
	}
}

func fakeKey(pk, sk string) map[string]types.AttributeValue {
	return map[string]types.AttributeValue{"PK": avS(pk), "SK": avS(sk)}
}

func avS(value string) types.AttributeValue {
	return &types.AttributeValueMemberS{Value: value}
}

func avN(value string) types.AttributeValue {
	return &types.AttributeValueMemberN{Value: value}
}
