package model_test

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	theorydbErrors "github.com/theory-cloud/tabletheory/v3/pkg/errors"
	"github.com/theory-cloud/tabletheory/v3/pkg/model"
	"github.com/theory-cloud/tabletheory/v3/pkg/naming"
)

// Test models with various struct tag configurations

type BasicModel struct {
	ID   string `theorydb:"pk"`
	Name string
}

type CompositeKeyModel struct {
	UserID    string    `theorydb:"pk"`
	Timestamp time.Time `theorydb:"sk"`
	Data      string
}

type IndexedModel struct {
	ID       string  `theorydb:"pk"`
	Email    string  `theorydb:"index:gsi-email"`
	Category string  `theorydb:"index:gsi-category-price,pk"`
	Status   string  `theorydb:"lsi:lsi-status"`
	Price    float64 `theorydb:"index:gsi-category-price,sk"`
}

type SpecialFieldsModel struct {
	CreatedAt time.Time `theorydb:"created_at"`
	UpdatedAt time.Time `theorydb:"updated_at"`
	ID        string    `theorydb:"pk"`
	Version   int       `theorydb:"version"`
	TTL       int64     `theorydb:"ttl"`
}

type CustomAttributeModel struct {
	ID       string   `theorydb:"pk,attr:userId"`
	UserName string   `theorydb:"attr:username"`
	Optional string   `theorydb:"omitempty"`
	Tags     []string `theorydb:"set"`
}

type InvalidModel struct {
	Name string // No primary key
}

type ImplicitTimestampModel struct {
	CreatedAt time.Time
	UpdatedAt time.Time
	ID        string `theorydb:"pk"`
}

func TestNewRegistry(t *testing.T) {
	registry := model.NewRegistry()
	assert.NotNil(t, registry)
}

func TestRegisterBasicModel(t *testing.T) {
	registry := model.NewRegistry()

	err := registry.Register(&BasicModel{})
	require.NoError(t, err)

	// Get metadata
	metadata, err := registry.GetMetadata(&BasicModel{})
	require.NoError(t, err)

	// Check table name
	assert.Equal(t, "BasicModels", metadata.TableName)

	// Check primary key
	require.NotNil(t, metadata.PrimaryKey)
	require.NotNil(t, metadata.PrimaryKey.PartitionKey)
	assert.Equal(t, "ID", metadata.PrimaryKey.PartitionKey.Name)
	assert.True(t, metadata.PrimaryKey.PartitionKey.IsPK)
	assert.Nil(t, metadata.PrimaryKey.SortKey)

	// Check fields
	assert.Len(t, metadata.Fields, 2)
	assert.Contains(t, metadata.Fields, "ID")
	assert.Contains(t, metadata.Fields, "Name")
}

func TestRegisterCompositeKeyModel(t *testing.T) {
	registry := model.NewRegistry()

	err := registry.Register(&CompositeKeyModel{})
	require.NoError(t, err)

	metadata, err := registry.GetMetadata(&CompositeKeyModel{})
	require.NoError(t, err)

	// Check composite key
	require.NotNil(t, metadata.PrimaryKey)
	require.NotNil(t, metadata.PrimaryKey.PartitionKey)
	require.NotNil(t, metadata.PrimaryKey.SortKey)

	assert.Equal(t, "UserID", metadata.PrimaryKey.PartitionKey.Name)
	assert.Equal(t, "Timestamp", metadata.PrimaryKey.SortKey.Name)
	assert.True(t, metadata.PrimaryKey.SortKey.IsSK)
}

func TestRegisterIndexedModel(t *testing.T) {
	registry := model.NewRegistry()

	err := registry.Register(&IndexedModel{})
	require.NoError(t, err)

	metadata, err := registry.GetMetadata(&IndexedModel{})
	require.NoError(t, err)

	// Check indexes
	assert.Len(t, metadata.Indexes, 3) // 2 GSIs + 1 LSI

	// Find GSI by name
	var emailGSI, categoryGSI, statusLSI *model.IndexSchema
	for i := range metadata.Indexes {
		switch metadata.Indexes[i].Name {
		case "gsi-email":
			emailGSI = &metadata.Indexes[i]
		case "gsi-category-price":
			categoryGSI = &metadata.Indexes[i]
		case "lsi-status":
			statusLSI = &metadata.Indexes[i]
		}
	}

	// Check email GSI
	require.NotNil(t, emailGSI)
	assert.Equal(t, model.GlobalSecondaryIndex, emailGSI.Type)
	assert.Equal(t, "Email", emailGSI.PartitionKey.Name)
	assert.Nil(t, emailGSI.SortKey)

	// Check category-price GSI
	require.NotNil(t, categoryGSI)
	assert.Equal(t, model.GlobalSecondaryIndex, categoryGSI.Type)
	assert.Equal(t, "Category", categoryGSI.PartitionKey.Name)
	assert.Equal(t, "Price", categoryGSI.SortKey.Name)

	// Check status LSI
	require.NotNil(t, statusLSI)
	assert.Equal(t, model.LocalSecondaryIndex, statusLSI.Type)
	assert.Equal(t, "Status", statusLSI.SortKey.Name)
}

