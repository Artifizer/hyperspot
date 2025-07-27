package config

import (
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"strings"
	"sync"
	"time"

	"github.com/mitchellh/mapstructure"
	"github.com/spf13/viper"
	"gopkg.in/yaml.v2"
)

var (
	instance *Config
	once     sync.Once
	mu       sync.RWMutex
)

// Get returns the current configuration instance
func Get() *Config {
	mu.RLock()
	defer mu.RUnlock()
	return instance
}

// Config represents the global configuration
type Config struct {
	Server ConfigServer `mapstructure:"server"`
	LLM    ConfigLLM    `mapstructure:"llm"`
}

// FullToYaml returns the YAML representation of the full configuration,
// merging the base config with logging and additional registered components (such as database and code_executors).
func (c *Config) ToYaml() (string, error) {
	// Start with the base config map.
	baseMap, ok := structToMap(c).(map[string]interface{})
	if !ok {
		return "", fmt.Errorf("failed extracting base config as map")
	}

	// Add logging configuration from registeredLoggers.
	loggingSection := make(map[string]interface{})
	for name, comp := range registeredLoggers {
		loggingSection[name] = structToMap(comp.GetDefault())
	}
	baseMap["logging"] = loggingSection

	// Merge all registered config components dynamically.
	for key, comp := range registeredConfigComponents {
		// Handle nested keys (e.g., "benchmarks.llm.coding.humaneval")
		if strings.Contains(key, ".") {
			parts := strings.Split(key, ".")
			addNestedComponent(baseMap, parts, structToMap(comp.GetDefault()))
		} else {
			baseMap[key] = structToMap(comp.GetDefault())
		}
	}

	yamlData, err := yaml.Marshal(baseMap)
	if err != nil {
		return "", fmt.Errorf("error marshaling full config to YAML: %v", err)
	}

	return string(yamlData), nil
}

// structToMap recursively converts a value (struct, slice, or map) to a corresponding standard Go data structure.
// It uses the "mapstructure" tag for struct fields so that nested pointers are handled correctly.
func structToMap(i interface{}) interface{} {
	val := reflect.ValueOf(i)
	if !val.IsValid() {
		return i
	}
	switch val.Kind() {
	case reflect.Ptr:
		if val.IsNil() {
			return nil
		}
		return structToMap(val.Elem().Interface())
	case reflect.Struct:
		out := make(map[string]interface{})
		typ := val.Type()
		for i := 0; i < typ.NumField(); i++ {
			field := typ.Field(i)
			// Only process exported fields.
			if field.PkgPath != "" {
				continue
			}
			// Get the "mapstructure" tag.
			tagFull := field.Tag.Get("mapstructure")
			// If no tag is specified, ignore the field.
			if tagFull == "" {
				continue
			}
			// If the tag contains "squash", then flatten (merge) its keys.
			if strings.Contains(tagFull, "squash") {
				fieldValue := val.Field(i).Interface()
				flattened := structToMap(fieldValue)
				if m, ok := flattened.(map[string]interface{}); ok {
					for k, v := range m {
						out[k] = v
					}
				}
				continue
			}
			// Otherwise, use the first part of the tag as the key.
			parts := strings.Split(tagFull, ",")
			tag := parts[0]
			fieldValue := val.Field(i).Interface()
			out[tag] = structToMap(fieldValue)
		}
		return out
	case reflect.Slice:
		outSlice := make([]interface{}, val.Len())
		for i := 0; i < val.Len(); i++ {
			outSlice[i] = structToMap(val.Index(i).Interface())
		}
		return outSlice
	case reflect.Map:
		outMap := make(map[interface{}]interface{})
		for _, key := range val.MapKeys() {
			outMap[key.Interface()] = structToMap(val.MapIndex(key).Interface())
		}
		return outMap
	default:
		return i
	}
}

type ConfigServer struct {
	Host       string `mapstructure:"host"`
	Port       int    `mapstructure:"port"`
	HomeDir    string `mapstructure:"home_dir"`
	TimeoutSec int    `mapstructure:"timeout_sec"`
}

