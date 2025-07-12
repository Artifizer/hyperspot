package utils

import (
	"fmt"
	"math"
	"os"
	"os/exec"
	"regexp"
	"runtime"
	"strconv"
	"strings"
	"time"

	"github.com/distatus/battery"
	"github.com/google/uuid"
	"github.com/hypernetix/hyperspot/libs/logging"
	"github.com/shirou/gopsutil/v4/cpu"
	"github.com/shirou/gopsutil/v4/host"
	"github.com/shirou/gopsutil/v4/mem"
)

// SystemInfo represents system information
type SystemInfo struct {
	ID          uuid.UUID `json:"id" gorm:"primaryKey"`
	CreatedAtMs int64     `json:"created_at" gorm:"index;type:bigint" doc:"unix timestamp in milliseconds"`

	// OS information
	OS struct {
		Name    string `json:"name" gorm:"column:os_name"`
		Version string `json:"version" gorm:"column:os_version"`
		Arch    string `json:"arch" gorm:"column:os_arch"`
	} `json:"os" gorm:"embedded"`

	// CPU information
	CPU struct {
		Model     string  `json:"model" gorm:"column:cpu_model"`
		NumCPUs   uint    `json:"num_cpus" gorm:"column:cpu_num_cpus"`
		Cores     uint    `json:"cores" gorm:"column:cpu_cores"`
		Frequency float64 `json:"frequency_mhz" gorm:"column:cpu_frequency_mhz"`
	} `json:"cpu" gorm:"embedded"`

	// GPU information
	GPUs []struct {
		Model         string  `json:"model" gorm:"column:gpu_model"`
		Cores         uint    `json:"cores" gorm:"column:gpu_cores"`
		TotalMemoryMB float64 `json:"total_memory_mb" gorm:"column:gpu_total_memory_mb"`
		UsedMemoryMB  float64 `json:"used_memory_mb" gorm:"column:gpu_used_memory_mb"`
	} `json:"gpus" gorm:"serializer:json"`

	// Memory information
	Memory struct {
		Total     uint64  `json:"total_bytes" gorm:"column:memory_total_bytes"`
		Available uint64  `json:"available_bytes" gorm:"column:memory_available_bytes"`
		Used      uint64  `json:"used_bytes" gorm:"column:memory_used_bytes"`
		UsedPerc  float64 `json:"used_percent" gorm:"column:memory_used_percent"`
	} `json:"memory" gorm:"embedded"`

	// Host information
	Host struct {
		Hostname string `json:"hostname" gorm:"column:host_hostname"`
		Uptime   uint64 `json:"uptime_sec" gorm:"column:host_uptime_sec"`
	} `json:"host" gorm:"embedded"`

	// Battery information
	Battery struct {
		OnBattery  bool `json:"on_battery" gorm:"column:on_battery"`
		Percentage uint `json:"percentage" gorm:"column:battery_percentage"`
	} `json:"battery" gorm:"embedded"`
}

func getMacCPUFreq() (int64, error) {
	// Try direct sysctl command first
	cmd := exec.Command("sysctl", "hw.cpufrequency")
	output, err := cmd.Output()
	if err == nil && len(output) > 0 {
		// Parse the output to extract frequency
		outputStr := string(output)
		parts := strings.Split(outputStr, ":")
		if len(parts) == 2 {
			freqStr := strings.TrimSpace(parts[1])
			freq, err := strconv.ParseInt(freqStr, 10, 64)
			if err == nil {
				return freq / 1000000, nil
			}
		}
	}

	// If first method fails, try with arch -x86_64 prefix
	cmd = exec.Command("arch", "-x86_64", "/bin/bash", "-c", "sysctl hw.cpufrequency")
	output, err = cmd.Output()
	if err != nil {
		return 0, fmt.Errorf("failed to get CPU frequency: %w", err)
	}

	outputStr := string(output)
	parts := strings.Split(outputStr, ":")
	if len(parts) != 2 {
		return 0, fmt.Errorf("unexpected output format from sysctl: %s", outputStr)
	}

	freqStr := strings.TrimSpace(parts[1])
	freq, err := strconv.ParseInt(freqStr, 10, 64)
	if err != nil {
		return 0, fmt.Errorf("failed to parse CPU frequency: %w", err)
	}

	return freq / 1000000, nil
}

