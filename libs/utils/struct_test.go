package utils

import (
	"encoding/json"
	"fmt"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
)

type EmbeddedTestStruct struct {
	Value string `json:"value" default:"embedded-default"`
}

type EmbeddedTestStruct2 struct {
	Value2 string `json:"value2" default:"embedded-default-2"`
}

type TestStruct struct {
	Name        string    `json:"name"`
	Age         int       `json:"age" default:"30"`
	Active      bool      `json:"active" default:"true"`
	Score       float64   `json:"score" default:"75.5"`
	Tags        []string  `json:"tags" default:"[\"test\",\"sample\"]"`
	CreatedAt   time.Time `json:"created_at"`
	ID          uuid.UUID `json:"id" default:"generate"`
	OptionalInt *int      `json:"optional_int"`
	Nested      struct {
		Value string `json:"value" default:"nested default"`
	} `json:"nested,inline"`
	Embedded  EmbeddedTestStruct  `json:"embedded"`
	Embedded2 EmbeddedTestStruct2 `json:",inline"`
}

func TestInitStructWithDefaults(t *testing.T) {
	// Create a struct with zero values
	s := TestStruct{}

	// Initialize with defaults
	InitStructWithDefaults(&s)

	// Check that defaults were applied
	assert.Equal(t, 30, s.Age)
	assert.Equal(t, true, s.Active)
	assert.Equal(t, 75.5, s.Score)
	assert.Equal(t, []string{"test", "sample"}, s.Tags)
	assert.Equal(t, "nested default", s.Nested.Value)
	assert.NotEqual(t, uuid.Nil, s.ID) // UUID should be generated
	assert.Nil(t, s.OptionalInt)       // No default, should remain nil
	assert.Equal(t, "embedded-default", s.Embedded.Value)
	assert.Equal(t, "embedded-default-2", s.Embedded2.Value2)
	// Test with non-pointer
	assert.NotPanics(t, func() {
		InitStructWithDefaults(s)
	})

	// Test with nil
	assert.NotPanics(t, func() {
		InitStructWithDefaults(nil)
	})
}

func TestStructToJSONString(t *testing.T) {
	// Create and initialize a test struct
	s := TestStruct{
		Name:   "Test",
		Age:    25,
		Active: true,
		Score:  98.6,
		Tags:   []string{"go", "testing"},
		ID:     uuid.New(),
	}

	// Convert to JSON string
	jsonStr, err := StructToJSONString(&s)
	assert.NoError(t, err)

	// Parse back and verify
	var parsed map[string]interface{}
	err = json.Unmarshal([]byte(jsonStr), &parsed)
	assert.NoError(t, err)

	assert.Equal(t, "Test", parsed["name"])
	assert.Equal(t, float64(25), parsed["age"])
	assert.Equal(t, true, parsed["active"])
	assert.Equal(t, 98.6, parsed["score"])

	// Test with non-struct
	_, err = StructToJSONString("not a struct")
	assert.Error(t, err)

	// Test with nil
	_, err = StructToJSONString(nil)
	assert.Error(t, err)

	// Test with slice
	slice := []TestStruct{s, s}
	jsonStr, err = StructToJSONString(slice)
	assert.NoError(t, err)

	// Parse back and verify
	var parsedSlice []interface{}
	err = json.Unmarshal([]byte(jsonStr), &parsedSlice)
	assert.NoError(t, err)
	assert.Len(t, parsedSlice, 2)

	// Test with pointer to pointer to pointer
	pps := &s
	ppps := &pps
	jsonStr, err = StructToJSONString(ppps)
	assert.NoError(t, err)

	// Verify the JSON string contains the expected values
	var parsedPPP map[string]interface{}
	err = json.Unmarshal([]byte(jsonStr), &parsedPPP)
	assert.NoError(t, err)
	assert.Equal(t, "Test", parsedPPP["name"])
	assert.Equal(t, float64(25), parsedPPP["age"])
	assert.Equal(t, true, parsedPPP["active"])
	assert.Equal(t, 98.6, parsedPPP["score"])

	// Test with nested structure
	type NestedStruct struct {
		ID    int    `json:"id"`
		Label string `json:"label"`
	}

	type OuterStruct struct {
		Title  string       `json:"title"`
		Nested NestedStruct `json:"nested"`
		Items  []string     `json:"items"`
	}

	outer := OuterStruct{
		Title: "Outer Title",
		Nested: NestedStruct{
			ID:    123,
			Label: "Nested Label",
		},
		Items: []string{"item1", "item2", "item3"},
	}

	jsonStr, err = StructToJSONString(outer)
	assert.NoError(t, err)

	var parsedOuter map[string]interface{}
	err = json.Unmarshal([]byte(jsonStr), &parsedOuter)
	assert.NoError(t, err)
	assert.Equal(t, "Outer Title", parsedOuter["title"])

	nestedMap, ok := parsedOuter["nested"].(map[string]interface{})
	assert.True(t, ok)
	assert.Equal(t, float64(123), nestedMap["id"])
	assert.Equal(t, "Nested Label", nestedMap["label"])

	items, ok := parsedOuter["items"].([]interface{})
	assert.True(t, ok)
	assert.Len(t, items, 3)
	assert.Equal(t, "item1", items[0])
	assert.Equal(t, "item2", items[1])
	assert.Equal(t, "item3", items[2])

	// Test with embedded inline structure
	type BaseStruct struct {
		ID        int    `json:"id"`
		CreatedAt string `json:"created_at"`
	}

	type EmbeddedStruct struct {
		BaseStruct `json:",inline"`
		Name       string `json:"name"`
		Active     bool   `json:"active"`
	}

	embedded := EmbeddedStruct{
		BaseStruct: BaseStruct{
			ID:        456,
			CreatedAt: "2023-01-01",
		},
		Name:   "Embedded Test",
		Active: true,
	}

	jsonStr, err = StructToJSONString(embedded)
	assert.NoError(t, err)

	var parsedEmbedded map[string]interface{}
	err = json.Unmarshal([]byte(jsonStr), &parsedEmbedded)
	assert.NoError(t, err)

	// Embedded fields should be flattened in the JSON
	assert.Equal(t, float64(456), parsedEmbedded["id"])
	assert.Equal(t, "2023-01-01", parsedEmbedded["created_at"])
	assert.Equal(t, "Embedded Test", parsedEmbedded["name"])
	assert.Equal(t, true, parsedEmbedded["active"])
}

