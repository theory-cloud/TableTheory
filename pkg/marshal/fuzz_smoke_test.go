package marshal

import (
	"encoding/binary"
	"sync"
	"testing"

	"github.com/theory-cloud/tabletheory/pkg/model"
)

type fuzzMarshalModel struct {
	PK string `theorydb:"pk" json:"pk"`
	SK string `theorydb:"sk" json:"sk"`

	Str   string            `theorydb:"omitempty" json:"str,omitempty"`
	Tags  []string          `theorydb:"omitempty" json:"tags,omitempty"`
	Attrs map[string]string `theorydb:"omitempty" json:"attrs,omitempty"`
	Blob  []byte            `theorydb:"omitempty" json:"blob,omitempty"`
	Num   int64             `theorydb:"omitempty" json:"num,omitempty"`
}

var (
	fuzzMarshalMetaOnce sync.Once
	fuzzMarshalMeta     *model.Metadata
	fuzzMarshalMetaErr  error
)

func fuzzMarshalMetadata() (*model.Metadata, error) {
	fuzzMarshalMetaOnce.Do(func() {
		registry := model.NewRegistry()
		if err := registry.Register(fuzzMarshalModel{}); err != nil {
			fuzzMarshalMetaErr = err
			return
		}
		fuzzMarshalMeta, fuzzMarshalMetaErr = registry.GetMetadata(fuzzMarshalModel{})
	})
	return fuzzMarshalMeta, fuzzMarshalMetaErr
}

func FuzzMarshaler_SafeAndUnsafe_NoPanics(f *testing.F) {
	f.Add("pk", "sk", []byte("blob"), uint8(0))
	f.Add("p", "s", []byte{0, 1, 2, 3, 4, 5, 6, 7}, uint8(3))

	f.Fuzz(func(t *testing.T, pk string, sk string, raw []byte, mode uint8) {
		meta, err := fuzzMarshalMetadata()
		if err != nil {
			t.Fatalf("failed to build metadata: %v", err)
		}

		const maxBytes = 512
		if len(raw) > maxBytes {
			raw = raw[:maxBytes]
		}

		u := binary.LittleEndian.Uint32(padTo4(raw))
		magnitude := int64(u & 0x7fffffff)
		if u&0x80000000 != 0 {
			magnitude = -magnitude
		}

		item := fuzzMarshalModel{
			PK:   truncateString(pk, 128),
			SK:   truncateString(sk, 128),
			Str:  truncateString(string(raw), 256),
			Num:  magnitude,
			Blob: append([]byte(nil), raw...),
			Tags: []string{truncateString(string(raw), 32), truncateString(string(raw), 32)},
			Attrs: map[string]string{
				"k": truncateString(string(raw), 64),
			},
		}

		safe := NewSafeMarshaler()
		if _, err := safe.MarshalItem(item, meta); err != nil {
			_ = err
		}

		if mode%2 == 0 {
			unsafeMarshaler := New(nil)
			if _, err := unsafeMarshaler.MarshalItem(item, meta); err != nil {
				_ = err
			}
		}
	})
}

func truncateString(s string, max int) string {
	if max <= 0 || len(s) <= max {
		return s
	}
	// Copy the truncated bytes so fuzzing doesn't retain a reference to a huge input string.
	return string(append([]byte(nil), s[:max]...))
}

func padTo4(b []byte) []byte {
	var out [4]byte
	copy(out[:], b)
	return out[:]
}
