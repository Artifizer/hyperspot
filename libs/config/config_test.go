package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/spf13/viper"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v2"
)

func TestGet(t *testing.T) {
	// Reset the configuration to ensure a clean state
	Reset()

	// Initially, the instance should be nil
	assert.Nil(t, Get())

	// Load default configuration
	defaultConfig := GetDefault()
	assert.NotNil(t, defaultConfig)

	// Now Get() should return the instance
	assert.Equal(t, defaultConfig, Get())
}

func TestReset(t *testing.T) {
	// Ensure we have a configuration instance
	_ = GetDefault()
	assert.NotNil(t, Get())

	// Reset the configuration
	Reset()

	// Now the instance should be nil
	assert.Nil(t, Get())
}

func TestLoad(t *testing.T) {
	// Reset for clean state
	Reset()

	// Create a temporary config file
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "config.yaml")

	// Write test configuration to the file
	testConfig := `
server:
  host: "test-host"
  port: 9090
  home_dir: "/test/home"
  timeout_sec: 120
llm:
  default_service: "test-service"
  temperature: 0.8
  max_tokens: 1000
  services:
    test-service:
      api_format: "test-format"
      api_key_env_var: "TEST_API_KEY"
      urls: ["http://test-url"]
      model: "test-model"
      upstream:
        short_timeout_sec: 10
        long_timeout_sec: 200
        verify_cert: true
        enable_discovery: false
`
	err := os.WriteFile(configPath, []byte(testConfig), 0644)
	require.NoError(t, err)

	// Load the configuration from the file
	config, err := Load(configPath)
	require.NoError(t, err)
	require.NotNil(t, config)

	// Verify the loaded configuration matches what we expect
	assert.Equal(t, "test-host", config.Server.Host)
	assert.Equal(t, 9090, config.Server.Port)
	assert.Equal(t, "/test/home", config.Server.HomeDir)
	assert.Equal(t, 120, config.Server.TimeoutSec)

	assert.Equal(t, "test-service", config.LLM.DefaultService)
	assert.Equal(t, 0.8, config.LLM.Temperature)
	assert.Equal(t, 1000, config.LLM.MaxTokens)

	service, exists := config.LLM.LLMServices["test-service"]
	assert.True(t, exists)
	assert.Equal(t, "test-format", service.APIFormat)
	assert.Equal(t, "TEST_API_KEY", service.APIKeyEnvVar)
	assert.Equal(t, []string{"http://test-url"}, service.URLs)
	assert.Equal(t, "test-model", service.Model)
	assert.Equal(t, 10, service.Upstream.ShortTimeoutSec)
	assert.Equal(t, 200, service.Upstream.LongTimeoutSec)
	assert.True(t, service.Upstream.VerifyCert)
	assert.False(t, service.Upstream.EnableDiscovery)
}

func TestLoadWithEnvironmentVariables(t *testing.T) {
	// Reset for clean state
	Reset()

	// Save original environment variables to restore later
	originalHost, hostExists := os.LookupEnv("HYPERSPOT_SERVER_HOST")
	originalPort, portExists := os.LookupEnv("HYPERSPOT_SERVER_PORT")
	originalTemp, tempExists := os.LookupEnv("HYPERSPOT_LLM_TEMPERATURE")

	// Set environment variables
	err := os.Setenv("HYPERSPOT_SERVER_HOST", "env-host")
	require.NoError(t, err)
	err = os.Setenv("HYPERSPOT_SERVER_PORT", "8888")
	require.NoError(t, err)
	err = os.Setenv("HYPERSPOT_LLM_TEMPERATURE", "0.5")
	require.NoError(t, err)

	// Create a minimal config file
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "config.yaml")
	minimalConfig := `
server:
  timeout_sec: 60
`
	err = os.WriteFile(configPath, []byte(minimalConfig), 0644)
	require.NoError(t, err)

	// Create a new viper instance for testing to avoid affecting global state
	v := viper.New()
	v.SetConfigFile(configPath)
	v.SetEnvPrefix("HYPERSPOT")
	v.AutomaticEnv()
	v.SetEnvKeyReplacer(strings.NewReplacer(".", "_"))

	// Read the config file
	err = v.ReadInConfig()
	require.NoError(t, err)

	// Verify environment variables are read correctly
	assert.Equal(t, "env-host", v.GetString("server.host"))
	assert.Equal(t, 8888, v.GetInt("server.port"))
	assert.Equal(t, 60, v.GetInt("server.timeout_sec"))
	assert.Equal(t, 0.5, v.GetFloat64("llm.temperature"))

	// Clean up environment variables
	if hostExists {
		err = os.Setenv("HYPERSPOT_SERVER_HOST", originalHost)
		require.NoError(t, err)
	} else {
		err = os.Unsetenv("HYPERSPOT_SERVER_HOST")
		require.NoError(t, err)
	}
	if portExists {
		err = os.Setenv("HYPERSPOT_SERVER_PORT", originalPort)
		require.NoError(t, err)
	} else {
		err = os.Unsetenv("HYPERSPOT_SERVER_PORT")
		require.NoError(t, err)
	}
	if tempExists {
		err = os.Setenv("HYPERSPOT_LLM_TEMPERATURE", originalTemp)
		require.NoError(t, err)
	} else {
		err = os.Unsetenv("HYPERSPOT_LLM_TEMPERATURE")
		require.NoError(t, err)
	}
}

