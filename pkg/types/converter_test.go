package types

import (
	"reflect"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go-v2/service/dynamodb/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestNewConverter tests the converter constructor
func TestNewConverter(t *testing.T) {
	converter := NewConverter()
	assert.NotNil(t, converter)
	assert.NotNil(t, converter.customConverters)
}

// TestToAttributeValue_BasicTypes tests conversion of basic Go types to AttributeValues
func TestToAttributeValue_BasicTypes(t *testing.T) {
	converter := NewConverter()

	tests := []struct {
		input    interface{}
		expected types.AttributeValue
		name     string
		wantErr  bool
	}{
		// String types
		{
			name:     "string",
			input:    "hello world",
			expected: &types.AttributeValueMemberS{Value: "hello world"},
		},
		{
			name:     "empty string",
			input:    "",
			expected: &types.AttributeValueMemberS{Value: ""},
		},
		{
			name:     "unicode string",
			input:    "Hello 世界 🌍",
			expected: &types.AttributeValueMemberS{Value: "Hello 世界 🌍"},
		},

		// Integer types
		{
			name:     "int",
			input:    42,
			expected: &types.AttributeValueMemberN{Value: "42"},
		},
		{
			name:     "negative int",
			input:    -42,
			expected: &types.AttributeValueMemberN{Value: "-42"},
		},
		{
			name:     "int8",
			input:    int8(127),
			expected: &types.AttributeValueMemberN{Value: "127"},
		},
		{
			name:     "int16",
			input:    int16(32767),
			expected: &types.AttributeValueMemberN{Value: "32767"},
		},
		{
			name:     "int32",
			input:    int32(2147483647),
			expected: &types.AttributeValueMemberN{Value: "2147483647"},
		},
		{
			name:     "int64",
			input:    int64(9223372036854775807),
			expected: &types.AttributeValueMemberN{Value: "9223372036854775807"},
		},

		// Unsigned integer types
		{
			name:     "uint",
			input:    uint(42),
			expected: &types.AttributeValueMemberN{Value: "42"},
		},
		{
			name:     "uint8",
			input:    uint8(255),
			expected: &types.AttributeValueMemberN{Value: "255"},
		},
		{
			name:     "uint16",
			input:    uint16(65535),
			expected: &types.AttributeValueMemberN{Value: "65535"},
		},
		{
			name:     "uint32",
			input:    uint32(4294967295),
			expected: &types.AttributeValueMemberN{Value: "4294967295"},
		},
		{
			name:     "uint64",
			input:    uint64(18446744073709551615),
			expected: &types.AttributeValueMemberN{Value: "18446744073709551615"},
		},

		// Float types
		{
			name:     "float32",
			input:    float32(3.14),
			expected: &types.AttributeValueMemberN{Value: "3.140000104904175"}, // float32 precision
		},
		{
			name:     "float64",
			input:    float64(3.14159265359),
			expected: &types.AttributeValueMemberN{Value: "3.14159265359"},
		},
		{
			name:     "float with exponent",
			input:    1.23e-10,
			expected: &types.AttributeValueMemberN{Value: "0.000000000123"},
		},

		// Boolean type
		{
			name:     "bool true",
			input:    true,
			expected: &types.AttributeValueMemberBOOL{Value: true},
		},
		{
			name:     "bool false",
			input:    false,
			expected: &types.AttributeValueMemberBOOL{Value: false},
		},

		// Nil values
		{
			name:     "nil",
			input:    nil,
			expected: &types.AttributeValueMemberNULL{Value: true},
		},
		{
			name:     "nil pointer",
			input:    (*string)(nil),
			expected: &types.AttributeValueMemberNULL{Value: true},
		},

		// Binary type
		{
			name:     "byte slice",
			input:    []byte("hello"),
			expected: &types.AttributeValueMemberB{Value: []byte("hello")},
		},
		{
			name:     "empty byte slice",
			input:    []byte{},
			expected: &types.AttributeValueMemberB{Value: []byte{}},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := converter.ToAttributeValue(tt.input)
			if tt.wantErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
				assert.Equal(t, tt.expected, result)
			}
		})
	}
}

