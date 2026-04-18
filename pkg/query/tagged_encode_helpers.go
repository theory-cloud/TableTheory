package query

import (
	"fmt"
	"reflect"
	"strings"

	"github.com/aws/aws-sdk-go-v2/service/dynamodb/types"

	"github.com/theory-cloud/tabletheory/internal/expr"
	"github.com/theory-cloud/tabletheory/internal/fieldcodec"
	"github.com/theory-cloud/tabletheory/internal/reflectutil"
	"github.com/theory-cloud/tabletheory/pkg/core"
)

func (q *Query) marshalItemTaggedFlat(modelValue reflect.Value) (map[string]types.AttributeValue, error) {
	fieldPlans, err := reflectutil.BuildVisibleFieldPlan(modelValue.Type(), nil)
	if err != nil {
		return nil, err
	}

	out := make(map[string]types.AttributeValue, len(fieldPlans))
	for _, fieldPlan := range fieldPlans {
		field := fieldPlan.Field
		tag := field.Tag.Get("theorydb")
		if tag == "-" {
			continue
		}

		fieldValue := modelValue.FieldByIndex(fieldPlan.IndexPath)
		if !fieldValue.IsValid() {
			continue
		}
		if strings.Contains(tag, "omitempty") && isZeroValue(fieldValue) {
			continue
		}

		av, err := q.marshalTaggedFieldAttributeValue(field, fieldValue)
		if err != nil {
			return nil, err
		}
		out[field.Name] = av
	}

	return out, nil
}

func (q *Query) buildUpdateExpressionFromTaggedVisibleFields(
	builder *expr.Builder,
	modelValue reflect.Value,
	primaryKey core.KeySchema,
) error {
	fieldPlans, err := reflectutil.BuildVisibleFieldPlan(modelValue.Type(), nil)
	if err != nil {
		return err
	}

	for _, fieldPlan := range fieldPlans {
		field := fieldPlan.Field
		tag := field.Tag.Get("theorydb")
		if shouldSkipUpdateField(field, tag, primaryKey) {
			continue
		}

		fieldValue := modelValue.FieldByIndex(fieldPlan.IndexPath)
		if strings.Contains(tag, "omitempty") && isZeroValue(fieldValue) {
			continue
		}

		valueToSet := fieldValue.Interface()
		if fieldcodec.HasJSONModifier(tag) {
			normalized, err := fieldcodec.NormalizeJSONReflectValue(field.Type, fieldValue)
			if err != nil {
				return fmt.Errorf("failed to normalize json field %s: %w", field.Name, err)
			}
			valueToSet = normalized
		}
		if err := builder.AddUpdateSet(q.resolveAttributeName(field.Name), valueToSet); err != nil {
			return fmt.Errorf("failed to build update for %s: %w", field.Name, err)
		}
	}

	return nil
}
