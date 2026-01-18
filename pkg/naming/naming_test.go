package naming

import (
	"reflect"
	"testing"
)

type sample struct {
	Simple         string
	URLValue       string
	ID             string
	CustomAttr     string `theorydb:"attr:customName"`
	Skip           string `theorydb:"-"`
	PK             string `theorydb:"pk"`
	SK             string `theorydb:"sk"`
	ExplicitPK     string `theorydb:"pk,attr:PK"`
	ExplicitCustom string `theorydb:"attr:camelCase"`
}

func TestDefaultAttrName(t *testing.T) {
	tests := map[string]string{
		"Name":      "name",
		"CreatedAt": "createdAt",
		"URLValue":  "urlValue",
		"ID":        "id",
		"UUID":      "uuid",
		"HTTPCode":  "httpCode",
		"PK":        "PK",
		"SK":        "SK",
	}

	for input, expected := range tests {
		if got := DefaultAttrName(input); got != expected {
			t.Errorf("DefaultAttrName(%q) = %q, want %q", input, got, expected)
		}
	}
}

func TestValidateAttrName(t *testing.T) {
	t.Run("CamelCase", func(t *testing.T) {
		valid := []string{"name", "createdAt", "value1", "PK", "SK"}
		for _, v := range valid {
			if err := ValidateAttrName(v, CamelCase); err != nil {
				t.Errorf("ValidateAttrName(%q, CamelCase) unexpected error: %v", v, err)
			}
		}

		invalid := []string{"", "snake_case", "CamelCase", "hyphen-name"}
		for _, v := range invalid {
			if err := ValidateAttrName(v, CamelCase); err == nil {
				t.Errorf("ValidateAttrName(%q, CamelCase) expected error", v)
			}
		}
	})

	t.Run("SnakeCase", func(t *testing.T) {
		valid := []string{"name", "created_at", "value_1", "user_id", "url_value"}
		for _, v := range valid {
			if err := ValidateAttrName(v, SnakeCase); err != nil {
				t.Errorf("ValidateAttrName(%q, SnakeCase) unexpected error: %v", v, err)
			}
		}

		invalid := []string{"", "CamelCase", "camelCase", "PK", "SK", "hyphen-name", "_leading", "trailing_"}
		for _, v := range invalid {
			if err := ValidateAttrName(v, SnakeCase); err == nil {
				t.Errorf("ValidateAttrName(%q, SnakeCase) expected error", v)
			}
		}
	})
}

func TestResolveAttrName(t *testing.T) {
	typ := reflect.TypeOf(sample{})

	field := typ.Field(0)
	name, skip := ResolveAttrName(field)
	if skip || name != "simple" {
		t.Fatalf("expected simple, got %q skip=%v", name, skip)
	}

	field = typ.Field(1)
	name, skip = ResolveAttrName(field)
	if skip || name != "urlValue" {
		t.Fatalf("expected urlValue, got %q", name)
	}

	field = typ.Field(3)
	name, skip = ResolveAttrName(field)
	if skip || name != "customName" {
		t.Fatalf("expected customName, got %q", name)
	}

	field = typ.Field(4)
	if _, skip = ResolveAttrName(field); !skip {
		t.Fatalf("expected skip for field with theorydb:\"-\"")
	}

	field = typ.Field(6)
	name, skip = ResolveAttrName(field)
	if skip || name != "SK" {
		t.Fatalf("expected SK, got %q", name)
	}

	field = typ.Field(7)
	name, skip = ResolveAttrName(field)
	if skip || name != "PK" {
		t.Fatalf("expected PK, got %q", name)
	}
}

func TestToSnakeCase(t *testing.T) {
	tests := map[string]string{
		// Basic cases
		"Name":      "name",
		"CreatedAt": "created_at",
		"UpdatedAt": "updated_at",
		"FirstName": "first_name",
		"LastName":  "last_name",

		// Acronyms and special cases
		"ID":        "id",
		"UserID":    "user_id",
		"UUID":      "uuid",
		"URLValue":  "url_value",
		"HTTPCode":  "http_code",
		"HTTPSPort": "https_port",
		"APIKey":    "api_key",

		// Numbers
		"Value1":  "value1",
		"Field2A": "field2a",

		// Single character
		"X": "x",
		"A": "a",

		// Already lowercase
		"lowercase": "lowercase",

		// Edge cases
		"PK":        "pk",
		"SK":        "sk",
		"AccountID": "account_id",
		"DeletedAt": "deleted_at",
	}

	for input, expected := range tests {
		if got := ToSnakeCase(input); got != expected {
			t.Errorf("ToSnakeCase(%q) = %q, want %q", input, got, expected)
		}
	}
}

func TestConvertAttrName(t *testing.T) {
	testCases := []struct {
		input      string
		expected   string
		convention Convention
	}{
		// CamelCase convention
		{"Name", "name", CamelCase},
		{"CreatedAt", "createdAt", CamelCase},
		{"URLValue", "urlValue", CamelCase},
		{"ID", "id", CamelCase},
		{"PK", "PK", CamelCase},
		{"SK", "SK", CamelCase},

		// SnakeCase convention
		{"Name", "name", SnakeCase},
		{"CreatedAt", "created_at", SnakeCase},
		{"URLValue", "url_value", SnakeCase},
		{"ID", "id", SnakeCase},
		{"PK", "pk", SnakeCase},
		{"SK", "sk", SnakeCase},
		{"UserID", "user_id", SnakeCase},
		{"FirstName", "first_name", SnakeCase},
	}

	for _, tc := range testCases {
		got := ConvertAttrName(tc.input, tc.convention)
		if got != tc.expected {
			t.Errorf("ConvertAttrName(%q, %v) = %q, want %q", tc.input, tc.convention, got, tc.expected)
		}
	}
}
