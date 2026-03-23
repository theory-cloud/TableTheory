package schema

import (
	"context"
	"fmt"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb/types"

	"github.com/theory-cloud/tabletheory/pkg/model"
)

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
	client, err := m.session.Client()
	if err != nil {
		return fmt.Errorf("failed to get client for ttl update: %w", err)
	}

	return m.syncModelTTL(ctx, client, metadata)
}

func (m *Manager) syncModelTTL(ctx context.Context, client *dynamodb.Client, metadata *model.Metadata) error {
	if metadata == nil || metadata.TTLField == nil {
		return nil
	}

	_, err := client.UpdateTimeToLive(ctx, &dynamodb.UpdateTimeToLiveInput{
		TableName: aws.String(metadata.TableName),
		TimeToLiveSpecification: &types.TimeToLiveSpecification{
			AttributeName: aws.String(metadata.TTLField.DBName),
			Enabled:       aws.Bool(true),
		},
	})
	if err != nil {
		return fmt.Errorf(
			"failed to enable ttl on table %s using attribute %s: %w",
			metadata.TableName,
			metadata.TTLField.DBName,
			err,
		)
	}

	return nil
}
