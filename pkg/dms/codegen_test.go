package dms

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/theory-cloud/tabletheory/pkg/dms/internal/codegenfixture"
	"github.com/theory-cloud/tabletheory/pkg/model"
)

func TestGenerateGoGoldenAndEquivalence(t *testing.T) {
	t.Parallel()

	doc := readCodegenFixture(t)
	got, err := Generate(doc, GenerateOptions{Lang: "go", PackageName: "codegenfixture"})
	require.NoError(t, err)
	want := readRepoFile(t, "internal", "codegenfixture", "dms_note.go")
	require.Equal(t, string(want), string(got))

	wantModel, ok := FindModel(doc, "DMSNote")
	require.True(t, ok)
	registry := model.NewRegistry()
	require.NoError(t, registry.Register(codegenfixture.DMSNote{}))
	meta, err := registry.GetMetadata(codegenfixture.DMSNote{})
	require.NoError(t, err)
	generatedModel, err := FromMetadata(meta)
	require.NoError(t, err)
	require.NoError(t, AssertModelsEquivalent(generatedModel, *wantModel, CompareOptions{}))
}

func TestGeneratedGoEquivalenceDetectsSpecCodeDrift(t *testing.T) {
	t.Parallel()

	doc := readCodegenFixture(t)
	wantModel, ok := FindModel(doc, "DMSNote")
	require.True(t, ok)

	registry := model.NewRegistry()
	require.NoError(t, registry.Register(codegenfixture.DMSNote{}))
	meta, err := registry.GetMetadata(codegenfixture.DMSNote{})
	require.NoError(t, err)
	generatedModel, err := FromMetadata(meta)
	require.NoError(t, err)

	drifted := generatedModel
	drifted.WritePolicy.ProtectedAttributes = []string{"count"}
	err = AssertModelsEquivalent(drifted, *wantModel, CompareOptions{})
	require.Error(t, err)
	require.Contains(t, err.Error(), "models not equivalent")
	require.Contains(t, err.Error(), "protected_attributes")
}

func TestGenerateTypeScriptGolden(t *testing.T) {
	t.Parallel()

	doc := readCodegenFixture(t)
	got, err := Generate(doc, GenerateOptions{Lang: "ts", RuntimeImport: "../../../src/index.js"})
	require.NoError(t, err)
	want := readRepoFile(t, "..", "..", "ts", "test", "fixtures", "dms-codegen", "generated-dms-note.ts")
	require.Equal(t, string(want), string(got))
}

func TestGeneratePythonGolden(t *testing.T) {
	t.Parallel()

	doc := readCodegenFixture(t)
	got, err := Generate(doc, GenerateOptions{Lang: "py"})
	require.NoError(t, err)
	want := readRepoFile(t, "..", "..", "py", "tests", "fixtures", "dms_codegen", "generated_dms_note.py")
	require.Equal(t, string(want), string(got))
}

func TestGenerateCDKGolden(t *testing.T) {
	t.Parallel()

	doc := readCodegenFixture(t)
	got, err := Generate(doc, GenerateOptions{Lang: "cdk"})
	require.NoError(t, err)
	want := readRepoFile(t, "testdata", "codegen", "dms-note.cdk.ts")
	require.Equal(t, string(want), string(got))

	// The emitted construct must reflect the DMS table/GSI/TTL shape.
	rendered := string(got)
	require.Contains(t, rendered, "tableName: 'dms_notes'")
	require.Contains(t, rendered, "partitionKey: { name: 'PK', type: dynamodb.AttributeType.STRING }")
	require.Contains(t, rendered, "timeToLiveAttribute: 'ttl'")
	require.Contains(t, rendered, "addGlobalSecondaryIndex")
	require.Contains(t, rendered, "projectionType: dynamodb.ProjectionType.INCLUDE")
	require.Contains(t, rendered, "nonKeyAttributes: ['count', 'payload']")

	// Generation must be deterministic for drift-gating.
	second, err := Generate(doc, GenerateOptions{Lang: "cdk"})
	require.NoError(t, err)
	require.True(t, bytes.Equal(got, second))
}

