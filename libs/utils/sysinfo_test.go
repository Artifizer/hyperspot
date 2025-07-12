package utils

import (
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
)

func TestCollectSysInfo(t *testing.T) {
	sysInfo, err := CollectSysInfo()
	assert.NoError(t, err, "CollectSysInfo should not return an error")
	assert.NotNil(t, sysInfo, "SystemInfo should not be nil")

	// Check that the generated UUID is not nil.
	assert.NotEqual(t, uuid.Nil, sysInfo.ID, "SystemInfo ID must not be nil")

	// CreatedAt should be within a reasonable range (e.g. within the last 2 minutes).
	assert.WithinDuration(t, time.Now(), time.UnixMilli(sysInfo.CreatedAtMs), 1*time.Second, "CreatedAt timestamp should be recent")

	// OS info should be available.
	assert.NotEmpty(t, sysInfo.OS.Name, "OS Name must be set")
	assert.NotEmpty(t, sysInfo.OS.Version, "OS Version must be set")
	assert.NotEmpty(t, sysInfo.OS.Arch, "OS Arch must be set")

	// CPU info
	assert.True(t, sysInfo.CPU.NumCPUs > 0, "There should be at least one CPU")
	// Even if Model and Frequency are not available on every system, cores should be a valid non-negative number.
	assert.True(t, sysInfo.CPU.Cores >= 0, "CPU cores should be zero or more")

	// Memory info
	assert.True(t, sysInfo.Memory.Total > 0, "Total memory should be greater than zero")

	// Host information
	assert.NotEmpty(t, sysInfo.Host.Hostname, "Hostname must be set")
	// Uptime may be zero if the system just started, so we only check it's non-negative.
	assert.True(t, sysInfo.Host.Uptime >= 0, "Uptime should be non-negative")
}

func TestFormatString(t *testing.T) {
	sysInfo, err := CollectSysInfo()
	assert.NoError(t, err, "CollectSysInfo should not error")

	formatted := sysInfo.FormatString()
	assert.NotEmpty(t, formatted, "FormatString should not return an empty string")

	// Check that the returned string contains key info.
	assert.True(t, strings.Contains(formatted, "System Information:"), "Formatted string should contain header")
	assert.True(t, strings.Contains(formatted, sysInfo.OS.Name), "Formatted string should contain OS Name")
	assert.True(t, strings.Contains(formatted, sysInfo.Host.Hostname), "Formatted string should contain Hostname")
}

func TestToMap(t *testing.T) {
	sysInfo, err := CollectSysInfo()
	assert.NoError(t, err, "CollectSysInfo should succeed")

	m := sysInfo.ToMap()
	// Verify expected keys exist in the map.
	expectedKeys := []string{
		"id",
		"created_at",
		"os_name",
		"os_version",
		"os_arch",
		"os_num_cpus",
		"cpu_model",
		"cpu_cores",
		"cpu_frequency_mhz",
		"memory_total_gb",
		"memory_used_pct",
		"host_hostname",
		"host_uptime_hours",
		"on_battery",
	}
	for _, key := range expectedKeys {
		_, found := m[key]
		assert.True(t, found, "Expected key '%s' in map", key)
	}

	// Additionally, check that some values are not empty.
	assert.NotEmpty(t, m["id"], "ID should not be empty")
	assert.NotEmpty(t, m["created_at"], "CreatedAtMs should not be empty")
	assert.NotEmpty(t, m["os_name"], "OS Name should not be empty")
	assert.NotEmpty(t, m["host_hostname"], "Hostname should not be empty")
}