func TestRegisterSpecialFieldsModel(t *testing.T) {
	registry := model.NewRegistry()

	err := registry.Register(&SpecialFieldsModel{})
	require.NoError(t, err)

	metadata, err := registry.GetMetadata(&SpecialFieldsModel{})
	require.NoError(t, err)

	// Check special fields
	require.NotNil(t, metadata.VersionField)
	assert.Equal(t, "Version", metadata.VersionField.Name)
	assert.True(t, metadata.VersionField.IsVersion)

	require.NotNil(t, metadata.TTLField)
	assert.Equal(t, "TTL", metadata.TTLField.Name)
	assert.True(t, metadata.TTLField.IsTTL)

	require.NotNil(t, metadata.CreatedAtField)
	assert.Equal(t, "CreatedAt", metadata.CreatedAtField.Name)
	assert.True(t, metadata.CreatedAtField.IsCreatedAt)

	require.NotNil(t, metadata.UpdatedAtField)
	assert.Equal(t, "UpdatedAt", metadata.UpdatedAtField.Name)
	assert.True(t, metadata.UpdatedAtField.IsUpdatedAt)
}

func TestRegisterCustomAttributeModel(t *testing.T) {
	registry := model.NewRegistry()

	err := registry.Register(&CustomAttributeModel{})
	require.NoError(t, err)

	metadata, err := registry.GetMetadata(&CustomAttributeModel{})
	require.NoError(t, err)

	// Check custom attribute names
	idField := metadata.Fields["ID"]
	require.NotNil(t, idField)
	assert.Equal(t, "userId", idField.DBName)

	usernameField := metadata.Fields["UserName"]
	require.NotNil(t, usernameField)
	assert.Equal(t, "username", usernameField.DBName)

	// Check set type
	tagsField := metadata.Fields["Tags"]
	require.NotNil(t, tagsField)
	assert.True(t, tagsField.IsSet)

	// Check omitempty
	optionalField := metadata.Fields["Optional"]
	require.NotNil(t, optionalField)
	assert.True(t, optionalField.OmitEmpty)

	// Check fields by DB name
	assert.Equal(t, idField, metadata.FieldsByDBName["userId"])
	assert.Equal(t, usernameField, metadata.FieldsByDBName["username"])
}

func TestRegisterPascalCaseNamingConvention_AllowsAcronymsAndGSIs(t *testing.T) {
	registry := model.NewRegistry()

	type PascalCaseUser struct {
		_ struct{} `theorydb:"naming:pascalCase"`

		ID string `theorydb:"pk"`
		SK string `theorydb:"sk"`

		GSI1PK string `theorydb:"index:gsi1,pk"`
		GSI1SK string `theorydb:"index:gsi1,sk"`
	}

	err := registry.Register(&PascalCaseUser{})
	require.NoError(t, err)

	metadata, err := registry.GetMetadata(&PascalCaseUser{})
	require.NoError(t, err)

	require.Equal(t, "ID", metadata.Fields["ID"].DBName)
	require.Equal(t, "SK", metadata.Fields["SK"].DBName)
	require.Equal(t, "GSI1PK", metadata.Fields["GSI1PK"].DBName)
	require.Equal(t, "GSI1SK", metadata.Fields["GSI1SK"].DBName)
}

