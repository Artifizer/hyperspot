package dataset

import (
	"context"
	"fmt"
	"os"
	"time"

	"github.com/google/uuid"
	"github.com/hypernetix/hyperspot/libs/api"
	"github.com/hypernetix/hyperspot/libs/core"
	"github.com/hypernetix/hyperspot/libs/db"
	"github.com/hypernetix/hyperspot/libs/logging"
	"github.com/hypernetix/hyperspot/libs/utils"
	"github.com/hypernetix/hyperspot/modules/job"
	"gorm.io/gorm"
)

type DatasetStatus string

const (
	DatasetStatusUnknown     DatasetStatus = ""
	DatasetStatusNotLoaded   DatasetStatus = "not_loaded"
	DatasetStatusInit        DatasetStatus = "init"
	DatasetStatusDownloading DatasetStatus = "downloading"
	DatasetStatusDownloaded  DatasetStatus = "downloaded"
	DatasetStatusCanceled    DatasetStatus = "canceled"
	DatasetStatusError       DatasetStatus = "error"
	DatasetStatusDeleted     DatasetStatus = "deleted"
)

type Dataset struct {
	// Immutable static fields
	Name        string `json:"name" gorm:"primaryKey"`
	Description string `json:"description"`
	OriginalURL string `json:"original_url"`
	LocalPath   string `json:"local_path"`
	// Runtime fields
	UpdatedAtMs      int64         `json:"updated_at" gorm:"index" doc:"unix timestamp in milliseconds"`
	DownloadedAtMs   int64         `json:"downloaded_at" gorm:"index" doc:"unix timestamp in milliseconds"`
	Size             int64         `json:"size"`
	Checksum         string        `json:"checksum"`
	Status           DatasetStatus `json:"status"`
	Error            string        `json:"error"`
	DownloadProgress float32       `json:"download_progress"`
	Downloaded       bool          `json:"downloaded"`
	DownloadJobId    uuid.UUID     `json:"download_job_id"` // Job ID of the job that has locked given dataset

	Mutex      utils.DebugMutex `json:"-" gorm:"-"`
	MutexTrace string           `json:"-" gorm:"-"`
}

func (d *Dataset) DbSave() error {
	d.UpdatedAtMs = time.Now().UnixMilli()
	return db.DB().Save(d).Error
}

func (d *Dataset) DbDelete() error {
	return db.DB().Delete(d).Error
}

func (d *Dataset) Reset() {
	d.Status = DatasetStatusUnknown
	d.DownloadProgress = 0
	d.Downloaded = false
	d.Size = 0
	d.Checksum = ""
	d.Error = ""
	d.DownloadedAtMs = 0
	d.DownloadJobId = uuid.Nil
	d.UpdatedAtMs = 0
}

func (d *Dataset) StatusStartDownload() error {
	d.Reset()
	d.Status = DatasetStatusDownloading
	return d.DbSave()
}

func (d *Dataset) StatusCancelDownload() error {
	d.Status = DatasetStatusCanceled
	return d.DbSave()
}

func (d *Dataset) StatusFailDownload(reason string) error {
	d.Status = DatasetStatusError
	d.Error = reason

	return d.DbSave()
}

func (d *Dataset) UpdateDownloadProgress(progress float32) error {
	if progress < 0 || progress > 100 {
		return fmt.Errorf("invalid progress value: %.1f", progress)
	}

	d.DownloadProgress = progress
	if progress != 100 {
		d.Downloaded = false
	}
	logging.Info("Download progress for '%s' dataset: %.1f%%", d.Name, progress)

	return d.DbSave()
}

func (d *Dataset) UpdateDownloadSize(size int64) error {
	d.Size = size
	return d.DbSave()
}

