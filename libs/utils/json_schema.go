package utils

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"reflect"
	"strings"

	"github.com/danielgtaylor/huma/v2"
)

var registry = huma.NewMapRegistry("#/", huma.DefaultSchemaNamer)

// filterSchemaProperties recursively removes properties from the schema
// if the corresponding struct field's tag (tagKey) equals tagValue.
func filterSchemaProperties(t reflect.Type, schema *huma.Schema, tagKey, tagValue string) {
	if schema == nil || schema.Properties == nil {
		return
	}

	// Build a mapping from JSON property name to the corresponding struct field using VisibleFields,
	// which includes promoted fields from embedded structs.
	fieldsMap := make(map[string]reflect.StructField)
	visibleFields := reflect.VisibleFields(t)
	for _, field := range visibleFields {
		tag := field.Tag.Get("json")
		var jsonTag string
		if tag == "" {
			jsonTag = field.Name
		} else {
			if comma := strings.Index(tag, ","); comma != -1 {
				jsonTag = tag[:comma]
			} else {
				jsonTag = tag
			}
		}
		// Skip omitted fields.
		if jsonTag == "-" {
			continue
		}
		fieldsMap[jsonTag] = field
	}

	// Iterate over schema properties using the JSON keys.
	for key, prop := range schema.Properties {
		// Debug: Print the property being processed.
		//fmt.Printf("[DEBUG] Processing property: %s\n", key)
		if field, ok := fieldsMap[key]; ok {
			// Debug: Field found details.
			//fmt.Printf("[DEBUG] Found field: %s, type: %s, tag: %v\n", field.Name, field.Type, field.Tag)
			if field.Tag.Get(tagKey) == tagValue {
				// fmt.Printf("[DEBUG] Deleting field: %s (tag %s:%s)\n", key, tagKey, tagValue)
				delete(schema.Properties, key)
				continue
			}
			// Handle nested struct fields recursively.
			ft := field.Type
			if ft.Kind() == reflect.Ptr {
				ft = ft.Elem()
			}
			if ft.Kind() == reflect.Struct && prop != nil {
				// Initialize properties map if nil
				if prop.Properties == nil {
					prop.Properties = make(map[string]*huma.Schema)
				}
				filterSchemaProperties(ft, prop, tagKey, tagValue)
			}
		} else {
			// fmt.Printf("[DEBUG] Field not found for property: %s\n", key)
		}
	}
}

// Similar to numa.SchemaFromType, but instead of referncing nested structs
// they are serialized.
func NestedSchemaFromType(r huma.Registry, t reflect.Type) *huma.Schema {
	// Create a new schema from the type
	schema := huma.SchemaFromType(r, t)

	// If it's not a struct, just return the schema
	if t.Kind() != reflect.Struct {
		return schema
	}

	// Process each field in the struct
	for i := 0; i < t.NumField(); i++ {
		field := t.Field(i)

		// Get the JSON tag name
		tag := field.Tag.Get("json")
		var jsonName string
		if tag == "" {
			jsonName = field.Name
		} else {
			if comma := strings.Index(tag, ","); comma != -1 {
				jsonName = tag[:comma]
			} else {
				jsonName = tag
			}
		}

		// Skip omitted fields
		if jsonName == "-" {
			continue
		}

		// Check if this field is a struct or a pointer to a struct
		fieldType := field.Type
		if fieldType.Kind() == reflect.Ptr {
			fieldType = fieldType.Elem()
		}

		// If it's a struct, recursively process it
		if fieldType.Kind() == reflect.Struct {
			// Get the property from the schema
			if prop, ok := schema.Properties[jsonName]; ok && prop != nil {
				// Create a new schema for the nested struct
				nestedSchema := NestedSchemaFromType(r, fieldType)

				// Replace the property with the nested schema
				schema.Properties[jsonName] = nestedSchema
			}
		}
	}

	return schema
}

// GenerateSchemaForGET generates a schema for a given struct excluding writeOnly fields.
// It uses huma registry to possibly create references for nested structs. The schema that
// is returned can then be passed to `huma.Validate` to efficiently validate incoming requests.
func GenerateSchemaForGET(v interface{}) *huma.Schema {
	// Get the underlying type.
	t := reflect.TypeOf(v)
	if t.Kind() == reflect.Ptr {
		t = t.Elem()
	}
	schema := huma.SchemaFromType(registry, t)
	filterSchemaProperties(t, schema, "writeOnly", "true")
	return schema
}

// The same as GenerateSchemaForGET but returns string representation of JSON
func GenerateSchemaForGETString(v interface{}) string {
	schema := GenerateSchemaForGET(v)
	schemaJSON, err := json.MarshalIndent(schema, "", "  ")
	if err != nil {
		return fmt.Sprintf("%v", schema)
	}
	return string(schemaJSON)
}

// GenerateNestedSchemaForGET generates a schema for a given struct excluding writeOnly fields.
// In contrast of GenerateSchemaForGET it serializes nested structures as well
func GenerateNestedSchemaForGET(v interface{}) *huma.Schema {
	// Get the underlying type.
	t := reflect.TypeOf(v)
	if t.Kind() == reflect.Ptr {
		t = t.Elem()
	}
	schema := NestedSchemaFromType(registry, t)
	filterSchemaProperties(t, schema, "writeOnly", "true")
	return schema
}

// The same as GenerateNestedSchemaForGET but returns string representation of JSON
func GenerateNestedSchemaForGETString(v interface{}) string {
	schema := GenerateNestedSchemaForGET(v)
	schemaJSON, err := json.MarshalIndent(schema, "", "  ")
	if err != nil {
		return fmt.Sprintf("%v", schema)
	}
	return string(schemaJSON)
}