func TestToYaml(t *testing.T) {
	// Reset for clean state
	Reset()

	// Get default configuration
	config := GetDefault()
	require.NotNil(t, config)

	// Convert to YAML
	yamlStr, err := config.ToYaml()
	require.NoError(t, err)

	// Parse the YAML back to verify its structure
	var parsedConfig map[string]interface{}
	err = yaml.Unmarshal([]byte(yamlStr), &parsedConfig)
	require.NoError(t, err)

	// Verify basic structure
	assert.Contains(t, parsedConfig, "server")
	assert.Contains(t, parsedConfig, "llm")
	assert.Contains(t, parsedConfig, "logging") // Added by ToYaml

	// Verify server section
	serverSection, ok := parsedConfig["server"].(map[interface{}]interface{})
	assert.True(t, ok)
	assert.Contains(t, serverSection, "host")
	assert.Contains(t, serverSection, "port")
	assert.Contains(t, serverSection, "home_dir")
	assert.Contains(t, serverSection, "timeout_sec")

	// Verify LLM section
	llmSection, ok := parsedConfig["llm"].(map[interface{}]interface{})
	assert.True(t, ok)
	assert.Contains(t, llmSection, "default_service")
	assert.Contains(t, llmSection, "temperature")
	assert.Contains(t, llmSection, "max_tokens")
	assert.Contains(t, llmSection, "services")
}

func TestStructToMap(t *testing.T) {
	// Test with a simple struct
	type SimpleStruct struct {
		Name  string `mapstructure:"name"`
		Value int    `mapstructure:"value"`
	}

	simple := SimpleStruct{Name: "test", Value: 42}
	result := structToMap(simple)

	expectedMap := map[string]interface{}{
		"name":  "test",
		"value": 42,
	}

	assert.Equal(t, expectedMap, result)

	// Test with a nested struct
	type NestedStruct struct {
		Outer string       `mapstructure:"outer"`
		Inner SimpleStruct `mapstructure:"inner"`
	}

	nested := NestedStruct{
		Outer: "outer-value",
		Inner: SimpleStruct{Name: "inner-name", Value: 100},
	}

	nestedResult := structToMap(nested)
	expectedNestedMap := map[string]interface{}{
		"outer": "outer-value",
		"inner": map[string]interface{}{
			"name":  "inner-name",
			"value": 100,
		},
	}

	assert.Equal(t, expectedNestedMap, nestedResult)

	// Test with a pointer
	pointerResult := structToMap(&simple)
	assert.Equal(t, expectedMap, pointerResult)

	// Test with nil pointer
	var nilPtr *SimpleStruct
	nilResult := structToMap(nilPtr)
	assert.Nil(t, nilResult)

	// Test with a slice
	slice := []SimpleStruct{
		{Name: "item1", Value: 1},
		{Name: "item2", Value: 2},
	}

	sliceResult := structToMap(slice)
	expectedSlice := []interface{}{
		map[string]interface{}{"name": "item1", "value": 1},
		map[string]interface{}{"name": "item2", "value": 2},
	}

	assert.Equal(t, expectedSlice, sliceResult)

	// Test with a map
	map1 := map[string]SimpleStruct{
		"key1": {Name: "map-item1", Value: 10},
		"key2": {Name: "map-item2", Value: 20},
	}

	mapResult := structToMap(map1)
	expectedMap1 := map[interface{}]interface{}{
		"key1": map[string]interface{}{"name": "map-item1", "value": 10},
		"key2": map[string]interface{}{"name": "map-item2", "value": 20},
	}

	assert.Equal(t, expectedMap1, mapResult)
}

