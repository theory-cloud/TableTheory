package marshal

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/theory-cloud/tabletheory/v2/pkg/model"
)

type omitEmptyPointerItem struct {
	AnyFlag  any `json:"anyFlag,omitempty" theorydb:"attr:anyFlag,omitempty"`
	AnyCount any `json:"anyCount,omitempty" theorydb:"attr:anyCount,omitempty"`
	AnyNote  any `json:"anyNote,omitempty" theorydb:"attr:anyNote,omitempty"`
	NilAny   any `json:"nilAny,omitempty" theorydb:"attr:nilAny,omitempty"`

	Flag    *bool   `json:"flag,omitempty" theorydb:"attr:flag,omitempty"`
	Count   *int    `json:"count,omitempty" theorydb:"attr:count,omitempty"`
	Note    *string `json:"note,omitempty" theorydb:"attr:note,omitempty"`
	NilFlag *bool   `json:"nilFlag,omitempty" theorydb:"attr:nilFlag,omitempty"`

	PK        string `json:"PK" theorydb:"pk"`
	SK        string `json:"SK" theorydb:"sk"`
	PlainNote string `json:"plainNote,omitempty" theorydb:"attr:plainNote,omitempty"`

	PlainCount int  `json:"plainCount,omitempty" theorydb:"attr:plainCount,omitempty"`
	PlainFlag  bool `json:"plainFlag,omitempty" theorydb:"attr:plainFlag,omitempty"`
}

func (omitEmptyPointerItem) TableName() string { return "omit_empty_pointer_items" }

func TestMarshalers_OmitEmptyPreservesNonNilPointerAndInterfaceZeroValues(t *testing.T) {
	falseValue := false
	zeroInt := 0
	emptyString := ""

	input := omitEmptyPointerItem{
		PK:       "PK#1",
		SK:       "SK#1",
		Flag:     &falseValue,
		Count:    &zeroInt,
		Note:     &emptyString,
		AnyFlag:  false,
		AnyCount: 0,
		AnyNote:  "",
	}
	metadata := omitEmptyPointerMetadata(t)

	for name, marshaler := range map[string]MarshalerInterface{
		"safe":      NewSafeMarshaler(),
		"optimized": New(nil),
	} {
		t.Run(name, func(t *testing.T) {
			out, err := marshaler.MarshalItem(input, metadata)
			require.NoError(t, err)

			require.False(t, requireAVBOOL(t, out["flag"]).Value)
			require.Equal(t, "0", requireAVN(t, out["count"]).Value)
			require.Equal(t, "", requireAVS(t, out["note"]).Value)

			require.False(t, requireAVBOOL(t, out["anyFlag"]).Value)
			require.Equal(t, "0", requireAVN(t, out["anyCount"]).Value)
			require.Equal(t, "", requireAVS(t, out["anyNote"]).Value)

			require.NotContains(t, out, "nilFlag")
			require.NotContains(t, out, "nilAny")

			require.NotContains(t, out, "plainFlag")
			require.NotContains(t, out, "plainCount")
			require.NotContains(t, out, "plainNote")
		})
	}
}

func omitEmptyPointerMetadata(t *testing.T) *model.Metadata {
	t.Helper()

	registry := model.NewRegistry()
	require.NoError(t, registry.Register(omitEmptyPointerItem{}))
	metadata, err := registry.GetMetadata(omitEmptyPointerItem{})
	require.NoError(t, err)
	return metadata
}
