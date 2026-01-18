package theorydb

import (
	"testing"

	"github.com/stretchr/testify/require"
)

type cov6ConditionModel struct {
	ID string `theorydb:"pk,attr:id"`
}

func (cov6ConditionModel) TableName() string { return "cov6_condition_models" }

func TestQuery_WithConditionExpression_ValidatesAndDetectsDuplicates_COV6(t *testing.T) {
	db := newBareDB()

	err := db.Model(&cov6ConditionModel{ID: "u1"}).WithConditionExpression("", nil).Create()
	require.ErrorContains(t, err, "condition expression cannot be empty")

	err = db.Model(&cov6ConditionModel{ID: "u2"}).
		WithConditionExpression("name = :raw", map[string]any{":raw": "alice"}).
		WithConditionExpression("other = :raw", map[string]any{":raw": "bob"}).
		Create()
	require.ErrorContains(t, err, "duplicate placeholder :raw")
}
