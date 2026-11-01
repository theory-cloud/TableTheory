package schema

import (
	"context"
	"errors"
	"fmt"
	"log"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb/types"
	"github.com/aws/smithy-go"

	theorydbErrors "github.com/theory-cloud/tabletheory/v6/pkg/errors"
	"github.com/theory-cloud/tabletheory/v6/pkg/model"
	"github.com/theory-cloud/tabletheory/v6/pkg/session"
)

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

// EnableTTL turns DynamoDB TTL on for the ttl-tagged field of an existing model
// table. This is an explicit opt-in: provisioning never calls it, because
// enabling TTL makes DynamoDB delete items that are already past their expiry.
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

	return m.enableModelTTL(ctx, client, metadata)
}

// VerifyTTL inspects the TTL state of an existing model table without changing
// it. TTL already enabled on the model attribute is an idempotent success, TTL
// enabled on a different attribute is a typed conflict, and a disabled TTL only
// reports a warning so provisioning keeps succeeding.
func (m *Manager) VerifyTTL(modelValue any) error {
	metadata, err := m.registry.GetMetadata(modelValue)
	if err != nil {
		return fmt.Errorf("failed to get model metadata: %w", err)
	}
	if metadata.TTLField == nil {
		return nil
	}

	ctx := context.Background()
	client, err := m.session.API()
	if err != nil {
		return fmt.Errorf("failed to get client for ttl check: %w", err)
	}

	return m.verifyModelTTL(ctx, client, metadata)
}

// enableModelTTL is reserved for tables this call just created. It must never
// run against a table that already existed before the call.
func (m *Manager) enableModelTTL(ctx context.Context, client session.DynamoDBAPI, metadata *model.Metadata) error {
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

// verifyModelTTL reports the TTL state of an existing table. It never calls
// UpdateTimeToLive: the caller decides whether expiring existing items is
// intended.
func (m *Manager) verifyModelTTL(ctx context.Context, client session.DynamoDBAPI, metadata *model.Metadata) error {
	if metadata == nil || metadata.TTLField == nil {
		return nil
	}

	describer, ok := client.(timeToLiveDescriber)
	if !ok {
		return nil
	}

	attributeName := metadata.TTLField.DBName
	out, err := describer.DescribeTimeToLive(ctx, &dynamodb.DescribeTimeToLiveInput{
		TableName: aws.String(metadata.TableName),
	})
	if err != nil {
		return fmt.Errorf("failed to describe ttl on table %s: %w", metadata.TableName, err)
	}

	if out == nil || out.TimeToLiveDescription == nil {
		warnModelTTLDisabled(metadata, attributeName)
		return nil
	}

	description := out.TimeToLiveDescription
	switch description.TimeToLiveStatus {
	case types.TimeToLiveStatusEnabled, types.TimeToLiveStatusEnabling:
		currentAttribute := aws.ToString(description.AttributeName)
		if currentAttribute == attributeName {
			return nil
		}
		return &ttlAttributeMismatchError{
			tableName:        metadata.TableName,
			attributeName:    attributeName,
			currentAttribute: currentAttribute,
		}
	}

	warnModelTTLDisabled(metadata, attributeName)
	return nil
}

func warnModelTTLDisabled(metadata *model.Metadata, attributeName string) {
	log.Printf(
		"tabletheory: table %s exists with ttl disabled, but the model declares ttl attribute %s; "+
			"leaving ttl disabled because enabling it would make DynamoDB expire items that are already past their expiry. "+
			"Enable ttl deliberately with UpdateTimeToLive or your infrastructure-as-code if that expiry is intended.",
		metadata.TableName,
		attributeName,
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

// ttlAttributeMismatchError keeps the established message text while making the
// conflict match errors.ErrTTLAttributeMismatch and errors.ErrInvalidModel.
type ttlAttributeMismatchError struct {
	cause            error
	tableName        string
	attributeName    string
	currentAttribute string
}

func (e *ttlAttributeMismatchError) Error() string {
	message := fmt.Sprintf(
		"table %s has ttl enabled on a different attribute %s, but the model declares ttl attribute %s",
		e.tableName,
		e.currentAttribute,
		e.attributeName,
	)
	if e.cause != nil {
		return message + ": " + e.cause.Error()
	}
	return message
}

func (e *ttlAttributeMismatchError) Unwrap() []error {
	causes := []error{theorydbErrors.ErrTTLAttributeMismatch}
	if e.cause != nil {
		causes = append(causes, e.cause)
	}
	return causes
}
