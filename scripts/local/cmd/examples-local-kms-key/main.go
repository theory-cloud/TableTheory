// Command examples-local-kms-key provisions the KMS key the encryption
// examples use and proves the local KMS can wrap and unwrap a data key with the
// same EncryptionContext the framework's v2 envelope sends.
//
// It prints the key ARN on stdout and refuses to run against anything but the
// harness's loopback endpoint with its static dummy credentials, so it can never
// create a key in a real AWS account.
package main

import (
	"bytes"
	"context"
	"flag"
	"fmt"
	"net/url"
	"os"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/kms"
	kmstypes "github.com/aws/aws-sdk-go-v2/service/kms/types"
)

const (
	dummyAccessKeyID     = "dummy"
	dummySecretAccessKey = "dummy"
	// The same EncryptionContext binding the Go, TypeScript and Python
	// runtimes use for a v2 envelope.
	trialEncryptionContextKey   = "theorydb:encryption:aad"
	trialEncryptionContextValue = "theorydb:encrypted:v2|attr=trial|table=trial|pk=S:trial"
	requestTimeout              = 30 * time.Second
)

func main() {
	endpoint := flag.String("endpoint", os.Getenv("AWS_ENDPOINT_URL_KMS"), "local KMS endpoint")
	flag.Parse()

	arn, err := run(context.Background(), *endpoint)
	if err != nil {
		fmt.Fprintf(os.Stderr, "examples-local-kms-key: FAIL: %v\n", err)
		os.Exit(1)
	}
	fmt.Println(arn)
}

func run(ctx context.Context, endpoint string) (string, error) {
	if err := assertLocalEndpoint(endpoint); err != nil {
		return "", err
	}

	ctx, cancel := context.WithTimeout(ctx, requestTimeout)
	defer cancel()

	cfg, err := config.LoadDefaultConfig(ctx)
	if err != nil {
		return "", fmt.Errorf("load AWS config: %w", err)
	}
	creds, err := cfg.Credentials.Retrieve(ctx)
	if err != nil {
		return "", fmt.Errorf("resolve AWS credentials: %w", err)
	}
	if creds.AccessKeyID != dummyAccessKeyID || creds.SecretAccessKey != dummySecretAccessKey {
		return "", fmt.Errorf("resolved credentials are not the harness dummy static keys; refusing to provision")
	}

	client := kms.NewFromConfig(cfg, func(o *kms.Options) {
		o.BaseEndpoint = aws.String(endpoint)
	})

	key, err := client.CreateKey(ctx, &kms.CreateKeyInput{
		Description: aws.String("tabletheory local examples envelope key"),
	})
	if err != nil {
		return "", fmt.Errorf("kms CreateKey: %w", err)
	}
	arn := aws.ToString(key.KeyMetadata.Arn)
	if arn == "" {
		return "", fmt.Errorf("kms CreateKey returned an empty ARN")
	}

	contextMap := map[string]string{trialEncryptionContextKey: trialEncryptionContextValue}
	dataKey, err := client.GenerateDataKey(ctx, &kms.GenerateDataKeyInput{
		KeyId:             aws.String(arn),
		KeySpec:           kmstypes.DataKeySpecAes256,
		EncryptionContext: contextMap,
	})
	if err != nil {
		return "", fmt.Errorf("kms GenerateDataKey: %w", err)
	}
	if len(dataKey.Plaintext) != 32 || len(dataKey.CiphertextBlob) == 0 {
		return "", fmt.Errorf(
			"kms GenerateDataKey returned plaintext=%d bytes ciphertext=%d bytes; want a 32-byte key and a ciphertext blob",
			len(dataKey.Plaintext), len(dataKey.CiphertextBlob),
		)
	}

	decrypted, err := client.Decrypt(ctx, &kms.DecryptInput{
		CiphertextBlob:    dataKey.CiphertextBlob,
		KeyId:             aws.String(arn),
		EncryptionContext: contextMap,
	})
	if err != nil {
		return "", fmt.Errorf("kms Decrypt: %w", err)
	}
	if !bytes.Equal(decrypted.Plaintext, dataKey.Plaintext) {
		return "", fmt.Errorf("kms Decrypt did not return the wrapped data key")
	}

	if _, err := client.Decrypt(ctx, &kms.DecryptInput{
		CiphertextBlob:    dataKey.CiphertextBlob,
		KeyId:             aws.String(arn),
		EncryptionContext: map[string]string{trialEncryptionContextKey: "wrong-context"},
	}); err == nil {
		return "", fmt.Errorf("kms Decrypt accepted a mismatched EncryptionContext")
	}

	return arn, nil
}

func assertLocalEndpoint(raw string) error {
	parsed, err := url.Parse(strings.TrimSpace(raw))
	if err != nil {
		return fmt.Errorf("parse KMS endpoint %q: %w", raw, err)
	}
	if parsed.Scheme == "" || parsed.Host == "" {
		return fmt.Errorf("KMS endpoint %q is unresolved; refusing to provision", raw)
	}
	switch strings.ToLower(parsed.Scheme) {
	case "http", "https":
	default:
		return fmt.Errorf("KMS endpoint %q is not HTTP; refusing to provision", raw)
	}
	switch strings.ToLower(parsed.Hostname()) {
	case "127.0.0.1", "::1", "localhost":
		return nil
	default:
		return fmt.Errorf("KMS endpoint host %q is not loopback; refusing to provision", parsed.Hostname())
	}
}
