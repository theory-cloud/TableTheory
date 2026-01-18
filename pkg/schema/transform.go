package schema

import (
	"fmt"
	"reflect"

	"github.com/aws/aws-sdk-go-v2/service/dynamodb/types"

	"github.com/theory-cloud/tabletheory/pkg/marshal"
	"github.com/theory-cloud/tabletheory/pkg/model"
	pkgTypes "github.com/theory-cloud/tabletheory/pkg/types"
)

// TransformFunc defines a function that transforms data during migration
// It takes a source item and returns a transformed target item
type TransformFunc func(source map[string]types.AttributeValue) (map[string]types.AttributeValue, error)

// ModelTransformFunc defines a function that transforms between model types
// It takes a source model instance and returns a target model instance
type ModelTransformFunc interface{}

// TransformValidator validates transform operations
type TransformValidator struct {
	sourceMetadata *model.Metadata
	targetMetadata *model.Metadata
}

// NewTransformValidator creates a new transform validator
func NewTransformValidator(sourceMetadata, targetMetadata *model.Metadata) *TransformValidator {
	return &TransformValidator{
		sourceMetadata: sourceMetadata,
		targetMetadata: targetMetadata,
	}
}

// ValidateTransform validates that a transform function is compatible with the source and target models
func (v *TransformValidator) ValidateTransform(transform interface{}) error {
	if transform == nil {
		return nil
	}

	transformType := reflect.TypeOf(transform)
	if transformType.Kind() != reflect.Func {
		return fmt.Errorf("transform must be a function, got %T", transform)
	}

	// Check function signature for model-to-model transforms
	if transformType.NumIn() == 1 && transformType.NumOut() == 1 {
		inputType := transformType.In(0)
		outputType := transformType.Out(0)

		// Validate input type matches source model
		if err := v.validateModelType(inputType, v.sourceMetadata, "source"); err != nil {
			return err
		}

		// Validate output type matches target model
		return v.validateModelType(outputType, v.targetMetadata, "target")
	}

	// Check function signature for AttributeValue transforms
	if transformType.NumIn() == 1 && transformType.NumOut() == 2 {
		inputType := transformType.In(0)
		outputType := transformType.Out(0)
		errorType := transformType.Out(1)

		// Check if it's a map[string]types.AttributeValue transform
		expectedInputType := reflect.TypeOf(map[string]types.AttributeValue{})
		expectedOutputType := reflect.TypeOf(map[string]types.AttributeValue{})
		expectedErrorType := reflect.TypeOf((*error)(nil)).Elem()

		if inputType == expectedInputType && outputType == expectedOutputType && errorType == expectedErrorType {
			return nil
		}
	}

	return fmt.Errorf("invalid transform function signature: expected func(SourceModel) TargetModel or func(map[string]types.AttributeValue) (map[string]types.AttributeValue, error)")
}

// validateModelType validates that a reflect.Type matches the expected model metadata
func (v *TransformValidator) validateModelType(modelType reflect.Type, _ *model.Metadata, role string) error {
	// Handle pointer types
	if modelType.Kind() == reflect.Ptr {
		modelType = modelType.Elem()
	}

	if modelType.Kind() != reflect.Struct {
		return fmt.Errorf("%s model must be a struct or pointer to struct, got %s", role, modelType.Kind())
	}

	// Additional validation could be added here to check field compatibility
	return nil
}

// CreateModelTransform creates a transform function from a model-to-model function
func CreateModelTransform(transformFunc interface{}, sourceMetadata, targetMetadata *model.Metadata) (TransformFunc, error) {
	if transformFunc == nil {
		return nil, nil
	}

	transformValue := reflect.ValueOf(transformFunc)
	transformType := transformValue.Type()

	validator := NewTransformValidator(sourceMetadata, targetMetadata)
	if err := validator.ValidateTransform(transformFunc); err != nil {
		return nil, err
	}

	if isAttributeValueTransform(transformType) {
		return wrapAttributeValueTransform(transformValue, sourceMetadata), nil
	}

	converter := pkgTypes.NewConverter()
	marshalerFactory := marshal.NewMarshalerFactory(marshal.DefaultConfig()).WithConverter(converter)
	marshaler, err := marshalerFactory.NewMarshaler()
	if err != nil {
		return nil, fmt.Errorf("failed to create marshaler: %w", err)
	}

	return wrapModelTransform(transformValue, transformType, sourceMetadata, targetMetadata, converter, marshaler), nil
}

