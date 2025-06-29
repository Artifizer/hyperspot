package orm

import (
	"fmt"
	"reflect"
	"regexp"
	"strings"
	"sync"

	"github.com/google/uuid"
	"github.com/hypernetix/hyperspot/libs/api"
	"github.com/hypernetix/hyperspot/libs/auth"
	"github.com/hypernetix/hyperspot/libs/db"
	"github.com/hypernetix/hyperspot/libs/errorx"
	"gorm.io/gorm"
)

func OrmInit(db *gorm.DB) error {
	// Add this to your initialization code
	err := db.Callback().Update().Before("gorm:update").Register("check_limit_on_update", func(db *gorm.DB) {
		// Check if there's a limit clause
		if db.Statement == nil || db.Statement.Clauses == nil {
			return
		}
		if _, exists := db.Statement.Clauses["LIMIT"]; exists {
			db.AddError(fmt.Errorf("LIMIT clause is not supported for UPDATE operations, use OrmUpdateObjs() instead"))
			return
		}
	})
	if err != nil {
		return fmt.Errorf("failed to register update callback: %w", err)
	}

	err = db.Callback().Delete().Before("gorm:delete").Register("check_limit_on_delete", func(db *gorm.DB) {
		// Check if there's a limit clause
		if db.Statement == nil || db.Statement.Clauses == nil {
			return
		}
		if _, exists := db.Statement.Clauses["LIMIT"]; exists {
			db.AddError(fmt.Errorf("LIMIT clause is not supported for DELETE operations, use OrmDeleteObjs() instead"))
			return
		}
	})
	if err != nil {
		return fmt.Errorf("failed to register delete callback: %w", err)
	}

	return nil
}

// ormMappingCache caches (per object type) the mapping from a field key (e.g., the JSON tag)
// to the actual column name as determined by the "gorm" struct tag.
var (
	ormMappingCache = make(map[reflect.Type]map[string]string)
	ormCacheMutex   sync.RWMutex
)

// toSnakeCase converts CamelCase to snake_case.
func toSnakeCase(s string) string {
	var matchFirstCap = regexp.MustCompile("(.)([A-Z][a-z]+)")
	var matchAllCap = regexp.MustCompile("([a-z0-9])([A-Z])")
	snake := matchFirstCap.ReplaceAllString(s, "${1}_${2}")
	snake = matchAllCap.ReplaceAllString(snake, "${1}_${2}")
	return strings.ToLower(snake)
}

// buildMapping recursively traverses struct fields (including embedded ones) to build a mapping
// from the JSON tag value to the GORM column name. If a gorm tag specifies a column override via "column:",
// then that value is used; otherwise it falls back to converting the field name to snake_case.
func buildMapping(t reflect.Type) map[string]string {
	mapping := make(map[string]string)
	if t.Kind() != reflect.Struct {
		return mapping
	}

	// Iterate over the struct fields.
	for i := 0; i < t.NumField(); i++ {
		field := t.Field(i)

		// If the field is anonymous (embedded) and is a struct, process its fields recursively.
		if field.Anonymous && field.Type.Kind() == reflect.Struct {
			embeddedMapping := buildMapping(field.Type)
			for k, v := range embeddedMapping {
				// Only insert if not already present (outer fields take precedence).
				if _, exists := mapping[k]; !exists {
					mapping[k] = v
				}
			}
			continue
		}

		// Retrieve the JSON tag and skip if empty or ignored.
		jsonTag := field.Tag.Get("json")
		if jsonTag == "" || jsonTag == "-" {
			continue
		}
		key := strings.Split(jsonTag, ",")[0]

		// Check for a column override in the gorm tag.
		gormTag := field.Tag.Get("gorm")
		col := ""
		if gormTag != "" {
			parts := strings.Split(gormTag, ";")
			for _, part := range parts {
				if strings.HasPrefix(part, "column:") {
					col = strings.TrimPrefix(part, "column:")
					break
				}
			}
		}
		if col == "" {
			col = toSnakeCase(field.Name)
		}
		mapping[key] = col
	}
	return mapping
}

// GetGormFieldName returns the database column name (as determined by gorm tags) for the given field key
// in the provided object (or pointer to object). The object must be a struct or pointer to a struct.
// The function caches each object's mapping (from field key to column name) protected by a mutex.
// If the field key is not found, it returns an empty string.
func GetGormFieldName(object interface{}, fieldKey string) (string, errorx.Error) {
	t := reflect.TypeOf(object)
	if t == nil {
		return "", errorx.NewErrorx("object is nil")
	}
	if t.Kind() == reflect.Ptr {
		t = t.Elem()
	}
	if t.Kind() != reflect.Struct {
		return "", errorx.NewErrorx("object is not a struct")
	}

	// Check the cache for the mapping
	ormCacheMutex.RLock()
	mapping, ok := ormMappingCache[t]
	ormCacheMutex.RUnlock()
	if !ok {
		// Build mapping if not already cached
		mapping = buildMapping(t)
		ormCacheMutex.Lock()
		ormMappingCache[t] = mapping
		ormCacheMutex.Unlock()
	}

	// Look up the provided key in the mapping and return it
	if col, found := mapping[fieldKey]; found {
		return col, nil
	}
	return "", errorx.NewErrBadRequest(fmt.Sprintf("field key '%s' not found in mapping for object type '%s'", fieldKey, t.Name()))
}

