package fakedb

import (
	"context"
	"testing"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb/types"
	"github.com/stretchr/testify/require"
)

func TestExpressionAndAttributeHelpers(t *testing.T) {
	item := map[string]types.AttributeValue{
		"blob":    &types.AttributeValueMemberB{Value: []byte("abc")},
		"blobSet": &types.AttributeValueMemberBS{Value: [][]byte{[]byte("abc")}},
		"flag":    &types.AttributeValueMemberBOOL{Value: true},
		"list": &types.AttributeValueMemberL{Value: []types.AttributeValue{
			&types.AttributeValueMemberS{Value: "needle"},
		}},
		"meta": &types.AttributeValueMemberM{Value: map[string]types.AttributeValue{
			"k": &types.AttributeValueMemberS{Value: "v"},
		}},
		"name":    &types.AttributeValueMemberS{Value: "one"},
		"nothing": &types.AttributeValueMemberNULL{Value: true},
		"nums":    &types.AttributeValueMemberNS{Value: []string{"7"}},
		"score":   &types.AttributeValueMemberN{Value: "7"},
		"tags":    &types.AttributeValueMemberSS{Value: []string{"blue"}},
	}
	names := map[string]string{
		"#blob":    "blob",
		"#blobSet": "blobSet",
		"#flag":    "flag",
		"#list":    "list",
		"#missing": "missing",
		"#name":    "name",
		"#nums":    "nums",
		"#score":   "score",
		"#tags":    "tags",
	}
	values := map[string]types.AttributeValue{
		":blob":    &types.AttributeValueMemberB{Value: []byte("abc")},
		":flag":    &types.AttributeValueMemberBOOL{Value: true},
		":max":     &types.AttributeValueMemberN{Value: "9"},
		":min":     &types.AttributeValueMemberN{Value: "1"},
		":needle":  &types.AttributeValueMemberS{Value: "needle"},
		":notName": &types.AttributeValueMemberS{Value: "two"},
		":num":     &types.AttributeValueMemberN{Value: "7"},
		":score":   &types.AttributeValueMemberN{Value: "7"},
		":tag":     &types.AttributeValueMemberS{Value: "blue"},
		":value":   &types.AttributeValueMemberS{Value: "one"},
	}

	for _, expr := range []string{
		"(#score >= :score AND (#name = :value OR #name = :notName))",
		"#score <> :min",
		"#score < :max",
		"#score <= :score",
		"#score > :min",
		"#name IN (:value, :notName)",
		"attribute_exists(#name)",
		"attribute_not_exists(#missing)",
		"begins_with(#name, :value)",
		"contains(#tags, :tag)",
		"contains(#nums, :num)",
		"contains(#list, :needle)",
		"#blob = :blob",
		"#flag = :flag",
	} {
		require.True(t, evalExpr(expr, item, names, values), expr)
	}
	require.False(t, evalExpr("contains(#blobSet, :blob)", item, names, values))
	require.False(t, evalExpr("contains(#list)", item, names, values))
	require.False(t, evalExpr("#name IN :value", item, names, values))
	require.False(t, evalExpr("#score BETWEEN :min", item, names, values))
	require.False(t, evalExpr("#name XOR :value", item, names, values))
}

