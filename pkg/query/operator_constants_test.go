package query_test

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/theory-cloud/tabletheory/v2/pkg/query"
)

func TestOperatorConstantsAreStringCompatible(t *testing.T) {
	op := string(query.OpBeginsWith)
	require.Equal(t, "BEGINS_WITH", op)
	require.Equal(t, []any{"A", "Z"}, query.Between("A", "Z"))
}