func TestToMapWithGPU(t *testing.T) {
	// Create a SystemInfo with GPU data
	sysInfo := &SystemInfo{}
	sysInfo.ID = uuid.New()
	sysInfo.CreatedAtMs = time.Now().UnixMilli()

	// Set GPU fields - now using array of GPUs
	sysInfo.GPUs = []struct {
		Model         string  `json:"model" gorm:"column:gpu_model"`
		Cores         uint    `json:"cores" gorm:"column:gpu_cores"`
		TotalMemoryMB float64 `json:"total_memory_mb" gorm:"column:gpu_total_memory_mb"`
		UsedMemoryMB  float64 `json:"used_memory_mb" gorm:"column:gpu_used_memory_mb"`
	}{
		{
			Model:         "Test GPU Model",
			Cores:         128,
			TotalMemoryMB: 8192.0,
			UsedMemoryMB:  4096.0,
		},
		{
			Model:         "Secondary GPU",
			Cores:         64,
			TotalMemoryMB: 4096.0,
			UsedMemoryMB:  2048.0,
		},
	}

	// Call ToMap
	m := sysInfo.ToMap()

	// Verify GPU fields are in the map
	assert.Equal(t, "2", m["gpu_count"])

	// Check individual GPU entries
	assert.Equal(t, "Test GPU Model", m["gpu_0_model"])
	assert.Equal(t, "128", m["gpu_0_cores"])
	assert.Equal(t, "Secondary GPU", m["gpu_1_model"])
	assert.Equal(t, "64", m["gpu_1_cores"])
}

func TestCollectSysInfoFields(t *testing.T) {
	// This test verifies that all fields in SystemInfo are populated
	sysInfo, err := CollectSysInfo()
	assert.NoError(t, err)

	// Test OS fields
	assert.NotEmpty(t, sysInfo.OS.Name)
	assert.NotEmpty(t, sysInfo.OS.Version)
	assert.NotEmpty(t, sysInfo.OS.Arch)

	// Test CPU fields
	assert.NotEmpty(t, sysInfo.CPU.Model)
	assert.True(t, sysInfo.CPU.NumCPUs > 0)
	assert.True(t, sysInfo.CPU.Cores >= 0)
	assert.True(t, sysInfo.CPU.Frequency >= 0)

	// Test Memory fields
	assert.True(t, sysInfo.Memory.Total > 0)
	assert.True(t, sysInfo.Memory.Available > 0)
	assert.True(t, sysInfo.Memory.Used > 0)
	assert.True(t, sysInfo.Memory.UsedPerc >= 0)

	// Test Host fields
	assert.NotEmpty(t, sysInfo.Host.Hostname)
	assert.True(t, sysInfo.Host.Uptime >= 0)

	// Test formatting methods
	formatted := sysInfo.FormatString()
	assert.Contains(t, formatted, sysInfo.OS.Name)
	assert.Contains(t, formatted, sysInfo.CPU.Model)
	assert.Contains(t, formatted, sysInfo.Host.Hostname)

	// Test ToMap with all fields
	m := sysInfo.ToMap()
	assert.NotEmpty(t, m["os_name"])
	assert.NotEmpty(t, m["cpu_model"])
	assert.NotEmpty(t, m["memory_total_gb"])
	assert.NotEmpty(t, m["host_hostname"])
}

