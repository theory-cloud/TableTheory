package encryption

import (
	"context"
	"testing"

	"github.com/aws/aws-sdk-go-v2/service/dynamodb/types"
	"github.com/stretchr/testify/require"

	"github.com/theory-cloud/tabletheory/pkg/model"
)

func TestIfNotExistsDefaultValueRef(t *testing.T) {
	cases := []struct {
		name string
		rhs  string
		want string
		ok   bool
	}{
		{name: "valid", rhs: "if_not_exists(#a, :v)", want: ":v", ok: true},
		{name: "valid_spacing", rhs: " if_not_exists( #a ,  :v ) ", want: ":v", ok: true},
		{name: "missing_prefix", rhs: "not_if_not_exists(#a, :v)", ok: false},
		{name: "missing_suffix", rhs: "if_not_exists(#a, :v", ok: false},
		{name: "wrong_arity", rhs: "if_not_exists(#a)", ok: false},
		{name: "second_arg_not_value_ref", rhs: "if_not_exists(#a, 123)", ok: false},
		{name: "second_arg_function", rhs: "if_not_exists(#a, if_not_exists(#b, :v))", ok: false},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, ok := ifNotExistsDefaultValueRef(tc.rhs)
			require.Equal(t, tc.ok, ok)
			require.Equal(t, tc.want, got)
		})
	}
}

func TestEncryptUpdateExpressionValues_EncryptsEncryptedSetAssignments(t *testing.T) {
	svc := newTestService(t)
	metadata := &model.Metadata{
		Fields: map[string]*model.FieldMetadata{
			"Secret": {DBName: "secret", IsEncrypted: true},
		},
	}

	exprAttrNames := map[string]string{"#s": "secret"}
	exprAttrValues := map[string]types.AttributeValue{":v": &types.AttributeValueMemberS{Value: "hello"}}

	require.NoError(t, EncryptUpdateExpressionValues(
		context.Background(),
		svc,
		metadata,
		"SET #s = :v",
		exprAttrNames,
		exprAttrValues,
	))
	require.IsType(t, &types.AttributeValueMemberM{}, exprAttrValues[":v"])
}

func TestEncryptUpdateExpressionValues_EncryptsIfNotExistsDefault(t *testing.T) {
	svc := newTestService(t)
	metadata := &model.Metadata{
		Fields: map[string]*model.FieldMetadata{
			"Secret": {DBName: "secret", IsEncrypted: true},
		},
	}

	exprAttrNames := map[string]string{"#s": "secret"}
	exprAttrValues := map[string]types.AttributeValue{":v": &types.AttributeValueMemberS{Value: "hello"}}

	require.NoError(t, EncryptUpdateExpressionValues(
		context.Background(),
		svc,
		metadata,
		"SET #s = if_not_exists(#s, :v)",
		exprAttrNames,
		exprAttrValues,
	))
	require.IsType(t, &types.AttributeValueMemberM{}, exprAttrValues[":v"])
}

func TestEncryptUpdateExpressionValues_RejectsEncryptedNestedOrIndexedUpdates(t *testing.T) {
	svc := newTestService(t)
	metadata := &model.Metadata{
		Fields: map[string]*model.FieldMetadata{
			"Secret": {DBName: "secret", IsEncrypted: true},
		},
	}

	exprAttrNames := map[string]string{"#s": "secret"}
	exprAttrValues := map[string]types.AttributeValue{":v": &types.AttributeValueMemberS{Value: "hello"}}

	err := EncryptUpdateExpressionValues(
		context.Background(),
		svc,
		metadata,
		"SET #s.a = :v",
		exprAttrNames,
		exprAttrValues,
	)
	require.Error(t, err)
	require.ErrorContains(t, err, "does not support nested or indexed updates")
}

func TestEncryptUpdateExpressionValues_RejectsUnsupportedIfNotExists(t *testing.T) {
	svc := newTestService(t)
	metadata := &model.Metadata{
		Fields: map[string]*model.FieldMetadata{
			"Secret": {DBName: "secret", IsEncrypted: true},
		},
	}

	exprAttrNames := map[string]string{"#s": "secret"}
	exprAttrValues := map[string]types.AttributeValue{":v": &types.AttributeValueMemberS{Value: "hello"}}

	err := EncryptUpdateExpressionValues(
		context.Background(),
		svc,
		metadata,
		"SET #s = if_not_exists(#s, 123)",
		exprAttrNames,
		exprAttrValues,
	)
	require.Error(t, err)
	require.ErrorContains(t, err, "unsupported if_not_exists expression")
}

func TestEncryptUpdateExpressionValues_RejectsUnsupportedEncryptedUpdateExpression(t *testing.T) {
	svc := newTestService(t)
	metadata := &model.Metadata{
		Fields: map[string]*model.FieldMetadata{
			"Secret": {DBName: "secret", IsEncrypted: true},
		},
	}

	exprAttrNames := map[string]string{"#s": "secret"}
	exprAttrValues := map[string]types.AttributeValue{
		":a": &types.AttributeValueMemberS{Value: "a"},
		":b": &types.AttributeValueMemberS{Value: "b"},
	}

	err := EncryptUpdateExpressionValues(
		context.Background(),
		svc,
		metadata,
		"SET #s = list_append(:a, :b)",
		exprAttrNames,
		exprAttrValues,
	)
	require.Error(t, err)
	require.ErrorContains(t, err, "unsupported update expression")
}

