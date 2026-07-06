package tabletheory

import (
	"testing"

	"github.com/stretchr/testify/require"
)

type typedFacadeRecord struct {
	PK string `theorydb:"pk" json:"PK"`
	SK string `theorydb:"sk" json:"SK"`
}

func TestTypedFacadeHelpers_THE2551(t *testing.T) {
	model := ModelOf[typedFacadeRecord](nil)

	require.Equal(t, NewKeyPair("tenant#1", "record#1"), model.Key("tenant#1", "record#1").Core())
	require.Equal(t, NewKeyPair("tenant#1"), NewTypedKey[typedFacadeRecord]("tenant#1").Core())
}
