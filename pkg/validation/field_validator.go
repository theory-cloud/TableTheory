package validation

import (
	"fmt"
	"reflect"
	"regexp"
	"strings"
	"unicode"
)

// SecurityError represents a security validation error
type SecurityError struct {
	Type   string
	Field  string
	Detail string
}

func (e *SecurityError) Error() string {
	// SECURITY: Don't expose user-generated field names or content in error messages
	// Only return the error type for secure logging
	return fmt.Sprintf("security validation failed: %s", e.Type)
}

// Field validation constants
const (
	MaxFieldNameLength   = 255
	MaxOperatorLength    = 20
	MaxValueStringLength = 400000 // DynamoDB item size limit
	MaxNestedDepth       = 32
	MaxExpressionLength  = 4096
)

// SQL injection and dangerous patterns - exact matches or patterns that are clearly malicious
var dangerousPatterns = []string{
	"'", "\"", ";", "--", "/*", "*/",
	"<script", "</script", "eval(", "expression(", "import(", "require(",
}

// SQL keywords that should only be rejected if they appear as whole words or in suspicious contexts
var sqlKeywords = []string{
	"union", "select", "insert", "update", "delete", "drop", "alter", "exec", "execute",
	"script", "javascript", "vbscript",
}

// Common legitimate field name patterns that contain SQL keywords but are safe
var legitimateFieldPatterns = []regexp.Regexp{
	*regexp.MustCompile(`(?i)^(created|updated)at$`),              // CreatedAt, UpdatedAt
	*regexp.MustCompile(`(?i)^create(d|r)_?(at|time|date)$`),      // created_at, creator_time, etc.
	*regexp.MustCompile(`(?i)^update(d|r)_?(at|time|date)$`),      // updated_at, updater_time, etc.
	*regexp.MustCompile(`(?i)^delete(d|r)_?(at|time|date|flag)$`), // deleted_at, deleter_time, delete_flag, etc.
	*regexp.MustCompile(`(?i)^insert(ed|er)_?(at|time|date)$`),    // inserted_at, inserter_time, etc.
	*regexp.MustCompile(`(?i)^select(ed|or)_?(at|time|date)$`),    // selected_at, selector_time, etc.
}

var valueScriptPatterns = []string{
	"<script", "</script", "eval(", "expression(", "import(", "require(",
	"javascript:", "vbscript:", "onload=", "onerror=", "onclick=",
}

var valueSQLInjectionPatterns = []string{
	"'; drop table", "'; delete from", "'; update ", "'; insert into",
	"\"; drop table", "\"; delete from", "\"; update ", "\"; insert into",
	"' or 1=1", "\" or 1=1", "' or '1'='1", "\" or \"1\"=\"1",
	"/**/union/**/select", "concat(0x", "char(", "load_file(",
	"--", // SQL comment at end of value is suspicious
}

// Valid operator whitelist
var allowedOperators = map[string]bool{
	"=":                    true,
	"!=":                   true,
	"<>":                   true,
	"<":                    true,
	"<=":                   true,
	">":                    true,
	">=":                   true,
	"BETWEEN":              true,
	"IN":                   true,
	"BEGINS_WITH":          true,
	"CONTAINS":             true,
	"EXISTS":               true,
	"NOT_EXISTS":           true,
	"ATTRIBUTE_EXISTS":     true,
	"ATTRIBUTE_NOT_EXISTS": true,
	"EQ":                   true,
	"NE":                   true,
	"LT":                   true,
	"LE":                   true,
	"GT":                   true,
	"GE":                   true,
}

// ValidateFieldName validates a DynamoDB attribute name according to AWS rules and security best practices
func ValidateFieldName(field string) error {
	if err := validateFieldNameBasics(field); err != nil {
		return err
	}

	fieldLower := strings.ToLower(field)
	if containsAnySubstring(fieldLower, dangerousPatterns) {
		return &SecurityError{
			Type:   "InjectionAttempt",
			Field:  "",
			Detail: "field name contains dangerous pattern",
		}
	}

	if err := validateFieldNameKeywords(fieldLower, field); err != nil {
		return err
	}

	if containsControlCharacters(field) {
		return &SecurityError{
			Type:   "InvalidField",
			Field:  "",
			Detail: "field name contains control characters",
		}
	}

	if strings.Contains(field, ".") {
		return validateNestedFieldPath(field)
	}

	return validateFieldPart(field)
}

func validateFieldNameBasics(field string) error {
	if field == "" {
		return &SecurityError{
			Type:   "InvalidField",
			Field:  "",
			Detail: "field name cannot be empty",
		}
	}

	if len(field) > MaxFieldNameLength {
		return &SecurityError{
			Type:   "InvalidField",
			Field:  "",
			Detail: "field name exceeds maximum length",
		}
	}

	return nil
}

