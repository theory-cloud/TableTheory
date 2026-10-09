package dms

import (
	"fmt"
	"go/parser"
	"go/token"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestValidateDMSNameAllowlist(t *testing.T) {
	t.Parallel()

	allowed := []string{
		"gsi-slug",
		"created_at",
		"PK",
		"emailHash",
		"v1.2.3",
		"a:b#c-d.e_f",
		"Ünïcode",
		"日本語",
	}
	for _, name := range allowed {
		require.NoError(t, validateDMSName("attribute", name), "name %q should be allowed", name)
	}

	rejected := []struct {
		name string
		want string
	}{
		{name: "a`b", want: "disallowed character '`'"},
		{name: `a"b`, want: `disallowed character '"'`},
		{name: `a\b`, want: `disallowed character '\\'`},
		{name: "a,b", want: "disallowed character ','"},
		{name: "a\nb", want: `disallowed character '\n'`},
		{name: "a\tb", want: `disallowed character '\t'`},
		{name: "a\x00b", want: `disallowed character '\x00'`},
		{name: "a\x07b", want: `disallowed character '\a'`},
		{name: "a b", want: "disallowed character ' '"},
	}
	for _, tc := range rejected {
		err := validateDMSName("attribute", tc.name)
		require.Error(t, err, "name %q should be rejected", tc.name)
		require.Contains(t, err.Error(), tc.want)
	}

	require.ErrorContains(t, validateDMSName("index", string([]byte{0xff, 0xfe})), "not valid UTF-8")

	// Blank names are handled (and rejected) by model-level validation, not here.
	require.NoError(t, validateDMSName("attribute", ""))
}

func TestGenerateGoRejectsUnsafeNames(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name   string
		mutate func(*Model)
		want   string
	}{
		{
			name: "unsafe_index_name",
			mutate: func(m *Model) {
				m.Indexes[0].Name = "gsi-title`}\nvar leaked = true\nfunc init() { leaked = true }\nvar _ = `"
			},
			want: "index name",
		},
		{
			name:   "attribute_backtick",
			mutate: func(m *Model) { m.Attributes[2].Attribute = "ti`tle" },
			want:   "attribute name",
		},
		{
			name:   "attribute_newline",
			mutate: func(m *Model) { m.Attributes[2].Attribute = "ti\ntle" },
			want:   "attribute name",
		},
		{
			name:   "attribute_tab",
			mutate: func(m *Model) { m.Attributes[2].Attribute = "ti\ttle" },
			want:   "attribute name",
		},
		{
			name:   "attribute_nul",
			mutate: func(m *Model) { m.Attributes[2].Attribute = "ti\x00tle" },
			want:   "attribute name",
		},
		{
			name:   "attribute_control",
			mutate: func(m *Model) { m.Attributes[2].Attribute = "ti\x07tle" },
			want:   "attribute name",
		},
		{
			name:   "attribute_double_quote",
			mutate: func(m *Model) { m.Attributes[2].Attribute = `ti"tle` },
			want:   "attribute name",
		},
		{
			name:   "attribute_backslash",
			mutate: func(m *Model) { m.Attributes[2].Attribute = `ti\tle` },
			want:   "attribute name",
		},
		{
			name:   "attribute_comma",
			mutate: func(m *Model) { m.Attributes[2].Attribute = "ti,tle" },
			want:   "attribute name",
		},
		{
			name:   "attribute_invalid_utf8",
			mutate: func(m *Model) { m.Attributes[2].Attribute = string([]byte{0xff, 0xfe}) },
			want:   "valid UTF-8",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			doc := readCodegenFixture(t)
			m, ok := FindModel(doc, "DMSNote")
			require.True(t, ok)
			tc.mutate(m)

			out, err := Generate(doc, GenerateOptions{Lang: "go", PackageName: "codegenfixture"})
			require.Error(t, err)
			require.Contains(t, err.Error(), tc.want)
			// Rejection happens before code emission: nothing is returned and no
			// attacker-controlled text leaked into the (empty) output.
			require.Empty(t, out)
			require.NotContains(t, string(out), "func init")
			require.NotContains(t, string(out), "leaked")
		})
	}
}

func TestGenerateGoAcceptsAuthorizedNames(t *testing.T) {
	t.Parallel()

	doc := &Document{
		DMSVersion: "0.2",
		Models: []Model{{
			Name:  "Note",
			Table: Table{Name: "notes"},
			Keys:  Keys{Partition: KeyAttribute{Attribute: "PK", Type: "S"}},
			Attributes: []Attribute{
				{Attribute: "PK", Type: "S", Roles: []string{"pk"}},
				{Attribute: "created_at", Type: "S", Optional: true},
				{Attribute: "slug", Type: "S", Optional: true},
				{Attribute: "cfg.value#1:2-x_y", Type: "S", Optional: true},
			},
			Indexes: []Index{{
				Name:      "gsi-slug",
				Type:      "GSI",
				Partition: KeyAttribute{Attribute: "slug", Type: "S"},
			}},
		}},
	}

	out, err := Generate(doc, GenerateOptions{Lang: "go", PackageName: "models"})
	require.NoError(t, err)

	require.Contains(t, string(out), "`theorydb:\"attr:slug,index:gsi-slug,pk\" json:\"slug\"`")
	require.Contains(t, string(out), "attr:created_at")
	require.Contains(t, string(out), "attr:cfg.value#1:2-x_y")

	// The generated source must be valid Go.
	fset := token.NewFileSet()
	_, perr := parser.ParseFile(fset, "models.go", out, parser.AllErrors)
	require.NoError(t, perr)
}

