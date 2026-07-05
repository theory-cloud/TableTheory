package theorydb

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/theory-cloud/tabletheory/pkg/core"
)

func TestCompiledConditionReferencesVersion_TokenBoundaries_THE2551(t *testing.T) {
	tests := []struct {
		name        string
		compiled    *core.CompiledQuery
		versionAttr string
		want        bool
	}{
		{
			name:        "requires a version attribute",
			compiled:    &core.CompiledQuery{ConditionExpression: "version = :v"},
			versionAttr: "",
			want:        false,
		},
		{
			name:        "bare version token",
			compiled:    &core.CompiledQuery{ConditionExpression: "attribute_exists(version) AND version = :v"},
			versionAttr: "version",
			want:        true,
		},
		{
			name: "placeholder maps to version attribute",
			compiled: &core.CompiledQuery{
				ConditionExpression:      "#v = :expected",
				ExpressionAttributeNames: map[string]string{"#v": "recordVersion"},
			},
			versionAttr: "recordVersion",
			want:        true,
		},
		{
			name:        "substring inside identifier is ignored",
			compiled:    &core.CompiledQuery{ConditionExpression: "preview_version = :v OR versioned = :next"},
			versionAttr: "version",
			want:        false,
		},
		{
			name:        "case insensitive bare token",
			compiled:    &core.CompiledQuery{ConditionExpression: "Version >= :expected"},
			versionAttr: "version",
			want:        true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			require.Equal(t, tt.want, compiledConditionReferencesVersion(tt.compiled, tt.versionAttr))
		})
	}
}
