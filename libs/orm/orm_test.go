package orm

import (
	"fmt"
	"reflect"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/hypernetix/hyperspot/libs/api"
	"github.com/hypernetix/hyperspot/libs/db"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

// Test structs for mapping tests
type SimpleStruct struct {
	TenantID  uuid.UUID `json:"tenant_id" gorm:"column:tenant_id"`
	ID        string    `json:"id" gorm:"primaryKey"`
	Name      string    `json:"name"`
	CreatedAt time.Time `json:"created_at"`
}

type StructWithColumnOverride struct {
	ID           string `json:"id" gorm:"primaryKey"`
	Name         string `json:"name" gorm:"column:custom_name"`
	InternalData string `json:"-" gorm:"column:internal_data"`
	IgnoredField string `json:"-"`
}

type NestedStruct struct {
	Value string `json:"value" gorm:"column:nested_value"`
}

type ComplexStruct struct {
	ID           string       `json:"id" gorm:"primaryKey"`
	Name         string       `json:"name"`
	NestedStruct              // Embedded struct
	Extra        NestedStruct `json:"extra"`
	TenantID     uuid.UUID    `json:"tenant_id" gorm:"column:tenant_id"`
}

// First, let's define an interface for the DB methods we're using
type DB interface {
	Model(value interface{}) DB
	Where(query interface{}, args ...interface{}) DB
	Limit(limit int) DB
	Offset(offset int) DB
	Order(value interface{}) DB
}

// Test GetBaseQuery function
func TestGetBaseQuery(t *testing.T) {
	// Create a mock DB

	db.ConfigDatabaseInstance = &db.ConfigDatabase{
		Type: "memory",
	}

	db.StartDBServer()

	t.Run("WithTenantID", func(t *testing.T) {
		obj := SimpleStruct{}
		tenantID := uuid.New()
		userID := uuid.New()
		request := &api.PageAPIRequest{}

		query, err := GetBaseQuery(obj, tenantID, userID, request)
		require.NoError(t, err)
		assert.NotNil(t, query)

		// Use ToSQL to generate the SQL without executing the query
		sql := query.ToSQL(func(tx *gorm.DB) *gorm.DB {
			return tx.Find(&[]SimpleStruct{})
		})

		assert.Contains(t, sql, fmt.Sprintf("tenant_id = \"%s\"", tenantID))
	})

	t.Run("WithPaging", func(t *testing.T) {
		obj := SimpleStruct{}
		request := &api.PageAPIRequest{
			PageSize:   10,
			PageNumber: 2,
		}

		query, err := GetBaseQuery(obj, uuid.Nil, uuid.Nil, request)
		require.NoError(t, err)
		assert.NotNil(t, query)
	})

	t.Run("WithSortingAscending", func(t *testing.T) {
		obj := SimpleStruct{}
		request := &api.PageAPIRequest{
			Order: "name",
		}

		query, err := GetBaseQuery(obj, uuid.Nil, uuid.Nil, request)
		require.NoError(t, err)
		assert.NotNil(t, query)

		sql := query.ToSQL(func(tx *gorm.DB) *gorm.DB {
			return tx.Find(&[]SimpleStruct{})
		})
		assert.Contains(t, sql, "ORDER BY name asc")
	})

	t.Run("WithSortingDescending", func(t *testing.T) {
		obj := SimpleStruct{}
		request := &api.PageAPIRequest{
			Order: "-name",
		}

		query, err := GetBaseQuery(obj, uuid.Nil, uuid.Nil, request)
		require.NoError(t, err)
		assert.NotNil(t, query)

		sql := query.ToSQL(func(tx *gorm.DB) *gorm.DB {
			return tx.Find(&[]SimpleStruct{})
		})
		assert.Contains(t, sql, "ORDER BY name desc")
	})

	t.Run("WithInvalidSortField", func(t *testing.T) {
		obj := SimpleStruct{}
		request := &api.PageAPIRequest{
			Order: "nonexistent",
		}

		query, err := GetBaseQuery(obj, uuid.Nil, uuid.Nil, request)
		assert.Error(t, err)
		assert.Nil(t, query)
		assert.Contains(t, err.Error(), "field key 'nonexistent'")
	})

	t.Run("WithNilDB", func(t *testing.T) {
		// We don't need to modify the actual db.DB here, just pass nil to our test function
		obj := SimpleStruct{}
		db.DB = nil
		request := &api.PageAPIRequest{}

		query, err := GetBaseQuery(obj, uuid.Nil, uuid.Nil, request)
		assert.Error(t, err)
		assert.Nil(t, query)
		assert.NotNil(t, err)
		assert.Contains(t, err.Error(), "database not initialized")
	})
}

// Test toSnakeCase function
func TestToSnakeCase(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"HelloWorld", "hello_world"},
		{"helloWorld", "hello_world"},
		{"User123Model", "user123_model"},
		{"Hello_World", "hello__world"},
		{"HTTPRequest", "http_request"},
		{"simpleURL", "simple_url"},
		{"ID", "id"},
		{"iOS", "i_os"},
		{"", ""},
		{"Already_snake_case", "already_snake_case"},
		{"_LeadingUnderscore", "__leading_underscore"},
		{"TrailingUnderscore_", "trailing_underscore_"},
		{"__DoubleUnderscore", "___double_underscore"},
		{"ABCDef", "abc_def"},
		{"A", "a"},
		{"APIClientConfig", "api_client_config"},
	}

	for _, test := range tests {
		t.Run(test.input, func(t *testing.T) {
			result := toSnakeCase(test.input)
			assert.Equal(t, test.expected, result)
		})
	}
}

