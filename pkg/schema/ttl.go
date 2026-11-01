package schema

import (
	"context"
	"errors"
	"fmt"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb/types"
	"github.com/aws/smithy-go"

	"github.com/theory-cloud/tabletheory/v6/pkg/model"
	"github.com/theory-cloud/tabletheory/v6/pkg/session"
)

// ErrTTLAttributeMismatch reports that a table's TTL is already enabled on a
// different attribute than the model declares. The error returned from TTL
// synchronization wraps it, so callers can match it with errors.Is.
var ErrTTLAttributeMismatch = errors.New("ttl attribute mismatch")

// ttlAttributeMismatchError carries the conflict details while keeping the
// message text unchanged from earlier releases.
type ttlAttributeMismatchError struct {
	tableName        string
	attributeName    string
	currentAttribute string
	cause            error
}

func (e *ttlAttributeMismatchError) Error() string {
	return fmt.Sprintf(
		"failed to enable ttl on table %s using attribute %s: ttl is already enabled on a different attribute %s: %v",
		e.tableName,
		e.attributeName,
		e.currentAttribute,
		e.cause,
	)
}

func (e *ttlAttributeMismatchError) Unwrap() []error {
	return []error{ErrTTLAttributeMismatch, e.cause}
}

// timeToLiveDescriber is satisfied by the real DynamoDB client. It is kept
// separate from session.DynamoDBAPI so narrow test doubles and existing
// implementations of that interface keep working.
type timeToLiveDescriber interface {
	DescribeTimeToLive(
		ctx context.Context,
		params *dynamodb.DescribeTimeToLiveInput,
		optFns ...func(*dynamodb.Options),
	) (*dynamodb.DescribeTimeToLiveOutput, error)
}

// EnableTTL enables DynamoDB TTL for the ttl-tagged field on an existing model table.
func (m *Manager) EnableTTL(modelValue any) error {
	metadata, err := m.registry.GetMetadata(modelValue)
	if err != nil {
		return fmt.Errorf("failed to get model metadata: %w", err)
	}

	if metadata.TTLField == nil {
		return fmt.Errorf("model %T does not define a ttl field", modelValue)
	}

	ctx := context.Background()
	client, err := m.session.API()
	if err != nil {
		return fmt.Errorf("failed to get client for ttl update: %w", err)
	}

	return m.syncModelTTL(ctx, client, metadata)
}

func (m *Manager) syncModelTTL(ctx context.Context, client session.DynamoDBAPI, metadata *model.Metadata) error {
	if metadata == nil || metadata.TTLField == nil {
		return nil
	}

	attributeName := metadata.TTLField.DBName
	_, err := client.UpdateTimeToLive(ctx, &dynamodb.UpdateTimeToLiveInput{
		TableName: aws.String(metadata.TableName),
		TimeToLiveSpecification: &types.TimeToLiveSpecification{
			AttributeName: aws.String(attributeName),
			Enabled:       aws.Bool(true),
		},
	})
	if err == nil {
		return nil
	}

	var apiErr smithy.APIError
	if errors.As(err, &apiErr) && apiErr.ErrorCode() == "ValidationException" {
		if handled, resolved := resolveTTLValidationFailure(ctx, client, metadata.TableName, attributeName, err); handled {
			return resolved
		}
	}

	return fmt.Errorf(
		"failed to enable ttl on table %s using attribute %s: %w",
		metadata.TableName,
		attributeName,
		err,
	)
}

// resolveTTLValidationFailure turns the two "TTL is already in the desired
// state" validation outcomes into an idempotent success while keeping a
// distinct-attribute conflict a hard error. DescribeTimeToLive is consulted
// instead of matching the service message text.
func resolveTTLValidationFailure(
	ctx context.Context,
	client session.DynamoDBAPI,
	tableName string,
	attributeName string,
	cause error,
) (bool, error) {
	describer, ok := client.(timeToLiveDescriber)
	if !ok {
		return false, nil
	}

	out, err := describer.DescribeTimeToLive(ctx, &dynamodb.DescribeTimeToLiveInput{
		TableName: aws.String(tableName),
	})
	if err != nil || out == nil || out.TimeToLiveDescription == nil {
		return false, nil
	}

	description := out.TimeToLiveDescription
	switch description.TimeToLiveStatus {
	case types.TimeToLiveStatusEnabled, types.TimeToLiveStatusEnabling:
	default:
		return false, nil
	}

	currentAttribute := aws.ToString(description.AttributeName)
	if currentAttribute == attributeName {
		return true, nil
	}

	return true, &ttlAttributeMismatchError{
		tableName:        tableName,
		attributeName:    attributeName,
		currentAttribute: currentAttribute,
		cause:            cause,
	}
}
