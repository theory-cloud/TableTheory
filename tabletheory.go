// Package tabletheory provides a type-safe ORM for Amazon DynamoDB in Go.
//
// Import path:
//
//	import "github.com/theory-cloud/tabletheory"
//
// Implementation lives in `internal/theorydb` so the repo root stays minimal.
package tabletheory

import (
	"context"

	"github.com/aws/aws-lambda-go/events"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb/types"

	internaltheorydb "github.com/theory-cloud/tabletheory/internal/theorydb"
	"github.com/theory-cloud/tabletheory/pkg/core"
	"github.com/theory-cloud/tabletheory/pkg/schema"
	"github.com/theory-cloud/tabletheory/pkg/session"
)

type (
	DB                = internaltheorydb.DB
	LambdaDB          = internaltheorydb.LambdaDB
	LambdaMemoryStats = internaltheorydb.LambdaMemoryStats
	ColdStartMetrics  = internaltheorydb.ColdStartMetrics

	MultiAccountDB = internaltheorydb.MultiAccountDB
	AccountConfig  = internaltheorydb.AccountConfig

	// Re-export types for convenience.
	Config            = session.Config
	AutoMigrateOption = schema.AutoMigrateOption
	BatchGetOptions   = core.BatchGetOptions
	KeyPair           = core.KeyPair
)

// Re-export AutoMigrate options for convenience.
var (
	WithBackupTable = schema.WithBackupTable
	WithDataCopy    = schema.WithDataCopy
	WithTargetModel = schema.WithTargetModel
	WithTransform   = schema.WithTransform
	WithBatchSize   = schema.WithBatchSize
)

func UnmarshalItem(item map[string]types.AttributeValue, dest interface{}) error {
	return internaltheorydb.UnmarshalItem(item, dest)
}

func UnmarshalItems(items []map[string]types.AttributeValue, dest interface{}) error {
	return internaltheorydb.UnmarshalItems(items, dest)
}

func UnmarshalStreamImage(streamImage map[string]events.DynamoDBAttributeValue, dest interface{}) error {
	return internaltheorydb.UnmarshalStreamImage(streamImage, dest)
}

func New(config session.Config) (core.ExtendedDB, error) {
	return internaltheorydb.New(config)
}

func NewBasic(config session.Config) (core.DB, error) {
	return internaltheorydb.NewBasic(config)
}

func NewKeyPair(partitionKey any, sortKey ...any) core.KeyPair {
	return internaltheorydb.NewKeyPair(partitionKey, sortKey...)
}

func DefaultBatchGetOptions() *core.BatchGetOptions {
	return internaltheorydb.DefaultBatchGetOptions()
}

func Condition(field, operator string, value any) core.TransactCondition {
	return internaltheorydb.Condition(field, operator, value)
}

func ConditionExpression(expression string, values map[string]any) core.TransactCondition {
	return internaltheorydb.ConditionExpression(expression, values)
}

func IfNotExists() core.TransactCondition {
	return internaltheorydb.IfNotExists()
}

func IfExists() core.TransactCondition {
	return internaltheorydb.IfExists()
}

func AtVersion(version int64) core.TransactCondition {
	return internaltheorydb.AtVersion(version)
}

func ConditionVersion(version int64) core.TransactCondition {
	return internaltheorydb.ConditionVersion(version)
}

func NewLambdaOptimized() (*LambdaDB, error) {
	return internaltheorydb.NewLambdaOptimized()
}

func IsLambdaEnvironment() bool {
	return internaltheorydb.IsLambdaEnvironment()
}

func GetLambdaMemoryMB() int {
	return internaltheorydb.GetLambdaMemoryMB()
}

func EnableXRayTracing() bool {
	return internaltheorydb.EnableXRayTracing()
}

func GetRemainingTimeMillis(ctx context.Context) int64 {
	return internaltheorydb.GetRemainingTimeMillis(ctx)
}

func LambdaInit(models ...any) (*LambdaDB, error) {
	return internaltheorydb.LambdaInit(models...)
}

func BenchmarkColdStart(models ...any) ColdStartMetrics {
	return internaltheorydb.BenchmarkColdStart(models...)
}

func NewMultiAccount(accounts map[string]AccountConfig) (*MultiAccountDB, error) {
	return internaltheorydb.NewMultiAccount(accounts)
}

func PartnerContext(ctx context.Context, partnerID string) context.Context {
	return internaltheorydb.PartnerContext(ctx, partnerID)
}

func GetPartnerFromContext(ctx context.Context) string {
	return internaltheorydb.GetPartnerFromContext(ctx)
}
