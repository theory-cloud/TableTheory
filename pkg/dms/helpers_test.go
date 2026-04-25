package dms

import (
	"math"
	"reflect"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/theory-cloud/tabletheory/pkg/model"
	"github.com/theory-cloud/tabletheory/pkg/naming"
)

func TestNormalizeJSONCompatible_CoversAdditionalBranches(t *testing.T) {
	got, err := normalizeJSONCompatible([]any{
		map[string]any{
			"count": uint64(2),
			"ratio": 1.5,
		},
	}, "dms.items")
	require.NoError(t, err)
	require.Equal(t, []any{map[string]any{"count": uint64(2), "ratio": 1.5}}, got)

	_, err = normalizeJSONCompatible(math.NaN(), "dms.nan")
	require.ErrorContains(t, err, "non-finite number")

	_, err = normalizeJSONCompatible(math.Inf(1), "dms.inf")
	require.ErrorContains(t, err, "non-finite number")

	_, err = normalizeJSONCompatible(struct{}{}, "dms.struct")
	require.ErrorContains(t, err, "non-JSON value")
}

func TestNormalizeForCompare_SortsDeterministically(t *testing.T) {
	modelValue := Model{
		Name:  "Demo",
		Table: Table{Name: "tbl"},
		Keys: Keys{
			Partition: KeyAttribute{Attribute: "PK", Type: "S"},
		},
		WritePolicy: WritePolicy{
			ProtectedAttributes: []string{"zeta", "alpha"},
		},
		Attributes: []Attribute{
			{
				Attribute: "zeta",
				Type:      "M",
				Roles:     []string{"ttl", "pk"},
				Optional:  true,
				JSON:      true,
			},
			{
				Attribute:  "alpha",
				Type:       "S",
				Roles:      []string{"updated_at", "created_at"},
				Required:   true,
				Encryption: map[string]any{"v": 1},
			},
		},
		Indexes: []Index{
			{
				Name:      "b-index",
				Type:      "GSI",
				Partition: KeyAttribute{Attribute: "gsi_pk", Type: "S"},
				Projection: Projection{
					Type:   "INCLUDE",
					Fields: []string{"two", "one"},
				},
			},
			{
				Name:      "a-index",
				Type:      "GSI",
				Partition: KeyAttribute{Attribute: "gsi2_pk", Type: "S"},
			},
		},
	}

	normalized := normalizeForCompare(modelValue, CompareOptions{})
	require.Equal(t, "camelCase", normalized.Naming)
	require.Equal(t, "tbl", normalized.TableName)
	require.Equal(t, "mutable", normalized.WritePolicy.Mode)
	require.Equal(t, []string{"alpha", "zeta"}, normalized.WritePolicy.ProtectedAttributes)
	require.Equal(t, "alpha", normalized.Attributes[0].Attribute)
	require.Equal(t, []string{"created_at", "updated_at"}, normalized.Attributes[0].Roles)
	require.True(t, normalized.Attributes[0].Encrypted)
	require.Equal(t, "zeta", normalized.Attributes[1].Attribute)
	require.Equal(t, []string{"pk", "ttl"}, normalized.Attributes[1].Roles)
	require.True(t, normalized.Attributes[1].JSON)
	require.Equal(t, "a-index", normalized.Indexes[0].Name)
	require.Equal(t, []string{"one", "two"}, normalized.Indexes[1].Projection.Fields)

	ignoredTable := normalizeForCompare(modelValue, CompareOptions{IgnoreTableName: true})
	require.Empty(t, ignoredTable.TableName)
}

func TestDMSMetadataHelpers_CoverJSONAndValidationBranches(t *testing.T) {
	field := &model.FieldMetadata{
		IsPK:        true,
		IsCreatedAt: true,
		IsTTL:       true,
	}
	require.Equal(t, []string{"created_at", "pk", "ttl"}, rolesFromField(field))

	require.Equal(t, "snake_case", namingConventionString(naming.SnakeCase))
	require.Equal(t, "pascalCase", namingConventionString(naming.PascalCase))
	require.Equal(t, "dynamorm", namingConventionString(naming.DynamORM))
	require.Equal(t, "camelCase", namingConventionString(naming.CamelCase))

	require.Equal(t, "B", scalarKeyTypeFromField(reflect.TypeOf([]byte{})))
	require.Equal(t, "S", scalarKeyTypeFromField(reflect.TypeOf((chan int)(nil))))

	require.Equal(t, "S", attributeTypeFromJSONField(nil))
	require.Equal(t, "S", attributeTypeFromJSONField(reflect.TypeOf((*any)(nil)).Elem()))
	require.Equal(t, "S", attributeTypeFromJSONField(reflect.TypeOf([]byte{})))
	require.Equal(t, "BOOL", attributeTypeFromJSONField(reflect.TypeOf(true)))

	require.Equal(t, reflect.TypeOf(""), derefType(reflect.TypeOf((**string)(nil))))
	require.True(t, isSupportedJSONAttributeType("M"))
	require.False(t, isSupportedJSONAttributeType("SS"))

	_, err := validateModelAttributes(Model{
		Name: "Demo",
		Attributes: []Attribute{
			{Attribute: "blob", Type: "S", Binary: true},
		},
	})
	require.ErrorContains(t, err, "binary requires type B")

	_, err = validateModelAttributes(Model{
		Name: "Demo",
		Attributes: []Attribute{
			{Attribute: "payload", Type: "B", JSON: true, Binary: true},
		},
	})
	require.ErrorContains(t, err, "cannot be both json and binary")

	_, err = validateModelAttributes(Model{
		Name: "Demo",
		Attributes: []Attribute{
			{Attribute: "payload", Type: "SS", JSON: true},
		},
	})
	require.ErrorContains(t, err, "json fields must use S/N/BOOL/NULL/L/M")
}
