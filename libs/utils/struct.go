package utils

import (
	"fmt"
	"reflect"
	"strconv"
	"strings"

	"encoding/json"

	"github.com/google/uuid"
)

// InitStructWithDefaults initializes a struct with default values from struct tags
func InitStructWithDefaults(v interface{}) {
	val := reflect.ValueOf(v)
	if val.Kind() != reflect.Ptr || val.Elem().Kind() != reflect.Struct {
		return
	}

	val = val.Elem()
	t := val.Type()

	for i := 0; i < val.NumField(); i++ {
		field := val.Field(i)
		fieldType := t.Field(i)

		// Skip unexported fields
		if !field.CanSet() {
			continue
		}

		// Special handling for UUID type (do this before other checks)
		defaultValue, hasDefault := fieldType.Tag.Lookup("default")
		if hasDefault && defaultValue == "generate" {
			// Check if it's a UUID type using both the type's string representation and direct type comparison
			isUUID := field.Type().String() == "github.com/google/uuid.UUID" ||
				field.Type() == reflect.TypeOf(uuid.UUID{})

			if isUUID {
				newUUID := uuid.New()
				field.Set(reflect.ValueOf(newUUID))
				continue // Skip further processing for this field
			}
		}

		// Handle nested structs (including embedded structs)
		if field.Kind() == reflect.Struct {
			if field.CanAddr() {
				InitStructWithDefaults(field.Addr().Interface())
			}
			continue
		}

		// Handle pointer to struct
		if field.Kind() == reflect.Ptr && field.Type().Elem().Kind() == reflect.Struct {
			if field.IsNil() {
				field.Set(reflect.New(field.Type().Elem()))
			}
			InitStructWithDefaults(field.Interface())
			continue
		}

		// Handle basic types with default tags
		if hasDefault {
			switch field.Kind() {
			case reflect.String:
				field.SetString(defaultValue)
			case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
				if intValue, err := strconv.ParseInt(defaultValue, 10, 64); err == nil {
					field.SetInt(intValue)
				}
			case reflect.Float32, reflect.Float64:
				if floatValue, err := strconv.ParseFloat(defaultValue, 64); err == nil {
					field.SetFloat(floatValue)
				}
			case reflect.Bool:
				if boolValue, err := strconv.ParseBool(defaultValue); err == nil {
					field.SetBool(boolValue)
				}
			case reflect.Slice:
				// Handle string slices
				if field.Type().Elem().Kind() == reflect.String {
					if strings.HasPrefix(defaultValue, "[") && strings.HasSuffix(defaultValue, "]") {
						// Parse JSON format
						var values []string
						if err := json.Unmarshal([]byte(defaultValue), &values); err == nil {
							slice := reflect.MakeSlice(field.Type(), len(values), len(values))
							for i, v := range values {
								slice.Index(i).SetString(v)
							}
							field.Set(slice)
						}
					} else if defaultValue != "" {
						// Parse comma-separated format
						values := strings.Split(defaultValue, ",")
						slice := reflect.MakeSlice(field.Type(), len(values), len(values))
						for i, v := range values {
							slice.Index(i).SetString(strings.TrimSpace(v))
						}
						field.Set(slice)
					}
				}
			}
		}
	}
}

