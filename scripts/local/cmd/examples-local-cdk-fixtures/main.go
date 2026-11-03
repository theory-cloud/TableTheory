// Command examples-local-cdk-fixtures prepares and inspects the local state the
// cdk-multilang rows need: it provisions the shared demo table, the TTL
// evidence table and the archive bucket, enables the evidence table's real
// DynamoDB stream, reads an encrypted item back in its raw DynamoDB envelope
// form, and reads the archive object the deployed archive handler wrote to the
// local S3 emulator.
//
// It is harness tooling, not an example: every write and read goes through the
// real AWS API surface against the local profile. The tool refuses to run with
// anything but the static dummy credentials and loopback endpoints. Stream
// records are read by examples-local-stream-event; this tool never manufactures
// one.
package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb"
	ddbtypes "github.com/aws/aws-sdk-go-v2/service/dynamodb/types"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	s3types "github.com/aws/aws-sdk-go-v2/service/s3/types"

	"github.com/theory-cloud/tabletheory/v6"
	"github.com/theory-cloud/tabletheory/v6/pkg/core"
	"github.com/theory-cloud/tabletheory/v6/scripts/local/internal/streams"
)

const (
	dummyAccessKeyID     = "dummy"
	dummySecretAccessKey = "dummy"
	defaultDemoTable     = "multilang_local"
	defaultEvidenceTable = "multilang_evidence_local"
	defaultArchiveBucket = "multilang-archive-local"
	defaultArchivePrefix = "evidence-snapshots"
	streamReadyTimeout   = 30 * time.Second
)

// demoItem mirrors the deployed DemoItem model. The harness cannot import a
// package main, so the schema is repeated here; the deployed Go row fails if
// the two drift.
type demoItem struct {
	PK     string `theorydb:"pk,attr:PK" json:"PK"`
	SK     string `theorydb:"sk,attr:SK" json:"SK"`
	Value  string `theorydb:"attr:value,omitempty" json:"value,omitempty"`
	Lang   string `theorydb:"attr:lang,omitempty" json:"lang,omitempty"`
	Secret string `theorydb:"encrypted,attr:secret,omitempty" json:"secret,omitempty"`
}

func (demoItem) TableName() string { return envOr("TABLE_NAME", defaultDemoTable) }

// evidenceItem mirrors the stack's EvidenceArchiveTable row: a TTL attribute
// drives retention and the archive handler snapshots the expiring image.
type evidenceItem struct {
	_         struct{} `theorydb:"naming:snake_case"`
	PK        string   `theorydb:"pk" json:"pk"`
	SK        string   `theorydb:"sk" json:"sk"`
	Payload   string   `theorydb:"omitempty" json:"payload,omitempty"`
	ExpiresAt int64    `theorydb:"ttl" json:"expires_at"`
}

func (evidenceItem) TableName() string {
	return envOr("EVIDENCE_TABLE_NAME", defaultEvidenceTable)
}

type options struct {
	mode      string
	pk        string
	sk        string
	bucket    string
	prefix    string
	plaintext string
}

