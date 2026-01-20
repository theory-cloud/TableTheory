// Package lease provides a small, correctness-first DynamoDB lease/lock helper.
//
// It is designed for ISR-style regeneration locks (FaceTheory) and similar distributed coordination needs.
package lease

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb/types"
	"github.com/google/uuid"
)

type DynamoDBLeaseAPI interface {
	PutItem(ctx context.Context, params *dynamodb.PutItemInput, optFns ...func(*dynamodb.Options)) (*dynamodb.PutItemOutput, error)
	UpdateItem(ctx context.Context, params *dynamodb.UpdateItemInput, optFns ...func(*dynamodb.Options)) (*dynamodb.UpdateItemOutput, error)
	DeleteItem(ctx context.Context, params *dynamodb.DeleteItemInput, optFns ...func(*dynamodb.Options)) (*dynamodb.DeleteItemOutput, error)
}

type Key struct {
	PK string
	SK string
}

type Lease struct {
	Key       Key
	Token     string
	ExpiresAt int64
}

type Manager struct {
	client DynamoDBLeaseAPI

	tableName string

	pkAttr string
	skAttr string

	tokenAttr     string
	expiresAtAttr string
	ttlAttr       string

	now           func() time.Time
	token         func() string
	ttlBuffer     time.Duration
	lockSortKey   string
	includeTTL    bool
	validateInput bool
}

type Option func(*Manager)

const (
	DefaultPKAttribute         = "pk"
	DefaultSKAttribute         = "sk"
	DefaultTokenAttribute      = "lease_token"
	DefaultExpiresAtAttribute  = "lease_expires_at"
	DefaultTTLAttribute        = "ttl"
	DefaultLockSortKey         = "LOCK"
	DefaultTTLBuffer           = time.Hour
	defaultValidateInput       = true
	defaultIncludeTTLAttribute = true
)

func WithNow(now func() time.Time) Option {
	return func(m *Manager) {
		if now != nil {
			m.now = now
		}
	}
}

func WithTokenGenerator(token func() string) Option {
	return func(m *Manager) {
		if token != nil {
			m.token = token
		}
	}
}

func WithTTLBuffer(buffer time.Duration) Option {
	return func(m *Manager) {
		m.ttlBuffer = buffer
	}
}

func WithLockSortKey(lockSortKey string) Option {
	return func(m *Manager) {
		if lockSortKey != "" {
			m.lockSortKey = lockSortKey
		}
	}
}

func WithKeyAttributeNames(pkAttr, skAttr string) Option {
	return func(m *Manager) {
		if pkAttr != "" {
			m.pkAttr = pkAttr
		}
		if skAttr != "" {
			m.skAttr = skAttr
		}
	}
}

func WithLeaseAttributeNames(tokenAttr, expiresAtAttr, ttlAttr string) Option {
	return func(m *Manager) {
		if tokenAttr != "" {
			m.tokenAttr = tokenAttr
		}
		if expiresAtAttr != "" {
			m.expiresAtAttr = expiresAtAttr
		}
		if ttlAttr != "" {
			m.ttlAttr = ttlAttr
		}
	}
}

func WithIncludeTTL(include bool) Option {
	return func(m *Manager) {
		m.includeTTL = include
	}
}

func WithValidateInput(validate bool) Option {
	return func(m *Manager) {
		m.validateInput = validate
	}
}

func NewManager(client DynamoDBLeaseAPI, tableName string, opts ...Option) (*Manager, error) {
	if client == nil {
		return nil, fmt.Errorf("lease manager: client is required")
	}
	if tableName == "" {
		return nil, fmt.Errorf("lease manager: tableName is required")
	}

	m := &Manager{
		client: client,

		tableName: tableName,

		pkAttr: DefaultPKAttribute,
		skAttr: DefaultSKAttribute,

		tokenAttr:     DefaultTokenAttribute,
		expiresAtAttr: DefaultExpiresAtAttribute,
		ttlAttr:       DefaultTTLAttribute,

		now:           time.Now,
		token:         uuid.NewString,
		ttlBuffer:     DefaultTTLBuffer,
		lockSortKey:   DefaultLockSortKey,
		includeTTL:    defaultIncludeTTLAttribute,
		validateInput: defaultValidateInput,
	}

	for _, opt := range opts {
		if opt != nil {
			opt(m)
		}
	}

	return m, nil
}

func (m *Manager) Acquire(ctx context.Context, pk string, duration time.Duration) (*Lease, error) {
	if m == nil {
		return nil, fmt.Errorf("lease manager: nil receiver")
	}
	return m.AcquireKey(ctx, Key{PK: pk, SK: m.lockSortKey}, duration)
}

