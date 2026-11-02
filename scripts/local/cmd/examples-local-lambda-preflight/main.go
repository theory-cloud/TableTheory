// Command examples-local-lambda-preflight runs inside the Lambda runtime
// container before any example handler and proves the function environment
// cannot reach real AWS: every SDK endpoint resolves to the local profile's
// service, only the harness dummy credentials are available, each backend
// speaks its AWS protocol, and a real AWS hostname does not resolve.
//
// The harness invokes it through the Runtime Interface Emulator with the
// endpoints it expects. A non-zero handler error stops the run before any
// example starts.
package main

import (
	"context"
	"fmt"
	"net"
	"net/url"
	"strings"
	"time"

	"github.com/aws/aws-lambda-go/lambda"
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
	probeTimeout         = 20 * time.Second
	dnsTimeout           = 5 * time.Second
	// A real AWS endpoint that must not resolve from inside the function
	// container: the harness points the container DNS at a closed address and
	// supplies every profile service through /etc/hosts.
	realAWSHost = "dynamodb.us-east-1.amazonaws.com"
)

type request struct {
	Endpoints map[string]string `json:"endpoints"`
}

type result struct {
	OK       bool              `json:"ok"`
	Resolved map[string]string `json:"resolved"`
	DNS      string            `json:"dns"`
}

func main() {
	lambda.Start(handle)
}

func handle(ctx context.Context, event request) (result, error) {
	ctx, cancel := context.WithTimeout(ctx, probeTimeout)
	defer cancel()

	expected := make(map[string]string, len(event.Endpoints))
	for name, raw := range event.Endpoints {
		normalized, err := normalizeEndpoint(raw)
		if err != nil {
			return result{}, fmt.Errorf("%s endpoint: %w", name, err)
		}
		expected[name] = normalized
	}
	for _, name := range []string{"dynamodb", "kms", "sts", "s3", "sqs"} {
		if expected[name] == "" {
			return result{}, fmt.Errorf("%s endpoint was not provided to the preflight", name)
		}
	}

	cfg, err := config.LoadDefaultConfig(ctx)
	if err != nil {
		return result{}, fmt.Errorf("load AWS config: %w", err)
	}
	creds, err := cfg.Credentials.Retrieve(ctx)
	if err != nil {
		return result{}, fmt.Errorf("resolve AWS credentials: %w", err)
	}
	if creds.AccessKeyID != dummyAccessKeyID || creds.SecretAccessKey != dummySecretAccessKey {
		return result{}, fmt.Errorf(
			"resolved credentials are not the harness dummy static keys (access key %q); refusing to run",
			creds.AccessKeyID,
		)
	}

	ddbClient := dynamodb.NewFromConfig(cfg)
	kmsClient := kms.NewFromConfig(cfg)
	stsClient := sts.NewFromConfig(cfg)
	sqsClient := sqs.NewFromConfig(cfg)
	s3Client := s3.NewFromConfig(cfg, func(o *s3.Options) {
		o.UsePathStyle = true
	})

	checks := []struct {
		name     string
		resolved string
		probe    func() error
	}{
		{"dynamodb", aws.ToString(ddbClient.Options().BaseEndpoint), func() error {
			_, err := ddbClient.ListTables(ctx, nil)
			return err
		}},
		{"kms", aws.ToString(kmsClient.Options().BaseEndpoint), func() error {
			_, err := kmsClient.ListKeys(ctx, nil)
			return err
		}},
		{"sts", aws.ToString(stsClient.Options().BaseEndpoint), func() error {
			_, err := stsClient.GetCallerIdentity(ctx, nil)
			return err
		}},
		{"s3", aws.ToString(s3Client.Options().BaseEndpoint), func() error {
			_, err := s3Client.ListBuckets(ctx, nil)
			return err
		}},
		{"sqs", aws.ToString(sqsClient.Options().BaseEndpoint), func() error {
			_, err := sqsClient.ListQueues(ctx, nil)
			return err
		}},
	}

	resolved := make(map[string]string, len(checks))
	for _, check := range checks {
		if err := assertEndpoint(check.name, check.resolved, expected[check.name]); err != nil {
			return result{}, err
		}
		resolved[check.name] = check.resolved
		if err := check.probe(); err != nil {
			return result{}, fmt.Errorf("%s protocol probe: %w", check.name, err)
		}
	}

	dnsCtx, cancelDNS := context.WithTimeout(ctx, dnsTimeout)
	defer cancelDNS()
	if addresses, err := net.DefaultResolver.LookupHost(dnsCtx, realAWSHost); err == nil {
		return result{}, fmt.Errorf(
			"%s resolves to %v inside the function container; refusing to run where a request could reach real AWS",
			realAWSHost, addresses,
		)
	}

	return result{
		OK:       true,
		Resolved: resolved,
		DNS:      realAWSHost + " does not resolve",
	}, nil
}

func assertEndpoint(name, resolved, expected string) error {
	if strings.TrimSpace(resolved) == "" {
		return fmt.Errorf("%s endpoint is unresolved; refusing to run where a request could fall through to real AWS", name)
	}
	normalized, err := normalizeEndpoint(resolved)
	if err != nil {
		return fmt.Errorf("resolved %s endpoint: %w", name, err)
	}
	if normalized != expected {
		return fmt.Errorf("%s resolves to %s, not the harness endpoint %s; refusing to run", name, normalized, expected)
	}
	return nil
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
	return parsed.String(), nil
}
