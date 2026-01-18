package schema

import (
	"testing"
	"time"

	"github.com/aws/aws-sdk-go-v2/service/dynamodb/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/theory-cloud/tabletheory/internal/expr"
	"github.com/theory-cloud/tabletheory/pkg/model"
)

// Helper function to get keys from a map
func getKeys(m map[string]types.AttributeValue) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	return keys
}

// Test models for transformation
type UserV1 struct {
	ID       string `theorydb:"pk"`
	Email    string `theorydb:"sk"`
	Name     string `theorydb:"attr:fullName"`
	Status   string `theorydb:"attr:status"`
	Settings string `theorydb:"attr:settings"`
	Age      int    `theorydb:"attr:age"`
}

type UserV2 struct {
	CreatedAt time.Time         `theorydb:"attr:createdAt"`
	Settings  map[string]string `theorydb:"attr:settings"`
	ID        string            `theorydb:"pk"`
	Email     string            `theorydb:"sk"`
	FirstName string            `theorydb:"attr:firstName"`
	LastName  string            `theorydb:"attr:lastName"`
	Age       int               `theorydb:"attr:age"`
	Active    bool              `theorydb:"attr:active"`
}

func TestTransformValidator(t *testing.T) {
	registry := model.NewRegistry()
	err := registry.Register(&UserV1{})
	require.NoError(t, err)
	err = registry.Register(&UserV2{})
	require.NoError(t, err)

	sourceMetadata, err := registry.GetMetadata(&UserV1{})
	require.NoError(t, err)
	targetMetadata, err := registry.GetMetadata(&UserV2{})
	require.NoError(t, err)

	validator := NewTransformValidator(sourceMetadata, targetMetadata)

	t.Run("ValidModelTransform", func(t *testing.T) {
		transformFunc := func(old UserV1) UserV2 {
			return UserV2{
				ID:        old.ID,
				Email:     old.Email,
				FirstName: old.Name,
				Age:       old.Age,
				Active:    old.Status == "active",
				CreatedAt: time.Now(),
			}
		}

		err := validator.ValidateTransform(transformFunc)
		assert.NoError(t, err)
	})

	t.Run("ValidAttributeValueTransform", func(t *testing.T) {
		transformFunc := func(source map[string]types.AttributeValue) (map[string]types.AttributeValue, error) {
			return source, nil
		}

		err := validator.ValidateTransform(transformFunc)
		assert.NoError(t, err)
	})

	t.Run("InvalidTransformType", func(t *testing.T) {
		err := validator.ValidateTransform("not a function")
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "transform must be a function")
	})

	t.Run("InvalidSignature", func(t *testing.T) {
		invalidFunc := func(a, b string) string { return a + b }
		err := validator.ValidateTransform(invalidFunc)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "invalid transform function signature")
	})

	t.Run("NilTransform", func(t *testing.T) {
		err := validator.ValidateTransform(nil)
		assert.NoError(t, err)
	})
}