func isAttributeValueTransform(transformType reflect.Type) bool {
	if transformType.NumIn() != 1 || transformType.NumOut() != 2 {
		return false
	}

	inputType := transformType.In(0)
	outputType := transformType.Out(0)
	errorType := transformType.Out(1)

	expectedMapType := reflect.TypeOf(map[string]types.AttributeValue{})
	expectedErrorType := reflect.TypeOf((*error)(nil)).Elem()

	return inputType == expectedMapType && outputType == expectedMapType && errorType == expectedErrorType
}

func wrapAttributeValueTransform(transformValue reflect.Value, sourceMetadata *model.Metadata) TransformFunc {
	return func(source map[string]types.AttributeValue) (map[string]types.AttributeValue, error) {
		augmented := augmentAttributeMapForTransform(source, sourceMetadata)
		return callAttributeValueTransform(transformValue, augmented)
	}
}

func callAttributeValueTransform(transformValue reflect.Value, source map[string]types.AttributeValue) (map[string]types.AttributeValue, error) {
	results := transformValue.Call([]reflect.Value{reflect.ValueOf(source)})

	output, ok := results[0].Interface().(map[string]types.AttributeValue)
	if !ok {
		return nil, fmt.Errorf("transform returned %T; expected map[string]types.AttributeValue", results[0].Interface())
	}

	if results[1].IsNil() {
		return output, nil
	}

	err, ok := results[1].Interface().(error)
	if !ok {
		return nil, fmt.Errorf("transform returned %T; expected error", results[1].Interface())
	}

	return output, err
}

func wrapModelTransform(
	transformValue reflect.Value,
	transformType reflect.Type,
	sourceMetadata *model.Metadata,
	targetMetadata *model.Metadata,
	converter *pkgTypes.Converter,
	marshaler marshal.MarshalerInterface,
) TransformFunc {
	return func(sourceItem map[string]types.AttributeValue) (map[string]types.AttributeValue, error) {
		sourceModel, err := buildSourceModel(transformType, sourceItem, sourceMetadata, converter)
		if err != nil {
			return nil, err
		}

		targetModel, err := callModelTransform(transformValue, sourceModel)
		if err != nil {
			return nil, err
		}

		targetMap, err := marshaler.MarshalItem(targetModel, targetMetadata)
		if err != nil {
			return nil, fmt.Errorf("failed to marshal target item: %w", err)
		}

		return targetMap, nil
	}
}

func buildSourceModel(
	transformType reflect.Type,
	sourceItem map[string]types.AttributeValue,
	sourceMetadata *model.Metadata,
	converter *pkgTypes.Converter,
) (reflect.Value, error) {
	sourceModelType := transformType.In(0)
	if sourceModelType.Kind() == reflect.Ptr {
		sourceModelType = sourceModelType.Elem()
	}
	sourceModel := reflect.New(sourceModelType).Elem()

	for attrName, attrValue := range sourceItem {
		field, exists := sourceMetadata.FieldsByDBName[attrName]
		if !exists {
			continue
		}

		structField := sourceModel.FieldByIndex(field.IndexPath)
		if !structField.CanSet() {
			continue
		}

		if err := converter.FromAttributeValue(attrValue, structField.Addr().Interface()); err != nil {
			return reflect.Value{}, fmt.Errorf("failed to unmarshal field %s: %w", field.Name, err)
		}
	}

	return sourceModel, nil
}

func callModelTransform(transformValue reflect.Value, sourceModel reflect.Value) (any, error) {
	results := transformValue.Call([]reflect.Value{sourceModel})
	if len(results) != 1 {
		return nil, fmt.Errorf("transform function must return exactly one value")
	}
	return results[0].Interface(), nil
}

