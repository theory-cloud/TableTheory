package transaction

import (
	"context"
	"errors"
	"fmt"
	"io"
	"reflect"
	"strings"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb/types"

	"github.com/theory-cloud/tabletheory/internal/encryption"
	"github.com/theory-cloud/tabletheory/pkg/core"
	customerrors "github.com/theory-cloud/tabletheory/pkg/errors"
	"github.com/theory-cloud/tabletheory/pkg/model"
	"github.com/theory-cloud/tabletheory/pkg/query"
	"github.com/theory-cloud/tabletheory/pkg/session"
	pkgTypes "github.com/theory-cloud/tabletheory/pkg/types"
)

const maxTransactGetItems = 100

type dynamoTransactGetAPI interface {
	TransactGetItems(ctx context.Context, params *dynamodb.TransactGetItemsInput, optFns ...func(*dynamodb.Options)) (*dynamodb.TransactGetItemsOutput, error)
}

// TransactGet executes a DynamoDB TransactGetItems request against TableTheory
// models. The result slice preserves request order and reports missing items as
// Found=false instead of returning ErrItemNotFound.
func TransactGet(
	ctx context.Context,
	sess *session.Session,
	registry *model.Registry,
	converter *pkgTypes.Converter,
	requests []core.TransactGetRequest,
) ([]core.TransactGetResult, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	if len(requests) == 0 {
		return nil, errors.New("transact get requires at least one item")
	}
	if len(requests) > maxTransactGetItems {
		return nil, fmt.Errorf("%w: dynamodb TransactGetItems supports up to %d items", customerrors.ErrInvalidOperator, maxTransactGetItems)
	}
	if sess == nil {
		return nil, errors.New("dynamodb session is not configured")
	}
	if registry == nil {
		return nil, errors.New("model registry is not configured")
	}
	if converter == nil {
		converter = pkgTypes.NewConverter()
	}

	items, metas, err := buildTransactGetItems(registry, converter, requests)
	if err != nil {
		return nil, err
	}

	client, err := sess.API()
	if err != nil {
		return nil, err
	}
	return transactGetWithClient(ctx, client, sess, requests, metas, items)
}

func transactGetWithClient(
	ctx context.Context,
	client dynamoTransactGetAPI,
	sess *session.Session,
	requests []core.TransactGetRequest,
	metas []*model.Metadata,
	items []types.TransactGetItem,
) ([]core.TransactGetResult, error) {
	output, err := client.TransactGetItems(ctx, &dynamodb.TransactGetItemsInput{TransactItems: items})
	if err != nil {
		return nil, err
	}

	return collectTransactGetResults(ctx, sess, requests, metas, output.Responses)
}

func buildTransactGetItems(
	registry *model.Registry,
	converter *pkgTypes.Converter,
	requests []core.TransactGetRequest,
) ([]types.TransactGetItem, []*model.Metadata, error) {
	items := make([]types.TransactGetItem, 0, len(requests))
	metas := make([]*model.Metadata, 0, len(requests))
	for i, req := range requests {
		metadata, err := transactGetMetadata(registry, req.Model, i)
		if err != nil {
			return nil, nil, err
		}
		key, err := buildTransactGetKey(metadata, converter, req.Key)
		if err != nil {
			return nil, nil, fmt.Errorf("transact get item %d key: %w", i, err)
		}

		get := &types.Get{
			TableName: aws.String(metadata.TableName),
			Key:       key,
		}
		if len(req.Projection) > 0 {
			expr, names, err := buildTransactGetProjection(metadata, req.Projection)
			if err != nil {
				return nil, nil, fmt.Errorf("transact get item %d projection: %w", i, err)
			}
			get.ProjectionExpression = aws.String(expr)
			get.ExpressionAttributeNames = names
		}
		items = append(items, types.TransactGetItem{Get: get})
		metas = append(metas, metadata)
	}
	return items, metas, nil
}

func transactGetMetadata(registry *model.Registry, modelValue any, index int) (*model.Metadata, error) {
	if modelValue == nil {
		return nil, fmt.Errorf("transact get item %d model cannot be nil", index)
	}
	if err := registry.Register(modelValue); err != nil {
		return nil, err
	}
	return registry.GetMetadata(modelValue)
}

func collectTransactGetResults(
	ctx context.Context,
	sess *session.Session,
	requests []core.TransactGetRequest,
	metas []*model.Metadata,
	responses []types.ItemResponse,
) ([]core.TransactGetResult, error) {
	results := make([]core.TransactGetResult, len(requests))
	for i := range requests {
		if i >= len(responses) || len(responses[i].Item) == 0 {
			results[i] = core.TransactGetResult{Found: false}
			continue
		}

		item := cloneAttributeMap(responses[i].Item)
		if err := decryptTransactGetItem(ctx, sess, metas[i], item); err != nil {
			return nil, err
		}
		if requests[i].Dest != nil {
			if err := query.UnmarshalItem(item, requests[i].Dest); err != nil {
				return nil, err
			}
		}
		results[i] = core.TransactGetResult{Found: true}
	}

	return results, nil
}

