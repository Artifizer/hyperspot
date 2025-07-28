package db

import (
	"github.com/hypernetix/hyperspot/libs/config"
	"github.com/hypernetix/hyperspot/libs/logging"
	"github.com/hypernetix/hyperspot/libs/utils"
)

type ConfigDatabaseLogger struct {
	Config config.ConfigLogging
}

var configDatabaseLoggerInstance = &ConfigDatabaseLogger{
	Config: config.ConfigLogging{
		ConsoleLevel: "error",
		FileLevel:    "debug",
		File:         "logs/db.log",
		MaxSizeMB:    1000,
		MaxBackups:   3,
		MaxAgeDays:   28,
	},
}

func (c *ConfigDatabaseLogger) GetDefault() interface{} {
	return c.Config
}

func (c *ConfigDatabaseLogger) Load(name string, configDict map[string]interface{}) error {
	cfg := &c.Config
	if err := config.UpdateStructFromConfig(cfg, configDict); err != nil {
		return err
	}

	c.Config = *cfg

	logger = logging.CreateLogger(&c.Config, utils.GoCallerPackageName(1))
	logger.ConsoleLogger.Debug("Database logger initialized")

	return nil
}

type ConfigDatabase struct {
	Type string `mapstructure:"type"`
	DSN  string `mapstructure:"dsn"`
} // For other databases

var ConfigDatabaseInstance = &ConfigDatabase{
	Type: "sqlite",
	DSN:  "database/database.db",
}

func (c *ConfigDatabase) Load(name string, configDict map[string]interface{}) error {
	err := config.UpdateStructFromConfig(c, configDict)
	if err != nil {
		return err
	}

	if c.Type == "sqlite" && c.DSN == "" {
		c.DSN = "database/data.db"
	}

	return nil
}

func (c *ConfigDatabase) GetDefault() interface{} {
	return c
}

func init() {
	config.RegisterLogger("database", configDatabaseLoggerInstance)
	config.RegisterConfigComponent("database", ConfigDatabaseInstance)
}
