package chat

import (
	"time"

	"github.com/hypernetix/hyperspot/libs/config"
	"github.com/hypernetix/hyperspot/libs/logging"
)

type chatConfig struct {
	SoftDeleteMessagesAfterDays     int    `mapstructure:"soft_delete_messages_after_days"`
	HardDeleteMessagesAfterDays     int    `mapstructure:"hard_delete_messages_after_days"`
	SoftDeleteTempThreadsAfterHours int    `mapstructure:"soft_delete_temp_threads_after_hours"`
	HardDeleteTempThreadsAfterHours int    `mapstructure:"hard_delete_temp_threads_after_hours"`
	UploadedFileTempDir             string `mapstructure:"uploaded_file_temp_dir"`
	UploadedFileMaxContentLength    int    `mapstructure:"uploaded_file_max_content_length" doc:"max uploaded file content length in characters, the tail will be truncated"`
	UploadedFileMaxSizeKB           int    `mapstructure:"uploaded_file_max_size_kb"`
}

var chatConfigInstance = &chatConfig{
	SoftDeleteMessagesAfterDays:     90,
	HardDeleteMessagesAfterDays:     120,
	SoftDeleteTempThreadsAfterHours: 24,
	HardDeleteTempThreadsAfterHours: 48,
	UploadedFileTempDir:             "", // empty means will be set by the system
	UploadedFileMaxContentLength:    16 * 1024,
	UploadedFileMaxSizeKB:           16 * 1024,
}

func (c *chatConfig) GetDefault() interface{} {
	return c
}

func (c *chatConfig) Load(name string, configDict map[string]interface{}) error {
	cfg := &chatConfig{}
	if err := config.UpdateStructFromConfig(cfg, configDict); err != nil {
		return err
	}

	if cfg.SoftDeleteMessagesAfterDays > 0 {
		chatConfigInstance.SoftDeleteMessagesAfterDays = cfg.SoftDeleteMessagesAfterDays
	}

	if cfg.HardDeleteMessagesAfterDays > 0 {
		chatConfigInstance.HardDeleteMessagesAfterDays = cfg.HardDeleteMessagesAfterDays
	}

	if cfg.SoftDeleteTempThreadsAfterHours > 0 {
		chatConfigInstance.SoftDeleteTempThreadsAfterHours = cfg.SoftDeleteTempThreadsAfterHours
	}

	if cfg.HardDeleteTempThreadsAfterHours > 0 {
		chatConfigInstance.HardDeleteTempThreadsAfterHours = cfg.HardDeleteTempThreadsAfterHours
	}

	err := retentionStart(time.Duration(300)*time.Second,
		chatConfigInstance.SoftDeleteMessagesAfterDays,
		chatConfigInstance.HardDeleteMessagesAfterDays,
		chatConfigInstance.SoftDeleteTempThreadsAfterHours,
		chatConfigInstance.HardDeleteTempThreadsAfterHours,
	)
	if err != nil {
		return err
	}
	return nil
}

type chatLoggerConfig struct {
	Config config.ConfigLogging
}

var chatLoggerConfigInstance = &chatLoggerConfig{
	Config: config.ConfigLogging{
		ConsoleLevel: "info",
		FileLevel:    "debug",
		File:         "logs/chat.log",
		MaxSizeMB:    1000,
		MaxBackups:   3,
		MaxAgeDays:   28,
	},
}

func (l *chatLoggerConfig) GetDefault() interface{} {
	return l.Config
}

func (l *chatLoggerConfig) Load(name string, configDict map[string]interface{}) error {
	cfg := &l.Config
	if err := config.UpdateStructFromConfig(cfg, configDict); err != nil {
		return err
	}

	logger = logging.CreateLogger(cfg)
	logger.ConsoleLogger.Debug("Chat logger initialized")

	return nil
}

func init() {
	config.RegisterLogger("chat", chatLoggerConfigInstance)
	config.RegisterConfigComponent("chat", chatConfigInstance)
}