func (d *Dataset) StatusCompleteDownload() error {
	d.Status = DatasetStatusDownloaded
	d.Downloaded = true
	d.DownloadProgress = 100
	d.DownloadedAtMs = time.Now().UnixMilli()
	size, err := utils.GetFileSize(d.LocalPath)
	if err != nil {
		return fmt.Errorf("failed to get dataset size: %w", err)
	}
	d.Size = size
	checksum, err := utils.GetFileChecksum(d.LocalPath)
	if err != nil {
		return fmt.Errorf("failed to get dataset checksum: %w", err)
	}
	d.Checksum = checksum
	return d.DbSave()
}

func (d *Dataset) TryLock() bool {
	logging.Trace("Trying to lock dataset '%s'", d.Name)
	return d.Mutex.TryLock()
}

func (d *Dataset) Lock() {
	logging.Trace("Locking dataset '%s'", d.Name)
	d.Mutex.Lock()
	d.MutexTrace = fmt.Sprintf("%s", d.MutexTrace)
}

func (d *Dataset) Unlock() {
	logging.Trace("Unlocking dataset '%s'", d.Name)
	d.Mutex.Unlock() // ignore error value
}

func (d *Dataset) SetDownloadJobId(downloadJobId uuid.UUID) error {
	d.DownloadJobId = downloadJobId
	return d.DbSave()
}

func (d *Dataset) Delete() error {
	d.Reset()

	err := os.RemoveAll(d.LocalPath)
	if err != nil {
		return fmt.Errorf("failed to delete dataset: %w", err)
	}
	logging.Info("Dataset '%s' deleted", d.Name)

	return d.DbSave()
}

var datasetsRegistry = []*Dataset{}
var datasetsRegistryByName = make(map[string]*Dataset)

func RegisterDataset(d *Dataset) error {
	if d.Name == "" {
		return fmt.Errorf("dataset name is empty")
	}

	if d.OriginalURL == "" {
		return fmt.Errorf("dataset original URL is empty")
	}

	if d.LocalPath == "" {
		return fmt.Errorf("dataset local path is empty")
	}

	_, ok := datasetsRegistryByName[d.Name]
	if ok {
		return nil
	}

	logging.Debug(fmt.Sprintf("Registering dataset: %s @ %s", d.Name, d.OriginalURL))

	dInDB := &Dataset{}
	err := db.DB().Where("name = ?", d.Name).First(dInDB).Error
	if err != nil {
		if err == gorm.ErrRecordNotFound {
			d.Reset()
			err = db.DB().Create(d).Error
			if err != nil {
				return err
			}
			return nil
		}
		return err
	}

	datasetsRegistry = append(datasetsRegistry, d)
	datasetsRegistryByName[dInDB.Name] = d
	return nil
}

// ListDatasets returns a list of available datasets
func ListDatasets(ctx context.Context, pageAPIRequest *api.PageAPIRequest) []*Dataset {
	return api.PageAPIPaginate(datasetsRegistry, pageAPIRequest)
}

func GetDataset(name string) *Dataset {
	return datasetsRegistryByName[name]
}

func (d *Dataset) UpdateStatusAndError(status DatasetStatus, error string) error {
	if d.Status != status || d.Error != error {
		d.Status = status
		d.Error = error
		return d.DbSave()
	}
	return nil
}