func main() {
	opts := options{}
	flag.StringVar(&opts.mode, "mode", "provision", "provision, verify-envelope or verify-archive")
	flag.StringVar(&opts.pk, "pk", "demo#1", "partition key")
	flag.StringVar(&opts.sk, "sk", "v1", "sort key")
	flag.StringVar(&opts.bucket, "bucket", defaultArchiveBucket, "archive bucket")
	flag.StringVar(&opts.prefix, "prefix", defaultArchivePrefix, "archive key prefix")
	flag.StringVar(&opts.plaintext, "plaintext", "", "plaintext that must not appear in the stored item")
	flag.Parse()

	if err := assertLocalEnvironment(); err != nil {
		fail("%v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()

	var err error
	switch opts.mode {
	case "provision":
		err = provision(ctx, opts)
	case "verify-envelope":
		err = verifyEnvelope(ctx, opts)
	case "verify-archive":
		err = verifyArchive(ctx, opts)
	default:
		err = fmt.Errorf("unknown mode %q", opts.mode)
	}
	if err != nil {
		fail("%v", err)
	}
}

func provision(ctx context.Context, opts options) error {
	db, err := newDB()
	if err != nil {
		return err
	}
	for _, model := range []any{&demoItem{}, &evidenceItem{}} {
		if err := db.EnsureTable(model); err != nil {
			return fmt.Errorf("provision %T: %w", model, err)
		}
	}

	ddb, err := newDynamoDB(ctx)
	if err != nil {
		return err
	}
	// The archive row consumes the table's real DynamoDB stream; enabling it
	// here keeps every cdk-multilang row on the same provisioned table.
	stream, err := streams.Ensure(ctx, ddb, evidenceItem{}.TableName(), streamReadyTimeout)
	if err != nil {
		return fmt.Errorf("enable evidence stream: %w", err)
	}

	client, err := newS3(ctx)
	if err != nil {
		return err
	}
	if _, err := client.CreateBucket(ctx, &s3.CreateBucketInput{Bucket: aws.String(opts.bucket)}); err != nil {
		var owned *s3types.BucketAlreadyOwnedByYou
		var exists *s3types.BucketAlreadyExists
		if !errors.As(err, &owned) && !errors.As(err, &exists) {
			return fmt.Errorf("create bucket %s: %w", opts.bucket, err)
		}
	}

	emit(map[string]any{
		"ok":                    true,
		"demo_table":            demoItem{}.TableName(),
		"evidence_table":        evidenceItem{}.TableName(),
		"evidence_stream_arn":   stream.StreamARN,
		"evidence_stream_label": stream.StreamLabel,
		"archive_bucket":        opts.bucket,
	})
	return nil
}

// verifyEnvelope proves the framework stored the secret as a v1/v2 envelope
// map: a version, an encrypted data key, a nonce and ciphertext, with no
// plaintext anywhere in the stored attribute.
func verifyEnvelope(ctx context.Context, opts options) error {
	client, err := newDynamoDB(ctx)
	if err != nil {
		return err
	}
	item, err := getItem(ctx, client, demoItem{}.TableName(), opts.pk, opts.sk)
	if err != nil {
		return err
	}
	if len(item) == 0 {
		return fmt.Errorf("item %s/%s not found in %s", opts.pk, opts.sk, demoItem{}.TableName())
	}

	secret, ok := item["secret"].(*ddbtypes.AttributeValueMemberM)
	if !ok {
		return fmt.Errorf("stored secret is not an envelope map: %T", item["secret"])
	}
	version := numericValue(secret.Value["v"])
	edk := bytesValue(secret.Value["edk"])
	nonce := bytesValue(secret.Value["nonce"])
	ciphertext := bytesValue(secret.Value["ct"])
	if version != 1 && version != 2 {
		return fmt.Errorf("unexpected envelope version %d", version)
	}
	if len(edk) == 0 || len(nonce) == 0 || len(ciphertext) == 0 {
		return fmt.Errorf("envelope is incomplete: edk=%d nonce=%d ct=%d", len(edk), len(nonce), len(ciphertext))
	}
	encoded, _ := json.Marshal(item)
	if opts.plaintext != "" && strings.Contains(string(encoded), opts.plaintext) {
		return fmt.Errorf("stored item contains the plaintext secret")
	}
	result := map[string]any{
		"found":            true,
		"value":            stringValue(item["value"]),
		"lang":             stringValue(item["lang"]),
		"envelope_version": version,
		"edk_bytes":        len(edk),
		"nonce_bytes":      len(nonce),
		"ct_bytes":         len(ciphertext),
		"stored_bytes":     len(encoded),
	}
	emit(result)
	return nil
}

func verifyArchive(ctx context.Context, opts options) error {
	client, err := newS3(ctx)
	if err != nil {
		return err
	}
	list, err := client.ListObjectsV2(ctx, &s3.ListObjectsV2Input{
		Bucket: aws.String(opts.bucket),
		Prefix: aws.String(opts.prefix + "/"),
	})
	if err != nil {
		return fmt.Errorf("list %s/%s: %w", opts.bucket, opts.prefix, err)
	}
	if len(list.Contents) == 0 {
		return fmt.Errorf("no archive object under %s/%s", opts.bucket, opts.prefix)
	}
	key := aws.ToString(list.Contents[0].Key)
	object, err := client.GetObject(ctx, &s3.GetObjectInput{
		Bucket: aws.String(opts.bucket),
		Key:    aws.String(key),
	})
	if err != nil {
		return fmt.Errorf("get %s/%s: %w", opts.bucket, key, err)
	}
	defer object.Body.Close()
	body, err := io.ReadAll(object.Body)
	if err != nil {
		return fmt.Errorf("read %s/%s: %w", opts.bucket, key, err)
	}
	var archived map[string]any
	if err := json.Unmarshal(body, &archived); err != nil {
		return fmt.Errorf("archive object is not JSON: %w", err)
	}
	emit(map[string]any{
		"objects":       len(list.Contents),
		"object_key":    key,
		"archived_at":   archived["archived_at"],
		"event_id":      archived["event_id"],
		"event_source":  archived["event_source_arn"],
		"old_image":     archived["old_image"],
		"keys":          archived["keys"],
		"ttl_attribute": archived["ttl_attribute"],
		"ttl_value":     archived["ttl_value"],
	})
	return nil
}

func newDB() (core.ExtendedDB, error) {
	return tabletheory.New(tabletheory.Config{
		Region: os.Getenv("AWS_REGION"),
	})
}

func newDynamoDB(ctx context.Context) (*dynamodb.Client, error) {
	cfg, err := config.LoadDefaultConfig(ctx)
	if err != nil {
		return nil, fmt.Errorf("load AWS config: %w", err)
	}
	return dynamodb.NewFromConfig(cfg), nil
}

func newS3(ctx context.Context) (*s3.Client, error) {
	cfg, err := config.LoadDefaultConfig(ctx)
	if err != nil {
		return nil, fmt.Errorf("load AWS config: %w", err)
	}
	return s3.NewFromConfig(cfg, func(o *s3.Options) {
		// The local S3 emulator serves every bucket from one endpoint.
		o.UsePathStyle = true
	}), nil
}

func getItem(ctx context.Context, client *dynamodb.Client, table, pk, sk string) (map[string]ddbtypes.AttributeValue, error) {
	return getItemWithKeys(ctx, client, table, "PK", "SK", pk, sk)
}

func getItemWithKeys(
	ctx context.Context,
	client *dynamodb.Client,
	table, pkName, skName, pk, sk string,
) (map[string]ddbtypes.AttributeValue, error) {
	out, err := client.GetItem(ctx, &dynamodb.GetItemInput{
		TableName: aws.String(table),
		Key: map[string]ddbtypes.AttributeValue{
			pkName: &ddbtypes.AttributeValueMemberS{Value: pk},
			skName: &ddbtypes.AttributeValueMemberS{Value: sk},
		},
		ConsistentRead: aws.Bool(true),
	})
	if err != nil {
		return nil, fmt.Errorf("get %s/%s/%s: %w", table, pk, sk, err)
	}
	return out.Item, nil
}

func stringValue(value ddbtypes.AttributeValue) string {
	if typed, ok := value.(*ddbtypes.AttributeValueMemberS); ok {
		return typed.Value
	}
	return ""
}

func numericValue(value ddbtypes.AttributeValue) int64 {
	if typed, ok := value.(*ddbtypes.AttributeValueMemberN); ok {
		parsed, _ := strconv.ParseInt(typed.Value, 10, 64)
		return parsed
	}
	return 0
}

func bytesValue(value ddbtypes.AttributeValue) []byte {
	if typed, ok := value.(*ddbtypes.AttributeValueMemberB); ok {
		return typed.Value
	}
	return nil
}

// assertLocalEnvironment refuses anything but the harness's dummy static
// credentials and loopback endpoints, so this tool can never touch real AWS.
func assertLocalEnvironment() error {
	if strings.TrimSpace(os.Getenv("AWS_ACCESS_KEY_ID")) != dummyAccessKeyID ||
		strings.TrimSpace(os.Getenv("AWS_SECRET_ACCESS_KEY")) != dummySecretAccessKey {
		return fmt.Errorf("credentials are not the harness dummy static keys; refusing to run")
	}
	for _, name := range []string{
		"AWS_ENDPOINT_URL",
		"AWS_ENDPOINT_URL_DYNAMODB",
		"DYNAMODB_ENDPOINT",
		"AWS_ENDPOINT_URL_S3",
		"S3_ENDPOINT",
	} {
		value := strings.TrimSpace(os.Getenv(name))
		if value == "" {
			return fmt.Errorf("%s is unset; the harness must own every endpoint", name)
		}
		host := strings.TrimPrefix(strings.TrimPrefix(value, "http://"), "https://")
		if index := strings.IndexAny(host, ":/"); index >= 0 {
			host = host[:index]
		}
		switch strings.ToLower(host) {
		case "127.0.0.1", "::1", "localhost":
		default:
			return fmt.Errorf("%s host %q is not loopback; refusing to run", name, host)
		}
	}
	return nil
}

func emit(payload map[string]any) {
	encoded, err := json.Marshal(payload)
	if err != nil {
		fail("encode result: %v", err)
	}
	fmt.Println(string(encoded))
}

func envOr(key, fallback string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return fallback
}

func fail(format string, args ...any) {
	fmt.Fprintf(os.Stderr, "cdk fixtures: "+format+"\n", args...)
	os.Exit(1)
}