// TestToAttributeValue_TimeType tests time.Time conversion
func TestToAttributeValue_TimeType(t *testing.T) {
	converter := NewConverter()

	testTime := time.Date(2023, 6, 15, 10, 30, 45, 123456789, time.UTC)
	result, err := converter.ToAttributeValue(testTime)

	require.NoError(t, err)
	s, ok := result.(*types.AttributeValueMemberS)
	require.True(t, ok)
	assert.Equal(t, testTime.Format(time.RFC3339Nano), s.Value)

	// Test time pointer
	timePtr := &testTime
	result, err = converter.ToAttributeValue(timePtr)
	require.NoError(t, err)
	s, ok = result.(*types.AttributeValueMemberS)
	require.True(t, ok)
	assert.Equal(t, testTime.Format(time.RFC3339Nano), s.Value)
}

// TestToAttributeValue_ComplexTypes tests conversion of complex types
func TestToAttributeValue_ComplexTypes(t *testing.T) {
	converter := NewConverter()

	t.Run("slice of strings", func(t *testing.T) {
		input := []string{"apple", "banana", "cherry"}
		result, err := converter.ToAttributeValue(input)

		require.NoError(t, err)
		list, ok := result.(*types.AttributeValueMemberL)
		require.True(t, ok)
		assert.Len(t, list.Value, 3)

		// Verify each element
		assert.Equal(t, &types.AttributeValueMemberS{Value: "apple"}, list.Value[0])
		assert.Equal(t, &types.AttributeValueMemberS{Value: "banana"}, list.Value[1])
		assert.Equal(t, &types.AttributeValueMemberS{Value: "cherry"}, list.Value[2])
	})

	t.Run("slice of integers", func(t *testing.T) {
		input := []int{1, 2, 3, 4, 5}
		result, err := converter.ToAttributeValue(input)

		require.NoError(t, err)
		list, ok := result.(*types.AttributeValueMemberL)
		require.True(t, ok)
		assert.Len(t, list.Value, 5)
	})

	t.Run("nested slices", func(t *testing.T) {
		input := [][]string{
			{"a", "b"},
			{"c", "d", "e"},
		}
		result, err := converter.ToAttributeValue(input)

		require.NoError(t, err)
		list, ok := result.(*types.AttributeValueMemberL)
		require.True(t, ok)
		assert.Len(t, list.Value, 2)

		// Check nested lists
		nested1, ok := list.Value[0].(*types.AttributeValueMemberL)
		require.True(t, ok)
		assert.Len(t, nested1.Value, 2)

		nested2, ok := list.Value[1].(*types.AttributeValueMemberL)
		require.True(t, ok)
		assert.Len(t, nested2.Value, 3)
	})

	t.Run("map[string]string", func(t *testing.T) {
		input := map[string]string{
			"name": "John",
			"city": "New York",
		}
		result, err := converter.ToAttributeValue(input)

		require.NoError(t, err)
		m, ok := result.(*types.AttributeValueMemberM)
		require.True(t, ok)
		assert.Len(t, m.Value, 2)
		assert.Equal(t, &types.AttributeValueMemberS{Value: "John"}, m.Value["name"])
		assert.Equal(t, &types.AttributeValueMemberS{Value: "New York"}, m.Value["city"])
	})

	t.Run("map[string]interface{}", func(t *testing.T) {
		// The converter doesn't handle interface{} directly,
		// so we need to skip this test
		t.Skip("interface{} values are not directly supported")
	})

	t.Run("struct", func(t *testing.T) {
		type Person struct {
			Name   string
			Age    int
			Active bool
			Score  float64
		}

		input := Person{
			Name:   "Bob",
			Age:    25,
			Active: true,
			Score:  98.5,
		}

		result, err := converter.ToAttributeValue(input)

		require.NoError(t, err)
		m, ok := result.(*types.AttributeValueMemberM)
		require.True(t, ok)
		assert.Len(t, m.Value, 4)
		assert.Equal(t, &types.AttributeValueMemberS{Value: "Bob"}, m.Value["Name"])
		assert.Equal(t, &types.AttributeValueMemberN{Value: "25"}, m.Value["Age"])
		assert.Equal(t, &types.AttributeValueMemberBOOL{Value: true}, m.Value["Active"])
		assert.Equal(t, &types.AttributeValueMemberN{Value: "98.5"}, m.Value["Score"])
	})

	t.Run("struct with zero values", func(t *testing.T) {
		type Person struct {
			Name   string
			Age    int
			Active bool
		}

		input := Person{
			Name: "Charlie",
			// Age and Active are zero values
		}

		result, err := converter.ToAttributeValue(input)

		require.NoError(t, err)
		m, ok := result.(*types.AttributeValueMemberM)
		require.True(t, ok)
		// Only non-zero values should be included
		assert.Len(t, m.Value, 1)
		assert.Equal(t, &types.AttributeValueMemberS{Value: "Charlie"}, m.Value["Name"])
	})

	t.Run("struct with unexported fields", func(t *testing.T) {
		type Secret struct {
			Public  string
			private string
		}

		input := Secret{
			Public:  "visible",
			private: "hidden",
		}

		result, err := converter.ToAttributeValue(input)

		require.NoError(t, err)
		m, ok := result.(*types.AttributeValueMemberM)
		require.True(t, ok)
		// Only exported fields should be included
		assert.Len(t, m.Value, 1)
		assert.Equal(t, &types.AttributeValueMemberS{Value: "visible"}, m.Value["Public"])
	})
}

