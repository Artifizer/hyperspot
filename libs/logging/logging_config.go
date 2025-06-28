package logging

import (
	"github.com/hypernetix/hyperspot/libs/config"
)

type mainLoggingConfig struct {
	Config config.ConfigLogging
}

var mainLoggerConfig = &mainLoggingConfig{
	Config: config.ConfigLogging{
		ConsoleLevel: "info",
		FileLevel:    "debug",
		File:         "logs/main.log",
		MaxSizeMB:    1000,
		MaxBackups:   3,
		MaxAgeDays:   28,
	},
}

func (l *mainLoggingConfig) GetDefault() interface{} {
	return l.Config
}

func (l *mainLoggingConfig) Load(name string, configDict map[string]interface{}) error {
	cfg := &l.Config
	if err := config.UpdateStructFromConfig(cfg, configDict); err != nil {
		return err
	}

	MainLogger = CreateLogger(cfg) // override main logger based on config settings
	MainLogger.ConsoleLogger.Debug("Main logger initialized")

	return nil
}

func init() {
	config.RegisterLogger("main", mainLoggerConfig)
}
