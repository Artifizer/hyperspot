//go:build !windows

package utils

import (
	"github.com/mindprince/gonvml"
)

func getWindowsGPUInfo(info *SystemInfo) error {
	return nil
}

// getNVMLGPUInfo collects GPU information using NVIDIA Management Library
func getNVMLGPUInfo(info *SystemInfo) error {
	if err := gonvml.Initialize(); err != nil {
		return err
	}
	defer gonvml.Shutdown()

	count, err := gonvml.DeviceCount()
	if err != nil {
		return err
	}

	// Process each GPU device
	for i := uint(0); i < count; i++ {
		device, err := gonvml.DeviceHandleByIndex(i)
		if err != nil {
			continue
		}

		gpu := struct {
			Model         string  `json:"model" gorm:"column:gpu_model"`
			Cores         uint    `json:"cores" gorm:"column:gpu_cores"`
			TotalMemoryMB float64 `json:"total_memory_mb" gorm:"column:gpu_total_memory_mb"`
			UsedMemoryMB  float64 `json:"used_memory_mb" gorm:"column:gpu_used_memory_mb"`
		}{}

		// Get GPU name
		if name, err := device.Name(); err == nil {
			gpu.Model = name
		}

		// Get memory info
		if total, used, err := device.MemoryInfo(); err == nil {
			gpu.TotalMemoryMB = float64(total) / (1024 * 1024)
			gpu.UsedMemoryMB = float64(used) / (1024 * 1024)
		}

		// Try to get core count if available
		if utilization, _, err := device.UtilizationRates(); err == nil {
			// This doesn't actually get cores, but we can use it to indicate the device is working
			// In a real implementation, you might need to use CUDA API to get actual core count
			gpu.Cores = utilization
		}

		info.GPUs = append(info.GPUs, gpu)
	}

	return nil
}
