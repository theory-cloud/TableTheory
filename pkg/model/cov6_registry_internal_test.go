package model

import (
	"reflect"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/theory-cloud/tabletheory/pkg/naming"
)

func TestApplyKeyValueTag_ProjectAndDefault_COV6(t *testing.T) {
	meta := &FieldMetadata{
		IndexInfo: make(map[string]IndexRole),
		Tags:      make(map[string]string),
	}

	require.NoError(t, applyKeyValueTag(meta, "project", "all"))
	require.Equal(t, "all", meta.Tags["project"])

	require.NoError(t, applyKeyValueTag(meta, "custom", "value"))
	require.Equal(t, "value", meta.Tags["custom"])
}

func TestGetTableName_Pluralization_COV6(t *testing.T) {
	type User struct{}
	type Company struct{}
	type Bus struct{}

	require.Equal(t, "Users", getTableName(reflect.TypeOf(User{})))
	require.Equal(t, "Companies", getTableName(reflect.TypeOf(Company{})))
	require.Equal(t, "Buses", getTableName(reflect.TypeOf(Bus{})))
}

func TestSplitTags_KeepsIndexClauseTogether_COV6(t *testing.T) {
	parts := splitTags("pk,index:gsi-users,pk,sk,sparse,updated_at")
	require.Equal(t, []string{"pk", "index:gsi-users,pk,sk,sparse", "updated_at"}, parts)

	parts = splitTags("lsi:lsi-status,sk,ttl")
	require.Equal(t, []string{"lsi:lsi-status,sk", "ttl"}, parts)
}

func TestDetectNamingConvention_COV6(t *testing.T) {
	type snake struct {
		_ string `theorydb:"naming:snake_case"`
	}
	type camel struct {
		_ string `theorydb:"naming:camelCase"`
	}
	type none struct {
		Field string
	}

	require.Equal(t, naming.SnakeCase, detectNamingConvention(reflect.TypeOf(snake{})))
	require.Equal(t, naming.CamelCase, detectNamingConvention(reflect.TypeOf(camel{})))
	require.Equal(t, naming.CamelCase, detectNamingConvention(reflect.TypeOf(none{})))
}

type cov6TableNameValue struct{}

func (cov6TableNameValue) TableName() string { return "value_table" }

type cov6TableNamePointer struct{}

func (*cov6TableNamePointer) TableName() string { return "pointer_table" }

type cov6TableNameBadSignature struct{}

func (cov6TableNameBadSignature) TableName(_ int) string { return "ignored" }

type cov6TableNameBadReturn struct{}

func (cov6TableNameBadReturn) TableName() int { return 1 }

func TestResolveTableName_UsesTableNameMethodWhenValid_COV6(t *testing.T) {
	require.Equal(t, "value_table", resolveTableName(reflect.TypeOf(cov6TableNameValue{})))
	require.Equal(t, "pointer_table", resolveTableName(reflect.TypeOf(cov6TableNamePointer{})))
}

func TestResolveTableName_FallsBackToPluralizedName_COV6(t *testing.T) {
	require.Equal(t, "cov6TableNameBadSignatures", resolveTableName(reflect.TypeOf(cov6TableNameBadSignature{})))
	require.Equal(t, "cov6TableNameBadReturns", resolveTableName(reflect.TypeOf(cov6TableNameBadReturn{})))
}

func TestApplySimpleTag_RejectsUnknownTag_COV6(t *testing.T) {
	meta := &FieldMetadata{Tags: make(map[string]string)}
	require.Error(t, applySimpleTag(meta, "unknown-tag"))
}