func (m *Manager) AcquireKey(ctx context.Context, key Key, duration time.Duration) (*Lease, error) {
	if m == nil {
		return nil, fmt.Errorf("lease manager: nil receiver")
	}
	if m.client == nil {
		return nil, fmt.Errorf("lease manager: client is nil")
	}

	if m.validateInput {
		if stringsEmpty(key.PK) || stringsEmpty(key.SK) {
			return nil, fmt.Errorf("lease manager: PK and SK are required")
		}
		if duration <= 0 {
			return nil, fmt.Errorf("lease manager: duration must be > 0")
		}
	}

	now := m.now()
	nowUnix := now.Unix()
	expiresAt := now.Add(duration).Unix()
	token := m.token()

	item := map[string]types.AttributeValue{
		m.pkAttr: &types.AttributeValueMemberS{Value: key.PK},
		m.skAttr: &types.AttributeValueMemberS{Value: key.SK},

		m.tokenAttr:     &types.AttributeValueMemberS{Value: token},
		m.expiresAtAttr: &types.AttributeValueMemberN{Value: strconv.FormatInt(expiresAt, 10)},
	}

	if m.includeTTL && m.ttlBuffer > 0 && m.ttlAttr != "" {
		ttl := expiresAt + int64(m.ttlBuffer.Seconds())
		item[m.ttlAttr] = &types.AttributeValueMemberN{Value: strconv.FormatInt(ttl, 10)}
	}

	input := &dynamodb.PutItemInput{
		TableName: aws.String(m.tableName),
		Item:      item,
		ConditionExpression: aws.String(
			"attribute_not_exists(#pk) OR #lease_expires_at <= :now",
		),
		ExpressionAttributeNames: map[string]string{
			"#pk":               m.pkAttr,
			"#lease_expires_at": m.expiresAtAttr,
		},
		ExpressionAttributeValues: map[string]types.AttributeValue{
			":now": &types.AttributeValueMemberN{Value: strconv.FormatInt(nowUnix, 10)},
		},
	}

	_, err := m.client.PutItem(ctx, input)
	if err != nil {
		if isConditionalCheckFailed(err) {
			return nil, &LeaseHeldError{Key: key}
		}
		return nil, fmt.Errorf("lease manager: acquire failed: %w", err)
	}

	return &Lease{
		Key:       key,
		Token:     token,
		ExpiresAt: expiresAt,
	}, nil
}

func (m *Manager) Refresh(ctx context.Context, lease Lease, duration time.Duration) (*Lease, error) {
	if m == nil {
		return nil, fmt.Errorf("lease manager: nil receiver")
	}
	if m.client == nil {
		return nil, fmt.Errorf("lease manager: client is nil")
	}

	if m.validateInput {
		if stringsEmpty(lease.Key.PK) || stringsEmpty(lease.Key.SK) {
			return nil, fmt.Errorf("lease manager: PK and SK are required")
		}
		if stringsEmpty(lease.Token) {
			return nil, fmt.Errorf("lease manager: token is required")
		}
		if duration <= 0 {
			return nil, fmt.Errorf("lease manager: duration must be > 0")
		}
	}

	now := m.now()
	nowUnix := now.Unix()
	expiresAt := now.Add(duration).Unix()

	key := map[string]types.AttributeValue{
		m.pkAttr: &types.AttributeValueMemberS{Value: lease.Key.PK},
		m.skAttr: &types.AttributeValueMemberS{Value: lease.Key.SK},
	}

	names := map[string]string{
		"#lease_token":      m.tokenAttr,
		"#lease_expires_at": m.expiresAtAttr,
	}
	values := map[string]types.AttributeValue{
		":token": &types.AttributeValueMemberS{Value: lease.Token},
		":now":   &types.AttributeValueMemberN{Value: strconv.FormatInt(nowUnix, 10)},
		":exp":   &types.AttributeValueMemberN{Value: strconv.FormatInt(expiresAt, 10)},
	}

	updateExpr := "SET #lease_expires_at = :exp"
	if m.includeTTL && m.ttlBuffer > 0 && m.ttlAttr != "" {
		ttl := expiresAt + int64(m.ttlBuffer.Seconds())
		names["#ttl"] = m.ttlAttr
		values[":ttl"] = &types.AttributeValueMemberN{Value: strconv.FormatInt(ttl, 10)}
		updateExpr = updateExpr + ", #ttl = :ttl"
	}

	input := &dynamodb.UpdateItemInput{
		TableName: aws.String(m.tableName),
		Key:       key,
		UpdateExpression: aws.String(
			updateExpr,
		),
		ConditionExpression: aws.String(
			"#lease_token = :token AND #lease_expires_at > :now",
		),
		ExpressionAttributeNames:  names,
		ExpressionAttributeValues: values,
	}

	_, err := m.client.UpdateItem(ctx, input)
	if err != nil {
		if isConditionalCheckFailed(err) {
			return nil, &LeaseNotOwnedError{Key: lease.Key}
		}
		return nil, fmt.Errorf("lease manager: refresh failed: %w", err)
	}

	out := lease
	out.ExpiresAt = expiresAt
	return &out, nil
}

func (m *Manager) Release(ctx context.Context, lease Lease) error {
	if m == nil {
		return fmt.Errorf("lease manager: nil receiver")
	}
	if m.client == nil {
		return fmt.Errorf("lease manager: client is nil")
	}

	if m.validateInput {
		if stringsEmpty(lease.Key.PK) || stringsEmpty(lease.Key.SK) {
			return fmt.Errorf("lease manager: PK and SK are required")
		}
		if stringsEmpty(lease.Token) {
			return fmt.Errorf("lease manager: token is required")
		}
	}

	key := map[string]types.AttributeValue{
		m.pkAttr: &types.AttributeValueMemberS{Value: lease.Key.PK},
		m.skAttr: &types.AttributeValueMemberS{Value: lease.Key.SK},
	}

	input := &dynamodb.DeleteItemInput{
		TableName: aws.String(m.tableName),
		Key:       key,
		ConditionExpression: aws.String(
			"#lease_token = :token",
		),
		ExpressionAttributeNames: map[string]string{
			"#lease_token": m.tokenAttr,
		},
		ExpressionAttributeValues: map[string]types.AttributeValue{
			":token": &types.AttributeValueMemberS{Value: lease.Token},
		},
	}

	_, err := m.client.DeleteItem(ctx, input)
	if err != nil {
		if isConditionalCheckFailed(err) {
			return nil // best-effort
		}
		return fmt.Errorf("lease manager: release failed: %w", err)
	}

	return nil
}

func isConditionalCheckFailed(err error) bool {
	var cfe *types.ConditionalCheckFailedException
	return errors.As(err, &cfe)
}

func stringsEmpty(s string) bool {
	return len(s) == 0
}
