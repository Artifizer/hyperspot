package test_setup

import (
	"runtime/debug"
	"sync"

	"github.com/hypernetix/hyperspot/libs/db"
	"github.com/hypernetix/hyperspot/libs/logging"
	"github.com/hypernetix/hyperspot/libs/orm"
	"github.com/hypernetix/hyperspot/libs/utils"
	"gorm.io/gorm"
)

var (
	testEnvDB   *gorm.DB
	testEnvErr  error
	testEnvOnce sync.Once
)

// Unit tests helper

func InitTestEnv() (*gorm.DB, error) {
	testEnvOnce.Do(func() {
		utils.SetMutexDebug(true)

		// Enable full database logging for debugging
		logging.ForceLogLevel(logging.TraceLevel)
		db.ForceLogLevel(logging.TraceLevel)

		// Disable GC to catch memory issues early
		debug.SetGCPercent(-1)

		testEnvDB, testEnvErr = db.InitInMemorySQLite(nil)
		if testEnvErr != nil {
			return
		}

		if testEnvErr = orm.OrmInit(testEnvDB); testEnvErr != nil {
			return
		}

		logging.Trace("=============== Test DB setup complete =================")
	})

	return testEnvDB, testEnvErr
}
