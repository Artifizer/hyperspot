package file_parser

import (
	"github.com/hypernetix/hyperspot/libs/config"
)

// FileParserConfig holds configuration for parsers including mapping of parser IDs to supported file extensions
type FileParserConfig struct {
	// Map of parser ID to supported file extensions
	ParserExtensionMap    map[string][]string `mapstructure:"parser_extension_map"`
	UploadedFileMaxSizeKB int                 `mapstructure:"uploaded_file_max_size_kb"`
	UploadedFileTempDir   string              `mapstructure:"uploaded_file_temp_dir"`
}

// Default instance of parser configuration
// This will be populated based on the parserMap in file_parser_base.go
var fileParserConfigInstance = &FileParserConfig{
	ParserExtensionMap: map[string][]string{
		"builtin": {},
	},
	UploadedFileMaxSizeKB: 16 * 1024,
	UploadedFileTempDir:   "", // empty means will be set by the system
}

// GetDefault returns the default configuration
func (c *FileParserConfig) GetDefault() interface{} {
	return c
}

// Load loads configuration from the provided config dictionary
func (c *FileParserConfig) Load(name string, configDict map[string]interface{}) error {
	cfg := &FileParserConfig{}
	if err := config.UpdateStructFromConfig(cfg, configDict); err != nil {
		return err
	}

	// Update the instance with loaded configuration values
	if len(cfg.ParserExtensionMap) > 0 {
		fileParserConfigInstance.ParserExtensionMap = cfg.ParserExtensionMap
	}

	// Update other configuration fields if they are provided
	if cfg.UploadedFileMaxSizeKB > 0 {
		fileParserConfigInstance.UploadedFileMaxSizeKB = cfg.UploadedFileMaxSizeKB
	}

	if cfg.UploadedFileTempDir != "" {
		fileParserConfigInstance.UploadedFileTempDir = cfg.UploadedFileTempDir
	}

	return nil
}

// GetSupportedExtensions returns the list of supported file extensions for a given parser ID
func GetSupportedExtensions(parserID string) []string {
	if extensions, ok := fileParserConfigInstance.ParserExtensionMap[parserID]; ok {
		return extensions
	}
	return []string{}
}

// IsSupportedExtension checks if a file extension is supported by a given parser ID
func IsSupportedExtension(parserID string, extension string) bool {
	if extensions, ok := fileParserConfigInstance.ParserExtensionMap[parserID]; ok {
		for _, ext := range extensions {
			if ext == extension {
				return true
			}
		}
	}
	return false
}

// GetParserIDs returns all configured parser IDs
func GetParserIDs() []string {
	ids := make([]string, 0, len(fileParserConfigInstance.ParserExtensionMap))
	for id := range fileParserConfigInstance.ParserExtensionMap {
		ids = append(ids, id)
	}
	return ids
}

func init() {
	// Initialize the ParserExtensionMap based on the parserMap
	// This ensures that the config only includes extensions that have parser implementations
	for ext := range parserMap {
		fileParserConfigInstance.ParserExtensionMap["builtin"] = append(
			fileParserConfigInstance.ParserExtensionMap["builtin"], ext)
	}

	config.RegisterConfigComponent("file_parser", fileParserConfigInstance)
}
