package query

import (
	"fmt"
	"reflect"
	"strings"

	"github.com/aws/aws-sdk-go-v2/service/dynamodb/types"

	"github.com/theory-cloud/tabletheory/internal/expr"
	"github.com/theory-cloud/tabletheory/internal/fieldcodec"
	"github.com/theory-cloud/tabletheory/internal/reflectutil"
)

type flatAnonymousEmbedEncodingLookup interface {
	FlatAnonymousEmbedEncodingEnabled() bool
}

func (q *Query) usesFlatAnonymousEmbedEncoding() bool {
	if q == nil || q.converter == nil {
		return false
	}

	lookup, ok := q.converter.(flatAnonymousEmbedEncodingLookup)
	return ok && lookup.FlatAnonymousEmbedEncodingEnabled()
}

func (q *Query) marshalTaggedFieldAttributeValue(field reflect.StructField, fieldValue reflect.Value) (types.AttributeValue, error) {
	valueToConvert, err := normalizeTaggedFieldValue(field, fieldValue)
	if err != nil {
		return nil, fmt.Errorf("failed to convert field %s: %w", field.Name, err)
	}

	tag := field.Tag.Get("theorydb")
	switch {
	case fieldcodec.HasJSONModifier(tag):
		av, err := expr.ConvertToAttributeValue(valueToConvert)
		if err != nil {
			return nil, fmt.Errorf("failed to convert field %s: %w", field.Name, err)
		}
		return av, nil
	case q != nil && q.converter != nil:
		av, err := q.converter.ToAttributeValue(valueToConvert)
		if err != nil {
			return nil, fmt.Errorf("failed to convert field %s: %w", field.Name, err)
		}
		return av, nil
	default:
		av, err := expr.ConvertToAttributeValue(valueToConvert)
		if err != nil {
			return nil, fmt.Errorf("failed to convert field %s: %w", field.Name, err)
		}
		return av, nil
	}
}

func (q *Query) findVisibleFieldByNames(modelValue reflect.Value, names ...string) (reflect.Value, reflect.StructField, bool) {
	if !modelValue.IsValid() || modelValue.Kind() != reflect.Struct {
		return reflect.Value{}, reflect.StructField{}, false
	}

	for _, name := range names {
		if name == "" {
			continue
		}
		fieldValue := modelValue.FieldByName(name)
		fieldStruct, ok := modelValue.Type().FieldByName(name)
		if fieldValue.IsValid() && ok {
			return fieldValue, fieldStruct, true
		}
	}

	fieldPlans, err := reflectutil.BuildVisibleFieldPlan(modelValue.Type(), nil)
	if err != nil {
		return reflect.Value{}, reflect.StructField{}, false
	}

	for _, fieldPlan := range fieldPlans {
		if !queryFieldMatchesNames(q, fieldPlan.Field, names...) {
			continue
		}
		return modelValue.FieldByIndex(fieldPlan.IndexPath), fieldPlan.Field, true
	}

	return reflect.Value{}, reflect.StructField{}, false
}

func queryFieldMatchesNames(q *Query, field reflect.StructField, names ...string) bool {
	if len(names) == 0 {
		return false
	}

	lookupNames := queryFieldLookupNames(q, field)
	for _, name := range names {
		if name == "" {
			continue
		}
		for _, candidate := range lookupNames {
			if name == candidate {
				return true
			}
		}
	}

	return false
}

func queryFieldLookupNames(q *Query, field reflect.StructField) []string {
	names := make([]string, 0, 6)
	names = appendQueryLookupName(names, field.Name)

	if q != nil {
		if meta := q.attributeMetadata(field.Name); meta != nil {
			names = appendQueryLookupName(names, meta.Name)
			names = appendQueryLookupName(names, meta.DynamoDBName)
		}
	}

	names = appendQueryTagLookupNames(names, field.Tag.Get("dynamodb"))
	names = appendQueryTheorydbLookupNames(names, field.Tag.Get("theorydb"))
	return names
}

func appendQueryTagLookupNames(names []string, tag string) []string {
	if tag == "" || tag == "-" {
		return names
	}

	name := strings.TrimSpace(strings.Split(tag, ",")[0])
	return appendQueryLookupName(names, name)
}

func appendQueryTheorydbLookupNames(names []string, tag string) []string {
	if tag == "" || tag == "-" {
		return names
	}

	name := strings.TrimSpace(strings.Split(tag, ",")[0])
	if name == "" || name == "-" {
		return names
	}

	names = appendQueryLookupName(names, name)
	if strings.HasPrefix(name, "attr:") {
		names = appendQueryLookupName(names, strings.TrimPrefix(name, "attr:"))
	}
	return names
}

func appendQueryLookupName(names []string, name string) []string {
	if name == "" || name == "-" {
		return names
	}

	for _, existing := range names {
		if existing == name {
			return names
		}
	}

	return append(names, name)
}

func normalizeTaggedFieldValue(field reflect.StructField, fieldValue reflect.Value) (any, error) {
	if !fieldcodec.HasJSONModifier(field.Tag.Get("theorydb")) {
		return fieldValue.Interface(), nil
	}

	return fieldcodec.NormalizeJSONReflectValue(field.Type, fieldValue)
}
