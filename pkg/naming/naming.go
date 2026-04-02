package naming

import (
	"fmt"
	"reflect"
	"regexp"
	"strings"
	"unicode"
)

// Convention represents the naming convention for DynamoDB attribute names.
type Convention int

const (
	// CamelCase convention: "firstName", "createdAt", with special handling for "PK" and "SK"
	CamelCase Convention = 0
	// SnakeCase convention: "first_name", "created_at"
	SnakeCase Convention = 1
	// PascalCase convention: "FirstName", "CreatedAt", "ID", "GSI1PK"
	// This is primarily intended for legacy DynamoDB schemas that use PascalCase and acronyms.
	PascalCase Convention = 2
	// DynamORM convention preserves legacy DynamORM semantics:
	// PK/SK are always uppercase, while all other fields use camelCase.
	DynamORM Convention = 3
)

var camelCasePattern = regexp.MustCompile(`^[a-z][A-Za-z0-9]*$`)
var snakeCasePattern = regexp.MustCompile(`^[a-z][a-z0-9]*(_[a-z0-9]+)*$`)
var pascalCasePattern = regexp.MustCompile(`^[A-Z][A-Za-z0-9]*$`)

// ResolveAttrName determines the DynamoDB attribute name for a field using CamelCase convention.
// It returns the attribute name and a bool indicating whether the field should be skipped.
func ResolveAttrName(field reflect.StructField) (string, bool) {
	return ResolveAttrNameWithConvention(field, CamelCase)
}

// ResolveAttrNameWithConvention determines the DynamoDB attribute name for a field using the specified convention.
// It returns the attribute name and a bool indicating whether the field should be skipped.
func ResolveAttrNameWithConvention(field reflect.StructField, convention Convention) (string, bool) {
	tag := field.Tag.Get("theorydb")
	if tag == "-" {
		return "", true
	}

	if attr := attrFromTag(tag); attr != "" {
		return attr, false
	}

	if convention == DynamORM {
		if hasStandaloneTagPart(tag, "pk") {
			return "PK", false
		}
		if hasStandaloneTagPart(tag, "sk") {
			return "SK", false
		}
		return DefaultAttrName(field.Name), false
	}

	return ConvertAttrName(field.Name, convention), false
}

// DefaultAttrName converts a Go struct field name to the preferred camelCase DynamoDB attribute name.
func DefaultAttrName(name string) string {
	if name == "" {
		return ""
	}

	if name == "PK" || name == "SK" {
		return name
	}

	runes := []rune(name)
	if len(runes) == 1 {
		return strings.ToLower(name)
	}

	boundary := 1
	for boundary < len(runes) {
		if !unicode.IsUpper(runes[boundary]) {
			break
		}

		if boundary+1 < len(runes) && !unicode.IsUpper(runes[boundary+1]) {
			break
		}

		boundary++
	}

	prefix := strings.ToLower(string(runes[:boundary]))
	return prefix + string(runes[boundary:])
}

// ToSnakeCase converts a Go struct field name to snake_case DynamoDB attribute name.
// It uses smart acronym handling: "URLValue" → "url_value", "ID" → "id", "UserID" → "user_id".
func ToSnakeCase(name string) string {
	if name == "" {
		return ""
	}

	runes := []rune(name)
	if len(runes) == 1 {
		return strings.ToLower(name)
	}

	var b strings.Builder
	b.Grow(len(runes) + len(runes)/2)

	for i, ch := range runes {
		if unicode.IsUpper(ch) {
			if i > 0 {
				prev := runes[i-1]
				nextIsLower := i+1 < len(runes) && unicode.IsLower(runes[i+1])
				if !unicode.IsDigit(prev) && (unicode.IsLower(prev) || (unicode.IsUpper(prev) && nextIsLower)) {
					b.WriteByte('_')
				}
			}
			b.WriteRune(unicode.ToLower(ch))
			continue
		}
		b.WriteRune(unicode.ToLower(ch))
	}

	return b.String()
}

// ConvertAttrName converts a field name to the appropriate naming convention.
func ConvertAttrName(name string, convention Convention) string {
	switch convention {
	case SnakeCase:
		return ToSnakeCase(name)
	case PascalCase:
		return name
	case DynamORM:
		return DefaultAttrName(name)
	case CamelCase:
		fallthrough
	default:
		return DefaultAttrName(name)
	}
}

// ValidateAttrName enforces the naming convention for DynamoDB attribute names.
// For CamelCase: allows "PK" and "SK" as exceptions, otherwise enforces camelCase pattern.
// For SnakeCase: enforces snake_case pattern (no special exceptions).
func ValidateAttrName(name string, convention Convention) error {
	if name == "" {
		return fmt.Errorf("attribute name cannot be empty")
	}

	switch convention {
	case SnakeCase:
		if !snakeCasePattern.MatchString(name) {
			return fmt.Errorf("attribute name must be snake_case (got %q)", name)
		}
		return nil
	case PascalCase:
		if !pascalCasePattern.MatchString(name) {
			return fmt.Errorf("attribute name must be pascalCase (got %q)", name)
		}
		return nil
	case DynamORM:
		fallthrough
	case CamelCase:
		fallthrough
	default:
		// CamelCase validation with PK/SK exceptions
		if name == "PK" || name == "SK" {
			return nil
		}
		if !camelCasePattern.MatchString(name) {
			return fmt.Errorf("attribute name must be camelCase (got %q)", name)
		}
		return nil
	}
}

func attrFromTag(tag string) string {
	if tag == "" {
		return ""
	}

	parts := strings.Split(tag, ",")
	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		if strings.HasPrefix(part, "attr:") {
			return strings.TrimPrefix(part, "attr:")
		}
	}
	return ""
}

func hasStandaloneTagPart(tag string, want string) bool {
	if tag == "" || want == "" {
		return false
	}

	parts := splitTheorydbTag(tag)
	for _, part := range parts {
		if part == want {
			return true
		}
	}

	return false
}

func splitTheorydbTag(tag string) []string {
	if tag == "" {
		return nil
	}

	rawParts := strings.Split(tag, ",")
	parts := make([]string, 0, len(rawParts))
	var current strings.Builder
	inIndexClause := false

	flushCurrent := func() {
		if current.Len() == 0 {
			return
		}
		parts = append(parts, current.String())
		current.Reset()
		inIndexClause = false
	}

	for _, raw := range rawParts {
		part := strings.TrimSpace(raw)
		if part == "" {
			continue
		}

		if inIndexClause {
			if isIndexModifier(part) {
				current.WriteString(",")
				current.WriteString(part)
				continue
			}
			flushCurrent()
		}

		if strings.HasPrefix(part, "index:") || strings.HasPrefix(part, "lsi:") {
			inIndexClause = true
			current.WriteString(part)
			continue
		}

		parts = append(parts, part)
	}

	flushCurrent()
	return parts
}

func isIndexModifier(token string) bool {
	switch token {
	case "pk", "sk", "sparse":
		return true
	default:
		return false
	}
}
