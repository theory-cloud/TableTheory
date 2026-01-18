package expr

import (
	"testing"
	"time"

	"github.com/aws/aws-sdk-go-v2/service/dynamodb/types"
	"github.com/stretchr/testify/require"

	"github.com/theory-cloud/tabletheory/pkg/validation"
)

type cov6Unmarshaler struct {
	called bool
}

func (u *cov6Unmarshaler) UnmarshalDynamoDBAttributeValue(types.AttributeValue) error {
	u.called = true
	return nil
}

func TestBuilder_addNameSecure_HandlesInvalidAndNested_COV6(t *testing.T) {
	b := NewBuilder()

	require.Equal(t, "#invalid", b.addNameSecure(""))

	first := b.addNameSecure("status")
	second := b.addNameSecure("status")
	require.Equal(t, first, second)

	nested := b.addNameSecure("profile.name")
	require.NotEqual(t, "#invalid", nested)
	require.Contains(t, nested, ".")
}

func TestBuilder_convertToSliceSecure_CoversErrorsAndTypes_COV6(t *testing.T) {
	b := NewBuilder()

	got, err := b.convertToSliceSecure([]any{"ok"})
	require.NoError(t, err)
	require.Equal(t, []any{"ok"}, got)

	_, err = b.convertToSliceSecure([]any{make(chan int)})
	var secErr *validation.SecurityError
	require.ErrorAs(t, err, &secErr)

	_, err = b.convertToSliceSecure([]string{"union select * from users"})
	require.ErrorAs(t, err, &secErr)

	got, err = b.convertToSliceSecure([]int{1, 2})
	require.NoError(t, err)
	require.Equal(t, []any{1, 2}, got)

	_, err = b.convertToSliceSecure(123)
	require.ErrorAs(t, err, &secErr)
}

func TestConvertToAttributeValueSecure_RejectsInvalidValue_COV6(t *testing.T) {
	_, err := ConvertToAttributeValueSecure(make(chan int))
	require.Error(t, err)
}

func TestBuilder_addValueAsSet_ReturnsErrorOnInvalidOrUnsupported_COV6(t *testing.T) {
	b := NewBuilder()

	_, err := b.addValueAsSet(make(chan int))
	require.Error(t, err)

	_, err = b.addValueAsSet([]struct{}{{}})
	require.Error(t, err)
}

func TestBuilder_convertToSetAttributeValue_CoversAdditionalBranches_COV6(t *testing.T) {
	b := NewBuilder()

	av, err := b.convertToSetAttributeValue([]string{})
	require.NoError(t, err)
	_, ok := av.(*types.AttributeValueMemberNULL)
	require.True(t, ok)

	av, err = b.convertToSetAttributeValue([]any{})
	require.NoError(t, err)
	_, ok = av.(*types.AttributeValueMemberNULL)
	require.True(t, ok)

	av, err = b.convertToSetAttributeValue([]any{"a", "b"})
	require.NoError(t, err)
	_, ok = av.(*types.AttributeValueMemberSS)
	require.True(t, ok)

	_, err = b.convertToSetAttributeValue([]any{"a", 1})
	require.ErrorContains(t, err, "mixed types")

	_, err = b.convertToSetAttributeValue(123)
	require.ErrorContains(t, err, "requires a slice value")

	av, err = b.convertToSetAttributeValue([]float64{1.25})
	require.NoError(t, err)
	_, ok = av.(*types.AttributeValueMemberNS)
	require.True(t, ok)

	av, err = b.convertToSetAttributeValue([][]byte{[]byte("x")})
	require.NoError(t, err)
	_, ok = av.(*types.AttributeValueMemberBS)
	require.True(t, ok)

	_, err = b.convertToSetAttributeValue([]struct{}{{}})
	require.ErrorContains(t, err, "unsupported set element type")

	_, err = b.convertToSetAttributeValue(make(chan int))
	require.ErrorContains(t, err, "security validation failed")
}

func TestConvertFromAttributeValue_CoversInterfacesPointersAndUnmarshalers_COV6(t *testing.T) {
	err := ConvertFromAttributeValue(&types.AttributeValueMemberS{Value: "x"}, "not-a-pointer")
	require.ErrorContains(t, err, "target must be a non-nil pointer")

	var u cov6Unmarshaler
	require.NoError(t, ConvertFromAttributeValue(&types.AttributeValueMemberS{Value: "x"}, &u))
	require.True(t, u.called)

	var out any
	require.NoError(t, ConvertFromAttributeValue(&types.AttributeValueMemberN{Value: "1"}, &out))
	require.Equal(t, int64(1), out)

	var sp *string
	require.NoError(t, ConvertFromAttributeValue(&types.AttributeValueMemberS{Value: "ok"}, &sp))
	require.NotNil(t, sp)
	require.Equal(t, "ok", *sp)

	now := time.Now().UTC().Truncate(time.Second)
	var tm time.Time
	require.NoError(t, ConvertFromAttributeValue(&types.AttributeValueMemberS{Value: now.Format(time.RFC3339Nano)}, &tm))
	require.WithinDuration(t, now, tm, time.Second)

	var badTime time.Time
	require.Error(t, ConvertFromAttributeValue(&types.AttributeValueMemberS{Value: "nope"}, &badTime))

	var u64 uint64
	require.NoError(t, ConvertFromAttributeValue(&types.AttributeValueMemberN{Value: "42"}, &u64))
	require.Equal(t, uint64(42), u64)

	var badInt int
	require.Error(t, ConvertFromAttributeValue(&types.AttributeValueMemberN{Value: "nope"}, &badInt))
}
