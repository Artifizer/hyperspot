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
	DB *gorm.DB
)

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

	switch ConfigDatabaseInstance.Type {
	case "sqlite":
		db, err = initSQLite(gormConfig)
	case "memory":
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
	sqlDB.SetMaxOpenConns(25)
	sqlDB.SetMaxIdleConns(5)
	sqlDB.SetConnMaxLifetime(5 * time.Minute)

	if err == nil {
		if DB == nil {
			DB = db
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
	return DB.Transaction(func(tx *gorm.DB) error {
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
	db.Exec("PRAGMA busy_timeout = 30000")
	db.Exec("PRAGMA journal_mode = WAL")
	db.Exec("PRAGMA synchronous = FULL")
	db.Exec("PRAGMA locking_mode = NORMAL")

	return db, nil
}

func DatabaseIsLocked(err error) bool {
	return err != nil && (strings.Contains(err.Error(), "database table is locked") || strings.Contains(err.Error(), "database is locked"))
}
