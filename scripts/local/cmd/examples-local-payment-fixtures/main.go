// Command examples-local-payment-fixtures provisions the local DynamoDB tables
// the Lambda and payment examples use, seeds fixture rows through the real
// TableTheory models, publishes a reconciliation object to the local S3
// emulator, and reads the stored state back for harness assertions.
//
// It is harness tooling, not an example: every write goes through the real
// library against the local profile, and the tool refuses to run with anything
// but the static dummy credentials.
package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"net/url"
	"os"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb"
	ddbtypes "github.com/aws/aws-sdk-go-v2/service/dynamodb/types"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	s3types "github.com/aws/aws-sdk-go-v2/service/s3/types"

	"github.com/theory-cloud/tabletheory/v6"
	payment "github.com/theory-cloud/tabletheory/v6/examples/payment"
	"github.com/theory-cloud/tabletheory/v6/examples/payment/utils"
	"github.com/theory-cloud/tabletheory/v6/pkg/core"
)

const (
	dummyAccessKeyID     = "dummy"
	dummySecretAccessKey = "dummy"
	defaultMerchantID    = "merchant_local"
	defaultWebhookURL    = "https://127.0.0.1:8443/hook"
	defaultBucket        = "reconciliation-local"
	defaultObjectKey     = "reconciliation/merchant-1/2026-11-01.csv"
	defaultUsersTable    = "lambda_users"
)

// lambdaPayment mirrors examples/lambda's Payment model. The harness cannot
// import a package main, so the schema is repeated here; the Lambda row fails
// if the two drift.
type lambdaPayment struct {
	CreatedAt time.Time `theorydb:"created_at" json:"created_at"`
	UpdatedAt time.Time `theorydb:"updated_at" json:"updated_at"`
	ID        string    `theorydb:"pk" json:"id"`
	PartnerID string    `theorydb:"index:partner-index,pk" json:"partner_id"`
	Currency  string    `json:"currency"`
	Status    string    `json:"status"`
	Amount    int64     `json:"amount"`
}

func (lambdaPayment) TableName() string { return "lambda_payments" }

// exportJob mirrors the query handler's ExportJob model.
type exportJob struct {
	CreatedAt  time.Time              `theorydb:"created_at" json:"created_at"`
	UpdatedAt  time.Time              `theorydb:"updated_at" json:"updated_at"`
	Metadata   map[string]interface{} `theorydb:"json" json:"metadata,omitempty"`
	ID         string                 `theorydb:"pk" json:"id"`
	MerchantID string                 `theorydb:"index:gsi-merchant,pk" json:"merchant_id"`
	Status     string                 `theorydb:"index:gsi-status,pk" json:"status"`
	Format     string                 `json:"format"`
	ResultURL  string                 `json:"result_url,omitempty"`
	Error      string                 `json:"error,omitempty"`
	ExpiresAt  int64                  `theorydb:"ttl" json:"expires_at"`
}

func (exportJob) TableName() string { return "ExportJobs" }

// lambdaUser is the TypeScript Lambda example's User table shape. The handler
// reads USERS_TABLE, which the harness points at this dedicated table so the
// row does not depend on ts/examples/local.ts.
type lambdaUser struct {
	PK        string    `theorydb:"pk,attr:PK" json:"PK"`
	SK        string    `theorydb:"sk,attr:SK" json:"SK"`
	Nickname  string    `json:"nickname,omitempty"`
	CreatedAt time.Time `theorydb:"created_at" json:"createdAt"`
	UpdatedAt time.Time `theorydb:"updated_at" json:"updatedAt"`
	Version   int       `theorydb:"version" json:"version"`
}

func (lambdaUser) TableName() string { return defaultUsersTable }

type options struct {
	mode       string
	merchant   string
	webhookURL string
	bucket     string
	key        string
	userTable  string
}