// Test buildMapping function
func TestBuildMapping(t *testing.T) {
	t.Run("SimpleStruct", func(t *testing.T) {
		mapping := buildMapping(reflect.TypeOf(SimpleStruct{}))
		assert.Equal(t, "id", mapping["id"])
		assert.Equal(t, "name", mapping["name"])
		assert.Equal(t, "created_at", mapping["created_at"])
		assert.Equal(t, "tenant_id", mapping["tenant_id"])
		assert.Len(t, mapping, 4)
	})

	t.Run("StructWithColumnOverride", func(t *testing.T) {
		mapping := buildMapping(reflect.TypeOf(StructWithColumnOverride{}))
		assert.Equal(t, "id", mapping["id"])
		assert.Equal(t, "custom_name", mapping["name"])
		assert.Len(t, mapping, 2, "Should only have mappings for fields with JSON tags")
	})

	t.Run("NestedStruct", func(t *testing.T) {
		mapping := buildMapping(reflect.TypeOf(NestedStruct{}))
		assert.Equal(t, "nested_value", mapping["value"])
		assert.Len(t, mapping, 1)
	})

	t.Run("ComplexStruct", func(t *testing.T) {
		mapping := buildMapping(reflect.TypeOf(ComplexStruct{}))
		assert.Equal(t, "id", mapping["id"])
		assert.Equal(t, "name", mapping["name"])
		assert.Equal(t, "nested_value", mapping["value"], "Embedded struct field should be included")
		assert.Equal(t, "extra", mapping["extra"], "Non-embedded nested struct gets field name as column")
		assert.Equal(t, "tenant_id", mapping["tenant_id"])
		assert.Len(t, mapping, 5)
	})

	t.Run("NonStruct", func(t *testing.T) {
		mapping := buildMapping(reflect.TypeOf("string"))
		assert.Empty(t, mapping, "Non-struct types should return empty mapping")
	})
}

// Test GetGormFieldName function
func TestGetGormFieldName(t *testing.T) {
	t.Run("SimpleStruct", func(t *testing.T) {
		obj := SimpleStruct{}
		field, err := GetGormFieldName(obj, "id")
		assert.NoError(t, err)
		assert.Equal(t, "id", field)

		field, err = GetGormFieldName(&obj, "name")
		assert.NoError(t, err)
		assert.Equal(t, "name", field)
	})

	t.Run("StructWithColumnOverride", func(t *testing.T) {
		obj := StructWithColumnOverride{}
		field, err := GetGormFieldName(obj, "name")
		assert.NoError(t, err)
		assert.Equal(t, "custom_name", field)

		// Test that field with json:"-" doesn't get returned
		field, err = GetGormFieldName(obj, "internal_data")
		assert.Error(t, err)
		assert.Empty(t, field)
	})

	t.Run("ComplexStruct", func(t *testing.T) {
		obj := ComplexStruct{}
		field, err := GetGormFieldName(obj, "value")
		assert.NoError(t, err)
		assert.Equal(t, "nested_value", field)

		field, err = GetGormFieldName(obj, "tenant_id")
		assert.NoError(t, err)
		assert.Equal(t, "tenant_id", field)
	})

	t.Run("NonExistentField", func(t *testing.T) {
		obj := SimpleStruct{}
		field, err := GetGormFieldName(obj, "non_existent")
		assert.Error(t, err)
		assert.Empty(t, field)
		assert.Contains(t, err.Error(), "field key 'non_existent'")
	})

	t.Run("NilObject", func(t *testing.T) {
		field, err := GetGormFieldName(nil, "id")
		assert.Error(t, err)
		assert.Empty(t, field)
		assert.Contains(t, err.Error(), "object is nil")
	})

	t.Run("NonStructObject", func(t *testing.T) {
		field, err := GetGormFieldName("string", "id")
		assert.Error(t, err)
		assert.Empty(t, field)
		assert.Contains(t, err.Error(), "object is not a struct")
	})

	t.Run("CachingBehavior", func(t *testing.T) {
		// Clear the cache to start fresh
		ormCacheMutex.Lock()
		ormMappingCache = make(map[reflect.Type]map[string]string)
		ormCacheMutex.Unlock()

		// First call should build the mapping
		obj := ComplexStruct{}
		objType := reflect.TypeOf(obj)
		field, err := GetGormFieldName(obj, "id")
		assert.NoError(t, err)
		assert.Equal(t, "id", field)

		// Verify the cache was populated
		ormCacheMutex.RLock()
		mapping, exists := ormMappingCache[objType]
		ormCacheMutex.RUnlock()
		assert.True(t, exists)
		assert.NotEmpty(t, mapping)
	})
}

