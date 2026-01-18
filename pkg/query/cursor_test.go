package query

import (
	"encoding/base64"
	"encoding/json"
	"testing"

	"github.com/aws/aws-sdk-go-v2/service/dynamodb/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestEncodeCursor(t *testing.T) {
	tests := []struct {
		name          string
		lastKey       map[string]types.AttributeValue
		indexName     string
		sortDirection string
		wantEmpty     bool
		wantErr       bool
	}{
		{
			name:      "empty last key returns empty string",
			lastKey:   map[string]types.AttributeValue{},
			indexName: "",
			wantEmpty: true,
		},
		{
			name: "simple string key",
			lastKey: map[string]types.AttributeValue{
				"id": &types.AttributeValueMemberS{Value: "user123"},
			},
			indexName:     "primary",
			sortDirection: "ASC",
			wantEmpty:     false,
		},
		{
			name: "composite key with multiple types",
			lastKey: map[string]types.AttributeValue{
				"pk":        &types.AttributeValueMemberS{Value: "USER#123"},
				"sk":        &types.AttributeValueMemberS{Value: "PROFILE#2024"},
				"timestamp": &types.AttributeValueMemberN{Value: "1234567890"},
			},
			indexName:     "gsi-1",
			sortDirection: "DESC",
			wantEmpty:     false,
		},
		{
			name: "all supported attribute types",
			lastKey: map[string]types.AttributeValue{
				"string":    &types.AttributeValueMemberS{Value: "test"},
				"number":    &types.AttributeValueMemberN{Value: "123.45"},
				"binary":    &types.AttributeValueMemberB{Value: []byte("binary data")},
				"bool":      &types.AttributeValueMemberBOOL{Value: true},
				"null":      &types.AttributeValueMemberNULL{Value: true},
				"stringSet": &types.AttributeValueMemberSS{Value: []string{"a", "b", "c"}},
				"numberSet": &types.AttributeValueMemberNS{Value: []string{"1", "2", "3"}},
				"binarySet": &types.AttributeValueMemberBS{Value: [][]byte{[]byte("data1"), []byte("data2")}},
				"list": &types.AttributeValueMemberL{Value: []types.AttributeValue{
					&types.AttributeValueMemberS{Value: "item1"},
					&types.AttributeValueMemberN{Value: "42"},
				}},
				"map": &types.AttributeValueMemberM{Value: map[string]types.AttributeValue{
					"nested": &types.AttributeValueMemberS{Value: "value"},
				}},
			},
			indexName: "complex-index",
			wantEmpty: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			encoded, err := EncodeCursor(tt.lastKey, tt.indexName, tt.sortDirection)

			if tt.wantErr {
				assert.Error(t, err)
				return
			}

			require.NoError(t, err)

			if tt.wantEmpty {
				assert.Empty(t, encoded)
				return
			}

			// Verify it's valid base64
			_, err = base64.URLEncoding.DecodeString(encoded)
			require.NoError(t, err)

			// Verify we can decode it back
			cursor, err := DecodeCursor(encoded)
			require.NoError(t, err)
			assert.Equal(t, tt.indexName, cursor.IndexName)
			assert.Equal(t, tt.sortDirection, cursor.SortDirection)
			assert.NotEmpty(t, cursor.LastEvaluatedKey)
		})
	}
}

func TestDecodeCursor(t *testing.T) {
	tests := []struct {
		wantCursor *Cursor
		name       string
		encoded    string
		wantNil    bool
		wantErr    bool
	}{
		{
			name:    "empty string returns nil",
			encoded: "",
			wantNil: true,
		},
		{
			name:    "invalid base64",
			encoded: "not-valid-base64!@#$",
			wantErr: true,
		},
		{
			name:    "invalid JSON",
			encoded: base64.URLEncoding.EncodeToString([]byte("not json")),
			wantErr: true,
		},
		{
			name:    "valid cursor",
			encoded: createValidCursor(t),
			wantCursor: &Cursor{
				LastEvaluatedKey: map[string]any{
					"id": map[string]any{"S": "test123"},
				},
				IndexName:     "test-index",
				SortDirection: "ASC",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cursor, err := DecodeCursor(tt.encoded)

			if tt.wantErr {
				assert.Error(t, err)
				return
			}

			require.NoError(t, err)

			if tt.wantNil {
				assert.Nil(t, cursor)
				return
			}

			assert.Equal(t, tt.wantCursor.IndexName, cursor.IndexName)
			assert.Equal(t, tt.wantCursor.SortDirection, cursor.SortDirection)
		})
	}
}