type verification struct {
	Merchant       string   `json:"merchant"`
	PaymentIDs     []string `json:"payment_ids"`
	PaymentStatus  []string `json:"payment_status"`
	Payments       int      `json:"payments"`
	Transactions   int      `json:"transactions"`
	Webhooks       int      `json:"webhooks"`
	UsageCounters  int      `json:"usage_counters"`
	UsageCounts    []int64  `json:"usage_counts"`
	Settlements    int      `json:"settlements"`
	SettlementIDs  []string `json:"settlement_ids"`
	AuditLogs      int      `json:"audit_logs"`
	ExportJobs     int      `json:"export_jobs"`
	IdempotencyRow int      `json:"idempotency_records"`
}

type publishedObject struct {
	Bucket string `json:"bucket"`
	Key    string `json:"key"`
	ETag   string `json:"etag"`
	Size   int    `json:"size"`
}

func main() {
	var opts options
	flag.StringVar(&opts.mode, "mode", "", "provision|seed-merchant|seed-user|verify-user|publish-reconcile|verify-payment")
	flag.StringVar(&opts.merchant, "merchant", defaultMerchantID, "merchant id to seed or verify")
	flag.StringVar(&opts.webhookURL, "webhook-url", defaultWebhookURL, "merchant webhook URL")
	flag.StringVar(&opts.bucket, "bucket", defaultBucket, "S3 bucket for the reconciliation object")
	flag.StringVar(&opts.key, "key", defaultObjectKey, "S3 object key for the reconciliation object")
	flag.Parse()

	if err := run(context.Background(), opts); err != nil {
		fmt.Fprintf(os.Stderr, "examples-local-payment-fixtures: FAIL: %v\n", err)
		os.Exit(1)
	}
}

func run(ctx context.Context, opts options) error {
	if err := assertHarnessCredentials(ctx); err != nil {
		return err
	}

	switch opts.mode {
	case "provision":
		db, err := newDB()
		if err != nil {
			return err
		}
		return provision(db)
	case "seed-merchant":
		db, err := newDB()
		if err != nil {
			return err
		}
		if err := provision(db); err != nil {
			return err
		}
		return seedMerchant(db, opts.merchant, opts.webhookURL)
	case "seed-user":
		db, err := newDB()
		if err != nil {
			return err
		}
		if err := provision(db); err != nil {
			return err
		}
		return seedUser(db)
	case "verify-user":
		return verifyUser(ctx)
	case "publish-reconcile":
		return publishReconcile(ctx, opts.bucket, opts.key)
	case "verify-payment":
		db, err := newDB()
		if err != nil {
			return err
		}
		return verifyPayment(db, opts.merchant)
	default:
		return fmt.Errorf("unknown mode %q", opts.mode)
	}
}

func newDB() (core.ExtendedDB, error) {
	return tabletheory.New(tabletheory.Config{
		Region:    os.Getenv("AWS_REGION"),
		KMSKeyARN: os.Getenv("KMS_KEY_ARN"),
	})
}

func provision(db core.ExtendedDB) error {
	models := []any{
		&lambdaPayment{},
		&lambdaUser{},
		&payment.Payment{},
		&payment.Transaction{},
		&payment.Merchant{},
		&payment.UsageCounter{},
		&payment.IdempotencyRecord{},
		&payment.Webhook{},
		&payment.Customer{},
		&payment.Settlement{},
		&utils.AuditLog{},
		&exportJob{},
	}
	for _, model := range models {
		if err := db.EnsureTable(model); err != nil {
			return fmt.Errorf("provision %T: %w", model, err)
		}
	}
	fmt.Printf("provisioned %d local tables\n", len(models))
	return nil
}

func seedMerchant(db core.ExtendedDB, merchantID, webhookURL string) error {
	merchant := &payment.Merchant{
		ID:         merchantID,
		Name:       "Local Harness Merchant",
		Email:      "merchant@local.test",
		Status:     payment.MerchantStatusActive,
		WebhookURL: webhookURL,
		RateLimits: payment.RateLimits{
			PaymentsPerMinute: 10,
			PaymentsPerDay:    100,
			MaxPaymentAmount:  100000,
		},
		CreatedAt: time.Now().UTC(),
		UpdatedAt: time.Now().UTC(),
		Version:   1,
	}
	if err := db.Model(merchant).CreateOrUpdate(); err != nil {
		return fmt.Errorf("seed merchant %s: %w", merchantID, err)
	}
	fmt.Printf("seeded merchant %s webhook=%s\n", merchantID, webhookURL)
	return nil
}