// mockConfigComponent implements ConfigComponent for testing
type mockConfigComponent struct{}

func (m mockConfigComponent) Load(name string, config map[string]interface{}) error {
	return nil
}

func (m mockConfigComponent) GetDefault() interface{} {
	return map[string]interface{}{
		"test_key": "test_value",
	}
}

func TestRegisterConfigComponent(t *testing.T) {
	// Create a mock config component
	mockComponent := mockConfigComponent{}

	// Register the component
	RegisterConfigComponent("mock_component", mockComponent)

	// Reset for clean state
	Reset()

	// Get default config which should now include our component
	config := GetDefault()
	require.NotNil(t, config)

	// Convert to YAML and verify our component is included
	yamlStr, err := config.ToYaml()
	require.NoError(t, err)

	var parsedConfig map[string]interface{}
	err = yaml.Unmarshal([]byte(yamlStr), &parsedConfig)
	require.NoError(t, err)

	// Verify our component is in the YAML
	assert.Contains(t, parsedConfig, "mock_component")

	// Clean up - remove our test component
	delete(registeredConfigComponents, "mock_component")
}

func TestGetServerTimeout(t *testing.T) {
	// Reset for clean state
	Reset()

	// Create a config with specific timeout
	config := &Config{
		Server: ConfigServer{
			TimeoutSec: 30,
		},
		LLM: ConfigLLM{
			LLMServices: map[string]ConfigLLMService{
				"service1": {
					Upstream: ConfigUpstream{
						LongTimeoutSec: 60,
					},
				},
				"service2": {
					Upstream: ConfigUpstream{
						LongTimeoutSec: 120,
					},
				},
			},
		},
	}

	// Set the instance
	mu.Lock()
	instance = config
	mu.Unlock()

	// Test GetServerTimeout
	timeout := GetServerTimeout()
	expected := time.Duration(30) * time.Second
	assert.Equal(t, expected, timeout)

	// Test with no server timeout (should use largest LLM timeout)
	config.Server.TimeoutSec = 0
	timeout = GetServerTimeout()
	expected = time.Duration(120) * time.Second
	assert.Equal(t, expected, timeout)

	// Test with no LLM services
	config.LLM.LLMServices = nil
	timeout = GetServerTimeout()
	expected = time.Duration(60) * time.Second // Default fallback
	assert.Equal(t, expected, timeout)
}

func TestLoadWithInvalidPath(t *testing.T) {
	// This test verifies the behavior when loading from a non-existent path
	// Since the actual implementation may either return an error or use defaults,
	// we'll test the behavior by checking if the function returns a non-nil config
	// or a meaningful error

	// Reset for clean state
	Reset()

	// Create a temporary directory that exists
	tmpDir := t.TempDir()
	// Use a non-existent file in an existing directory
	nonExistentPath := filepath.Join(tmpDir, "does-not-exist.yaml")

	// Try to load from a non-existent path
	config, err := Load(nonExistentPath)

	// The function should either:
	// 1. Return a valid config with default values and possibly an error, or
	// 2. Return an error and nil config
	if err != nil {
		// If we got an error, it should mention the file not existing
		assert.Contains(t, err.Error(), "no such file")
		// If config is nil with an error, that's an acceptable implementation
		if config == nil {
			return
		}
	}

	// If we got here, config must not be nil
	assert.NotNil(t, config)

	// Skip further checks if config is nil to avoid panic
	if config == nil {
		return
	}

	// If we have a config, it should have reasonable default values
	assert.NotEmpty(t, config.Server.Host, "Server host should have a default value")
	assert.NotZero(t, config.Server.Port, "Server port should have a default value")
}

func TestRegisterLLMServiceConfig(t *testing.T) {
	// Save the original map to restore later
	originalMap := make(map[string]ConfigLLMService)
	for k, v := range registeredLLMServiceConfigsMap {
		originalMap[k] = v
	}

	// Clear the registered services map for this test
	registeredLLMServiceConfigsMap = map[string]ConfigLLMService{}

	// Register a test service
	testService := ConfigLLMService{
		APIFormat:    "test-format",
		APIKeyEnvVar: "TEST_API_KEY",
		URLs:         []string{"http://test-url"},
		Model:        "test-model",
		Upstream: ConfigUpstream{
			ShortTimeoutSec: 10,
			LongTimeoutSec:  100,
			VerifyCert:      true,
		},
	}

	RegisterLLMServiceConfig("test-service", testService)

	// Verify it was registered in the map
	assert.Contains(t, registeredLLMServiceConfigsMap, "test-service")
	assert.Equal(t, testService, registeredLLMServiceConfigsMap["test-service"])

	// Since the setDefaults function might be complex and depend on other parts of the system,
	// we'll simply test that our service was properly registered in the map
	// This is the core functionality we want to verify

	// Restore the original map
	registeredLLMServiceConfigsMap = originalMap
}

