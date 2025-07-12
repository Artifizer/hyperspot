package dataset

import (
	"context"
	"errors"
	"io/ioutil"
	"os"
	"testing"

	"github.com/google/uuid"
	"github.com/hypernetix/hyperspot/libs/db"
	"github.com/hypernetix/hyperspot/libs/orm"
	"github.com/hypernetix/hyperspot/libs/utils"
	"github.com/hypernetix/hyperspot/modules/job"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

// setupDatasetTestDB initializes an in-memory SQLite database and auto-migrates the Dataset schema.
func setupDatasetTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	testDB, err := db.InitInMemorySQLite(nil)
	require.NoError(t, err, "Failed to connect to test DB")
	db.DB = testDB
	err = orm.OrmInit(testDB)
	require.NoError(t, err, "Failed to initialize ORM for test DB")
	err = db.SafeAutoMigrate(testDB, &Dataset{})
	require.NoError(t, err, "Failed to migrate test database")
	job.JEInit()
	return testDB
}

//
// Tests using real GORM and Testify assertions
//

func TestDbSave(t *testing.T) {
	setupDatasetTestDB(t)

	d := &Dataset{Name: "test"}
	err := d.DbSave()
	require.NoError(t, err, "DbSave should not return an error")
	assert.NotZero(t, d.UpdatedAtMs, "UpdatedAtMs should be updated")

	// Verify the record is saved using GORM
	var ds Dataset
	err = db.DB.First(&ds, "name = ?", "test").Error
	require.NoError(t, err, "Dataset should be saved in DB")
	assert.Equal(t, d.Name, ds.Name, "Saved dataset name should match")
}

func TestDbDelete(t *testing.T) {
	setupDatasetTestDB(t)

	d := &Dataset{Name: "delete_test"}
	err := d.DbSave()
	require.NoError(t, err, "DbSave should succeed")

	err = d.DbDelete()
	require.NoError(t, err, "DbDelete should succeed")

	var ds Dataset
	err = db.DB.First(&ds, "name = ?", "delete_test").Error
	assert.Error(t, err, "Record should be deleted and not found")
	assert.Equal(t, gorm.ErrRecordNotFound, err)
}

func TestReset(t *testing.T) {
	d := &Dataset{
		Status:           DatasetStatusDownloaded,
		DownloadProgress: 50,
		Downloaded:       true,
		Size:             123,
		Checksum:         "abc",
		Error:            "some error",
		DownloadedAtMs:   999,
		DownloadJobId:    uuid.New(),
		UpdatedAtMs:      888,
	}
	d.Reset()
	assert.Equal(t, DatasetStatusUnknown, d.Status)
	assert.Equal(t, 0, int(d.DownloadProgress))
	assert.False(t, d.Downloaded)
	assert.Equal(t, int64(0), d.Size)
	assert.Empty(t, d.Checksum)
	assert.Empty(t, d.Error)
	assert.Equal(t, int64(0), d.DownloadedAtMs)
	assert.Equal(t, uuid.Nil, d.DownloadJobId)
	assert.Equal(t, int64(0), d.UpdatedAtMs)
}

func TestStatusStartDownload(t *testing.T) {
	setupDatasetTestDB(t)

	d := &Dataset{Name: "start_download"}
	d.Status = DatasetStatusInit
	err := d.StatusStartDownload()
	require.NoError(t, err)
	assert.Equal(t, DatasetStatusDownloading, d.Status)
}

func TestStatusCancelDownload(t *testing.T) {
	setupDatasetTestDB(t)

	d := &Dataset{Name: "cancel_download"}
	err := d.StatusCancelDownload()
	require.NoError(t, err)
	assert.Equal(t, DatasetStatusCanceled, d.Status)
}

func TestStatusFailDownload(t *testing.T) {
	setupDatasetTestDB(t)

	d := &Dataset{Name: "fail_download"}
	testErr := errors.New("download failed")
	err := d.StatusFailDownload(testErr.Error())
	require.NoError(t, err)
	assert.Equal(t, DatasetStatusError, d.Status)
	assert.Equal(t, "download failed", d.Error)
}

func TestUpdateDownloadProgress(t *testing.T) {
	setupDatasetTestDB(t)

	d := &Dataset{Name: "progress_test"}
	err := d.UpdateDownloadProgress(-10)
	assert.Error(t, err, "Expected error for progress -10")

	err = d.UpdateDownloadProgress(150)
	assert.Error(t, err, "Expected error for progress 150")

	// Valid progress less than 100–Download flag should be false.
	err = d.UpdateDownloadProgress(50)
	require.NoError(t, err)
	assert.Equal(t, float32(50), d.DownloadProgress)
	assert.False(t, d.Downloaded)

	// Set progress to 100.
	err = d.UpdateDownloadProgress(100)
	require.NoError(t, err)
	assert.Equal(t, float32(100), d.DownloadProgress)
}

