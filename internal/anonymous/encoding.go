package anonymous

import (
	"fmt"
	"reflect"

	"github.com/aws/aws-sdk-go-v2/service/dynamodb/types"
)

// FlatEncodingLookup reports whether a converter or option carrier requests flat
// promoted-field encoding for exported anonymous embedded structs.
type FlatEncodingLookup interface {
	FlatAnonymousEmbedEncodingEnabled() bool
}

// RequestsFlatEncoding returns true when the supplied value opts into flat
// anonymous-embed encoding.
func RequestsFlatEncoding(value any) bool {
	if value == nil {
		return false
	}
	lookup, ok := value.(FlatEncodingLookup)
	return ok && lookup.FlatAnonymousEmbedEncodingEnabled()
}

// MarshalContainerNamesForField resolves legacy anonymous-container names for a
// promoted field. Every ancestor container is examined for an exclusion tag even
// when flat encoding is enabled: flat encoding writes the promoted field
// directly into the root map, so a skipped container must suppress the field
// rather than merely omitting the nested container names.
func MarshalContainerNamesForField(
	modelType reflect.Type,
	indexPath []int,
	resolve func(reflect.StructField) (string, bool),
	flatten bool,
) ([]string, bool) {
	if len(indexPath) <= 1 {
		return nil, false
	}

	names := make([]string, 0, len(indexPath)-1)
	for depth := 1; depth < len(indexPath); depth++ {
		field := modelType.FieldByIndex(indexPath[:depth])
		name, skip := resolve(field)
		if skip {
			return nil, true
		}
		if !flatten {
			names = append(names, name)
		}
	}

	if flatten {
		return nil, false
	}
	return names, false
}

// SetMarshaledAttributeValue writes a field AttributeValue either directly into
// the root map for flat encoding or into the nested legacy anonymous-container
// path for compatibility encoding.
func SetMarshaledAttributeValue(
	root map[string]types.AttributeValue,
	containerNames []string,
	fieldName string,
	av types.AttributeValue,
	flatten bool,
) error {
	if flatten {
		root[fieldName] = av
		return nil
	}

	current := root
	for _, containerName := range containerNames {
		existing, ok := current[containerName]
		if !ok {
			child := make(map[string]types.AttributeValue)
			current[containerName] = &types.AttributeValueMemberM{Value: child}
			current = child
			continue
		}

		nested, ok := existing.(*types.AttributeValueMemberM)
		if !ok {
			return fmt.Errorf("legacy anonymous container %s must marshal as map, got %T", containerName, existing)
		}
		current = nested.Value
	}

	current[fieldName] = av
	return nil
}