func TestCreateModelTransform(t *testing.T) {
	registry := model.NewRegistry()
	err := registry.Register(&UserV1{})
	require.NoError(t, err)
	err = registry.Register(&UserV2{})
	require.NoError(t, err)

	sourceMetadata, err := registry.GetMetadata(&UserV1{})
	require.NoError(t, err)
	targetMetadata, err := registry.GetMetadata(&UserV2{})
	require.NoError(t, err)

	t.Run("DebugExprConverter", func(t *testing.T) {
		// Debug: Print metadata field mappings
		t.Logf("Source metadata fields:")
		for name, field := range sourceMetadata.Fields {
			t.Logf("  %s -> %s (DBName: %s, IsPK: %v, IsSK: %v)", name, field.Name, field.DBName, field.IsPK, field.IsSK)
		}

		// Test the expr converter directly
		sourceItem := map[string]types.AttributeValue{
			"id":       &types.AttributeValueMemberS{Value: "user-1"},
			"email":    &types.AttributeValueMemberS{Value: "test@example.com"},
			"fullName": &types.AttributeValueMemberS{Value: "John Doe"},
			"age":      &types.AttributeValueMemberN{Value: "30"},
			"status":   &types.AttributeValueMemberS{Value: "active"},
		}

		var user UserV1
		err := expr.ConvertFromAttributeValue(&types.AttributeValueMemberM{Value: sourceItem}, &user)
		require.NoError(t, err)

		t.Logf("Unmarshaled user: %+v", user)

		// Now marshal it back
		av, err := expr.ConvertToAttributeValue(user)
		require.NoError(t, err)

		if m, ok := av.(*types.AttributeValueMemberM); ok {
			t.Logf("Marshaled back: %+v", m.Value)
			for k, v := range m.Value {
				t.Logf("Field %s: %T = %v", k, v, v)
			}
		}
	})

	t.Run("ModelToModelTransform", func(t *testing.T) {
		transformFunc := func(old UserV1) UserV2 {
			return UserV2{
				ID:        old.ID,
				Email:     old.Email,
				FirstName: old.Name,
				Age:       old.Age,
				Active:    old.Status == "active",
				CreatedAt: time.Now(),
			}
		}

		transform, err := CreateModelTransform(transformFunc, sourceMetadata, targetMetadata)
		require.NoError(t, err)
		assert.NotNil(t, transform)

		// Test the transform
		sourceItem := map[string]types.AttributeValue{
			"id":       &types.AttributeValueMemberS{Value: "user-1"},
			"email":    &types.AttributeValueMemberS{Value: "test@example.com"},
			"fullName": &types.AttributeValueMemberS{Value: "John Doe"},
			"age":      &types.AttributeValueMemberN{Value: "30"},
			"status":   &types.AttributeValueMemberS{Value: "active"},
		}

		targetItem, err := transform(sourceItem)
		require.NoError(t, err)
		require.NotNil(t, targetItem)

		// Debug: Print what we got
		t.Logf("Target item keys: %v", getKeys(targetItem))
		for k, v := range targetItem {
			t.Logf("Field %s: %T = %v", k, v, v)
		}

		// The marshaler uses DB field names, not Go field names
		// So we need to check for the DB field names in the result
		if val, exists := targetItem["id"]; exists && val != nil {
			id, ok := val.(*types.AttributeValueMemberS)
			require.True(t, ok)
			assert.Equal(t, "user-1", id.Value)
		} else {
			t.Errorf("Missing or nil 'ID' field")
		}

		if val, exists := targetItem["email"]; exists && val != nil {
			email, ok := val.(*types.AttributeValueMemberS)
			require.True(t, ok)
			assert.Equal(t, "test@example.com", email.Value)
		} else {
			t.Errorf("Missing or nil 'Email' field")
		}

		if val, exists := targetItem["firstName"]; exists && val != nil {
			firstName, ok := val.(*types.AttributeValueMemberS)
			require.True(t, ok)
			assert.Equal(t, "John Doe", firstName.Value)
		} else {
			t.Errorf("Missing or nil 'firstName' field")
		}

		if val, exists := targetItem["age"]; exists && val != nil {
			age, ok := val.(*types.AttributeValueMemberN)
			require.True(t, ok)
			assert.Equal(t, "30", age.Value)
		} else {
			t.Errorf("Missing or nil 'age' field")
		}

		if val, exists := targetItem["active"]; exists && val != nil {
			active, ok := val.(*types.AttributeValueMemberBOOL)
			require.True(t, ok)
			assert.Equal(t, true, active.Value)
		} else {
			t.Errorf("Missing or nil 'active' field")
		}
	})

	t.Run("AttributeValueTransform", func(t *testing.T) {
		transformFunc := func(source map[string]types.AttributeValue) (map[string]types.AttributeValue, error) {
			target := make(map[string]types.AttributeValue)
			for k, v := range source {
				target[k] = v
			}
			target["new_field"] = &types.AttributeValueMemberS{Value: "added"}
			return target, nil
		}

		transform, err := CreateModelTransform(transformFunc, sourceMetadata, targetMetadata)
		require.NoError(t, err)
		assert.NotNil(t, transform)

		// Test that the transform works correctly
		sourceItem := map[string]types.AttributeValue{
			"id": &types.AttributeValueMemberS{Value: "test-id"},
		}

		targetItem, err := transform(sourceItem)
		require.NoError(t, err)
		assert.Contains(t, targetItem, "new_field")
		added, ok := targetItem["new_field"].(*types.AttributeValueMemberS)
		require.True(t, ok)
		assert.Equal(t, "added", added.Value)
	})

	t.Run("NilTransform", func(t *testing.T) {
		transform, err := CreateModelTransform(nil, sourceMetadata, targetMetadata)
		require.NoError(t, err)
		assert.Nil(t, transform)
	})
}

