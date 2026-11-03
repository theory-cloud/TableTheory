// Command examples-local-payment-fixtures provisions the local DynamoDB tables
// the Lambda and payment examples use, seeds fixture rows through the real
// TableTheory models, configures a real moto S3 bucket notification and puts
// the reconciliation object, and reads the stored state back for harness
// assertions.
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
	"github.com/aws/aws-sdk-go-v2/service/sqs"
	sqstypes "github.com/aws/aws-sdk-go-v2/service/sqs/types"

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
	defaultQueueName     = "tabletheory-reconcile-notifications"
	notificationTimeout  = 30 * time.Second
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
	endpoint   string
	queue      string
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
	flag.StringVar(&opts.mode, "mode", "", "provision|seed-merchant|seed-user|verify-user|publish-reconcile|publish-reconcile-notification|verify-payment")
	flag.StringVar(&opts.merchant, "merchant", defaultMerchantID, "merchant id to seed or verify")
	flag.StringVar(&opts.webhookURL, "webhook-url", defaultWebhookURL, "merchant webhook URL")
	flag.StringVar(&opts.bucket, "bucket", defaultBucket, "S3 bucket for the reconciliation object")
	flag.StringVar(&opts.key, "key", defaultObjectKey, "S3 object key for the reconciliation object")
	flag.StringVar(&opts.endpoint, "endpoint", "", "S3 and SQS endpoint for the notification probe (the profile's moto)")
	flag.StringVar(&opts.queue, "queue", defaultQueueName, "SQS queue that receives the bucket notification")
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
	case "publish-reconcile-notification":
		return publishReconcileNotification(ctx, opts)
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

type s3Notification struct {
	Records []s3NotificationRecord `json:"Records"`
}

type s3NotificationRecord struct {
	EventVersion string `json:"eventVersion"`
	EventSource  string `json:"eventSource"`
	AWSRegion    string `json:"awsRegion"`
	EventTime    string `json:"eventTime"`
	EventName    string `json:"eventName"`
	S3           struct {
		S3SchemaVersion string `json:"s3SchemaVersion"`
		ConfigurationID string `json:"configurationId"`
		Bucket          struct {
			Name string `json:"name"`
			ARN  string `json:"arn"`
		} `json:"bucket"`
		Object struct {
			Key  string `json:"key"`
			Size int64  `json:"size"`
			ETag string `json:"etag"`
		} `json:"object"`
	} `json:"s3"`
}