// TestToAttributeValue_ErrorCases tests error handling
func TestToAttributeValue_ErrorCases(t *testing.T) {
	converter := NewConverter()

	t.Run("unsupported type", func(t *testing.T) {
		input := make(chan int) // channels are not supported
		_, err := converter.ToAttributeValue(input)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "unsupported type")
	})

	t.Run("map with non-string keys", func(t *testing.T) {
		input := map[int]string{
			1: "one",
			2: "two",
		}
		_, err := converter.ToAttributeValue(input)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "map keys must be strings")
	})
}

// TestFromAttributeValue_BasicTypes tests conversion from AttributeValues to Go types
func TestFromAttributeValue_BasicTypes(t *testing.T) {
	converter := NewConverter()

	t.Run("string", func(t *testing.T) {
		av := &types.AttributeValueMemberS{Value: "hello world"}
		var result string
		err := converter.FromAttributeValue(av, &result)
		assert.NoError(t, err)
		assert.Equal(t, "hello world", result)
	})

	t.Run("number to int", func(t *testing.T) {
		av := &types.AttributeValueMemberN{Value: "42"}
		var result int
		err := converter.FromAttributeValue(av, &result)
		assert.NoError(t, err)
		assert.Equal(t, 42, result)
	})

	t.Run("number to int64", func(t *testing.T) {
		av := &types.AttributeValueMemberN{Value: "9223372036854775807"}
		var result int64
		err := converter.FromAttributeValue(av, &result)
		assert.NoError(t, err)
		assert.Equal(t, int64(9223372036854775807), result)
	})

	t.Run("number to uint", func(t *testing.T) {
		av := &types.AttributeValueMemberN{Value: "42"}
		var result uint
		err := converter.FromAttributeValue(av, &result)
		assert.NoError(t, err)
		assert.Equal(t, uint(42), result)
	})

	t.Run("number to float64", func(t *testing.T) {
		av := &types.AttributeValueMemberN{Value: "3.14159"}
		var result float64
		err := converter.FromAttributeValue(av, &result)
		assert.NoError(t, err)
		assert.Equal(t, 3.14159, result)
	})

	t.Run("boolean", func(t *testing.T) {
		av := &types.AttributeValueMemberBOOL{Value: true}
		var result bool
		err := converter.FromAttributeValue(av, &result)
		assert.NoError(t, err)
		assert.True(t, result)
	})

	t.Run("binary", func(t *testing.T) {
		av := &types.AttributeValueMemberB{Value: []byte("hello")}
		var result []byte
		err := converter.FromAttributeValue(av, &result)
		assert.NoError(t, err)
		assert.Equal(t, []byte("hello"), result)
	})

	t.Run("null", func(t *testing.T) {
		av := &types.AttributeValueMemberNULL{Value: true}
		var result string
		err := converter.FromAttributeValue(av, &result)
		assert.NoError(t, err)
		assert.Equal(t, "", result) // Should remain zero value
	})

	t.Run("time.Time", func(t *testing.T) {
		testTime := time.Date(2023, 6, 15, 10, 30, 45, 123456789, time.UTC)
		av := &types.AttributeValueMemberS{Value: testTime.Format(time.RFC3339Nano)}
		var result time.Time
		err := converter.FromAttributeValue(av, &result)
		assert.NoError(t, err)
		assert.Equal(t, testTime, result)
	})
}