func TestGenerateGoGoldenUnaffectedByValidator(t *testing.T) {
	t.Parallel()

	doc := readCodegenFixture(t)
	got, err := Generate(doc, GenerateOptions{Lang: "go", PackageName: "codegenfixture"})
	require.NoError(t, err)
	want := readRepoFile(t, "internal", "codegenfixture", "dms_note.go")
	require.Equal(t, string(want), string(got))
}

func demoDocWithIndex(indexName, partitionAttribute, projectionField string) []byte {
	projection := ""
	if projectionField != "" {
		projection = fmt.Sprintf("\n        projection: { type: \"INCLUDE\", fields: [%q] }", projectionField)
	}
	return []byte(fmt.Sprintf(`
dms_version: "0.2"
models:
  - name: "Demo"
    table: { name: "tbl" }
    keys:
      partition: { attribute: "PK", type: "S" }
    attributes:
      - attribute: "PK"
        type: "S"
        required: true
        roles: ["pk"]
      - attribute: "slug"
        type: "S"
        optional: true
    indexes:
      - name: %q
        type: "GSI"
        partition: { attribute: %q, type: "S" }%s
`, indexName, partitionAttribute, projection))
}

func TestParseDocumentRejectsUnsafeNames(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name string
		want string
		raw  []byte
	}{
		{
			name: "unsafe_index_name",
			raw:  demoDocWithIndex("gsi-`}\nfunc init() {}\nvar _ = `", "slug", ""),
			want: "index name",
		},
		{
			name: "unsafe_index_name_control",
			raw:  demoDocWithIndex("gsi-\nfunc init() {}", "slug", ""),
			want: "index name",
		},
		{
			name: "unsafe_partition_key_attribute",
			raw:  demoDocWithIndex("gsi-slug", "sl`ug", ""),
			want: "partition key attribute",
		},
		{
			name: "unsafe_projection_field",
			raw:  demoDocWithIndex("gsi-slug", "slug", "sl`ug"),
			want: "projection field",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			_, err := ParseDocument(tc.raw)
			require.Error(t, err)
			require.Contains(t, err.Error(), tc.want)
		})
	}
}

func TestParseDocumentAcceptsAuthorizedNames(t *testing.T) {
	t.Parallel()

	doc, err := ParseDocument(demoDocWithIndex("gsi-slug", "slug", "slug"))
	require.NoError(t, err)

	m, ok := FindModel(doc, "Demo")
	require.True(t, ok)
	require.Len(t, m.Indexes, 1)
	require.Equal(t, "gsi-slug", m.Indexes[0].Name)
}

func TestValidateModelIndexesRejectsDuplicatesAndDanglingReferences(t *testing.T) {
	t.Parallel()

	base := Model{
		Name:  "Demo",
		Table: Table{Name: "tbl"},
		Keys:  Keys{Partition: KeyAttribute{Attribute: "PK", Type: "S"}},
		Attributes: []Attribute{
			{Attribute: "PK", Type: "S"},
			{Attribute: "slug", Type: "S"},
		},
	}

	duplicate := base
	duplicate.Indexes = []Index{
		{Name: "gsi-slug", Type: "GSI", Partition: KeyAttribute{Attribute: "slug", Type: "S"}},
		{Name: "gsi-slug", Type: "GSI", Partition: KeyAttribute{Attribute: "PK", Type: "S"}},
	}
	require.ErrorContains(t, validateDocument(&Document{DMSVersion: "0.2", Models: []Model{duplicate}}), "duplicate index")

	danglingKey := base
	danglingKey.Indexes = []Index{
		{Name: "gsi-missing", Type: "GSI", Partition: KeyAttribute{Attribute: "missing", Type: "S"}},
	}
	require.ErrorContains(t, validateDocument(&Document{DMSVersion: "0.2", Models: []Model{danglingKey}}), "key attribute not found")

	danglingProjection := base
	danglingProjection.Indexes = []Index{
		{
			Name:       "gsi-slug",
			Type:       "GSI",
			Partition:  KeyAttribute{Attribute: "slug", Type: "S"},
			Projection: Projection{Type: "INCLUDE", Fields: []string{"missing"}},
		},
	}
	require.ErrorContains(t, validateDocument(&Document{DMSVersion: "0.2", Models: []Model{danglingProjection}}), "projection field not found")

	blankName := base
	blankName.Indexes = []Index{
		{Name: "  ", Type: "GSI", Partition: KeyAttribute{Attribute: "slug", Type: "S"}},
	}
	require.ErrorContains(t, validateDocument(&Document{DMSVersion: "0.2", Models: []Model{blankName}}), "index missing name")
}