// publishReconcileNotification configures a real bucket notification on the
// profile's moto S3, drains the s3:TestEvent AWS sends when a notification is
// configured, puts the reconciliation CSV, and returns the ObjectCreated record
// moto delivered to the queue. The event shape is asserted against
// https://docs.aws.amazon.com/AmazonS3/latest/userguide/notification-content-structure.html
// (the record eventName has no s3: prefix; only the configuration uses one).
func publishReconcileNotification(ctx context.Context, opts options) error {
	if strings.TrimSpace(opts.endpoint) == "" {
		return fmt.Errorf("-endpoint is required for publish-reconcile-notification")
	}
	if err := assertLoopbackEndpoint("-endpoint", opts.endpoint); err != nil {
		return err
	}

	cfg, err := config.LoadDefaultConfig(ctx)
	if err != nil {
		return fmt.Errorf("load AWS config: %w", err)
	}
	s3Client := s3.NewFromConfig(cfg, func(options *s3.Options) {
		options.BaseEndpoint = aws.String(opts.endpoint)
		options.UsePathStyle = true
	})
	sqsClient := sqs.NewFromConfig(cfg, func(options *sqs.Options) {
		options.BaseEndpoint = aws.String(opts.endpoint)
	})

	queue, err := sqsClient.CreateQueue(ctx, &sqs.CreateQueueInput{
		QueueName: aws.String(opts.queue),
	})
	if err != nil {
		return fmt.Errorf("create queue %s: %w", opts.queue, err)
	}
	queueURL := aws.ToString(queue.QueueUrl)
	attributes, err := sqsClient.GetQueueAttributes(ctx, &sqs.GetQueueAttributesInput{
		QueueUrl:       aws.String(queueURL),
		AttributeNames: []sqstypes.QueueAttributeName{sqstypes.QueueAttributeNameQueueArn},
	})
	if err != nil {
		return fmt.Errorf("read queue ARN: %w", err)
	}
	queueARN := attributes.Attributes[string(sqstypes.QueueAttributeNameQueueArn)]
	if queueARN == "" {
		return fmt.Errorf("queue %s has no ARN", opts.queue)
	}

	if _, err := s3Client.CreateBucket(ctx, &s3.CreateBucketInput{
		Bucket: aws.String(opts.bucket),
	}); err != nil {
		var owned *s3types.BucketAlreadyOwnedByYou
		var exists *s3types.BucketAlreadyExists
		if !errors.As(err, &owned) && !errors.As(err, &exists) {
			return fmt.Errorf("create bucket %s: %w", opts.bucket, err)
		}
	}
	if _, err := s3Client.PutBucketNotificationConfiguration(ctx, &s3.PutBucketNotificationConfigurationInput{
		Bucket: aws.String(opts.bucket),
		NotificationConfiguration: &s3types.NotificationConfiguration{
			QueueConfigurations: []s3types.QueueConfiguration{{
				Id:       aws.String("tabletheory-reconcile"),
				QueueArn: aws.String(queueARN),
				Events:   []s3types.Event{s3types.EventS3ObjectCreated},
			}},
		},
	}); err != nil {
		return fmt.Errorf("configure notification on %s: %w", opts.bucket, err)
	}

	testEventID, _, err := receiveNotification(ctx, sqsClient, queueURL, func(body string) bool {
		return strings.Contains(body, "s3:TestEvent")
	})
	if err != nil {
		return fmt.Errorf("the s3:TestEvent was not delivered: %w", err)
	}

	body := []byte("payment_id,transaction_id,status,fee,processed_date,settlement_date\n" +
		"pay_1,tx_1,succeeded,100,2026-11-01,2026-11-01\n" +
		"pay_2,tx_2,succeeded,250,2026-11-01,2026-11-01\n")
	if _, err := s3Client.PutObject(ctx, &s3.PutObjectInput{
		Bucket:      aws.String(opts.bucket),
		Key:         aws.String(opts.key),
		Body:        bytes.NewReader(body),
		ContentType: aws.String("text/csv"),
	}); err != nil {
		return fmt.Errorf("put %s/%s: %w", opts.bucket, opts.key, err)
	}

	messageID, raw, err := receiveNotification(ctx, sqsClient, queueURL, func(body string) bool {
		return strings.Contains(body, "ObjectCreated:Put")
	})
	if err != nil {
		return fmt.Errorf("the ObjectCreated notification was not delivered: %w", err)
	}

	var notification s3Notification
	if err := json.Unmarshal([]byte(raw), &notification); err != nil {
		return fmt.Errorf("notification body is not JSON: %w", err)
	}
	if len(notification.Records) != 1 {
		return fmt.Errorf("notification carried %d records, want 1", len(notification.Records))
	}
	record := notification.Records[0]
	if record.EventVersion != "2.1" {
		return fmt.Errorf("unexpected eventVersion %q", record.EventVersion)
	}
	if record.EventSource != "aws:s3" {
		return fmt.Errorf("unexpected eventSource %q", record.EventSource)
	}
	if record.EventName != "ObjectCreated:Put" {
		return fmt.Errorf("unexpected eventName %q", record.EventName)
	}
	if record.S3.S3SchemaVersion != "1.0" {
		return fmt.Errorf("unexpected s3SchemaVersion %q", record.S3.S3SchemaVersion)
	}
	if record.S3.Bucket.Name != opts.bucket || record.S3.Bucket.ARN != "arn:aws:s3:::"+opts.bucket {
		return fmt.Errorf("notification names bucket %q/%q, want %q", record.S3.Bucket.Name, record.S3.Bucket.ARN, opts.bucket)
	}
	decodedKey, err := url.QueryUnescape(record.S3.Object.Key)
	if err != nil {
		return fmt.Errorf("notification key %q is not URL-encoded: %w", record.S3.Object.Key, err)
	}
	if decodedKey != opts.key {
		return fmt.Errorf("notification key %q decodes to %q, want %q", record.S3.Object.Key, decodedKey, opts.key)
	}
	if record.S3.Object.Size <= 0 || record.S3.Object.ETag == "" {
		return fmt.Errorf("notification object has size %d and eTag %q", record.S3.Object.Size, record.S3.Object.ETag)
	}

	var decoded map[string]any
	if err := json.Unmarshal([]byte(raw), &decoded); err != nil {
		return fmt.Errorf("decode notification envelope: %w", err)
	}
	emit(map[string]any{
		"source":        "moto-s3-notification",
		"bucket":        opts.bucket,
		"key":           opts.key,
		"queue_url":     queueURL,
		"queue_arn":     queueARN,
		"message_id":    messageID,
		"test_event_id": testEventID,
		"notification":  decoded,
		"event_name":    record.EventName,
		"size":          record.S3.Object.Size,
		"etag":          record.S3.Object.ETag,
	})
	return nil
}

func receiveNotification(
	ctx context.Context,
	client *sqs.Client,
	queueURL string,
	match func(string) bool,
) (string, string, error) {
	deadline := time.Now().Add(notificationTimeout)
	for time.Now().Before(deadline) {
		out, err := client.ReceiveMessage(ctx, &sqs.ReceiveMessageInput{
			QueueUrl:            aws.String(queueURL),
			MaxNumberOfMessages: 10,
			WaitTimeSeconds:     1,
		})
		if err != nil {
			return "", "", fmt.Errorf("receive from %s: %w", queueURL, err)
		}
		for _, message := range out.Messages {
			body := aws.ToString(message.Body)
			if !match(body) {
				continue
			}
			if _, err := client.DeleteMessage(ctx, &sqs.DeleteMessageInput{
				QueueUrl:      aws.String(queueURL),
				ReceiptHandle: message.ReceiptHandle,
			}); err != nil {
				return "", "", fmt.Errorf("delete notification: %w", err)
			}
			return aws.ToString(message.MessageId), body, nil
		}
	}
	return "", "", fmt.Errorf("no matching notification arrived within %s", notificationTimeout)
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
		if err := assertLoopbackEndpoint(name, value); err != nil {
			return err
		}
	}
	return nil
}

func emit(payload map[string]any) {
	encoded, err := json.Marshal(payload)
	if err != nil {
		fmt.Fprintf(os.Stderr, "examples-local-payment-fixtures: FAIL: encode result: %v\n", err)
		os.Exit(1)
	}
	fmt.Println(string(encoded))
}

func assertLoopbackEndpoint(name, value string) error {
	parsed, err := url.Parse(value)
	if err != nil {
		return fmt.Errorf("%s is not a URL: %w", name, err)
	}
	switch strings.ToLower(parsed.Hostname()) {
	case "127.0.0.1", "::1", "localhost":
		return nil
	default:
		return fmt.Errorf("%s host %q is not loopback; refusing to run", name, parsed.Hostname())
	}
}
