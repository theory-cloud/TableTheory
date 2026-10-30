package driver

import (
	"context"
	"crypto/aes"
	"crypto/cipher"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"reflect"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb/types"
	"github.com/aws/aws-sdk-go-v2/service/kms"
	kmsTypes "github.com/aws/aws-sdk-go-v2/service/kms/types"

	"github.com/theory-cloud/tabletheory/v6"
	"github.com/theory-cloud/tabletheory/v6/pkg/core"
	theorydbErrors "github.com/theory-cloud/tabletheory/v6/pkg/errors"
	"github.com/theory-cloud/tabletheory/v6/pkg/releasestate"
	"github.com/theory-cloud/tabletheory/v6/pkg/schema"
	"github.com/theory-cloud/tabletheory/v6/pkg/session"
	"github.com/theory-cloud/tabletheory/v6/pkg/testing/fakedb"
)

type ErrorCode string

type structuredProfileRecord struct {
	Source string `theorydb:"attr:source" json:"source"`
}

type structuredProfileUserRecordCarrier struct {
	PK      string                  `theorydb:"pk,attr:PK" json:"PK"`
	SK      string                  `theorydb:"sk,attr:SK" json:"SK"`
	Profile structuredProfileRecord `theorydb:"attr:profile,omitempty" json:"profile,omitempty"`
	Version int64                   `theorydb:"version,attr:version" json:"version"`
}

func (structuredProfileUserRecordCarrier) TableName() string { return "users_contract" }

const (
	ErrItemNotFound      ErrorCode = "ErrItemNotFound"
	ErrConditionFailed   ErrorCode = "ErrConditionFailed"
	ErrVersionConflict   ErrorCode = "ErrVersionConflict"
	ErrInvalidModel      ErrorCode = "ErrInvalidModel"
	ErrMissingPrimaryKey ErrorCode = "ErrMissingPrimaryKey"
	ErrInvalidOperator   ErrorCode = "ErrInvalidOperator"

	ErrEncryptionNotConfigured     ErrorCode = "ErrEncryptionNotConfigured"
	ErrEncryptedFieldNotQueryable  ErrorCode = "ErrEncryptedFieldNotQueryable"
	ErrInvalidEncryptedEnvelope    ErrorCode = "ErrInvalidEncryptedEnvelope"
	ErrMigrationEncryptionRequired ErrorCode = "ErrMigrationEncryptionRequired"

	ErrImmutableModelMutation          ErrorCode = "ErrImmutableModelMutation"
	ErrProtectedFieldMutation          ErrorCode = "ErrProtectedFieldMutation"
	ErrRejectedDeployAuthorityEvidence ErrorCode = "ErrRejectedDeployAuthorityEvidence"
)

type Driver interface {
	Capabilities() []string
	Create(ctx context.Context, model string, item map[string]any, ifNotExists bool) error
	Get(ctx context.Context, model string, key map[string]any) (map[string]any, error)
	GetOptional(ctx context.Context, model string, key map[string]any) (map[string]any, bool, error)
	Update(ctx context.Context, model string, item map[string]any, fields []string, protectedAttributes []string) error
	Save(ctx context.Context, model string, item map[string]any) error
	Delete(ctx context.Context, model string, key map[string]any) error
	Query(ctx context.Context, model string, req ReadRequest) (ReadResult, error)
	Scan(ctx context.Context, model string, req ReadRequest) (ReadResult, error)
	CountQuery(ctx context.Context, model string, req ReadRequest) (ReadResult, error)
	CountScan(ctx context.Context, model string, req ReadRequest) (ReadResult, error)
	TransactGet(ctx context.Context, model string, items []KeyedItem) (ReadResult, error)
	BatchGet(ctx context.Context, model string, keys []map[string]any) (ReadResult, error)
	BatchWrite(ctx context.Context, model string, puts []map[string]any, deletes []map[string]any) error
	TransactWrite(ctx context.Context, model string, actions []TransactWriteAction) error
	TransitionAppendEvent(ctx context.Context, actual TransitionActual, event TransitionEvent) error
	ValidateProvenance(ctx context.Context, model string, item map[string]any) error
	SetStepEncryption(mode string) error
	RawPut(ctx context.Context, table string, rawItem map[string]any) error
	SeedEncrypted(ctx context.Context, table string, item map[string]any, envelopeVersion int, corrupt string, reset bool) error
	Migrate(ctx context.Context, req MigrateRequest, targetTable string) error
}

type MigrateRequest struct {
	SourceTable string
	BatchSize   int
	Encryption  string
}

type ReadRequest struct {
	Index          string
	Partition      *ReadCondition
	Sort           *ReadCondition
	Filter         []ReadCondition
	SortDirection  string
	Limit          int
	Projection     []string
	Cursor         string
	ConsistentRead *bool
}

type ReadCondition struct {
	Attribute string
	Operator  string
	Value     any
	Values    []any
}

type ReadResult struct {
	Items  []map[string]any
	Cursor string
	Count  *int64
}

type KeyedItem struct {
	Model string
	Key   map[string]any
}

type TransactWriteAction struct {
	Kind                      string
	Model                     string
	Item                      map[string]any
	Key                       map[string]any
	Set                       map[string]any
	ConditionExpression       string
	ExpressionAttributeNames  map[string]string
	ExpressionAttributeValues map[string]any
	IfNotExists               bool
}

type TransitionActual struct {
	Model           string
	Key             map[string]any
	Set             map[string]any
	ExpectedVersion *int64
}

type TransitionEvent struct {
	Model   string
	Item    map[string]any
	Binding map[string]string
}

func MapError(err error) ErrorCode {
	codes := MapErrors(err)
	if len(codes) == 0 {
		return ""
	}
	return codes[0]
}

func MapErrors(err error) []ErrorCode {
	switch {
	case errors.Is(err, theorydbErrors.ErrItemNotFound):
		return []ErrorCode{ErrItemNotFound}
	case errors.Is(err, theorydbErrors.ErrVersionConflict):
		return []ErrorCode{ErrConditionFailed, ErrVersionConflict}
	case errors.Is(err, theorydbErrors.ErrConditionFailed):
		return []ErrorCode{ErrConditionFailed}
	case errors.Is(err, theorydbErrors.ErrInvalidModel):
		return []ErrorCode{ErrInvalidModel}
	case errors.Is(err, theorydbErrors.ErrMissingPrimaryKey):
		return []ErrorCode{ErrMissingPrimaryKey}
	case errors.Is(err, theorydbErrors.ErrInvalidOperator):
		return []ErrorCode{ErrInvalidOperator}
	case errors.Is(err, theorydbErrors.ErrEncryptionNotConfigured):
		return []ErrorCode{ErrEncryptionNotConfigured}
	case errors.Is(err, theorydbErrors.ErrEncryptedFieldNotQueryable):
		return []ErrorCode{ErrEncryptedFieldNotQueryable}
	case errors.Is(err, theorydbErrors.ErrInvalidEncryptedEnvelope):
		return []ErrorCode{ErrInvalidEncryptedEnvelope}
	case errors.Is(err, schema.ErrMigrationEncryptionRequired):
		return []ErrorCode{ErrMigrationEncryptionRequired}
	case errors.Is(err, theorydbErrors.ErrImmutableModelMutation):
		return []ErrorCode{ErrImmutableModelMutation}
	case errors.Is(err, theorydbErrors.ErrProtectedFieldMutation):
		return []ErrorCode{ErrProtectedFieldMutation}
	case errors.Is(err, theorydbErrors.ErrRejectedDeployAuthorityEvidence):
		return []ErrorCode{ErrRejectedDeployAuthorityEvidence}
	default:
		return nil
	}
}

type rawClient interface {
	PutItem(ctx context.Context, params *dynamodb.PutItemInput, optFns ...func(*dynamodb.Options)) (*dynamodb.PutItemOutput, error)
	GetItem(ctx context.Context, params *dynamodb.GetItemInput, optFns ...func(*dynamodb.Options)) (*dynamodb.GetItemOutput, error)
	DescribeTable(ctx context.Context, params *dynamodb.DescribeTableInput, optFns ...func(*dynamodb.Options)) (*dynamodb.DescribeTableOutput, error)
	CreateTable(ctx context.Context, params *dynamodb.CreateTableInput, optFns ...func(*dynamodb.Options)) (*dynamodb.CreateTableOutput, error)
	DeleteTable(ctx context.Context, params *dynamodb.DeleteTableInput, optFns ...func(*dynamodb.Options)) (*dynamodb.DeleteTableOutput, error)
}

type TheorydbDriver struct {
	db                      core.ExtendedDB
	deterministicEncryption bool

	// stepEncryption selects a database instance without KMS for steps that
	// prove fail-closed behavior; "" uses the scenario-configured instance.
	stepEncryption string
	noEncryptionDB core.ExtendedDB
	failingDB      core.ExtendedDB

	ddb  *dynamodb.Client
	raw  rawClient
	fake *fakedb.Fake

	kmsClient *deterministicKMSClient
	kmsRand   io.Reader

	endpoint    string
	region      string
	loadOptions []func(*config.LoadOptions) error
}

