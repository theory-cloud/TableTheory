package query

import (
	"reflect"
	"testing"

	"github.com/aws/aws-sdk-go-v2/service/dynamodb/types"
	"github.com/stretchr/testify/require"

	"github.com/theory-cloud/tabletheory/v4/pkg/core"
	pkgtypes "github.com/theory-cloud/tabletheory/v4/pkg/types"
)

type QueryHiddenPrivileges struct {
	IsAdmin bool `json:"is_admin"`
}

// queryItemExcluded excludes its promoted subtree with theorydb:"-".
//
//nolint:govet // Field order and anonymous embeds mirror the security fixture under test.
type queryItemExcluded struct {
	QueryHiddenPrivileges `theorydb:"-"`
	Name                  string `json:"name"`
}

// queryItemVisible is the authorized-success control.
//
//nolint:govet
type queryItemVisible struct {
	QueryHiddenPrivileges
	Name string `json:"name"`
}

func TestUnmarshalItemExcludesTaggedAnonymousEmbeds(t *testing.T) {
	item := map[string]types.AttributeValue{
		"is_admin": &types.AttributeValueMemberBOOL{Value: true},
		"name":     &types.AttributeValueMemberS{Value: "visible"},
	}

	var excluded queryItemExcluded
	require.NoError(t, UnmarshalItem(item, &excluded))
	require.False(t, excluded.IsAdmin, "excluded embed must not accept a promoted attribute")
	require.Equal(t, "visible", excluded.Name)

	var visible queryItemVisible
	require.NoError(t, UnmarshalItem(item, &visible))
	require.True(t, visible.IsAdmin)
	require.Equal(t, "visible", visible.Name)
}

func TestMarshalItemTaggedFlatExcludesTaggedAnonymousEmbeds(t *testing.T) {
	excluded := New(&queryItemExcluded{},
		&cov4Metadata{table: "tbl", pk: core.KeySchema{PartitionKey: "Name"}},
		&cov4Executor{},
	).WithConverter(pkgtypes.NewConverter().WithFlatAnonymousEmbedEncoding())

	out, err := excluded.marshalItemTaggedFlat(reflect.ValueOf(queryItemExcluded{
		QueryHiddenPrivileges: QueryHiddenPrivileges{IsAdmin: true},
		Name:                  "visible",
	}))
	require.NoError(t, err)
	require.Contains(t, out, "Name")
	require.NotContains(t, out, "IsAdmin")
	require.NotContains(t, out, "is_admin")

	visible := New(&queryItemVisible{},
		&cov4Metadata{table: "tbl", pk: core.KeySchema{PartitionKey: "Name"}},
		&cov4Executor{},
	).WithConverter(pkgtypes.NewConverter().WithFlatAnonymousEmbedEncoding())

	vout, err := visible.marshalItemTaggedFlat(reflect.ValueOf(queryItemVisible{
		QueryHiddenPrivileges: QueryHiddenPrivileges{IsAdmin: true},
		Name:                  "visible",
	}))
	require.NoError(t, err)
	require.Contains(t, vout, "IsAdmin")
	require.Contains(t, vout, "Name")
}
