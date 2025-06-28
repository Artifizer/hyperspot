//go:build windows

package utils

import (
	"github.com/StackExchange/wmi"
)

type videoController struct {
	Name           string
	DriverVersion  string
	AdapterRAM     uint64
	VideoProcessor string
	PNPDeviceID    string
	Status         string
	DeviceID       string
}

func getWindowsGPUInfo(info *SystemInfo) error {
	var gpus []videoController
	query := wmi.CreateQuery(&gpus, "")
	err := wmi.Query(query, &gpus)
	if err != nil {
		return err
	}

	for i, gpu := range gpus {
		info.GPUs[i].Model = gpu.Name
		info.GPUs[i].TotalMemoryMB = float64(gpu.AdapterRAM) / (1024 * 1024)
		info.GPUs[i].Cores = 0
		info.GPUs[i].UsedMemoryMB = 0
	}

	return nil
}

func getNVMLGPUInfo(_ *SystemInfo) error {
	return nil
}