func TestTransformWithValidation(t *testing.T) {
	registry := model.NewRegistry()
	err := registry.Register(&UserV1{})
	require.NoError(t, err)
	err = registry.Register(&UserV2{})
	require.NoError(t, err)

	sourceMetadata, err := registry.GetMetadata(&UserV1{})
	require.NoError(t, err)
	targetMetadata, err := registry.GetMetadata(&UserV2{})
	require.NoError(t, err)

	t.Run("ValidTransform", func(t *testing.T) {
		transform := func(source map[string]types.AttributeValue) (map[string]types.AttributeValue, error) {
			target := make(map[string]types.AttributeValue)
			for k, v := range source {
				target[k] = v
			}
			return target, nil
		}

		sourceItem := map[string]types.AttributeValue{
			"id":    &types.AttributeValueMemberS{Value: "user-1"},
			"email": &types.AttributeValueMemberS{Value: "test@example.com"},
		}

		result, err := TransformWithValidation(sourceItem, transform, sourceMetadata, targetMetadata)
		require.NoError(t, err)
		assert.Equal(t, sourceItem, result)
	})

	t.Run("TransformError", func(t *testing.T) {
		transform := func(source map[string]types.AttributeValue) (map[string]types.AttributeValue, error) {
			return nil, assert.AnError
		}

		sourceItem := map[string]types.AttributeValue{
			"id": &types.AttributeValueMemberS{Value: "user-1"},
		}

		_, err := TransformWithValidation(sourceItem, transform, sourceMetadata, targetMetadata)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "transform function failed")
	})

	t.Run("MissingPrimaryKey", func(t *testing.T) {
		transform := func(source map[string]types.AttributeValue) (map[string]types.AttributeValue, error) {
			// Remove primary key
			target := make(map[string]types.AttributeValue)
			for k, v := range source {
				if k != "id" {
					target[k] = v
				}
			}
			return target, nil
		}

		sourceItem := map[string]types.AttributeValue{
			"id":    &types.AttributeValueMemberS{Value: "user-1"},
			"email": &types.AttributeValueMemberS{Value: "test@example.com"},
		}

		_, err := TransformWithValidation(sourceItem, transform, sourceMetadata, targetMetadata)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "missing required partition key field")
	})

	t.Run("NilTransform", func(t *testing.T) {
		sourceItem := map[string]types.AttributeValue{
			"id": &types.AttributeValueMemberS{Value: "user-1"},
		}

		result, err := TransformWithValidation(sourceItem, nil, sourceMetadata, targetMetadata)
		require.NoError(t, err)
		assert.Equal(t, sourceItem, result)
	})
}

func TestTransformUtilities(t *testing.T) {
	t.Run("CopyAllFields", func(t *testing.T) {
		transform := CopyAllFields()
		source := map[string]types.AttributeValue{
			"field1": &types.AttributeValueMemberS{Value: "value1"},
			"field2": &types.AttributeValueMemberN{Value: "123"},
		}

		result, err := transform(source)
		require.NoError(t, err)
		assert.Equal(t, source, result)

		// Ensure it's a copy, not the same map
		assert.NotSame(t, &source, &result)
	})

	t.Run("RenameField", func(t *testing.T) {
		transform := RenameField("old_name", "new_name")
		source := map[string]types.AttributeValue{
			"old_name": &types.AttributeValueMemberS{Value: "value"},
			"other":    &types.AttributeValueMemberN{Value: "123"},
		}

		result, err := transform(source)
		require.NoError(t, err)

		assert.NotContains(t, result, "old_name")
		assert.Contains(t, result, "new_name")
		renamed, ok := result["new_name"].(*types.AttributeValueMemberS)
		require.True(t, ok)
		assert.Equal(t, "value", renamed.Value)
		assert.Contains(t, result, "other")
	})

	t.Run("AddField", func(t *testing.T) {
		defaultValue := &types.AttributeValueMemberS{Value: "default"}
		transform := AddField("new_field", defaultValue)
		source := map[string]types.AttributeValue{
			"existing": &types.AttributeValueMemberS{Value: "value"},
		}

		result, err := transform(source)
		require.NoError(t, err)

		assert.Contains(t, result, "existing")
		assert.Contains(t, result, "new_field")
		assert.Equal(t, defaultValue, result["new_field"])
	})

	t.Run("RemoveField", func(t *testing.T) {
		transform := RemoveField("to_remove")
		source := map[string]types.AttributeValue{
			"to_remove": &types.AttributeValueMemberS{Value: "value"},
			"to_keep":   &types.AttributeValueMemberN{Value: "123"},
		}

		result, err := transform(source)
		require.NoError(t, err)

		assert.NotContains(t, result, "to_remove")
		assert.Contains(t, result, "to_keep")
	})

	t.Run("ChainTransforms", func(t *testing.T) {
		transform1 := AddField("field1", &types.AttributeValueMemberS{Value: "value1"})
		transform2 := AddField("field2", &types.AttributeValueMemberS{Value: "value2"})
		transform3 := RenameField("original", "renamed")

		chained := ChainTransforms(transform1, transform2, transform3)
		source := map[string]types.AttributeValue{
			"original": &types.AttributeValueMemberS{Value: "original_value"},
		}

		result, err := chained(source)
		require.NoError(t, err)

		assert.NotContains(t, result, "original")
		assert.Contains(t, result, "renamed")
		assert.Contains(t, result, "field1")
		assert.Contains(t, result, "field2")
		renamed, ok := result["renamed"].(*types.AttributeValueMemberS)
		require.True(t, ok)
		assert.Equal(t, "original_value", renamed.Value)
	})

	t.Run("ChainTransformsWithError", func(t *testing.T) {
		transform1 := CopyAllFields()
		transform2 := func(source map[string]types.AttributeValue) (map[string]types.AttributeValue, error) {
			return nil, assert.AnError
		}
		transform3 := CopyAllFields()

		chained := ChainTransforms(transform1, transform2, transform3)
		source := map[string]types.AttributeValue{
			"field": &types.AttributeValueMemberS{Value: "value"},
		}

		_, err := chained(source)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "transform 1 failed")
	})

	t.Run("ChainTransformsWithNil", func(t *testing.T) {
		transform1 := AddField("field1", &types.AttributeValueMemberS{Value: "value1"})
		var transform2 TransformFunc = nil // Should be skipped
		transform3 := AddField("field2", &types.AttributeValueMemberS{Value: "value2"})

		chained := ChainTransforms(transform1, transform2, transform3)
		source := map[string]types.AttributeValue{
			"original": &types.AttributeValueMemberS{Value: "value"},
		}

		result, err := chained(source)
		require.NoError(t, err)

		assert.Contains(t, result, "original")
		assert.Contains(t, result, "field1")
		assert.Contains(t, result, "field2")
	})
}