func GetBaseQuery(obj interface{}, tenantID uuid.UUID, userID uuid.UUID, request *api.PageAPIRequest) (*gorm.DB, errorx.Error) {
	if db.DB == nil {
		return nil, errorx.NewErrInternalServerError("database not initialized")
	}

	query := db.DB.Model(obj)

	if tenantID != uuid.Nil {
		query = query.Where("tenant_id = ?", tenantID)
	}

	if userID != uuid.Nil {
		query = query.Where("user_id = ?", userID)
	}

	if request != nil && request.PageSize > 0 {
		query = query.Limit(request.PageSize)
	}

	if request != nil && request.PageNumber > 0 {
		query = query.Offset(request.GetOffset())
	}

	order := "asc"
	orderBy := ""
	if request != nil && request.Order != "" {
		orderBy = request.Order
	}

	if strings.HasPrefix(orderBy, "-") {
		order = "desc"
		orderBy = strings.TrimPrefix(orderBy, "-")
	}

	if orderBy != "" {
		// Only apply sorting if we have a valid column name obtained from utils.GetGormFieldName.
		orderByField, err := GetGormFieldName(obj, orderBy)
		if err != nil {
			return nil, err
		}

		if orderByField != "" {
			query = query.Order(fmt.Sprintf("%s %s", orderByField, order))
		}
	}

	return query, nil
}

// The TUU version - the same as GetBaseQuery() but with tenant, user, undeleted filter applied
func GetBaseQueryTUU(obj interface{}, request *api.PageAPIRequest) (*gorm.DB, errorx.Error) {
	query, errx := GetBaseQuery(obj, auth.GetTenantID(), auth.GetUserID(), request)
	if errx != nil {
		return nil, errx
	}

	query = query.Where("is_deleted = ?", false)

	return query, nil
}

// parseGormColumnName extracts the column name from a GORM tag.
// If a "column:" specification exists, it returns that value;
// otherwise, it falls back to the default field name.
func parseGormColumnName(tag, defaultName string) string {
	if tag == "" {
		return defaultName
	}
	parts := strings.Split(tag, ";")
	for _, part := range parts {
		if strings.HasPrefix(part, "column:") {
			return strings.TrimPrefix(part, "column:")
		}
	}
	return defaultName
}

// OrmUpdateObjFields accepts a model and a variable list of pointers to fields that need updating.
// It uses reflection to determine the corresponding GORM column name for each field,
// builds an update map, and performs the update via GORM.
func OrmUpdateObjFields(model interface{}, pkFields map[string]interface{}, updates ...interface{}) errorx.Error {
	// Recursively unwrap pointers until we get to a pointer to a struct
	modelValue := reflect.ValueOf(model)
	for modelValue.Kind() == reflect.Ptr && modelValue.Elem().Kind() == reflect.Ptr {
		modelValue = modelValue.Elem()
		model = modelValue.Interface()
	}

	if db.DB == nil {
		return errorx.NewErrInternalServerError("database is not initialized")
	}
	query := db.DB.Model(model)
	rv := reflect.ValueOf(model)
	if rv.Kind() != reflect.Ptr || rv.Elem().Kind() != reflect.Struct {
		return errorx.NewErrInternalServerError(fmt.Sprintf("the model parameter must be a pointer to a struct, got %T", model))
	}
	st := rv.Elem()
	typ := st.Type()
	updateMap := make(map[string]interface{})

	// Validate and add the primary key WHERE condition
	if len(pkFields) == 0 {
		return errorx.NewErrInternalServerError("primary key field is required for update")
	}

	// Add WHERE condition for the primary key
	for field, value := range pkFields {
		query = query.Where(field+" = ?", value)
	}

	// Loop through each provided update pointer.
	for _, updatePtr := range updates {
		uVal := reflect.ValueOf(updatePtr)
		if uVal.Kind() != reflect.Ptr {
			return errorx.NewErrInternalServerError("update parameter must be a pointer")
		}

		// Try to find the field in the struct hierarchy
		found := false
		fieldInfo, err := findFieldByPointer(st, typ, updatePtr)
		if err == nil {
			found = true
			gormTag := fieldInfo.Tag.Get("gorm")
			colName := parseGormColumnName(gormTag, fieldInfo.Name)
			// Get the actual value from the original pointer
			updateMap[colName] = reflect.ValueOf(updatePtr).Elem().Interface()
		}

		if !found {
			return errorx.NewErrBadRequest(fmt.Sprintf("field not found for update parameter: %v", updatePtr))
		}
	}
	// Use GORM to update only the specified fields.
	err := query.Updates(updateMap).Error
	if err != nil {
		return errorx.NewErrInternalServerError(err.Error())
	}
	return nil
}

// findFieldByPointer recursively searches for a field matching the given pointer
// in a struct and its embedded fields.
func findFieldByPointer(structVal reflect.Value, structType reflect.Type, targetPtr interface{}) (reflect.StructField, error) {
	// Check direct fields in the current struct
	for i := 0; i < structVal.NumField(); i++ {
		fieldVal := structVal.Field(i)
		fieldType := structType.Field(i)

		// Check if this field is addressable and matches our pointer
		if fieldVal.CanAddr() && fieldVal.Addr().Interface() == targetPtr {
			return fieldType, nil
		}

		// If this is an embedded struct, recursively search inside it
		if fieldType.Anonymous && fieldVal.Kind() == reflect.Struct {
			// Recursively check embedded struct
			embeddedField, err := findFieldByPointer(fieldVal, fieldType.Type, targetPtr)
			if err == nil {
				return embeddedField, nil
			}
			// Error means not found in this embedded struct, continue searching
		}
	}

	return reflect.StructField{}, fmt.Errorf("field not found")
}
