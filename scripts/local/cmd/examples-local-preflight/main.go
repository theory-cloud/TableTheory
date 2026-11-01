// Command examples-local-preflight proves the environment the example harness
// hands to every child process resolves DynamoDB, KMS, STS, S3 and SQS to the
// checked-in local profile instead of the real AWS endpoints, that the SDK sees
// nothing but the harness's static dummy credentials, and that each local
// backend speaks its real AWS protocol.
//
// The harness runs it after the profile stack answers; a non-zero exit stops the
// run before any example starts.
package main

import (
	"context"
	"flag"
	"fmt"
	"net/url"
	"os"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb"
	"github.com/aws/aws-sdk-go-v2/service/kms"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/aws/aws-sdk-go-v2/service/sqs"
	"github.com/aws/aws-sdk-go-v2/service/sts"
)

const (
	dummyAccessKeyID     = "dummy"
	dummySecretAccessKey = "dummy"
	resolveTimeout       = 15 * time.Second
)

type expectations struct {
	dynamodb string
	kms      string
	sts      string
	s3       string
	sqs      string
}

func main() {
	var want expectations
	flag.StringVar(&want.dynamodb, "dynamodb", "", "endpoint every DynamoDB client must resolve to")
	flag.StringVar(&want.kms, "kms", "", "endpoint every KMS client must resolve to")
	flag.StringVar(&want.sts, "sts", "", "endpoint every STS client must resolve to")
	flag.StringVar(&want.s3, "s3", "", "endpoint every S3 client must resolve to")
	flag.StringVar(&want.sqs, "sqs", "", "endpoint every SQS client must resolve to")
	flag.Parse()

	if err := run(context.Background(), want); err != nil {
		fmt.Fprintf(os.Stderr, "preflight: FAIL: %v\n", err)
		os.Exit(1)
	}
}

func run(ctx context.Context, want expectations) error {
	expected, err := want.normalized()
	if err != nil {
		return err
	}

	ctx, cancel := context.WithTimeout(ctx, resolveTimeout)
	defer cancel()

	cfg, err := config.LoadDefaultConfig(ctx)
	if err != nil {
		return fmt.Errorf("load AWS config: %w", err)
	}
	if strings.TrimSpace(cfg.Region) == "" {
		return fmt.Errorf("AWS region is unresolved; the harness must set AWS_REGION")
	}

	creds, err := cfg.Credentials.Retrieve(ctx)
	if err != nil {
		return fmt.Errorf("resolve AWS credentials: %w", err)
	}
	if creds.AccessKeyID != dummyAccessKeyID || creds.SecretAccessKey != dummySecretAccessKey {
		return fmt.Errorf(
			"resolved credentials are not the harness dummy static keys (access key %q); refusing to run with credentials that could reach real AWS",
			creds.AccessKeyID,
		)
	}

	ddbClient := dynamodb.NewFromConfig(cfg)
	kmsClient := kms.NewFromConfig(cfg)
	stsClient := sts.NewFromConfig(cfg)
	sqsClient := sqs.NewFromConfig(cfg)
	s3Client := s3.NewFromConfig(cfg, func(o *s3.Options) {
		// The local S3 emulator serves every bucket from one loopback port, so
		// requests must be path-style instead of virtual-host-style.
		o.UsePathStyle = true
	})

	for _, check := range []struct {
		name     string
		resolved string
		expected string
	}{
		{"DynamoDB", aws.ToString(ddbClient.Options().BaseEndpoint), expected.dynamodb},
		{"KMS", aws.ToString(kmsClient.Options().BaseEndpoint), expected.kms},
		{"STS", aws.ToString(stsClient.Options().BaseEndpoint), expected.sts},
		{"S3", aws.ToString(s3Client.Options().BaseEndpoint), expected.s3},
		{"SQS", aws.ToString(sqsClient.Options().BaseEndpoint), expected.sqs},
	} {
		if err := assertLocalEndpoint(check.name, check.resolved, check.expected); err != nil {
			return err
		}
	}

	if _, err := ddbClient.ListTables(ctx, nil); err != nil {
		return fmt.Errorf("DynamoDB protocol probe (ListTables): %w", err)
	}
	if _, err := kmsClient.ListKeys(ctx, nil); err != nil {
		return fmt.Errorf("KMS protocol probe (ListKeys): %w", err)
	}
	if _, err := stsClient.GetCallerIdentity(ctx, nil); err != nil {
		return fmt.Errorf("STS protocol probe (GetCallerIdentity): %w", err)
	}
	if _, err := s3Client.ListBuckets(ctx, nil); err != nil {
		return fmt.Errorf("S3 protocol probe (ListBuckets): %w", err)
	}
	if _, err := sqsClient.ListQueues(ctx, nil); err != nil {
		return fmt.Errorf("SQS protocol probe (ListQueues): %w", err)
	}

	fmt.Printf("preflight: DynamoDB resolves to %s (ListTables ok)\n", expected.dynamodb)
	fmt.Printf("preflight: KMS resolves to %s (ListKeys ok)\n", expected.kms)
	fmt.Printf("preflight: STS resolves to %s (GetCallerIdentity ok)\n", expected.sts)
	fmt.Printf("preflight: S3 resolves to %s (ListBuckets ok)\n", expected.s3)
	fmt.Printf("preflight: SQS resolves to %s (ListQueues ok)\n", expected.sqs)
	fmt.Printf("preflight: static dummy credentials, region %s\n", cfg.Region)
	return nil
}