func TestGenerateCDKRejectsUnsupportedKeyType(t *testing.T) {
	t.Parallel()

	doc := &Document{
		DMSVersion: "0.1",
		Models: []Model{
			{
				Name:  "Bad",
				Table: Table{Name: "bad"},
				Keys:  Keys{Partition: KeyAttribute{Attribute: "PK", Type: "BOOL"}},
				Attributes: []Attribute{
					{Attribute: "PK", Type: "BOOL", Roles: []string{"pk"}},
				},
			},
		},
	}
	_, err := Generate(doc, GenerateOptions{Lang: "cdk"})
	require.ErrorContains(t, err, "unsupported key attribute type")
}

func TestGenerateCDKCoversIndexProjectionBranches_THE2551(t *testing.T) {
	t.Parallel()

	doc := &Document{
		DMSVersion: "0.1",
		Models: []Model{
			{
				Name:  "Branchy",
				Table: Table{Name: "branchy"},
				Keys: Keys{
					Partition: KeyAttribute{Attribute: "PK", Type: "S"},
					Sort:      &KeyAttribute{Attribute: "SK", Type: "N"},
				},
				Attributes: []Attribute{
					{Attribute: "PK", Type: "S", Roles: []string{"pk"}},
					{Attribute: "SK", Type: "N", Roles: []string{"sk"}},
					{Attribute: "blob", Type: "B", Roles: []string{"gsi1pk"}},
					{Attribute: "score", Type: "N", Roles: []string{"lsi:by-score,sk"}},
					{Attribute: "summary", Type: "S"},
				},
				Indexes: []Index{
					{
						Name:       "by-blob",
						Type:       "GSI",
						Partition:  KeyAttribute{Attribute: "blob", Type: "B"},
						Projection: Projection{Type: "KEYS_ONLY"},
					},
					{
						Name:       "by-score",
						Type:       indexTypeLSI,
						Partition:  KeyAttribute{Attribute: "PK", Type: "S"},
						Sort:       &KeyAttribute{Attribute: "score", Type: "N"},
						Projection: Projection{Type: projectionInclude, Fields: []string{"summary"}},
					},
				},
			},
		},
	}

	got, err := Generate(doc, GenerateOptions{Lang: "cdk"})
	require.NoError(t, err)
	rendered := string(got)
	require.Contains(t, rendered, "sortKey: { name: 'SK', type: dynamodb.AttributeType.NUMBER }")
	require.Contains(t, rendered, "partitionKey: { name: 'blob', type: dynamodb.AttributeType.BINARY }")
	require.Contains(t, rendered, "projectionType: dynamodb.ProjectionType.KEYS_ONLY")
	require.Contains(t, rendered, "table.addLocalSecondaryIndex")
	require.Contains(t, rendered, "sortKey: { name: 'score', type: dynamodb.AttributeType.NUMBER }")
	require.Contains(t, rendered, "projectionType: dynamodb.ProjectionType.INCLUDE")
	require.Contains(t, rendered, "nonKeyAttributes: ['summary']")
	require.NotContains(t, rendered, "timeToLiveAttribute")
}