func TestEncryptUpdateExpressionValues_ErrorsWhenEncryptedValueRefMissing(t *testing.T) {
	svc := newTestService(t)
	metadata := &model.Metadata{
		Fields: map[string]*model.FieldMetadata{
			"Secret": {DBName: "secret", IsEncrypted: true},
		},
	}

	exprAttrNames := map[string]string{"#s": "secret"}
	exprAttrValues := map[string]types.AttributeValue{
		":other": &types.AttributeValueMemberS{Value: "hello"},
	}

	err := EncryptUpdateExpressionValues(
		context.Background(),
		svc,
		metadata,
		"SET #s = :missing",
		exprAttrNames,
		exprAttrValues,
	)
	require.Error(t, err)
	require.ErrorContains(t, err, "missing expression attribute value")
}

func TestEncryptUpdateExpressionValues_RejectsADDAndDELETE(t *testing.T) {
	svc := newTestService(t)
	metadata := &model.Metadata{
		Fields: map[string]*model.FieldMetadata{
			"Secret": {DBName: "secret", IsEncrypted: true},
		},
	}

	t.Run("rejects_add", func(t *testing.T) {
		exprAttrNames := map[string]string{"#s": "secret"}
		exprAttrValues := map[string]types.AttributeValue{
			":v": &types.AttributeValueMemberS{Value: "hello"},
			":n": &types.AttributeValueMemberN{Value: "1"},
		}

		err := EncryptUpdateExpressionValues(
			context.Background(),
			svc,
			metadata,
			"SET #s = :v ADD #s :n",
			exprAttrNames,
			exprAttrValues,
		)
		require.Error(t, err)
		require.ErrorContains(t, err, "does not support ADD updates")
	})

	t.Run("rejects_delete", func(t *testing.T) {
		exprAttrNames := map[string]string{"#s": "secret"}
		exprAttrValues := map[string]types.AttributeValue{
			":v": &types.AttributeValueMemberS{Value: "hello"},
		}

		err := EncryptUpdateExpressionValues(
			context.Background(),
			svc,
			metadata,
			"DELETE #s :v",
			exprAttrNames,
			exprAttrValues,
		)
		require.Error(t, err)
		require.ErrorContains(t, err, "does not support DELETE updates")
	})
}

func TestEncryptUpdateExpressionValues_IgnoresAddForNonEncryptedField(t *testing.T) {
	svc := newTestService(t)
	metadata := &model.Metadata{
		Fields: map[string]*model.FieldMetadata{
			"Secret": {DBName: "secret", IsEncrypted: true},
		},
	}

	exprAttrNames := map[string]string{"#p": "plain"}
	exprAttrValues := map[string]types.AttributeValue{":n": &types.AttributeValueMemberN{Value: "1"}}

	require.NoError(t, EncryptUpdateExpressionValues(
		context.Background(),
		svc,
		metadata,
		"ADD #p :n",
		exprAttrNames,
		exprAttrValues,
	))
}

func TestEncryptUpdateExpressionValues_EarlyReturnWhenNoValues(t *testing.T) {
	svc := newTestService(t)
	metadata := &model.Metadata{
		Fields: map[string]*model.FieldMetadata{
			"Secret": {DBName: "secret", IsEncrypted: true},
		},
	}

	require.NoError(t, EncryptUpdateExpressionValues(
		context.Background(),
		svc,
		metadata,
		"SET #s = :v",
		map[string]string{"#s": "secret"},
		nil,
	))
}

func TestSplitAssignment_NoEquals_ReturnsFalse(t *testing.T) {
	_, _, ok := splitAssignment("not-an-assignment")
	require.False(t, ok)
}

func TestSplitTopLevelCommaSeparated_EmptyReturnsNil(t *testing.T) {
	require.Nil(t, splitTopLevelCommaSeparated(""))
	require.Nil(t, splitTopLevelCommaSeparated("   "))
}

func TestEncryptSetAssignment_IgnoresUnknownOrUnencryptedFields(t *testing.T) {
	svc := newTestService(t)

	t.Run("unknown_attribute_name_mapping", func(t *testing.T) {
		values := map[string]types.AttributeValue{":v": &types.AttributeValueMemberS{Value: "hello"}}
		require.NoError(t, encryptSetAssignment(
			context.Background(),
			svc,
			map[string]struct{}{"secret": {}},
			"#s = :v",
			map[string]string{},
			values,
		))
		require.IsType(t, &types.AttributeValueMemberS{}, values[":v"])
	})

	t.Run("not_encrypted", func(t *testing.T) {
		values := map[string]types.AttributeValue{":v": &types.AttributeValueMemberS{Value: "hello"}}
		require.NoError(t, encryptSetAssignment(
			context.Background(),
			svc,
			map[string]struct{}{"other": {}},
			"#s = :v",
			map[string]string{"#s": "secret"},
			values,
		))
		require.IsType(t, &types.AttributeValueMemberS{}, values[":v"])
	})
}

func TestEncryptedAttributeNameSet_CoversNilAndTaggedFields(t *testing.T) {
	require.Nil(t, encryptedAttributeNameSet(nil))

	metadata := &model.Metadata{
		Fields: map[string]*model.FieldMetadata{
			"Nil":      nil,
			"Tagged":   {DBName: "tagged", Tags: map[string]string{"encrypted": ""}},
			"Explicit": {DBName: "explicit", IsEncrypted: true},
			"Plain":    {DBName: "plain"},
		},
	}

	set := encryptedAttributeNameSet(metadata)
	_, ok := set["tagged"]
	require.True(t, ok)
	_, ok = set["explicit"]
	require.True(t, ok)
	_, ok = set["plain"]
	require.False(t, ok)
}

func TestEncryptValueRef_PropagatesEncryptErrors(t *testing.T) {
	values := map[string]types.AttributeValue{":v": &types.AttributeValueMemberS{Value: "hello"}}
	err := encryptValueRef(context.Background(), nil, "secret", ":v", values)
	require.Error(t, err)
	require.ErrorContains(t, err, "encryption service is nil")
}