func (d *TheorydbDriver) Capabilities() []string {
	capabilities := []string{
		"crud",
		"omitempty",
		"lifecycle.timestamps",
		"optimistic_lock.version",
		"error.version_conflict",
		"ttl.epoch_seconds",
		"number.precision.exact",
		"type.matrix",
		"query.basic",
		"scan.basic",
		"count.native",
		"get.optional",
		"transact_get",
		"batch.get",
		"batch.write",
		"transact.write",
		"release_state.write_policy",
		"release_state.transactional_transition",
		"release_state.provenance_confidence",
		"encryption.fail_closed",
		"encryption.migration_rebinding",
	}
	if d.deterministicEncryption {
		capabilities = append(capabilities, "encryption.deterministic_interop")
	}
	return capabilities
}

type Options struct {
	Encryption EncryptionOptions
}

type EncryptionOptions struct {
	Provider string
	Seed     string
}

func NewTheorydbDriver(options ...Options) (*TheorydbDriver, error) {
	endpoint := os.Getenv("DYNAMODB_ENDPOINT")
	if endpoint == "" {
		endpoint = "http://localhost:8000"
	}
	region := os.Getenv("AWS_REGION")
	if region == "" {
		region = os.Getenv("AWS_DEFAULT_REGION")
	}
	if region == "" {
		region = "us-east-1"
	}

	loadOptions := []func(*config.LoadOptions) error{
		config.WithCredentialsProvider(credentials.NewStaticCredentialsProvider("dummy", "dummy", "")),
		config.WithRegion(region),
	}
	cfg := session.Config{
		Region:           region,
		Endpoint:         endpoint,
		AWSConfigOptions: loadOptions,
	}
	driver := &TheorydbDriver{
		endpoint:    endpoint,
		region:      region,
		loadOptions: loadOptions,
	}
	if len(options) > 0 && options[0].Encryption.Provider != "" {
		kmsClient, rng, err := deterministicEncryptionForOptions(options[0].Encryption)
		if err != nil {
			return nil, err
		}
		cfg.KMSKeyARN = "arn:aws:kms:us-east-1:111111111111:key/contract-deterministic"
		cfg.KMSClient = kmsClient
		cfg.EncryptionRand = rng
		driver.deterministicEncryption = true
		driver.kmsClient = kmsClient
		driver.kmsRand = rng
	}

	db, err := tabletheory.New(cfg)
	if err != nil {
		return nil, err
	}
	driver.db = db
	if !driver.deterministicEncryption {
		driver.noEncryptionDB = db
	}

	awsCfg, err := config.LoadDefaultConfig(context.Background(), loadOptions...)
	if err != nil {
		return nil, err
	}
	driver.ddb = dynamodb.NewFromConfig(awsCfg, func(o *dynamodb.Options) {
		o.BaseEndpoint = aws.String(endpoint)
	})
	driver.raw = driver.ddb

	return driver, nil
}

func NewFakeTheorydbDriver(options ...Options) (*TheorydbDriver, *fakedb.Fake, error) {
	cfg := session.Config{
		Region: "us-east-1",
	}
	deterministicEncryption := false
	var kmsClient *deterministicKMSClient
	var rng io.Reader
	if len(options) > 0 && options[0].Encryption.Provider != "" {
		var err error
		kmsClient, rng, err = deterministicEncryptionForOptions(options[0].Encryption)
		if err != nil {
			return nil, nil, err
		}
		cfg.KMSKeyARN = "arn:aws:kms:us-east-1:111111111111:key/contract-deterministic"
		cfg.KMSClient = kmsClient
		cfg.EncryptionRand = rng
		deterministicEncryption = true
	}

	fake := fakedb.New()
	db, err := tabletheory.NewWithClient(cfg, fake)
	if err != nil {
		return nil, nil, err
	}

	return &TheorydbDriver{
		db:                      db,
		deterministicEncryption: deterministicEncryption,
		kmsClient:               kmsClient,
		kmsRand:                 rng,
		fake:                    fake,
		raw:                     fake,
	}, fake, nil
}

func (d *TheorydbDriver) Create(ctx context.Context, model string, item map[string]any, ifNotExists bool) error {
	if err := validateReleaseStateMetadataIfPresent(model, item); err != nil {
		return err
	}
	instance, err := modelFromMap(model, item)
	if err != nil {
		return err
	}

	q := d.activeDB().WithContext(ctx).Model(instance)
	if ifNotExists {
		q = q.IfNotExists()
	}
	return q.Create()
}

func (d *TheorydbDriver) Get(ctx context.Context, model string, key map[string]any) (map[string]any, error) {
	pk, sk, err := keyValues(key)
	if err != nil {
		return nil, err
	}

	switch model {
	case "User":
		var out User
		err := d.activeDB().WithContext(ctx).Model(&User{}).Where("PK", "=", pk).Where("SK", "=", sk).First(&out)
		if err != nil {
			return nil, err
		}
		return normalizeUser(out), nil
	case "StructuredProfileUser":
		var out structuredProfileUserRecordCarrier
		err := d.activeDB().WithContext(ctx).Model(&structuredProfileUserRecordCarrier{}).
			Where("PK", "=", pk).Where("SK", "=", sk).First(&out)
		if err != nil {
			return nil, err
		}
		return normalizeStructuredProfileUser(out), nil
	case "Order":
		var out Order
		err := d.activeDB().WithContext(ctx).Model(&Order{}).Where("PK", "=", pk).Where("SK", "=", sk).First(&out)
		if err != nil {
			return nil, err
		}
		return normalizeOrder(out), nil
	case "NumberPrecision":
		var out NumberPrecision
		err := d.activeDB().WithContext(ctx).Model(&NumberPrecision{}).Where("PK", "=", pk).Where("SK", "=", sk).First(&out)
		if err != nil {
			return nil, err
		}
		return normalizeNumberPrecision(out), nil
	case "TypeMatrix":
		var out TypeMatrix
		err := d.activeDB().WithContext(ctx).Model(&TypeMatrix{}).Where("PK", "=", pk).Where("SK", "=", sk).First(&out)
		if err != nil {
			return nil, err
		}
		return normalizeTypeMatrix(out), nil
	case "SnakeCaseRecord":
		var out SnakeCaseRecord
		err := d.activeDB().WithContext(ctx).Model(&SnakeCaseRecord{}).Where("pk", "=", pk).Where("sk", "=", sk).First(&out)
		if err != nil {
			return nil, err
		}
		return normalizeSnakeCaseRecord(out), nil
	case "EncryptedRecord":
		var out EncryptedRecord
		err := d.activeDB().WithContext(ctx).Model(&EncryptedRecord{}).Where("PK", "=", pk).Where("SK", "=", sk).First(&out)
		if err != nil {
			return nil, err
		}
		return normalizeEncryptedRecord(out), nil
	case "ReleaseStateActual":
		var out ReleaseStateActual
		err := d.activeDB().WithContext(ctx).Model(&ReleaseStateActual{}).Where("PK", "=", pk).Where("SK", "=", sk).First(&out)
		if err != nil {
			return nil, err
		}
		return normalizeReleaseStateActual(out), nil
	case "ReleaseStateEvent":
		var out ReleaseStateEvent
		err := d.activeDB().WithContext(ctx).Model(&ReleaseStateEvent{}).Where("PK", "=", pk).Where("SK", "=", sk).First(&out)
		if err != nil {
			return nil, err
		}
		return normalizeReleaseStateEvent(out), nil
	default:
		return nil, fmt.Errorf("%w: unknown model %q", theorydbErrors.ErrInvalidModel, model)
	}
}

func (d *TheorydbDriver) GetOptional(ctx context.Context, model string, key map[string]any) (map[string]any, bool, error) {
	item, err := d.Get(ctx, model, key)
	if err == nil {
		return item, true, nil
	}
	if errors.Is(err, theorydbErrors.ErrItemNotFound) {
		return nil, false, nil
	}
	return nil, false, err
}

func (d *TheorydbDriver) Update(ctx context.Context, model string, item map[string]any, fields []string, protectedAttributes []string) error {
	instance, err := modelFromMap(model, item)
	if err != nil {
		return err
	}
	if mutatesProtectedAttribute(fields, protectedAttributes) {
		return fmt.Errorf("%w: per-call protected attribute", theorydbErrors.ErrProtectedFieldMutation)
	}
	return d.activeDB().WithContext(ctx).Model(instance).Update(fields...)
}

func (d *TheorydbDriver) Save(ctx context.Context, model string, item map[string]any) error {
	if err := validateReleaseStateMetadataIfPresent(model, item); err != nil {
		return err
	}
	instance, err := modelFromMap(model, item)
	if err != nil {
		return err
	}
	return d.activeDB().WithContext(ctx).Model(instance).CreateOrUpdate()
}