// TestFromAttributeValue_ComplexTypes tests conversion of complex types
func TestFromAttributeValue_ComplexTypes(t *testing.T) {
	converter := NewConverter()

	t.Run("list to slice", func(t *testing.T) {
		av := &types.AttributeValueMemberL{
			Value: []types.AttributeValue{
				&types.AttributeValueMemberS{Value: "apple"},
				&types.AttributeValueMemberS{Value: "banana"},
				&types.AttributeValueMemberS{Value: "cherry"},
			},
		}

		var result []string
		err := converter.FromAttributeValue(av, &result)
		assert.NoError(t, err)
		assert.Equal(t, []string{"apple", "banana", "cherry"}, result)
	})

	t.Run("map to map", func(t *testing.T) {
		av := &types.AttributeValueMemberM{
			Value: map[string]types.AttributeValue{
				"name": &types.AttributeValueMemberS{Value: "John"},
				"city": &types.AttributeValueMemberS{Value: "New York"},
			},
		}

		var result map[string]string
		err := converter.FromAttributeValue(av, &result)
		assert.NoError(t, err)
		assert.Equal(t, map[string]string{
			"name": "John",
			"city": "New York",
		}, result)
	})

	t.Run("map to struct", func(t *testing.T) {
		type Person struct {
			Name   string
			Age    int
			Active bool
		}

		av := &types.AttributeValueMemberM{
			Value: map[string]types.AttributeValue{
				"name":   &types.AttributeValueMemberS{Value: "Alice"},
				"age":    &types.AttributeValueMemberN{Value: "25"},
				"active": &types.AttributeValueMemberBOOL{Value: true},
			},
		}

		var result Person
		err := converter.FromAttributeValue(av, &result)
		assert.NoError(t, err)
		assert.Equal(t, Person{Name: "Alice", Age: 25, Active: true}, result)
	})

	t.Run("map to struct with attr tags", func(t *testing.T) {
		type MerchantBusinessAddress struct {
			Line1      string `theorydb:"attr:line1"`
			PostalCode string `theorydb:"attr:postalCode"`
		}

		type MerchantUnderwritingData struct {
			BusinessName    string                  `theorydb:"attr:businessName"`
			BusinessAddress MerchantBusinessAddress `theorydb:"attr:businessAddress"`
		}

		type MerchantOnboardingBusiness struct {
			UnderwritingData MerchantUnderwritingData `theorydb:"attr:underwritingData"`
		}

		type MerchantOnboardingData struct {
			MerchantUID string                     `theorydb:"attr:merchantUid"`
			Business    MerchantOnboardingBusiness `theorydb:"attr:business"`
		}

		localConverter := NewConverter()
		av := &types.AttributeValueMemberM{
			Value: map[string]types.AttributeValue{
				"merchantUid": &types.AttributeValueMemberS{Value: "merchant-123"},
				"business": &types.AttributeValueMemberM{
					Value: map[string]types.AttributeValue{
						"underwritingData": &types.AttributeValueMemberM{
							Value: map[string]types.AttributeValue{
								"businessName": &types.AttributeValueMemberS{Value: "Example LLC"},
								"businessAddress": &types.AttributeValueMemberM{
									Value: map[string]types.AttributeValue{
										"line1":      &types.AttributeValueMemberS{Value: "123 Main"},
										"postalCode": &types.AttributeValueMemberS{Value: "90210"},
									},
								},
							},
						},
					},
				},
			},
		}

		var dest MerchantOnboardingData
		err := localConverter.FromAttributeValue(av, &dest)
		require.NoError(t, err)
		assert.Equal(t, "merchant-123", dest.MerchantUID)
		assert.Equal(t, "Example LLC", dest.Business.UnderwritingData.BusinessName)
		assert.Equal(t, "90210", dest.Business.UnderwritingData.BusinessAddress.PostalCode)
	})

	t.Run("string set", func(t *testing.T) {
		av := &types.AttributeValueMemberSS{
			Value: []string{"red", "green", "blue"},
		}

		var result []string
		err := converter.FromAttributeValue(av, &result)
		assert.NoError(t, err)
		assert.Equal(t, []string{"red", "green", "blue"}, result)
	})

	t.Run("number set", func(t *testing.T) {
		av := &types.AttributeValueMemberNS{
			Value: []string{"1", "2", "3"},
		}

		var result []int
		err := converter.FromAttributeValue(av, &result)
		assert.NoError(t, err)
		assert.Equal(t, []int{1, 2, 3}, result)
	})

	t.Run("binary set", func(t *testing.T) {
		av := &types.AttributeValueMemberBS{
			Value: [][]byte{
				[]byte("hello"),
				[]byte("world"),
			},
		}

		var result [][]byte
		err := converter.FromAttributeValue(av, &result)
		assert.NoError(t, err)
		assert.Equal(t, [][]byte{
			[]byte("hello"),
			[]byte("world"),
		}, result)
	})
}

