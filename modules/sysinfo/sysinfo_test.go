package sysinfo_test

import (
	"testing"

	"github.com/google/uuid"
	"github.com/hypernetix/hyperspot/libs/db"
	"github.com/hypernetix/hyperspot/libs/utils"
	"github.com/hypernetix/hyperspot/modules/sysinfo"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

// Setup test database
func setupTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	testDB, err := db.InitInMemorySQLite(nil)
	require.NoError(t, err, "Failed to connect to test DB")

	db.SetDB(testDB)
	err = db.SafeAutoMigrate(testDB,
		&utils.SystemInfo{},
	)
	require.NoError(t, err, "Failed to migrate test database")

	return testDB
}

// MockCollector is a mock implementation of the system info collector
type MockCollector struct {
	info *utils.SystemInfo
	err  error
}

func (c *MockCollector) CollectSysInfo() (*utils.SystemInfo, error) {
	return c.info, c.err
}

// MockDB is a mock implementation of the database interface
type MockDB struct {
	sysInfo *utils.SystemInfo
	err     error
}

func (db *MockDB) Where(query interface{}, args ...interface{}) *gorm.DB {
	return &gorm.DB{}
}

func (db *MockDB) First(out interface{}, conds ...interface{}) *gorm.DB {
	if db.err != nil {
		return &gorm.DB{Error: db.err}
	}
	if sysInfo, ok := out.(*utils.SystemInfo); ok {
		*sysInfo = *db.sysInfo
	}
	return &gorm.DB{}
}

func (db *MockDB) Save(value interface{}) *gorm.DB {
	return &gorm.DB{Error: db.err}
}

// createTestSystemInfo creates a test SystemInfo with predefined values
func createTestSystemInfo() *utils.SystemInfo {
	return &utils.SystemInfo{
		ID: uuid.MustParse("2a701d75-0aac-4fd9-9fe2-df26c372650e"),
		OS: struct {
			Name    string `json:"name" gorm:"column:os_name"`
			Version string `json:"version" gorm:"column:os_version"`
			Arch    string `json:"arch" gorm:"column:os_arch"`
		}{
			Name:    "test-os",
			Version: "1.0",
			Arch:    "test-arch",
		},
		CPU: struct {
			Model     string  `json:"model" gorm:"column:cpu_model"`
			NumCPUs   uint    `json:"num_cpus" gorm:"column:cpu_num_cpus"`
			Cores     uint    `json:"cores" gorm:"column:cpu_cores"`
			Frequency float64 `json:"frequency_mhz" gorm:"column:cpu_frequency_mhz"`
		}{
			Model:     "test-cpu",
			NumCPUs:   4,
			Cores:     4,
			Frequency: 2000,
		},
	}
}

func TestGetSysInfo(t *testing.T) {
	// This is an integration test since we can't mock the system calls
	info, err := sysinfo.GetSysInfo()
	assert.NoError(t, err)
	assert.NotNil(t, info)
	assert.NotEqual(t, uuid.Nil, info.ID)
	assert.NotEmpty(t, info.OS.Name)
	assert.NotEmpty(t, info.OS.Version)
	assert.NotEmpty(t, info.OS.Arch)
	assert.True(t, info.CPU.NumCPUs > 0)
	assert.True(t, info.CPU.Cores >= 0)
	assert.True(t, info.CPU.Frequency >= 0)
	assert.True(t, len(info.GPUs) >= 0)
	assert.True(t, info.Memory.Total > 0)
	assert.True(t, info.Memory.Available > 0)
	assert.True(t, info.Memory.Used > 0)
	assert.True(t, info.Memory.UsedPerc >= 0)
	assert.NotEmpty(t, info.Host.Hostname)
	assert.True(t, info.Host.Uptime > 0)
	assert.True(t, info.Battery.OnBattery || !info.Battery.OnBattery)
}

func TestGetSysInfoFromDB(t *testing.T) {
	setupTestDB(t)

	// This is an integration test since we can't mock the database
	// We'll test with a non-existent ID to verify error handling
	id := uuid.New()
	info, err := sysinfo.GetSysInfoFromDB(id)
	assert.Error(t, err)
	assert.Nil(t, info)
}

func TestSaveSysInfoToDB(t *testing.T) {
	setupTestDB(t)

	// This is an integration test since we can't mock the database
	sysInfo := createTestSystemInfo()
	err := sysinfo.SaveSysInfoToDB(sysInfo)
	assert.NoError(t, err)

	// Verify we can retrieve it
	info, err := sysinfo.GetSysInfoFromDB(sysInfo.ID)
	assert.NoError(t, err)
	assert.NotNil(t, info)
	assert.Equal(t, sysInfo.ID, info.ID)
	assert.Equal(t, sysInfo.OS.Name, info.OS.Name)
	assert.Equal(t, sysInfo.OS.Version, info.OS.Version)
	assert.Equal(t, sysInfo.OS.Arch, info.OS.Arch)
	assert.Equal(t, sysInfo.CPU.Model, info.CPU.Model)
	assert.Equal(t, sysInfo.CPU.NumCPUs, info.CPU.NumCPUs)
	assert.Equal(t, sysInfo.CPU.Cores, info.CPU.Cores)
	assert.Equal(t, sysInfo.CPU.Frequency, info.CPU.Frequency)
}

func TestGetSysInfoPersisted(t *testing.T) {
	// Set up test database
	originalDB := db.DB()
	defer func() {
		// Restore original DB after test
		db.SetDB(originalDB)
	}()

	setupTestDB(t)

	// Test successful case
	t.Run("Success", func(t *testing.T) {
		// GetSysInfoPersisted calls utils.CollectSysInfo and SaveSysInfoToDB
		// We can't easily mock utils.CollectSysInfo without modifying sysinfo.go,
		// so this is more of an integration test

		// Call the function under test
		sysInfo, err := sysinfo.GetSysInfoPersisted()

		// Verify results
		assert.NoError(t, err)
		assert.NotNil(t, sysInfo)
		assert.NotEqual(t, uuid.Nil, sysInfo.ID)

		// Verify the system info was saved to the database
		savedInfo, err := sysinfo.GetSysInfoFromDB(sysInfo.ID)
		assert.NoError(t, err)
		assert.NotNil(t, savedInfo)
		assert.Equal(t, sysInfo.ID, savedInfo.ID)
	})

	// Test database error case by using a closed database connection
	t.Run("Database Error", func(t *testing.T) {
		// Set up a database that will cause errors
		testDB, err := db.InitInMemorySQLite(nil)
		require.NoError(t, err)

		// Get the underlying *sql.DB
		sqlDB, err := testDB.DB()
		require.NoError(t, err)

		// Close the database to force errors
		err = sqlDB.Close()
		require.NoError(t, err)

		// Set the global DB to the closed database
		db.SetDB(testDB)

		// Call the function under test
		sysInfo, err := sysinfo.GetSysInfoPersisted()

		// Verify results
		assert.Error(t, err)
		assert.Nil(t, sysInfo)
		assert.Contains(t, err.Error(), "failed to save sysinfo to db")
	})
}