// GenerateSchemaForPOSTPUT generates a schema for a given struct excluding readOnly fields.
// It uses huma registry to possibly create references for nested structs. The schema that
// is returned can then be passed to `huma.Validate` to efficiently validate incoming requests.
func GenerateSchemaForPOSTPUT(v interface{}) *huma.Schema {
	// Handle nil input
	if v == nil {
		// Return a simple empty object schema
		return &huma.Schema{
			Type: "object",
		}
	}

	// Get the underlying type.
	t := reflect.TypeOf(v)
	if t.Kind() == reflect.Ptr {
		t = t.Elem()
	}

	schema := huma.SchemaFromType(registry, t)
	filterSchemaProperties(t, schema, "readOnly", "true")
	return schema
}

// The same as GenerateSchemaForPOSTPUT but returns string representation of JSON
func GenerateSchemaForPOSTPUTString(v interface{}) string {
	// Handle nil input directly to avoid unnecessary function call
	if v == nil {
		return `{"type":"object"}`
	}

	schema := GenerateSchemaForPOSTPUT(v)
	schemaJSON, err := json.MarshalIndent(schema, "", "  ")
	if err != nil {
		return fmt.Sprintf("%v", schema)
	}
	return string(schemaJSON)
}

// GenerateNestedSchemaForPOSTPUT generates a schema for a given struct excluding readOnly fields.
// In contrast of GenerateSchemaForPOSTPUT it serializes nested structures as well
func GenerateNestedSchemaForPOSTPUT(v interface{}) *huma.Schema {
	t := reflect.TypeOf(v)
	if t.Kind() == reflect.Ptr {
		t = t.Elem()
	}

	schema := NestedSchemaFromType(registry, t)
	filterSchemaProperties(t, schema, "readOnly", "true")
	return schema
}

// The same as GenerateNestedSchemaForPOSTPUT but returns string representation of JSON
func GenerateNestedSchemaForPOSTPUTString(v interface{}) string {
	schema := GenerateNestedSchemaForPOSTPUT(v)
	schemaJSON, err := json.MarshalIndent(schema, "", "  ")
	if err != nil {
		return fmt.Sprintf("%v", schema)
	}
	return string(schemaJSON)
}

// ValidateJSONAgainstSchemaOnGet validates the JSON against the schema for GET requests
func ValidateJSONAgainstSchemaOnGet(paramsBytes []byte, params interface{}) error {
	// Create a new JSON decoder reading from the given byte slice.
	dec := json.NewDecoder(bytes.NewReader(paramsBytes))
	// Enable strict mode: reject keys that are not defined in the schema.
	dec.DisallowUnknownFields()

	// Decode the JSON into the provided schema.
	if err := dec.Decode(params); err != nil {
		schemaObj := GenerateSchemaForGET(params)
		schemaJSON, jsonErr := json.MarshalIndent(schemaObj, "", "  ")
		schemaStr := ""
		if jsonErr != nil {
			schemaStr = fmt.Sprintf("%v", schemaObj)
		} else {
			schemaStr = string(schemaJSON)
		}
		return fmt.Errorf("JSON does not match schema, expected schema: %s, got: %s", schemaStr, string(paramsBytes))
	}

	// Ensure that there is no extra non-whitespace data after the JSON value.
	var extra interface{}
	if err := dec.Decode(&extra); err != io.EOF {
		return fmt.Errorf("unexpected extra JSON data")
	}

	return nil
}

func ValidateJSONAgainstSchemaOnPost(paramsBytes []byte, params interface{}) error {
	// Create a new JSON decoder reading from the given byte slice.
	dec := json.NewDecoder(bytes.NewReader(paramsBytes))
	// Enable strict mode: reject keys that are not defined in the schema.
	dec.DisallowUnknownFields()

	// Handle both struct and pointer to struct
	var target interface{}
	if reflect.TypeOf(params).Kind() == reflect.Ptr {
		target = params
	} else {
		// Create a new pointer to the same type as params
		target = reflect.New(reflect.TypeOf(params)).Interface()
	}

	// Decode the JSON into the provided schema.
	if err := dec.Decode(target); err != nil {
		schemaObj := GenerateSchemaForPOSTPUT(params)
		schemaJSON, jsonErr := json.MarshalIndent(schemaObj, "", "  ")
		schemaStr := ""
		if jsonErr != nil {
			schemaStr = fmt.Sprintf("%v", schemaObj)
		} else {
			schemaStr = string(schemaJSON)
		}
		return fmt.Errorf("JSON does not match schema, expected schema: %s, got: %s", schemaStr, string(paramsBytes))
	}

	// Ensure that there is no extra non-whitespace data after the JSON value.
	var extra interface{}
	if err := dec.Decode(&extra); err != io.EOF {
		return fmt.Errorf("unexpected extra JSON data")
	}

	return nil
}

func GetJsonFieldName(v interface{}, fieldKey string) string {
	t := reflect.TypeOf(v)
	if t.Kind() == reflect.Ptr {
		t = t.Elem()
	}
	field, ok := t.FieldByName(fieldKey)
	if !ok {
		return ""
	}
	return field.Tag.Get("json")
}

// StringToPrettyJSON converts a string to a formatted JSON object
func StringToPrettyJSON(data string) string {
	var jsonData map[string]interface{}
	if err := json.Unmarshal([]byte(data), &jsonData); err == nil {
		jsonBytes, _ := json.MarshalIndent(jsonData, "", "  ")
		return string(jsonBytes)
	}
	return data
}
