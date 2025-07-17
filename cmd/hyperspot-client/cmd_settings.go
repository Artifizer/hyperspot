package main

import (
	"encoding/json"
	"fmt"
	"os"

	"github.com/olekukonko/tablewriter"
	"github.com/urfave/cli/v2"
)

func getSettingsCommands() *cli.Command {
	return &cli.Command{
		Name:  "settings",
		Usage: "User settings operations",
		Subcommands: []*cli.Command{
			{
				Name:  "get",
				Usage: "Get user settings",
				Action: func(c *cli.Context) error {
					client := NewClient(c.String("base-url"), c.Int("port"), c.Int("timeout"))

					// Build path with query parameters
					path := "/settings"

					// Make request
					resp, err := client.doRequest("GET", path, nil)
					if err != nil {
						return fmt.Errorf("error requesting GET %s: %v", path, err)
					}
					if resp.Body != nil {
						defer resp.Body.Close()
					}

					// Process response
					respBody, err := processResponse(resp)
					if err != nil {
						return err
					}

					// If JSON output is requested, just print the raw response
					if c.Bool("json-output") {
						fmt.Println(respBody.String())
						return nil
					}

					// Parse the settings
					var setting struct {
						Theme    string `json:"theme"`
						Language string `json:"language"`
					}

					if err := json.Unmarshal(respBody.Bytes(), &setting); err != nil {
						return fmt.Errorf("error parsing settings: %v", err)
					}

					// Display settings in a table
					table := tablewriter.NewWriter(os.Stdout)
					table.SetHeader([]string{"Property", "Value"})

					table.Append([]string{"Theme", setting.Theme})
					table.Append([]string{"Language", setting.Language})

					table.Render()
					return nil
				},
			},
			{
				Name:  "update",
				Usage: "Update user settings",
				Flags: []cli.Flag{
					&cli.StringFlag{
						Name:  "theme",
						Usage: "Theme (e.g., light, dark)",
						Value: "light",
					},
					&cli.StringFlag{
						Name:  "language",
						Usage: "Language code (e.g., en, fr, es)",
						Value: "en",
					},
				},
				Action: func(c *cli.Context) error {
					client := NewClient(c.String("base-url"), c.Int("port"), c.Int("timeout"))

					// Create request body
					requestBody := map[string]interface{}{
						"theme":    c.String("theme"),
						"language": c.String("language"),
					}

					// Make request
					resp, err := client.doRequest("POST", "/settings", requestBody)
					if err != nil {
						return fmt.Errorf("error requesting POST /settings: %v", err)
					}
					if resp.Body != nil {
						defer resp.Body.Close()
					}

					// Process response
					respBody, err := processResponse(resp)
					if err != nil {
						return err
					}

					// If JSON output is requested, just print the raw response
					if c.Bool("json-output") {
						fmt.Println(respBody.String())
						return nil
					}

					// Parse the updated settings
					var setting struct {
						Theme    string `json:"theme"`
						Language string `json:"language"`
					}

					if err := json.Unmarshal(respBody.Bytes(), &setting); err != nil {
						return fmt.Errorf("error parsing settings: %v", err)
					}

					// Display settings in a table
					table := tablewriter.NewWriter(os.Stdout)
					table.SetHeader([]string{"Property", "Value"})

					table.Append([]string{"Theme", setting.Theme})
					table.Append([]string{"Language", setting.Language})

					table.Render()
					return nil
				},
			},
			{
				Name:  "patch",
				Usage: "Partially update user settings",
				Flags: []cli.Flag{
					&cli.StringFlag{
						Name:  "theme",
						Usage: "Theme (e.g., light, dark)",
					},
					&cli.StringFlag{
						Name:  "language",
						Usage: "Language code (e.g., en, fr, es)",
					},
				},
				Action: func(c *cli.Context) error {
					client := NewClient(c.String("base-url"), c.Int("port"), c.Int("timeout"))

					// Create request body with only the provided fields
					requestBody := map[string]interface{}{}

					// Only add fields that were explicitly provided
					if c.IsSet("theme") {
						requestBody["theme"] = c.String("theme")
					}
					if c.IsSet("language") {
						requestBody["language"] = c.String("language")
					}

					// Make request
					resp, err := client.doRequest("PATCH", "/settings", requestBody)
					if err != nil {
						return fmt.Errorf("error requesting PATCH /settings: %v", err)
					}
					if resp.Body != nil {
						defer resp.Body.Close()
					}

					// Process response
					respBody, err := processResponse(resp)
					if err != nil {
						return err
					}

					// If JSON output is requested, just print the raw response
					if c.Bool("json-output") {
						fmt.Println(respBody.String())
						return nil
					}

					// Parse the updated settings
					var setting struct {
						Theme    string `json:"theme"`
						Language string `json:"language"`
					}

					if err := json.Unmarshal(respBody.Bytes(), &setting); err != nil {
						return fmt.Errorf("error parsing settings: %v", err)
					}

					// Display settings in a table
					table := tablewriter.NewWriter(os.Stdout)
					table.SetHeader([]string{"Property", "Value"})

					table.Append([]string{"Theme", setting.Theme})
					table.Append([]string{"Language", setting.Language})

					table.Render()
					return nil
				},
			},
		},
	}
}
