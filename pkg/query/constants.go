package query

// Operator identifies a DynamoDB condition/filter operator accepted by TableTheory query builders.
type Operator string

const (
	operationQuery = "Query"
	operationScan  = "Scan"
)

const (
	// OpEqual compares equality.
	OpEqual Operator = "="
	// OpNotEqual compares inequality.
	OpNotEqual Operator = "<>"
	// OpLessThan compares values lower than the operand.
	OpLessThan Operator = "<"
	// OpLessThanOrEqual compares values lower than or equal to the operand.
	OpLessThanOrEqual Operator = "<="
	// OpGreaterThan compares values greater than the operand.
	OpGreaterThan Operator = ">"
	// OpGreaterThanOrEqual compares values greater than or equal to the operand.
	OpGreaterThanOrEqual Operator = ">="
	// OpBetween compares a value against an inclusive lower/upper bound pair.
	OpBetween Operator = "BETWEEN"
	// OpIn compares membership in a bounded value list.
	OpIn Operator = "IN"
	// OpBeginsWith matches string/binary sort-key or filter prefixes.
	OpBeginsWith Operator = "BEGINS_WITH"
	// OpContains matches collection or substring containment.
	OpContains Operator = "CONTAINS"
	// OpExists checks that an attribute exists.
	OpExists Operator = "ATTRIBUTE_EXISTS"
	// OpNotExists checks that an attribute does not exist.
	OpNotExists Operator = "ATTRIBUTE_NOT_EXISTS"
)

const (
	// OpEQ is an alias for OpEqual.
	OpEQ Operator = "EQ"
	// OpNE is an alias for OpNotEqual.
	OpNE Operator = "NE"
	// OpLT is an alias for OpLessThan.
	OpLT Operator = "LT"
	// OpLE is an alias for OpLessThanOrEqual.
	OpLE Operator = "LE"
	// OpGT is an alias for OpGreaterThan.
	OpGT Operator = "GT"
	// OpGE is an alias for OpGreaterThanOrEqual.
	OpGE Operator = "GE"
)

// Between returns the two-value operand shape expected by DynamoDB BETWEEN expressions.
func Between(lo, hi any) []any {
	return []any{lo, hi}
}
