package theorydb

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/theory-cloud/tabletheory/v4/pkg/core"
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
			name:        "case sensitive bare token does not match different case",
			compiled:    &core.CompiledQuery{ConditionExpression: "Version >= :expected"},
			versionAttr: "version",
			want:        false,
		},
		{
			name:        "case sensitive bare token matches exact case",
			compiled:    &core.CompiledQuery{ConditionExpression: "version >= :expected"},
			versionAttr: "version",
			want:        true,
		},
		{
			name: "placeholder prefix is not a whole token",
			compiled: &core.CompiledQuery{
				ConditionExpression:      "#value = :expected",
				ExpressionAttributeNames: map[string]string{"#v": "version", "#value": "status"},
			},
			versionAttr: "version",
			want:        false,
		},
		{
			name: "numbered placeholder prefix is not a whole token",
			compiled: &core.CompiledQuery{
				ConditionExpression:      "#n10 = :expected",
				ExpressionAttributeNames: map[string]string{"#n1": "version", "#n10": "status"},
			},
			versionAttr: "version",
			want:        false,
		},
		{
			name: "numbered placeholder exact token matches",
			compiled: &core.CompiledQuery{
				ConditionExpression:      "#n1 = :expected",
				ExpressionAttributeNames: map[string]string{"#n1": "version"},
			},
			versionAttr: "version",
			want:        true,
		},
		{
			name: "placeholder attribute name case must match",
			compiled: &core.CompiledQuery{
				ConditionExpression:      "#v = :expected",
				ExpressionAttributeNames: map[string]string{"#v": "Version"},
			},
			versionAttr: "version",
			want:        false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			require.Equal(t, tt.want, compiledConditionReferencesVersion(tt.compiled, tt.versionAttr))
		})
	}
}
