package job

import (
	"github.com/hypernetix/hyperspot/libs/config"
	"github.com/hypernetix/hyperspot/libs/logging"
)

type jobLoggerConfig struct {
	Config config.ConfigLogging
}

var jobLoggerConfigInstance = &jobLoggerConfig{
	Config: config.ConfigLogging{
		ConsoleLevel: "info",
		FileLevel:    "debug",
		File:         "logs/job.log",
		MaxSizeMB:    1000,
		MaxBackups:   3,
		MaxAgeDays:   28,
	},
}

func (l *jobLoggerConfig) GetDefault() interface{} {
	return l.Config
}

func (l *jobLoggerConfig) Load(name string, configDict map[string]interface{}) error {
	cfg := &l.Config
	if err := config.UpdateStructFromConfig(cfg, configDict); err != nil {
		return err
	}

	logger = logging.CreateLogger(cfg)
	logger.ConsoleLogger.Debug("Upstream API client logger initialized")

	return nil
}

func init() {
	config.RegisterLogger("job", jobLoggerConfigInstance)
}
