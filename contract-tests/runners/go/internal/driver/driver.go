package driver

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strconv"
	"time"

	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/credentials"

	"github.com/theory-cloud/tabletheory"
	"github.com/theory-cloud/tabletheory/pkg/core"
	theorydbErrors "github.com/theory-cloud/tabletheory/pkg/errors"
	"github.com/theory-cloud/tabletheory/pkg/session"
)

type ErrorCode string

const (
	ErrItemNotFound      ErrorCode = "ErrItemNotFound"
	ErrConditionFailed   ErrorCode = "ErrConditionFailed"
	ErrInvalidModel      ErrorCode = "ErrInvalidModel"
	ErrMissingPrimaryKey ErrorCode = "ErrMissingPrimaryKey"
	ErrInvalidOperator   ErrorCode = "ErrInvalidOperator"
)

type Driver interface {
	Create(ctx context.Context, model string, item map[string]any, ifNotExists bool) error
	Get(ctx context.Context, model string, key map[string]any) (map[string]any, error)
	Update(ctx context.Context, model string, item map[string]any, fields []string) error
	Delete(ctx context.Context, model string, key map[string]any) error
}

func MapError(err error) ErrorCode {
	switch {
	case errors.Is(err, theorydbErrors.ErrItemNotFound):
		return ErrItemNotFound
	case errors.Is(err, theorydbErrors.ErrConditionFailed):
		return ErrConditionFailed
	case errors.Is(err, theorydbErrors.ErrInvalidModel):
		return ErrInvalidModel
	case errors.Is(err, theorydbErrors.ErrMissingPrimaryKey):
		return ErrMissingPrimaryKey
	case errors.Is(err, theorydbErrors.ErrInvalidOperator):
		return ErrInvalidOperator
	default:
		return ""
	}
}

type TheorydbDriver struct {
	db core.ExtendedDB
}

func NewTheorydbDriver() (*TheorydbDriver, error) {
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

	db, err := tabletheory.New(session.Config{
		Region:   region,
		Endpoint: endpoint,
		AWSConfigOptions: []func(*config.LoadOptions) error{
			config.WithCredentialsProvider(credentials.NewStaticCredentialsProvider("dummy", "dummy", "")),
			config.WithRegion(region),
		},
	})
	if err != nil {
		return nil, err
	}

	return &TheorydbDriver{db: db}, nil
}

func (d *TheorydbDriver) Create(ctx context.Context, model string, item map[string]any, ifNotExists bool) error {
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
	default:
		return nil, fmt.Errorf("%w: unknown model %q", theorydbErrors.ErrInvalidModel, model)
	}
}

func (d *TheorydbDriver) Update(ctx context.Context, model string, item map[string]any, fields []string) error {
	instance, err := modelFromMap(model, item)
	if err != nil {
		return err
	}
	return d.db.WithContext(ctx).Model(instance).Update(fields...)
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
	default:
		return fmt.Errorf("%w: unknown model %q", theorydbErrors.ErrInvalidModel, model)
	}
}

func keyValues(key map[string]any) (string, string, error) {
	pkVal, ok := key["PK"]
	if !ok {
		return "", "", fmt.Errorf("%w: PK is required", theorydbErrors.ErrMissingPrimaryKey)
	}
	skVal, ok := key["SK"]
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
	default:
		return nil, fmt.Errorf("%w: unknown model %q", theorydbErrors.ErrInvalidModel, model)
	}
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

// ---- Models (Go reference) ----

// User matches the v0.1 DMS fixture under `contract-tests/dms/v0.1/models/user.yml`.
type User struct {
	PK        string   `theorydb:"pk"`
	SK        string   `theorydb:"sk"`
	EmailHash string   `theorydb:"index:gsi-email,pk,omitempty"`
	Nickname  string   `theorydb:"omitempty"`
	Tags      []string `theorydb:"set,omitempty"`

	CreatedAt time.Time `theorydb:"created_at"`
	UpdatedAt time.Time `theorydb:"updated_at"`
	Version   int64     `theorydb:"version"`
	TTL       int64     `theorydb:"ttl,omitempty"`
}

func (User) TableName() string { return "users_contract" }

// Order matches the v0.1 DMS fixture under `contract-tests/dms/v0.1/models/order.yml`.
type Order struct {
	PK     string `theorydb:"pk"`
	SK     string `theorydb:"sk"`
	Status string `theorydb:"index:gsi-status,pk,omitempty"`
	Amount int64  `theorydb:"omitempty"`

	CreatedAt time.Time `theorydb:"created_at"`
	UpdatedAt time.Time `theorydb:"updated_at"`
	Version   int64     `theorydb:"version"`
	TTL       int64     `theorydb:"ttl,omitempty"`
}

func (Order) TableName() string { return "orders_contract" }

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
