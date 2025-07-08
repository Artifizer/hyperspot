package db

import (
	"fmt"
	"path/filepath"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/hypernetix/hyperspot/libs/logging"
	"github.com/hypernetix/hyperspot/libs/utils"
	"gorm.io/driver/mysql"
	"gorm.io/driver/postgres"
	"gorm.io/driver/sqlite"
	"gorm.io/driver/sqlserver"
	"gorm.io/gorm"
	gorm_logger "gorm.io/gorm/logger"
)

var (
	mu       utils.DebugMutex
	dbGlobal *gorm.DB
)

func DB() *gorm.DB {
	mu.Lock()
	defer mu.Unlock()
	return dbGlobal
}

func SetDB(db *gorm.DB) {
	mu.Lock()
	defer mu.Unlock()
	dbGlobal = db
}

// SafeAutoMigrate performs AutoMigrate and ensures DDL operations are fully committed
// before proceeding. This prevents SQLite race conditions where DDL operations appear
// to complete but internal locks are still held.
func SafeAutoMigrate(db *gorm.DB, dst ...interface{}) error {
	// Perform the migration
	if err := db.AutoMigrate(dst...); err != nil {
		return fmt.Errorf("failed to auto migrate: %w", err)
	}

	// Ensure DDL operations are fully committed before proceeding
	// This is critical for SQLite which may still have internal locks
	// even after AutoMigrate appears to complete successfully

	// Force a transaction commit to ensure all DDL operations are persisted
	tx := db.Begin()
	if tx.Error != nil {
		return fmt.Errorf("failed to begin verification transaction: %w", tx.Error)
	}
	if err := tx.Commit().Error; err != nil {
		return fmt.Errorf("failed to commit verification transaction: %w", err)
	}

	// Execute a simple query to verify the database is ready and unlocked
	// This will fail if there are still any locks from the DDL operations
	var count int64
	if len(dst) > 0 {
		// Use the first migrated model for verification
		if err := db.Model(dst[0]).Count(&count).Error; err != nil {
			return fmt.Errorf("failed to verify database readiness after migration: %w", err)
		}
	} else {
		// Fallback: execute a simple query
		if err := db.Exec("SELECT 1").Error; err != nil {
			return fmt.Errorf("failed to verify database readiness after migration: %w", err)
		}
	}

	// Additional safety: ensure any background SQLite operations complete
	// This small delay allows SQLite to fully release internal locks
	time.Sleep(10 * time.Millisecond)

	return nil
}

// Add health check endpoint
func healthCheck(db *gorm.DB) error {
	return db.Exec("SELECT 1").Error
}

func initSQLite(gormConfig *gorm.Config) (*gorm.DB, error) {
	if logger == nil {
		return nil, fmt.Errorf("logger not initialized")
	}

	// Normalize the database file path for cross-platform compatibility
	databaseFile := filepath.ToSlash(filepath.Clean(ConfigDatabaseInstance.DSN))

	if err := utils.PrepareFilePath(databaseFile); err != nil {
		return nil, fmt.Errorf("failed to prepare database file path: %w", err)
	}

	db, err := gorm.Open(sqlite.Open(databaseFile), gormConfig)
	if err != nil {
		return nil, fmt.Errorf("failed to connect database: %w", err)
	}

	return db, nil
}

