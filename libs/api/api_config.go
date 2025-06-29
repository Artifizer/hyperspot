package api

import (
	"github.com/hypernetix/hyperspot/libs/config"
	"github.com/hypernetix/hyperspot/libs/logging"
)

type apiLoggerConfig struct {
	Config config.ConfigLogging
}

var apiLoggerConfigInstance = &apiLoggerConfig{
	Config: config.ConfigLogging{
		ConsoleLevel: "info",
		FileLevel:    "debug",
		File:         "logs/api.log",
		MaxSizeMB:    1000,
		MaxBackups:   3,
		MaxAgeDays:   28,
	},
}

func (l *apiLoggerConfig) GetDefault() interface{} {
	return l.Config
}

func (l *apiLoggerConfig) Load(name string, configDict map[string]interface{}) error {
	cfg := &l.Config
	if err := config.UpdateStructFromConfig(cfg, configDict); err != nil {
		return err
	}

	logger = logging.CreateLogger(cfg)

	logger.ConsoleLogger.Debug("API logger initialized")
	return nil
}

func init() {
	config.RegisterLogger("api", apiLoggerConfigInstance)
}