type ConfigLLM struct {
	DefaultService string                      `mapstructure:"default_service"`
	Temperature    float64                     `mapstructure:"temperature"`
	MaxTokens      int                         `mapstructure:"max_tokens"`
	LLMServices    map[string]ConfigLLMService `mapstructure:"services"`
}

// ServiceConfig holds configuration for a specific LLM service type
type ConfigLLMService struct {
	APIFormat    string                 `mapstructure:"api_format"` // e.g., "openai", "ollama", etc.
	APIKeyEnvVar string                 `mapstructure:"api_key_env_var" doc:"Environment variable containing the API key for the LLM service"`
	APIKey       string                 `mapstructure:"api_key" doc:"API key for the LLM service"`
	URLs         []string               `mapstructure:"urls"`
	Model        string                 `mapstructure:"model"` // Default model for this service, FIXME: not supported for now
	Upstream     ConfigUpstream         `mapstructure:"upstream"`
	Params       map[string]interface{} `mapstructure:"params,omitempty"`
}

type ConfigLogging struct {
	ConsoleLevel string `mapstructure:"console_level" default:"info"`
	FileLevel    string `mapstructure:"file_level" default:"debug"`
	File         string `mapstructure:"file" default:"logs/main.log"`
	MaxSizeMB    int    `mapstructure:"max_size_mb" default:"1000"`
	MaxBackups   int    `mapstructure:"max_backups" default:"3"`
	MaxAgeDays   int    `mapstructure:"max_age_days" default:"7"`
}

// GetHyperspotHomeDir returns the resolved .hyperspot directory path
func GetHyperspotHomeDir() (string, error) {
	return ResolveHomeDir("~/.hyperspot")
}

// ResolveHomeDir resolves the home directory path based on OS and expands aliases
func ResolveHomeDir(homeDir string) (string, error) {
	if homeDir == "" {
		return "", fmt.Errorf("home dir path is not set")
	}

	switch runtime.GOOS {
	case "windows":
		homeDir = os.ExpandEnv(homeDir)
	case "darwin", "linux":
		homeDir = expandHomeDir(homeDir)
	default:
		homeDir = expandHomeDir(homeDir)
	}

	return filepath.Clean(homeDir), nil
}

// expandHomeDir expands ~/ to the user's home directory
func expandHomeDir(path string) string {
	if strings.HasPrefix(path, "~/") {
		home, err := os.UserHomeDir()
		if err != nil {
			return path // fallback to original path
		}
		return filepath.Join(home, path[2:])
	}
	return path
}

func getDefaultHomeDir() string {
	homeDir, err := GetHyperspotHomeDir()
	if err != nil {
		// Fallback to the original logic
		switch runtime.GOOS {
		case "windows":
			home := os.Getenv("USERPROFILE")
			if home == "" {
				home = os.Getenv("HOMEDRIVE") + os.Getenv("HOMEPATH")
			}
			return filepath.Join(home, ".hyperspot")
		case "darwin":
			return os.ExpandEnv("~/.hyperspot")
		case "linux":
			return os.ExpandEnv("~/.hyperspot")
		}
		return os.ExpandEnv("~/.hyperspot")
	}
	return homeDir
}

// ConfigComponent is an interface that configuration components must implement
type ConfigComponent interface {
	// Load loads and validates the configuration
	Load(name string, config map[string]interface{}) error

	// GetDefault returns the default configuration
	GetDefault() interface{}
}

// Register a list of known config loaders
var registeredLoggers = make(map[string]ConfigComponent)

// RegisterLogger registers a component that implements ConfigLogging
func RegisterLogger(name string, component ConfigComponent) {
	if _, ok := registeredLoggers[name]; ok {
		panic(fmt.Sprintf("Logger %s already registered", name))
	}
	registeredLoggers[name] = component
}

// Register a list of known config loaders
var registeredConfigComponents = make(map[string]ConfigComponent)