func newDB() (*gorm.DB, error) {
	var db *gorm.DB
	var err error

	gormConfig := &gorm.Config{
		Logger: (&GormLogger{*logger, gorm_logger.Info}),
		NowFunc: func() time.Time {
			return time.Now().UTC()
		},
	}

	maxOpenConns := 25
	maxIdleConns := 5
	connMaxLifetime := 5 * time.Minute

	switch ConfigDatabaseInstance.Type {
	case "sqlite":
		maxOpenConns = 1
		maxIdleConns = 1
		db, err = initSQLite(gormConfig)
	case "memory":
		maxOpenConns = 1
		maxIdleConns = 1
		connMaxLifetime = 0
		db, err = InitInMemorySQLite(gormConfig)
	case "postgres":
		db, err = gorm.Open(postgres.Open(ConfigDatabaseInstance.DSN), gormConfig)
	case "mysql":
		db, err = gorm.Open(mysql.Open(ConfigDatabaseInstance.DSN), gormConfig)
	case "sqlserver":
		db, err = gorm.Open(sqlserver.Open(ConfigDatabaseInstance.DSN), gormConfig)
	default:
		return nil, fmt.Errorf("unsupported database type: %s", ConfigDatabaseInstance.Type)
	}

	if err != nil {
		return nil, fmt.Errorf("failed to initialize database: %w", err)
	}

	if err := healthCheck(db); err != nil {
		return nil, fmt.Errorf("database health check failed: %w", err)
	}

	// Configure connection pool
	sqlDB, err := db.DB()
	if err != nil {
		return nil, fmt.Errorf("failed to get database instance: %w", err)
	}
	sqlDB.SetMaxOpenConns(maxOpenConns)
	sqlDB.SetMaxIdleConns(maxIdleConns)
	sqlDB.SetConnMaxLifetime(connMaxLifetime)

	if err == nil {
		if DB() == nil {
			SetDB(db)
		}
		logDB(logging.InfoLevel, "Database initialized successfully")
	}

	return db, nil
}

func StartDBServer() (*gorm.DB, error) {
	typeName := ConfigDatabaseInstance.Type

	if typeName == "postgres" || typeName == "mysql" || typeName == "sqlserver" || typeName == "sqlite" || typeName == "memory" {
		logDB(logging.InfoLevel, "Initializing database: %s", typeName)
	} else {
		logDB(logging.WarnLevel, "Unsupported database type %s, choose one of: sqlite, postgres, mysql, sqlserver, memory", typeName)
	}

	// Initialize database connection
	db, err := newDB()
	if err != nil {
		return nil, err
	}

	return db, nil
}

// Add transaction helper
func WithTransaction(fn func(tx *gorm.DB) error) error {
	return DB().Transaction(func(tx *gorm.DB) error {
		return fn(tx)
	})
}

func InitInMemorySQLite(gormConfig *gorm.Config) (*gorm.DB, error) {
	if logger == nil {
		return nil, fmt.Errorf("logger not initialized")
	}
	if gormConfig == nil {
		gormConfig = &gorm.Config{
			Logger: (&GormLogger{*logger, gorm_logger.Info}),
			NowFunc: func() time.Time {
				return time.Now().UTC()
			},
		}
	}

	uniqueDBName := uuid.New().String()
	dbURL := "file:/" + uniqueDBName + "?vfs=memdb&cache=shared"

	// Use a mode with better concurrency handling
	db, err := gorm.Open(sqlite.Open(dbURL), gormConfig)
	if err != nil {
		return nil, fmt.Errorf("failed to connect to test database: %s", err.Error())
	}

	// Set PRAGMA busy_timeout to avoid "database table is locked" errors in concurrent tests.
	if err := db.Exec("PRAGMA busy_timeout = 30000").Error; err != nil {
		return nil, fmt.Errorf("failed to set busy timeout: %s", err.Error())
	}
	if err := db.Exec("PRAGMA journal_mode = WAL").Error; err != nil {
		return nil, fmt.Errorf("failed to set journal mode: %s", err.Error())
	}
	if err := db.Exec("PRAGMA synchronous = FULL").Error; err != nil {
		return nil, fmt.Errorf("failed to set synchronous mode: %s", err.Error())
	}
	if err := db.Exec("PRAGMA locking_mode = NORMAL").Error; err != nil {
		return nil, fmt.Errorf("failed to set locking mode: %s", err.Error())
	}
	if err := db.Exec("PRAGMA wal_checkpoint(FULL)").Error; err != nil {
		return nil, fmt.Errorf("failed to checkpoint WAL: %s", err.Error())
	}

	sqlDB, err := db.DB()
	if err != nil {
		return nil, fmt.Errorf("failed to get database instance: %w", err)
	}

	// Limit connections to catch table lock issues
	sqlDB.SetMaxOpenConns(1)
	sqlDB.SetMaxIdleConns(1)
	sqlDB.SetConnMaxLifetime(0)

	return db, nil
}

func DatabaseIsLocked(err error) bool {
	return err != nil && (strings.Contains(err.Error(), "database table is locked") || strings.Contains(err.Error(), "database is locked"))
}
