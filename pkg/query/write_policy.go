package query

import (
	"fmt"
	"reflect"
	"strings"

	"github.com/theory-cloud/tabletheory/v3/pkg/core"
	theorydbErrors "github.com/theory-cloud/tabletheory/v3/pkg/errors"
	"github.com/theory-cloud/tabletheory/v3/pkg/model"
)

func (q *Query) writePolicy() model.WritePolicy {
	if q == nil {
		return model.DefaultWritePolicy()
	}
	if q.rawMetadata != nil {
		return q.rawMetadata.WritePolicy
	}
	if provider, ok := q.metadata.(core.WritePolicyProvider); ok && provider != nil {
		return provider.WritePolicy()
	}
	return model.DefaultWritePolicy()
}

func (q *Query) rejectWriteOnceMutation(operation string) error {
	if q.writePolicy().Mode != model.WritePolicyModeWriteOnce {
		return nil
	}
	if operation == "" {
		operation = "mutation"
	}
	return fmt.Errorf("%w: %s", theorydbErrors.ErrImmutableModelMutation, operation)
}

func (q *Query) rejectProtectedOverwrite(operation string) error {
	if len(q.writePolicy().ProtectedAttributes) == 0 {
		return nil
	}
	if operation == "" {
		operation = "overwrite"
	}
	return fmt.Errorf("%w: %s", theorydbErrors.ErrProtectedFieldMutation, operation)
}

func (q *Query) rejectProtectedFieldMutation(fields []string, extraProtected []string) error {
	protected := q.protectedAttributeSet(extraProtected)
	if len(protected) == 0 {
		return nil
	}

	for _, field := range fields {
		attrName := q.canonicalAttributeName(field)
		root := rootAttributeName(attrName)
		if _, ok := protected[root]; ok {
			return fmt.Errorf("%w: %s", theorydbErrors.ErrProtectedFieldMutation, root)
		}
	}
	return nil
}

func (q *Query) updatePolicyFields(modelValue reflect.Value, fields []string) ([]string, error) {
	if len(fields) > 0 {
		return appendPolicyAutoFields(append([]string(nil), fields...), q.rawMetadata), nil
	}
	if q.rawMetadata == nil {
		return nil, nil
	}
	return appendPolicyAutoFields(q.metadataFieldsToUpdate(modelValue), q.rawMetadata), nil
}

func appendPolicyAutoFields(fields []string, metadata *model.Metadata) []string {
	if metadata == nil {
		return fields
	}
	if metadata.UpdatedAtField != nil {
		fields = append(fields, metadata.UpdatedAtField.DBName)
	}
	if metadata.VersionField != nil {
		fields = append(fields, metadata.VersionField.DBName)
	}
	return fields
}

func (q *Query) protectedAttributeSet(extra []string) map[string]struct{} {
	policy := q.writePolicy()
	if len(policy.ProtectedAttributes) == 0 && len(extra) == 0 {
		return nil
	}

	protected := make(map[string]struct{}, len(policy.ProtectedAttributes)+len(extra))
	for _, attr := range policy.ProtectedAttributes {
		addProtectedAttribute(protected, q.canonicalAttributeName(attr))
	}
	for _, attr := range extra {
		addProtectedAttribute(protected, q.canonicalAttributeName(attr))
	}
	return protected
}

func addProtectedAttribute(protected map[string]struct{}, attr string) {
	attr = rootAttributeName(attr)
	if attr == "" {
		return
	}
	protected[attr] = struct{}{}
}

func (q *Query) canonicalAttributeName(field string) string {
	field = strings.TrimSpace(field)
	if field == "" || q == nil || q.metadata == nil {
		return field
	}
	root := rootAttributeName(field)
	if root == "" {
		return field
	}
	if meta := q.metadata.AttributeMetadata(root); meta != nil {
		if meta.DynamoDBName != "" {
			return replaceRootAttribute(field, meta.DynamoDBName)
		}
		if meta.Name != "" {
			return replaceRootAttribute(field, meta.Name)
		}
	}
	return field
}

func rootAttributeName(field string) string {
	field = strings.TrimSpace(field)
	if field == "" {
		return ""
	}
	root := field
	if idx := strings.IndexAny(root, ".["); idx >= 0 {
		root = root[:idx]
	}
	return root
}

func replaceRootAttribute(field, replacement string) string {
	root := rootAttributeName(field)
	if root == "" || replacement == "" || root == replacement {
		return field
	}
	return replacement + strings.TrimPrefix(field, root)
}