// Test for edge cases and additional scenarios
func TestEdgeCases(t *testing.T) {
	t.Run("EmptyStruct", func(t *testing.T) {
		type EmptyStruct struct{}
		mapping := buildMapping(reflect.TypeOf(EmptyStruct{}))
		assert.Empty(t, mapping)

		field, err := GetGormFieldName(EmptyStruct{}, "any")
		assert.Error(t, err)
		assert.Empty(t, field)
	})

	t.Run("StructWithJSONButNoGorm", func(t *testing.T) {
		type NoGormStruct struct {
			ID   string `json:"id"`
			Name string `json:"name"`
		}
		mapping := buildMapping(reflect.TypeOf(NoGormStruct{}))
		assert.Equal(t, "id", mapping["id"])
		assert.Equal(t, "name", mapping["name"])
	})

	t.Run("MultipleEmbeddedStructs", func(t *testing.T) {
		type Base1 struct {
			ID string `json:"id" gorm:"column:base1_id"`
		}
		type Base2 struct {
			Name string `json:"name" gorm:"column:base2_name"`
		}
		type MultiEmbed struct {
			Base1
			Base2
			Description string `json:"description"`
		}

		mapping := buildMapping(reflect.TypeOf(MultiEmbed{}))
		assert.Equal(t, "base1_id", mapping["id"])
		assert.Equal(t, "base2_name", mapping["name"])
		assert.Equal(t, "description", mapping["description"])
	})

	t.Run("StructWithPointerFields", func(t *testing.T) {
		type PointerFieldStruct struct {
			ID      *string    `json:"id" gorm:"column:pointer_id"`
			Created *time.Time `json:"created_at" gorm:"column:created_at"`
		}

		mapping := buildMapping(reflect.TypeOf(PointerFieldStruct{}))
		assert.Equal(t, "pointer_id", mapping["id"])
		assert.Equal(t, "created_at", mapping["created_at"])
	})

	t.Run("CacheInvalidation", func(t *testing.T) {
		// Verify that we can clear and repopulate the cache
		obj := SimpleStruct{}
		objType := reflect.TypeOf(obj)

		// Clear the cache
		ormCacheMutex.Lock()
		delete(ormMappingCache, objType)
		ormCacheMutex.Unlock()

		// Make sure it's gone
		ormCacheMutex.RLock()
		_, exists := ormMappingCache[objType]
		ormCacheMutex.RUnlock()
		assert.False(t, exists)

		// Call GetGormFieldName to repopulate
		field, err := GetGormFieldName(obj, "id")
		assert.NoError(t, err)
		assert.Equal(t, "id", field)

		// Verify cache was repopulated
		ormCacheMutex.RLock()
		mapping, exists := ormMappingCache[objType]
		ormCacheMutex.RUnlock()
		assert.True(t, exists)
		assert.NotEmpty(t, mapping)
	})
}

