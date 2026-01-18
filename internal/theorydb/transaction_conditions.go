package theorydb

import "github.com/theory-cloud/tabletheory/pkg/core"

// Condition creates a simple field comparison condition for transactional writes.
func Condition(field, operator string, value any) core.TransactCondition {
	return core.TransactCondition{
		Kind:     core.TransactConditionKindField,
		Field:    field,
		Operator: operator,
		Value:    value,
	}
}

// ConditionExpression registers a raw condition expression for transactional writes.
func ConditionExpression(expression string, values map[string]any) core.TransactCondition {
	return core.TransactCondition{
		Kind:       core.TransactConditionKindExpression,
		Expression: expression,
		Values:     cloneConditionValues(values),
	}
}

// IfNotExists guards a write with attribute_not_exists on the item's primary key.
func IfNotExists() core.TransactCondition {
	return core.TransactCondition{Kind: core.TransactConditionKindPrimaryKeyNotExists}
}

// IfExists guards a write with attribute_exists on the item's primary key.
func IfExists() core.TransactCondition {
	return core.TransactCondition{Kind: core.TransactConditionKindPrimaryKeyExists}
}

// AtVersion enforces an optimistic locking check on the model's version field.
func AtVersion(version int64) core.TransactCondition {
	return core.TransactCondition{
		Kind:  core.TransactConditionKindVersionEquals,
		Value: version,
	}
}

// ConditionVersion is an alias for AtVersion for API parity with UpdateBuilder helpers.
func ConditionVersion(version int64) core.TransactCondition {
	return AtVersion(version)
}

func cloneConditionValues(values map[string]any) map[string]any {
	if len(values) == 0 {
		return nil
	}
	cloned := make(map[string]any, len(values))
	for k, v := range values {
		cloned[k] = v
	}
	return cloned
}