func TestFormatStringWithGPU(t *testing.T) {
	// Create a SystemInfo with GPU data to test GPU formatting section
	sysInfo := &SystemInfo{}
	sysInfo.ID = uuid.New()
	sysInfo.CreatedAtMs = time.Now().UnixMilli()

	// OS and CPU info for complete string
	sysInfo.OS.Name = "Test OS"
	sysInfo.OS.Version = "1.0"
	sysInfo.OS.Arch = "x64"
	sysInfo.CPU.Model = "Test CPU"
	sysInfo.CPU.Cores = 8
	sysInfo.CPU.Frequency = 3200
	sysInfo.Memory.Total = 16 * 1024 * 1024 * 1024    // 16 GB
	sysInfo.Memory.Available = 8 * 1024 * 1024 * 1024 // 8 GB
	sysInfo.Memory.UsedPerc = 50.0
	sysInfo.Host.Hostname = "testhost"
	sysInfo.Host.Uptime = 3600 * 24 // 24 hours

	// Set GPU fields - now using array of GPUs
	sysInfo.GPUs = []struct {
		Model         string  `json:"model" gorm:"column:gpu_model"`
		Cores         uint    `json:"cores" gorm:"column:gpu_cores"`
		TotalMemoryMB float64 `json:"total_memory_mb" gorm:"column:gpu_total_memory_mb"`
		UsedMemoryMB  float64 `json:"used_memory_mb" gorm:"column:gpu_used_memory_mb"`
	}{
		{
			Model:         "Test GPU Model",
			Cores:         256,
			TotalMemoryMB: 8192.0,
			UsedMemoryMB:  4096.0,
		},
		{
			Model:         "Secondary GPU",
			Cores:         128,
			TotalMemoryMB: 4096.0,
			UsedMemoryMB:  2048.0,
		},
	}

	// Format and verify GPU information appears in the output
	formatted := sysInfo.FormatString()

	assert.Contains(t, formatted, "GPU Count: 2")
	assert.Contains(t, formatted, "GPU #1: Test GPU Model")
	assert.Contains(t, formatted, "Total Memory: 8192.00 MB")
	assert.Contains(t, formatted, "Used Memory: 4096.00 MB")
	assert.Contains(t, formatted, "GPU #2: Secondary GPU")
}

// This is a mock function to help test specific error scenarios
type mockSystemFunc struct {
	name     string
	original interface{}
	mock     interface{}
}

// TestCollectSysInfoErrors uses a helper to simulate errors from system calls
func TestCollectSysInfoErrors(t *testing.T) {
	// Create a helper to temporarily replace functions during testing
	// Note: This approach requires github.com/agiledragon/gomonkey or similar
	// and may not work on some platforms. An alternative would be using interfaces.
	t.Skip("This test requires gomonkey package to simulate errors. Implement if appropriate.")
	/*
		// Sample implementation using gomonkey:

		// Test host.Info() error
		patches := gomonkey.ApplyFunc(host.Info, func() (*host.InfoStat, error) {
			return nil, fmt.Errorf("simulated host.Info error")
		})
		defer patches.Reset()

		sysInfo, err := CollectSysInfo()
		assert.Error(t, err)
		assert.Nil(t, sysInfo)
		assert.Contains(t, err.Error(), "failed to get host info")

		// Test cpu.Info() error
		patches.Reset()
		patches = gomonkey.ApplyFunc(host.Info, func() (*host.InfoStat, error) {
			// Return valid host info
			return &host.InfoStat{
				Platform:        "test",
				PlatformVersion: "1.0",
				Uptime:          3600,
			}, nil
		})
		patches = patches.ApplyFunc(cpu.Info, func() ([]cpu.InfoStat, error) {
			return nil, fmt.Errorf("simulated cpu.Info error")
		})

		sysInfo, err = CollectSysInfo()
		assert.Error(t, err)
		assert.Nil(t, sysInfo)
		assert.Contains(t, err.Error(), "failed to get CPU info")

		// Similar tests for mem.VirtualMemory() and os.Hostname()
	*/
}

// Alternative approach that doesn't require mocking packages
func TestSystemInfoEdgeCases(t *testing.T) {
	// Test with zero values in important fields
	sysInfo := &SystemInfo{}
	sysInfo.ID = uuid.New()
	sysInfo.CreatedAtMs = time.Now().UnixMilli()

	// Initialize with empty GPUs array
	sysInfo.GPUs = []struct {
		Model         string  `json:"model" gorm:"column:gpu_model"`
		Cores         uint    `json:"cores" gorm:"column:gpu_cores"`
		TotalMemoryMB float64 `json:"total_memory_mb" gorm:"column:gpu_total_memory_mb"`
		UsedMemoryMB  float64 `json:"used_memory_mb" gorm:"column:gpu_used_memory_mb"`
	}{}

	// Convert to string with zero values
	formatted := sysInfo.FormatString()
	assert.NotEmpty(t, formatted, "FormatString should handle zero values gracefully")
	assert.Contains(t, formatted, "CPU: ")
	assert.NotContains(t, formatted, "CPU Cores: 0")
	assert.NotContains(t, formatted, "CPU Frequency: 0.00 MHz")

	// Test empty GPU array doesn't generate GPU section
	assert.NotContains(t, formatted, "GPU #1:")
	assert.NotContains(t, formatted, "GPU Count:")

	// Generate map with zero values
	m := sysInfo.ToMap()
	assert.NotEmpty(t, m, "ToMap should handle zero values")
	assert.Equal(t, "0", m["cpu_cores"])
	assert.Equal(t, "0.00", m["cpu_frequency_mhz"])
	assert.Equal(t, "0.0", m["memory_used_pct"])

	// GPU fields should not be present when GPUs array is empty
	_, hasGPUCount := m["gpu_count"]
	assert.False(t, hasGPUCount, "GPU count should not be in map when GPUs array is empty")
}