func TestStructToJSONStringWithIndent(t *testing.T) {
	// Test with indentation
	type TestStruct struct {
		Name   string `json:"name"`
		Value  int    `json:"value"`
		Nested struct {
			Field string `json:"field"`
		} `json:"nested"`
	}

	test := TestStruct{
		Name:  "Test",
		Value: 42,
	}
	test.Nested.Field = "nested value"

	// Test with indentation
	jsonStr, err := StructToJSONString(test)
	assert.NoError(t, err)
	assert.Contains(t, jsonStr, "{\n")
	assert.Contains(t, jsonStr, "  \"name\": \"Test\"")
	assert.Contains(t, jsonStr, "  \"nested\": {")

	// Test with error case - channels can't be marshaled to JSON
	badStruct := struct {
		Ch chan int `json:"ch"`
	}{
		Ch: make(chan int),
	}

	_, err = StructToJSONString(badStruct)
	assert.Error(t, err)
}

func TestUnmarshalParquetFile(t *testing.T) {
	// This is difficult to test without a real parquet file
	// So we'll just test the error case with a non-existent file

	var data []TestStruct
	err := UnmarshalParquetFile("/path/to/nonexistent/file.parquet", &data)
	assert.Error(t, err)

	// Test with non-pointer
	err = UnmarshalParquetFile("/path/to/nonexistent/file.parquet", data)
	assert.Error(t, err)

	// Test with nil
	err = UnmarshalParquetFile("/path/to/nonexistent/file.parquet", nil)
	assert.Error(t, err)
}