func seedUser(db core.ExtendedDB) error {
	now := time.Now().UTC()
	user := &lambdaUser{
		PK:        "USER#local",
		SK:        "PROFILE",
		Nickname:  "Alice",
		CreatedAt: now,
		UpdatedAt: now,
		Version:   1,
	}
	if err := db.Model(user).CreateOrUpdate(); err != nil {
		return fmt.Errorf("seed %s: %w", defaultUsersTable, err)
	}
	fmt.Printf("seeded %s USER#local\n", defaultUsersTable)
	return nil
}

func verifyUser(ctx context.Context) error {
	cfg, err := config.LoadDefaultConfig(ctx)
	if err != nil {
		return fmt.Errorf("load AWS config: %w", err)
	}
	client := dynamodb.NewFromConfig(cfg)
	out, err := client.GetItem(ctx, &dynamodb.GetItemInput{
		TableName: aws.String(defaultUsersTable),
		Key: map[string]ddbtypes.AttributeValue{
			"PK": &ddbtypes.AttributeValueMemberS{Value: "USER#local"},
			"SK": &ddbtypes.AttributeValueMemberS{Value: "PROFILE"},
		},
		ConsistentRead: aws.Bool(true),
	})
	if err != nil {
		return fmt.Errorf("read %s: %w", defaultUsersTable, err)
	}
	result := map[string]any{"found": len(out.Item) > 0}
	for _, name := range []string{"PK", "SK", "nickname"} {
		if value, ok := out.Item[name].(*ddbtypes.AttributeValueMemberS); ok {
			result[name] = value.Value
		}
	}
	encoded, err := json.Marshal(result)
	if err != nil {
		return fmt.Errorf("marshal user: %w", err)
	}
	fmt.Println(string(encoded))
	return nil
}

func publishReconcile(ctx context.Context, bucket, key string) error {
	cfg, err := config.LoadDefaultConfig(ctx)
	if err != nil {
		return fmt.Errorf("load AWS config: %w", err)
	}
	client := s3.NewFromConfig(cfg, func(o *s3.Options) {
		// The local S3 emulator serves buckets from one endpoint.
		o.UsePathStyle = true
	})

	if _, err := client.CreateBucket(ctx, &s3.CreateBucketInput{Bucket: aws.String(bucket)}); err != nil {
		var exists *s3types.BucketAlreadyOwnedByYou
		var alsoExists *s3types.BucketAlreadyExists
		if !errors.As(err, &exists) && !errors.As(err, &alsoExists) {
			return fmt.Errorf("create bucket %s: %w", bucket, err)
		}
	}

	body := []byte("payment_id,transaction_id,status,fee,processed_date,settlement_date\n" +
		"pay_1,tx_1,succeeded,100,2026-11-01,2026-11-01\n" +
		"pay_2,tx_2,succeeded,250,2026-11-01,2026-11-01\n")
	if _, err := client.PutObject(ctx, &s3.PutObjectInput{
		Bucket: aws.String(bucket),
		Key:    aws.String(key),
		Body:   bytes.NewReader(body),
	}); err != nil {
		return fmt.Errorf("put %s/%s: %w", bucket, key, err)
	}

	head, err := client.HeadObject(ctx, &s3.HeadObjectInput{
		Bucket: aws.String(bucket),
		Key:    aws.String(key),
	})
	if err != nil {
		return fmt.Errorf("head %s/%s: %w", bucket, key, err)
	}

	out, err := json.Marshal(publishedObject{
		Bucket: bucket,
		Key:    key,
		ETag:   aws.ToString(head.ETag),
		Size:   int(aws.ToInt64(head.ContentLength)),
	})
	if err != nil {
		return fmt.Errorf("marshal published object: %w", err)
	}
	fmt.Println(string(out))
	return nil
}

