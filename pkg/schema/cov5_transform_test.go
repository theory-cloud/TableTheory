package schema

import (
	"fmt"
	"reflect"
	"testing"

	"github.com/aws/aws-sdk-go-v2/service/dynamodb/types"
	"github.com/stretchr/testify/require"

	"github.com/theory-cloud/tabletheory/pkg/model"
)

func TestTransformValidator_validateModelType_RejectsNonStruct_COV5(t *testing.T) {
	validator := NewTransformValidator(&model.Metadata{}, &model.Metadata{})

	require.Error(t, validator.validateModelType(reflect.TypeOf(0), nil, "source"))
	require.NoError(t, validator.validateModelType(reflect.TypeOf(&struct{}{}), nil, "target"))
}

func TestCallAttributeValueTransform_CoversTypeMismatchPaths_COV5(t *testing.T) {
	source := map[string]types.AttributeValue{
		"id": &types.AttributeValueMemberS{Value: "u1"},
	}

	okTransform := reflect.ValueOf(func(in map[string]types.AttributeValue) (map[string]types.AttributeValue, error) {
		return in, nil
	})
	out, err := callAttributeValueTransform(okTransform, source)
	require.NoError(t, err)
	require.Equal(t, source, out)

	wrongOutput := reflect.ValueOf(func(map[string]types.AttributeValue) (any, error) {
		return "nope", nil
	})
	_, err = callAttributeValueTransform(wrongOutput, source)
	require.Error(t, err)

	wrongError := reflect.ValueOf(func(map[string]types.AttributeValue) (map[string]types.AttributeValue, interface{}) {
		return map[string]types.AttributeValue{}, "not-an-error"
	})
	_, err = callAttributeValueTransform(wrongError, source)
	require.Error(t, err)
}

func TestAugmentAttributeMapForTransform_CoversMetadataMapping_COV5(t *testing.T) {
	meta := &model.Metadata{
		FieldsByDBName: map[string]*model.FieldMetadata{
			"id": {Name: "ID"},
		},
	}

	item := map[string]types.AttributeValue{
		"id": &types.AttributeValueMemberS{Value: "u1"},
	}

	augmented := augmentAttributeMapForTransform(item, meta)
	require.Contains(t, augmented, "id")
	require.Contains(t, augmented, "ID")
}

func TestTransformWithValidation_CoversErrorAndRequiredFieldValidation_COV5(t *testing.T) {
	item := map[string]types.AttributeValue{
		"id": &types.AttributeValueMemberS{Value: "u1"},
	}

	out, err := TransformWithValidation(item, nil, nil, nil)
	require.NoError(t, err)
	require.Equal(t, item, out)

	_, err = TransformWithValidation(item, func(map[string]types.AttributeValue) (map[string]types.AttributeValue, error) {
		return nil, fmt.Errorf("boom")
	}, nil, &model.Metadata{})
	require.Error(t, err)

	target := &model.Metadata{
		PrimaryKey: &model.KeySchema{
			PartitionKey: &model.FieldMetadata{DBName: "pk"},
		},
	}
	_, err = TransformWithValidation(item, func(map[string]types.AttributeValue) (map[string]types.AttributeValue, error) {
		return map[string]types.AttributeValue{}, nil
	}, nil, target)
	require.Error(t, err)
}