func TestStructToJSONStringComplex(t *testing.T) {
	// Test struct with nested structs, pointers, slices, and special types
	type NestedLevel2 struct {
		Value string `json:"value"`
	}

	type NestedStruct struct {
		Name      string        `json:"name"`
		Level2    NestedLevel2  `json:"level2"`
		Level2Ptr *NestedLevel2 `json:"level2_ptr,omitempty"`
	}

	type ComplexStruct struct {
		ID                uuid.UUID       `json:"id"`
		Name              string          `json:"name"`
		Nested            NestedStruct    `json:"nested"`
		NestedPtr         *NestedStruct   `json:"nested_ptr,omitempty"`
		NilPtr            *NestedStruct   `json:"nil_ptr,omitempty"`
		StructSlice       []NestedStruct  `json:"struct_slice"`
		PtrSlice          []*NestedStruct `json:"ptr_slice"`
		EmptySlice        []NestedStruct  `json:"empty_slice"`
		NilSlice          []NestedStruct  `json:"nil_slice"`
		StringWithBadUTF8 string          `json:"bad_utf8"`
		SkippedField      string          `json:"-"`
	}

	// Create test data
	level2 := NestedLevel2{Value: "inner value"}
	nested := NestedStruct{
		Name:      "nested struct",
		Level2:    level2,
		Level2Ptr: &level2,
	}

	testID := uuid.New()
	complex := ComplexStruct{
		ID:          testID,
		Name:        "complex test",
		Nested:      nested,
		NestedPtr:   &nested,
		NilPtr:      nil,
		StructSlice: []NestedStruct{nested, nested},
		PtrSlice:    []*NestedStruct{&nested, &nested},
		EmptySlice:  []NestedStruct{},
		NilSlice:    nil,
		// Create a string with invalid UTF-8 sequence
		StringWithBadUTF8: "valid" + string([]byte{0xC0, 0xAF}) + "text",
	}

	// Convert to JSON
	jsonStr, err := StructToJSONString(complex)
	assert.NoError(t, err)

	// Parse back to verify
	var parsed map[string]interface{}
	err = json.Unmarshal([]byte(jsonStr), &parsed)
	assert.NoError(t, err)

	// Verify UUID field
	assert.Equal(t, testID.String(), parsed["id"])

	// Verify nested struct
	nestedMap, ok := parsed["nested"].(map[string]interface{})
	assert.True(t, ok)
	assert.Equal(t, "nested struct", nestedMap["name"])

	// Verify nested pointer
	nestedPtrMap, ok := parsed["nested_ptr"].(map[string]interface{})
	assert.True(t, ok)
	assert.Equal(t, "nested struct", nestedPtrMap["name"])

	// Verify nil pointer is omitted
	_, hasNilPtr := parsed["nil_ptr"]
	assert.True(t, hasNilPtr)

	// Verify struct slice
	structSlice, ok := parsed["struct_slice"].([]interface{})
	assert.True(t, ok)
	assert.Len(t, structSlice, 2)

	// Verify pointer slice
	ptrSlice, ok := parsed["ptr_slice"].([]interface{})
	assert.True(t, ok)
	assert.Len(t, ptrSlice, 2)

	// Verify empty slice
	emptySlice, ok := parsed["empty_slice"].([]interface{})
	assert.True(t, ok)
	assert.Len(t, emptySlice, 0)

	// Verify nil slice becomes empty array in JSON
	nilSlice, ok := parsed["nil_slice"].([]interface{})
	assert.True(t, ok)
	assert.Len(t, nilSlice, 0)

	// Verify invalid UTF-8 was sanitized
	assert.Equal(t, "validtext", parsed["bad_utf8"])
}

func TestStructToJSONStringWithArrayOfPrimitives(t *testing.T) {
	// Test with arrays of primitive types
	type ArrayStruct struct {
		IntArray    [3]int          `json:"int_array"`
		StringArray [2]string       `json:"string_array"`
		FloatSlice  []float64       `json:"float_slice"`
		BoolMap     map[string]bool `json:"bool_map"`
	}

	test := ArrayStruct{
		IntArray:    [3]int{1, 2, 3},
		StringArray: [2]string{"hello", "world"},
		FloatSlice:  []float64{1.1, 2.2, 3.3},
		BoolMap:     map[string]bool{"true": true, "false": false},
	}

	jsonStr, err := StructToJSONString(test)
	assert.NoError(t, err)

	var parsed map[string]interface{}
	err = json.Unmarshal([]byte(jsonStr), &parsed)
	assert.NoError(t, err)

	// Verify arrays
	intArray, ok := parsed["int_array"].([]interface{})
	assert.True(t, ok)
	assert.Len(t, intArray, 3)
	assert.Equal(t, float64(1), intArray[0]) // JSON numbers are float64

	stringArray, ok := parsed["string_array"].([]interface{})
	assert.True(t, ok)
	assert.Len(t, stringArray, 2)
	assert.Equal(t, "hello", stringArray[0])

	floatSlice, ok := parsed["float_slice"].([]interface{})
	assert.True(t, ok)
	assert.Len(t, floatSlice, 3)

	// Verify map
	boolMap, ok := parsed["bool_map"].(map[string]interface{})
	assert.True(t, ok)
	assert.Len(t, boolMap, 2)
	assert.Equal(t, true, boolMap["true"])
}