func (w expectations) normalized() (expectations, error) {
	out := expectations{}
	targets := []struct {
		name  string
		value string
		dst   *string
	}{
		{"dynamodb", w.dynamodb, &out.dynamodb},
		{"kms", w.kms, &out.kms},
		{"sts", w.sts, &out.sts},
		{"s3", w.s3, &out.s3},
		{"sqs", w.sqs, &out.sqs},
	}
	for _, target := range targets {
		value, err := normalizeEndpoint(target.value)
		if err != nil {
			return expectations{}, fmt.Errorf("harness %s endpoint: %w", target.name, err)
		}
		*target.dst = value
	}
	return out, nil
}

func assertLocalEndpoint(name, resolved, expected string) error {
	if strings.TrimSpace(resolved) == "" {
		return fmt.Errorf(
			"%s endpoint is unresolved; refusing to run where a request could fall through to real AWS",
			name,
		)
	}
	normalized, err := normalizeEndpoint(resolved)
	if err != nil {
		return fmt.Errorf("resolved %s endpoint: %w", name, err)
	}
	if normalized != expected {
		return fmt.Errorf("%s resolves to %s, not the harness endpoint %s; refusing to run", name, normalized, expected)
	}
	parsed, err := url.Parse(normalized)
	if err != nil {
		return fmt.Errorf("parse %s endpoint: %w", name, err)
	}
	switch strings.ToLower(parsed.Hostname()) {
	case "127.0.0.1", "::1", "localhost":
		return nil
	default:
		return fmt.Errorf("%s endpoint host %q is not loopback; refusing to run", name, parsed.Hostname())
	}
}

func normalizeEndpoint(raw string) (string, error) {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return "", fmt.Errorf("empty endpoint")
	}
	parsed, err := url.Parse(trimmed)
	if err != nil {
		return "", fmt.Errorf("parse %q: %w", trimmed, err)
	}
	if parsed.Scheme == "" || parsed.Host == "" {
		return "", fmt.Errorf("parse %q: endpoint must be an absolute URL", trimmed)
	}
	parsed.Scheme = strings.ToLower(parsed.Scheme)
	parsed.Host = strings.ToLower(parsed.Host)
	parsed.Path = strings.TrimRight(parsed.Path, "/")
	parsed.RawQuery = ""
	parsed.Fragment = ""
	return parsed.String(), nil
}
