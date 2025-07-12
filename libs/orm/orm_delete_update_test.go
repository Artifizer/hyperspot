package orm

import (
	"fmt"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/hypernetix/hyperspot/libs/db"
	"github.com/hypernetix/hyperspot/libs/logging"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

// Create a test type to ensure we're testing with a clean slate
type TestDeleteStruct struct {
	ID          string `json:"id" gorm:"primaryKey"`
	Name        string `json:"name"`
	CreatedAtMs int64  `json:"created_at"`
}

// Create a test type to ensure we're testing with a clean slate
type TestUpdateStruct struct {
	ID          string `json:"id" gorm:"primaryKey"`
	Name        string `json:"name"`
	Value       int    `json:"value"`
	CreatedAtMs int64  `json:"created_at"`
}

// setupTestDB initializes an in-memory SQLite database and auto-migrates the test schema.
func setupTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	testDB, err := db.InitInMemorySQLite(nil)
	require.NoError(t, err, "Failed to connect to test DB")
	db.DB = testDB
	OrmInit(testDB)
	err = db.SafeAutoMigrate(testDB, &TestDeleteStruct{}, &TestUpdateStruct{})
	require.NoError(t, err, "Failed to migrate test database")

	logging.ForceLogLevel(logging.TraceLevel)
	db.ForceLogLevel(logging.TraceLevel)

	return testDB
}

// Helper function to count records
func countDelete(t *testing.T) int64 {
	var count int64
	db.DB.Model(&TestDeleteStruct{}).Count(&count)
	return count
}

func countUpdate(t *testing.T) int64 {
	var count int64
	db.DB.Model(&TestUpdateStruct{}).Count(&count)
	return count
}

func countValue(t *testing.T, v int) int64 {
	var count int64
	db.DB.Model(&TestUpdateStruct{}).Where("value = ?", v).Count(&count)
	return count
}

// TestOrmDeleteObjs verifies that OrmDeleteObjs correctly deletes records in batches
func TestOrmDeleteObjs(t *testing.T) {
	setupTestDB(t)

	// Helper function to create test records
	create := func(n int) {
		for i := 0; i < n; i++ {
			record := TestDeleteStruct{
				ID:          uuid.New().String(),
				Name:        fmt.Sprintf("Test Record %d", i),
				CreatedAtMs: time.Now().UnixMilli(),
			}
			result := db.DB.Create(&record)
			require.NoError(t, result.Error, "Failed to create test record")
		}
	}

	t.Run("DeleteAllRecords", func(t *testing.T) {
		// Clean up before test
		db.DB.Exec("DELETE FROM test_delete_structs")

		// Create test records
		create(5)

		// Verify records were created
		var count int64
		result := db.DB.Model(&TestDeleteStruct{}).Count(&count)
		require.NoError(t, result.Error, "Failed to count records: %s", result.Error)
		assert.Equal(t, int64(5), count, "Should have created 5 records")

		// Delete all records via query
		deleted, err := OrmDeleteObjs(db.DB.Model(&TestDeleteStruct{}), "id", 0, 2)
		require.NoError(t, err, "Failed to delete records: %v", err)
		assert.Equal(t, int64(5), deleted, "Should have deleted 5 records")
		assert.Equal(t, int64(0), countDelete(t))
	})

	t.Run("DeleteWithFilter", func(t *testing.T) {
		// Clean up before test
		db.DB.Exec("DELETE FROM test_delete_structs")

		// Create test records
		create(3)

		// Create a specific record to filter by
		specificRecord := TestDeleteStruct{
			ID:          uuid.New().String(),
			Name:        "Specific Record",
			CreatedAtMs: time.Now().UnixMilli(),
		}
		result := db.DB.Create(&specificRecord)
		require.NoError(t, result.Error, "Failed to create specific record")

		// Delete only the specific record
		filter := map[string]interface{}{
			"name": "Specific Record",
		}
		deleted, err := OrmDeleteObjs(db.DB.Model(&TestDeleteStruct{}).Where(filter), "id", 0, 10)
		require.NoError(t, err, "Failed to delete records with filter")
		assert.Equal(t, int64(1), deleted, "Should have deleted 1 record")

		// Verify the correct records were deleted
		var remaining []TestDeleteStruct
		db.DB.Find(&remaining)
		for _, r := range remaining {
			assert.NotEqual(t, r.Name, "Specific Record")
		}
	})

	t.Run("DeleteWithLimit", func(t *testing.T) {
		// Clean up before test
		db.DB.Exec("DELETE FROM test_delete_structs")

		// Create test records
		create(4)

		// Delete only 2 records
		deleted, err := OrmDeleteObjs(db.DB.Model(&TestDeleteStruct{}), "id", 2, 10)
		require.NoError(t, err)
		assert.Equal(t, int64(2), deleted)
		assert.Equal(t, int64(2), countDelete(t))
	})

	t.Run("InvalidQuery", func(t *testing.T) {
		_, err := OrmDeleteObjs(nil, "id", 0, 10)
		assert.Error(t, err)
	})
}

// TestOrmUpdateObjs verifies that OrmUpdateObjs correctly updates records in batches
func TestOrmUpdateObjs(t *testing.T) {
	setupTestDB(t)

	// Helper function to create test records
	create := func(n int) {
		for i := 0; i < n; i++ {
			record := TestUpdateStruct{
				ID:          uuid.New().String(),
				Name:        fmt.Sprintf("Test Record %d", i),
				Value:       i,
				CreatedAtMs: time.Now().UnixMilli(),
			}
			result := db.DB.Create(&record)
			require.NoError(t, result.Error, "Failed to create test record")
		}
	}

	t.Run("UpdateAllRecords", func(t *testing.T) {
		// Clean up before test
		db.DB.Exec("DELETE FROM test_update_structs")

		// Create test records
		create(4)

		// Verify records were created
		var count int64
		result := db.DB.Model(&TestUpdateStruct{}).Count(&count)
		require.NoError(t, result.Error, "Failed to count records")
		assert.Equal(t, int64(4), count, "Should have created 4 records")

		// Update all records
		updates := map[string]interface{}{
			"value": 9,
		}
		updated, err := OrmUpdateObjs(db.DB.Model(&TestUpdateStruct{}), "id", updates, 0, 2)
		require.NoError(t, err)
		assert.Equal(t, int64(4), updated)
		assert.Equal(t, int64(4), countValue(t, 9))
	})

	t.Run("UpdateWithEqualityFilter", func(t *testing.T) {
		// Clean up before test
		db.DB.Exec("DELETE FROM test_update_structs")

		// Create test records
		create(5)

		// Update only one record with value equal to 1
		updates := map[string]interface{}{"value": 42}
		filter := map[string]interface{}{"value": 1}
		updated, err := OrmUpdateObjs(db.DB.Model(&TestUpdateStruct{}).Where(filter), "id", updates, 0, 1)
		require.NoError(t, err)
		assert.Equal(t, int64(1), updated)
		assert.Equal(t, int64(1), countValue(t, 42))
	})

	t.Run("InvalidInputs", func(t *testing.T) {
		// Test with invalid query (nil pointer)
		_, err := OrmUpdateObjs(nil, "id", map[string]interface{}{"v": 1}, 0, 1)
		assert.Error(t, err)

		// Test with empty updates
		_, err = OrmUpdateObjs(db.DB.Model(&TestUpdateStruct{}), "id", map[string]interface{}{}, 0, 1)
		assert.Error(t, err)
	})
}
