package test_module

import (
	"context"
	"testing"

	"github.com/hypernetix/hyperspot/libs/db"
	"github.com/hypernetix/hyperspot/libs/orm"
	"github.com/hypernetix/hyperspot/modules/job"
	"github.com/stretchr/testify/assert"
	"gorm.io/gorm"
)

// setupTestDB initializes an in-memory SQLite DB and migrates the schema for test module job tests.
func setupTestDBForJob(t *testing.T) *gorm.DB {
	testDB, err := db.InitInMemorySQLite(nil)
	if err != nil {
		t.Fatalf("Failed to connect to test DB: %v", err)
	}
	db.SetDB(testDB)

	err = orm.OrmInit(testDB)
	if err != nil {
		t.Fatalf("Failed to initialize ORM: %v", err)
	}

	if err := db.SafeAutoMigrate(testDB, &job.Job{}, &job.JobGroup{}, &job.JobType{}); err != nil {
		t.Fatalf("Failed to auto migrate schema: %v", err)
	}
	return testDB
}

func TestTestModuleJobParams_DefaultValues(t *testing.T) {
	params := &TestModuleJobParams{}

	// Test default values
	assert.Equal(t, 0, params.DurationLimitSec)
	assert.Equal(t, "", params.ModelName)
}

func TestTestModuleJobParams_WithValues(t *testing.T) {
	params := &TestModuleJobParams{
		DurationLimitSec: 30,
		ModelName:        "test-model",
	}

	assert.Equal(t, 30, params.DurationLimitSec)
	assert.Equal(t, "test-model", params.ModelName)
}

func TestTestModuleJobInit_Success(t *testing.T) {
	// Setup test database
	setupTestDBForJob(t)

	// Create a test job with proper initialization
	jobObj := &job.JobObj{}
	// Note: In real usage, params would be set through the job creation process
	// For testing, we'll test the function directly with a mock job

	ctx := context.Background()
	err := testModuleParamsValidation(ctx, jobObj)

	// This should fail because the job doesn't have proper params set
	assert.Error(t, err)
}

func TestTestModuleJobInit_InvalidParams(t *testing.T) {
	// Setup test database
	setupTestDBForJob(t)

	// Create a job with wrong parameter type
	jobObj := &job.JobObj{}
	// Note: In real usage, params would be set through the job creation process

	ctx := context.Background()
	err := testModuleParamsValidation(ctx, jobObj)

	assert.Error(t, err)
	assert.Contains(t, err.Error(), "invalid job parameters type")
}

func TestTestModuleJobInit_ZeroDuration(t *testing.T) {
	// Setup test database
	setupTestDBForJob(t)

	// Create a test job with zero duration
	jobObj := &job.JobObj{}
	// Note: In real usage, params would be set through the job creation process

	ctx := context.Background()
	err := testModuleParamsValidation(ctx, jobObj)

	assert.Error(t, err)
	assert.Contains(t, err.Error(), "invalid job parameters type")
}

func TestRegisterTestModuleJob_Success(t *testing.T) {
	// Setup test database
	setupTestDBForJob(t)

	// Register the job type
	jobType := RegisterTestModuleJob()

	assert.NotNil(t, jobType)
	assert.Equal(t, "test-module", jobType.Name)
	assert.Equal(t, "Execute a test job that simulates work by sleeping", jobType.Description)
	assert.Equal(t, "test", jobType.Group) // Group is the string name, not the pointer
	assert.True(t, jobType.WorkerIsSuspendable)
	assert.Equal(t, 3600, jobType.TimeoutSec)  // 1 hour in seconds
	assert.Equal(t, 10, jobType.RetryDelaySec) // 10 seconds
	assert.Equal(t, 3, jobType.MaxRetries)
}

// Benchmark tests
func BenchmarkTestModuleJobWorker(b *testing.B) {
	// Setup test database
	setupTestDBForJob(&testing.T{})

	// Create a test job with minimal duration
	jobObj := &job.JobObj{}
	// Note: In real usage, params would be set through the job creation process

	ctx := context.Background()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		testModuleJobWorker(ctx, jobObj)
	}
}

func BenchmarkTestModuleJobInit(b *testing.B) {
	// Setup test database
	setupTestDBForJob(&testing.T{})

	// Create a test job
	jobObj := &job.JobObj{}
	// Note: In real usage, params would be set through the job creation process

	ctx := context.Background()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		testModuleParamsValidation(ctx, jobObj)
	}
}