func TestRegisterDynamORMNamingConvention_UsesPKSKAndCamelCase(t *testing.T) {
	registry := model.NewRegistry()

	type LegacyUser struct {
		_ struct{} `theorydb:"naming:dynamorm"`

		UserID    string `theorydb:"pk"`
		Entity    string `theorydb:"sk"`
		FirstName string
		EmailHash string `theorydb:"index:gsi-email,pk"`
	}

	err := registry.Register(&LegacyUser{})
	require.NoError(t, err)

	metadata, err := registry.GetMetadata(&LegacyUser{})
	require.NoError(t, err)

	require.Equal(t, naming.DynamORM, metadata.NamingConvention)
	require.Equal(t, "PK", metadata.Fields["UserID"].DBName)
	require.Equal(t, "SK", metadata.Fields["Entity"].DBName)
	require.Equal(t, "firstName", metadata.Fields["FirstName"].DBName)
	require.Equal(t, "emailHash", metadata.Fields["EmailHash"].DBName)
	require.Equal(t, "PK", metadata.PrimaryKey.PartitionKey.DBName)
	require.Equal(t, "SK", metadata.PrimaryKey.SortKey.DBName)
	require.Len(t, metadata.Indexes, 1)
	require.Equal(t, model.GlobalSecondaryIndex, metadata.Indexes[0].Type)
	require.Equal(t, "emailHash", metadata.Indexes[0].PartitionKey.DBName)
}

func TestRegisterInvalidModel(t *testing.T) {
	registry := model.NewRegistry()

	// Should fail - no primary key
	err := registry.Register(&InvalidModel{})
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "missing primary key")
}

func TestRegisterImplicitTimestampModel(t *testing.T) {
	registry := model.NewRegistry()

	err := registry.Register(&ImplicitTimestampModel{})
	require.NoError(t, err)

	metadata, err := registry.GetMetadata(&ImplicitTimestampModel{})
	require.NoError(t, err)

	require.NotNil(t, metadata.CreatedAtField)
	assert.Equal(t, "CreatedAt", metadata.CreatedAtField.Name)
	assert.True(t, metadata.CreatedAtField.IsCreatedAt)

	require.NotNil(t, metadata.UpdatedAtField)
	assert.Equal(t, "UpdatedAt", metadata.UpdatedAtField.Name)
	assert.True(t, metadata.UpdatedAtField.IsUpdatedAt)
}

func TestRegisterDuplicatePrimaryKey(t *testing.T) {
	type DuplicatePKModel struct {
		ID1 string `theorydb:"pk"`
		ID2 string `theorydb:"pk"`
	}
	type DynamORMDuplicatePKModel struct {
		_ struct{} `theorydb:"naming:dynamorm"`

		ID1 string `theorydb:"pk"`
		ID2 string `theorydb:"pk"`
	}
	type DynamORMDuplicateSKModel struct {
		_ struct{} `theorydb:"naming:dynamorm"`

		PK  string `theorydb:"pk"`
		SK1 string `theorydb:"sk"`
		SK2 string `theorydb:"sk"`
	}

	tests := []struct {
		model any
		name  string
	}{
		{name: "distinct database attributes", model: &DuplicatePKModel{}},
		{name: "DynamORM partition key alias", model: &DynamORMDuplicatePKModel{}},
		{name: "DynamORM sort key alias", model: &DynamORMDuplicateSKModel{}},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			registry := model.NewRegistry()
			err := registry.Register(test.model)
			require.ErrorIs(t, err, theorydbErrors.ErrDuplicatePrimaryKey)
			require.ErrorIs(t, err, theorydbErrors.ErrInvalidModel)
		})
	}
}

func TestRegisterRejectsDuplicateDBNameMappings(t *testing.T) {
	type DirectCollision struct {
		PK     string `theorydb:"pk,attr:PK" json:"PK"`
		First  string `theorydb:"attr:shared" json:"first"`
		Second string `theorydb:"attr:shared" json:"second"`
	}
	type EmbeddedFields struct {
		First string `theorydb:"attr:shared" json:"first"`
	}
	type EmbeddedCollision struct {
		EmbeddedFields
		PK     string `theorydb:"pk,attr:PK" json:"PK"`
		Second string `theorydb:"attr:shared" json:"second"`
	}

	tests := []struct {
		model any
		name  string
	}{
		{name: "direct fields", model: &DirectCollision{}},
		{name: "embedded field", model: &EmbeddedCollision{}},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			registry := model.NewRegistry()
			err := registry.Register(test.model)
			require.ErrorIs(t, err, theorydbErrors.ErrInvalidModel)
			require.NotErrorIs(t, err, theorydbErrors.ErrDuplicatePrimaryKey)
			require.ErrorContains(t, err, `duplicate database attribute name "shared"`)
			require.ErrorContains(t, err, "First")
			require.ErrorContains(t, err, "Second")
		})
	}
}