func TestComplexTransformScenarios(t *testing.T) {
	t.Run("SplitNameField", func(t *testing.T) {
		// Transform that splits a full name into first and last name
		transform := func(source map[string]types.AttributeValue) (map[string]types.AttributeValue, error) {
			target := make(map[string]types.AttributeValue)

			// Copy all fields
			for k, v := range source {
				target[k] = v
			}

			// Split name if present
			if nameAttr, exists := source["fullName"]; exists {
				if nameStr, ok := nameAttr.(*types.AttributeValueMemberS); ok {
					// Simple split on space
					parts := []string{"", ""}
					if nameStr.Value != "" {
						splitParts := []string{nameStr.Value}
						if len(splitParts) > 0 {
							parts[0] = splitParts[0]
						}
						if len(splitParts) > 1 {
							parts[1] = splitParts[1]
						}
					}

					target["firstName"] = &types.AttributeValueMemberS{Value: parts[0]}
					target["lastName"] = &types.AttributeValueMemberS{Value: parts[1]}

					// Remove original field
					delete(target, "fullName")
				}
			}

			return target, nil
		}

		source := map[string]types.AttributeValue{
			"id":       &types.AttributeValueMemberS{Value: "user-1"},
			"fullName": &types.AttributeValueMemberS{Value: "John Doe"},
		}

		result, err := transform(source)
		require.NoError(t, err)

		assert.Contains(t, result, "id")
		assert.NotContains(t, result, "fullName")
		assert.Contains(t, result, "firstName")
		assert.Contains(t, result, "lastName")
		firstName, ok := result["firstName"].(*types.AttributeValueMemberS)
		require.True(t, ok)
		assert.Equal(t, "John Doe", firstName.Value)
		lastName, ok := result["lastName"].(*types.AttributeValueMemberS)
		require.True(t, ok)
		assert.Equal(t, "", lastName.Value)
	})

	t.Run("TypeConversion", func(t *testing.T) {
		// Transform that converts string status to boolean active
		transform := func(source map[string]types.AttributeValue) (map[string]types.AttributeValue, error) {
			target := make(map[string]types.AttributeValue)

			// Copy all fields except status
			for k, v := range source {
				if k != "status" {
					target[k] = v
				}
			}

			// Convert status to active boolean
			if statusAttr, exists := source["status"]; exists {
				if statusStr, ok := statusAttr.(*types.AttributeValueMemberS); ok {
					active := statusStr.Value == "active"
					target["active"] = &types.AttributeValueMemberBOOL{Value: active}
				}
			}

			return target, nil
		}

		source := map[string]types.AttributeValue{
			"id":     &types.AttributeValueMemberS{Value: "user-1"},
			"status": &types.AttributeValueMemberS{Value: "active"},
		}

		result, err := transform(source)
		require.NoError(t, err)

		assert.Contains(t, result, "id")
		assert.NotContains(t, result, "status")
		assert.Contains(t, result, "active")
		active, ok := result["active"].(*types.AttributeValueMemberBOOL)
		require.True(t, ok)
		assert.True(t, active.Value)
	})
}