// RegisterConfigComponent registers a component that implements ConfigLoader
// The name can be dot-separated (e.g., "benchmarks.llm.coding.humaneval") to create a nested structure
func RegisterConfigComponent(name string, component ConfigComponent) {
	if _, ok := registeredConfigComponents[name]; ok {
		panic(fmt.Sprintf("Config component %s already registered", name))
	}
	registeredConfigComponents[name] = component
}

// Load loads the configuration from files and environment variables
func Load(configPaths ...string) (*Config, error) {
	if instance != nil {
		return instance, nil
	}

	var err error
	once.Do(func() {
		instance, err = load(configPaths...)
	})

	return instance, err
}

// load is the internal function that actually loads the config
func load(configPaths ...string) (*Config, error) {
	v := viper.New()

	// Set default values
	setDefaults(v)

	// Read config files
	for _, path := range configPaths {
		v.SetConfigFile(path)
		if err := v.MergeInConfig(); err != nil {
			return nil, fmt.Errorf("failed to read config file %s: %w", path, err)
		}
	}

	// Read from environment variables
	v.SetEnvPrefix("HYPERSPOT")
	v.AutomaticEnv()

	var config Config
	if err := v.Unmarshal(&config); err != nil {
		return nil, fmt.Errorf("failed to unmarshal config: %w", err)
	}

	// Execute all registered logger initializers
	for name, component := range registeredLoggers {
		// Get the config section for this component
		var componentConfig map[string]interface{}
		if subConfig := v.Sub(fmt.Sprintf("logging.%s", name)); subConfig != nil {
			componentConfig = subConfig.AllSettings()
		} else {
			// If no specific config exists, use an empty map
			componentConfig = make(map[string]interface{})
		}

		if err := component.Load(name, componentConfig); err != nil {
			return nil, fmt.Errorf("failed to load config for component %s: %w", name, err)
		}
	}

	// Execute all registered config components
	for name, component := range registeredConfigComponents {
		// Get the config section for this component
		var componentConfig map[string]interface{}

		// Handle nested keys (e.g., "benchmarks.llm.coding.humaneval")
		if strings.Contains(name, ".") {
			// For nested components, we need to get the deepest config section
			if subConfig := v.Sub(name); subConfig != nil {
				componentConfig = subConfig.AllSettings()
			} else {
				// If no specific config exists, use an empty map
				continue
			}
		} else {
			if subConfig := v.Sub(name); subConfig != nil {
				componentConfig = subConfig.AllSettings()
			} else {
				// If no specific config exists, use an empty map
				continue
			}
		}

		if componentConfig == nil {
			continue
		}

		if err := component.Load(name, componentConfig); err != nil {
			return nil, fmt.Errorf("failed to load config for component %s: %w", name, err)
		}
	}

	return &config, nil
}

// Reset clears the current configuration instance (mainly for testing)
func Reset() {
	mu.Lock()
	defer mu.Unlock()
	instance = nil
	once = sync.Once{}
}

var registeredLLMServiceConfigsMap = map[string]ConfigLLMService{}

func RegisterLLMServiceConfig(name string, config ConfigLLMService) {
	if _, ok := registeredLLMServiceConfigsMap[name]; ok {
		panic(fmt.Sprintf("LLM service config %s already registered", name))
	}
	registeredLLMServiceConfigsMap[name] = config
}

func setDefaults(v *viper.Viper) {
	// Server defaults
	v.SetDefault("server.host", "127.0.0.1")
	v.SetDefault("server.port", 8087)
	v.SetDefault("server.home_dir", getDefaultHomeDir())

	// Service configurations
	// Upstream defaults
	v.SetDefault("llm.services", registeredLLMServiceConfigsMap)

	// FIXME: make the schema and defaults aligned with the struct
	// FIXME: decentralize, split this config implementation by "services"

	for name, component := range registeredConfigComponents {
		v.SetDefault(fmt.Sprintf("logging.%s", name), component.GetDefault())
	}

	for name, component := range registeredConfigComponents {
		v.SetDefault(name, component.GetDefault())
	}
}