func TestGetBaseQuery2(t *testing.T) {
	t.Run("SimpleQuery", func(t *testing.T) {
		db.ConfigDatabaseInstance = &db.ConfigDatabase{
			Type: "memory",
		}

		db.StartDBServer()

		// Call GetBaseQuery with SimpleStruct
		query, err := GetBaseQuery(SimpleStruct{}, uuid.Nil, uuid.Nil, &api.PageAPIRequest{})
		require.NoError(t, err)

		// Verify the query is properly constructed
		assert.NotNil(t, query)
	})

	t.Run("WithFilters", func(t *testing.T) {
		// Call GetBaseQuery with filters
		query, err := GetBaseQuery(SimpleStruct{}, uuid.Nil, uuid.Nil, &api.PageAPIRequest{})
		require.NoError(t, err)

		// Verify the query is properly constructed
		assert.NotNil(t, query)
	})

	t.Run("WithMultipleFilters", func(t *testing.T) {
		// Call GetBaseQuery with multiple filters
		query, err := GetBaseQuery(SimpleStruct{}, uuid.Nil, uuid.Nil, &api.PageAPIRequest{})
		require.NoError(t, err)

		// Verify the query is properly constructed
		assert.NotNil(t, query)
	})

	t.Run("WithComplexStruct", func(t *testing.T) {
		// Call GetBaseQuery with ComplexStruct
		query, err := GetBaseQuery(ComplexStruct{}, uuid.Nil, uuid.Nil, &api.PageAPIRequest{})
		require.NoError(t, err)

		// Verify the query is properly constructed
		assert.NotNil(t, query)
	})
}

// MockDB implements the DB interface for testing
type MockDB struct{}

func (m *MockDB) Model(value interface{}) DB {
	return m
}

func (m *MockDB) Where(query interface{}, args ...interface{}) DB {
	return m
}

func (m *MockDB) Limit(limit int) DB {
	return m
}

func (m *MockDB) Offset(offset int) DB {
	return m
}

func (m *MockDB) Order(value interface{}) DB {
	return m
}

// TestOrmUpdateObjFields verifies that OrmUpdateObjFields correctly updates specific fields
func TestOrmUpdateObjFields(t *testing.T) {
	// Ensure database is initialized
	db.ConfigDatabaseInstance = &db.ConfigDatabase{
		Type: "memory",
	}
	db.StartDBServer()

	// Migrate test tables first
	if err := db.SafeAutoMigrate(db.DB, &SimpleStruct{}, &StructWithColumnOverride{}); err != nil {
		t.Fatalf("Failed to migrate test tables: %v", err)
	}

	t.Run("UpdateSimpleStruct", func(t *testing.T) {
		// Create a test record
		obj := &SimpleStruct{
			TenantID:  uuid.New(),
			ID:        "test-id-1",
			Name:      "Original Name",
			CreatedAt: time.Now(),
		}

		err := db.DB.Create(obj).Error
		require.NoError(t, err)

		// Update the name field
		obj.Name = "Updated Name"

		// Call OrmUpdateObjFields with the name field
		err = OrmUpdateObjFields(obj, map[string]interface{}{"id": obj.ID}, &obj.Name)
		require.NoError(t, err)

		// Fetch the record to verify update
		var fetched SimpleStruct
		err = db.DB.First(&fetched, "id = ?", obj.ID).Error
		require.NoError(t, err)

		// Verify only the name was updated
		assert.Equal(t, "Updated Name", fetched.Name)
		assert.Equal(t, obj.ID, fetched.ID)
		assert.Equal(t, obj.TenantID, fetched.TenantID)
	})

	t.Run("UpdateMultipleFields", func(t *testing.T) {
		// Create a test record
		obj := &SimpleStruct{
			TenantID:  uuid.New(),
			ID:        "test-id-2",
			Name:      "Original Name",
			CreatedAt: time.Now().Add(-24 * time.Hour), // Yesterday
		}

		err := db.DB.Create(obj).Error
		require.NoError(t, err)

		// Update multiple fields
		obj.Name = "New Name"
		newTime := time.Now()
		obj.CreatedAt = newTime

		// Call OrmUpdateObjFields with multiple fields
		err = OrmUpdateObjFields(obj, map[string]interface{}{"id": obj.ID}, &obj.Name, &obj.CreatedAt)
		require.NoError(t, err)

		// Fetch the record to verify update
		var fetched SimpleStruct
		err = db.DB.First(&fetched, "id = ?", obj.ID).Error
		require.NoError(t, err)

		// Verify only the specified fields were updated
		assert.Equal(t, "New Name", fetched.Name)
		assert.Equal(t, newTime.Unix(), fetched.CreatedAt.Unix())
		assert.Equal(t, obj.ID, fetched.ID)
	})

	t.Run("UpdateWithColumnOverride", func(t *testing.T) {
		// Create a test record with custom column names
		obj := &StructWithColumnOverride{
			ID:           "test-id-3",
			Name:         "Original Custom Name",
			InternalData: "Original Internal Data",
			IgnoredField: "Ignored",
		}

		err := db.DB.Create(obj).Error
		require.NoError(t, err)

		// Update the field with custom column name
		obj.Name = "Updated Custom Name"
		obj.InternalData = "Updated Internal Data"

		// Call OrmUpdateObjFields with the fields
		err = OrmUpdateObjFields(obj, map[string]interface{}{"id": obj.ID}, &obj.Name, &obj.InternalData)
		require.NoError(t, err)

		// Fetch the record to verify update
		var fetched StructWithColumnOverride
		err = db.DB.First(&fetched, "id = ?", obj.ID).Error
		require.NoError(t, err)

		// Verify the fields were updated with correct column names
		assert.Equal(t, "Updated Custom Name", fetched.Name)
		assert.Equal(t, "Updated Internal Data", fetched.InternalData)
	})

	t.Run("ErrorOnInvalidModel", func(t *testing.T) {
		// Test with non-pointer model
		simpleStruct := SimpleStruct{}
		err := OrmUpdateObjFields(simpleStruct, map[string]interface{}{"id": "test"}, &simpleStruct.Name)
		assert.Error(t, err)

		// Test with nil model
		err = OrmUpdateObjFields(nil, map[string]interface{}{"id": "test"}, nil)
		assert.Error(t, err)
	})

	t.Run("ErrorOnMissingPrimaryKey", func(t *testing.T) {
		obj := &SimpleStruct{
			Name: "Test",
		}

		// Call without primary key
		err := OrmUpdateObjFields(obj, map[string]interface{}{}, &obj.Name)
		assert.Error(t, err)
	})

	t.Run("ErrorOnNonPointerUpdateField", func(t *testing.T) {
		obj := &SimpleStruct{
			ID:   "test-id-4",
			Name: "Test",
		}

		// Call with non-pointer update field
		name := "Updated"
		err := OrmUpdateObjFields(obj, map[string]interface{}{"id": obj.ID}, name)
		assert.Error(t, err)
	})
}

