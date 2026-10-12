package dms_test

import (
	"encoding/json"
	"regexp"
	"testing"

	"github.com/aws/aws-sdk-go-v2/service/dynamodb/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/theory-cloud/tabletheory/v4"
	"github.com/theory-cloud/tabletheory/v4/pkg/dms"
	"github.com/theory-cloud/tabletheory/v4/pkg/session"
	"github.com/theory-cloud/tabletheory/v4/pkg/testing/fakedb"
)

const exactNumberDMSDocument = `
dms_version: "0.2"
models:
  - name: "ExactNumber"
    table: { name: "exact_numbers" }
    keys:
      partition: { attribute: "PK", type: "S" }
    attributes:
      - { attribute: "PK", type: "S", required: true, roles: ["pk"] }
      - { attribute: "amount", type: "N", format: "decimal_string", optional: true }
`

type exactNumberRecord struct {
	PK     string      `theorydb:"pk,attr:PK" json:"PK"`
	Amount json.Number `theorydb:"attr:amount" json:"amount"`
}

func (exactNumberRecord) TableName() string { return "exact_numbers" }

func TestGenerateGoDecimalStringEmitsJSONNumber(t *testing.T) {
	doc, err := dms.ParseDocument([]byte(exactNumberDMSDocument))
	require.NoError(t, err)

	out, err := dms.Generate(doc, dms.GenerateOptions{Lang: "go", PackageName: "driver"})
	require.NoError(t, err)
	generated := string(out)

	assert.Contains(t, generated, `"encoding/json"`)
	assert.Regexp(t, regexp.MustCompile(`Amount\s+json\.Number`), generated)
	assert.NotContains(t, generated, "DecimalString")
}

// A generated decimal_string field must round-trip DynamoDB N through the normal
// framework paths, with no out-of-band converter registration.
func TestGeneratedExactNumberFieldRoundTripsAsDynamoDBN(t *testing.T) {
	fake := fakedb.New()
	db, err := tabletheory.NewWithClient(session.Config{Region: "us-east-1"}, fake)
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&exactNumberRecord{}))

	// "9007199254740993" is not representable as a float64, so a plain string
	// type would have marshaled it as S and lost the numeric contract.
	require.NoError(t, db.Model(&exactNumberRecord{PK: "1", Amount: json.Number("9007199254740993")}).Create())

	items := fake.Items("exact_numbers")
	require.Len(t, items, 1)
	stored, ok := items[0]["amount"].(*types.AttributeValueMemberN)
	require.True(t, ok, "generated exact-number field must marshal as N, got %T", items[0]["amount"])
	assert.Equal(t, "9007199254740993", stored.Value)

	var got exactNumberRecord
	require.NoError(t, db.Model(&exactNumberRecord{}).Where("PK", "=", "1").First(&got))
	assert.Equal(t, json.Number("9007199254740993"), got.Amount)
}

func TestGeneratedExactNumberFieldFailsClosedOnInvalidText(t *testing.T) {
	fake := fakedb.New()
	db, err := tabletheory.NewWithClient(session.Config{Region: "us-east-1"}, fake)
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&exactNumberRecord{}))
	require.NoError(t, fake.Seed("exact_numbers", map[string]types.AttributeValue{
		"PK":     &types.AttributeValueMemberS{Value: "1"},
		"amount": &types.AttributeValueMemberN{Value: "not-a-number"},
	}))

	var got exactNumberRecord
	err = db.Model(&exactNumberRecord{}).Where("PK", "=", "1").First(&got)
	require.Error(t, err)
}