func TestUpdateDownloadSize(t *testing.T) {
	setupDatasetTestDB(t)

	d := &Dataset{Name: "size_test"}
	err := d.UpdateDownloadSize(1024)
	require.NoError(t, err)
	assert.Equal(t, int64(1024), d.Size)
}

func TestStatusCompleteDownload(t *testing.T) {
	setupDatasetTestDB(t)

	t.Run("nonexistent file", func(t *testing.T) {
		d := &Dataset{Name: "complete_fail", LocalPath: "nonexistent_file"}
		err := d.StatusCompleteDownload()
		require.Error(t, err)
		assert.Contains(t, err.Error(), "failed to get dataset size")
	})

	t.Run("successful complete download", func(t *testing.T) {
		// Create a temporary file to simulate a downloaded dataset.
		tmpFile, err := ioutil.TempFile("", "dataset_test")
		require.NoError(t, err)
		defer os.Remove(tmpFile.Name())
		_, err = tmpFile.WriteString("hello")
		require.NoError(t, err)
		tmpFile.Close()

		d := &Dataset{Name: "complete_success", LocalPath: tmpFile.Name()}
		err = d.StatusCompleteDownload()
		require.NoError(t, err)
		assert.Equal(t, DatasetStatusDownloaded, d.Status)
		assert.True(t, d.Downloaded)
		assert.Equal(t, float32(100.0), d.DownloadProgress)

		info, err := os.Stat(tmpFile.Name())
		require.NoError(t, err)
		assert.Equal(t, info.Size(), d.Size)
		assert.NotEmpty(t, d.Checksum)
	})
}

func TestTryLockUnlock(t *testing.T) {
	d := &Dataset{Name: "mutex_test"}
	assert.True(t, d.TryLock(), "TryLock should succeed initially")
	d.Unlock()

	// Test Lock/Unlock sequence.
	d.Lock()
	d.Unlock() // Should not panic.
	assert.True(t, d.TryLock(), "TryLock should succeed after unlock")
	d.Unlock()
}

func TestSetDownloadJobId(t *testing.T) {
	setupDatasetTestDB(t)

	d := &Dataset{Name: "jobid_test"}
	newID := uuid.New()
	err := d.SetDownloadJobId(newID)
	require.NoError(t, err)
	assert.Equal(t, newID, d.DownloadJobId)
}

func TestDelete(t *testing.T) {
	setupDatasetTestDB(t)

	// Create a temporary directory to simulate dataset files.
	tmpDir, err := os.MkdirTemp("", "dataset_delete")
	require.NoError(t, err)
	// Create a file inside the directory.
	tmpFilePath := tmpDir + "/test.txt"
	err = ioutil.WriteFile(tmpFilePath, []byte("data"), 0644)
	require.NoError(t, err)

	d := &Dataset{Name: "delete_dataset", LocalPath: tmpDir}
	err = d.Delete()
	require.NoError(t, err)
	// Verify the directory has been removed.
	_, err = os.Stat(tmpDir)
	assert.True(t, os.IsNotExist(err), "Delete() should remove the dataset directory")
}

func TestRegisterDataset(t *testing.T) {
	setupDatasetTestDB(t)

	// Reset global registries.
	datasetsRegistry = []*Dataset{}
	datasetsRegistryByName = make(map[string]*Dataset)

	// Case: empty Name.
	d1 := &Dataset{OriginalURL: "url", LocalPath: "path"}
	err := RegisterDataset(d1)
	assert.Error(t, err, "RegisterDataset should error for empty Name")

	// Case: empty OriginalURL.
	d2 := &Dataset{Name: "d2", LocalPath: "path"}
	err = RegisterDataset(d2)
	assert.Error(t, err, "RegisterDataset should error for empty OriginalURL")

	// Case: empty LocalPath.
	d3 := &Dataset{Name: "d3", OriginalURL: "url"}
	err = RegisterDataset(d3)
	assert.Error(t, err, "RegisterDataset should error for empty LocalPath")

	// Valid dataset.
	d4 := &Dataset{Name: "d4", OriginalURL: "url", LocalPath: "path"}
	err = RegisterDataset(d4)
	require.NoError(t, err, "RegisterDataset should succeed for valid dataset")
	// Calling again should return nil.
	err = RegisterDataset(d4)
	require.NoError(t, err, "RegisterDataset on duplicate should succeed")
}

func TestListDatasets(t *testing.T) {
	// Set a known value for datasetsRegistry.
	d1 := &Dataset{Name: "list1"}
	d2 := &Dataset{Name: "list2"}
	datasetsRegistry = []*Dataset{d1, d2}

	// For simplicity, pass nil as page API request.
	list := ListDatasets(context.Background(), nil)
	assert.Equal(t, datasetsRegistry, list, "ListDatasets should return the registry")
}

func TestGetDataset(t *testing.T) {
	d := &Dataset{Name: "get_test"}
	datasetsRegistryByName = map[string]*Dataset{"get_test": d}
	got := GetDataset("get_test")
	assert.Equal(t, d, got, "GetDataset should return the correct dataset")
}