func validateFieldNameKeywords(fieldLower, field string) error {
	for _, keyword := range sqlKeywords {
		if !strings.Contains(fieldLower, keyword) {
			continue
		}
		if isLegitimateFieldName(field) {
			continue
		}
		if isStandaloneOrSuspiciousKeyword(fieldLower, keyword) {
			return &SecurityError{
				Type:   "InjectionAttempt",
				Field:  "",
				Detail: "field name contains suspicious content",
			}
		}
	}
	return nil
}

func isLegitimateFieldName(field string) bool {
	for _, pattern := range legitimateFieldPatterns {
		if pattern.MatchString(field) {
			return true
		}
	}
	return false
}

func containsControlCharacters(field string) bool {
	for _, r := range field {
		if unicode.IsControl(r) {
			return true
		}
	}
	return false
}

func validateNestedFieldPath(field string) error {
	parts := strings.Split(field, ".")
	if len(parts) > MaxNestedDepth {
		return &SecurityError{
			Type:   "InvalidField",
			Field:  "",
			Detail: "nested field depth exceeds maximum",
		}
	}

	for _, part := range parts {
		if err := validateFieldPart(part); err != nil {
			return &SecurityError{
				Type:   "InvalidField",
				Field:  "",
				Detail: "invalid field part",
			}
		}
	}

	return nil
}

// isStandaloneOrSuspiciousKeyword checks if a SQL keyword appears in a suspicious way
func isStandaloneOrSuspiciousKeyword(fieldLower, keyword string) bool {
	// Exact match (standalone keyword)
	if fieldLower == keyword {
		return true
	}

	// Check for suspicious patterns like "field; DROP" or "field UNION"
	suspiciousPatterns := []string{
		keyword + ";", ";" + keyword, keyword + " ", " " + keyword,
		keyword + ".", "." + keyword, keyword + "-", "-" + keyword,
	}

	for _, pattern := range suspiciousPatterns {
		if strings.Contains(fieldLower, pattern) {
			return true
		}
	}

	// Allow compound words where the keyword is naturally part of a field name
	// For example, "UserCreated", "PostUpdated", "OrderDeleted" are likely legitimate
	return false
}

// validateFieldPart validates a single part of a field name
func validateFieldPart(part string) error {
	if part == "" {
		return fmt.Errorf("field part cannot be empty")
	}

	// Handle DynamoDB list element access syntax: fieldName[index]
	if strings.Contains(part, "[") && strings.Contains(part, "]") {
		// Split into field name and index parts
		openBracket := strings.Index(part, "[")
		closeBracket := strings.LastIndex(part, "]")

		if closeBracket <= openBracket {
			return fmt.Errorf("invalid bracket syntax in field part")
		}

		fieldName := part[:openBracket]
		indexPart := part[openBracket+1 : closeBracket]
		remainingPart := part[closeBracket+1:]

		// Validate the field name part
		fieldPattern := regexp.MustCompile(`^[a-zA-Z_][a-zA-Z0-9_]*$`)
		if !fieldPattern.MatchString(fieldName) {
			return fmt.Errorf("field name part must start with letter or underscore and contain only alphanumeric characters and underscores")
		}

		// Validate the index part (must be a number)
		indexPattern := regexp.MustCompile(`^[0-9]+$`)
		if !indexPattern.MatchString(indexPart) {
			return fmt.Errorf("list index must be a number")
		}

		// Validate any remaining part after the bracket (should be empty for simple cases)
		if remainingPart != "" {
			return fmt.Errorf("unexpected characters after list index")
		}

		return nil
	}

	// AWS DynamoDB attribute name rules for regular field names
	validPattern := regexp.MustCompile(`^[a-zA-Z_][a-zA-Z0-9_]*$`)
	if !validPattern.MatchString(part) {
		return fmt.Errorf("field part must start with letter or underscore and contain only alphanumeric characters and underscores")
	}

	return nil
}

// ValidateOperator validates a DynamoDB condition operator
func ValidateOperator(op string) error {
	if op == "" {
		return &SecurityError{
			Type:   "InvalidOperator",
			Field:  "",
			Detail: "operator cannot be empty",
		}
	}

	if len(op) > MaxOperatorLength {
		return &SecurityError{
			Type:   "InvalidOperator",
			Field:  "",
			Detail: "operator exceeds maximum length",
		}
	}

	// Check against whitelist
	opUpper := strings.ToUpper(strings.TrimSpace(op))
	if !allowedOperators[opUpper] {
		return &SecurityError{
			Type:   "InvalidOperator",
			Field:  "",
			Detail: "operator not allowed",
		}
	}

	// Check for dangerous patterns
	opLower := strings.ToLower(op)
	for _, pattern := range dangerousPatterns {
		if strings.Contains(opLower, pattern) {
			return &SecurityError{
				Type:   "InjectionAttempt",
				Field:  "",
				Detail: "operator contains dangerous pattern",
			}
		}
	}

	return nil
}