func getMacGPUInfo(info *SystemInfo) error {
	// Get GPU information using system_profiler command
	cmd := exec.Command("system_profiler", "SPDisplaysDataType")
	output, err := cmd.Output()
	if err != nil {
		return fmt.Errorf("failed to get GPU info: %v", err)
	}

	// Parse the output to extract GPU model and memory
	outputStr := string(output)

	// Find all GPU entries
	modelRegex := regexp.MustCompile(`Chipset Model: (.+)`)
	modelMatches := modelRegex.FindAllStringSubmatch(outputStr, -1)

	// VRAM regex for memory information
	vramRegex := regexp.MustCompile(`VRAM \(.*\): (\d+) MB`)
	vramMatches := vramRegex.FindAllStringSubmatch(outputStr, -1)

	// Create GPU entries for each found GPU
	for i, match := range modelMatches {
		if len(match) > 1 {
			gpu := struct {
				Model         string  `json:"model" gorm:"column:gpu_model"`
				Cores         uint    `json:"cores" gorm:"column:gpu_cores"`
				TotalMemoryMB float64 `json:"total_memory_mb" gorm:"column:gpu_total_memory_mb"`
				UsedMemoryMB  float64 `json:"used_memory_mb" gorm:"column:gpu_used_memory_mb"`
			}{
				Model: strings.TrimSpace(match[1]),
				Cores: 0, // Mac doesn't easily expose core count
			}

			// Try to match VRAM info if available
			if i < len(vramMatches) && len(vramMatches[i]) > 1 {
				if vram, err := strconv.ParseFloat(vramMatches[i][1], 64); err == nil {
					gpu.TotalMemoryMB = vram
					// Used memory is not available through system_profiler
					gpu.UsedMemoryMB = 0
				}
			}

			info.GPUs = append(info.GPUs, gpu)
		}
	}

	return nil
}

// isMacArm checks if the system is a Mac with ARM architecture
func isMacArm() bool {
	return runtime.GOOS == "darwin" && (runtime.GOARCH == "arm64" || strings.Contains(runtime.GOARCH, "arm"))
}

func isWindows() bool {
	return runtime.GOOS == "windows"
}

func isUnix() bool {
	return runtime.GOOS == "linux" || runtime.GOOS == "darwin"
}

// Collect gathers system information
func CollectSysInfo() (*SystemInfo, error) {
	info := &SystemInfo{}

	// OS information
	hostInfo, err := host.Info()
	if err != nil {
		return nil, fmt.Errorf("failed to get host info: %w", err)
	}

	numcpu := runtime.NumCPU()

	info.OS.Name = hostInfo.Platform
	info.OS.Version = hostInfo.PlatformVersion
	info.OS.Arch = runtime.GOARCH
	if numcpu > 0 {
		info.CPU.NumCPUs = uint(numcpu)
	}

	// CPU information
	cpuInfo, err := cpu.Info()
	if err != nil {
		return nil, fmt.Errorf("failed to get CPU info: %w", err)
	}

	if len(cpuInfo) > 0 {
		info.CPU.Model = cpuInfo[0].ModelName
		if cpuInfo[0].Cores > 0 {
			// Safely convert int32 to uint, clamping negative values to 0
			cores := cpuInfo[0].Cores
			if cores > 0 {
				info.CPU.Cores = uint(uint32(cores))
			}
		}
		info.CPU.Frequency = float64(cpuInfo[0].Mhz)
	}

	if info.CPU.Model == "" {
		// Fallback to sysctl on macOS
		info.CPU.Model = "Unknown " + info.OS.Arch + " CPU"
	}

	if isMacArm() && info.CPU.Frequency < 10 {
		// I see buggy 4Mhz value on Apple Silicon
		freqMhz, err := getMacCPUFreq()
		if err == nil {
			info.CPU.Frequency = float64(freqMhz)
		}
	}

	// GPU information
	if isMacArm() {
		if err := getMacGPUInfo(info); err != nil {
			logging.Warn("Failed to get Mac GPU info: %v", err)
		}
	} else if isWindows() {
		if err := getWindowsGPUInfo(info); err != nil {
			logging.Warn("Failed to get Windows GPU info: %v", err)
		}
	} else if isUnix() {
		if err := getNVMLGPUInfo(info); err != nil {
			logging.Warn("Failed to get NVML GPU info: %v", err)
		}
	} else {
		// Not supported for other platforms
		logging.Warn("GPU information is not supported on this platform")
	}

	// Memory information
	memInfo, err := mem.VirtualMemory()
	if err != nil {
		return nil, fmt.Errorf("failed to get memory info: %w", err)
	}

	info.Memory.Total = memInfo.Total
	info.Memory.Available = memInfo.Available
	info.Memory.Used = memInfo.Used
	info.Memory.UsedPerc = memInfo.UsedPercent

	// Host information
	hostname, err := os.Hostname()
	if err != nil {
		return nil, fmt.Errorf("failed to get hostname: %w", err)
	}

	info.Host.Hostname = hostname
	info.Host.Uptime = hostInfo.Uptime

	// Battery information
	batteries, err := battery.GetAll()
	if err == nil && len(batteries) > 0 {
		info.Battery.OnBattery = batteries[0].State.Raw == battery.Discharging
		// Convert current battery capacity to percentage (0-100 range)
		if batteries[0].Full > 0 {
			info.Battery.Percentage = uint(math.Round((float64(batteries[0].Current) / float64(batteries[0].Full)) * 100))
		} else {
			info.Battery.Percentage = 0
		}
	}

	jsonString, err := StructToJSONString(info)
	if err != nil {
		return nil, fmt.Errorf("failed to get JSON string: %w", err)
	}

	info.ID = uuid.NewSHA1(uuid.NameSpaceOID, []byte(jsonString))
	info.CreatedAtMs = time.Now().UnixMilli()

	return info, nil
}