func TestCursor_ToAttributeValues(t *testing.T) {
	tests := []struct {
		cursor  *Cursor
		name    string
		wantNil bool
		wantErr bool
	}{
		{
			name:    "nil cursor returns nil",
			cursor:  nil,
			wantNil: true,
		},
		{
			name: "empty LastEvaluatedKey returns nil",
			cursor: &Cursor{
				LastEvaluatedKey: map[string]any{},
			},
			wantNil: true,
		},
		{
			name: "simple string attribute",
			cursor: &Cursor{
				LastEvaluatedKey: map[string]any{
					"id": map[string]any{"S": "user123"},
				},
			},
		},
		{
			name: "multiple attribute types",
			cursor: &Cursor{
				LastEvaluatedKey: map[string]any{
					"pk":     map[string]any{"S": "USER#123"},
					"sk":     map[string]any{"S": "PROFILE"},
					"count":  map[string]any{"N": "42"},
					"active": map[string]any{"BOOL": true},
				},
			},
		},
		{
			name: "invalid attribute format",
			cursor: &Cursor{
				LastEvaluatedKey: map[string]any{
					"invalid": "not a map",
				},
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			avs, err := tt.cursor.ToAttributeValues()

			if tt.wantErr {
				assert.Error(t, err)
				return
			}

			require.NoError(t, err)

			if tt.wantNil {
				assert.Nil(t, avs)
				return
			}

			assert.NotEmpty(t, avs)

			// Verify the attributes were converted correctly
			if tt.cursor.LastEvaluatedKey["id"] != nil {
				assert.IsType(t, &types.AttributeValueMemberS{}, avs["id"])
			}
		})
	}
}

func TestAttributeValueToJSON(t *testing.T) {
	tests := []struct {
		av      types.AttributeValue
		want    any
		name    string
		wantErr bool
	}{
		{
			name: "string value",
			av:   &types.AttributeValueMemberS{Value: "test"},
			want: map[string]any{"S": "test"},
		},
		{
			name: "number value",
			av:   &types.AttributeValueMemberN{Value: "123.45"},
			want: map[string]any{"N": "123.45"},
		},
		{
			name: "binary value",
			av:   &types.AttributeValueMemberB{Value: []byte("binary")},
			want: map[string]any{"B": base64.StdEncoding.EncodeToString([]byte("binary"))},
		},
		{
			name: "boolean value",
			av:   &types.AttributeValueMemberBOOL{Value: true},
			want: map[string]any{"BOOL": true},
		},
		{
			name: "null value",
			av:   &types.AttributeValueMemberNULL{Value: true},
			want: map[string]any{"NULL": true},
		},
		{
			name: "string set",
			av:   &types.AttributeValueMemberSS{Value: []string{"a", "b", "c"}},
			want: map[string]any{"SS": []string{"a", "b", "c"}},
		},
		{
			name: "number set",
			av:   &types.AttributeValueMemberNS{Value: []string{"1", "2", "3"}},
			want: map[string]any{"NS": []string{"1", "2", "3"}},
		},
		{
			name: "list with mixed types",
			av: &types.AttributeValueMemberL{Value: []types.AttributeValue{
				&types.AttributeValueMemberS{Value: "string"},
				&types.AttributeValueMemberN{Value: "42"},
				&types.AttributeValueMemberBOOL{Value: false},
			}},
		},
		{
			name: "nested map",
			av: &types.AttributeValueMemberM{Value: map[string]types.AttributeValue{
				"field1": &types.AttributeValueMemberS{Value: "value1"},
				"field2": &types.AttributeValueMemberN{Value: "99"},
			}},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := attributeValueToJSON(tt.av)

			if tt.wantErr {
				assert.Error(t, err)
				return
			}

			require.NoError(t, err)

			if tt.want != nil {
				assert.Equal(t, tt.want, result)
			} else {
				assert.NotNil(t, result)
			}
		})
	}
}

