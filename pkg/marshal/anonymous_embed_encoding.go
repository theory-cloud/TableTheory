package marshal

import (
	"reflect"

	"github.com/aws/aws-sdk-go-v2/service/dynamodb/types"

	pkgTypes "github.com/theory-cloud/tabletheory/pkg/types"
)

func converterRequestsFlatAnonymousEmbeds(converter *pkgTypes.Converter) bool {
	return converter != nil && converter.FlatAnonymousEmbedEncodingEnabled()
}

func setMarshaledAttributeValue(
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

	return setLegacyNestedAttributeValue(root, containerNames, fieldName, av)
}

func marshalContainerNamesForField(
	modelType reflect.Type,
	indexPath []int,
	resolve func(reflect.StructField) (string, bool),
	flatten bool,
) ([]string, bool) {
	if flatten {
		return nil, false
	}

	return resolveMarshalContainerNames(modelType, indexPath, resolve)
}
