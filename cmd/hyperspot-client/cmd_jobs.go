package main

import (
	"encoding/json"
	"fmt"
	"os"
	"sort"
	"time"

	"github.com/google/uuid"
	"github.com/olekukonko/tablewriter"
	"github.com/urfave/cli/v2"
)

func jobsCommands() *cli.Command {
	return &cli.Command{
		Name:  "jobs",
		Usage: "Job management commands",
		Subcommands: []*cli.Command{
			{
				Name:  "list",
				Usage: "List all jobs",
				Flags: []cli.Flag{
					&cli.IntFlag{
						Name:  "page",
						Usage: "Page number",
						Value: 1,
					},
					&cli.IntFlag{
						Name:  "page-size",
						Usage: "Items per page",
						Value: 20,
					},
					&cli.StringFlag{
						Name:  "status",
						Usage: "Filter by status",
						Value: "",
					},
					&cli.StringFlag{
						Name:  "order",
						Usage: "Sort order",
						Value: "-scheduled_at",
					},
				},
				Action: func(c *cli.Context) error {
					client := NewClient(c.String("base-url"), c.Int("port"), c.Int("timeout"))

					page := c.Int("page")
					pageSize := c.Int("page-size")
					order := c.String("order")
					status := c.String("status")
					path := fmt.Sprintf("/jobs?page=%d&page_size=%d&order=%s", page, pageSize, order)

					if status != "" {
						path += fmt.Sprintf("&status=%s", status)
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
					} else {
						body := struct {
							Jobs []struct {
								ID        string      `json:"id"`
								Type      string      `json:"type"`
								Params    interface{} `json:"params,omitempty"`
								Status    string      `json:"status"`
								Progress  float32     `json:"progress"`
								UpdatedAt int64       `json:"updated_at"`
								Error     string      `json:"error"`
							} `json:"jobs"`
						}{}
						if err := json.NewDecoder(respBody).Decode(&body); err != nil {
							return err
						}

						table := tablewriter.NewWriter(os.Stdout)
						table.SetHeader([]string{"ID", "Type", "Status", "Progress", "UpdatedAt", "Error"})

						for _, job := range body.Jobs {
							errMsg := job.Error
							if errMsg != "" {
								if len(errMsg) > 16 {
									errMsg = errMsg[:16] + "..."
								}
							}
							table.Append([]string{job.ID, job.Type, job.Status, fmt.Sprintf("%.1f%%", job.Progress), time.UnixMilli(job.UpdatedAt).Format("2006-01-02 15:04:05"), errMsg})
						}

						table.Render()
					}
					return nil
				},
			},
			{
				Name:  "get",
				Usage: "Get job details",
				Flags: []cli.Flag{
					&cli.StringFlag{
						Name:     "id",
						Usage:    "Job ID",
						Required: true,
					},
				},
				Action: func(c *cli.Context) error {
					client := NewClient(c.String("base-url"), c.Int("port"), c.Int("timeout"))
					jobID := c.String("id")

					path := fmt.Sprintf("/jobs/%s", jobID)
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

					var job map[string]interface{}
					if err := json.Unmarshal(respBody.Bytes(), &job); err != nil {
						return err
					}

					table := tablewriter.NewWriter(os.Stdout)
					table.SetHeader([]string{"Field", "Value"})

					// Format timestamps
					keys := make([]string, 0, len(job))
					for k := range job {
						keys = append(keys, k)
					}
					sort.Strings(keys)
					for _, k := range keys {
						v := job[k]
						if k == "created_at" || k == "started_at" || k == "completed_at" || k == "scheduled_at" || k == "eta" || k == "updated_at" {
							if timestamp, ok := v.(float64); ok && timestamp > 0 {
								job[k] = time.UnixMilli(int64(timestamp)).Format("2006-01-02 15:04:05.123")
							}
						}
						// Format progress as percentage
						if k == "progress" {
							if progress, ok := v.(float32); ok {
								job[k] = fmt.Sprintf("%.1f%%", progress)
							}
						}

						if k == "params" {
							// parse string to json first
							var jsonParams map[string]interface{}
							if err := json.Unmarshal([]byte(v.(string)), &jsonParams); err != nil {
								return err
							}

							jsonParamsString, err := json.MarshalIndent(jsonParams, "", "  ")
							if err != nil {
								return err
							}
							job[k] = string(jsonParamsString)
						}
					}

					// Display fields in a alphabetical order
					orderedFields := sort.StringSlice(keys)
					sort.Sort(orderedFields)

					for _, field := range orderedFields {
						if v, ok := job[field]; ok {
							table.Append([]string{field, fmt.Sprintf("%v", v)})
							delete(job, field)
						}
					}

					// Display remaining fields
					for k, v := range job {
						table.Append([]string{k, fmt.Sprintf("%v", v)})
					}

					table.Render()
					return nil
				},
			},
			{
				Name:  "run",
				Usage: "Run a new job",
				Flags: []cli.Flag{
					&cli.StringFlag{
						Name:     "type",
						Usage:    "Job type",
						Required: true,
					},
					&cli.StringFlag{
						Name:  "params",
						Usage: "JSON with job parameters",
					},
					&cli.StringFlag{
						Name:  "params-file",
						Usage: "Path to JSON file with job parameters",
					},
				},
				Action: func(c *cli.Context) error {
					client := NewClient(c.String("base-url"), c.Int("port"), c.Int("timeout"))

					params, err := getParams(c)
					if err != nil {
						return err
					}

					body := map[string]interface{}{
						"type":            c.String("type"),
						"idempotency_key": uuid.New().String(),
						"params":          params,
					}

					resp, err := client.doRequest("POST", "/jobs", body)
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
					} else {
						type jobResponse struct {
							ID string `json:"id"`
						}

						var job jobResponse
						if err := json.Unmarshal(respBody.Bytes(), &job); err != nil {
							return err
						}
						fmt.Printf("Job '%s' scheduled successfully\n", job.ID)
					}
					return nil
				},
			},
			{
				Name:  "cancel",
				Usage: "Cancel a job",
				Flags: []cli.Flag{
					&cli.StringFlag{
						Name:     "id",
						Usage:    "Job ID",
						Required: true,
					},
				},
				Action: func(c *cli.Context) error {
					client := NewClient(c.String("base-url"), c.Int("port"), c.Int("timeout"))
					jobID := c.String("id")

					path := fmt.Sprintf("/jobs/%s/cancel", jobID)
					resp, err := client.doRequest("POST", path, nil)
					if err != nil {
						return err
					}
					if resp.Body != nil {
						defer resp.Body.Close()
					}

					if resp.StatusCode == 200 || resp.StatusCode == 204 {
						fmt.Println("Job canceled successfully")
						return nil
					}

					respBody, err := processResponse(resp)
					if err != nil {
						return err
					}

					if c.Bool("json-output") {
						fmt.Println(respBody.String())
					} else {
						fmt.Println("Job cancellation request sent")
					}
					return nil
				},
			},
			{
				Name:  "delete",
				Usage: "Delete a job",
				Flags: []cli.Flag{
					&cli.StringFlag{
						Name:     "id",
						Usage:    "Job ID",
						Required: true,
					},
				},
				Action: func(c *cli.Context) error {
					client := NewClient(c.String("base-url"), c.Int("port"), c.Int("timeout"))
					jobID := c.String("id")

					path := fmt.Sprintf("/jobs/%s", jobID)
					resp, err := client.doRequest("DELETE", path, nil)
					if err != nil {
						return err
					}
					if resp.Body != nil {
						defer resp.Body.Close()
					}

					if resp.StatusCode == 202 {
						fmt.Println("Job deleted successfully")
						return nil
					}

					respBody, err := processResponse(resp)
					if err != nil {
						return err
					}

					if c.Bool("json-output") {
						fmt.Println(respBody.String())
					} else {
						fmt.Println("Job deletion request sent")
					}
					return nil
				},
			},
			{
				Name:  "suspend",
				Usage: "Suspend a job",
				Flags: []cli.Flag{
					&cli.StringFlag{
						Name:     "id",
						Usage:    "Job ID",
						Required: true,
					},
				},
				Action: func(c *cli.Context) error {
					client := NewClient(c.String("base-url"), c.Int("port"), c.Int("timeout"))
					jobID := c.String("id")

					path := fmt.Sprintf("/jobs/%s/suspend", jobID)
					resp, err := client.doRequest("POST", path, nil)
					if err != nil {
						return err
					}
					if resp.Body != nil {
						defer resp.Body.Close()
					}

					if resp.StatusCode == 200 || resp.StatusCode == 204 {
						fmt.Println("Job suspended successfully")
						return nil
					}

					respBody, err := processResponse(resp)
					if err != nil {
						return err
					}

					if c.Bool("json-output") {
						fmt.Println(respBody.String())
					} else {
						fmt.Println("Job suspension request sent")
					}
					return nil
				},
			},
			{
				Name:  "resume",
				Usage: "Resume a suspended job",
				Flags: []cli.Flag{
					&cli.StringFlag{
						Name:     "id",
						Usage:    "Job ID",
						Required: true,
					},
				},
				Action: func(c *cli.Context) error {
					client := NewClient(c.String("base-url"), c.Int("port"), c.Int("timeout"))
					jobID := c.String("id")

					path := fmt.Sprintf("/jobs/%s/resume", jobID)
					resp, err := client.doRequest("POST", path, nil)
					if err != nil {
						return err
					}
					if resp.Body != nil {
						defer resp.Body.Close()
					}

					if resp.StatusCode == 200 || resp.StatusCode == 204 {
						fmt.Println("Job resumed successfully")
						return nil
					}

					respBody, err := processResponse(resp)
					if err != nil {
						return err
					}

					if c.Bool("json-output") {
						fmt.Println(respBody.String())
					} else {
						fmt.Println("Job resume request sent")
					}
					return nil
				},
			},
		},
	}
}