func TestGenerateCDKRejectsUnsupportedIndexKeyTypes_THE2551(t *testing.T) {
	t.Parallel()

	base := Model{
		Name:  "BadIndex",
		Table: Table{Name: "bad_index"},
		Keys:  Keys{Partition: KeyAttribute{Attribute: "PK", Type: "S"}},
		Attributes: []Attribute{
			{Attribute: "PK", Type: "S", Roles: []string{"pk"}},
		},
	}

	gsi := base
	gsi.Indexes = []Index{{
		Name:      "bad-gsi",
		Type:      "GSI",
		Partition: KeyAttribute{Attribute: "bad", Type: "BOOL"},
	}}
	_, err := Generate(&Document{DMSVersion: "0.1", Models: []Model{gsi}}, GenerateOptions{Lang: "cdk"})
	require.ErrorContains(t, err, "index bad-gsi partition key")

	lsi := base
	lsi.Indexes = []Index{{
		Name:      "bad-lsi",
		Type:      indexTypeLSI,
		Partition: KeyAttribute{Attribute: "PK", Type: "S"},
		Sort:      &KeyAttribute{Attribute: "bad", Type: "BOOL"},
	}}
	_, err = Generate(&Document{DMSVersion: "0.1", Models: []Model{lsi}}, GenerateOptions{Lang: "cdk"})
	require.ErrorContains(t, err, "index bad-lsi sort key")
}

func TestGenerateOptionsAndErrors(t *testing.T) {
	t.Parallel()

	doc := readCodegenFixture(t)
	one, err := Generate(doc, GenerateOptions{Lang: "ts", ModelName: "DMSNote"})
	require.NoError(t, err)
	require.Contains(t, string(one), "DMSNoteSchema")

	_, err = Generate(doc, GenerateOptions{Lang: "ts", ModelName: "Missing"})
	require.ErrorContains(t, err, "DMS model not found")

	_, err = Generate(doc, GenerateOptions{Lang: "ruby"})
	require.ErrorContains(t, err, "unsupported generation language")

	_, err = Generate(nil, GenerateOptions{Lang: "go"})
	require.ErrorContains(t, err, "DMS document is nil")

	_, err = Generate(&Document{}, GenerateOptions{Lang: "go"})
	require.ErrorContains(t, err, "DMS document has no models")

	_, err = Generate(doc, GenerateOptions{Lang: "go", PackageName: "Models"})
	require.ErrorContains(t, err, "invalid Go package name")
}

func TestGeneratedOutputDeterministic(t *testing.T) {
	t.Parallel()

	doc := readCodegenFixture(t)
	first, err := Generate(doc, GenerateOptions{Lang: "ts"})
	require.NoError(t, err)
	second, err := Generate(doc, GenerateOptions{Lang: "ts"})
	require.NoError(t, err)
	require.True(t, bytes.Equal(first, second))
}

func TestGenerateCoversProjectionAndNamingBranches(t *testing.T) {
	t.Parallel()

	doc := readCodegenFixture(t)
	model := doc.Models[0]
	model.Naming = Naming{Convention: namingConventionSnakeCase}
	model.Indexes = []Index{
		{
			Name:      "gsi-title",
			Type:      "GSI",
			Partition: KeyAttribute{Attribute: "title", Type: "S"},
			Projection: Projection{
				Type: "KEYS_ONLY",
			},
		},
		{
			Name:      "lsi-count",
			Type:      indexTypeLSI,
			Partition: KeyAttribute{Attribute: "PK", Type: "S"},
			Sort:      &KeyAttribute{Attribute: "count", Type: "N"},
			Projection: Projection{
				Type:   "INCLUDE",
				Fields: []string{"secret"},
			},
		},
	}
	variant := &Document{DMSVersion: doc.DMSVersion, Models: []Model{model}}

	goOut, err := Generate(variant, GenerateOptions{Lang: "go"})
	require.NoError(t, err)
	require.Contains(t, string(goOut), "theorydb:\"naming:snake_case\"")
	require.Contains(t, string(goOut), "lsi:lsi-count,pk")
	require.Contains(t, string(goOut), "lsi:lsi-count,sk")
	require.Contains(t, string(goOut), "Type: \"KEYS_ONLY\"")

	tsOut, err := Generate(variant, GenerateOptions{Lang: "ts", RuntimeImport: "runtime'\nmodule"})
	require.NoError(t, err)
	require.Contains(t, string(tsOut), "runtime\\'\\nmodule")
	require.Contains(t, string(tsOut), "type: 'KEYS_ONLY'")

	pyOut, err := Generate(variant, GenerateOptions{Lang: "py"})
	require.NoError(t, err)
	require.Contains(t, string(pyOut), "Projection.keys_only()")
	require.Contains(t, string(pyOut), "Projection.include(\"secret\")")
	require.Contains(t, string(pyOut), "lsi(")
}