func TestMergeJSONWithDefaults(t *testing.T) {
	// Define a test struct with various field types
	type TestConfig struct {
		Name        string   `json:"name" default:"default-name"`
		Count       int      `json:"count" default:"42"`
		Enabled     bool     `json:"enabled" default:"true"`
		Rate        float64  `json:"rate" default:"0.5"`
		Tags        []string `json:"tags" default:"[\"default\",\"tags\"]"`
		Description string   `json:"description"`
	}

	// Test cases
	tests := []struct {
		name          string
		inputJSON     string
		expectedJSON  string
		expectedError bool
	}{
		{
			name:         "Empty input should return defaults",
			inputJSON:    "",
			expectedJSON: `{"count":42,"description":"","enabled":true,"name":"default-name","rate":0.5,"tags":["default","tags"]}`,
		},
		{
			name:         "Partial input should merge with defaults",
			inputJSON:    `{"name":"custom-name","count":100}`,
			expectedJSON: `{"count":100,"description":"","enabled":true,"name":"custom-name","rate":0.5,"tags":["default","tags"]}`,
		},
		{
			name:         "Complete input should override all defaults",
			inputJSON:    `{"name":"full-custom","count":200,"enabled":false,"rate":1.5,"tags":["custom"],"description":"Custom description"}`,
			expectedJSON: `{"count":200,"description":"Custom description","enabled":false,"name":"full-custom","rate":1.5,"tags":["custom"]}`,
		},
		{
			name:          "Invalid JSON should return error",
			inputJSON:     `{"name":"broken"`,
			expectedError: true,
		},
		{
			name:          "Unknown field should return error",
			inputJSON:     `{"unknown_field":"value"}`,
			expectedError: true,
		},
		{
			name:         "Empty array should override default array",
			inputJSON:    `{"tags":[]}`,
			expectedJSON: `{"count":42,"description":"","enabled":true,"name":"default-name","rate":0.5,"tags":[]}`,
		},
		{
			name:         "Null values should be preserved",
			inputJSON:    `{"name":null,"tags":null}`,
			expectedJSON: `{"count":42,"description":"","enabled":true,"name":null,"rate":0.5,"tags":null}`,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			fmt.Printf("Test case: %s: %s\n", tc.name, tc.inputJSON)

			// Create a new instance of the test struct
			defaultPtr := &TestConfig{}

			// Call MergeJSONWithDefaults
			result, err := MergeJSONWithDefaults(defaultPtr, tc.inputJSON)

			// Check error expectations
			if tc.expectedError {
				assert.Error(t, err)
				return
			}

			assert.NoError(t, err)

			// Normalize the expected and actual JSON for comparison
			var expectedMap map[string]interface{}
			var actualMap map[string]interface{}

			err = json.Unmarshal([]byte(tc.expectedJSON), &expectedMap)
			assert.NoError(t, err)

			err = json.Unmarshal([]byte(result), &actualMap)
			assert.NoError(t, err)

			assert.Equal(t, expectedMap, actualMap)

			// Also verify that the defaultPtr was properly updated
			if tc.inputJSON != "" && !tc.expectedError {
				// For non-empty valid inputs, the struct should be populated
				var expectedStruct TestConfig
				err = json.Unmarshal([]byte(result), &expectedStruct)
				assert.NoError(t, err)

				assert.Equal(t, expectedStruct.Name, defaultPtr.Name)
				assert.Equal(t, expectedStruct.Count, defaultPtr.Count)
				assert.Equal(t, expectedStruct.Enabled, defaultPtr.Enabled)
				assert.Equal(t, expectedStruct.Rate, defaultPtr.Rate)
				assert.Equal(t, expectedStruct.Tags, defaultPtr.Tags)
				assert.Equal(t, expectedStruct.Description, defaultPtr.Description)
			}
		})
	}
}

func TestMergeJSONWithDefaultsEdgeCases(t *testing.T) {
	// Test with non-struct pointer
	result, err := MergeJSONWithDefaults("not a struct pointer", "{}")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "must be a pointer to a struct")
	assert.Empty(t, result)

	// Test with nil pointer
	result, err = MergeJSONWithDefaults(nil, "{}")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "must be a pointer to a struct")
	assert.Empty(t, result)

	// Test with embedded structs
	type EmbeddedStruct struct {
		Value string `json:"value" default:"embedded-default"`
	}

	type ParentStruct struct {
		Name     string         `json:"name" default:"parent-default"`
		Embedded EmbeddedStruct `json:"embedded"`
	}

	// Test embedded struct defaults
	parent := &ParentStruct{}
	result, err = MergeJSONWithDefaults(parent, `{"embedded":{"value":"custom-embedded"}}`)
	assert.NoError(t, err)

	var parsed ParentStruct
	err = json.Unmarshal([]byte(result), &parsed)
	assert.NoError(t, err)

	assert.Equal(t, "parent-default", parsed.Name)
	assert.Equal(t, "custom-embedded", parsed.Embedded.Value)

	// Test with empty string input - should get all defaults
	parent = &ParentStruct{}
	result, err = MergeJSONWithDefaults(parent, "")
	assert.NoError(t, err)

	parsed = ParentStruct{}
	err = json.Unmarshal([]byte(result), &parsed)
	assert.NoError(t, err)

	assert.Equal(t, "parent-default", parsed.Name)
	assert.Equal(t, "embedded-default", parsed.Embedded.Value)
}