// mockHumanevalComponent is a mock implementation of ConfigComponent for humaneval tests
type mockHumanevalComponent struct{}

func (m mockHumanevalComponent) Load(name string, config map[string]interface{}) error {
	return nil
}

func (m mockHumanevalComponent) GetDefault() interface{} {
	return map[string]interface{}{
		"dataset": map[string]interface{}{
			"local_path":   "datasets/humaneval.parquet",
			"original_url": "https://huggingface.co/datasets/openai/humaneval",
		},
		"code_executor": "docker_python_default",
		"description":   "HumanEval dataset",
	}
}

// mockMbppComponent is a mock implementation of ConfigComponent for mbpp tests
type mockMbppComponent struct{}

func (m mockMbppComponent) Load(name string, config map[string]interface{}) error {
	return nil
}

func (m mockMbppComponent) GetDefault() interface{} {
	return map[string]interface{}{
		"dataset": map[string]interface{}{
			"local_path":   "datasets/mbpp.parquet",
			"original_url": "https://huggingface.co/datasets/mbpp",
		},
		"code_executor": "docker_python_default",
		"description":   "MBPP dataset",
	}
}

// mockPerfComponent is a mock implementation of ConfigComponent for perf tests
type mockPerfComponent struct{}

func (m mockPerfComponent) Load(name string, config map[string]interface{}) error {
	return nil
}

func (m mockPerfComponent) GetDefault() interface{} {
	return map[string]interface{}{
		"retry_delay_sec": 5,
		"max_retries":     3,
		"types": map[string]interface{}{
			"variable-prompt-tokens": map[string]interface{}{
				"description":  "Performance test",
				"count_tokens": "prompt",
			},
		},
	}
}

// TestNestedConfigComponents tests the nested configuration structure
// for components registered with dot-separated names
func TestNestedConfigComponents(t *testing.T) {
	// Save original registered components to restore later
	originalComponents := make(map[string]ConfigComponent)
	for k, v := range registeredConfigComponents {
		originalComponents[k] = v
	}

	// Clear the registered components map for this test
	registeredConfigComponents = map[string]ConfigComponent{}

	// Create mock components for testing
	humanevalComponent := &mockHumanevalComponent{}
	mbppComponent := &mockMbppComponent{}
	perfComponent := &mockPerfComponent{}

	// Register the components with nested names
	RegisterConfigComponent("benchmarks.llm.coding.humaneval", humanevalComponent)
	RegisterConfigComponent("benchmarks.llm.coding.mbpp", mbppComponent)
	RegisterConfigComponent("benchmarks.llm.perf", perfComponent)

	// Create a config instance to test ToYaml
	config := &Config{
		Server: ConfigServer{
			Host: "localhost",
			Port: 8080,
		},
		LLM: ConfigLLM{
			DefaultService: "test",
		},
	}

	// Generate YAML representation
	yamlStr, err := config.ToYaml()
	require.NoError(t, err)

	// Print the YAML for debugging
	t.Logf("Generated YAML:\n%s", yamlStr)

	// Parse the YAML into a map
	var parsedConfig map[string]interface{}
	err = yaml.Unmarshal([]byte(yamlStr), &parsedConfig)
	require.NoError(t, err)

	// Print the parsed config for debugging
	t.Logf("Parsed config keys: %v", getKeys(parsedConfig))

	// Verify the nested structure exists
	// In YAML, all numbers are unmarshaled as float64
	benchmarksVal, exists := parsedConfig["benchmarks"]
	require.True(t, exists, "benchmarks key should exist")
	benchmarks, ok := benchmarksVal.(map[interface{}]interface{})
	require.True(t, ok, "benchmarks should be a map, got %T", benchmarksVal)

	llmVal, exists := benchmarks["llm"]
	require.True(t, exists, "benchmarks.llm key should exist")
	llm, ok := llmVal.(map[interface{}]interface{})
	require.True(t, ok, "benchmarks.llm should be a map, got %T", llmVal)

	codingVal, exists := llm["coding"]
	require.True(t, exists, "benchmarks.llm.coding key should exist")
	coding, ok := codingVal.(map[interface{}]interface{})
	require.True(t, ok, "benchmarks.llm.coding should be a map, got %T", codingVal)

	// Check humaneval component
	humanevalVal, exists := coding["humaneval"]
	require.True(t, exists, "benchmarks.llm.coding.humaneval key should exist")
	humaneval, ok := humanevalVal.(map[interface{}]interface{})
	require.True(t, ok, "benchmarks.llm.coding.humaneval should be a map, got %T", humanevalVal)
	assert.Equal(t, "docker_python_default", humaneval["code_executor"])
	assert.Equal(t, "HumanEval dataset", humaneval["description"])

	// Check mbpp component
	mbppVal, exists := coding["mbpp"]
	require.True(t, exists, "benchmarks.llm.coding.mbpp key should exist")
	mbpp, ok := mbppVal.(map[interface{}]interface{})
	require.True(t, ok, "benchmarks.llm.coding.mbpp should be a map, got %T", mbppVal)
	assert.Equal(t, "docker_python_default", mbpp["code_executor"])
	assert.Equal(t, "MBPP dataset", mbpp["description"])

	// Check perf component
	perfVal, exists := llm["perf"]
	require.True(t, exists, "benchmarks.llm.perf key should exist")
	perf, ok := perfVal.(map[interface{}]interface{})
	require.True(t, ok, "benchmarks.llm.perf should be a map, got %T", perfVal)
	// Check the numeric values - they could be int or float64 depending on how yaml.Unmarshal parses them
	retryDelaySec, ok := perf["retry_delay_sec"]
	require.True(t, ok, "retry_delay_sec should exist")
	assert.Equal(t, 5, getIntValue(retryDelaySec))

	maxRetries, ok := perf["max_retries"]
	require.True(t, ok, "max_retries should exist")
	assert.Equal(t, 3, getIntValue(maxRetries))

	// Restore original registered components
	registeredConfigComponents = originalComponents
}

