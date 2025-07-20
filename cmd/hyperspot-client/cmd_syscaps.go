package main

import (
	"encoding/json"
	"fmt"
	"os"
	"strconv"
	"strings"

	"github.com/olekukonko/tablewriter"
	"github.com/urfave/cli/v2"
)

func getSysCapsCommands() *cli.Command {
	return &cli.Command{
		Name:  "syscaps",
		Usage: "System capabilities detection and querying",
		Subcommands: []*cli.Command{
			{
				Name:  "list",
				Usage: "List all system capabilities or capabilities by category",
				Flags: []cli.Flag{
					&cli.StringFlag{
						Name:  "category",
						Usage: "Filter by category (hardware, software, os)",
					},
				},
				Action: func(c *cli.Context) error {
					client := NewClient(c.String("base-url"), c.Int("port"), c.Int("timeout"))

					var path string
					if category := c.String("category"); category != "" {
						// Validate category
						if category != "hardware" && category != "software" && category != "os" && category != "llm_service" && category != "module" {
							return fmt.Errorf("invalid category. Must be one of: hardware, software, os, llm_service, module")
						}
						path = fmt.Sprintf("/syscaps/%s", category)
					} else {
						path = "/syscaps"
					}

					resp, err := client.doRequest("GET", path, nil)
					if err != nil {
						return fmt.Errorf("error requesting GET %s: %v", path, err)
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

					var result []struct {
						Category        string   `json:"category"`
						Name            string   `json:"name"`
						DisplayName     string   `json:"display_name"`
						Present         bool     `json:"present"`
						Version         *string  `json:"version"`
						Amount          *float64 `json:"amount"`
						AmountDimension *string  `json:"amount_dimension"`
						Details         *string  `json:"details"`
					}

					if err := json.Unmarshal(respBody.Bytes(), &result); err != nil {
						return fmt.Errorf("error parsing response: %v", err)
					}

					// Create and display table
					table := tablewriter.NewWriter(os.Stdout)
					table.SetHeader([]string{"Category", "Key", "Display Name", "Present", "Version", "Amount", "Details"})
					table.SetBorder(false)
					table.SetHeaderAlignment(tablewriter.ALIGN_LEFT)
					table.SetAlignment(tablewriter.ALIGN_LEFT)

					for _, cap := range result {
						version := ""
						if cap.Version != nil {
							version = *cap.Version
						}

						amount := ""
						if cap.Amount != nil {
							if cap.AmountDimension != nil {
								amount = fmt.Sprintf("%.2f %s", *cap.Amount, *cap.AmountDimension)
							} else {
								amount = fmt.Sprintf("%.2f", *cap.Amount)
							}
						}

						details := ""
						if cap.Details != nil {
							details = *cap.Details
							// Truncate long details for table display
							if len(details) > 70 {
								details = details[:70] + "..."
							}
						}

						presentStr := "No"
						if cap.Present {
							presentStr = "Yes"
						}

						table.Append([]string{
							cap.Category,
							cap.Name,
							cap.DisplayName,
							presentStr,
							version,
							amount,
							details,
						})
					}

					table.Render()
					return nil
				},
			},
			{
				Name:  "get",
				Usage: "Get a specific capability by key (category:name) or by category and name",
				Flags: []cli.Flag{
					&cli.StringFlag{
						Name:  "key",
						Usage: "Capability key in format 'category:name' (e.g., 'hardware:GPU')",
					},
					&cli.StringFlag{
						Name:  "category",
						Usage: "Capability category (hardware, software, os, llm_service, module)",
					},
					&cli.StringFlag{
						Name:  "name",
						Usage: "Capability name",
					},
				},
				Action: func(c *cli.Context) error {
					client := NewClient(c.String("base-url"), c.Int("port"), c.Int("timeout"))

					var path string
					key := c.String("key")
					category := c.String("category")
					name := c.String("name")

					if key != "" {
						// Validate key format
						if !strings.Contains(key, ":") {
							return fmt.Errorf("invalid key format. Expected 'category:name', got: %s", key)
						}
						path = fmt.Sprintf("/syscaps/%s", key)
					} else if category != "" && name != "" {
						// Validate category
						if category != "hardware" && category != "software" && category != "os" && category != "llm_service" && category != "module" {
							return fmt.Errorf("invalid category. Must be one of: hardware, software, os, llm_service, module")
						}
						path = fmt.Sprintf("/syscaps/%s/%s", category, name)
					} else {
						return fmt.Errorf("must specify either --key or both --category and --name")
					}

					resp, err := client.doRequest("GET", path, nil)
					if err != nil {
						return fmt.Errorf("error requesting GET %s: %v", path, err)
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
						Category        string   `json:"category"`
						Name            string   `json:"name"`
						Present         bool     `json:"present"`
						Version         *string  `json:"version"`
						Amount          *float64 `json:"amount"`
						AmountDimension *string  `json:"amount_dimension"`
						Details         *string  `json:"details"`
					}

					if err := json.Unmarshal(respBody.Bytes(), &result); err != nil {
						return fmt.Errorf("error parsing response: %v", err)
					}

					cap := result

					// Create and display table
					table := tablewriter.NewWriter(os.Stdout)
					table.SetHeader([]string{"Property", "Value"})
					table.SetBorder(false)
					table.SetHeaderAlignment(tablewriter.ALIGN_LEFT)
					table.SetAlignment(tablewriter.ALIGN_LEFT)

					table.Append([]string{"Category", cap.Category})
					table.Append([]string{"Name", cap.Name})

					presentStr := "No"
					if cap.Present {
						presentStr = "Yes"
					}
					table.Append([]string{"Present", presentStr})

					if cap.Version != nil {
						table.Append([]string{"Version", *cap.Version})
					}

					if cap.Amount != nil {
						amountStr := strconv.FormatFloat(*cap.Amount, 'f', -1, 64)
						if cap.AmountDimension != nil {
							amountStr += " " + *cap.AmountDimension
						}
						table.Append([]string{"Amount", amountStr})
					}

					if cap.Details != nil {
						table.Append([]string{"Details", *cap.Details})
					}

					table.Render()
					return nil
				},
			},
			{
				Name:  "refresh",
				Usage: "Refresh capabilities cache (all or by category)",
				Flags: []cli.Flag{
					&cli.StringFlag{
						Name:  "category",
						Usage: "Refresh only specific category (hardware, software, os)",
					},
				},
				Action: func(c *cli.Context) error {
					client := NewClient(c.String("base-url"), c.Int("port"), c.Int("timeout"))

					var path string
					if category := c.String("category"); category != "" {
						// Validate category
						if category != "hardware" && category != "software" && category != "os" && category != "llm_service" && category != "module" {
							return fmt.Errorf("invalid category. Must be one of: hardware, software, os, llm_service, module")
						}
						path = fmt.Sprintf("/syscaps/%s/refresh", category)
					} else {
						path = "/syscaps/refresh"
					}

					resp, err := client.doRequest("POST", path, nil)
					if err != nil {
						return fmt.Errorf("error requesting POST %s: %v", path, err)
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

					var result []struct {
						Category string `json:"category"`
						Name     string `json:"name"`
						Present  bool   `json:"present"`
					}

					if err := json.Unmarshal(respBody.Bytes(), &result); err != nil {
						return fmt.Errorf("error parsing response: %v", err)
					}

					if category := c.String("category"); category != "" {
						fmt.Printf("Successfully refreshed %s capabilities cache\n", category)
					} else {
						fmt.Println("Successfully refreshed all capabilities cache")
					}

					fmt.Printf("Found %d capabilities\n", len(result))
					return nil
				},
			},
		},
	}
}
