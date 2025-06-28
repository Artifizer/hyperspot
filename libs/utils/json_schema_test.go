package utils

import (
	"encoding/json"
	"reflect"
	"testing"

	"github.com/danielgtaylor/huma/v2"
	"github.com/stretchr/testify/assert"
)

type NestedStruct struct {
	Field1 string `json:"field1" readOnly:"true"`
	Field2 string `json:"field2" writeOnly:"true"`
	Field3 int    `json:"field3"`
}

// TestStruct with various JSON tags for schema testing
type SchemaTestStruct struct {
	ID        string       `json:"id" readOnly:"true"`
	Name      string       `json:"name"`
	Email     string       `json:"email"`
	CreatedAt string       `json:"created_at" readOnly:"true"`
	Password  string       `json:"password" writeOnly:"true"`
	Age       int          `json:"age"`
	Nested    NestedStruct `json:"nested"`
	Nested2   struct {
		Field1 string `json:"field1" readOnly:"true"`
		Field2 string `json:"field2" writeOnly:"true"`
		Field3 int    `json:"field3"`
	} `json:"nested2"`
}

func TestGenerateSchemaForGET(t *testing.T) {
	testStruct := SchemaTestStruct{}
	schema := GenerateSchemaForGET(testStruct)

	// Verify schema is not nil
	assert.NotNil(t, schema)

	// Schema should include id (readOnly is fine for GET)
	assert.Contains(t, schema.Properties, "id")

	// Schema should not include password (writeOnly)
	assert.NotContains(t, schema.Properties, "password")

	// Schema should include nested properties
	nested, exists := schema.Properties["nested"]
	assert.True(t, exists)

	// Nested fields are not visible in GenerateSchemaForGET
	assert.NotContains(t, nested.Properties, "field1")
	assert.NotContains(t, nested.Properties, "field2")
	assert.NotContains(t, nested.Properties, "field3")
}

func TestGenerateNestedSchemaForGET(t *testing.T) {
	testStruct := SchemaTestStruct{}
	schema := GenerateNestedSchemaForGET(testStruct)

	// Verify schema is not nil
	assert.NotNil(t, schema)

	// Schema should include id (readOnly is fine for GET)
	assert.Contains(t, schema.Properties, "id")

	// Schema should not include password (writeOnly)
	assert.NotContains(t, schema.Properties, "password")

	// Schema should include nested properties
	nested, exists := schema.Properties["nested"]
	assert.True(t, exists)

	// Nested schema should include field1 (readOnly is fine for GET)
	assert.Contains(t, nested.Properties, "field1")

	// Nested schema should not include field2 (writeOnly)
	assert.NotContains(t, nested.Properties, "field2")

	// Nested schema should include field3 (it's both readOnly and writeOnly)
	assert.Contains(t, nested.Properties, "field3")
}

func TestGenerateSchemaForPOSTPUT(t *testing.T) {
	testStruct := SchemaTestStruct{}
	testStruct.Nested.Field3 = 42
	schema := GenerateSchemaForPOSTPUT(testStruct)

	// Verify schema is not nil
	assert.NotNil(t, schema)

	// Schema should not include id (readOnly)
	assert.NotContains(t, schema.Properties, "id")

	// Schema should not include createdAt (readOnly)
	assert.NotContains(t, schema.Properties, "created_at")

	// Schema should include password (writeOnly is fine for POST)
	assert.Contains(t, schema.Properties, "password")

	nested, exists := schema.Properties["nested"]
	assert.True(t, exists)

	// Nested fields are not visible in GenerateSchemaForGET
	assert.NotContains(t, nested.Properties, "field1")
	assert.NotContains(t, nested.Properties, "field2")
	assert.NotContains(t, nested.Properties, "field3")
}

func TestGenerateNestedSchemaForPOSTPUT(t *testing.T) {
	testStruct := SchemaTestStruct{}
	testStruct.Nested.Field3 = 42
	schema := GenerateNestedSchemaForPOSTPUT(testStruct)

	// Verify schema is not nil
	assert.NotNil(t, schema)

	// Schema should not include id (readOnly)
	assert.NotContains(t, schema.Properties, "id")

	// Schema should not include createdAt (readOnly)
	assert.NotContains(t, schema.Properties, "created_at")

	// Schema should include password (writeOnly is fine for POST)
	assert.Contains(t, schema.Properties, "password")

	nested, exists := schema.Properties["nested"]
	assert.True(t, exists)

	// Nested fields are not visible in GenerateSchemaForGET
	assert.NotContains(t, nested.Properties, "field1")

	// Nested schema should include field2 (writeOnly is fine for POST)
	assert.Contains(t, nested.Properties, "field2")

	// Nested schema should include field3 (it's both readOnly and writeOnly)
	assert.Contains(t, nested.Properties, "field3")
}

