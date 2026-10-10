// Package dynamodbprobe verifies that a local endpoint actually speaks the
// DynamoDB protocol instead of merely accepting HTTP connections.
//
// The example Makefile targets use it to decide whether DynamoDB Local is
// already running, so a non-DynamoDB service listening on the example port is
// not mistaken for DynamoDB Local.
package dynamodbprobe

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/aws/aws-sdk-go-v2/aws"
	awsmiddleware "github.com/aws/aws-sdk-go-v2/aws/middleware"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb"
	"github.com/aws/smithy-go/logging"
)

// DefaultEndpoint is the endpoint the TableTheory examples expect DynamoDB
// Local to listen on.
const DefaultEndpoint = "http://localhost:8000"

// ErrNotDynamoDB reports that an endpoint did not complete a DynamoDB
// operation, which means it is not speaking the DynamoDB protocol. Callers can
// treat any error wrapping it as "not DynamoDB Local".
var ErrNotDynamoDB = errors.New("endpoint does not speak the DynamoDB protocol")

const (
	probeRegion          = "us-east-1"
	probeAccessKeyID     = "dummy"
	probeSecretAccessKey = "dummy"
)

// CheckEndpoint issues an explicit DynamoDB ListTables call against endpoint
// using static dummy credentials (the same dummy credentials the examples
// use).
//
// It returns nil only when the endpoint behaves as a DynamoDB service. An
// endpoint that accepts HTTP but is not DynamoDB, an endpoint that cannot be
// reached, and an endpoint that answers with an unexpected body all yield a
// non-nil error wrapping ErrNotDynamoDB, so callers such as the example
// Makefiles fall back to starting DynamoDB Local themselves.
func CheckEndpoint(ctx context.Context, endpoint string) error {
	if strings.TrimSpace(endpoint) == "" {
		return fmt.Errorf("%w: empty endpoint", ErrNotDynamoDB)
	}

	cfg, err := config.LoadDefaultConfig(ctx,
		config.WithRegion(probeRegion),
		config.WithCredentialsProvider(credentials.NewStaticCredentialsProvider(
			probeAccessKeyID, probeSecretAccessKey, "")),
		config.WithRetryMaxAttempts(1),
		// The probe is a single best-effort call; SDK-level diagnostics such as
		// "failed to close HTTP response body" are noise for a reachability check.
		config.WithLogger(logging.Nop{}),
	)
	if err != nil {
		return fmt.Errorf("%w: load AWS config: %v", ErrNotDynamoDB, err)
	}

	client := dynamodb.NewFromConfig(cfg, func(o *dynamodb.Options) {
		o.BaseEndpoint = aws.String(endpoint)
	})

	out, err := client.ListTables(ctx, &dynamodb.ListTablesInput{
		Limit: aws.Int32(1),
	})
	if err != nil {
		return fmt.Errorf("%w: ListTables against %s failed: %v", ErrNotDynamoDB, endpoint, err)
	}

	// A successful HTTP call is not sufficient: the SDK tolerates empty or
	// arbitrary JSON success bodies, so a plain HTTP service would look like an
	// empty table list. DynamoDB responses always carry an AWS request ID
	// (x-amzn-RequestId), which the SDK surfaces in the result metadata, so its
	// absence means the endpoint did not answer as a DynamoDB service.
	if _, ok := awsmiddleware.GetRequestIDMetadata(out.ResultMetadata); !ok {
		return fmt.Errorf("%w: %s answered HTTP but returned no DynamoDB response metadata", ErrNotDynamoDB, endpoint)
	}

	return nil
}