// TestFromAttributeValue_ErrorCases tests error handling in FromAttributeValue
func TestFromAttributeValue_ErrorCases(t *testing.T) {
	converter := NewConverter()

	t.Run("non-pointer target", func(t *testing.T) {
		av := &types.AttributeValueMemberS{Value: "hello"}
		var result string
		err := converter.FromAttributeValue(av, result) // Not a pointer
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "target must be a pointer")
	})

	t.Run("nil pointer", func(t *testing.T) {
		av := &types.AttributeValueMemberS{Value: "hello"}
		var result *string
		err := converter.FromAttributeValue(av, result) // Nil pointer
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "target pointer is nil")
	})

	t.Run("type mismatch", func(t *testing.T) {
		av := &types.AttributeValueMemberS{Value: "hello"}
		var result int
		err := converter.FromAttributeValue(av, &result)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "cannot convert string to int")
	})

	t.Run("invalid number", func(t *testing.T) {
		av := &types.AttributeValueMemberN{Value: "not-a-number"}
		var result int
		err := converter.FromAttributeValue(av, &result)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "invalid number")
	})

	t.Run("unsupported AttributeValue type", func(t *testing.T) {
		// Create a mock unsupported type
		type unsupportedAV struct {
			types.AttributeValue
		}
		av := &unsupportedAV{}
		var result string
		err := converter.FromAttributeValue(av, &result)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "unsupported AttributeValue type")
	})
}