func (d *TheorydbDriver) Delete(ctx context.Context, model string, key map[string]any) error {
	pk, sk, err := keyValues(key)
	if err != nil {
		return err
	}

	switch model {
	case "User":
		return d.activeDB().WithContext(ctx).Model(&User{}).Where("PK", "=", pk).Where("SK", "=", sk).Delete()
	case "Order":
		return d.activeDB().WithContext(ctx).Model(&Order{}).Where("PK", "=", pk).Where("SK", "=", sk).Delete()
	case "NumberPrecision":
		return d.activeDB().WithContext(ctx).Model(&NumberPrecision{}).Where("PK", "=", pk).Where("SK", "=", sk).Delete()
	case "TypeMatrix":
		return d.activeDB().WithContext(ctx).Model(&TypeMatrix{}).Where("PK", "=", pk).Where("SK", "=", sk).Delete()
	case "SnakeCaseRecord":
		return d.activeDB().WithContext(ctx).Model(&SnakeCaseRecord{}).Where("pk", "=", pk).Where("sk", "=", sk).Delete()
	case "EncryptedRecord":
		return d.activeDB().WithContext(ctx).Model(&EncryptedRecord{}).Where("PK", "=", pk).Where("SK", "=", sk).Delete()
	case "ReleaseStateActual":
		return d.activeDB().WithContext(ctx).Model(&ReleaseStateActual{}).Where("PK", "=", pk).Where("SK", "=", sk).Delete()
	case "ReleaseStateEvent":
		return d.activeDB().WithContext(ctx).Model(&ReleaseStateEvent{}).Where("PK", "=", pk).Where("SK", "=", sk).Delete()
	default:
		return fmt.Errorf("%w: unknown model %q", theorydbErrors.ErrInvalidModel, model)
	}
}

func (d *TheorydbDriver) Query(ctx context.Context, model string, req ReadRequest) (ReadResult, error) {
	q, err := d.buildReadQuery(ctx, model, req)
	if err != nil {
		return ReadResult{}, err
	}
	if req.Partition == nil {
		return ReadResult{}, fmt.Errorf("%w: query partition is required", theorydbErrors.ErrMissingPrimaryKey)
	}
	q = q.Where(req.Partition.Attribute, req.Partition.Operator, conditionValue(*req.Partition))
	if req.Sort != nil {
		q = q.Where(req.Sort.Attribute, req.Sort.Operator, conditionValue(*req.Sort))
	}
	return d.executeRead(model, q)
}

func (d *TheorydbDriver) Scan(ctx context.Context, model string, req ReadRequest) (ReadResult, error) {
	q, err := d.buildReadQuery(ctx, model, req)
	if err != nil {
		return ReadResult{}, err
	}
	return d.executeRead(model, q)
}

func (d *TheorydbDriver) buildReadQuery(ctx context.Context, modelName string, req ReadRequest) (core.Query, error) {
	instance, err := emptyModel(modelName)
	if err != nil {
		return nil, err
	}
	q := d.activeDB().WithContext(ctx).Model(instance)
	if req.Index != "" {
		q = q.Index(req.Index)
	}
	for _, filter := range req.Filter {
		q = q.Filter(filter.Attribute, filter.Operator, conditionValue(filter))
	}
	if req.SortDirection != "" {
		sortAttr := ""
		if req.Sort != nil {
			sortAttr = req.Sort.Attribute
		}
		q = q.OrderBy(sortAttr, req.SortDirection)
	}
	if req.Limit > 0 {
		q = q.Limit(req.Limit)
	}
	if len(req.Projection) > 0 {
		q = q.Select(req.Projection...)
	}
	if req.Cursor != "" {
		q = q.Cursor(req.Cursor)
	}
	if req.ConsistentRead != nil && *req.ConsistentRead {
		q = q.ConsistentRead()
	}
	return q, nil
}

func (d *TheorydbDriver) executeRead(model string, q core.Query) (ReadResult, error) {
	switch model {
	case "User":
		var out []User
		page, err := q.AllPaginated(&out)
		if err != nil {
			return ReadResult{}, err
		}
		return ReadResult{Items: normalizeUsers(out), Cursor: page.NextCursor}, nil
	case "Order":
		var out []Order
		page, err := q.AllPaginated(&out)
		if err != nil {
			return ReadResult{}, err
		}
		return ReadResult{Items: normalizeOrders(out), Cursor: page.NextCursor}, nil
	case "NumberPrecision":
		var out []NumberPrecision
		page, err := q.AllPaginated(&out)
		if err != nil {
			return ReadResult{}, err
		}
		return ReadResult{Items: normalizeNumberPrecisions(out), Cursor: page.NextCursor}, nil
	case "TypeMatrix":
		var out []TypeMatrix
		page, err := q.AllPaginated(&out)
		if err != nil {
			return ReadResult{}, err
		}
		return ReadResult{Items: normalizeTypeMatrices(out), Cursor: page.NextCursor}, nil
	case "SnakeCaseRecord":
		var out []SnakeCaseRecord
		page, err := q.AllPaginated(&out)
		if err != nil {
			return ReadResult{}, err
		}
		return ReadResult{Items: normalizeSnakeCaseRecords(out), Cursor: page.NextCursor}, nil
	case "EncryptedRecord":
		var out []EncryptedRecord
		page, err := q.AllPaginated(&out)
		if err != nil {
			return ReadResult{}, err
		}
		return ReadResult{Items: normalizeEncryptedRecords(out), Cursor: page.NextCursor}, nil
	case "ReleaseStateActual":
		var out []ReleaseStateActual
		page, err := q.AllPaginated(&out)
		if err != nil {
			return ReadResult{}, err
		}
		return ReadResult{Items: normalizeReleaseStateActuals(out), Cursor: page.NextCursor}, nil
	case "ReleaseStateEvent":
		var out []ReleaseStateEvent
		page, err := q.AllPaginated(&out)
		if err != nil {
			return ReadResult{}, err
		}
		return ReadResult{Items: normalizeReleaseStateEvents(out), Cursor: page.NextCursor}, nil
	default:
		return ReadResult{}, fmt.Errorf("%w: unknown model %q", theorydbErrors.ErrInvalidModel, model)
	}
}

func conditionValue(cond ReadCondition) any {
	if len(cond.Values) > 0 {
		return append([]any(nil), cond.Values...)
	}
	return cond.Value
}

func (d *TheorydbDriver) CountQuery(ctx context.Context, model string, req ReadRequest) (ReadResult, error) {
	q, err := d.buildReadQuery(ctx, model, req)
	if err != nil {
		return ReadResult{}, err
	}
	if req.Partition == nil {
		return ReadResult{}, fmt.Errorf("%w: query partition is required", theorydbErrors.ErrMissingPrimaryKey)
	}
	q = q.Where(req.Partition.Attribute, req.Partition.Operator, conditionValue(*req.Partition))
	if req.Sort != nil {
		q = q.Where(req.Sort.Attribute, req.Sort.Operator, conditionValue(*req.Sort))
	}
	return countResult(q)
}

func (d *TheorydbDriver) CountScan(ctx context.Context, model string, req ReadRequest) (ReadResult, error) {
	q, err := d.buildReadQuery(ctx, model, req)
	if err != nil {
		return ReadResult{}, err
	}
	return countResult(q)
}

func countResult(q core.Query) (ReadResult, error) {
	count, err := q.Count()
	if err != nil {
		return ReadResult{}, err
	}
	return ReadResult{Items: []map[string]any{}, Count: &count}, nil
}

func (d *TheorydbDriver) TransactGet(ctx context.Context, model string, items []KeyedItem) (ReadResult, error) {
	getter, ok := d.activeDB().(core.TransactGetter)
	if !ok {
		return ReadResult{}, fmt.Errorf("%w: transact get extension unavailable", theorydbErrors.ErrInvalidOperator)
	}

	requests := make([]core.TransactGetRequest, 0, len(items))
	dests := make([]any, 0, len(items))
	models := make([]string, 0, len(items))
	for _, item := range items {
		itemModel := item.Model
		if itemModel == "" {
			itemModel = model
		}
		empty, err := emptyModel(itemModel)
		if err != nil {
			return ReadResult{}, err
		}
		dest, err := newModelDest(itemModel)
		if err != nil {
			return ReadResult{}, err
		}
		key, err := keyPairFromMap(item.Key)
		if err != nil {
			return ReadResult{}, err
		}
		requests = append(requests, core.TransactGetRequest{
			Model: empty,
			Key:   key,
			Dest:  dest,
		})
		dests = append(dests, dest)
		models = append(models, itemModel)
	}

	results, err := getter.TransactGet(ctx, requests)
	if err != nil {
		return ReadResult{}, err
	}
	out := make([]map[string]any, 0, len(results))
	for i, result := range results {
		if !result.Found {
			continue
		}
		normalized, err := normalizeModel(models[i], dests[i])
		if err != nil {
			return ReadResult{}, err
		}
		out = append(out, normalized)
	}
	return ReadResult{Items: out}, nil
}

