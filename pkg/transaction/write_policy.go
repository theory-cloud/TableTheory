package transaction

import (
	"fmt"
	"reflect"

	"github.com/theory-cloud/tabletheory/internal/reflectutil"
	theorydbErrors "github.com/theory-cloud/tabletheory/pkg/errors"
	"github.com/theory-cloud/tabletheory/pkg/model"
)

func rejectWriteOnceMutation(metadata *model.Metadata, operation string) error {
	if metadata == nil || metadata.WritePolicy.Mode != model.WritePolicyModeWriteOnce {
		return nil
	}
	if operation == "" {
		operation = "mutation"
	}
	return fmt.Errorf("%w: %s", theorydbErrors.ErrImmutableModelMutation, operation)
}

func rejectProtectedFieldMutation(metadata *model.Metadata, fields []string) error {
	protected := protectedAttributeSet(metadata)
	if len(protected) == 0 {
		return nil
	}

	for _, field := range fields {
		attrName := resolveMetadataAttributeName(metadata, field)
		if _, ok := protected[attrName]; ok {
			return fmt.Errorf("%w: %s", theorydbErrors.ErrProtectedFieldMutation, attrName)
		}
	}
	return nil
}

func protectedAttributeSet(metadata *model.Metadata) map[string]struct{} {
	if metadata == nil || len(metadata.WritePolicy.ProtectedAttributes) == 0 {
		return nil
	}
	protected := make(map[string]struct{}, len(metadata.WritePolicy.ProtectedAttributes))
	for _, attr := range metadata.WritePolicy.ProtectedAttributes {
		if attr == "" {
			continue
		}
		protected[resolveMetadataAttributeName(metadata, attr)] = struct{}{}
	}
	return protected
}

func resolveMetadataAttributeName(metadata *model.Metadata, field string) string {
	if metadata == nil || field == "" {
		return field
	}
	if meta := metadata.Fields[field]; meta != nil && meta.DBName != "" {
		return meta.DBName
	}
	if meta := metadata.FieldsByDBName[field]; meta != nil && meta.DBName != "" {
		return meta.DBName
	}
	return field
}

func fieldsMutatedByTransactionUpdate(modelValue reflect.Value, metadata *model.Metadata) []string {
	if metadata == nil {
		return nil
	}
	fields := make([]string, 0, len(metadata.Fields))
	for _, fieldMeta := range metadata.Fields {
		if fieldMeta == nil || fieldMeta.IsPK || fieldMeta.IsSK {
			continue
		}

		fieldValue := modelValue.FieldByIndex(fieldMeta.IndexPath)
		if !fieldValue.IsValid() || (fieldMeta.OmitEmpty && reflectutil.IsEmpty(fieldValue)) {
			continue
		}
		fields = append(fields, fieldMeta.DBName)
	}
	return fields
}