func StructToJSONString(v interface{}) (string, error) {
	// Get the value and type of the interface
	val := reflect.ValueOf(v)

	if val.Kind() == reflect.Ptr {
		// Check if the pointer is nil
		if val.IsNil() {
			return "", fmt.Errorf("nil pointer encountered")
		}
		return StructToJSONString(val.Elem().Interface())
	}

	// If it's a slice, handle it directly
	if val.Kind() == reflect.Slice {
		sliceLen := val.Len()
		sliceValue := make([]interface{}, sliceLen)
		for j := 0; j < sliceLen; j++ {
			elem := val.Index(j)
			if elem.Kind() == reflect.Struct || (elem.Kind() == reflect.Ptr && elem.Elem().Kind() == reflect.Struct) {
				nestedJSON, err := StructToJSONString(elem.Interface())
				if err != nil {
					return "", err
				}
				var nestedMap map[string]interface{}
				if err := json.Unmarshal([]byte(nestedJSON), &nestedMap); err != nil {
					return "", err
				}
				sliceValue[j] = nestedMap
			} else {
				sliceValue[j] = elem.Interface()
			}
		}
		// Marshal the slice directly
		jsonBytes, err := json.MarshalIndent(sliceValue, "", "  ")
		if err != nil {
			return "", err
		}
		return string(jsonBytes), nil
	}

	// Handle struct case as before
	if val.Kind() != reflect.Struct {
		return "", fmt.Errorf("value must be a struct or slice, got %v", val.Kind())
	}

	// Create a map to store the struct fields
	m := make(map[string]interface{})

	// Iterate through struct fields
	t := val.Type()
	for i := 0; i < val.NumField(); i++ {
		field := val.Field(i)
		fieldType := t.Field(i)

		// Skip unexported fields
		if !field.CanInterface() {
			continue
		}

		// Skip function fields
		if field.Kind() == reflect.Func {
			continue
		}
		// Also skip interface fields if they wrap a function
		if field.Kind() == reflect.Interface && !field.IsNil() && field.Elem().Kind() == reflect.Func {
			continue
		}
		// Skip interface fields altogether (e.g., LLMService interface)
		if fieldType.Type.Kind() == reflect.Interface {
			continue
		}

		// Get the json tag name; skip field if not present or marked as "-"
		jsonTag := fieldType.Tag.Get("json")
		if jsonTag == "" || jsonTag == "-" {
			continue
		}

		// Check if this is an embedded inline struct
		isInline := false
		tagParts := strings.Split(jsonTag, ",")
		jsonName := tagParts[0]

		for _, part := range tagParts {
			if part == "inline" {
				isInline = true
				break
			}
		}

		// Handle embedded inline structs
		if isInline && field.Kind() == reflect.Struct {
			nestedJSON, err := StructToJSONString(field.Interface())
			if err != nil {
				return "", err
			}
			var nestedMap map[string]interface{}
			if err := json.Unmarshal([]byte(nestedJSON), &nestedMap); err != nil {
				return "", err
			}
			// Merge the nested map directly into the parent map
			for k, v := range nestedMap {
				m[k] = v
			}
			continue
		}

		// If field is exactly a uuid.UUID, serialize it as its string value.
		if field.Type() == reflect.TypeOf(uuid.UUID{}) {
			m[jsonName] = field.Interface().(uuid.UUID).String()
			continue
		}

		var fieldValue interface{}

		switch field.Kind() {
		case reflect.Struct:
			// Recursively handle nested structs
			nestedJSON, err := StructToJSONString(field.Interface())
			if err != nil {
				return "", err
			}
			var nestedMap map[string]interface{}
			if err := json.Unmarshal([]byte(nestedJSON), &nestedMap); err != nil {
				return "", err
			}
			fieldValue = nestedMap

		case reflect.Ptr:
			if !field.IsNil() {
				if field.Elem().Kind() == reflect.Struct {
					// Recursively handle pointer to struct
					nestedJSON, err := StructToJSONString(field.Interface())
					if err != nil {
						return "", err
					}
					var nestedMap map[string]interface{}
					if err := json.Unmarshal([]byte(nestedJSON), &nestedMap); err != nil {
						return "", err
					}
					fieldValue = nestedMap
				} else {
					fieldValue = field.Elem().Interface()
				}
			}

		case reflect.Slice, reflect.Array:
			if field.Kind() == reflect.Slice && field.IsNil() {
				fieldValue = []interface{}{}
				break
			}
			sliceLen := field.Len()
			sliceValue := make([]interface{}, sliceLen)
			for j := 0; j < sliceLen; j++ {
				elem := field.Index(j)
				if elem.Kind() == reflect.Struct || (elem.Kind() == reflect.Ptr && elem.Elem().Kind() == reflect.Struct) {
					nestedJSON, err := StructToJSONString(elem.Interface())
					if err != nil {
						return "", err
					}
					var nestedMap map[string]interface{}
					if err := json.Unmarshal([]byte(nestedJSON), &nestedMap); err != nil {
						return "", err
					}
					sliceValue[j] = nestedMap
				} else {
					sliceValue[j] = elem.Interface()
				}
			}
			fieldValue = sliceValue

		case reflect.String:
			// Ensure the string is valid UTF-8.
			s := field.Interface().(string)
			fieldValue = strings.ToValidUTF8(s, "")
		default:
			// If field is a uuid.UUID, serialize as its string value.
			if field.Type() == reflect.TypeOf(uuid.UUID{}) {
				fieldValue = field.Interface().(uuid.UUID).String()
			} else {
				fieldValue = field.Interface()
			}
		}

		m[jsonName] = fieldValue
	}

	// Marshal the final map to JSON
	jsonBytes, err := json.MarshalIndent(m, "", "  ")
	if err != nil {
		return "", err
	}

	return string(jsonBytes), nil
}

