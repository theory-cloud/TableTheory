package query

import (
	"reflect"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go-v2/service/dynamodb/types"
	"github.com/stretchr/testify/require"
)

func TestUnmarshalAttributeValue_CoversCommonTypesAndErrors_COV6(t *testing.T) {
	t.Run("cannot set", func(t *testing.T) {
		err := unmarshalAttributeValue(&types.AttributeValueMemberS{Value: "x"}, reflect.ValueOf("x"))
		require.ErrorContains(t, err, "cannot set value")
	})

	t.Run("pointer handling (nil/null/alloc)", func(t *testing.T) {
		var p *string
		dest := reflect.ValueOf(&p).Elem()

		require.NoError(t, unmarshalAttributeValue(nil, dest))
		require.Nil(t, p)

		require.NoError(t, unmarshalAttributeValue(&types.AttributeValueMemberNULL{Value: true}, dest))
		require.Nil(t, p)

		require.NoError(t, unmarshalAttributeValue(&types.AttributeValueMemberS{Value: "ok"}, dest))
		require.NotNil(t, p)
		require.Equal(t, "ok", *p)
	})

	t.Run("interface handling", func(t *testing.T) {
		var out any
		dest := reflect.ValueOf(&out).Elem()

		require.NoError(t, unmarshalAttributeValue(&types.AttributeValueMemberN{Value: "1"}, dest))
		require.Equal(t, int64(1), out)

		require.NoError(t, unmarshalAttributeValue(&types.AttributeValueMemberN{Value: "1.5"}, dest))
		require.Equal(t, float64(1.5), out)

		require.NoError(t, unmarshalAttributeValue(&types.AttributeValueMemberNULL{Value: true}, dest))
		require.Nil(t, out)
	})

	t.Run("string variants", func(t *testing.T) {
		var s string
		dest := reflect.ValueOf(&s).Elem()
		require.NoError(t, unmarshalAttributeValue(&types.AttributeValueMemberS{Value: "hi"}, dest))
		require.Equal(t, "hi", s)

		var tm time.Time
		destTime := reflect.ValueOf(&tm).Elem()
		now := time.Now().UTC().Truncate(time.Second)
		require.NoError(t, unmarshalAttributeValue(&types.AttributeValueMemberS{Value: now.Format(time.RFC3339)}, destTime))
		require.WithinDuration(t, now, tm, time.Second)

		var jsonMap map[string]string
		destMap := reflect.ValueOf(&jsonMap).Elem()
		require.NoError(t, unmarshalAttributeValue(&types.AttributeValueMemberS{Value: `{"a":"b"}`}, destMap))
		require.Equal(t, map[string]string{"a": "b"}, jsonMap)

		var bad int
		destBad := reflect.ValueOf(&bad).Elem()
		require.ErrorContains(t, unmarshalAttributeValue(&types.AttributeValueMemberS{Value: "x"}, destBad), "cannot unmarshal string")
	})

	t.Run("number parsing", func(t *testing.T) {
		var i int64
		destI := reflect.ValueOf(&i).Elem()
		require.NoError(t, unmarshalAttributeValue(&types.AttributeValueMemberN{Value: "42"}, destI))
		require.Equal(t, int64(42), i)

		var f float64
		destF := reflect.ValueOf(&f).Elem()
		require.NoError(t, unmarshalAttributeValue(&types.AttributeValueMemberN{Value: "3.14"}, destF))
		require.InEpsilon(t, 3.14, f, 0.0001)

		var bad int
		destBad := reflect.ValueOf(&bad).Elem()
		require.Error(t, unmarshalAttributeValue(&types.AttributeValueMemberN{Value: "nope"}, destBad))
	})

	t.Run("bool", func(t *testing.T) {
		var b bool
		dest := reflect.ValueOf(&b).Elem()
		require.NoError(t, unmarshalAttributeValue(&types.AttributeValueMemberBOOL{Value: true}, dest))
		require.True(t, b)

		var s string
		destBad := reflect.ValueOf(&s).Elem()
		require.ErrorContains(t, unmarshalAttributeValue(&types.AttributeValueMemberBOOL{Value: true}, destBad), "cannot unmarshal bool")
	})

	t.Run("list", func(t *testing.T) {
		var nums []int64
		dest := reflect.ValueOf(&nums).Elem()
		require.NoError(t, unmarshalAttributeValue(&types.AttributeValueMemberL{Value: []types.AttributeValue{
			&types.AttributeValueMemberN{Value: "1"},
			&types.AttributeValueMemberN{Value: "2"},
		}}, dest))
		require.Equal(t, []int64{1, 2}, nums)

		var notSlice int
		destBad := reflect.ValueOf(&notSlice).Elem()
		require.ErrorContains(t, unmarshalAttributeValue(&types.AttributeValueMemberL{Value: []types.AttributeValue{}}, destBad), "non-slice")
	})

	t.Run("map into map and struct", func(t *testing.T) {
		var out map[string]any
		dest := reflect.ValueOf(&out).Elem()
		require.NoError(t, unmarshalAttributeValue(&types.AttributeValueMemberM{Value: map[string]types.AttributeValue{
			"A": &types.AttributeValueMemberS{Value: "x"},
			"B": &types.AttributeValueMemberN{Value: "1"},
		}}, dest))
		require.Equal(t, map[string]any{"A": "x", "B": int64(1)}, out)

		type item struct {
			A string
			B int64
		}
		var st item
		destStruct := reflect.ValueOf(&st).Elem()
		require.NoError(t, unmarshalAttributeValue(&types.AttributeValueMemberM{Value: map[string]types.AttributeValue{
			"A": &types.AttributeValueMemberS{Value: "y"},
			"B": &types.AttributeValueMemberN{Value: "2"},
			"C": &types.AttributeValueMemberS{Value: "ignored"},
		}}, destStruct))
		require.Equal(t, item{A: "y", B: 2}, st)

		var bad map[int]string
		destBad := reflect.ValueOf(&bad).Elem()
		require.Error(t, unmarshalAttributeValue(&types.AttributeValueMemberM{Value: map[string]types.AttributeValue{}}, destBad))

		var defaultNoop string
		destNoop := reflect.ValueOf(&defaultNoop).Elem()
		require.NoError(t, unmarshalAttributeValue(&types.AttributeValueMemberM{Value: map[string]types.AttributeValue{
			"A": &types.AttributeValueMemberS{Value: "z"},
		}}, destNoop))
	})

	t.Run("sets and binary", func(t *testing.T) {
		var ss []string
		destSS := reflect.ValueOf(&ss).Elem()
		require.NoError(t, unmarshalAttributeValue(&types.AttributeValueMemberSS{Value: []string{"a", "b"}}, destSS))
		require.Equal(t, []string{"a", "b"}, ss)

		var ns []int64
		destNS := reflect.ValueOf(&ns).Elem()
		require.NoError(t, unmarshalAttributeValue(&types.AttributeValueMemberNS{Value: []string{"1", "2"}}, destNS))
		require.Equal(t, []int64{1, 2}, ns)

		var bs [][]byte
		destBS := reflect.ValueOf(&bs).Elem()
		require.NoError(t, unmarshalAttributeValue(&types.AttributeValueMemberBS{Value: [][]byte{[]byte("x")}}, destBS))
		require.Len(t, bs, 1)
		require.Equal(t, []byte("x"), bs[0])

		var b []byte
		destB := reflect.ValueOf(&b).Elem()
		require.NoError(t, unmarshalAttributeValue(&types.AttributeValueMemberB{Value: []byte("y")}, destB))
		require.Equal(t, []byte("y"), b)
	})

	t.Run("parseNumberToInterface invalid", func(t *testing.T) {
		_, err := parseNumberToInterface("nope")
		require.Error(t, err)
	})

	t.Run("parseTimeString invalid", func(t *testing.T) {
		_, err := parseTimeString("nope")
		require.Error(t, err)
	})
}