func buildTransactGetKey(metadata *model.Metadata, converter *pkgTypes.Converter, key any) (map[string]types.AttributeValue, error) {
	if metadata == nil || metadata.PrimaryKey == nil || metadata.PrimaryKey.PartitionKey == nil {
		return nil, fmt.Errorf("model primary key is required")
	}
	if key == nil {
		return nil, fmt.Errorf("key cannot be nil")
	}

	if pair, ok := key.(core.KeyPair); ok {
		return keyFromValues(metadata, converter, pair.PartitionKey, pair.SortKey, true)
	}
	if m, ok := key.(map[string]any); ok {
		pk, pkOK := valueFromKeyMap(m, metadata.PrimaryKey.PartitionKey)
		var sk any
		skOK := true
		if metadata.PrimaryKey.SortKey != nil {
			sk, skOK = valueFromKeyMap(m, metadata.PrimaryKey.SortKey)
		}
		if !pkOK || !skOK {
			return nil, fmt.Errorf("missing primary key attributes")
		}
		return keyFromValues(metadata, converter, pk, sk, true)
	}

	value := reflect.ValueOf(key)
	if value.Kind() == reflect.Ptr {
		if value.IsNil() {
			return nil, fmt.Errorf("key pointer cannot be nil")
		}
		value = value.Elem()
	}
	if value.Kind() != reflect.Struct {
		if metadata.PrimaryKey.SortKey != nil {
			return nil, fmt.Errorf("composite key requires both partition and sort key")
		}
		return keyFromValues(metadata, converter, key, nil, false)
	}

	pk := value.Field(metadata.PrimaryKey.PartitionKey.Index).Interface()
	var sk any
	hasSK := false
	if metadata.PrimaryKey.SortKey != nil {
		sk = value.Field(metadata.PrimaryKey.SortKey.Index).Interface()
		hasSK = true
	}
	return keyFromValues(metadata, converter, pk, sk, hasSK)
}

func keyFromValues(metadata *model.Metadata, converter *pkgTypes.Converter, pk any, sk any, hasSK bool) (map[string]types.AttributeValue, error) {
	pkAV, err := converter.ToAttributeValue(pk)
	if err != nil {
		return nil, fmt.Errorf("failed to convert partition key: %w", err)
	}
	key := map[string]types.AttributeValue{
		metadata.PrimaryKey.PartitionKey.DBName: pkAV,
	}
	if metadata.PrimaryKey.SortKey != nil {
		if !hasSK || sk == nil {
			return nil, fmt.Errorf("sort key %s is required", metadata.PrimaryKey.SortKey.Name)
		}
		skAV, err := converter.ToAttributeValue(sk)
		if err != nil {
			return nil, fmt.Errorf("failed to convert sort key: %w", err)
		}
		key[metadata.PrimaryKey.SortKey.DBName] = skAV
	}
	return key, nil
}

func valueFromKeyMap(values map[string]any, field *model.FieldMetadata) (any, bool) {
	if field == nil {
		return nil, false
	}
	for _, name := range []string{field.Name, field.DBName} {
		if v, ok := values[name]; ok {
			return v, true
		}
	}
	for k, v := range values {
		if strings.EqualFold(k, field.Name) || strings.EqualFold(k, field.DBName) {
			return v, true
		}
	}
	return nil, false
}

func buildTransactGetProjection(metadata *model.Metadata, fields []string) (string, map[string]string, error) {
	names := make(map[string]string, len(fields))
	parts := make([]string, 0, len(fields))
	for i, field := range fields {
		dbName, err := resolveProjectionField(metadata, field)
		if err != nil {
			return "", nil, err
		}
		placeholder := fmt.Sprintf("#p%d", i)
		names[placeholder] = dbName
		parts = append(parts, placeholder)
	}
	return strings.Join(parts, ", "), names, nil
}

func resolveProjectionField(metadata *model.Metadata, field string) (string, error) {
	if metadata == nil {
		return "", fmt.Errorf("model metadata is required")
	}
	if meta := metadata.Fields[field]; meta != nil {
		return meta.DBName, nil
	}
	if meta := metadata.FieldsByDBName[field]; meta != nil {
		return meta.DBName, nil
	}
	for _, meta := range metadata.Fields {
		if meta == nil {
			continue
		}
		if strings.EqualFold(field, meta.Name) || strings.EqualFold(field, meta.DBName) {
			return meta.DBName, nil
		}
	}
	return "", fmt.Errorf("unknown projection field %s", field)
}

func decryptTransactGetItem(ctx context.Context, sess *session.Session, metadata *model.Metadata, item map[string]types.AttributeValue) error {
	if len(item) == 0 || !encryption.MetadataHasEncryptedFields(metadata) {
		return nil
	}
	if err := encryption.FailClosedIfEncryptedWithoutKMSKeyARN(sess, metadata); err != nil {
		return err
	}
	cfg := sess.Config()
	keyARN := ""
	var rng io.Reader
	if cfg != nil {
		keyARN = cfg.KMSKeyARN
		rng = cfg.EncryptionRand
	}
	var svc *encryption.Service
	if cfg != nil && cfg.KMSClient != nil {
		svc = encryption.NewServiceWithRand(keyARN, cfg.KMSClient, rng)
	} else {
		svc = encryption.NewServiceFromAWSConfigWithRand(keyARN, sess.AWSConfig(), rng)
	}
	for attrName, attrValue := range item {
		fieldMeta, ok := metadata.FieldsByDBName[attrName]
		if !ok || fieldMeta == nil || !fieldMeta.IsEncrypted {
			continue
		}
		decrypted, err := svc.DecryptAttributeValue(ctx, fieldMeta.DBName, attrValue)
		if err != nil {
			return &customerrors.EncryptedFieldError{
				Operation: "decrypt",
				Field:     fieldMeta.Name,
				Err:       err,
			}
		}
		item[attrName] = decrypted
	}
	return nil
}

func cloneAttributeMap(in map[string]types.AttributeValue) map[string]types.AttributeValue {
	out := make(map[string]types.AttributeValue, len(in))
	for k, v := range in {
		out[k] = v
	}
	return out
}