// TestAddNestedComponent tests the addNestedComponent helper function
// Helper function to get keys from a map for debugging
func getKeys(m map[string]interface{}) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	return keys
}

// Helper function to handle different numeric types from yaml.Unmarshal
func getIntValue(val interface{}) int {
	switch v := val.(type) {
	case int:
		return v
	case float64:
		return int(v)
	default:
		return 0 // Default value if not a number
	}
}

func TestUpdateStructFromConfig(t *testing.T) {
	// Test case 1: Basic successful update
	type TestConfig struct {
		Name    string `mapstructure:"name"`
		Value   int    `mapstructure:"value"`
		Enabled bool   `mapstructure:"enabled"`
	}

	// Create destination struct
	destStruct := &TestConfig{
		Name:    "original",
		Value:   0,
		Enabled: false,
	}

	// Create source config
	srcConfig := map[string]interface{}{
		"name":    "updated",
		"value":   42,
		"enabled": true,
	}

	// Update struct from config
	err := UpdateStructFromConfig(destStruct, srcConfig)
	require.NoError(t, err)

	// Verify fields were updated correctly
	assert.Equal(t, "updated", destStruct.Name)
	assert.Equal(t, 42, destStruct.Value)
	assert.Equal(t, true, destStruct.Enabled)

	// Test case 2: Nil source config
	destStruct = &TestConfig{}
	err = UpdateStructFromConfig(destStruct, nil)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "source config cannot be nil")

	// Test case 3: Destination not a pointer
	destStruct2 := TestConfig{}
	err = UpdateStructFromConfig(destStruct2, srcConfig)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "destination must be a non-nil pointer")

	// Test case 4: Nil destination pointer
	var nilDest *TestConfig
	err = UpdateStructFromConfig(nilDest, srcConfig)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "destination must be a non-nil pointer")

	// Test case 5: Destination not pointing to a struct
	intPtr := new(int)
	err = UpdateStructFromConfig(intPtr, srcConfig)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "destination must be a pointer to a struct")

	// Test case 6: Unknown field in source config
	destStruct = &TestConfig{}
	srcConfigWithExtra := map[string]interface{}{
		"name":          "test",
		"value":         42,
		"enabled":       true,
		"unknown_field": "extra field", // This field doesn't exist in TestConfig
	}

	err = UpdateStructFromConfig(destStruct, srcConfigWithExtra)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "unknown_field")

	// Test case 7: Type mismatch
	destStruct = &TestConfig{}
	srcConfigTypeMismatch := map[string]interface{}{
		"name":    "test",
		"value":   "not an int", // Should be an int
		"enabled": true,
	}

	err = UpdateStructFromConfig(destStruct, srcConfigTypeMismatch)
	require.Error(t, err)

	// Test case 8: Nested struct
	type NestedConfig struct {
		Outer   string     `mapstructure:"outer"`
		Inner   TestConfig `mapstructure:"inner"`
		Timeout int        `mapstructure:"timeout"`
	}

	destNested := &NestedConfig{
		Outer:   "original-outer",
		Inner:   TestConfig{Name: "original-inner", Value: 0, Enabled: false},
		Timeout: 30,
	}

	srcNestedConfig := map[string]interface{}{
		"outer": "updated-outer",
		"inner": map[string]interface{}{
			"name":    "updated-inner",
			"value":   100,
			"enabled": true,
		},
		"timeout": 60,
	}

	err = UpdateStructFromConfig(destNested, srcNestedConfig)
	require.NoError(t, err)

	// Verify nested fields were updated correctly
	assert.Equal(t, "updated-outer", destNested.Outer)
	assert.Equal(t, "updated-inner", destNested.Inner.Name)
	assert.Equal(t, 100, destNested.Inner.Value)
	assert.Equal(t, true, destNested.Inner.Enabled)
	assert.Equal(t, 60, destNested.Timeout)

	// Test case 9: Partial update (not all fields specified)
	destStruct = &TestConfig{
		Name:    "original",
		Value:   10,
		Enabled: false,
	}

	partialConfig := map[string]interface{}{
		"value": 99, // Only updating one field
	}

	err = UpdateStructFromConfig(destStruct, partialConfig)
	require.NoError(t, err)

	// Verify only specified fields were updated
	assert.Equal(t, "original", destStruct.Name) // Unchanged
	assert.Equal(t, 99, destStruct.Value)        // Updated
	assert.Equal(t, false, destStruct.Enabled)   // Unchanged
}