func TestJSONToAttributeValue(t *testing.T) {
	tests := []struct {
		json    any
		check   func(t *testing.T, av types.AttributeValue)
		name    string
		wantErr bool
	}{
		{
			name: "string value",
			json: map[string]any{"S": "test"},
			check: func(t *testing.T, av types.AttributeValue) {
				s, ok := av.(*types.AttributeValueMemberS)
				require.True(t, ok)
				assert.Equal(t, "test", s.Value)
			},
		},
		{
			name: "number value",
			json: map[string]any{"N": "123.45"},
			check: func(t *testing.T, av types.AttributeValue) {
				n, ok := av.(*types.AttributeValueMemberN)
				require.True(t, ok)
				assert.Equal(t, "123.45", n.Value)
			},
		},
		{
			name: "binary value",
			json: map[string]any{"B": base64.StdEncoding.EncodeToString([]byte("binary"))},
			check: func(t *testing.T, av types.AttributeValue) {
				b, ok := av.(*types.AttributeValueMemberB)
				require.True(t, ok)
				assert.Equal(t, []byte("binary"), b.Value)
			},
		},
		{
			name: "boolean value",
			json: map[string]any{"BOOL": true},
			check: func(t *testing.T, av types.AttributeValue) {
				b, ok := av.(*types.AttributeValueMemberBOOL)
				require.True(t, ok)
				assert.True(t, b.Value)
			},
		},
		{
			name: "null value",
			json: map[string]any{"NULL": true},
			check: func(t *testing.T, av types.AttributeValue) {
				_, ok := av.(*types.AttributeValueMemberNULL)
				require.True(t, ok)
			},
		},
		{
			name:    "not a map",
			json:    "invalid",
			wantErr: true,
		},
		{
			name:    "unknown type",
			json:    map[string]any{"UNKNOWN": "value"},
			wantErr: true,
		},
		{
			name:    "invalid string value",
			json:    map[string]any{"S": 123},
			wantErr: true,
		},
		{
			name:    "invalid binary encoding",
			json:    map[string]any{"B": "not-base64!"},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			av, err := jsonToAttributeValue(tt.json)

			if tt.wantErr {
				assert.Error(t, err)
				return
			}

			require.NoError(t, err)
			require.NotNil(t, av)

			if tt.check != nil {
				tt.check(t, av)
			}
		})
	}
}

func TestEncodeDecode_RoundTrip(t *testing.T) {
	// Test that encoding and then decoding produces the same result
	originalKey := map[string]types.AttributeValue{
		"pk":        &types.AttributeValueMemberS{Value: "USER#123"},
		"sk":        &types.AttributeValueMemberS{Value: "PROFILE#2024"},
		"timestamp": &types.AttributeValueMemberN{Value: "1640995200"},
		"active":    &types.AttributeValueMemberBOOL{Value: true},
		"tags":      &types.AttributeValueMemberSS{Value: []string{"tag1", "tag2", "tag3"}},
		"metadata": &types.AttributeValueMemberM{Value: map[string]types.AttributeValue{
			"version": &types.AttributeValueMemberN{Value: "2"},
			"author":  &types.AttributeValueMemberS{Value: "system"},
		}},
	}

	// Encode
	encoded, err := EncodeCursor(originalKey, "test-index", "DESC")
	require.NoError(t, err)
	require.NotEmpty(t, encoded)

	// Decode
	cursor, err := DecodeCursor(encoded)
	require.NoError(t, err)
	require.NotNil(t, cursor)
	assert.Equal(t, "test-index", cursor.IndexName)
	assert.Equal(t, "DESC", cursor.SortDirection)

	// Convert back to AttributeValues
	resultKey, err := cursor.ToAttributeValues()
	require.NoError(t, err)
	require.NotNil(t, resultKey)

	// Verify all keys are present
	assert.Len(t, resultKey, len(originalKey))

	// Verify string values
	pkResult, ok := resultKey["pk"].(*types.AttributeValueMemberS)
	require.True(t, ok)
	pkOriginal, ok := originalKey["pk"].(*types.AttributeValueMemberS)
	require.True(t, ok)
	assert.Equal(t, pkOriginal.Value, pkResult.Value)

	// Verify number values
	tsResult, ok := resultKey["timestamp"].(*types.AttributeValueMemberN)
	require.True(t, ok)
	tsOriginal, ok := originalKey["timestamp"].(*types.AttributeValueMemberN)
	require.True(t, ok)
	assert.Equal(t, tsOriginal.Value, tsResult.Value)

	// Verify boolean values
	activeResult, ok := resultKey["active"].(*types.AttributeValueMemberBOOL)
	require.True(t, ok)
	activeOriginal, ok := originalKey["active"].(*types.AttributeValueMemberBOOL)
	require.True(t, ok)
	assert.Equal(t, activeOriginal.Value, activeResult.Value)
}

// Helper function to create a valid cursor for testing
func createValidCursor(t *testing.T) string {
	cursor := Cursor{
		LastEvaluatedKey: map[string]any{
			"id": map[string]any{"S": "test123"},
		},
		IndexName:     "test-index",
		SortDirection: "ASC",
	}

	data, err := json.Marshal(cursor)
	require.NoError(t, err)

	return base64.URLEncoding.EncodeToString(data)
}