func augmentAttributeMapForTransform(item map[string]types.AttributeValue, metadata *model.Metadata) map[string]types.AttributeValue {
	if metadata == nil || len(metadata.FieldsByDBName) == 0 {
		cloned := make(map[string]types.AttributeValue, len(item))
		for k, v := range item {
			cloned[k] = v
		}
		return cloned
	}

	augmented := make(map[string]types.AttributeValue, len(item)*2)
	for k, v := range item {
		augmented[k] = v
		if fieldMeta, exists := metadata.FieldsByDBName[k]; exists {
			if _, ok := augmented[fieldMeta.Name]; !ok {
				augmented[fieldMeta.Name] = v
			}
		}
	}
	return augmented
}

// TransformWithValidation applies a transform with validation and error handling
func TransformWithValidation(item map[string]types.AttributeValue, transform TransformFunc,
	sourceMetadata, targetMetadata *model.Metadata) (map[string]types.AttributeValue, error) {

	if transform == nil {
		return item, nil
	}

	_ = sourceMetadata

	// Apply transform
	transformedItem, err := transform(item)
	if err != nil {
		return nil, fmt.Errorf("transform function failed: %w", err)
	}

	// Validate required fields in target
	if err := validateRequiredFields(transformedItem, targetMetadata); err != nil {
		return nil, fmt.Errorf("transform validation failed: %w", err)
	}

	return transformedItem, nil
}

// validateRequiredFields ensures all required fields are present in the transformed item
func validateRequiredFields(item map[string]types.AttributeValue, metadata *model.Metadata) error {
	// Check primary key fields
	if metadata.PrimaryKey.PartitionKey != nil {
		pkField := metadata.PrimaryKey.PartitionKey
		if _, exists := item[pkField.DBName]; !exists {
			return fmt.Errorf("missing required partition key field: %s", pkField.DBName)
		}
	}

	if metadata.PrimaryKey.SortKey != nil {
		skField := metadata.PrimaryKey.SortKey
		if _, exists := item[skField.DBName]; !exists {
			return fmt.Errorf("missing required sort key field: %s", skField.DBName)
		}
	}

	// Additional validation for other fields could be added here if needed.

	return nil
}

// Common transform utilities

// CopyAllFields creates a transform that copies all fields from source to target
func CopyAllFields() TransformFunc {
	return func(source map[string]types.AttributeValue) (map[string]types.AttributeValue, error) {
		target := make(map[string]types.AttributeValue, len(source))
		for k, v := range source {
			target[k] = v
		}
		return target, nil
	}
}

// RenameField creates a transform that renames a field
func RenameField(oldName, newName string) TransformFunc {
	return func(source map[string]types.AttributeValue) (map[string]types.AttributeValue, error) {
		target := make(map[string]types.AttributeValue, len(source))
		for k, v := range source {
			if k == oldName {
				target[newName] = v
			} else {
				target[k] = v
			}
		}
		return target, nil
	}
}

// AddField creates a transform that adds a new field with a default value
func AddField(fieldName string, defaultValue types.AttributeValue) TransformFunc {
	return func(source map[string]types.AttributeValue) (map[string]types.AttributeValue, error) {
		target := make(map[string]types.AttributeValue, len(source)+1)
		for k, v := range source {
			target[k] = v
		}
		target[fieldName] = defaultValue
		return target, nil
	}
}

// RemoveField creates a transform that removes a field
func RemoveField(fieldName string) TransformFunc {
	return func(source map[string]types.AttributeValue) (map[string]types.AttributeValue, error) {
		target := make(map[string]types.AttributeValue, len(source))
		for k, v := range source {
			if k != fieldName {
				target[k] = v
			}
		}
		return target, nil
	}
}

// ChainTransforms combines multiple transforms into a single transform
func ChainTransforms(transforms ...TransformFunc) TransformFunc {
	return func(source map[string]types.AttributeValue) (map[string]types.AttributeValue, error) {
		current := source
		for i, transform := range transforms {
			if transform == nil {
				continue
			}
			var err error
			current, err = transform(current)
			if err != nil {
				return nil, fmt.Errorf("transform %d failed: %w", i, err)
			}
		}
		return current, nil
	}
}
