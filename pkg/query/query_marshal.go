package query

import (
	"fmt"
	"reflect"
	"time"

	"github.com/aws/aws-sdk-go-v2/service/dynamodb/types"

	"github.com/theory-cloud/tabletheory/v3/internal/expr"
	"github.com/theory-cloud/tabletheory/v3/internal/fieldcodec"
	"github.com/theory-cloud/tabletheory/v3/internal/reflectutil"
	"github.com/theory-cloud/tabletheory/v3/pkg/model"
)

func (q *Query) marshalItem(item any) (map[string]types.AttributeValue, error) {
	if q == nil {
		return nil, fmt.Errorf("query cannot be nil")
	}

	if q.rawMetadata != nil {
		if q.marshaler != nil {
			return q.marshaler.MarshalItem(item, q.rawMetadata)
		}
		return q.marshalItemReflect(item)
	}

	return q.marshalItemTagged(item)
}

func (q *Query) marshalItemReflect(item any) (map[string]types.AttributeValue, error) {
	if q == nil || q.rawMetadata == nil {
		return nil, fmt.Errorf("model metadata is required for reflection marshal")
	}

	modelValue := reflect.ValueOf(item)
	if modelValue.Kind() == reflect.Ptr {
		if modelValue.IsNil() {
			return nil, fmt.Errorf("item cannot be nil")
		}
		modelValue = modelValue.Elem()
	}
	if modelValue.Kind() != reflect.Struct {
		return nil, fmt.Errorf("item must be a struct or pointer to struct")
	}

	itemMap := make(map[string]types.AttributeValue)
	now := time.Now()

	for _, fieldMeta := range q.rawMetadata.Fields {
		if fieldMeta == nil {
			continue
		}

		av, skip, err := q.marshalFieldValueReflect(modelValue, fieldMeta, now)
		if err != nil {
			return nil, err
		}
		if skip {
			continue
		}
		itemMap[fieldMeta.DBName] = av
	}

	return itemMap, nil
}

func (q *Query) marshalFieldValueReflect(modelValue reflect.Value, fieldMeta *model.FieldMetadata, now time.Time) (types.AttributeValue, bool, error) {
	fieldValue := modelValue.FieldByIndex(fieldMeta.IndexPath)
	if fieldMeta.OmitEmpty && reflectutil.IsEmpty(fieldValue) {
		return nil, true, nil
	}

	valueToConvert, err := q.marshalFieldSourceValue(fieldMeta, fieldValue, now)
	if err != nil {
		return nil, false, err
	}

	av, err := q.marshalAttributeValue(fieldMeta, valueToConvert)
	if err != nil {
		return nil, false, fmt.Errorf("failed to convert field %s: %w", fieldMeta.DBName, err)
	}

	if _, isNull := av.(*types.AttributeValueMemberNULL); isNull && fieldMeta.OmitEmpty {
		return nil, true, nil
	}

	return av, false, nil
}

func (q *Query) marshalFieldSourceValue(fieldMeta *model.FieldMetadata, fieldValue reflect.Value, now time.Time) (any, error) {
	valueToConvert := fieldValue.Interface()

	switch {
	case fieldMeta.IsCreatedAt || fieldMeta.IsUpdatedAt:
		return now, nil
	case fieldMeta.IsVersion:
		if fieldValue.IsZero() {
			return int64(0), nil
		}
		return valueToConvert, nil
	case fieldMeta.IsTTL:
		return ttlUnixSecondsIfTime(fieldMeta.DBName, fieldValue, valueToConvert)
	case fieldcodec.HasJSONTag(fieldMeta.Tags):
		return fieldcodec.NormalizeJSONReflectValue(fieldMeta.Type, fieldValue)
	default:
		return valueToConvert, nil
	}
}

func ttlUnixSecondsIfTime(fieldName string, fieldValue reflect.Value, value any) (any, error) {
	if fieldValue.Type() != reflect.TypeOf(time.Time{}) || fieldValue.IsZero() {
		return value, nil
	}

	ttlTime, ok := value.(time.Time)
	if !ok {
		return nil, fmt.Errorf("expected time.Time for TTL field %s, got %T", fieldName, value)
	}
	return ttlTime.Unix(), nil
}

func (q *Query) marshalAttributeValue(fieldMeta *model.FieldMetadata, value any) (types.AttributeValue, error) {
	if fieldcodec.HasJSONTag(fieldMeta.Tags) {
		return expr.ConvertToAttributeValue(value)
	}
	if q.converter != nil {
		if fieldMeta.IsSet {
			return q.converter.ConvertToSet(value, true)
		}
		return q.converter.ToAttributeValue(value)
	}
	return expr.ConvertToAttributeValue(value)
}

func (q *Query) marshalItemTagged(item any) (map[string]types.AttributeValue, error) {
	modelValue := reflect.ValueOf(item)
	if !modelValue.IsValid() {
		return nil, fmt.Errorf("item must be a struct")
	}
	if modelValue.Kind() == reflect.Ptr {
		if modelValue.IsNil() {
			return nil, fmt.Errorf("item must be a struct")
		}
		modelValue = modelValue.Elem()
	}
	if modelValue.Kind() != reflect.Struct {
		return nil, fmt.Errorf("item must be a struct")
	}

	if q.usesFlatAnonymousEmbedEncoding() {
		return q.marshalItemTaggedFlat(modelValue)
	}

	modelType := modelValue.Type()
	out := make(map[string]types.AttributeValue)

	// Legacy tag-driven helper marshaling intentionally preserves anonymous
	// embedded struct container writes (for example `BaseObject: {...}`) rather
	// than flattening promoted fields. Focused regressions fence that shape.
	for i := 0; i < modelType.NumField(); i++ {
		field := modelType.Field(i)
		if !field.IsExported() {
			continue
		}

		tag := field.Tag.Get("theorydb")
		if tag == "-" {
			continue
		}

		fieldValue := modelValue.Field(i)
		if !fieldValue.IsValid() {
			continue
		}

		if fieldcodec.HasModifier(tag, "omitempty") && reflectutil.IsEmpty(fieldValue) {
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

func (q *Query) updateTimestampsInModel() {
	if q == nil || q.rawMetadata == nil || q.model == nil {
		return
	}

	modelValue := reflect.ValueOf(q.model)
	if modelValue.Kind() != reflect.Ptr || modelValue.IsNil() {
		return
	}
	modelValue = modelValue.Elem()
	if modelValue.Kind() != reflect.Struct {
		return
	}

	now := time.Now()

	for _, fieldMeta := range q.rawMetadata.Fields {
		if fieldMeta == nil || (!fieldMeta.IsCreatedAt && !fieldMeta.IsUpdatedAt) {
			continue
		}

		field := modelValue.FieldByIndex(fieldMeta.IndexPath)
		if field.CanSet() && field.Type() == reflect.TypeOf(time.Time{}) {
			field.Set(reflect.ValueOf(now))
		}
	}
}

// OrFilter adds an OR filter condition
