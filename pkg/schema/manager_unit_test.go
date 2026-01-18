package schema

import (
	"reflect"
	"testing"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb/types"
	"github.com/stretchr/testify/require"

	"github.com/theory-cloud/tabletheory/pkg/model"
)

func TestTableOptions_ApplyToCreateTableInput(t *testing.T) {
	t.Run("WithBillingMode pay-per-request clears throughput", func(t *testing.T) {
		input := &dynamodb.CreateTableInput{
			BillingMode: types.BillingModeProvisioned,
			ProvisionedThroughput: &types.ProvisionedThroughput{
				ReadCapacityUnits:  aws.Int64(1),
				WriteCapacityUnits: aws.Int64(1),
			},
		}

		WithBillingMode(types.BillingModePayPerRequest)(input)
		require.Equal(t, types.BillingModePayPerRequest, input.BillingMode)
		require.Nil(t, input.ProvisionedThroughput)
	})

	t.Run("WithThroughput sets provisioned mode", func(t *testing.T) {
		input := &dynamodb.CreateTableInput{}

		WithThroughput(3, 4)(input)
		require.Equal(t, types.BillingModeProvisioned, input.BillingMode)
		require.NotNil(t, input.ProvisionedThroughput)
		require.Equal(t, int64(3), aws.ToInt64(input.ProvisionedThroughput.ReadCapacityUnits))
		require.Equal(t, int64(4), aws.ToInt64(input.ProvisionedThroughput.WriteCapacityUnits))
	})

	t.Run("WithStreamSpecification and WithSSESpecification", func(t *testing.T) {
		input := &dynamodb.CreateTableInput{}

		streamSpec := types.StreamSpecification{StreamEnabled: aws.Bool(true), StreamViewType: types.StreamViewTypeNewImage}
		WithStreamSpecification(streamSpec)(input)
		require.NotNil(t, input.StreamSpecification)
		require.True(t, aws.ToBool(input.StreamSpecification.StreamEnabled))

		sseSpec := types.SSESpecification{Enabled: aws.Bool(true), SSEType: types.SSETypeKms}
		WithSSESpecification(sseSpec)(input)
		require.NotNil(t, input.SSESpecification)
		require.True(t, aws.ToBool(input.SSESpecification.Enabled))
	})
}

func TestManager_BuildKeySchemaAndIndexes(t *testing.T) {
	registry := model.NewRegistry()
	require.NoError(t, registry.Register(&Product{}))

	meta, err := registry.GetMetadata(&Product{})
	require.NoError(t, err)

	manager := NewManager(nil, registry)

	keySchema := manager.buildKeySchema(meta)
	require.Len(t, keySchema, 2)

	gsis, lsis := manager.buildIndexes(meta)
	require.Len(t, gsis, 1)
	require.Len(t, lsis, 1)
	require.Equal(t, "name-index", aws.ToString(gsis[0].IndexName))
	require.Equal(t, "updated-lsi", aws.ToString(lsis[0].IndexName))
}

func TestManager_BuildIndexes_ProjectionInclude(t *testing.T) {
	manager := &Manager{}

	metadata := &model.Metadata{
		PrimaryKey: &model.KeySchema{
			PartitionKey: &model.FieldMetadata{DBName: "pk", Type: reflect.TypeOf("")},
		},
		Indexes: []model.IndexSchema{
			{
				Name:            "gsi-include",
				Type:            model.GlobalSecondaryIndex,
				PartitionKey:    &model.FieldMetadata{DBName: "gpk", Type: reflect.TypeOf("")},
				SortKey:         &model.FieldMetadata{DBName: "gsk", Type: reflect.TypeOf("")},
				ProjectionType:  "INCLUDE",
				ProjectedFields: []string{"a", "b"},
			},
		},
	}

	gsis, lsis := manager.buildIndexes(metadata)
	require.Len(t, gsis, 1)
	require.Empty(t, lsis)
	require.Equal(t, types.ProjectionTypeInclude, gsis[0].Projection.ProjectionType)
	require.Equal(t, []string{"a", "b"}, gsis[0].Projection.NonKeyAttributes)
}

