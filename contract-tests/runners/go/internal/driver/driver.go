package driver

import (
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"os"
	"reflect"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/credentials"
	ddbtypes "github.com/aws/aws-sdk-go-v2/service/dynamodb/types"

	"github.com/theory-cloud/tabletheory/v2"
	"github.com/theory-cloud/tabletheory/v2/pkg/core"
	theorydbErrors "github.com/theory-cloud/tabletheory/v2/pkg/errors"
	"github.com/theory-cloud/tabletheory/v2/pkg/releasestate"
	"github.com/theory-cloud/tabletheory/v2/pkg/session"
	"github.com/theory-cloud/tabletheory/v2/pkg/testing/fakedb"
)

type ErrorCode string

const (
	ErrItemNotFound      ErrorCode = "ErrItemNotFound"
	ErrConditionFailed   ErrorCode = "ErrConditionFailed"
	ErrVersionConflict   ErrorCode = "ErrVersionConflict"
	ErrInvalidModel      ErrorCode = "ErrInvalidModel"
	ErrMissingPrimaryKey ErrorCode = "ErrMissingPrimaryKey"
	ErrInvalidOperator   ErrorCode = "ErrInvalidOperator"

	ErrEncryptionNotConfigured    ErrorCode = "ErrEncryptionNotConfigured"
	ErrEncryptedFieldNotQueryable ErrorCode = "ErrEncryptedFieldNotQueryable"
	ErrInvalidEncryptedEnvelope   ErrorCode = "ErrInvalidEncryptedEnvelope"

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
	Model string
	Item  map[string]any
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

type TheorydbDriver struct {
	db                      core.ExtendedDB
	deterministicEncryption bool
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

	cfg := session.Config{
		Region:   region,
		Endpoint: endpoint,
		AWSConfigOptions: []func(*config.LoadOptions) error{
			config.WithCredentialsProvider(credentials.NewStaticCredentialsProvider("dummy", "dummy", "")),
			config.WithRegion(region),
		},
	}
	deterministicEncryption := false
	if len(options) > 0 && options[0].Encryption.Provider != "" {
		kmsClient, rng, err := deterministicEncryptionForOptions(options[0].Encryption)
		if err != nil {
			return nil, err
		}
		cfg.KMSKeyARN = "arn:aws:kms:us-east-1:111111111111:key/contract-deterministic"
		cfg.KMSClient = kmsClient
		cfg.EncryptionRand = rng
		deterministicEncryption = true
	}

	db, err := tabletheory.New(cfg)
	if err != nil {
		return nil, err
	}

	if err := db.RegisterTypeConverter(reflect.TypeOf(DecimalString("")), decimalStringConverter{}); err != nil {
		return nil, err
	}

	return &TheorydbDriver{db: db, deterministicEncryption: deterministicEncryption}, nil
}

func NewFakeTheorydbDriver(options ...Options) (*TheorydbDriver, *fakedb.Fake, error) {
	cfg := session.Config{
		Region: "us-east-1",
	}
	deterministicEncryption := false
	if len(options) > 0 && options[0].Encryption.Provider != "" {
		kmsClient, rng, err := deterministicEncryptionForOptions(options[0].Encryption)
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

	if err := db.RegisterTypeConverter(reflect.TypeOf(DecimalString("")), decimalStringConverter{}); err != nil {
		return nil, nil, err
	}

	return &TheorydbDriver{db: db, deterministicEncryption: deterministicEncryption}, fake, nil
}

func (d *TheorydbDriver) Create(ctx context.Context, model string, item map[string]any, ifNotExists bool) error {
	if err := validateReleaseStateMetadataIfPresent(model, item); err != nil {
		return err
	}
	instance, err := modelFromMap(model, item)
	if err != nil {
		return err
	}

	q := d.db.WithContext(ctx).Model(instance)
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
		err := d.db.WithContext(ctx).Model(&User{}).Where("PK", "=", pk).Where("SK", "=", sk).First(&out)
		if err != nil {
			return nil, err
		}
		return normalizeUser(out), nil
	case "Order":
		var out Order
		err := d.db.WithContext(ctx).Model(&Order{}).Where("PK", "=", pk).Where("SK", "=", sk).First(&out)
		if err != nil {
			return nil, err
		}
		return normalizeOrder(out), nil
	case "NumberPrecision":
		var out NumberPrecision
		err := d.db.WithContext(ctx).Model(&NumberPrecision{}).Where("PK", "=", pk).Where("SK", "=", sk).First(&out)
		if err != nil {
			return nil, err
		}
		return normalizeNumberPrecision(out), nil
	case "TypeMatrix":
		var out TypeMatrix
		err := d.db.WithContext(ctx).Model(&TypeMatrix{}).Where("PK", "=", pk).Where("SK", "=", sk).First(&out)
		if err != nil {
			return nil, err
		}
		return normalizeTypeMatrix(out), nil
	case "SnakeCaseRecord":
		var out SnakeCaseRecord
		err := d.db.WithContext(ctx).Model(&SnakeCaseRecord{}).Where("pk", "=", pk).Where("sk", "=", sk).First(&out)
		if err != nil {
			return nil, err
		}
		return normalizeSnakeCaseRecord(out), nil
	case "EncryptedRecord":
		var out EncryptedRecord
		err := d.db.WithContext(ctx).Model(&EncryptedRecord{}).Where("PK", "=", pk).Where("SK", "=", sk).First(&out)
		if err != nil {
			return nil, err
		}
		return normalizeEncryptedRecord(out), nil
	case "ReleaseStateActual":
		var out ReleaseStateActual
		err := d.db.WithContext(ctx).Model(&ReleaseStateActual{}).Where("PK", "=", pk).Where("SK", "=", sk).First(&out)
		if err != nil {
			return nil, err
		}
		return normalizeReleaseStateActual(out), nil
	case "ReleaseStateEvent":
		var out ReleaseStateEvent
		err := d.db.WithContext(ctx).Model(&ReleaseStateEvent{}).Where("PK", "=", pk).Where("SK", "=", sk).First(&out)
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
	return d.db.WithContext(ctx).Model(instance).Update(fields...)
}

func (d *TheorydbDriver) Save(ctx context.Context, model string, item map[string]any) error {
	if err := validateReleaseStateMetadataIfPresent(model, item); err != nil {
		return err
	}
	instance, err := modelFromMap(model, item)
	if err != nil {
		return err
	}
	return d.db.WithContext(ctx).Model(instance).CreateOrUpdate()
}

func (d *TheorydbDriver) Delete(ctx context.Context, model string, key map[string]any) error {
	pk, sk, err := keyValues(key)
	if err != nil {
		return err
	}

	switch model {
	case "User":
		return d.db.WithContext(ctx).Model(&User{}).Where("PK", "=", pk).Where("SK", "=", sk).Delete()
	case "Order":
		return d.db.WithContext(ctx).Model(&Order{}).Where("PK", "=", pk).Where("SK", "=", sk).Delete()
	case "NumberPrecision":
		return d.db.WithContext(ctx).Model(&NumberPrecision{}).Where("PK", "=", pk).Where("SK", "=", sk).Delete()
	case "TypeMatrix":
		return d.db.WithContext(ctx).Model(&TypeMatrix{}).Where("PK", "=", pk).Where("SK", "=", sk).Delete()
	case "SnakeCaseRecord":
		return d.db.WithContext(ctx).Model(&SnakeCaseRecord{}).Where("pk", "=", pk).Where("sk", "=", sk).Delete()
	case "EncryptedRecord":
		return d.db.WithContext(ctx).Model(&EncryptedRecord{}).Where("PK", "=", pk).Where("SK", "=", sk).Delete()
	case "ReleaseStateActual":
		return d.db.WithContext(ctx).Model(&ReleaseStateActual{}).Where("PK", "=", pk).Where("SK", "=", sk).Delete()
	case "ReleaseStateEvent":
		return d.db.WithContext(ctx).Model(&ReleaseStateEvent{}).Where("PK", "=", pk).Where("SK", "=", sk).Delete()
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
	q := d.db.WithContext(ctx).Model(instance)
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
	getter, ok := d.db.(core.TransactGetter)
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
		err := d.db.WithContext(ctx).Model(&User{}).BatchGet(keyPairs, &out)
		return ReadResult{Items: normalizeUsers(out)}, err
	case "Order":
		var out []Order
		err := d.db.WithContext(ctx).Model(&Order{}).BatchGet(keyPairs, &out)
		return ReadResult{Items: normalizeOrders(out)}, err
	case "NumberPrecision":
		var out []NumberPrecision
		err := d.db.WithContext(ctx).Model(&NumberPrecision{}).BatchGet(keyPairs, &out)
		return ReadResult{Items: normalizeNumberPrecisions(out)}, err
	case "TypeMatrix":
		var out []TypeMatrix
		err := d.db.WithContext(ctx).Model(&TypeMatrix{}).BatchGet(keyPairs, &out)
		return ReadResult{Items: normalizeTypeMatrices(out)}, err
	case "SnakeCaseRecord":
		var out []SnakeCaseRecord
		err := d.db.WithContext(ctx).Model(&SnakeCaseRecord{}).BatchGet(keyPairs, &out)
		return ReadResult{Items: normalizeSnakeCaseRecords(out)}, err
	case "EncryptedRecord":
		var out []EncryptedRecord
		err := d.db.WithContext(ctx).Model(&EncryptedRecord{}).BatchGet(keyPairs, &out)
		return ReadResult{Items: normalizeEncryptedRecords(out)}, err
	case "ReleaseStateActual":
		var out []ReleaseStateActual
		err := d.db.WithContext(ctx).Model(&ReleaseStateActual{}).BatchGet(keyPairs, &out)
		return ReadResult{Items: normalizeReleaseStateActuals(out)}, err
	case "ReleaseStateEvent":
		var out []ReleaseStateEvent
		err := d.db.WithContext(ctx).Model(&ReleaseStateEvent{}).BatchGet(keyPairs, &out)
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
	return d.db.WithContext(ctx).Model(empty).BatchWrite(putItems, deleteKeys)
}

func (d *TheorydbDriver) TransactWrite(ctx context.Context, model string, actions []TransactWriteAction) error {
	return d.db.TransactWrite(ctx, func(tx core.TransactionBuilder) error {
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
	if typ.Kind() == reflect.Ptr {
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

// decimalStringConverter stores exact DynamoDB N decimal strings for generated DecimalString fields.
type decimalStringConverter struct{}

func (decimalStringConverter) ToAttributeValue(value any) (ddbtypes.AttributeValue, error) {
	decimal, ok := value.(DecimalString)
	if !ok {
		return nil, fmt.Errorf("expected DecimalString, got %T", value)
	}
	return &ddbtypes.AttributeValueMemberN{Value: string(decimal)}, nil
}

func (decimalStringConverter) FromAttributeValue(av ddbtypes.AttributeValue, target any) error {
	number, ok := av.(*ddbtypes.AttributeValueMemberN)
	if !ok {
		return fmt.Errorf("expected DynamoDB N for DecimalString, got %T", av)
	}
	switch out := target.(type) {
	case *DecimalString:
		*out = DecimalString(number.Value)
		return nil
	default:
		return fmt.Errorf("expected *DecimalString, got %T", target)
	}
}

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
		n.LargeInteger = DecimalString(fmt.Sprintf("%v", v))
	}
	if v, ok := item["preciseDecimal"]; ok {
		n.PreciseDecimal = DecimalString(fmt.Sprintf("%v", v))
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
		"createdAt": u.CreatedAt.Format(time.RFC3339Nano),
		"updatedAt": u.UpdatedAt.Format(time.RFC3339Nano),
		"version":   u.Version,
		"ttl":       u.TTL,
	}
	return out
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
		"largeInteger":   string(n.LargeInteger),
		"preciseDecimal": string(n.PreciseDecimal),
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