func (d *Dataset) IsDownloaded() bool {
	d.Mutex.Lock()
	defer d.Mutex.Unlock()

	if !utils.FileExists(d.LocalPath) {
		if err := d.UpdateStatusAndError(DatasetStatusNotLoaded, "file not found"); err != nil {
			logging.Error("failed to update dataset status: %v", err)
		}
		return false
	}

	size, err := utils.GetFileSize(d.LocalPath)
	if err != nil {
		if err := d.UpdateStatusAndError(DatasetStatusError, err.Error()); err != nil {
			logging.Error("failed to update dataset status: %v", err)
		}
		return false
	}

	checksum, err := utils.GetFileChecksum(d.LocalPath)
	if err != nil {
		if err := d.UpdateStatusAndError(DatasetStatusError, err.Error()); err != nil {
			logging.Error("failed to update dataset status: %v", err)
		}
		return false
	}

	dInDB := &Dataset{}
	err = db.DB().Where("name = ?", d.Name).First(dInDB).Error
	if err != nil {
		return false
	}

	if dInDB.Size == 0 {
		return false
	}

	if dInDB.Size != size {
		if err := d.UpdateStatusAndError(DatasetStatusError, fmt.Sprintf("dataset size mismatch: %d != %d", dInDB.Size, size)); err != nil {
			logging.Error("failed to update dataset status: %v", err)
		}
		return false
	}

	if dInDB.Checksum != "" && dInDB.Checksum != checksum {
		if err := d.UpdateStatusAndError(DatasetStatusError, fmt.Sprintf("dataset checksum mismatch: %s != %s", dInDB.Checksum, checksum)); err != nil {
			logging.Error("failed to update dataset status: %v", err)
		}
		return false
	}

	if dInDB.OriginalURL != d.OriginalURL {
		if err := d.UpdateStatusAndError(DatasetStatusError, fmt.Sprintf("dataset original URL mismatch: %s != %s", dInDB.OriginalURL, d.OriginalURL)); err != nil {
			logging.Error("failed to update dataset status: %v", err)
		}
		return false
	}

	d.Status = DatasetStatusDownloaded
	return true
}

func (d *Dataset) Download(ctx context.Context, contextJob *job.JobObj) error {
	if d.IsDownloaded() {
		return nil
	}

	if contextJob == nil {
		panic("internal error: contextJob is nil")
	}

	var downloadJob *job.JobObj
	var err error

	d.Lock()
	downloadJobId := d.DownloadJobId
	if downloadJobId == uuid.Nil {
		downloadJob, err = ScheduleDownloadJob(ctx, d.Name)
		if err != nil {
			d.Unlock()
			return fmt.Errorf("failed to schedule dataset download job: %w", err)
		}
	} else {
		downloadJob, err = job.JEGetJobByID(ctx, downloadJobId)
		if err != nil {
			d.Unlock()
			return fmt.Errorf("failed to get dataset download job: %w", err)
		}
	}
	if err := d.SetDownloadJobId(downloadJob.GetJobID()); err != nil {
		d.Unlock()
		return fmt.Errorf("failed to set download job ID: %w", err)
	}
	d.Unlock()

	if err := contextJob.SetLockedBy(ctx, downloadJob.GetJobID()); err != nil {
		logging.Error("Failed to set job as locked: %v", err)
	}
	job.JEWaitJob(ctx, downloadJob.GetJobID(), downloadJob.GetTimeoutSec()*time.Second)
	if err := contextJob.SetUnlocked(ctx); err != nil {
		logging.Error("Failed to set job as unlocked: %v", err)
	}

	return nil
}

func (d *Dataset) updateInDB() error {
	defer d.DbSave()

	var err error

	d.UpdatedAtMs = time.Now().UnixMilli()
	d.DownloadedAtMs = time.Now().UnixMilli()
	d.DownloadProgress = 100
	d.Downloaded = true
	d.Size, err = utils.GetFileSize(d.LocalPath)
	if err != nil {
		d.Status = DatasetStatusError
		d.Error = err.Error()
		return err
	}

	d.Checksum, err = utils.GetFileChecksum(d.LocalPath)
	if err != nil {
		d.Status = DatasetStatusError
		d.Error = err.Error()
		return err
	}

	d.Error = ""

	return nil
}

// FIXME: reset Dataset locks in DB on service restart

func initDatasets() error {
	initJobs()
	return nil
}

func init() {
	core.RegisterModule(&core.Module{
		Migrations: []interface{}{
			&Dataset{},
		},
		InitMain:      initDatasets,
		InitAPIRoutes: initDatasetAPIRoutes,
		Name:          "datasets",
	})
}