func TestUpdateTableHelpers(t *testing.T) {
	t.Run("applyBillingModeUpdate respects current summary", func(t *testing.T) {
		current := &types.TableDescription{
			BillingModeSummary: &types.BillingModeSummary{BillingMode: types.BillingModePayPerRequest},
		}

		createInput := &dynamodb.CreateTableInput{
			BillingMode: types.BillingModePayPerRequest,
		}

		updateInput := &dynamodb.UpdateTableInput{}
		applyBillingModeUpdate(updateInput, createInput, current)
		require.Empty(t, updateInput.BillingMode)

		createInput = &dynamodb.CreateTableInput{
			BillingMode: types.BillingModeProvisioned,
			ProvisionedThroughput: &types.ProvisionedThroughput{
				ReadCapacityUnits:  aws.Int64(1),
				WriteCapacityUnits: aws.Int64(1),
			},
		}
		applyBillingModeUpdate(updateInput, createInput, current)
		require.Equal(t, types.BillingModeProvisioned, updateInput.BillingMode)
		require.NotNil(t, updateInput.ProvisionedThroughput)
	})

	t.Run("applyStreamUpdate and applySSEUpdate", func(t *testing.T) {
		updateInput := &dynamodb.UpdateTableInput{}
		createInput := &dynamodb.CreateTableInput{
			StreamSpecification: &types.StreamSpecification{StreamEnabled: aws.Bool(true)},
			SSESpecification:    &types.SSESpecification{Enabled: aws.Bool(true)},
		}

		applyStreamUpdate(updateInput, createInput)
		applySSEUpdate(updateInput, createInput)

		require.NotNil(t, updateInput.StreamSpecification)
		require.NotNil(t, updateInput.SSESpecification)
	})
}

func TestApplyGSIUpdates(t *testing.T) {
	manager := &Manager{}

	t.Run("no changes", func(t *testing.T) {
		registry := model.NewRegistry()
		require.NoError(t, registry.Register(&User{}))
		meta, err := registry.GetMetadata(&User{})
		require.NoError(t, err)

		current := &types.TableDescription{}
		updateInput := &dynamodb.UpdateTableInput{}
		require.NoError(t, manager.applyGSIUpdates(updateInput, meta, current))
		require.Nil(t, updateInput.GlobalSecondaryIndexUpdates)
	})

	t.Run("fails when multiple changes are required", func(t *testing.T) {
		registry := model.NewRegistry()
		require.NoError(t, registry.Register(&Order{}))
		meta, err := registry.GetMetadata(&Order{})
		require.NoError(t, err)

		current := &types.TableDescription{BillingModeSummary: &types.BillingModeSummary{BillingMode: types.BillingModeProvisioned}}
		updateInput := &dynamodb.UpdateTableInput{}
		require.Error(t, manager.applyGSIUpdates(updateInput, meta, current))
	})

	t.Run("creates a single GSI update", func(t *testing.T) {
		registry := model.NewRegistry()
		require.NoError(t, registry.Register(&Product{}))
		meta, err := registry.GetMetadata(&Product{})
		require.NoError(t, err)

		current := &types.TableDescription{BillingModeSummary: &types.BillingModeSummary{BillingMode: types.BillingModeProvisioned}}
		updateInput := &dynamodb.UpdateTableInput{}
		require.NoError(t, manager.applyGSIUpdates(updateInput, meta, current))
		require.Len(t, updateInput.GlobalSecondaryIndexUpdates, 1)
		require.NotNil(t, updateInput.GlobalSecondaryIndexUpdates[0].Create)
		require.NotNil(t, updateInput.GlobalSecondaryIndexUpdates[0].Create.ProvisionedThroughput)
	})

	t.Run("deletes a single GSI update", func(t *testing.T) {
		registry := model.NewRegistry()
		require.NoError(t, registry.Register(&User{}))
		meta, err := registry.GetMetadata(&User{})
		require.NoError(t, err)

		current := &types.TableDescription{
			GlobalSecondaryIndexes: []types.GlobalSecondaryIndexDescription{
				{IndexName: aws.String("old-index")},
			},
		}
		updateInput := &dynamodb.UpdateTableInput{}
		require.NoError(t, manager.applyGSIUpdates(updateInput, meta, current))
		require.Len(t, updateInput.GlobalSecondaryIndexUpdates, 1)
		require.NotNil(t, updateInput.GlobalSecondaryIndexUpdates[0].Delete)
		require.Equal(t, "old-index", aws.ToString(updateInput.GlobalSecondaryIndexUpdates[0].Delete.IndexName))
	})
}

func TestMarkerOptionsAndInputBuilder(t *testing.T) {
	input := buildCreateTableInput([]TableOption{
		WithGSICreate("g", "pk", "sk", types.ProjectionTypeAll),
		WithGSIDelete("g"),
	})

	// Marker options should not mutate the input.
	require.Equal(t, &dynamodb.CreateTableInput{}, input)
}