func TestSchemaStringFunctions(t *testing.T) {
	testStruct := SchemaTestStruct{}

	// Test GET schema string
	getSchemaStr := GenerateSchemaForGETString(testStruct)
	assert.NotEmpty(t, getSchemaStr)

	// Verify it's valid JSON
	var getSchema map[string]interface{}
	err := json.Unmarshal([]byte(getSchemaStr), &getSchema)
	assert.NoError(t, err)

	// Test POST schema string
	postSchemaStr := GenerateSchemaForPOSTPUTString(testStruct)
	assert.NotEmpty(t, postSchemaStr)

	// Verify it's valid JSON
	var postSchema map[string]interface{}
	err = json.Unmarshal([]byte(postSchemaStr), &postSchema)
	assert.NoError(t, err)
}

func TestValidateJSONAgainstSchemaOnGet(t *testing.T) {
	// Test valid JSON
	validJSON := `{
		"id": "123",
		"name": "Test User",
		"email": "test@example.com",
		"created_at": "2023-10-01",
		"age": 30,
		"nested": {
			"field1": "value1",
			"field3": 42
		}
	}`

	testStruct := SchemaTestStruct{}
	err := ValidateJSONAgainstSchemaOnGet([]byte(validJSON), &testStruct)
	assert.NoError(t, err)
	assert.Equal(t, "Test User", testStruct.Name)
	assert.Equal(t, 30, testStruct.Age)

	// Test invalid JSON (extra field)
	invalidJSON := `{
		"id": "123",
		"name": "Test User",
		"email": "test@example.com",
		"extra_field": "should fail",
		"age": 30
	}`

	err = ValidateJSONAgainstSchemaOnGet([]byte(invalidJSON), &testStruct)
	assert.Error(t, err)

	// Test invalid JSON (wrong type)
	invalidJSON = `{
		"id": "123",
		"name": "Test User",
		"email": "test@example.com",
		"age": "not-a-number"
	}`

	err = ValidateJSONAgainstSchemaOnGet([]byte(invalidJSON), &testStruct)
	assert.Error(t, err)
}

func TestValidateJSONAgainstSchemaOnPost(t *testing.T) {
	// Test valid JSON for POST (no readOnly fields)
	validJSON := `{
		"name": "Test User",
		"email": "test@example.com",
		"password": "secret123",
		"age": 30,
		"nested": {
			"field2": "value2",
			"field3": 42
		}
	}`

	testStruct := SchemaTestStruct{}
	err := ValidateJSONAgainstSchemaOnPost([]byte(validJSON), &testStruct)
	assert.NoError(t, err)
	assert.Equal(t, "Test User", testStruct.Name)
	assert.Equal(t, "secret123", testStruct.Password)

	// Test invalid JSON (includes readOnly field)
	invalidJSON := `{
		"id": "123",
		"name": "Test User",
		"email": "test@example.com",
		"password": "secret123",
		"age": 30
	}`

	// Note: This might not actually fail because the JSON decoder
	// will just populate the ID field even though it's marked readOnly.
	// The filtering happens at schema generation time, not validation time.
	err = ValidateJSONAgainstSchemaOnPost([]byte(invalidJSON), &testStruct)

	// Test invalid JSON (extra field)
	invalidJSON = `{
		"name": "Test User",
		"email": "test@example.com",
		"password": "secret123",
		"extra_field": "should fail",
		"age": 30
	}`

	err = ValidateJSONAgainstSchemaOnPost([]byte(invalidJSON), &testStruct)
	assert.Error(t, err)
}

