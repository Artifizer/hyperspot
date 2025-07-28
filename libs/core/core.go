package core

import (
	"fmt"
	"log"
	"os"

	"github.com/danielgtaylor/huma/v2"
	"github.com/hypernetix/hyperspot/libs/api"
	"github.com/hypernetix/hyperspot/libs/config"
	"github.com/hypernetix/hyperspot/libs/db"
	"github.com/hypernetix/hyperspot/libs/logging"
	"github.com/hypernetix/hyperspot/libs/orm"
)

// Module represents a package of callbacks and attributes required to initialize the module
// The attributes below are listed in the order of initialization flow
type Module struct {
	Name          string
	Migrations    []interface{}      // called after PreInit
	InitAPIRoutes func(api huma.API) // called after Migratins, registers API routes
	InitMain      func() error       // main init worker, caled in the end
}

// Global slice to store registered initialization routines
var modules []Module

// RegisterModule adds a new package to be executed after DB initialization
// Packages will be executed in the order they were registered
func RegisterModule(pkg *Module) {
	modules = append(modules, *pkg)
}

func Init(cfg *config.Config) error {
	homeDir, err := serverHomeDirInit(cfg)
	if err != nil {
		return err
	}

	logging.Info("Working directory initialized: %s", homeDir)

	// Execute all registered initialization routines in order
	_, err = db.StartDBServer()
	if err != nil {
		return err
	}

	if err := orm.OrmInit(db.DB()); err != nil {
		return fmt.Errorf("failed to initialize ORM: %w", err)
	}

	// Make migrations
	err = makeMigrations()
	if err != nil {
		return err
	}

	err = initAPIRoutes()
	if err != nil {
		return err
	}

	// Execute all registered initialization routines in order
	err = initMain()
	if err != nil {
		return err
	}

	return nil
}

// initMain executes all registered initialization routines in their registration order
func makeMigrations() error {
	for _, module := range modules {
		if len(module.Migrations) > 0 {
			logging.Trace("Making migrations for: %s", module.Name)
			for _, migration := range module.Migrations {
				err := db.SafeAutoMigrate(db.DB(), migration)
				if err != nil {
					msg := fmt.Sprintf("failed to make migrations for %s: %s", module.Name, err)
					logging.Error("%s", msg)
					return fmt.Errorf("%s", msg)
				}
			}
		}
	}
	return nil
}

func initAPIRoutes() error {
	for _, module := range modules {
		if module.InitAPIRoutes != nil {
			api.RegisterAPIRoutes(module.InitAPIRoutes)
		}
	}
	return nil
}

// initMain executes all registered initialization routines in their registration order
func initMain() error {
	for _, module := range modules {
		if module.InitMain != nil {
			logging.Trace("Initializing: %s", module.Name)
			err := module.InitMain()
			if err != nil {
				err = fmt.Errorf("failed to initialize module: %s", err)
				logging.Error(err.Error())
				return err
			}
		}
	}
	return nil
}

// ServerHomeDirInit initializes the server's home directory and changes to it
func serverHomeDirInit(cfg *config.Config) (string, error) {
	homeDir, err := config.ResolveHomeDir(cfg.Server.HomeDir)
	if err != nil {
		return "", err
	}

	// Create the directory if it doesn't exist
	if err := os.MkdirAll(homeDir, 0755); err != nil {
		log.Fatalf("Failed to create home directory %s: %v", homeDir, err)
	}

	// Change to the home directory
	if err := os.Chdir(homeDir); err != nil {
		log.Fatalf("Failed to change working directory to %s: %v", homeDir, err)
	}

	return homeDir, nil
}

// GetHyperspotHomeDir returns the resolved .hyperspot directory path
func GetHyperspotHomeDir() (string, error) {
	return config.GetHyperspotHomeDir()
}
