package sysinfo

import (
	"fmt"
	"runtime"

	"github.com/hypernetix/hyperspot/libs/utils"
	"github.com/hypernetix/hyperspot/modules/syscap"
)

var SysCapHwNameGPU = syscap.SysCapName("gpu")
var SysCapHwNameARM64 = syscap.SysCapName("arm64")
var SysCapHwNameX86_64 = syscap.SysCapName("x86_64")
var SysCapHwNameRAM = syscap.SysCapName("ram")
var SysCapHwNameBattery = syscap.SysCapName("battery")

// Hardware capability names (machine-friendly)
var (
	CapabilityHwGPU     = syscap.SysCap{Category: syscap.CategoryHardware, Name: SysCapHwNameGPU, DisplayName: "GPU", CacheTTLMsec: 600, Detector: detectGPU}
	CapabilityHwARM64   = syscap.SysCap{Category: syscap.CategoryHardware, Name: SysCapHwNameARM64, DisplayName: "ARM64", CacheTTLMsec: 600, Detector: detectARM64}
	CapabilityHwX86_64  = syscap.SysCap{Category: syscap.CategoryHardware, Name: SysCapHwNameX86_64, DisplayName: "x86_64", CacheTTLMsec: 600, Detector: detectX86_64}
	CapabilityHwRAM     = syscap.SysCap{Category: syscap.CategoryHardware, Name: SysCapHwNameRAM, DisplayName: "RAM", CacheTTLMsec: 10, Detector: detectRAM}
	CapabilityHwBattery = syscap.SysCap{Category: syscap.CategoryHardware, Name: SysCapHwNameBattery, DisplayName: "Battery", CacheTTLMsec: 10, Detector: detectBattery}
)

// initHardwareSysCaps registers all hardware capability detectors
func initHardwareSysCaps() {
	syscap.RegisterSysCap(&CapabilityHwGPU)
	syscap.RegisterSysCap(&CapabilityHwARM64)
	syscap.RegisterSysCap(&CapabilityHwX86_64)
	syscap.RegisterSysCap(&CapabilityHwRAM)
	syscap.RegisterSysCap(&CapabilityHwBattery)
}

// detectGPU detects if GPU is present on the machine
// Utilizes platform-specific GPU detection from sysinfo
func detectGPU(c *syscap.SysCap) error {
	sysInfo, err := utils.CollectSysInfo()
	if err != nil {
		return err
	}

	c.Present = len(sysInfo.GPUs) > 0

	if len(sysInfo.GPUs) > 0 {
		// Set amount as number of GPUs
		c.SetAmount(float64(len(sysInfo.GPUs)), syscap.DimensionCount)

		// Set details with GPU models and platform-specific info
		details := ""
		totalMemoryMB := float64(0)

		for i, gpu := range sysInfo.GPUs {
			if i > 0 {
				details += ", "
			}
			details += gpu.Model

			if gpu.TotalMemoryMB > 0 {
				details += fmt.Sprintf(" (%.0f MB", gpu.TotalMemoryMB)
				totalMemoryMB += gpu.TotalMemoryMB

				if gpu.UsedMemoryMB > 0 {
					usagePercent := (gpu.UsedMemoryMB / gpu.TotalMemoryMB) * 100
					details += fmt.Sprintf(", %.1f%% used", usagePercent)
				}
				details += ")"
			}

			if gpu.Cores > 0 {
				details += fmt.Sprintf(", %d cores", gpu.Cores)
			}
		}

		// Add platform-specific detection method
		switch runtime.GOOS {
		case "darwin":
			if runtime.GOARCH == "arm64" {
				details += " [detected via macOS system_profiler]"
			}
		case "windows":
			details += " [detected via Windows WMI]"
		case "linux":
			details += " [detected via NVIDIA NVML]"
		}

		if totalMemoryMB > 0 {
			details = fmt.Sprintf("Total GPU Memory: %.2f GB, GPUs: %s", totalMemoryMB/1024, details)
		}

		c.SetDetails(details)
	}

	return nil
}