// TestConvertToSet tests set conversion functionality
func TestConvertToSet(t *testing.T) {
	converter := NewConverter()

	t.Run("string set", func(t *testing.T) {
		input := []string{"apple", "banana", "cherry"}
		result, err := converter.ConvertToSet(input, true)

		require.NoError(t, err)
		ss, ok := result.(*types.AttributeValueMemberSS)
		require.True(t, ok)
		assert.Equal(t, []string{"apple", "banana", "cherry"}, ss.Value)
	})

	t.Run("number set from int slice", func(t *testing.T) {
		input := []int{1, 2, 3, 4, 5}
		result, err := converter.ConvertToSet(input, true)

		require.NoError(t, err)
		ns, ok := result.(*types.AttributeValueMemberNS)
		require.True(t, ok)
		assert.Equal(t, []string{"1", "2", "3", "4", "5"}, ns.Value)
	})

	t.Run("number set from float slice", func(t *testing.T) {
		input := []float64{1.1, 2.2, 3.3}
		result, err := converter.ConvertToSet(input, true)

		require.NoError(t, err)
		ns, ok := result.(*types.AttributeValueMemberNS)
		require.True(t, ok)
		assert.Equal(t, []string{"1.1", "2.2", "3.3"}, ns.Value)
	})

	t.Run("binary set", func(t *testing.T) {
		input := [][]byte{
			[]byte("hello"),
			[]byte("world"),
		}
		result, err := converter.ConvertToSet(input, true)

		require.NoError(t, err)
		bs, ok := result.(*types.AttributeValueMemberBS)
		require.True(t, ok)
		assert.Equal(t, [][]byte{
			[]byte("hello"),
			[]byte("world"),
		}, bs.Value)
	})

	t.Run("empty slice", func(t *testing.T) {
		input := []string{}
		result, err := converter.ConvertToSet(input, true)

		require.NoError(t, err)
		null, ok := result.(*types.AttributeValueMemberNULL)
		require.True(t, ok)
		assert.True(t, null.Value)
	})

	t.Run("not a set", func(t *testing.T) {
		input := []string{"apple", "banana"}
		result, err := converter.ConvertToSet(input, false)

		require.NoError(t, err)
		// Should be converted as regular list
		list, ok := result.(*types.AttributeValueMemberL)
		require.True(t, ok)
		assert.Len(t, list.Value, 2)
	})

	t.Run("non-slice input", func(t *testing.T) {
		input := "not a slice"
		_, err := converter.ConvertToSet(input, true)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "set tag requires slice type")
	})

	t.Run("unsupported set type", func(t *testing.T) {
		input := []struct{ Name string }{
			{Name: "test"},
		}
		_, err := converter.ConvertToSet(input, true)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "unsupported set element type")
	})
}

// MockConverter is a custom converter for testing
type MockConverter struct {
	toAttributeValueCalled   bool
	fromAttributeValueCalled bool
}

func (m *MockConverter) ToAttributeValue(value any) (types.AttributeValue, error) {
	m.toAttributeValueCalled = true
	return &types.AttributeValueMemberS{Value: "custom"}, nil
}

func (m *MockConverter) FromAttributeValue(av types.AttributeValue, target any) error {
	m.fromAttributeValueCalled = true
	if s, ok := target.(*string); ok {
		*s = "custom"
	}
	return nil
}

// TestCustomConverter tests custom converter functionality
func TestCustomConverter(t *testing.T) {
	converter := NewConverter()
	mockConverter := &MockConverter{}

	type CustomType struct {
		Value string
	}

	// Register custom converter
	converter.RegisterConverter(reflect.TypeOf(CustomType{}), mockConverter)

	t.Run("ToAttributeValue with custom converter", func(t *testing.T) {
		input := CustomType{Value: "test"}
		result, err := converter.ToAttributeValue(input)

		require.NoError(t, err)
		assert.True(t, mockConverter.toAttributeValueCalled)
		s, ok := result.(*types.AttributeValueMemberS)
		require.True(t, ok)
		assert.Equal(t, "custom", s.Value)
	})

	t.Run("FromAttributeValue with custom converter", func(t *testing.T) {
		av := &types.AttributeValueMemberS{Value: "test"}
		var result CustomType
		err := converter.FromAttributeValue(av, &result)

		require.NoError(t, err)
		assert.True(t, mockConverter.fromAttributeValueCalled)
	})
}