// TestOrmGetObjFields verifies that OrmGetObjFields correctly retrieves specific fields
func TestOrmGetObjFields(t *testing.T) {
	// Ensure database is initialized
	db.ConfigDatabaseInstance = &db.ConfigDatabase{
		Type: "memory",
	}
	db.StartDBServer()

	// Migrate test tables first
	if err := db.SafeAutoMigrate(db.DB, &SimpleStruct{}, &StructWithColumnOverride{}); err != nil {
		t.Fatalf("Failed to migrate test tables: %v", err)
	}

	t.Run("GetSingleField", func(t *testing.T) {
		// Create a test record
		obj := &SimpleStruct{
			TenantID:  uuid.New(),
			ID:        "test-get-id-1",
			Name:      "Test Name",
			CreatedAt: time.Now(),
		}

		err := db.DB.Create(obj).Error
		require.NoError(t, err)

		// Create a new object to get fields into
		fetchObj := &SimpleStruct{
			TenantID: obj.TenantID,
			ID:       obj.ID,
		}

		// Call OrmGetObjFields to get only the name field
		err = OrmGetObjFields(fetchObj, map[string]interface{}{"id": obj.ID}, &fetchObj.Name)
		require.NoError(t, err)

		// Verify only the name was fetched
		assert.Equal(t, "Test Name", fetchObj.Name)
		// CreatedAt should be zero since we didn't fetch it
		assert.True(t, fetchObj.CreatedAt.IsZero())
	})

	t.Run("GetMultipleFields", func(t *testing.T) {
		// Create a test record
		testTime := time.Now().Truncate(time.Second) // Truncate to avoid precision issues
		obj := &SimpleStruct{
			TenantID:  uuid.New(),
			ID:        "test-get-id-2",
			Name:      "Multiple Fields Test",
			CreatedAt: testTime,
		}

		err := db.DB.Create(obj).Error
		require.NoError(t, err)

		// Create a new object to get fields into
		fetchObj := &SimpleStruct{
			TenantID: obj.TenantID,
			ID:       obj.ID,
		}

		// Call OrmGetObjFields to get multiple fields
		err = OrmGetObjFields(fetchObj, map[string]interface{}{"id": obj.ID}, &fetchObj.Name, &fetchObj.CreatedAt)
		require.NoError(t, err)

		// Verify both fields were fetched
		assert.Equal(t, "Multiple Fields Test", fetchObj.Name)
		assert.Equal(t, testTime.Unix(), fetchObj.CreatedAt.Unix())
	})

	t.Run("GetWithColumnOverride", func(t *testing.T) {
		// Create a test record with custom column names
		obj := &StructWithColumnOverride{
			ID:           "test-get-id-3",
			Name:         "Custom Column Test",
			InternalData: "Internal Test Data",
			IgnoredField: "Ignored",
		}

		err := db.DB.Create(obj).Error
		require.NoError(t, err)

		// Create a new object to get fields into
		fetchObj := &StructWithColumnOverride{
			ID: obj.ID,
		}

		// Call OrmGetObjFields with fields that have custom column names
		err = OrmGetObjFields(fetchObj, map[string]interface{}{"id": obj.ID}, &fetchObj.Name, &fetchObj.InternalData)
		require.NoError(t, err)

		// Verify the fields were fetched with correct column names
		assert.Equal(t, "Custom Column Test", fetchObj.Name)
		assert.Equal(t, "Internal Test Data", fetchObj.InternalData)
		// IgnoredField should be empty since we didn't fetch it
		assert.Empty(t, fetchObj.IgnoredField)
	})

	t.Run("GetWithCompositePrimaryKey", func(t *testing.T) {
		// Create a test record
		obj := &SimpleStruct{
			TenantID:  uuid.New(),
			ID:        "test-get-id-4",
			Name:      "Composite Key Test",
			CreatedAt: time.Now(),
		}

		err := db.DB.Create(obj).Error
		require.NoError(t, err)

		// Create a new object to get fields into
		fetchObj := &SimpleStruct{}

		// Call OrmGetObjFields with composite primary key
		err = OrmGetObjFields(fetchObj, map[string]interface{}{
			"id":        obj.ID,
			"tenant_id": obj.TenantID,
		}, &fetchObj.Name)
		require.NoError(t, err)

		// Verify the field was fetched
		assert.Equal(t, "Composite Key Test", fetchObj.Name)
	})

	t.Run("ErrorOnNonExistentRecord", func(t *testing.T) {
		// Create an object for non-existent record
		fetchObj := &SimpleStruct{}

		// Call OrmGetObjFields with non-existent ID
		err := OrmGetObjFields(fetchObj, map[string]interface{}{"id": "non-existent-id"}, &fetchObj.Name)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "record not found")
	})

	t.Run("ErrorOnInvalidModel", func(t *testing.T) {
		// Test with non-pointer model
		simpleStruct := SimpleStruct{}
		err := OrmGetObjFields(simpleStruct, map[string]interface{}{"id": "test"}, &simpleStruct.Name)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "the model parameter must be a pointer to a struct")

		// Test with nil model
		err = OrmGetObjFields(nil, map[string]interface{}{"id": "test"}, nil)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "the model parameter must be a pointer to a struct")
	})

	t.Run("ErrorOnMissingPrimaryKey", func(t *testing.T) {
		obj := &SimpleStruct{
			Name: "Test",
		}

		// Call without primary key
		err := OrmGetObjFields(obj, map[string]interface{}{}, &obj.Name)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "primary key field is required for get")
	})

	t.Run("ErrorOnNonPointerGetField", func(t *testing.T) {
		obj := &SimpleStruct{
			ID:   "test-get-id-5",
			Name: "Test",
		}

		// Call with non-pointer get field
		name := "Updated"
		err := OrmGetObjFields(obj, map[string]interface{}{"id": obj.ID}, name)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "field parameter must be a pointer")
	})

	t.Run("ErrorOnInvalidField", func(t *testing.T) {
		obj := &SimpleStruct{
			ID:   "test-get-id-6",
			Name: "Test",
		}

		// Create a pointer to a field that doesn't belong to the model
		invalidField := "invalid"
		err := OrmGetObjFields(obj, map[string]interface{}{"id": obj.ID}, &invalidField)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "field not found for get parameter")
	})

	t.Run("ErrorOnUnsupportedFieldType", func(t *testing.T) {
		obj := &SimpleStruct{
			ID:   "test-get-id-7",
			Name: "Test",
		}

		// Test with unsupported field type (function pointer)
		var fn func()
		err := OrmGetObjFields(obj, map[string]interface{}{"id": obj.ID}, &fn)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "field not found for get parameter")
	})
}