func TestSystemInfoWithNegativeValues(t *testing.T) {
	// Test handling of unexpected negative values
	sysInfo := &SystemInfo{}
	sysInfo.CPU.Cores = 0
	sysInfo.CPU.Frequency = -100
	sysInfo.Host.Uptime = 0

	// Add a GPU with negative values
	sysInfo.GPUs = []struct {
		Model         string  `json:"model" gorm:"column:gpu_model"`
		Cores         uint    `json:"cores" gorm:"column:gpu_cores"`
		TotalMemoryMB float64 `json:"total_memory_mb" gorm:"column:gpu_total_memory_mb"`
		UsedMemoryMB  float64 `json:"used_memory_mb" gorm:"column:gpu_used_memory_mb"`
	}{
		{
			Model:         "Negative Test GPU",
			Cores:         0,
			TotalMemoryMB: -1000.0,
			UsedMemoryMB:  -500.0,
		},
	}

	// This should not panic
	formatted := sysInfo.FormatString()
	assert.NotEmpty(t, formatted)
	assert.NotContains(t, formatted, "CPU Frequency: -100.00 MHz")
	assert.Contains(t, formatted, "Uptime: 0 hours")
	assert.Contains(t, formatted, "GPU #1: Negative Test GPU")
	assert.NotContains(t, formatted, "Cores: 0")
	assert.NotContains(t, formatted, "Total Memory: -1000.00 MB")

	// Map generation should also handle negative values
	m := sysInfo.ToMap()
	assert.Equal(t, "0", m["cpu_cores"])
	assert.Equal(t, "-100.00", m["cpu_frequency_mhz"])
	assert.Equal(t, "0", m["host_uptime_hours"])
	assert.Equal(t, "Negative Test GPU", m["gpu_0_model"])
	assert.Equal(t, "0", m["gpu_0_cores"])
	assert.Equal(t, "-1000.00", m["gpu_0_total_memory_mb"])
}

func TestSystemInfoGPUValidation(t *testing.T) {
	// Test a SystemInfo with invalid GPU memory values
	sysInfo := &SystemInfo{}
	sysInfo.ID = uuid.New()
	sysInfo.CreatedAtMs = time.Now().UnixMilli()

	// Set GPU fields with extreme values - using array of GPUs
	sysInfo.GPUs = []struct {
		Model         string  `json:"model" gorm:"column:gpu_model"`
		Cores         uint    `json:"cores" gorm:"column:gpu_cores"`
		TotalMemoryMB float64 `json:"total_memory_mb" gorm:"column:gpu_total_memory_mb"`
		UsedMemoryMB  float64 `json:"used_memory_mb" gorm:"column:gpu_used_memory_mb"`
	}{
		{
			Model:         "Extreme Test GPU",
			Cores:         10000,     // Unreasonably high
			TotalMemoryMB: 1000000.0, // 1TB in MB, very high
			UsedMemoryMB:  2000000.0, // Value higher than total (impossible)
		},
	}

	// Format should not panic with extreme values
	formatted := sysInfo.FormatString()
	assert.Contains(t, formatted, "GPU #1: Extreme Test GPU")
	assert.Contains(t, formatted, "Cores: 10000")
	assert.Contains(t, formatted, "Total Memory: 1000000.00 MB")
	assert.Contains(t, formatted, "Used Memory: 2000000.00 MB") // Should still display the value even if illogical

	// The ToMap method should handle these values as well
	m := sysInfo.ToMap()
	assert.Equal(t, "1", m["gpu_count"])
	assert.Equal(t, "1000000.00", m["gpu_0_total_memory_mb"])
	assert.Equal(t, "2000000.00", m["gpu_0_used_memory_mb"])
}

