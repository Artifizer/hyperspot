package main

import (
	"encoding/json"
	"fmt"
	"os"
	"time"

	"github.com/olekukonko/tablewriter"
	"github.com/urfave/cli/v2"
)

func getJobGroupsCommands() *cli.Command {
	return &cli.Command{
		Name:  "job-groups",
		Usage: "Job group management commands",
		Subcommands: []*cli.Command{
			{
				Name:  "list",
				Usage: "List all job groups",
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
						Name:  "order",
						Usage: "Sort order",
						Value: "-updated_at",
					},
				},
				Action: func(c *cli.Context) error {
					client := NewClient(c.String("base-url"), c.Int("port"), c.Int("timeout"))

					page := c.Int("page")
					pageSize := c.Int("page-size")
					order := c.String("order")

					path := fmt.Sprintf("/job_groups?page=%d&page_size=%d&order=%s", page, pageSize, order)

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

					var body struct {
						JobGroups []struct {
							ID          string `json:"id"`
							Name        string `json:"name"`
							Description string `json:"description"`
							CreatedAt   int64  `json:"created_at"`
							UpdatedAt   int64  `json:"updated_at"`
						} `json:"job_groups"`
					}

					if err := json.Unmarshal(respBody.Bytes(), &body); err != nil {
						return err
					}

					table := tablewriter.NewWriter(os.Stdout)
					table.SetHeader([]string{"ID", "Name", "Description", "Created At", "Updated At"})

					for _, group := range body.JobGroups {
						table.Append([]string{
							group.ID,
							group.Name,
							group.Description,
							time.UnixMilli(group.CreatedAt).Format("2006-01-02 15:04:05"),
							time.UnixMilli(group.UpdatedAt).Format("2006-01-02 15:04:05"),
						})
					}

					table.Render()
					return nil
				},
			},
			{
				Name:  "get",
				Usage: "Get a job group by ID",
				Flags: []cli.Flag{
					&cli.StringFlag{
						Name:     "group",
						Usage:    "Job group ID",
						Required: true,
					},
				},
				Action: func(c *cli.Context) error {
					client := NewClient(c.String("base-url"), c.Int("port"), c.Int("timeout"))
					path := fmt.Sprintf("/job_groups/%s", c.String("group"))
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

					// Format as table
					table := tablewriter.NewWriter(os.Stdout)
					table.SetHeader([]string{"Field", "Value"})

					var jobGroup map[string]interface{}
					if err := json.Unmarshal(respBody.Bytes(), &jobGroup); err != nil {
						return fmt.Errorf("error unmarshalling GET %s json: %v", path, err)
					}

					for k, v := range jobGroup {
						table.Append([]string{k, fmt.Sprintf("%v", v)})
					}

					table.Render()
					return nil
				},
			},
		},
	}
}