// GetDefault returns the default configuration
// addNestedComponent adds a component to a nested map structure based on the given path parts
func addNestedComponent(baseMap map[string]interface{}, parts []string, value interface{}) {
	if len(parts) == 0 {
		// No parts left, this shouldn't happen but handle gracefully
		return
	}

	if len(parts) == 1 {
		baseMap[parts[0]] = value
		return
	}

	// Create or get the current level map
	currentKey := parts[0]
	var currentMap map[string]interface{}

	if existing, ok := baseMap[currentKey]; ok {
		// If the key exists, try to convert it to a map
		if existingMap, ok := existing.(map[string]interface{}); ok {
			currentMap = existingMap
		} else {
			// If it exists but isn't a map, create a new map
			currentMap = make(map[string]interface{})
			baseMap[currentKey] = currentMap
		}
	} else {
		// If the key doesn't exist, create a new map
		currentMap = make(map[string]interface{})
		baseMap[currentKey] = currentMap
	}

	// Recursively add to the next level
	if len(parts) > 1 {
		addNestedComponent(currentMap, parts[1:], value)
	}
}

func GetDefault() *Config {
	v := viper.New()
	setDefaults(v)

	var config Config
	if err := v.Unmarshal(&config); err != nil {
		// If we can't unmarshal the defaults, we should panic since this is a critical error
		panic(fmt.Errorf("failed to unmarshal default config: %w", err))
	}

	// Set the instance
	mu.Lock()
	instance = &config
	mu.Unlock()

	// Execute all registered config components
	for name, component := range registeredLoggers {
		// Get the config section for this component
		var componentConfig map[string]interface{}
		if subConfig := v.Sub(fmt.Sprintf("logging.%s", name)); subConfig != nil {
			componentConfig = subConfig.AllSettings()
		} else {
			// If no specific config exists, use an empty map
			componentConfig = make(map[string]interface{})
		}

		if err := component.Load(name, componentConfig); err != nil {
			panic(fmt.Errorf("failed to load config for component %s: %w", name, err))
		}
	}

	// Execute all registered config components
	for name, component := range registeredConfigComponents {
		// Get the config section for this component
		var componentConfig map[string]interface{}
		if subConfig := v.Sub(name); subConfig != nil {
			componentConfig = subConfig.AllSettings()
		} else {
			// If no specific config exists, use an empty map
			componentConfig = make(map[string]interface{})
		}

		if err := component.Load(name, componentConfig); err != nil {
			panic(fmt.Errorf("failed to load config for component %s: %w", name, err))
		}
	}

	return &config
}

// ConfigUpstream holds configuration for upstream requests to LLM services.
type ConfigUpstream struct {
	ShortTimeoutSec int  `mapstructure:"short_timeout_sec" default:"5"`  // can be used for LLM operations requests
	LongTimeoutSec  int  `mapstructure:"long_timeout_sec" default:"180"` // can be used for LLM chat requests
	VerifyCert      bool `mapstructure:"verify_cert"`
	EnableDiscovery bool `mapstructure:"enable_discovery"`
}

func getLargestLLMTimeout() time.Duration {
	cfg := Get()

	largestTimeout := 60 * time.Second
	if cfg == nil || cfg.LLM.LLMServices == nil || len(cfg.LLM.LLMServices) == 0 {
		return largestTimeout
	}

	for _, service := range cfg.LLM.LLMServices {
		serviceTimeout := time.Duration(service.Upstream.LongTimeoutSec) * time.Second
		if serviceTimeout > largestTimeout {
			largestTimeout = serviceTimeout
		}
	}

	return largestTimeout
}

func GetServerTimeout() time.Duration {
	cfg := Get()

	if cfg != nil && cfg.Server.TimeoutSec > 0 {
		return time.Duration(cfg.Server.TimeoutSec) * time.Second
	}

	return getLargestLLMTimeout()
}