// ValidateValue validates a value used in DynamoDB expressions
func ValidateValue(value any) error {
	if value == nil {
		return nil // NULL values are allowed
	}

	switch v := value.(type) {
	case string:
		return validateStringValue(v)
	case []any:
		return validateSliceValue(v)
	case map[string]any:
		return validateMapValue(v)
	case map[string]string:
		return validateTypedMapValue(v)
	case map[string]int:
		return validateTypedMapIntValue(v)
	default:
		if length, ok := isBuiltInScalarSlice(v); ok {
			return validateSliceLength(length)
		}
		return validateBasicValue(v)
	}
}

// validateStringValue validates string values
func validateStringValue(s string) error {
	if len(s) > MaxValueStringLength {
		return &SecurityError{
			Type:   "InvalidValue",
			Field:  "",
			Detail: "string value exceeds maximum length",
		}
	}

	stringLower := strings.ToLower(s)

	if containsAnySubstring(stringLower, valueScriptPatterns) ||
		(strings.Contains(s, "/*") && strings.Contains(s, "*/")) ||
		containsAnySubstring(stringLower, valueSQLInjectionPatterns) ||
		looksLikeUnionSelectInjection(stringLower, s) {
		return &SecurityError{
			Type:   "InjectionAttempt",
			Field:  "",
			Detail: "string value contains dangerous pattern",
		}
	}

	return nil
}

func containsAnySubstring(haystack string, needles []string) bool {
	for _, needle := range needles {
		if strings.Contains(haystack, needle) {
			return true
		}
	}
	return false
}

func isBuiltInScalarSlice(value any) (int, bool) {
	rv := reflect.ValueOf(value)
	if rv.Kind() != reflect.Slice {
		return 0, false
	}

	elem := rv.Type().Elem()
	if elem.PkgPath() != "" {
		return 0, false
	}

	switch elem.Kind() {
	case reflect.String,
		reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64,
		reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64,
		reflect.Float32, reflect.Float64,
		reflect.Bool:
		return rv.Len(), true
	default:
		return 0, false
	}
}

func looksLikeUnionSelectInjection(stringLower, raw string) bool {
	if !strings.Contains(stringLower, "union") || !strings.Contains(stringLower, "select") {
		return false
	}

	if !strings.Contains(stringLower, "union select") &&
		!strings.Contains(stringLower, "union all select") &&
		!strings.Contains(stringLower, "union/**/select") {
		return false
	}

	return strings.Contains(stringLower, "from") ||
		strings.Contains(stringLower, "*") ||
		strings.HasSuffix(raw, "--") ||
		strings.HasSuffix(raw, ";")
}

// validateSliceValue validates slice values (for IN operator, etc.)
func validateSliceValue(slice []any) error {
	if len(slice) > 100 { // DynamoDB IN operator limit
		return &SecurityError{
			Type:   "InvalidValue",
			Field:  "",
			Detail: "slice value exceeds maximum length of 100 items",
		}
	}

	for _, item := range slice {
		if err := ValidateValue(item); err != nil {
			return &SecurityError{
				Type:   "InvalidValue",
				Field:  "",
				Detail: "invalid item in collection",
			}
		}
	}

	return nil
}

// validateMapValue validates map values
func validateMapValue(m map[string]any) error {
	if len(m) > 100 { // Reasonable limit for map size
		return &SecurityError{
			Type:   "InvalidValue",
			Field:  "",
			Detail: "map value exceeds maximum keys",
		}
	}

	for key, value := range m {
		if err := ValidateFieldName(key); err != nil {
			return &SecurityError{
				Type:   "InvalidValue",
				Field:  "",
				Detail: "invalid map key",
			}
		}

		if err := ValidateValue(value); err != nil {
			return &SecurityError{
				Type:   "InvalidValue",
				Field:  "",
				Detail: "invalid map value",
			}
		}
	}

	return nil
}

// validateTypedMapValue validates typed map values
func validateTypedMapValue(m map[string]string) error {
	if len(m) > 100 { // Reasonable limit for map size
		return &SecurityError{
			Type:   "InvalidValue",
			Field:  "",
			Detail: "map value exceeds maximum keys",
		}
	}

	for key, value := range m {
		if err := ValidateFieldName(key); err != nil {
			return &SecurityError{
				Type:   "InvalidValue",
				Field:  "",
				Detail: "invalid map key",
			}
		}

		if err := ValidateValue(value); err != nil {
			return &SecurityError{
				Type:   "InvalidValue",
				Field:  "",
				Detail: "invalid map value",
			}
		}
	}

	return nil
}