// detectARM64 detects if current architecture is ARM64
func detectARM64(capability *syscap.SysCap) error {
	isARM64 := runtime.GOARCH == "arm64"
	capability.Present = isARM64

	if isARM64 {
		// Get additional info from system info
		sysInfo, err := utils.CollectSysInfo()
		if err == nil {
			details := fmt.Sprintf("ARM64 architecture detected, CPU: %s", sysInfo.CPU.Model)
			if sysInfo.CPU.Frequency > 0 {
				details += fmt.Sprintf(", Frequency: %.0f MHz", sysInfo.CPU.Frequency)
			}
			capability.SetDetails(details)
		} else {
			capability.SetDetails("ARM64 architecture detected")
		}
	}

	return nil
}

// detectX86_64 detects if current architecture is x86_64
func detectX86_64(capability *syscap.SysCap) error {
	isX86_64 := runtime.GOARCH == "amd64"
	capability.Present = isX86_64

	if isX86_64 {
		// Get additional info from system info
		sysInfo, err := utils.CollectSysInfo()
		if err == nil {
			details := fmt.Sprintf("x86_64 architecture detected, CPU: %s", sysInfo.CPU.Model)
			if sysInfo.CPU.Cores > 0 {
				details += fmt.Sprintf(", Cores: %d", sysInfo.CPU.Cores)
			}
			if sysInfo.CPU.Frequency > 0 {
				details += fmt.Sprintf(", Frequency: %.0f MHz", sysInfo.CPU.Frequency)
			}
			capability.SetDetails(details)
		} else {
			capability.SetDetails("x86_64 architecture detected")
		}
	}

	return nil
}

// detectRAM detects RAM information using sysinfo memory detection
func detectRAM(c *syscap.SysCap) error {
	sysInfo, err := utils.CollectSysInfo()
	if err != nil {
		return err
	}

	c.Present = true

	// Set amount in GB
	totalGB := float64(sysInfo.Memory.Total) / (1024 * 1024 * 1024)
	c.SetAmount(totalGB, syscap.DimensionGB)

	// Set detailed memory information
	availableGB := float64(sysInfo.Memory.Available) / (1024 * 1024 * 1024)
	usedGB := float64(sysInfo.Memory.Used) / (1024 * 1024 * 1024)
	usedPerc := sysInfo.Memory.UsedPerc

	details := fmt.Sprintf("Total: %.2f GB, Used: %.2f GB (%.1f%%), Available: %.2f GB",
		totalGB, usedGB, usedPerc, availableGB)

	// Add platform-specific memory information
	if sysInfo.Host.Hostname != "" {
		details += fmt.Sprintf(", Host: %s", sysInfo.Host.Hostname)
	}

	c.SetDetails(details)

	return nil
}

// detectBattery detects battery information using cross-platform battery detection
func detectBattery(c *syscap.SysCap) error {
	sysInfo, err := utils.CollectSysInfo()
	if err != nil {
		return err
	}

	// Battery is present if we have battery information
	// The sysinfo package uses cross-platform battery detection
	hasBattery := sysInfo.Battery.Percentage > 0 || sysInfo.Battery.OnBattery
	c.Present = hasBattery

	if hasBattery {
		// Set amount as percentage
		c.SetAmount(float64(sysInfo.Battery.Percentage), syscap.DimensionPercent)

		// Set detailed battery status
		status := "charging"
		if sysInfo.Battery.OnBattery {
			status = "discharging (on battery power)"
		}

		details := fmt.Sprintf("Status: %s, Level: %d%%", status, sysInfo.Battery.Percentage)

		// Add platform information
		details += fmt.Sprintf(" [detected on %s %s]", sysInfo.OS.Name, sysInfo.OS.Arch)

		c.SetDetails(details)
	} else {
		// No battery detected - likely a desktop system
		details := fmt.Sprintf("No battery detected [%s %s system]", sysInfo.OS.Name, sysInfo.OS.Arch)
		c.SetDetails(details)
	}

	return nil
}