// UpdateStructFromConfig updates the *toStruct* fields from a config map
// represented as *fromConfig*. The function returns an error with a list of
// problematic fields in case of failure. The field name is taken from the
// mapstructure tag if available, otherwise from the struct field name.
//
// UpdateStructFromConfig ensures that the *fromConfig* doesn't have a field that
// is not present in *toStruct*, and also validates the field types match.
// All the nested fields are examined recursively as well.
//
// This function is primarily designed for configuration structs that use
// mapstructure tags for field mapping. It performs detailed validation
// before attempting to update the destination struct.
func UpdateStructFromConfig(toStruct any, fromConfig map[string]interface{}) error {
	// Handle nil cases first
	if fromConfig == nil {
		return fmt.Errorf("source config cannot be nil")
	}

	// Check if 'toStruct' is a pointer
	toVal := reflect.ValueOf(toStruct)
	if toVal.Kind() != reflect.Ptr || toVal.IsNil() {
		return fmt.Errorf("destination must be a non-nil pointer")
	}

	// Check if 'toStruct' points to a struct
	toElemType := toVal.Type().Elem() // Get the type that the pointer points to
	if toElemType.Kind() != reflect.Struct {
		return fmt.Errorf("destination must be a pointer to a struct, got pointer to %v", toElemType.Kind())
	}

	// Use mapstructure for the copy operation to respect mapstructure tags
	decoder, err := mapstructure.NewDecoder(&mapstructure.DecoderConfig{
		TagName:          "mapstructure",
		ErrorUnused:      true,
		WeaklyTypedInput: true,
		Result:           toStruct,
		ZeroFields:       false,
	})
	if err != nil {
		return fmt.Errorf("error creating mapstructure decoder for copy: %v", err)
	}

	return decoder.Decode(fromConfig)
}

// copyValues copies values from source to destination when they're the same type
// validateNestedFields recursively checks if all fields in the source map exist in the destination struct
func validateNestedFields(sourceMap map[string]interface{}, destType reflect.Type, prefix string, extraFields []string) []string {
	for sourceKey, sourceValue := range sourceMap {
		// Try to find a matching field in the destination struct
		var fieldFound bool
		var matchingField reflect.StructField

		// Check each field in the destination struct
		for i := 0; i < destType.NumField(); i++ {
			destField := destType.Field(i)

			// Get field name from mapstructure tag if available
			fieldName := destField.Name
			if tag, ok := destField.Tag.Lookup("mapstructure"); ok && tag != "" {
				// Handle tag options (e.g., mapstructure:"name,omitempty")
				tagParts := strings.Split(tag, ",")
				if tagParts[0] != "" {
					fieldName = tagParts[0]
				}
			}

			// Check if this field matches the source key
			if fieldName == sourceKey {
				fieldFound = true
				matchingField = destField
				break
			}
		}

		// If field not found in destination, add to extra fields list
		if !fieldFound {
			fieldPath := sourceKey
			if prefix != "" {
				fieldPath = prefix + "." + sourceKey
			}
			extraFields = append(extraFields, fmt.Sprintf("field '%s' exists in source but not in destination", fieldPath))
		} else {
			// If field is found and it's a nested map in the source, check if destination field is a struct
			if nestedMap, isMap := sourceValue.(map[string]interface{}); isMap {
				nestedFieldType := matchingField.Type

				// If destination field is a pointer to a struct, get the struct type
				if nestedFieldType.Kind() == reflect.Ptr {
					nestedFieldType = nestedFieldType.Elem()
				}

				// Only recurse if destination field is a struct
				if nestedFieldType.Kind() == reflect.Struct {
					fieldPath := sourceKey
					if prefix != "" {
						fieldPath = prefix + "." + sourceKey
					}
					extraFields = validateNestedFields(nestedMap, nestedFieldType, fieldPath, extraFields)
				}
			}
		}
	}

	return extraFields
}