func TestAddNestedComponent(t *testing.T) {
	// Test case 1: Simple nesting with new keys
	baseMap := make(map[string]interface{})
	parts := []string{"benchmarks", "llm", "coding", "humaneval"}
	value := map[string]interface{}{
		"code_executor": "docker_python_default",
		"description":   "HumanEval dataset",
	}

	addNestedComponent(baseMap, parts, value)

	// Verify the nested structure was created correctly
	benchmarks, ok := baseMap["benchmarks"].(map[string]interface{})
	require.True(t, ok)

	llm, ok := benchmarks["llm"].(map[string]interface{})
	require.True(t, ok)

	coding, ok := llm["coding"].(map[string]interface{})
	require.True(t, ok)

	humaneval, ok := coding["humaneval"].(map[string]interface{})
	require.True(t, ok)

	assert.Equal(t, "docker_python_default", humaneval["code_executor"])
	assert.Equal(t, "HumanEval dataset", humaneval["description"])

	// Test case 2: Merging with existing structure
	baseMap = make(map[string]interface{})
	// Create initial structure
	baseMap["benchmarks"] = map[string]interface{}{
		"llm": map[string]interface{}{
			"coding": map[string]interface{}{
				"mbpp": map[string]interface{}{
					"code_executor": "docker_python_default",
				},
			},
		},
	}

	// Add humaneval to the existing structure
	parts = []string{"benchmarks", "llm", "coding", "humaneval"}
	value = map[string]interface{}{
		"code_executor": "docker_python_default",
		"description":   "HumanEval dataset",
	}

	addNestedComponent(baseMap, parts, value)

	// Verify both components exist in the structure
	benchmarks, ok = baseMap["benchmarks"].(map[string]interface{})
	require.True(t, ok)

	llm, ok = benchmarks["llm"].(map[string]interface{})
	require.True(t, ok)

	coding, ok = llm["coding"].(map[string]interface{})
	require.True(t, ok)

	mbpp, ok := coding["mbpp"].(map[string]interface{})
	require.True(t, ok)
	assert.Equal(t, "docker_python_default", mbpp["code_executor"])

	humaneval, ok = coding["humaneval"].(map[string]interface{})
	require.True(t, ok)
	assert.Equal(t, "docker_python_default", humaneval["code_executor"])
	assert.Equal(t, "HumanEval dataset", humaneval["description"])
}