// TestPointerHandling tests pointer type handling
func TestPointerHandling(t *testing.T) {
	converter := NewConverter()

	t.Run("pointer to string", func(t *testing.T) {
		str := "hello"
		input := &str
		result, err := converter.ToAttributeValue(input)

		require.NoError(t, err)
		s, ok := result.(*types.AttributeValueMemberS)
		require.True(t, ok)
		assert.Equal(t, "hello", s.Value)
	})

	t.Run("pointer to struct", func(t *testing.T) {
		type Person struct {
			Name string
			Age  int
		}

		person := &Person{Name: "John", Age: 30}
		result, err := converter.ToAttributeValue(person)

		require.NoError(t, err)
		m, ok := result.(*types.AttributeValueMemberM)
		require.True(t, ok)
		assert.Len(t, m.Value, 2)
	})

	t.Run("FromAttributeValue to pointer", func(t *testing.T) {
		av := &types.AttributeValueMemberS{Value: "hello"}
		var result *string
		err := converter.FromAttributeValue(av, &result)

		require.NoError(t, err)
		require.NotNil(t, result)
		assert.Equal(t, "hello", *result)
	})
}

// TestEdgeCases tests various edge cases
func TestEdgeCases(t *testing.T) {
	converter := NewConverter()

	t.Run("empty struct", func(t *testing.T) {
		type Empty struct{}
		input := Empty{}
		result, err := converter.ToAttributeValue(input)

		require.NoError(t, err)
		m, ok := result.(*types.AttributeValueMemberM)
		require.True(t, ok)
		assert.Len(t, m.Value, 0)
	})

	t.Run("struct with only unexported fields", func(t *testing.T) {
		type Private struct {
			private1 string
			private2 int
		}
		input := Private{private1: "secret", private2: 42}
		result, err := converter.ToAttributeValue(input)

		require.NoError(t, err)
		m, ok := result.(*types.AttributeValueMemberM)
		require.True(t, ok)
		assert.Len(t, m.Value, 0)
	})

	t.Run("nested maps", func(t *testing.T) {
		// Skip this test as interface{} is not directly supported
		t.Skip("interface{} values are not directly supported")
	})
}

// BenchmarkToAttributeValue benchmarks conversion performance
func BenchmarkToAttributeValue(b *testing.B) {
	converter := NewConverter()

	b.Run("BasicTypes", func(b *testing.B) {
		for i := 0; i < b.N; i++ {
			if _, err := converter.ToAttributeValue("test string"); err != nil {
				b.Fatal(err)
			}
			if _, err := converter.ToAttributeValue(42); err != nil {
				b.Fatal(err)
			}
			if _, err := converter.ToAttributeValue(3.14); err != nil {
				b.Fatal(err)
			}
			if _, err := converter.ToAttributeValue(true); err != nil {
				b.Fatal(err)
			}
		}
	})

	b.Run("ComplexTypes", func(b *testing.B) {
		type Person struct {
			Scores map[string]float64
			Name   string
			Tags   []string
			Age    int
		}

		person := Person{
			Name: "Test User",
			Age:  30,
			Tags: []string{"tag1", "tag2", "tag3"},
			Scores: map[string]float64{
				"math":    95.5,
				"science": 88.0,
			},
		}

		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			if _, err := converter.ToAttributeValue(person); err != nil {
				b.Fatal(err)
			}
		}
	})
}

// BenchmarkFromAttributeValue benchmarks conversion performance
func BenchmarkFromAttributeValue(b *testing.B) {
	converter := NewConverter()

	av := &types.AttributeValueMemberM{
		Value: map[string]types.AttributeValue{
			"Name":   &types.AttributeValueMemberS{Value: "Test User"},
			"Age":    &types.AttributeValueMemberN{Value: "30"},
			"Active": &types.AttributeValueMemberBOOL{Value: true},
		},
	}

	type Person struct {
		Name   string
		Age    int
		Active bool
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		var result Person
		if err := converter.FromAttributeValue(av, &result); err != nil {
			b.Fatal(err)
		}
	}
}