func TestRegisterRejectsEmbeddedTaggedFieldShadowing(t *testing.T) {
	type EmbeddedFields struct {
		Name string `theorydb:"attr:name" json:"name"`
	}
	type OtherEmbeddedFields struct {
		Name string `theorydb:"attr:name" json:"name"`
	}
	type EmbeddedShadowing struct {
		PK   string `theorydb:"pk,attr:PK" json:"PK"`
		Name string `theorydb:"attr:name" json:"name"`
		EmbeddedFields
	}
	type AmbiguousEmbedding struct {
		PK string `theorydb:"pk,attr:PK" json:"PK"`
		EmbeddedFields
		OtherEmbeddedFields
	}

	for _, test := range []struct {
		name  string
		model any
	}{
		{name: "outer field shadows embedded field", model: &EmbeddedShadowing{}},
		{name: "two embedded fields are ambiguous", model: &AmbiguousEmbedding{}},
	} {
		t.Run(test.name, func(t *testing.T) {
			registry := model.NewRegistry()
			err := registry.Register(test.model)
			require.ErrorIs(t, err, theorydbErrors.ErrInvalidModel)
			require.NotErrorIs(t, err, theorydbErrors.ErrDuplicatePrimaryKey)
			require.ErrorContains(
				t,
				err,
				`duplicate tagged Go field name "Name" mapped to database attribute "name" in a model with embedded structs is not supported`,
			)
			require.ErrorContains(t, err, "TableTheory may persist and read a different field than the consumer's Go code addresses")
			require.NotContains(t, err.Error(), "fields Name and Name")
		})
	}
}

func TestRegisterModelWithIndexModifiers(t *testing.T) {
	type IndexModifierModel struct {
		PK     string `theorydb:"pk,attr:PK"`
		SK     string `theorydb:"sk,attr:SK"`
		GSI1PK string `theorydb:"index:user-list-index,pk,attr:gsi1PK"`
		GSI1SK string `theorydb:"index:user-list-index,sk,attr:gsi1SK"`
		GSI2PK string `theorydb:"index:role-index,pk"`
		GSI2SK string `theorydb:"index:role-index,sk"`
	}

	registry := model.NewRegistry()

	err := registry.Register(&IndexModifierModel{})
	require.NoError(t, err)

	metadata, err := registry.GetMetadata(&IndexModifierModel{})
	require.NoError(t, err)

	require.NotNil(t, metadata.PrimaryKey)
	assert.Equal(t, "PK", metadata.PrimaryKey.PartitionKey.Name)
	assert.Equal(t, "SK", metadata.PrimaryKey.SortKey.Name)

	// Expect both GSIs with their respective keys
	require.Len(t, metadata.Indexes, 2)

	findIndex := func(name string) *model.IndexSchema {
		for i := range metadata.Indexes {
			if metadata.Indexes[i].Name == name {
				return &metadata.Indexes[i]
			}
		}
		return nil
	}

	userIndex := findIndex("user-list-index")
	require.NotNil(t, userIndex)
	assert.Equal(t, "GSI1PK", userIndex.PartitionKey.Name)
	assert.Equal(t, "GSI1SK", userIndex.SortKey.Name)

	roleIndex := findIndex("role-index")
	require.NotNil(t, roleIndex)
	assert.Equal(t, "GSI2PK", roleIndex.PartitionKey.Name)
	assert.Equal(t, "GSI2SK", roleIndex.SortKey.Name)
}

func TestRegisterRejectsEncryptedOnKeyFields(t *testing.T) {
	t.Run("PrimaryKey", func(t *testing.T) {
		type EncryptedPKModel struct {
			PK string `theorydb:"pk,encrypted"`
		}

		registry := model.NewRegistry()
		err := registry.Register(&EncryptedPKModel{})
		require.ErrorIs(t, err, theorydbErrors.ErrInvalidTag)
	})

	t.Run("IndexKey", func(t *testing.T) {
		type EncryptedIndexKeyModel struct {
			ID    string `theorydb:"pk"`
			Email string `theorydb:"index:gsi-email,pk,encrypted"`
		}

		registry := model.NewRegistry()
		err := registry.Register(&EncryptedIndexKeyModel{})
		require.ErrorIs(t, err, theorydbErrors.ErrInvalidTag)
	})
}