func TestUpdateAndCloneHelpers(t *testing.T) {
	item := map[string]types.AttributeValue{
		"count": &types.AttributeValueMemberN{Value: "1"},
		"drop":  &types.AttributeValueMemberS{Value: "x"},
		"name":  &types.AttributeValueMemberS{Value: "one"},
		"set":   &types.AttributeValueMemberSS{Value: []string{"a"}},
	}
	names := map[string]string{
		"#count": "count",
		"#drop":  "drop",
		"#name":  "name",
		"#set":   "set",
	}
	values := map[string]types.AttributeValue{
		":fallback": &types.AttributeValueMemberS{Value: "fallback"},
		":inc":      &types.AttributeValueMemberN{Value: "2"},
		":set":      &types.AttributeValueMemberSS{Value: []string{"a"}},
	}

	err := applyUpdateExpression(
		item,
		aws.String("SET #name = if_not_exists(#name, :fallback), #count = #count + :inc REMOVE #drop DELETE #set :set"),
		names,
		values,
	)
	require.NoError(t, err)
	require.Equal(t, &types.AttributeValueMemberS{Value: "one"}, item["name"])
	require.Equal(t, &types.AttributeValueMemberN{Value: "3"}, item["count"])
	require.Nil(t, item["drop"])
	require.Nil(t, item["set"])

	err = applyUpdateExpression(item, aws.String("SET #name"), names, values)
	require.Error(t, err)
	_, err = evalUpdateValue("#count + :inc + :fallback", item, names, values)
	require.Error(t, err)
	require.Equal(t, "2", addNumberStrings("bad", "2"))

	cloned := cloneAV(&types.AttributeValueMemberM{Value: map[string]types.AttributeValue{
		"bs":   &types.AttributeValueMemberBS{Value: [][]byte{[]byte("x")}},
		"bool": &types.AttributeValueMemberBOOL{Value: false},
		"list": &types.AttributeValueMemberL{Value: []types.AttributeValue{
			&types.AttributeValueMemberNULL{Value: true},
		}},
	}})
	require.NotNil(t, cloned)
	require.Equal(t, 0, compareAV(nil, nil))
	require.Equal(t, -1, compareAV(nil, &types.AttributeValueMemberS{Value: "x"}))
	require.Equal(t, 1, compareAV(&types.AttributeValueMemberS{Value: "x"}, nil))
	require.Equal(t, 0, compareAV(&types.AttributeValueMemberB{Value: []byte("a")}, &types.AttributeValueMemberB{Value: []byte("a")}))
	require.Equal(t, -1, compareAV(&types.AttributeValueMemberBOOL{Value: false}, &types.AttributeValueMemberBOOL{Value: true}))
	require.NotEmpty(t, avKeyComponent(cloned))
	require.True(t, parensBalanced("(#a AND (#b))"))
	require.False(t, parensBalanced("(#a"))
}

func TestErrorBranchesAndScalarHelpers(t *testing.T) {
	ctx := context.Background()
	fake := New()
	_, err := fake.CreateTable(ctx, nil)
	require.Error(t, err)
	_, err = fake.CreateBackup(ctx, &dynamodb.CreateBackupInput{TableName: aws.String("missing")})
	require.Error(t, err)
	_, err = fake.UpdateTable(ctx, nil)
	require.Error(t, err)
	_, err = fake.UpdateTimeToLive(ctx, &dynamodb.UpdateTimeToLiveInput{TableName: aws.String("missing")})
	require.Error(t, err)
	_, err = fake.Query(ctx, nil)
	require.Error(t, err)
	_, err = fake.Scan(ctx, nil)
	require.Error(t, err)
	_, err = fake.BatchGetItem(ctx, nil)
	require.Error(t, err)
	_, err = fake.BatchWriteItem(ctx, nil)
	require.Error(t, err)

	require.Equal(t, int32(maxDynamoDBCount), safeInt32(int(maxDynamoDBCount)+1))
	tables := map[string]*tableState{}
	require.Same(t, tableOrDefault(tables, ""), tableOrDefault(tables, "default"))
	require.NotNil(t, tableDescription(&tableState{name: "pk_only", pk: "PK"}))

	require.NotZero(t, compareAV(&types.AttributeValueMemberS{Value: "x"}, &types.AttributeValueMemberN{Value: "1"}))
	require.NotZero(t, compareAV(&types.AttributeValueMemberN{Value: "bad"}, &types.AttributeValueMemberN{Value: "1"}))
	require.NotZero(t, compareAV(&types.AttributeValueMemberB{Value: []byte("b")}, &types.AttributeValueMemberB{Value: []byte("a")}))
	require.Zero(t, compareAV(&types.AttributeValueMemberBOOL{Value: true}, &types.AttributeValueMemberBOOL{Value: true}))
	require.Zero(t, compareAV(&types.AttributeValueMemberNULL{Value: true}, &types.AttributeValueMemberNULL{Value: true}))
	require.NotEmpty(t, stringValue(&types.AttributeValueMemberBOOL{Value: true}))
	require.Equal(t, &types.AttributeValueMemberS{Value: "fallback"}, addAV(nil, &types.AttributeValueMemberS{Value: "fallback"}))
	require.Equal(t, &types.AttributeValueMemberS{Value: "right"}, addAV(&types.AttributeValueMemberS{Value: "left"}, &types.AttributeValueMemberS{Value: "right"}))
	require.NotEmpty(t, avKeyComponent(&types.AttributeValueMemberBOOL{Value: true}))
	require.NotEmpty(t, avKeyComponent(&types.AttributeValueMemberNULL{Value: true}))
	require.NotEmpty(t, avKeyComponent(&types.AttributeValueMemberSS{Value: []string{"a"}}))
	require.NotEmpty(t, avKeyComponent(&types.AttributeValueMemberL{Value: []types.AttributeValue{&types.AttributeValueMemberS{Value: "a"}}}))
	require.Nil(t, cloneAV(nil))
}