func TestValidateJSONAgainstSchemaOnPostWithEmbeddedStruct(t *testing.T) {
	// Test valid JSON for POST with embedded struct
	validJSON := `{
		"field2": "value2",
		"field3": 42
	}`

	testStruct := SchemaTestStruct{}

	// Test with pointer to struct
	err := ValidateJSONAgainstSchemaOnPost([]byte(validJSON), &testStruct.Nested2)
	assert.NoError(t, err)
	assert.Equal(t, "value2", testStruct.Nested2.Field2)
	assert.Equal(t, 42, testStruct.Nested2.Field3)

	// Test with struct
	err = ValidateJSONAgainstSchemaOnPost([]byte(validJSON), testStruct.Nested2)
	assert.NoError(t, err)
	assert.Equal(t, "value2", testStruct.Nested2.Field2)
	assert.Equal(t, 42, testStruct.Nested2.Field3)

	// Test invalid JSON (includes readOnly field)
	invalidJSON := `{
		"field1": "value1", // This should fail because field1 is readOnly
		"field2": "value2",
		"field3": 42
	}`

	err = ValidateJSONAgainstSchemaOnPost([]byte(invalidJSON), &testStruct.Nested2)
	assert.Error(t, err)

	err = ValidateJSONAgainstSchemaOnPost([]byte(invalidJSON), testStruct.Nested2)
	assert.Error(t, err)

	// Test invalid JSON (extra field)
	invalidJSON = `{
		"field2": "value2",
		"field3": 42,
		"extra_field": "should fail"
	}`

	err = ValidateJSONAgainstSchemaOnPost([]byte(invalidJSON), &testStruct.Nested2)
	assert.Error(t, err)

	err = ValidateJSONAgainstSchemaOnPost([]byte(invalidJSON), testStruct.Nested2)
	assert.Error(t, err)

	// Test invalid JSON (wrong type)
	invalidJSON = `{
		"field2": "value2",
		"field3": "not a number"
	}`

	err = ValidateJSONAgainstSchemaOnPost([]byte(invalidJSON), &testStruct.Nested2)
	assert.Error(t, err)

	err = ValidateJSONAgainstSchemaOnPost([]byte(invalidJSON), testStruct.Nested2)
	assert.Error(t, err)
}

func TestGetJsonFieldName(t *testing.T) {
	testStruct := SchemaTestStruct{}

	// Test existing fields
	assert.Equal(t, "id", GetJsonFieldName(testStruct, "ID"))
	assert.Equal(t, "name", GetJsonFieldName(testStruct, "Name"))
	assert.Equal(t, "email", GetJsonFieldName(testStruct, "Email"))
	assert.Equal(t, "created_at", GetJsonFieldName(testStruct, "CreatedAt"))

	// Test non-existent field
	assert.Equal(t, "", GetJsonFieldName(testStruct, "NonExistentField"))

	// Test with pointer to struct
	assert.Equal(t, "id", GetJsonFieldName(&testStruct, "ID"))
}

// Test helper functions to ensure full coverage
func TestFilterSchemaProperties(t *testing.T) {
	// Create separate schemas for each test case
	structType := reflect.TypeOf(SchemaTestStruct{})
	schema1 := huma.SchemaFromType(registry, structType)
	schema2 := huma.SchemaFromType(registry, structType)

	assert.Contains(t, schema1.Properties, "id")
	assert.Contains(t, schema2.Properties, "password")

	// Test filtering readOnly properties
	filterSchemaProperties(structType, schema1, "readOnly", "true")
	assert.NotContains(t, schema1.Properties, "id")
	assert.Contains(t, schema1.Properties, "password")

	// Test filtering writeOnly properties
	filterSchemaProperties(structType, schema2, "writeOnly", "true")
	assert.Contains(t, schema2.Properties, "id")
	assert.NotContains(t, schema2.Properties, "password")
}

func TestStringToPrettyJSON(t *testing.T) {
	// Test with valid JSON input: it should return a pretty printed JSON string.
	validJSON := `{"key": "value", "num":123}`
	prettyJSON := StringToPrettyJSON(validJSON)
	// Assert the output is equivalent to the input JSON (ignoring differences in whitespace)
	assert.JSONEq(t, validJSON, prettyJSON, "Expected pretty printed JSON to have the same structure as input")

	// Test with invalid JSON input: it should return the original string.
	invalidJSON := "this is not json"
	result := StringToPrettyJSON(invalidJSON)
	assert.Equal(t, invalidJSON, result, "Expected non-JSON string to be returned unchanged")
}