func TestGenerateRejectsUnsupportedAttributeTypes(t *testing.T) {
	t.Parallel()

	doc := readCodegenFixture(t)
	model := doc.Models[0]
	model.Attributes = append([]Attribute(nil), model.Attributes...)
	model.Attributes[0].Type = "Z"
	bad := &Document{DMSVersion: doc.DMSVersion, Models: []Model{model}}

	_, err := Generate(bad, GenerateOptions{Lang: "go"})
	require.ErrorContains(t, err, `unsupported DMS attribute type "Z"`)

	_, err = Generate(bad, GenerateOptions{Lang: "py"})
	require.ErrorContains(t, err, `unsupported DMS attribute type "Z"`)
}

func TestGeneratePythonEmptyModel(t *testing.T) {
	t.Parallel()

	doc := &Document{
		DMSVersion: "0.1",
		Models: []Model{{
			Name:  "empty_model",
			Table: Table{Name: "empty_models"},
		}},
	}
	got, err := Generate(doc, GenerateOptions{Lang: "py"})
	require.NoError(t, err)
	require.Contains(t, string(got), "class EmptyModel:")
	require.Contains(t, string(got), "    pass")
	require.NotContains(t, string(got), "Projection")
	require.NotContains(t, string(got), "WritePolicy")
}

func TestCodegenHelperEdges(t *testing.T) {
	t.Parallel()

	imports := map[string]struct{}{}
	typ, err := goTypeForAttribute(Attribute{Type: attributeTypeBool}, imports)
	require.NoError(t, err)
	require.Equal(t, "bool", typ)
	typ, err = goTypeForAttribute(Attribute{Type: "L"}, imports)
	require.NoError(t, err)
	require.Equal(t, "[]any", typ)
	typ, err = goTypeForAttribute(Attribute{Type: attributeTypeNull}, imports)
	require.NoError(t, err)
	require.Equal(t, "any", typ)

	pyType, pyDefault, err := pyTypeAndDefault(Attribute{Type: "N", Required: true})
	require.NoError(t, err)
	require.Equal(t, "int", pyType)
	require.Empty(t, pyDefault)

	pyType, pyDefault, err = pyTypeAndDefault(Attribute{Type: "N", Required: true, Roles: []string{"pk"}})
	require.NoError(t, err)
	require.Equal(t, "int", pyType)
	require.Empty(t, pyDefault)

	std, thirdParty := partitionGoImports([]string{"time", "github.com/theory-cloud/tabletheory/pkg/model"})
	require.Equal(t, []string{"time"}, std)
	require.Equal(t, []string{"github.com/theory-cloud/tabletheory/pkg/model"}, thirdParty)

	require.Equal(t, "'quote\\' newline\\n tab\\t slash\\\\'", tsString("quote' newline\n tab\t slash\\"))
	require.Equal(t, "Field", uniqueGoFieldName("", map[string]int{}))
	require.Equal(t, "Field9bad", uniqueGoFieldName("9bad", map[string]int{}))
	require.Equal(t, "field_class", snakeIdentifier("class"))
	require.Equal(t, "field", snakeIdentifier("!!!"))
	require.Equal(t, "ID", exportName("id"))
	require.Empty(t, exportName("!!!"))
}

func readCodegenFixture(t *testing.T) *Document {
	t.Helper()
	data := readRepoFile(t, "testdata", "codegen", "dms-note.yml")
	doc, err := ParseDocument(data)
	require.NoError(t, err)
	return doc
}

func readRepoFile(t *testing.T, parts ...string) []byte {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(parts...)) // #nosec G304 -- tests read fixed repository fixtures.
	require.NoError(t, err)
	return data
}