func (d *TheorydbDriver) BatchGet(ctx context.Context, model string, keys []map[string]any) (ReadResult, error) {
	keyPairs := make([]any, 0, len(keys))
	for _, key := range keys {
		pair, err := keyPairFromMap(key)
		if err != nil {
			return ReadResult{}, err
		}
		keyPairs = append(keyPairs, pair)
	}

	switch model {
	case "User":
		var out []User
		err := d.activeDB().WithContext(ctx).Model(&User{}).BatchGet(keyPairs, &out)
		return ReadResult{Items: normalizeUsers(out)}, err
	case "Order":
		var out []Order
		err := d.activeDB().WithContext(ctx).Model(&Order{}).BatchGet(keyPairs, &out)
		return ReadResult{Items: normalizeOrders(out)}, err
	case "NumberPrecision":
		var out []NumberPrecision
		err := d.activeDB().WithContext(ctx).Model(&NumberPrecision{}).BatchGet(keyPairs, &out)
		return ReadResult{Items: normalizeNumberPrecisions(out)}, err
	case "TypeMatrix":
		var out []TypeMatrix
		err := d.activeDB().WithContext(ctx).Model(&TypeMatrix{}).BatchGet(keyPairs, &out)
		return ReadResult{Items: normalizeTypeMatrices(out)}, err
	case "SnakeCaseRecord":
		var out []SnakeCaseRecord
		err := d.activeDB().WithContext(ctx).Model(&SnakeCaseRecord{}).BatchGet(keyPairs, &out)
		return ReadResult{Items: normalizeSnakeCaseRecords(out)}, err
	case "EncryptedRecord":
		var out []EncryptedRecord
		err := d.activeDB().WithContext(ctx).Model(&EncryptedRecord{}).BatchGet(keyPairs, &out)
		return ReadResult{Items: normalizeEncryptedRecords(out)}, err
	case "ReleaseStateActual":
		var out []ReleaseStateActual
		err := d.activeDB().WithContext(ctx).Model(&ReleaseStateActual{}).BatchGet(keyPairs, &out)
		return ReadResult{Items: normalizeReleaseStateActuals(out)}, err
	case "ReleaseStateEvent":
		var out []ReleaseStateEvent
		err := d.activeDB().WithContext(ctx).Model(&ReleaseStateEvent{}).BatchGet(keyPairs, &out)
		return ReadResult{Items: normalizeReleaseStateEvents(out)}, err
	default:
		return ReadResult{}, fmt.Errorf("%w: unknown model %q", theorydbErrors.ErrInvalidModel, model)
	}
}

func (d *TheorydbDriver) BatchWrite(ctx context.Context, model string, puts []map[string]any, deletes []map[string]any) error {
	putItems := make([]any, 0, len(puts))
	for _, item := range puts {
		instance, err := modelFromMap(model, item)
		if err != nil {
			return err
		}
		putItems = append(putItems, instance)
	}

	deleteKeys := make([]any, 0, len(deletes))
	for _, key := range deletes {
		pair, err := keyPairFromMap(key)
		if err != nil {
			return err
		}
		deleteKeys = append(deleteKeys, pair)
	}

	empty, err := emptyModel(model)
	if err != nil {
		return err
	}
	return d.activeDB().WithContext(ctx).Model(empty).BatchWrite(putItems, deleteKeys)
}

func (d *TheorydbDriver) TransactWrite(ctx context.Context, model string, actions []TransactWriteAction) error {
	return d.activeDB().TransactWrite(ctx, func(tx core.TransactionBuilder) error {
		for _, action := range actions {
			actionModel := action.Model
			if actionModel == "" {
				actionModel = model
			}
			conditions := transactConditionsFromAction(action)
			switch strings.ToLower(action.Kind) {
			case "put", "create":
				instance, err := modelFromMap(actionModel, action.Item)
				if err != nil {
					return err
				}
				if action.IfNotExists || strings.EqualFold(action.Kind, "create") {
					tx = tx.Create(instance, conditions...)
				} else {
					tx = tx.Put(instance, conditions...)
				}
			case "update":
				item := mergeMaps(action.Key, action.Set)
				instance, err := modelFromMap(actionModel, item)
				if err != nil {
					return err
				}
				fields, err := updateFieldNames(actionModel, action.Set)
				if err != nil {
					return err
				}
				tx = tx.Update(instance, fields, conditions...)
			case "delete":
				instance, err := modelFromMap(actionModel, action.Key)
				if err != nil {
					return err
				}
				tx = tx.Delete(instance, conditions...)
			case "condition", "condition_check":
				instance, err := modelFromMap(actionModel, action.Key)
				if err != nil {
					return err
				}
				tx = tx.ConditionCheck(instance, conditions...)
			default:
				return fmt.Errorf("%w: unsupported transact_write action %q", theorydbErrors.ErrInvalidOperator, action.Kind)
			}
		}
		return nil
	})
}

func (d *TheorydbDriver) TransitionAppendEvent(ctx context.Context, actual TransitionActual, event TransitionEvent) error {
	if actual.Model != "ReleaseStateActual" || event.Model != "ReleaseStateEvent" {
		return fmt.Errorf("%w: unsupported transition models %s/%s", theorydbErrors.ErrInvalidModel, actual.Model, event.Model)
	}

	actualItem := make(map[string]any, len(actual.Key))
	for k, v := range actual.Key {
		actualItem[k] = v
	}
	actualModel, err := releaseStateActualFromMap(actualItem)
	if err != nil {
		return err
	}

	eventModel, err := releaseStateEventFromMap(event.Item)
	if err != nil {
		return err
	}

	return releasestate.TransitionAppendEvent(ctx, d.db, releasestate.TransitionAppendEventInput{
		Actual:          actualModel,
		Event:           eventModel,
		Set:             actual.Set,
		ExpectedVersion: actual.ExpectedVersion,
		EventBinding:    event.Binding,
	})
}

func (d *TheorydbDriver) ValidateProvenance(ctx context.Context, model string, item map[string]any) error {
	if model != "ReleaseStateActual" {
		return fmt.Errorf("%w: validate_provenance unsupported for %s", theorydbErrors.ErrInvalidModel, model)
	}
	return releasestate.ValidateDeployAuthorityMetadata(item)
}

func validateReleaseStateMetadataIfPresent(model string, item map[string]any) error {
	if model != "ReleaseStateActual" {
		return nil
	}
	_, hasProvenance := item["provenance"]
	_, hasConfidence := item["confidence"]
	if !hasProvenance && !hasConfidence {
		return nil
	}
	return releasestate.ValidateDeployAuthorityMetadata(item)
}

func keyValues(key map[string]any) (string, string, error) {
	pkVal, ok := key["PK"]
	if !ok {
		pkVal, ok = key["pk"]
	}
	if !ok {
		return "", "", fmt.Errorf("%w: PK is required", theorydbErrors.ErrMissingPrimaryKey)
	}
	skVal, ok := key["SK"]
	if !ok {
		skVal, ok = key["sk"]
	}
	if !ok {
		return "", "", fmt.Errorf("%w: SK is required", theorydbErrors.ErrMissingPrimaryKey)
	}
	return fmt.Sprintf("%v", pkVal), fmt.Sprintf("%v", skVal), nil
}

func modelFromMap(model string, item map[string]any) (any, error) {
	switch model {
	case "User":
		return userFromMap(item)
	case "StructuredProfileUser":
		return structuredProfileUserFromMap(item)
	case "Order":
		return orderFromMap(item)
	case "NumberPrecision":
		return numberPrecisionFromMap(item)
	case "TypeMatrix":
		return typeMatrixFromMap(item)
	case "SnakeCaseRecord":
		return snakeCaseRecordFromMap(item)
	case "EncryptedRecord":
		return encryptedRecordFromMap(item)
	case "ReleaseStateActual":
		return releaseStateActualFromMap(item)
	case "ReleaseStateEvent":
		return releaseStateEventFromMap(item)
	default:
		return nil, fmt.Errorf("%w: unknown model %q", theorydbErrors.ErrInvalidModel, model)
	}
}

func emptyModel(model string) (any, error) {
	switch model {
	case "User":
		return &User{}, nil
	case "StructuredProfileUser":
		return &structuredProfileUserRecordCarrier{}, nil
	case "Order":
		return &Order{}, nil
	case "NumberPrecision":
		return &NumberPrecision{}, nil
	case "TypeMatrix":
		return &TypeMatrix{}, nil
	case "SnakeCaseRecord":
		return &SnakeCaseRecord{}, nil
	case "EncryptedRecord":
		return &EncryptedRecord{}, nil
	case "ReleaseStateActual":
		return &ReleaseStateActual{}, nil
	case "ReleaseStateEvent":
		return &ReleaseStateEvent{}, nil
	default:
		return nil, fmt.Errorf("%w: unknown model %q", theorydbErrors.ErrInvalidModel, model)
	}
}

func newModelDest(model string) (any, error) {
	return emptyModel(model)
}

func normalizeModel(model string, value any) (map[string]any, error) {
	switch model {
	case "User":
		if v, ok := value.(*User); ok {
			return normalizeUser(*v), nil
		}
	case "StructuredProfileUser":
		if v, ok := value.(*structuredProfileUserRecordCarrier); ok {
			return normalizeStructuredProfileUser(*v), nil
		}
	case "Order":
		if v, ok := value.(*Order); ok {
			return normalizeOrder(*v), nil
		}
	case "NumberPrecision":
		if v, ok := value.(*NumberPrecision); ok {
			return normalizeNumberPrecision(*v), nil
		}
	case "TypeMatrix":
		if v, ok := value.(*TypeMatrix); ok {
			return normalizeTypeMatrix(*v), nil
		}
	case "SnakeCaseRecord":
		if v, ok := value.(*SnakeCaseRecord); ok {
			return normalizeSnakeCaseRecord(*v), nil
		}
	case "EncryptedRecord":
		if v, ok := value.(*EncryptedRecord); ok {
			return normalizeEncryptedRecord(*v), nil
		}
	case "ReleaseStateActual":
		if v, ok := value.(*ReleaseStateActual); ok {
			return normalizeReleaseStateActual(*v), nil
		}
	case "ReleaseStateEvent":
		if v, ok := value.(*ReleaseStateEvent); ok {
			return normalizeReleaseStateEvent(*v), nil
		}
	}
	return nil, fmt.Errorf("%w: cannot normalize %T as %s", theorydbErrors.ErrInvalidModel, value, model)
}