// MergeJSONWithDefaults merges a given JSON string into the struct pointed to by defaultPtr,
// preserving existing default values for any missing fields. It also validates the merged JSON
// against the schema defined by defaultPtr using ValidateJSONAgainstSchemaOnPost.
// If inputJSON is empty, it returns the default struct as JSON.
// This function updates the value pointed to by defaultPtr with the merged result.
// It supports deep embedded structs and embedded structs within other embedded structs.
func MergeJSONWithDefaults(defaultPtr interface{}, inputJSON string) (string, error) {
	// Ensure that defaultPtr is a pointer to a struct
	val := reflect.ValueOf(defaultPtr)
	if val.Kind() != reflect.Ptr || val.Elem().Kind() != reflect.Struct {
		return "", fmt.Errorf("defaultPtr must be a pointer to a struct")
	}

	// Create a copy of the default struct to avoid modifying the original immediately
	defaultCopy := reflect.New(val.Elem().Type()).Interface()
	reflect.ValueOf(defaultCopy).Elem().Set(val.Elem())

	// Initialize defaults on the copy
	InitStructWithDefaults(defaultCopy)

	// Use StructToJSONString to produce a complete JSON representation (including zero values)
	defaultString, err := StructToJSONString(defaultCopy)
	if err != nil {
		return "", fmt.Errorf("failed to convert default struct to JSON: %w", err)
	}
	var defaultMap map[string]interface{}
	if err := json.Unmarshal([]byte(defaultString), &defaultMap); err != nil {
		return "", fmt.Errorf("failed to unmarshal default struct JSON: %w", err)
	}

	// If no input is provided, update defaultPtr and return defaults.
	if inputJSON == "" {
		reflect.ValueOf(defaultPtr).Elem().Set(reflect.ValueOf(defaultCopy).Elem())
		return defaultString, nil
	}

	// Unmarshal incoming JSON into a map.
	var incomingParams map[string]interface{}
	if err := json.Unmarshal([]byte(inputJSON), &incomingParams); err != nil {
		return "", fmt.Errorf("failed to unmarshal incoming JSON: %w", err)
	}

	// Recursively merge the incoming parameters with defaults
	mergedMap := recursiveMerge(defaultMap, incomingParams)

	// Marshal the merged map to JSON.
	mergedJSONBytes, err := json.Marshal(mergedMap)
	if err != nil {
		return "", fmt.Errorf("failed to marshal merged parameters: %w", err)
	}

	// Validate the merged JSON against the schema using the copy.
	if err := ValidateJSONAgainstSchemaOnPost(mergedJSONBytes, defaultCopy); err != nil {
		return "", fmt.Errorf("merged JSON validation error: %w", err)
	}

	// Create a new instance to unmarshal into, to ensure we properly handle null values
	resultCopy := reflect.New(val.Elem().Type()).Interface()

	// Unmarshal the merged JSON into the new instance
	if err := json.Unmarshal(mergedJSONBytes, resultCopy); err != nil {
		return "", fmt.Errorf("failed to unmarshal merged JSON into result: %w", err)
	}

	// Update the original defaultPtr with the merged result.
	reflect.ValueOf(defaultPtr).Elem().Set(reflect.ValueOf(resultCopy).Elem())

	return string(mergedJSONBytes), nil
}

// recursiveMerge merges source into target, handling nested maps recursively.
// Values from source take precedence over target.
// It properly handles embedded structs at any nesting level.
func recursiveMerge(target, source map[string]interface{}) map[string]interface{} {
	result := make(map[string]interface{})

	// First copy all target values to result
	for k, v := range target {
		result[k] = v
	}

	// Then merge source values, handling nested maps
	for k, sourceVal := range source {
		// If key doesn't exist in target, just use source value
		targetVal, exists := target[k]
		if !exists {
			result[k] = sourceVal
			continue
		}

		// If both values are maps, merge them recursively
		sourceMap, sourceIsMap := sourceVal.(map[string]interface{})
		targetMap, targetIsMap := targetVal.(map[string]interface{})

		if sourceIsMap && targetIsMap {
			result[k] = recursiveMerge(targetMap, sourceMap)
		} else if sourceIsMap && !targetIsMap {
			// If source is a map but target isn't, use source
			result[k] = sourceVal
		} else if !sourceIsMap && targetIsMap {
			// If target is a map but source isn't, use source (overwrite)
			result[k] = sourceVal
		} else if sourceSlice, ok := sourceVal.([]interface{}); ok {
			// Handle slices - replace the entire slice
			result[k] = sourceSlice
		} else {
			// Otherwise source value overrides target
			result[k] = sourceVal
		}
	}

	return result
}