func TestFormatStringSpecialCharacters(t *testing.T) {
	// Test with special characters in fields
	sysInfo := &SystemInfo{}
	sysInfo.OS.Name = "Test OS 😊"
	sysInfo.OS.Version = "1.0 ß"
	sysInfo.OS.Arch = "x64 ñ"
	sysInfo.CPU.Model = "CPU with © symbol"
	sysInfo.GPUs = []struct {
		Model         string  `json:"model" gorm:"column:gpu_model"`
		Cores         uint    `json:"cores" gorm:"column:gpu_cores"`
		TotalMemoryMB float64 `json:"total_memory_mb" gorm:"column:gpu_total_memory_mb"`
		UsedMemoryMB  float64 `json:"used_memory_mb" gorm:"column:gpu_used_memory_mb"`
	}{
		{
			Model:         "GPU with € symbol",
			Cores:         0,
			TotalMemoryMB: 0.0,
			UsedMemoryMB:  0.0,
		},
	}
	sysInfo.Host.Hostname = "host-\"special\"-chars"

	// Should not panic or fail with special characters
	formatted := sysInfo.FormatString()
	assert.Contains(t, formatted, "OS: Test OS 😊")
	assert.Contains(t, formatted, "CPU with © symbol")

	// Special characters should be preserved in map
	m := sysInfo.ToMap()
	assert.Equal(t, "Test OS 😊", m["os_name"])
	assert.Equal(t, "CPU with © symbol", m["cpu_model"])
}

func TestSystemInfoEmptyWithEmptyHostname(t *testing.T) {
	// Test with empty hostname
	sysInfo := &SystemInfo{}
	sysInfo.ID = uuid.New()
	// Empty hostname
	sysInfo.Host.Hostname = ""

	// Initialize with empty GPUs array
	sysInfo.GPUs = []struct {
		Model         string  `json:"model" gorm:"column:gpu_model"`
		Cores         uint    `json:"cores" gorm:"column:gpu_cores"`
		TotalMemoryMB float64 `json:"total_memory_mb" gorm:"column:gpu_total_memory_mb"`
		UsedMemoryMB  float64 `json:"used_memory_mb" gorm:"column:gpu_used_memory_mb"`
	}{}

	formatted := sysInfo.FormatString()
	assert.Contains(t, formatted, "Hostname: ")

	m := sysInfo.ToMap()
	assert.Equal(t, "", m["host_hostname"])
}

