package main

import (
	"encoding/json"
	"fmt"
	"os"

	"github.com/google/uuid"
	"github.com/olekukonko/tablewriter"
	"github.com/urfave/cli/v2"
)

func getSystemInfoCommands() *cli.Command {
	return &cli.Command{
		Name:  "sysinfo",
		Usage: "System information commands",
		Subcommands: []*cli.Command{
			{
				Name:  "get",
				Usage: "Get system information",
				Flags: []cli.Flag{
					&cli.StringFlag{
						Name:  "id",
						Usage: "Get historical system info by ID (UUID)",
					},
				},
				Action: func(c *cli.Context) error {
					client := NewClient(c.String("base-url"), c.Int("port"), c.Int("timeout"))

					var path string
					if id := c.String("id"); id != "" {
						// Validate UUID format
						if _, err := uuid.Parse(id); err != nil {
							return fmt.Errorf("invalid UUID format: %s", id)
						}
						path = fmt.Sprintf("/sysinfo/%s", id)
					} else {
						path = "/sysinfo"
					}

					resp, err := client.doRequest("GET", path, nil)
					if err != nil {
						return err
					}
					if resp.Body != nil {
						defer resp.Body.Close()
					}

					respBody, err := processResponse(resp)
					if err != nil {
						return err
					}

					if c.Bool("json-output") {
						fmt.Println(respBody.String())
						return nil
					}

					var result struct {
						ID          string `json:"id"`
						CreatedAtMs int64  `json:"created_at"`
						OS          struct {
							Name    string `json:"name"`
							Version string `json:"version"`
							Arch    string `json:"arch"`
						} `json:"os"`
						CPU struct {
							Model     string  `json:"model"`
							NumCPUs   uint    `json:"num_cpus"`
							Cores     uint    `json:"cores"`
							Frequency float64 `json:"frequency_mhz"`
						} `json:"cpu"`
						GPUs []struct {
							Model         string  `json:"model"`
							Cores         uint    `json:"cores"`
							TotalMemoryMB float64 `json:"total_memory_mb"`
							UsedMemoryMB  float64 `json:"used_memory_mb"`
						} `json:"gpus"`
						Memory struct {
							Total     uint64  `json:"total_bytes"`
							Available uint64  `json:"available_bytes"`
							Used      uint64  `json:"used_bytes"`
							UsedPerc  float64 `json:"used_percent"`
						} `json:"memory"`
						Host struct {
							Hostname string `json:"hostname"`
							Uptime   uint64 `json:"uptime_sec"`
						} `json:"host"`
						Battery struct {
							OnBattery  bool `json:"on_battery"`
							Percentage int  `json:"percentage"`
						} `json:"battery"`
					}

					if err := json.Unmarshal(respBody.Bytes(), &result); err != nil {
						return err
					}

					// Create tables for different sections
					osTable := tablewriter.NewWriter(os.Stdout)
					osTable.SetHeader([]string{"OS Information", "Value"})
					osTable.Append([]string{"Name", result.OS.Name})
					osTable.Append([]string{"Version", result.OS.Version})
					osTable.Append([]string{"Architecture", result.OS.Arch})
					osTable.Render()

					cpuTable := tablewriter.NewWriter(os.Stdout)
					cpuTable.SetHeader([]string{"CPU Information", "Value"})
					cpuTable.Append([]string{"Model", result.CPU.Model})
					cpuTable.Append([]string{"Number of CPUs", fmt.Sprintf("%d", result.CPU.NumCPUs)})
					cpuTable.Append([]string{"Cores", fmt.Sprintf("%d", result.CPU.Cores)})
					cpuTable.Append([]string{"Frequency", fmt.Sprintf("%.2f MHz", result.CPU.Frequency)})
					cpuTable.Render()

					if len(result.GPUs) > 0 {
						fmt.Printf("\nGPU Information (Total: %d)\n", len(result.GPUs))
						for i, gpu := range result.GPUs {
							gpuTable := tablewriter.NewWriter(os.Stdout)
							gpuTable.SetHeader([]string{fmt.Sprintf("GPU #%d", i+1), "Value"})
							gpuTable.Append([]string{"Model", gpu.Model})
							if gpu.Cores > 0 {
								gpuTable.Append([]string{"Cores", fmt.Sprintf("%d", gpu.Cores)})
							}
							gpuTable.Append([]string{"Total Memory", fmt.Sprintf("%.2f MB", gpu.TotalMemoryMB)})
							gpuTable.Append([]string{"Used Memory", fmt.Sprintf("%.2f MB", gpu.UsedMemoryMB)})
							gpuTable.Render()
						}
					}

					memTable := tablewriter.NewWriter(os.Stdout)
					memTable.SetHeader([]string{"Memory Information", "Value"})
					memTable.Append([]string{"Total", fmt.Sprintf("%.2f GB", float64(result.Memory.Total)/(1024*1024*1024))})
					memTable.Append([]string{"Available", fmt.Sprintf("%.2f GB", float64(result.Memory.Available)/(1024*1024*1024))})
					memTable.Append([]string{"Used", fmt.Sprintf("%.2f GB", float64(result.Memory.Used)/(1024*1024*1024))})
					memTable.Append([]string{"Used Percentage", fmt.Sprintf("%.1f%%", result.Memory.UsedPerc)})
					memTable.Render()

					hostTable := tablewriter.NewWriter(os.Stdout)
					hostTable.SetHeader([]string{"Host Information", "Value"})
					hostTable.Append([]string{"Hostname", result.Host.Hostname})
					hostTable.Append([]string{"Uptime", fmt.Sprintf("%d hours", result.Host.Uptime/3600)})
					hostTable.Render()

					batteryTable := tablewriter.NewWriter(os.Stdout)
					batteryTable.SetHeader([]string{"Battery Information", "Value"})
					batteryTable.Append([]string{"On Battery", fmt.Sprintf("%v", result.Battery.OnBattery)})
					batteryTable.Append([]string{"Percentage", fmt.Sprintf("%d%%", result.Battery.Percentage)})
					batteryTable.Render()

					return nil
				},
			},
		},
	}
}