func verifyPayment(db core.ExtendedDB, merchantID string) error {
	var payments []*payment.Payment
	if err := db.Model(&payment.Payment{}).
		Index("gsi-merchant").
		Where("MerchantID", "=", merchantID).
		All(&payments); err != nil {
		return fmt.Errorf("query payments: %w", err)
	}

	var transactions []*payment.Transaction
	if err := db.Model(&payment.Transaction{}).Scan(&transactions); err != nil {
		return fmt.Errorf("scan transactions: %w", err)
	}

	var webhooks []*payment.Webhook
	if err := db.Model(&payment.Webhook{}).
		Index("gsi-merchant").
		Where("MerchantID", "=", merchantID).
		All(&webhooks); err != nil {
		return fmt.Errorf("query webhooks: %w", err)
	}

	var counters []*payment.UsageCounter
	if err := db.Model(&payment.UsageCounter{}).Scan(&counters); err != nil {
		return fmt.Errorf("scan usage counters: %w", err)
	}

	var settlements []*payment.Settlement
	if err := db.Model(&payment.Settlement{}).
		Index("gsi-merchant").
		Where("MerchantID", "=", merchantID).
		All(&settlements); err != nil {
		return fmt.Errorf("query settlements: %w", err)
	}

	var auditLogs []*utils.AuditLog
	if err := db.Model(&utils.AuditLog{}).Scan(&auditLogs); err != nil {
		return fmt.Errorf("scan audit logs: %w", err)
	}

	var exports []*exportJob
	if err := db.Model(&exportJob{}).Scan(&exports); err != nil {
		return fmt.Errorf("scan export jobs: %w", err)
	}

	var idempotency []*payment.IdempotencyRecord
	if err := db.Model(&payment.IdempotencyRecord{}).Scan(&idempotency); err != nil {
		return fmt.Errorf("scan idempotency records: %w", err)
	}

	result := verification{
		Merchant:       merchantID,
		Payments:       len(payments),
		Transactions:   len(transactions),
		Webhooks:       len(webhooks),
		UsageCounters:  len(counters),
		Settlements:    len(settlements),
		AuditLogs:      len(auditLogs),
		ExportJobs:     len(exports),
		IdempotencyRow: len(idempotency),
	}
	for _, record := range payments {
		result.PaymentIDs = append(result.PaymentIDs, record.ID)
		result.PaymentStatus = append(result.PaymentStatus, record.Status)
	}
	for _, record := range settlements {
		result.SettlementIDs = append(result.SettlementIDs, record.ID)
	}
	for _, counter := range counters {
		result.UsageCounts = append(result.UsageCounts, counter.Count)
	}

	out, err := json.Marshal(result)
	if err != nil {
		return fmt.Errorf("marshal verification: %w", err)
	}
	fmt.Println(string(out))
	return nil
}

func assertHarnessCredentials(ctx context.Context) error {
	cfg, err := config.LoadDefaultConfig(ctx)
	if err != nil {
		return fmt.Errorf("load AWS config: %w", err)
	}
	creds, err := cfg.Credentials.Retrieve(ctx)
	if err != nil {
		return fmt.Errorf("resolve AWS credentials: %w", err)
	}
	if creds.AccessKeyID != dummyAccessKeyID || creds.SecretAccessKey != dummySecretAccessKey {
		return fmt.Errorf("resolved credentials are not the harness dummy static keys; refusing to run")
	}
	for _, name := range []string{
		"AWS_ENDPOINT_URL_DYNAMODB",
		"DYNAMODB_ENDPOINT",
		"AWS_ENDPOINT_URL_S3",
		"S3_ENDPOINT",
	} {
		value := strings.TrimSpace(os.Getenv(name))
		if value == "" {
			return fmt.Errorf("%s is unset; the harness must own every endpoint", name)
		}
		parsed, err := url.Parse(value)
		if err != nil {
			return fmt.Errorf("%s is not a URL: %w", name, err)
		}
		switch strings.ToLower(parsed.Hostname()) {
		case "127.0.0.1", "::1", "localhost":
		default:
			return fmt.Errorf("%s host %q is not loopback; refusing to run", name, parsed.Hostname())
		}
	}
	return nil
}
