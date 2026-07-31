package tabletheory

import (
	"testing"

	"github.com/aws/aws-sdk-go-v2/service/dynamodb/types"
	"github.com/stretchr/testify/require"

	pkgTypes "github.com/theory-cloud/tabletheory/v3/pkg/types"
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

type fixedArrayFacadeRecord struct {
	PK   string   `theorydb:"pk,attr:PK" json:"PK"`
	Hash [32]byte `theorydb:"attr:hash" json:"hash"`
}

func TestUnmarshalItemRoundTripsFixedArray(t *testing.T) {
	hash := [32]byte{0, 1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15,
		16, 17, 18, 19, 20, 21, 22, 23, 24, 25, 26, 27, 28, 29, 30, 31}
	hashValue, err := pkgTypes.NewConverter().ToAttributeValue(hash)
	require.NoError(t, err)
	require.IsType(t, &types.AttributeValueMemberL{}, hashValue)

	var out fixedArrayFacadeRecord
	require.NoError(t, UnmarshalItem(map[string]types.AttributeValue{
		"PK":   &types.AttributeValueMemberS{Value: "HASH#stream"},
		"hash": hashValue,
	}, &out))
	require.Equal(t, "HASH#stream", out.PK)
	require.Equal(t, hash, out.Hash)
}

func TestUnmarshalItemRejectsFixedArrayLengthMismatch(t *testing.T) {
	var out fixedArrayFacadeRecord
	err := UnmarshalItem(map[string]types.AttributeValue{
		"PK": &types.AttributeValueMemberS{Value: "HASH#mismatch"},
		"hash": &types.AttributeValueMemberL{Value: []types.AttributeValue{
			&types.AttributeValueMemberN{Value: "1"},
		}},
	}, &out)
	require.EqualError(t, err, "failed to unmarshal field Hash: list length 1 does not match array length 32")
}
