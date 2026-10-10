package model_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	theorydbErrors "github.com/theory-cloud/tabletheory/v4/pkg/errors"
	"github.com/theory-cloud/tabletheory/v4/pkg/model"
)

// A present-but-empty attr: override is malformed and must fail registration
// before the naming convention can substitute an inferred name. The matrix
// covers each naming convention.
type emptyAttrCamelCase struct {
	ID    string `theorydb:"pk,attr:"`
	Value string `theorydb:"attr:valueName"`
}

type emptyAttrSnakeCase struct {
	_     struct{} `theorydb:"naming:snake_case"`
	ID    string   `theorydb:"pk,attr:user_id"`
	Value string   `theorydb:"attr:"`
}

type emptyAttrPascalCase struct {
	_     struct{} `theorydb:"naming:pascalCase"`
	ID    string   `theorydb:"pk,attr:ID"`
	Value string   `theorydb:"attr:"`
}

type emptyAttrDynamORM struct {
	_  struct{} `theorydb:"naming:dynamorm"`
	ID string   `theorydb:"pk,attr:"`
}

func TestRegisterRejectsPresentButEmptyAttrOverride(t *testing.T) {
	cases := map[string]any{
		"camelCase":  &emptyAttrCamelCase{},
		"snake_case": &emptyAttrSnakeCase{},
		"pascalCase": &emptyAttrPascalCase{},
		"dynamorm":   &emptyAttrDynamORM{},
	}

	for name, mdl := range cases {
		t.Run(name, func(t *testing.T) {
			registry := model.NewRegistry()
			err := registry.Register(mdl)
			require.Error(t, err)
			require.ErrorIs(t, err, theorydbErrors.ErrInvalidTag)
			assert.Contains(t, err.Error(), "attr:")
		})
	}
}

func TestRegisterConventionNamesAndValidAttrOverride(t *testing.T) {
	t.Run("camelCase default", func(t *testing.T) {
		type m struct {
			ID       string `theorydb:"pk"`
			UserName string `theorydb:"attr:username"`
			Plain    string
		}
		registry := model.NewRegistry()
		require.NoError(t, registry.Register(&m{}))
		meta, err := registry.GetMetadata(&m{})
		require.NoError(t, err)
		assert.Equal(t, "id", meta.Fields["ID"].DBName)
		assert.Equal(t, "username", meta.Fields["UserName"].DBName)
		assert.Equal(t, "plain", meta.Fields["Plain"].DBName)
	})

	t.Run("snake_case", func(t *testing.T) {
		type m struct {
			_        struct{} `theorydb:"naming:snake_case"`
			ID       string   `theorydb:"pk"`
			UserName string   `theorydb:"attr:username"`
			Plain    string
		}
		registry := model.NewRegistry()
		require.NoError(t, registry.Register(&m{}))
		meta, err := registry.GetMetadata(&m{})
		require.NoError(t, err)
		assert.Equal(t, "id", meta.Fields["ID"].DBName)
		assert.Equal(t, "username", meta.Fields["UserName"].DBName)
		assert.Equal(t, "plain", meta.Fields["Plain"].DBName)
	})

	t.Run("pascalCase", func(t *testing.T) {
		type m struct {
			_        struct{} `theorydb:"naming:pascalCase"`
			ID       string   `theorydb:"pk"`
			UserName string   `theorydb:"attr:UserNameOverride"`
			Plain    string
		}
		registry := model.NewRegistry()
		require.NoError(t, registry.Register(&m{}))
		meta, err := registry.GetMetadata(&m{})
		require.NoError(t, err)
		assert.Equal(t, "ID", meta.Fields["ID"].DBName)
		assert.Equal(t, "UserNameOverride", meta.Fields["UserName"].DBName)
		assert.Equal(t, "Plain", meta.Fields["Plain"].DBName)
	})
}