func TestUpdateStatusAndError(t *testing.T) {
	setupDatasetTestDB(t)

	d := &Dataset{Name: "update_status", Status: DatasetStatusInit, Error: ""}
	err := d.UpdateStatusAndError(DatasetStatusInit, "")
	require.NoError(t, err)
	assert.Equal(t, DatasetStatusInit, d.Status)
	assert.Equal(t, "", d.Error)

	err = d.UpdateStatusAndError(DatasetStatusDownloaded, "ok")
	require.NoError(t, err)
	assert.Equal(t, DatasetStatusDownloaded, d.Status)
	assert.Equal(t, "ok", d.Error)
}

func TestIsDownloaded(t *testing.T) {
	setupDatasetTestDB(t)

	// (a) File does not exist.
	d := &Dataset{Name: "isdl1", OriginalURL: "url", LocalPath: "nonexistent_file"}
	downloaded := d.IsDownloaded()
	assert.False(t, downloaded, "Expected IsDownloaded to return false for nonexistent file")
	assert.Equal(t, DatasetStatusNotLoaded, d.Status)

	// Create a temporary file.
	tmpFile, err := ioutil.TempFile("", "is_downloaded")
	require.NoError(t, err)
	_, err = tmpFile.WriteString("data")
	require.NoError(t, err)
	tmpFile.Close()

	// (b) File exists but DB record not found.
	d = &Dataset{Name: "isdl2", OriginalURL: "url", LocalPath: tmpFile.Name()}
	downloaded = d.IsDownloaded()
	assert.False(t, downloaded, "Expected IsDownloaded to return false when DB record is not found")

	// (c) DB record with Size 0.
	d = &Dataset{Name: "isdl3", OriginalURL: "url", LocalPath: tmpFile.Name()}
	err = d.DbSave() // Saves with Size == 0
	require.NoError(t, err)
	downloaded = d.IsDownloaded()
	assert.False(t, downloaded, "Expected IsDownloaded to return false for record with Size 0")

	// (d) Mismatch size.
	d = &Dataset{Name: "isdl4", OriginalURL: "url", LocalPath: tmpFile.Name()}
	d.Size = 999
	err = d.DbSave()
	require.NoError(t, err)
	downloaded = d.IsDownloaded()
	assert.False(t, downloaded, "Mismatch in size should return false")

	// (e) Checksum mismatch.
	info, err := os.Stat(tmpFile.Name())
	require.NoError(t, err)
	d = &Dataset{Name: "isdl5", OriginalURL: "url", LocalPath: tmpFile.Name()}
	d.Size = info.Size()
	d.Checksum = "wrong"
	err = d.DbSave()
	require.NoError(t, err)
	downloaded = d.IsDownloaded()
	assert.False(t, downloaded, "Checksum mismatch should return false")

	// (f) Original URL mismatch.
	d = &Dataset{Name: "isdl6", OriginalURL: "url", LocalPath: tmpFile.Name()}
	d.Size = info.Size()
	d.Checksum = ""
	// Insert a record with a different OriginalURL.
	err = db.DB.Create(&Dataset{Name: "isdl6", OriginalURL: "different_url", LocalPath: tmpFile.Name(), Size: info.Size()}).Error
	require.NoError(t, err)
	downloaded = d.IsDownloaded()
	assert.False(t, downloaded, "Original URL mismatch should return false")

	// (g) Success case.
	checksum, err := utils.GetFileChecksum(tmpFile.Name())
	require.NoError(t, err)
	d = &Dataset{Name: "isdl7", OriginalURL: "url", LocalPath: tmpFile.Name()}
	d.Size = info.Size()
	d.Checksum = checksum
	err = d.DbSave()
	require.NoError(t, err)
	downloaded = d.IsDownloaded()
	assert.True(t, downloaded, "Matching record should return true")
	assert.Equal(t, DatasetStatusDownloaded, d.Status)

	os.Remove(tmpFile.Name())
}

func TestDownloadPanic(t *testing.T) {
	// Test that Download panics when contextJob is nil.
	d := &Dataset{Name: "download_panic", OriginalURL: "url", LocalPath: "nonexistent_file"}
	assert.Panics(t, func() {
		_ = d.Download(context.Background(), nil)
	}, "Download should panic when contextJob is nil")
}

func TestInitDatasets(t *testing.T) {
	err := initDatasets()
	require.NoError(t, err, "initDatasets should not return an error")
}

func TestDbSave_Migrations(t *testing.T) {
	// Initialize real database migrations.
	setupDatasetTestDB(t)

	d := &Dataset{Name: "migration_test"}
	err := d.DbSave()
	require.NoError(t, err, "DbSave should succeed")
	// The migration should have created the table for Dataset and persisted the record.
	var ds Dataset
	err = db.DB.First(&ds, "name = ?", "migration_test").Error
	require.NoError(t, err, "Dataset should be retrieved from the migrated DB")
}