func TestGPUInformationHandling(t *testing.T) {
	// Create a test system with partial GPU information to simulate
	// different error conditions in the GPU information gathering code

	// Scenario 1: Successfully got count but failed to get details
	sysInfo1 := &SystemInfo{}
	sysInfo1.GPUs = []struct {
		Model         string  `json:"model" gorm:"column:gpu_model"`
		Cores         uint    `json:"cores" gorm:"column:gpu_cores"`
		TotalMemoryMB float64 `json:"total_memory_mb" gorm:"column:gpu_total_memory_mb"`
		UsedMemoryMB  float64 `json:"used_memory_mb" gorm:"column:gpu_used_memory_mb"`
	}{
		{
			Model:         "",  // Failed to get name
			Cores:         0,   // Failed to get cores
			TotalMemoryMB: 0.0, // Failed to get memory info
			UsedMemoryMB:  0.0,
		},
		{
			Model:         "",  // Failed to get name
			Cores:         0,   // Failed to get cores
			TotalMemoryMB: 0.0, // Failed to get memory info
			UsedMemoryMB:  0.0,
		},
	}

	// Should still format count but show empty or zero values for other fields
	formatted1 := sysInfo1.FormatString()
	assert.Contains(t, formatted1, "GPU Count: 2")
	assert.Contains(t, formatted1, "GPU #1: ") // Empty model

	m1 := sysInfo1.ToMap()
	assert.Equal(t, "2", m1["gpu_count"])
	assert.Equal(t, "", m1["gpu_0_model"])
	assert.Equal(t, "0.00", m1["gpu_0_total_memory_mb"])

	// Scenario 2: Only device name is available, memory info fails
	sysInfo2 := &SystemInfo{}
	sysInfo2.GPUs = []struct {
		Model         string  `json:"model" gorm:"column:gpu_model"`
		Cores         uint    `json:"cores" gorm:"column:gpu_cores"`
		TotalMemoryMB float64 `json:"total_memory_mb" gorm:"column:gpu_total_memory_mb"`
		UsedMemoryMB  float64 `json:"used_memory_mb" gorm:"column:gpu_used_memory_mb"`
	}{
		{
			Model:         "Test GPU", // Success
			Cores:         uint(0),    // Failed
			TotalMemoryMB: 0.0,        // Failed
			UsedMemoryMB:  0.0,
		},
	}

	formatted2 := sysInfo2.FormatString()
	assert.Contains(t, formatted2, "GPU #1: Test GPU")
	assert.NotContains(t, formatted2, "Total Memory: 0.00 MB")

	m2 := sysInfo2.ToMap()
	assert.Equal(t, "Test GPU", m2["gpu_0_model"])
	assert.Equal(t, "0.00", m2["gpu_0_total_memory_mb"])

	// Scenario 3: Only memory info is available, name fails
	sysInfo3 := &SystemInfo{}
	sysInfo3.GPUs = []struct {
		Model         string  `json:"model" gorm:"column:gpu_model"`
		Cores         uint    `json:"cores" gorm:"column:gpu_cores"`
		TotalMemoryMB float64 `json:"total_memory_mb" gorm:"column:gpu_total_memory_mb"`
		UsedMemoryMB  float64 `json:"used_memory_mb" gorm:"column:gpu_used_memory_mb"`
	}{
		{
			Model:         "",     // Failed
			Cores:         0,      // Failed
			TotalMemoryMB: 8192.0, // Success
			UsedMemoryMB:  4096.0, // Success
		},
	}

	formatted3 := sysInfo3.FormatString()
	assert.Contains(t, formatted3, "GPU #1: ") // Empty model
	assert.Contains(t, formatted3, "Total Memory: 8192.00 MB")
	assert.Contains(t, formatted3, "Used Memory: 4096.00 MB")

	m3 := sysInfo3.ToMap()
	assert.Equal(t, "", m3["gpu_0_model"])
	assert.Equal(t, "8192.00", m3["gpu_0_total_memory_mb"])

	// Scenario 4: Both zero and negative GPU memory values (boundary testing)
	sysInfo4 := &SystemInfo{}
	sysInfo4.GPUs = []struct {
		Model         string  `json:"model" gorm:"column:gpu_model"`
		Cores         uint    `json:"cores" gorm:"column:gpu_cores"`
		TotalMemoryMB float64 `json:"total_memory_mb" gorm:"column:gpu_total_memory_mb"`
		UsedMemoryMB  float64 `json:"used_memory_mb" gorm:"column:gpu_used_memory_mb"`
	}{
		{
			Model:         "Test GPU", // Success
			Cores:         0,          // Failed
			TotalMemoryMB: -1.0,       // Invalid/error scenario
			UsedMemoryMB:  0.0,
		},
	}

	formatted4 := sysInfo4.FormatString()
	assert.NotContains(t, formatted4, "Total Memory: -1.00 MB") // Should not display if negative

	m4 := sysInfo4.ToMap()
	assert.Equal(t, "-1.00", m4["gpu_0_total_memory_mb"])
}
