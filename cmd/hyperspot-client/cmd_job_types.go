package main

import (
	"encoding/json"
	"fmt"
	"os"

	"github.com/olekukonko/tablewriter"
	"github.com/urfave/cli/v2"
)

func getJobTypesCommands() *cli.Command {
	return &cli.Command{
		Name:  "job-types",
		Usage: "Job type management commands",
		Subcommands: []*cli.Command{
			{
				Name:  "list",
				Usage: "List all job types",
				Flags: []cli.Flag{
					&cli.IntFlag{
						Name:  "page_size",
						Usage: "Page size",
						Value: 100,
					},
					&cli.IntFlag{
						Name:  "page",
						Usage: "Page number",
						Value: 1,
					},
				},
				Action: func(c *cli.Context) error {
					client := NewClient(c.String("base-url"), c.Int("port"), c.Int("timeout"))
					pageSize := c.Int("page_size")
					page := c.Int("page")
					path := fmt.Sprintf("/job_types?page_size=%d&page=%d", pageSize, page)
					resp, err := client.doRequest("GET", path, nil)
					if err != nil {
						return fmt.Errorf("error requesting GET %s json: %v", path, err)
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

					var body struct {
						JobTypes []struct {
							TypeID      string `json:"type_id"`
							Description string `json:"description"`
							Suspendable bool   `json:"suspendable"`
							TimeoutSec  int    `json:"timeout_sec"`
							Schema      struct {
								Properties map[string]struct {
									Type        string `json:"type"`
									Description string `json:"description"`
								} `json:"properties"`
							} `json:"schema"`
						} `json:"job_types"`
					}
					if err := json.Unmarshal(respBody.Bytes(), &body); err != nil {
						return fmt.Errorf("error unmarshalling GET %s json: %v", path, err)
					}

					table := tablewriter.NewWriter(os.Stdout)
					table.SetHeader([]string{"Type", "Description", "Suspendable", "Timeout Sec"})

					for _, jt := range body.JobTypes {
						table.Append([]string{jt.TypeID, jt.Description, fmt.Sprintf("%t", jt.Suspendable), fmt.Sprintf("%d", jt.TimeoutSec)})
					}

					table.Render()
					return nil
				},
			},
			{
				Name:  "get",
				Usage: "Get job type details",
				Flags: []cli.Flag{
					&cli.StringFlag{
						Name:     "type",
						Usage:    "Job type ID",
						Required: true,
					},
				},
				Action: func(c *cli.Context) error {
					client := NewClient(c.String("base-url"), c.Int("port"), c.Int("timeout"))
					path := fmt.Sprintf("/job_types/%s", c.String("type"))
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

					// Parse the direct API response
					var jobTypeResponse struct {
						TypeID        string      `json:"type_id"`
						Name          string      `json:"name"`
						Group         string      `json:"group"`
						Description   string      `json:"description"`
						TimeoutSec    int64       `json:"timeout_sec"`
						MaxRetries    int         `json:"max_retries"`
						RetryDelaySec int64       `json:"retry_delay_sec"`
						Params        interface{} `json:"params"`
						ParamsSchema  string      `json:"params_schema"`
					}

					if err := json.Unmarshal(respBody.Bytes(), &jobTypeResponse); err != nil {
						return fmt.Errorf("error parsing job type: %v", err)
					}

					// Display basic job type info
					fmt.Printf("Type ID: %s\n", jobTypeResponse.TypeID)
					fmt.Printf("Name: %s\n", jobTypeResponse.Name)
					fmt.Printf("Group: %s\n", jobTypeResponse.Group)
					fmt.Printf("Description: %s\n", jobTypeResponse.Description)
					fmt.Printf("Timeout: %d sec\n", jobTypeResponse.TimeoutSec/1000000000)
					fmt.Printf("Max Retries: %d\n", jobTypeResponse.MaxRetries)
					fmt.Printf("Retry Delay: %d sec\n", jobTypeResponse.RetryDelaySec/1000000000)
					fmt.Println("\nParameters:")

					// Parse the params_schema
					var paramsSchema struct {
						Type                 string `json:"type"`
						AdditionalProperties bool   `json:"additionalProperties"`
						Properties           map[string]struct {
							Type        string `json:"type"`
							Description string `json:"description,omitempty"`
						} `json:"properties"`
						Required []string `json:"required"`
					}

					if err := json.Unmarshal([]byte(jobTypeResponse.ParamsSchema), &paramsSchema); err != nil {
						// If we can't parse the schema, just show the raw params
						table := tablewriter.NewWriter(os.Stdout)
						table.SetHeader([]string{"Parameter", "Default Value"})

						// Handle interface{} type for params
						if paramsMap, ok := jobTypeResponse.Params.(map[string]interface{}); ok {
							for name, value := range paramsMap {
								table.Append([]string{name, fmt.Sprintf("%v", value)})
							}
						}

						table.Render()
						return nil
					}

					// Create a map to track required parameters
					requiredParams := make(map[string]bool)
					for _, name := range paramsSchema.Required {
						requiredParams[name] = true
					}

					// Display parameters with their details
					table := tablewriter.NewWriter(os.Stdout)
					table.SetHeader([]string{"Name", "Type", "Required", "Default", "Description"})

					for name, prop := range paramsSchema.Properties {
						isRequired := "No"
						if requiredParams[name] {
							isRequired = "Yes"
						}

						defaultValue := "-"
						// Handle interface{} type for params
						if paramsMap, ok := jobTypeResponse.Params.(map[string]interface{}); ok {
							if val, exists := paramsMap[name]; exists && val != nil {
								defaultValue = fmt.Sprintf("%v", val)
							}
						}

						description := prop.Description
						if description == "" {
							description = "-"
						}

						table.Append([]string{name, prop.Type, isRequired, defaultValue, description})
					}

					table.Render()
					return nil
				},
			},
		},
	}
}
