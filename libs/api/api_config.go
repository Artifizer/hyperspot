package api

import (
	"github.com/hypernetix/hyperspot/libs/config"
	"github.com/hypernetix/hyperspot/libs/logging"
)

// API configuration structure
type apiConfig struct {
	DocsOverview       string `mapstructure:"docs_overview" yaml:"docs_overview,omitempty"`
	OpenAIAPIPrefix    string `mapstructure:"open_ai_api_prefix" yaml:"open_ai_api_prefix,omitempty"`
	OllamaAPIPrefix    string `mapstructure:"ollama_api_prefix" yaml:"ollama_api_prefix,omitempty"`
	HyperspotAPIPrefix string `mapstructure:"hyperspot_api_prefix" yaml:"hyperspot_api_prefix,omitempty"`
}

// Logger configuration structure
type apiLoggerConfig struct {
	Config config.ConfigLogging
}

// API configuration instance with default values
var apiConfigInstance = &apiConfig{
	DocsOverview:       "HyperSpot Server API description",
	OpenAIAPIPrefix:    "/api/v1/",
	OllamaAPIPrefix:    "/api/",
	HyperspotAPIPrefix: "/",
}

// Logger configuration instance with default values
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

// GetDefault returns the default API configuration
func (c *apiConfig) GetDefault() interface{} {
	return c
}

// GetDefault returns the default logger configuration
func (l *apiLoggerConfig) GetDefault() interface{} {
	return l.Config
}

// Load updates the API configuration from the provided config dictionary
func (c *apiConfig) Load(name string, configDict map[string]interface{}) error {
	return config.UpdateStructFromConfig(c, configDict)
}

// Load updates the logger configuration from the provided config dictionary
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
	// Register API configuration
	config.RegisterConfigComponent("api", apiConfigInstance)

	// Register logger configuration
	config.RegisterLogger("api", apiLoggerConfigInstance)
}