// FormatString returns a human-readable string representation of system information
func (si *SystemInfo) FormatString() string {
	var b strings.Builder

	b.WriteString("System Information:\n")
	b.WriteString(fmt.Sprintf("  OS: %s %s (%s)\n", si.OS.Name, si.OS.Version, si.OS.Arch))
	b.WriteString(fmt.Sprintf("  CPU: %s\n", si.CPU.Model))
	if si.CPU.Cores > 0 {
		b.WriteString(fmt.Sprintf("  CPU Cores: %d\n", si.CPU.Cores))
	}
	if si.CPU.Frequency > 0 {
		b.WriteString(fmt.Sprintf("  CPU Frequency: %.2f MHz\n", si.CPU.Frequency))
	}

	// GPU information
	if len(si.GPUs) > 0 {
		b.WriteString(fmt.Sprintf("  GPU Count: %d\n", len(si.GPUs)))
		for i, gpu := range si.GPUs {
			b.WriteString(fmt.Sprintf("  GPU #%d: %s\n", i+1, gpu.Model))
			if gpu.Cores > 0 {
				b.WriteString(fmt.Sprintf("    Cores: %d\n", gpu.Cores))
			}
			if gpu.TotalMemoryMB > 0 {
				b.WriteString(fmt.Sprintf("    Total Memory: %.2f MB\n", gpu.TotalMemoryMB))
			}
			if gpu.UsedMemoryMB > 0 {
				b.WriteString(fmt.Sprintf("    Used Memory: %.2f MB (%.1f%%)\n",
					gpu.UsedMemoryMB,
					(gpu.UsedMemoryMB/gpu.TotalMemoryMB)*100))
			}
		}
	}

	b.WriteString(fmt.Sprintf("  Memory Total: %.2f GB\n", float64(si.Memory.Total)/(1024*1024*1024)))
	b.WriteString(fmt.Sprintf("  Memory Available: %.2f GB\n", float64(si.Memory.Available)/(1024*1024*1024)))
	b.WriteString(fmt.Sprintf("  Memory Used: %.1f%%\n", si.Memory.UsedPerc))
	b.WriteString(fmt.Sprintf("  Hostname: %s\n", si.Host.Hostname))
	b.WriteString(fmt.Sprintf("  Uptime: %d hours\n", si.Host.Uptime/3600))
	b.WriteString(fmt.Sprintf("  On Battery: %v\n", si.Battery.OnBattery))

	return b.String()
}

// ToMap converts system information to a map[string]string
func (si *SystemInfo) ToMap() map[string]string {
	m := map[string]string{
		"id":                si.ID.String(),
		"created_at":        fmt.Sprintf("%d", si.CreatedAtMs),
		"os_name":           si.OS.Name,
		"os_version":        si.OS.Version,
		"os_arch":           si.OS.Arch,
		"os_num_cpus":       fmt.Sprintf("%d", si.CPU.NumCPUs),
		"cpu_model":         si.CPU.Model,
		"cpu_cores":         fmt.Sprintf("%d", si.CPU.Cores),
		"cpu_frequency_mhz": fmt.Sprintf("%.2f", si.CPU.Frequency),
		"memory_total_gb":   fmt.Sprintf("%.2f", float64(si.Memory.Total)/(1024*1024*1024)),
		"memory_used_pct":   fmt.Sprintf("%.1f", si.Memory.UsedPerc),
		"host_hostname":     si.Host.Hostname,
		"host_uptime_hours": fmt.Sprintf("%d", si.Host.Uptime/3600),
		"on_battery":        fmt.Sprintf("%v", si.Battery.OnBattery),
	}

	// Add GPU information
	if len(si.GPUs) > 0 {
		m["gpu_count"] = fmt.Sprintf("%d", len(si.GPUs))

		// Add information for each GPU
		for i, gpu := range si.GPUs {
			prefix := fmt.Sprintf("gpu_%d_", i)
			m[prefix+"model"] = gpu.Model
			m[prefix+"cores"] = fmt.Sprintf("%d", gpu.Cores)
			m[prefix+"total_memory_mb"] = fmt.Sprintf("%.2f", gpu.TotalMemoryMB)
			m[prefix+"used_memory_mb"] = fmt.Sprintf("%.2f", gpu.UsedMemoryMB)
		}
	}

	return m
}