// validateTypedMapIntValue validates typed map values
func validateTypedMapIntValue(m map[string]int) error {
	if len(m) > 100 { // Reasonable limit for map size
		return &SecurityError{
			Type:   "InvalidValue",
			Field:  "",
			Detail: "map value exceeds maximum keys",
		}
	}

	for key, value := range m {
		if err := ValidateFieldName(key); err != nil {
			return &SecurityError{
				Type:   "InvalidValue",
				Field:  "",
				Detail: "invalid map key",
			}
		}

		if err := ValidateValue(value); err != nil {
			return &SecurityError{
				Type:   "InvalidValue",
				Field:  "",
				Detail: "invalid map value",
			}
		}
	}

	return nil
}

// validateBasicValue validates basic types (int, float, bool)
func validateBasicValue(value any) error {
	if value == nil {
		return nil
	}

	rv := reflect.ValueOf(value)
	// Unwrap pointers/interfaces
	for rv.Kind() == reflect.Pointer || rv.Kind() == reflect.Interface {
		if rv.IsNil() {
			return nil
		}
		rv = rv.Elem()
	}

	switch rv.Kind() {
	case reflect.Func, reflect.Chan, reflect.UnsafePointer, reflect.Uintptr,
		reflect.Invalid, reflect.Complex64, reflect.Complex128:
		return &SecurityError{
			Type:   "InvalidValue",
			Field:  "",
			Detail: "unsupported value type",
		}
	case reflect.Struct:
		// Allow all structs - they will be marshaled as DynamoDB maps
		return nil
	case reflect.Slice:
		// Allow slices - they will be marshaled as DynamoDB lists
		return nil
	case reflect.Map, reflect.Array:
		// Allow maps and arrays - they will be marshaled as DynamoDB maps/lists
		return nil
	}

	return nil
}

// ValidateExpression validates a complete expression for security
func ValidateExpression(expression string) error {
	if len(expression) > MaxExpressionLength {
		return &SecurityError{
			Type:   "InvalidExpression",
			Field:  "",
			Detail: "expression exceeds maximum length",
		}
	}

	// Check for dangerous patterns
	exprLower := strings.ToLower(expression)
	for _, pattern := range dangerousPatterns {
		if strings.Contains(exprLower, pattern) {
			return &SecurityError{
				Type:   "InjectionAttempt",
				Field:  "",
				Detail: "expression contains dangerous pattern",
			}
		}
	}

	// Check for SQL injection patterns in expressions
	sqlInjectionPatterns := []string{
		"union select", "insert into", "update set", "delete from",
		"drop table", "alter table", "exec ", "execute ",
	}

	for _, pattern := range sqlInjectionPatterns {
		if strings.Contains(exprLower, pattern) {
			return &SecurityError{
				Type:   "InjectionAttempt",
				Field:  "",
				Detail: "expression contains dangerous pattern",
			}
		}
	}

	return nil
}

// ValidateTableName validates a DynamoDB table name
func ValidateTableName(name string) error {
	if len(name) < 3 || len(name) > 255 {
		return &SecurityError{
			Type:   "InvalidTableName",
			Field:  "",
			Detail: "table name length invalid",
		}
	}

	// AWS table name pattern
	pattern := regexp.MustCompile(`^[a-zA-Z0-9_.-]+$`)
	if !pattern.MatchString(name) {
		return &SecurityError{
			Type:   "InvalidTableName",
			Field:  "",
			Detail: "table name contains invalid characters",
		}
	}

	// Check for dangerous patterns
	nameLower := strings.ToLower(name)
	for _, dangerousPattern := range dangerousPatterns {
		if strings.Contains(nameLower, dangerousPattern) {
			return &SecurityError{
				Type:   "InjectionAttempt",
				Field:  "",
				Detail: "table name contains dangerous pattern",
			}
		}
	}

	return nil
}

// ValidateIndexName validates a DynamoDB index name
func ValidateIndexName(name string) error {
	if name == "" {
		return nil // Empty index name is allowed (means no index)
	}

	if len(name) < 3 || len(name) > 255 {
		return &SecurityError{
			Type:   "InvalidIndexName",
			Field:  "",
			Detail: "index name length invalid",
		}
	}

	// AWS index name pattern (similar to table name)
	pattern := regexp.MustCompile(`^[a-zA-Z0-9_.-]+$`)
	if !pattern.MatchString(name) {
		return &SecurityError{
			Type:   "InvalidIndexName",
			Field:  "",
			Detail: "index name contains invalid characters",
		}
	}

	return nil
}

// validateSliceLength validates slice lengths for typed slices
func validateSliceLength(length int) error {
	if length > 100 { // DynamoDB IN operator limit
		return &SecurityError{
			Type:   "InvalidValue",
			Field:  "",
			Detail: "slice value exceeds maximum length",
		}
	}

	return nil
}
