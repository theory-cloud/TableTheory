package model_test

import (
	"encoding/json"
	"os"
	"reflect"
	"regexp"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	theorydbErrors "github.com/theory-cloud/tabletheory/v4/pkg/errors"
	"github.com/theory-cloud/tabletheory/v4/pkg/model"
)

var vocabularyGoTagPattern = regexp.MustCompile(`theorydb:"([^"]*)"`)

type vocabularyArtifact struct {
	Tags []struct {
		Tag string `json:"tag"`
		Go  string `json:"go"`
	} `json:"tags"`
}

func vocabularyGoTags(t *testing.T) map[string][]string {
	t.Helper()

	data, err := os.ReadFile("../../docs/reference/tabletheory-vocabulary.json")
	require.NoError(t, err)

	var vocab vocabularyArtifact
	require.NoError(t, json.Unmarshal(data, &vocab))

	tags := make(map[string][]string)
	for _, entry := range vocab.Tags {
		for _, match := range vocabularyGoTagPattern.FindAllStringSubmatch(entry.Go, -1) {
			tags[entry.Tag] = append(tags[entry.Tag], match[1])
		}
	}
	return tags
}

func vocabularyIndexModel(t *testing.T, indexTag string) any {
	t.Helper()

	fields := []reflect.StructField{
		{Name: "PK", Type: reflect.TypeOf(""), Tag: `theorydb:"pk"`},
		{Name: "SK", Type: reflect.TypeOf(""), Tag: `theorydb:"sk"`},
		{Name: "IndexKey", Type: reflect.TypeOf(""), Tag: reflect.StructTag(`theorydb:"` + indexTag + `"`)},
	}

	if indexName, ok := strings.CutPrefix(indexTag, "index:"); ok {
		if name, ok := strings.CutSuffix(indexName, ",sk"); ok {
			fields = append(fields, reflect.StructField{
				Name: "IndexPartitionKey",
				Type: reflect.TypeOf(""),
				Tag:  reflect.StructTag(`theorydb:"index:` + name + `,pk"`),
			})
		}
	}

	return reflect.New(reflect.StructOf(fields)).Interface()
}

func TestVocabularyGoIndexTagsRegister(t *testing.T) {
	tags := vocabularyGoTags(t)

	for _, name := range []string{"gsiNpk", "gsiNsk"} {
		t.Run(name, func(t *testing.T) {
			values := tags[name]
			require.NotEmpty(t, values)

			for _, indexTag := range values {
				err := model.NewRegistry().Register(vocabularyIndexModel(t, indexTag))
				require.NoError(t, err, "index tag %q should register", indexTag)
			}
		})
	}
}

func TestVocabularyRejectsLegacyGoIndexTags(t *testing.T) {
	for _, legacyTag := range []string{"gsi1pk", "gsi1sk"} {
		t.Run(legacyTag, func(t *testing.T) {
			err := model.NewRegistry().Register(vocabularyIndexModel(t, legacyTag))
			require.ErrorIs(t, err, theorydbErrors.ErrInvalidTag)
		})
	}
}