func TestMergeJSONWithDefaultsEmbeddedStructs(t *testing.T) {
	// Define embedded struct types for testing
	type DeepEmbeddedStruct struct {
		Value int    `json:"value" default:"42"`
		Text  string `json:"text" default:"deep-default"`
	}

	type EmbeddedStruct struct {
		Name         string             `json:"name" default:"embedded-default"`
		Active       bool               `json:"active" default:"true"`
		DeepEmbedded DeepEmbeddedStruct `json:"deep_embedded"`
	}

	type ParentStruct struct {
		ID        string         `json:"id" default:"parent-id"`
		Count     int            `json:"count" default:"10"`
		Embedded  EmbeddedStruct `json:"embedded"`
		Tags      []string       `json:"tags" default:"[\"default\"]"`
		CreatedAt int64          `json:"created_at"`
	}

	t.Run("Partial JSON with embedded fields", func(t *testing.T) {
		parent := &ParentStruct{}
		inputJSON := `{
			"id": "custom-id",
			"embedded": {
				"name": "custom-name",
				"deep_embedded": {
					"value": 100
				}
			}
		}`

		result, err := MergeJSONWithDefaults(parent, inputJSON)
		assert.NoError(t, err)

		var parsed ParentStruct
		err = json.Unmarshal([]byte(result), &parsed)
		assert.NoError(t, err)

		// Check parent level fields
		assert.Equal(t, "custom-id", parsed.ID)
		assert.Equal(t, 10, parsed.Count)                 // Default value
		assert.Equal(t, []string{"default"}, parsed.Tags) // Default value

		// Check nested fields
		assert.Equal(t, "custom-name", parsed.Embedded.Name)
		assert.Equal(t, true, parsed.Embedded.Active) // Default value

		// Check deep nested fields
		assert.Equal(t, 100, parsed.Embedded.DeepEmbedded.Value)
		assert.Equal(t, "deep-default", parsed.Embedded.DeepEmbedded.Text) // Default value
	})

	t.Run("Empty JSON should use all defaults", func(t *testing.T) {
		parent := &ParentStruct{}
		result, err := MergeJSONWithDefaults(parent, "")
		assert.NoError(t, err)

		var parsed ParentStruct
		err = json.Unmarshal([]byte(result), &parsed)
		assert.NoError(t, err)

		// All fields should have default values
		assert.Equal(t, "parent-id", parsed.ID)
		assert.Equal(t, 10, parsed.Count)
		assert.Equal(t, "embedded-default", parsed.Embedded.Name)
		assert.Equal(t, true, parsed.Embedded.Active)
		assert.Equal(t, 42, parsed.Embedded.DeepEmbedded.Value)
		assert.Equal(t, "deep-default", parsed.Embedded.DeepEmbedded.Text)
		assert.Equal(t, []string{"default"}, parsed.Tags)
	})

	t.Run("Overriding all fields", func(t *testing.T) {
		parent := &ParentStruct{}
		inputJSON := `{
			"id": "override-id",
			"count": 99,
			"embedded": {
				"name": "override-name",
				"active": false,
				"deep_embedded": {
					"value": 123,
					"text": "override-text"
				}
			},
			"tags": ["tag1", "tag2"]
		}`

		result, err := MergeJSONWithDefaults(parent, inputJSON)
		assert.NoError(t, err)

		var parsed ParentStruct
		err = json.Unmarshal([]byte(result), &parsed)
		assert.NoError(t, err)

		// Check all fields are overridden
		assert.Equal(t, "override-id", parsed.ID)
		assert.Equal(t, 99, parsed.Count)
		assert.Equal(t, "override-name", parsed.Embedded.Name)
		assert.Equal(t, false, parsed.Embedded.Active)
		assert.Equal(t, 123, parsed.Embedded.DeepEmbedded.Value)
		assert.Equal(t, "override-text", parsed.Embedded.DeepEmbedded.Text)
		assert.Equal(t, []string{"tag1", "tag2"}, parsed.Tags)
	})
}