func TestRegisterInvalidTagTypes(t *testing.T) {
	tests := []struct {
		name  string
		model any
		error string
	}{
		{
			name: "invalid version type",
			model: &struct {
				ID      string `theorydb:"pk"`
				Version string `theorydb:"version"`
			}{},
			error: "version field must be numeric",
		},
		{
			name: "invalid ttl type",
			model: &struct {
				ID  string `theorydb:"pk"`
				TTL string `theorydb:"ttl"`
			}{},
			error: "ttl field must be int64 or uint64",
		},
		{
			name: "invalid set type",
			model: &struct {
				ID   string `theorydb:"pk"`
				Tags string `theorydb:"set"`
			}{},
			error: "set tag can only be used on slice types",
		},
		{
			name: "invalid timestamp type",
			model: &struct {
				ID        string `theorydb:"pk"`
				CreatedAt string `theorydb:"created_at"`
			}{},
			error: "created_at/updated_at fields must be time.Time",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			registry := model.NewRegistry()
			err := registry.Register(tt.model)
			assert.Error(t, err)
			assert.Contains(t, err.Error(), tt.error)
		})
	}
}

func TestRegisterInvalidTagNamesOffendingField(t *testing.T) {
	type TypoTagModel struct {
		ID     string `theorydb:"pk"`
		Status string `theorydb:"creted_at"`
	}

	registry := model.NewRegistry()
	err := registry.Register(&TypoTagModel{})
	require.ErrorIs(t, err, theorydbErrors.ErrInvalidTag)
	require.Contains(t, err.Error(), "Status")
	require.Contains(t, err.Error(), "unknown tag 'creted_at'")
}

func TestRegisterLSIPrefixWarning(t *testing.T) {
	type LegacyLSIPrefixModel struct {
		PK     string `theorydb:"pk"`
		SK     string `theorydb:"sk"`
		Status string `theorydb:"index:lsi-status,sk"`
	}

	registry := model.NewRegistry()
	require.NoError(t, registry.Register(&LegacyLSIPrefixModel{}))

	metadata, err := registry.GetMetadata(&LegacyLSIPrefixModel{})
	require.NoError(t, err)
	require.NotEmpty(t, metadata.Warnings)
	require.Contains(t, metadata.Warnings[0], "Status")
	require.Contains(t, metadata.Warnings[0], `theorydb:"lsi:lsi-status"`)

	type ExplicitLSIModel struct {
		PK     string `theorydb:"pk"`
		SK     string `theorydb:"sk"`
		Status string `theorydb:"lsi:lsi-status"`
	}

	registry = model.NewRegistry()
	require.NoError(t, registry.Register(&ExplicitLSIModel{}))
	metadata, err = registry.GetMetadata(&ExplicitLSIModel{})
	require.NoError(t, err)
	require.Empty(t, metadata.Warnings)
}

func TestGetMetadataByTable(t *testing.T) {
	registry := model.NewRegistry()

	err := registry.Register(&BasicModel{})
	require.NoError(t, err)

	// Get by table name
	metadata, err := registry.GetMetadataByTable("BasicModels")
	require.NoError(t, err)
	assert.Equal(t, "BasicModels", metadata.TableName)

	// Non-existent table
	_, err = registry.GetMetadataByTable("NonExistent")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "table not found")
}

func TestTableNameDerivation(t *testing.T) {
	tests := []struct {
		model     any
		tableName string
	}{
		{&BasicModel{}, "BasicModels"},
		{&struct {
			ID string `theorydb:"pk"`
		}{}, "s"},
		{
			&struct {
				ID string `theorydb:"pk"`
			}{},
			"s",
		},
	}

	for _, tt := range tests {
		t.Run(tt.tableName, func(t *testing.T) {
			registry := model.NewRegistry()
			err := registry.Register(tt.model)
			require.NoError(t, err)

			metadata, err := registry.GetMetadata(tt.model)
			require.NoError(t, err)
			assert.Equal(t, tt.tableName, metadata.TableName)
		})
	}
}

func TestRegisterPointerVsValue(t *testing.T) {
	registry := model.NewRegistry()

	// Register with pointer
	err := registry.Register(&BasicModel{})
	require.NoError(t, err)

	// Get metadata with value
	metadata1, err := registry.GetMetadata(BasicModel{})
	require.NoError(t, err)

	// Get metadata with pointer
	metadata2, err := registry.GetMetadata(&BasicModel{})
	require.NoError(t, err)

	// Should be the same metadata
	assert.Equal(t, metadata1, metadata2)
}