func keyPairFromMap(key map[string]any) (core.KeyPair, error) {
	pk, sk, err := keyValues(key)
	if err != nil {
		return core.KeyPair{}, err
	}
	return core.NewKeyPair(pk, sk), nil
}

func transactConditionsFromAction(action TransactWriteAction) []core.TransactCondition {
	if strings.TrimSpace(action.ConditionExpression) == "" {
		return nil
	}
	values := make(map[string]any, len(action.ExpressionAttributeValues))
	for k, v := range action.ExpressionAttributeValues {
		values[k] = v
	}
	return []core.TransactCondition{
		{
			Kind:       core.TransactConditionKindExpression,
			Expression: action.ConditionExpression,
			Values:     values,
		},
	}
}

func mergeMaps(left map[string]any, right map[string]any) map[string]any {
	out := make(map[string]any, len(left)+len(right))
	for k, v := range left {
		out[k] = v
	}
	for k, v := range right {
		out[k] = v
	}
	return out
}

func sortedMapKeys(values map[string]any) []string {
	keys := make([]string, 0, len(values))
	for k := range values {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}

func updateFieldNames(model string, values map[string]any) ([]string, error) {
	fields := sortedMapKeys(values)
	instance, err := emptyModel(model)
	if err != nil {
		return nil, err
	}
	typ := reflect.TypeOf(instance)
	if typ.Kind() == reflect.Pointer {
		typ = typ.Elem()
	}
	resolved := make([]string, 0, len(fields))
	for _, field := range fields {
		goName, ok := resolveGoFieldName(typ, field)
		if !ok {
			return nil, fmt.Errorf("%w: unknown update field %s", theorydbErrors.ErrInvalidModel, field)
		}
		resolved = append(resolved, goName)
	}
	return resolved, nil
}

func resolveGoFieldName(typ reflect.Type, attr string) (string, bool) {
	for i := 0; i < typ.NumField(); i++ {
		field := typ.Field(i)
		if field.Name == "_" {
			continue
		}
		candidates := []string{field.Name, lowerFirst(field.Name)}
		if tag := field.Tag.Get("theorydb"); tag != "" {
			for _, part := range strings.Split(tag, ",") {
				if strings.HasPrefix(part, "attr:") {
					candidates = append(candidates, strings.TrimPrefix(part, "attr:"))
				}
			}
		}
		for _, candidate := range candidates {
			if candidate == attr || strings.EqualFold(candidate, attr) {
				return field.Name, true
			}
		}
	}
	return "", false
}

func lowerFirst(value string) string {
	if value == "" {
		return value
	}
	return strings.ToLower(value[:1]) + value[1:]
}

func asStringSlice(v any) ([]string, error) {
	if v == nil {
		return nil, nil
	}
	switch s := v.(type) {
	case []string:
		return s, nil
	case []any:
		out := make([]string, 0, len(s))
		for _, item := range s {
			out = append(out, fmt.Sprintf("%v", item))
		}
		return out, nil
	default:
		return nil, fmt.Errorf("expected []string, got %T", v)
	}
}

func asStringMap(v any) (map[string]any, error) {
	if v == nil {
		return nil, nil
	}
	switch m := v.(type) {
	case map[string]any:
		out := make(map[string]any, len(m))
		for k, value := range m {
			out[k] = value
		}
		return out, nil
	case map[any]any:
		out := make(map[string]any, len(m))
		for k, value := range m {
			key, ok := k.(string)
			if !ok {
				return nil, fmt.Errorf("expected string map key, got %T", k)
			}
			out[key] = value
		}
		return out, nil
	default:
		return nil, fmt.Errorf("expected map[string]any, got %T", v)
	}
}

func asInt64(v any) (int64, error) {
	if v == nil {
		return 0, nil
	}
	switch n := v.(type) {
	case int:
		return int64(n), nil
	case int64:
		return n, nil
	case uint64:
		return int64(n), nil
	case float64:
		return int64(n), nil
	case string:
		parsed, err := strconv.ParseInt(n, 10, 64)
		if err != nil {
			return 0, err
		}
		return parsed, nil
	default:
		return 0, fmt.Errorf("expected number, got %T", v)
	}
}

func asInt64Slice(v any) ([]int64, error) {
	if v == nil {
		return nil, nil
	}
	switch s := v.(type) {
	case []int64:
		return append([]int64(nil), s...), nil
	case []any:
		out := make([]int64, 0, len(s))
		for _, item := range s {
			n, err := asInt64(item)
			if err != nil {
				return nil, err
			}
			out = append(out, n)
		}
		return out, nil
	default:
		return nil, fmt.Errorf("expected []number, got %T", v)
	}
}

func asBase64Bytes(v any) ([]byte, error) {
	if v == nil {
		return nil, nil
	}
	switch b := v.(type) {
	case []byte:
		return append([]byte(nil), b...), nil
	case string:
		return base64.StdEncoding.DecodeString(b)
	default:
		return nil, fmt.Errorf("expected base64 string, got %T", v)
	}
}

func asBase64ByteSlices(v any) ([][]byte, error) {
	if v == nil {
		return nil, nil
	}
	switch s := v.(type) {
	case [][]byte:
		out := make([][]byte, len(s))
		for i := range s {
			out[i] = append([]byte(nil), s[i]...)
		}
		return out, nil
	case []any:
		out := make([][]byte, 0, len(s))
		for _, item := range s {
			b, err := asBase64Bytes(item)
			if err != nil {
				return nil, err
			}
			out = append(out, b)
		}
		return out, nil
	default:
		return nil, fmt.Errorf("expected []base64 string, got %T", v)
	}
}

func asBool(v any) (bool, error) {
	if v == nil {
		return false, nil
	}
	b, ok := v.(bool)
	if !ok {
		return false, fmt.Errorf("expected bool, got %T", v)
	}
	return b, nil
}

func asAnySlice(v any) ([]any, error) {
	if v == nil {
		return nil, nil
	}
	s, ok := v.([]any)
	if !ok {
		return nil, fmt.Errorf("expected []any, got %T", v)
	}
	return append([]any(nil), s...), nil
}

func mutatesProtectedAttribute(fields []string, protectedAttributes []string) bool {
	if len(fields) == 0 || len(protectedAttributes) == 0 {
		return false
	}
	protected := make(map[string]struct{}, len(protectedAttributes))
	for _, attr := range protectedAttributes {
		protected[attr] = struct{}{}
	}
	for _, field := range fields {
		if _, ok := protected[field]; ok {
			return true
		}
	}
	return false
}

// ---- Contract model glue ----

func userFromMap(item map[string]any) (*User, error) {
	u := &User{}
	if v, ok := item["PK"]; ok {
		u.PK = fmt.Sprintf("%v", v)
	}
	if v, ok := item["SK"]; ok {
		u.SK = fmt.Sprintf("%v", v)
	}
	if v, ok := item["emailHash"]; ok {
		u.EmailHash = fmt.Sprintf("%v", v)
	}
	if v, ok := item["nickname"]; ok {
		u.Nickname = fmt.Sprintf("%v", v)
	}
	if v, ok := item["tags"]; ok {
		tags, err := asStringSlice(v)
		if err != nil {
			return nil, err
		}
		u.Tags = tags
	}
	if v, ok := item["profile"]; ok {
		profile, err := asStringMap(v)
		if err != nil {
			return nil, err
		}
		u.Profile = profile
	}
	if v, ok := item["version"]; ok {
		n, err := asInt64(v)
		if err != nil {
			return nil, err
		}
		u.Version = n
	}
	if v, ok := item["ttl"]; ok {
		n, err := asInt64(v)
		if err != nil {
			return nil, err
		}
		u.TTL = n
	}
	return u, nil
}

func structuredProfileUserFromMap(item map[string]any) (*structuredProfileUserRecordCarrier, error) {
	u := &structuredProfileUserRecordCarrier{}
	if v, ok := item["PK"]; ok {
		u.PK = fmt.Sprintf("%v", v)
	}
	if v, ok := item["SK"]; ok {
		u.SK = fmt.Sprintf("%v", v)
	}
	if v, ok := item["profile"]; ok {
		profile, err := asStringMap(v)
		if err != nil {
			return nil, err
		}
		if source, ok := profile["source"]; ok {
			u.Profile.Source = fmt.Sprintf("%v", source)
		}
	}
	if v, ok := item["version"]; ok {
		n, err := asInt64(v)
		if err != nil {
			return nil, err
		}
		u.Version = n
	}
	return u, nil
}

func orderFromMap(item map[string]any) (*Order, error) {
	o := &Order{}
	if v, ok := item["PK"]; ok {
		o.PK = fmt.Sprintf("%v", v)
	}
	if v, ok := item["SK"]; ok {
		o.SK = fmt.Sprintf("%v", v)
	}
	if v, ok := item["status"]; ok {
		o.Status = fmt.Sprintf("%v", v)
	}
	if v, ok := item["amount"]; ok {
		n, err := asInt64(v)
		if err != nil {
			return nil, err
		}
		o.Amount = n
	}
	if v, ok := item["version"]; ok {
		n, err := asInt64(v)
		if err != nil {
			return nil, err
		}
		o.Version = n
	}
	if v, ok := item["ttl"]; ok {
		n, err := asInt64(v)
		if err != nil {
			return nil, err
		}
		o.TTL = n
	}
	return o, nil
}

func numberPrecisionFromMap(item map[string]any) (*NumberPrecision, error) {
	n := &NumberPrecision{}
	if v, ok := item["PK"]; ok {
		n.PK = fmt.Sprintf("%v", v)
	}
	if v, ok := item["SK"]; ok {
		n.SK = fmt.Sprintf("%v", v)
	}
	if v, ok := item["largeInteger"]; ok {
		n.LargeInteger = json.Number(fmt.Sprintf("%v", v))
	}
	if v, ok := item["preciseDecimal"]; ok {
		n.PreciseDecimal = json.Number(fmt.Sprintf("%v", v))
	}
	return n, nil
}

func typeMatrixFromMap(item map[string]any) (*TypeMatrix, error) {
	tm := &TypeMatrix{}
	if v, ok := item["PK"]; ok {
		tm.PK = fmt.Sprintf("%v", v)
	}
	if v, ok := item["SK"]; ok {
		tm.SK = fmt.Sprintf("%v", v)
	}
	if v, ok := item["numberSet"]; ok {
		values, err := asInt64Slice(v)
		if err != nil {
			return nil, err
		}
		tm.NumberSet = values
	}
	if v, ok := item["binaryBlob"]; ok {
		value, err := asBase64Bytes(v)
		if err != nil {
			return nil, err
		}
		tm.BinaryBlob = value
	}
	if v, ok := item["binarySet"]; ok {
		values, err := asBase64ByteSlices(v)
		if err != nil {
			return nil, err
		}
		tm.BinarySet = values
	}
	if v, ok := item["flag"]; ok {
		value, err := asBool(v)
		if err != nil {
			return nil, err
		}
		tm.Flag = value
	}
	if v, ok := item["nothing"]; ok && v != nil {
		value := fmt.Sprintf("%v", v)
		tm.Nothing = &value
	}
	if v, ok := item["items"]; ok {
		values, err := asAnySlice(v)
		if err != nil {
			return nil, err
		}
		tm.Items = values
	}
	if v, ok := item["metadata"]; ok {
		value, err := asStringMap(v)
		if err != nil {
			return nil, err
		}
		tm.Metadata = value
	}
	if v, ok := item["emptyNumberSet"]; ok {
		values, err := asInt64Slice(v)
		if err != nil {
			return nil, err
		}
		tm.EmptyNumberSet = values
	}
	if v, ok := item["emptyBinarySet"]; ok {
		values, err := asBase64ByteSlices(v)
		if err != nil {
			return nil, err
		}
		tm.EmptyBinarySet = values
	}
	if v, ok := item["optionalString"]; ok {
		tm.OptionalString = fmt.Sprintf("%v", v)
	}
	return tm, nil
}

func snakeCaseRecordFromMap(item map[string]any) (*SnakeCaseRecord, error) {
	record := &SnakeCaseRecord{}
	if v, ok := item["pk"]; ok {
		record.PK = fmt.Sprintf("%v", v)
	}
	if v, ok := item["sk"]; ok {
		record.SK = fmt.Sprintf("%v", v)
	}
	if v, ok := item["display_name"]; ok {
		record.DisplayName = fmt.Sprintf("%v", v)
	}
	if v, ok := item["email_hash"]; ok {
		record.EmailHash = fmt.Sprintf("%v", v)
	}
	return record, nil
}

func encryptedRecordFromMap(item map[string]any) (*EncryptedRecord, error) {
	record := &EncryptedRecord{}
	if v, ok := item["PK"]; ok {
		record.PK = fmt.Sprintf("%v", v)
	}
	if v, ok := item["SK"]; ok {
		record.SK = fmt.Sprintf("%v", v)
	}
	if v, ok := item["secret"]; ok {
		record.Secret = fmt.Sprintf("%v", v)
	}
	return record, nil
}

func releaseStateActualFromMap(item map[string]any) (*ReleaseStateActual, error) {
	actual := &ReleaseStateActual{}
	if v, ok := item["PK"]; ok {
		actual.PK = fmt.Sprintf("%v", v)
	}
	if v, ok := item["SK"]; ok {
		actual.SK = fmt.Sprintf("%v", v)
	}
	if v, ok := item["status"]; ok {
		actual.Status = fmt.Sprintf("%v", v)
	}
	if v, ok := item["pinnedReleaseId"]; ok {
		actual.PinnedReleaseID = fmt.Sprintf("%v", v)
	}
	if v, ok := item["previousReleaseId"]; ok {
		actual.PreviousReleaseID = fmt.Sprintf("%v", v)
	}
	if v, ok := item["provenance"]; ok {
		provenance, err := asStringMap(v)
		if err != nil {
			return nil, err
		}
		actual.Provenance = provenance
	}
	if v, ok := item["confidence"]; ok {
		confidence, err := asStringMap(v)
		if err != nil {
			return nil, err
		}
		actual.Confidence = confidence
	}
	if v, ok := item["version"]; ok {
		n, err := asInt64(v)
		if err != nil {
			return nil, err
		}
		actual.Version = n
	}
	return actual, nil
}

func releaseStateEventFromMap(item map[string]any) (*ReleaseStateEvent, error) {
	event := &ReleaseStateEvent{}
	if v, ok := item["PK"]; ok {
		event.PK = fmt.Sprintf("%v", v)
	}
	if v, ok := item["SK"]; ok {
		event.SK = fmt.Sprintf("%v", v)
	}
	if v, ok := item["releaseId"]; ok {
		event.ReleaseID = fmt.Sprintf("%v", v)
	}
	if v, ok := item["eventType"]; ok {
		event.EventType = fmt.Sprintf("%v", v)
	}
	if v, ok := item["provenance"]; ok {
		provenance, err := asStringMap(v)
		if err != nil {
			return nil, err
		}
		event.Provenance = provenance
	}
	if v, ok := item["confidence"]; ok {
		confidence, err := asStringMap(v)
		if err != nil {
			return nil, err
		}
		event.Confidence = confidence
	}
	if v, ok := item["recordedAt"]; ok {
		event.RecordedAt = fmt.Sprintf("%v", v)
	}
	if v, ok := item["ttl"]; ok {
		n, err := asInt64(v)
		if err != nil {
			return nil, err
		}
		event.TTL = n
	}
	return event, nil
}

func normalizeUser(u User) map[string]any {
	out := map[string]any{
		"PK":        u.PK,
		"SK":        u.SK,
		"emailHash": u.EmailHash,
		"nickname":  u.Nickname,
		"tags":      append([]string(nil), u.Tags...),
		"profile":   normalizeDocumentValue(u.Profile),
		"createdAt": u.CreatedAt.Format(time.RFC3339Nano),
		"updatedAt": u.UpdatedAt.Format(time.RFC3339Nano),
		"version":   u.Version,
		"ttl":       u.TTL,
	}
	return out
}

func normalizeStructuredProfileUser(u structuredProfileUserRecordCarrier) map[string]any {
	return map[string]any{
		"PK": u.PK,
		"SK": u.SK,
		"profile": map[string]any{
			"source": u.Profile.Source,
		},
		"version": u.Version,
	}
}

func normalizeUsers(items []User) []map[string]any {
	out := make([]map[string]any, 0, len(items))
	for _, item := range items {
		out = append(out, normalizeUser(item))
	}
	return out
}

func normalizeNumberPrecision(n NumberPrecision) map[string]any {
	return map[string]any{
		"PK":             n.PK,
		"SK":             n.SK,
		"largeInteger":   n.LargeInteger.String(),
		"preciseDecimal": n.PreciseDecimal.String(),
	}
}

func normalizeNumberPrecisions(items []NumberPrecision) []map[string]any {
	out := make([]map[string]any, 0, len(items))
	for _, item := range items {
		out = append(out, normalizeNumberPrecision(item))
	}
	return out
}

func normalizeTypeMatrix(tm TypeMatrix) map[string]any {
	return map[string]any{
		"PK":             tm.PK,
		"SK":             tm.SK,
		"numberSet":      append([]int64(nil), tm.NumberSet...),
		"binaryBlob":     base64.StdEncoding.EncodeToString(tm.BinaryBlob),
		"binarySet":      base64Strings(tm.BinarySet),
		"flag":           tm.Flag,
		"nothing":        nil,
		"items":          normalizeDocumentValue(tm.Items),
		"metadata":       normalizeDocumentValue(tm.Metadata),
		"emptyNumberSet": append([]int64(nil), tm.EmptyNumberSet...),
		"emptyBinarySet": base64Strings(tm.EmptyBinarySet),
		"optionalString": tm.OptionalString,
	}
}

func normalizeTypeMatrices(items []TypeMatrix) []map[string]any {
	out := make([]map[string]any, 0, len(items))
	for _, item := range items {
		out = append(out, normalizeTypeMatrix(item))
	}
	return out
}

func normalizeSnakeCaseRecord(record SnakeCaseRecord) map[string]any {
	return map[string]any{
		"pk":           record.PK,
		"sk":           record.SK,
		"display_name": record.DisplayName,
		"email_hash":   record.EmailHash,
	}
}

func normalizeSnakeCaseRecords(items []SnakeCaseRecord) []map[string]any {
	out := make([]map[string]any, 0, len(items))
	for _, item := range items {
		out = append(out, normalizeSnakeCaseRecord(item))
	}
	return out
}

func normalizeEncryptedRecord(record EncryptedRecord) map[string]any {
	return map[string]any{
		"PK":     record.PK,
		"SK":     record.SK,
		"secret": record.Secret,
	}
}

func normalizeEncryptedRecords(items []EncryptedRecord) []map[string]any {
	out := make([]map[string]any, 0, len(items))
	for _, item := range items {
		out = append(out, normalizeEncryptedRecord(item))
	}
	return out
}

func base64Strings(values [][]byte) []string {
	out := make([]string, 0, len(values))
	for _, value := range values {
		out = append(out, base64.StdEncoding.EncodeToString(value))
	}
	sort.Strings(out)
	return out
}

func normalizeDocumentValue(value any) any {
	switch v := value.(type) {
	case []any:
		out := make([]any, len(v))
		for i, item := range v {
			out[i] = normalizeDocumentValue(item)
		}
		return out
	case map[string]any:
		out := make(map[string]any, len(v))
		for key, item := range v {
			out[key] = normalizeDocumentValue(item)
		}
		return out
	default:
		return value
	}
}

func normalizeReleaseStateActual(actual ReleaseStateActual) map[string]any {
	return map[string]any{
		"PK":                actual.PK,
		"SK":                actual.SK,
		"status":            actual.Status,
		"pinnedReleaseId":   actual.PinnedReleaseID,
		"previousReleaseId": actual.PreviousReleaseID,
		"provenance":        actual.Provenance,
		"confidence":        actual.Confidence,
		"updatedAt":         actual.UpdatedAt.Format(time.RFC3339Nano),
		"version":           actual.Version,
	}
}

func normalizeReleaseStateActuals(items []ReleaseStateActual) []map[string]any {
	out := make([]map[string]any, 0, len(items))
	for _, item := range items {
		out = append(out, normalizeReleaseStateActual(item))
	}
	return out
}

func normalizeReleaseStateEvent(event ReleaseStateEvent) map[string]any {
	return map[string]any{
		"PK":         event.PK,
		"SK":         event.SK,
		"releaseId":  event.ReleaseID,
		"eventType":  event.EventType,
		"provenance": event.Provenance,
		"confidence": event.Confidence,
		"recordedAt": event.RecordedAt,
		"ttl":        event.TTL,
	}
}

func normalizeReleaseStateEvents(items []ReleaseStateEvent) []map[string]any {
	out := make([]map[string]any, 0, len(items))
	for _, item := range items {
		out = append(out, normalizeReleaseStateEvent(item))
	}
	return out
}

func normalizeOrder(o Order) map[string]any {
	out := map[string]any{
		"PK":        o.PK,
		"SK":        o.SK,
		"status":    o.Status,
		"amount":    o.Amount,
		"createdAt": o.CreatedAt.Format(time.RFC3339Nano),
		"updatedAt": o.UpdatedAt.Format(time.RFC3339Nano),
		"version":   o.Version,
		"ttl":       o.TTL,
	}
	return out
}

func normalizeOrders(items []Order) []map[string]any {
	out := make([]map[string]any, 0, len(items))
	for _, item := range items {
		out = append(out, normalizeOrder(item))
	}
	return out
}

const (
	migrationSourceTable    = "encrypted_records_migration_source"
	migrationTargetTable    = "encrypted_records_contract"
	migrationSourceSecret   = "secret"
	legacyEnvelopeAttribute = "secret"
)

// migrationEncryptedSourceRecord is the migration source shape: the same
// encrypted attribute name as the target model, on a different table.
type migrationEncryptedSourceRecord struct {
	PK     string `theorydb:"pk,attr:PK" json:"PK"`
	SK     string `theorydb:"sk,attr:SK" json:"SK"`
	Secret string `theorydb:"attr:secret,encrypted" json:"secret"`
}

func (migrationEncryptedSourceRecord) TableName() string { return migrationSourceTable }

func (d *TheorydbDriver) activeDB() core.ExtendedDB {
	if d.stepEncryption == "none" && d.noEncryptionDB != nil {
		return d.noEncryptionDB
	}
	return d.db
}

// SetStepEncryption selects the database instance a scenario step uses. "none"
// exercises fail-closed behavior with no KMS configuration.
func (d *TheorydbDriver) SetStepEncryption(mode string) error {
	switch mode {
	case "", "deterministic":
		d.stepEncryption = ""
		return nil
	case "none":
		if d.noEncryptionDB == nil {
			db, err := d.databaseWithoutEncryption()
			if err != nil {
				return err
			}
			d.noEncryptionDB = db
		}
		d.stepEncryption = "none"
		return nil
	default:
		return fmt.Errorf("unsupported step encryption mode %q", mode)
	}
}

func (d *TheorydbDriver) RawPut(ctx context.Context, table string, rawItem map[string]any) error {
	item := make(map[string]types.AttributeValue, len(rawItem))
	for name, raw := range rawItem {
		av, err := decodeRawAttributeValue(raw)
		if err != nil {
			return fmt.Errorf("raw_put attribute %q: %w", name, err)
		}
		item[name] = av
	}
	_, err := d.raw.PutItem(ctx, &dynamodb.PutItemInput{
		TableName: aws.String(table),
		Item:      item,
	})
	return err
}

func decodeRawAttributeValue(raw any) (types.AttributeValue, error) {
	values, ok := raw.(map[string]any)
	if !ok {
		return nil, fmt.Errorf("raw value must be a map, got %T", raw)
	}
	if value, ok := values["s"]; ok {
		return &types.AttributeValueMemberS{Value: fmt.Sprint(value)}, nil
	}
	if value, ok := values["n"]; ok {
		return &types.AttributeValueMemberN{Value: fmt.Sprint(value)}, nil
	}
	if value, ok := values["b64"]; ok {
		decoded, err := base64.StdEncoding.DecodeString(fmt.Sprint(value))
		if err != nil {
			return nil, err
		}
		return &types.AttributeValueMemberB{Value: decoded}, nil
	}
	if value, ok := values["bool"]; ok {
		flag, ok := value.(bool)
		if !ok {
			return nil, fmt.Errorf("bool raw value must be a boolean")
		}
		return &types.AttributeValueMemberBOOL{Value: flag}, nil
	}
	if value, ok := values["null"]; ok {
		if flag, ok := value.(bool); ok && flag {
			return &types.AttributeValueMemberNULL{Value: true}, nil
		}
	}
	if rawEnvelope, ok := values["envelope"]; ok {
		return decodeRawEnvelope(rawEnvelope)
	}
	return nil, fmt.Errorf("unsupported raw value shape")
}

func decodeRawEnvelope(raw any) (types.AttributeValue, error) {
	values, ok := raw.(map[string]any)
	if !ok {
		return nil, fmt.Errorf("envelope must be a map")
	}
	version := fmt.Sprint(values["version"])
	edk, err := decodeRawBase64(values["edk_b64"])
	if err != nil {
		return nil, fmt.Errorf("envelope edk: %w", err)
	}
	nonce, err := decodeRawBase64(values["nonce_b64"])
	if err != nil {
		return nil, fmt.Errorf("envelope nonce: %w", err)
	}
	ct, err := decodeRawBase64(values["ct_b64"])
	if err != nil {
		return nil, fmt.Errorf("envelope ct: %w", err)
	}
	return &types.AttributeValueMemberM{Value: map[string]types.AttributeValue{
		"v":     &types.AttributeValueMemberN{Value: version},
		"edk":   &types.AttributeValueMemberB{Value: edk},
		"nonce": &types.AttributeValueMemberB{Value: nonce},
		"ct":    &types.AttributeValueMemberB{Value: ct},
	}}, nil
}

func decodeRawBase64(raw any) ([]byte, error) {
	text := fmt.Sprint(raw)
	if text == "" {
		return nil, nil
	}
	return base64.StdEncoding.DecodeString(text)
}

// SeedEncrypted writes one migration source row. v2 rows go through the normal
// bound write path; v1 rows are written as legacy envelopes so the migration
// path can prove it upgrades the binding.
func (d *TheorydbDriver) SeedEncrypted(ctx context.Context, table string, item map[string]any, envelopeVersion int, corrupt string, reset bool) error {
	if table != migrationSourceTable {
		return fmt.Errorf("seed_encrypted supports table %q only", migrationSourceTable)
	}
	if reset {
		if err := d.resetMigrationSourceTable(ctx); err != nil {
			return err
		}
	}
	if err := d.ensureMigrationSourceTable(ctx); err != nil {
		return err
	}
	pk := fmt.Sprint(item["PK"])
	sk := fmt.Sprint(item["SK"])
	secret := fmt.Sprint(item[migrationSourceSecret])

	if envelopeVersion == 2 {
		record := &migrationEncryptedSourceRecord{PK: pk, SK: sk, Secret: secret}
		if err := d.activeDB().WithContext(ctx).Model(record).CreateOrUpdate(); err != nil {
			return err
		}
	} else {
		envelope, err := d.legacyV1Envelope(ctx, secret)
		if err != nil {
			return err
		}
		if _, err := d.raw.PutItem(ctx, &dynamodb.PutItemInput{
			TableName: aws.String(table),
			Item: map[string]types.AttributeValue{
				"PK":     &types.AttributeValueMemberS{Value: pk},
				"SK":     &types.AttributeValueMemberS{Value: sk},
				"secret": envelope,
			},
		}); err != nil {
			return err
		}
	}

	if corrupt == "" {
		return nil
	}
	return d.corruptStoredEnvelope(ctx, table, pk, sk, corrupt)
}

func (d *TheorydbDriver) resetMigrationSourceTable(ctx context.Context) error {
	_, err := d.raw.DeleteTable(ctx, &dynamodb.DeleteTableInput{TableName: aws.String(migrationSourceTable)})
	if err != nil {
		var notFound *types.ResourceNotFoundException
		if !errors.As(err, &notFound) {
			return err
		}
	}
	if d.ddb == nil {
		return nil
	}
	return dynamodb.NewTableNotExistsWaiter(d.ddb).Wait(ctx, &dynamodb.DescribeTableInput{TableName: aws.String(migrationSourceTable)}, 2*time.Minute)
}

func (d *TheorydbDriver) ensureMigrationSourceTable(ctx context.Context) error {
	_, err := d.raw.DescribeTable(ctx, &dynamodb.DescribeTableInput{TableName: aws.String(migrationSourceTable)})
	if err == nil {
		return nil
	}
	var notFound *types.ResourceNotFoundException
	if !errors.As(err, &notFound) {
		return err
	}
	_, err = d.raw.CreateTable(ctx, &dynamodb.CreateTableInput{
		TableName: aws.String(migrationSourceTable),
		AttributeDefinitions: []types.AttributeDefinition{
			{AttributeName: aws.String("PK"), AttributeType: types.ScalarAttributeTypeS},
			{AttributeName: aws.String("SK"), AttributeType: types.ScalarAttributeTypeS},
		},
		KeySchema: []types.KeySchemaElement{
			{AttributeName: aws.String("PK"), KeyType: types.KeyTypeHash},
			{AttributeName: aws.String("SK"), KeyType: types.KeyTypeRange},
		},
		BillingMode: types.BillingModePayPerRequest,
	})
	if err != nil {
		var exists *types.ResourceInUseException
		if errors.As(err, &exists) {
			return nil
		}
		return err
	}
	if d.ddb == nil {
		return nil
	}
	return dynamodb.NewTableExistsWaiter(d.ddb).Wait(ctx, &dynamodb.DescribeTableInput{TableName: aws.String(migrationSourceTable)}, 2*time.Minute)
}

func (d *TheorydbDriver) legacyV1Envelope(ctx context.Context, secret string) (types.AttributeValue, error) {
	if d.kmsClient == nil || d.kmsRand == nil {
		return nil, fmt.Errorf("seed_encrypted v1 requires the deterministic scenario encryption provider")
	}
	dataKey, err := d.kmsClient.GenerateDataKey(ctx, &kms.GenerateDataKeyInput{
		KeyId:   aws.String("arn:aws:kms:us-east-1:111111111111:key/contract-deterministic"),
		KeySpec: kmsTypes.DataKeySpecAes256,
	})
	if err != nil {
		return nil, err
	}
	block, err := aes.NewCipher(dataKey.Plaintext)
	if err != nil {
		return nil, err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}
	nonce := make([]byte, gcm.NonceSize())
	if _, err := io.ReadFull(d.kmsRand, nonce); err != nil {
		return nil, err
	}
	payload, err := json.Marshal(map[string]any{"t": "S", "s": secret})
	if err != nil {
		return nil, err
	}
	aad := []byte(fmt.Sprintf("theorydb:encrypted:v1|attr=%s", legacyEnvelopeAttribute))
	ct := gcm.Seal(nil, nonce, payload, aad)
	return &types.AttributeValueMemberM{Value: map[string]types.AttributeValue{
		"v":     &types.AttributeValueMemberN{Value: "1"},
		"edk":   &types.AttributeValueMemberB{Value: dataKey.CiphertextBlob},
		"nonce": &types.AttributeValueMemberB{Value: nonce},
		"ct":    &types.AttributeValueMemberB{Value: ct},
	}}, nil
}

func (d *TheorydbDriver) corruptStoredEnvelope(ctx context.Context, table, pk, sk, mode string) error {
	out, err := d.raw.GetItem(ctx, &dynamodb.GetItemInput{
		TableName: aws.String(table),
		Key: map[string]types.AttributeValue{
			"PK": &types.AttributeValueMemberS{Value: pk},
			"SK": &types.AttributeValueMemberS{Value: sk},
		},
		ConsistentRead: aws.Bool(true),
	})
	if err != nil {
		return err
	}
	envelope, ok := out.Item["secret"].(*types.AttributeValueMemberM)
	if !ok {
		return fmt.Errorf("stored migration row has no encrypted envelope")
	}
	switch mode {
	case "nonce":
		envelope.Value["nonce"] = &types.AttributeValueMemberB{Value: []byte("badnonce")}
	case "ciphertext":
		ciphertext, ok := envelope.Value["ct"].(*types.AttributeValueMemberB)
		if !ok || len(ciphertext.Value) == 0 {
			return fmt.Errorf("stored envelope has no ciphertext")
		}
		tampered := append([]byte(nil), ciphertext.Value...)
		tampered[len(tampered)-1] ^= 0xff
		envelope.Value["ct"] = &types.AttributeValueMemberB{Value: tampered}
	default:
		return fmt.Errorf("unsupported corrupt mode %q", mode)
	}
	_, err = d.raw.PutItem(ctx, &dynamodb.PutItemInput{
		TableName: aws.String(table),
		Item:      out.Item,
	})
	return err
}

// Migrate runs the runtime's schema migration from the fixed migration source
// shape into the scenario model. The encryption mode selects the provider
// available to the copy: scenario-configured, none, or a failing provider.
func (d *TheorydbDriver) Migrate(ctx context.Context, req MigrateRequest, targetTable string) error {
	if targetTable != migrationTargetTable {
		return fmt.Errorf("migrate supports target table %q only", migrationTargetTable)
	}
	var db core.ExtendedDB
	switch req.Encryption {
	case "", "deterministic":
		db = d.db
	case "none":
		without, err := d.databaseWithoutEncryption()
		if err != nil {
			return err
		}
		db = without
	case "failing":
		failing, err := d.failingDatabase()
		if err != nil {
			return err
		}
		db = failing
	default:
		return fmt.Errorf("unsupported migrate encryption mode %q", req.Encryption)
	}
	options := []schema.AutoMigrateOption{
		schema.WithTargetModel(&EncryptedRecord{}),
		schema.WithDataCopy(true),
		schema.WithContext(ctx),
	}
	if req.BatchSize > 0 {
		options = append(options, schema.WithBatchSize(req.BatchSize))
	}
	return db.AutoMigrateWithOptions(&migrationEncryptedSourceRecord{}, options...)
}

func (d *TheorydbDriver) databaseWithoutEncryption() (core.ExtendedDB, error) {
	if d.noEncryptionDB != nil {
		return d.noEncryptionDB, nil
	}
	cfg := session.Config{
		Region:           d.region,
		Endpoint:         d.endpoint,
		AWSConfigOptions: d.loadOptions,
	}
	var (
		db  core.ExtendedDB
		err error
	)
	if d.fake != nil {
		db, err = tabletheory.NewWithClient(cfg, d.fake)
	} else {
		db, err = tabletheory.New(cfg)
	}
	if err != nil {
		return nil, err
	}
	d.noEncryptionDB = db
	return db, nil
}

func (d *TheorydbDriver) failingDatabase() (core.ExtendedDB, error) {
	if d.failingDB != nil {
		return d.failingDB, nil
	}
	cfg := session.Config{
		Region:           d.region,
		Endpoint:         d.endpoint,
		KMSKeyARN:        "arn:aws:kms:us-east-1:111111111111:key/contract-failing",
		KMSClient:        failingKMS{},
		AWSConfigOptions: d.loadOptions,
	}
	var (
		db  core.ExtendedDB
		err error
	)
	if d.fake != nil {
		db, err = tabletheory.NewWithClient(cfg, d.fake)
	} else {
		db, err = tabletheory.New(cfg)
	}
	if err != nil {
		return nil, err
	}
	d.failingDB = db
	return db, nil
}

type failingKMS struct{}

func (failingKMS) GenerateDataKey(context.Context, *kms.GenerateDataKeyInput, ...func(*kms.Options)) (*kms.GenerateDataKeyOutput, error) {
	return nil, fmt.Errorf("contract failing KMS provider")
}

func (failingKMS) Decrypt(context.Context, *kms.DecryptInput, ...func(*kms.Options)) (*kms.DecryptOutput, error) {
	return nil, fmt.Errorf("contract failing KMS provider")
}
