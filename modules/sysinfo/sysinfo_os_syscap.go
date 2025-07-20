package sysinfo

import (
	"fmt"
	"runtime"
	"strings"

	"github.com/hypernetix/hyperspot/libs/utils"
	"github.com/hypernetix/hyperspot/modules/syscap"
)

// OS capability names (machine-friendly)
var (
	CapabilityOsMacOS   = syscap.SysCap{Category: syscap.CategoryOS, Name: "macos", DisplayName: "macOS", CacheTTLMsec: 60, Detector: detectMacOS}
	CapabilityOsWindows = syscap.SysCap{Category: syscap.CategoryOS, Name: "windows", DisplayName: "Windows", CacheTTLMsec: 60, Detector: detectWindows}
	CapabilityOsLinux   = syscap.SysCap{Category: syscap.CategoryOS, Name: "linux", DisplayName: "Linux", CacheTTLMsec: 60, Detector: detectLinux}
)

// initOSSysCaps registers all OS capability detectors
func initOSSysCaps() {
	syscap.RegisterSysCap(&CapabilityOsMacOS)
	syscap.RegisterSysCap(&CapabilityOsWindows)
	syscap.RegisterSysCap(&CapabilityOsLinux)
}

// detectMacOS detects if the system is macOS
// Utilizes sysinfo for detailed macOS information
func detectMacOS(c *syscap.SysCap) error {
	sysInfo, err := utils.CollectSysInfo()
	if err != nil {
		return err
	}

	// Check multiple indicators for macOS
	isMacOS := runtime.GOOS == "darwin" ||
		strings.Contains(strings.ToLower(sysInfo.OS.Name), "mac") ||
		strings.Contains(strings.ToLower(sysInfo.OS.Name), "darwin")

	c.Present = isMacOS

	if isMacOS {
		c.SetVersion(sysInfo.OS.Version)

		// Create detailed macOS information
		details := fmt.Sprintf("Platform: %s, Version: %s, Architecture: %s",
			sysInfo.OS.Name, sysInfo.OS.Version, sysInfo.OS.Arch)

		// Add uptime information
		if sysInfo.Host.Uptime > 0 {
			uptimeHours := sysInfo.Host.Uptime / 3600
			uptimeDays := uptimeHours / 24
			if uptimeDays > 0 {
				details += fmt.Sprintf(", Uptime: %d days %d hours", uptimeDays, uptimeHours%24)
			} else {
				details += fmt.Sprintf(", Uptime: %d hours", uptimeHours)
			}
		}

		// Add hostname
		if sysInfo.Host.Hostname != "" {
			details += fmt.Sprintf(", Hostname: %s", sysInfo.Host.Hostname)
		}

		// Add CPU information for macOS
		if sysInfo.CPU.Model != "" {
			details += fmt.Sprintf(", CPU: %s", sysInfo.CPU.Model)
		}

		// Detect if it's Apple Silicon
		if runtime.GOARCH == "arm64" {
			details += " [Apple Silicon]"
		} else if runtime.GOARCH == "amd64" {
			details += " [Intel]"
		}

		c.SetDetails(details)
	}

	return nil
}

// detectWindows detects if the system is Windows
// Utilizes sysinfo for detailed Windows information
func detectWindows(c *syscap.SysCap) error {
	sysInfo, err := utils.CollectSysInfo()
	if err != nil {
		return err
	}

	// Check multiple indicators for Windows
	isWindows := runtime.GOOS == "windows" ||
		strings.Contains(strings.ToLower(sysInfo.OS.Name), "windows")

	c.Present = isWindows

	if isWindows {
		c.SetVersion(sysInfo.OS.Version)

		// Create detailed Windows information
		details := fmt.Sprintf("Platform: %s, Version: %s, Architecture: %s",
			sysInfo.OS.Name, sysInfo.OS.Version, sysInfo.OS.Arch)

		// Add uptime information
		if sysInfo.Host.Uptime > 0 {
			uptimeHours := sysInfo.Host.Uptime / 3600
			uptimeDays := uptimeHours / 24
			if uptimeDays > 0 {
				details += fmt.Sprintf(", Uptime: %d days %d hours", uptimeDays, uptimeHours%24)
			} else {
				details += fmt.Sprintf(", Uptime: %d hours", uptimeHours)
			}
		}

		// Add hostname
		if sysInfo.Host.Hostname != "" {
			details += fmt.Sprintf(", Hostname: %s", sysInfo.Host.Hostname)
		}

		// Add CPU information for Windows
		if sysInfo.CPU.Model != "" {
			details += fmt.Sprintf(", CPU: %s", sysInfo.CPU.Model)
		}

		// Note Windows-specific GPU detection capability
		if len(sysInfo.GPUs) > 0 {
			details += fmt.Sprintf(", GPUs detected: %d [via WMI]", len(sysInfo.GPUs))
		}

		c.SetDetails(details)
	}

	return nil
}

// detectLinux detects if the system is Linux
// Utilizes sysinfo for detailed Linux information
func detectLinux(c *syscap.SysCap) error {
	sysInfo, err := utils.CollectSysInfo()
	if err != nil {
		return err
	}

	// Check multiple indicators for Linux
	isLinux := runtime.GOOS == "linux" ||
		strings.Contains(strings.ToLower(sysInfo.OS.Name), "linux") ||
		strings.Contains(strings.ToLower(sysInfo.OS.Name), "ubuntu") ||
		strings.Contains(strings.ToLower(sysInfo.OS.Name), "debian") ||
		strings.Contains(strings.ToLower(sysInfo.OS.Name), "fedora") ||
		strings.Contains(strings.ToLower(sysInfo.OS.Name), "centos") ||
		strings.Contains(strings.ToLower(sysInfo.OS.Name), "rhel")

	c.Present = isLinux

	if isLinux {
		c.SetVersion(sysInfo.OS.Version)

		// Create detailed Linux information
		details := fmt.Sprintf("Platform: %s, Version: %s, Architecture: %s",
			sysInfo.OS.Name, sysInfo.OS.Version, sysInfo.OS.Arch)

		// Add uptime information
		if sysInfo.Host.Uptime > 0 {
			uptimeHours := sysInfo.Host.Uptime / 3600
			uptimeDays := uptimeHours / 24
			if uptimeDays > 0 {
				details += fmt.Sprintf(", Uptime: %d days %d hours", uptimeDays, uptimeHours%24)
			} else {
				details += fmt.Sprintf(", Uptime: %d hours", uptimeHours)
			}
		}

		// Add hostname
		if sysInfo.Host.Hostname != "" {
			details += fmt.Sprintf(", Hostname: %s", sysInfo.Host.Hostname)
		}

		// Add CPU information for Linux
		if sysInfo.CPU.Model != "" {
			details += fmt.Sprintf(", CPU: %s", sysInfo.CPU.Model)
		}

		// Note Linux-specific GPU detection capability
		if len(sysInfo.GPUs) > 0 {
			details += fmt.Sprintf(", NVIDIA GPUs detected: %d [via NVML]", len(sysInfo.GPUs))
		}

		// Add distribution detection hint
		distroName := strings.ToLower(sysInfo.OS.Name)
		if strings.Contains(distroName, "ubuntu") {
			details += " [Ubuntu]"
		} else if strings.Contains(distroName, "debian") {
			details += " [Debian]"
		} else if strings.Contains(distroName, "fedora") {
			details += " [Fedora]"
		} else if strings.Contains(distroName, "centos") {
			details += " [CentOS]"
		} else if strings.Contains(distroName, "rhel") {
			details += " [RHEL]"
		}

		c.SetDetails(details)
	}

	return nil
}
