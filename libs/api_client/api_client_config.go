package api_client

import (
	"github.com/hypernetix/hyperspot/libs/config"
	"github.com/hypernetix/hyperspot/libs/logging"
)

type upstreamLoggerConfig struct {
	Config config.ConfigLogging
}

var upstreamLoggerConfigInstance = &upstreamLoggerConfig{
	Config: config.ConfigLogging{
		ConsoleLevel: "info",
		FileLevel:    "debug",
		File:         "logs/upstream.log",
		MaxSizeMB:    1000,
		MaxBackups:   3,
		MaxAgeDays:   28,
	},
}

func (l *upstreamLoggerConfig) GetDefault() interface{} {
	return l.Config
}

func (l *upstreamLoggerConfig) Load(name string, configDict map[string]interface{}) error {
	cfg := &l.Config
	if err := config.UpdateStructFromConfig(cfg, configDict); err != nil {
		return err
	}

	logger = logging.CreateLogger(cfg)
	logger.ConsoleLogger.Debug("Upstream API client logger initialized")

	return nil
}

func init() {
	config.RegisterLogger("upstream", upstreamLoggerConfigInstance)
}
