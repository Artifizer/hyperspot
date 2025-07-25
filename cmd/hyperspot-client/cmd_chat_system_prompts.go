package main

import (
	"encoding/json"
	"fmt"
	"os"
	"time"

	"github.com/olekukonko/tablewriter"
	"github.com/urfave/cli/v2"
)

// getSystemPromptCommands returns the CLI commands for managing system prompts
func getSystemPromptCommands() *cli.Command {
	return &cli.Command{
		Name:  "system-prompts",
		Usage: "System prompt management",
		Subcommands: []*cli.Command{
			{
				Name:  "list",
				Usage: "List system prompts",
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
						Usage: "Sort order (name, -name, updated_at, -updated_at)",
						Value: "name",
					},
					&cli.StringFlag{
						Name:  "is-default",
						Usage: "Filter by default status (true/false)",
					},
					&cli.StringFlag{
						Name:  "is-file",
						Usage: "Filter by file-based prompts (true/false)",
					},
					&cli.StringFlag{
						Name:  "auto-update",
						Usage: "Filter by auto-update status (true/false)",
					},
					&cli.StringFlag{
						Name:  "name",
						Usage: "Filter by name (partial match, case-insensitive)",
					},
				},
				Action: func(c *cli.Context) error {
					client := NewClient(c.String("base-url"), c.Int("port"), c.Int("timeout"))
					page := c.Int("page")
					pageSize := c.Int("page-size")
					order := c.String("order")

					// Build query parameters
					params := fmt.Sprintf("page=%d&page_size=%d&order=%s", page, pageSize, order)

					// Add filter parameters if provided
					if isDefault := c.String("is-default"); isDefault != "" {
						params += fmt.Sprintf("&is_default=%s", isDefault)
					}
					if isFile := c.String("is-file"); isFile != "" {
						params += fmt.Sprintf("&is_file=%s", isFile)
					}
					if autoUpdate := c.String("auto-update"); autoUpdate != "" {
						params += fmt.Sprintf("&auto_update=%s", autoUpdate)
					}
					if name := c.String("name"); name != "" {
						params += fmt.Sprintf("&name=%s", name)
					}

					path := fmt.Sprintf("/chat/system-prompts?%s", params)
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
						SystemPrompts []struct {
							ID         string `json:"id"`
							Name       string `json:"name"`
							Content    string `json:"content"`
							UserID     string `json:"user_id"`
							CreatedAt  int64  `json:"created_at"`
							UpdatedAt  int64  `json:"updated_at"`
							UIMeta     string `json:"ui_meta"`
							IsDefault  bool   `json:"is_default"`
							FilePath   string `json:"file_path"`
							AutoUpdate bool   `json:"auto_update"`
						} `json:"system_prompts"`
						Total int `json:"total"`
					}

					if err := json.Unmarshal(respBody.Bytes(), &body); err != nil {
						return err
					}

					table := tablewriter.NewWriter(os.Stdout)
					table.SetHeader([]string{"ID", "Name", "Updated At", "Is Default", "File Path", "Auto-Update", "Content Preview"})

					for _, prompt := range body.SystemPrompts {
						// Get a preview of the content (first 30 chars)
						contentPreview := prompt.Content
						if len(contentPreview) > 30 {
							contentPreview = contentPreview[:27] + "..."
						}

						// Format file path display
						filePath := prompt.FilePath
						if filePath == "" {
							filePath = "-"
						}

						// Format auto-update display
						autoUpdate := "No"
						if prompt.AutoUpdate {
							autoUpdate = "Yes"
						}

						table.Append([]string{
							prompt.ID,
							prompt.Name,
							time.UnixMilli(prompt.UpdatedAt).Format("2006-01-02 15:04:05"),
							fmt.Sprintf("%v", prompt.IsDefault),
							filePath,
							autoUpdate,
							contentPreview,
						})
					}

					table.Render()
					fmt.Printf("Total system prompts: %d\n", body.Total)
					return nil
				},
			},
			{
				Name:  "create",
				Usage: "Create a new system prompt",
				Flags: []cli.Flag{
					&cli.StringFlag{
						Name:     "name",
						Usage:    "System prompt name",
						Required: true,
					},
					&cli.StringFlag{
						Name:     "content",
						Usage:    "System prompt content (required if no file-path)",
						Required: false,
					},
					&cli.StringFlag{
						Name:     "ui-meta",
						Usage:    "System prompt UI meta",
						Required: false,
					},
					&cli.BoolFlag{
						Name:     "is-default",
						Usage:    "Set system prompt as default (true/false)",
						Required: false,
						Value:    false,
					},
					&cli.StringFlag{
						Name:     "file-path",
						Usage:    "Path to file containing system prompt content",
						Required: false,
					},
					&cli.BoolFlag{
						Name:     "auto-update",
						Usage:    "Automatically update content when file changes",
						Required: false,
						Value:    false,
					},
				},
				Action: func(c *cli.Context) error {
					client := NewClient(c.String("base-url"), c.Int("port"), c.Int("timeout"))
					name := c.String("name")
					content := c.String("content")
					uiMeta := c.String("ui-meta")
					isDefault := c.Bool("is-default")
					filePath := c.String("file-path")
					autoUpdate := c.Bool("auto-update")

					// Validate that either content or file-path is provided
					if content == "" && filePath == "" {
						return fmt.Errorf("either --content or --file-path must be provided")
					}

					// Validate that if update-from-file is set, file-path must be provided
					if autoUpdate && filePath == "" {
						return fmt.Errorf("--file-path is required when --update-from-file is set")
					}

					body := map[string]interface{}{
						"name":       name,
						"content":    content,
						"ui_meta":    uiMeta,
						"is_default": isDefault,
					}

					// Add file-related fields if provided
					if filePath != "" {
						body["file_path"] = filePath
					}
					if autoUpdate {
						body["auto_update"] = autoUpdate
					}

					resp, err := client.doRequest("POST", "/chat/system-prompts", body)
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
						ID         string `json:"id"`
						Name       string `json:"name"`
						Content    string `json:"content"`
						UserID     string `json:"user_id"`
						CreatedAt  int64  `json:"created_at"`
						UpdatedAt  int64  `json:"updated_at"`
						UIMeta     string `json:"ui_meta"`
						IsDefault  bool   `json:"is_default"`
						FilePath   string `json:"file_path"`
						AutoUpdate bool   `json:"auto_update"`
					}

					if err := json.Unmarshal(respBody.Bytes(), &result); err != nil {
						return err
					}

					fmt.Printf("System prompt created with ID: %s\n", result.ID)
					fmt.Printf("Name: %s\n", result.Name)
					fmt.Printf("Created At: %s\n", time.UnixMilli(result.CreatedAt).Format("2006-01-02 15:04:05"))
					fmt.Printf("Content: %s\n", result.Content)
					fmt.Printf("UI Meta: %s\n", result.UIMeta)
					fmt.Printf("Is Default: %v\n", result.IsDefault)
					if result.FilePath != "" {
						fmt.Printf("File Path: %s\n", result.FilePath)
						fmt.Printf("Auto-Update from File: %v\n", result.AutoUpdate)
					}
					return nil
				},
			},
			{
				Name:  "get",
				Usage: "Get a system prompt by ID",
				Flags: []cli.Flag{
					&cli.StringFlag{
						Name:     "id",
						Usage:    "System prompt ID",
						Required: true,
					},
				},
				Action: func(c *cli.Context) error {
					client := NewClient(c.String("base-url"), c.Int("port"), c.Int("timeout"))
					promptID := c.String("id")

					path := fmt.Sprintf("/chat/system-prompts/%s", promptID)
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
						ID         string `json:"id"`
						Name       string `json:"name"`
						Content    string `json:"content"`
						UserID     string `json:"user_id"`
						CreatedAt  int64  `json:"created_at"`
						UpdatedAt  int64  `json:"updated_at"`
						IsDefault  bool   `json:"is_default"`
						UIMeta     string `json:"ui_meta"`
						FilePath   string `json:"file_path"`
						AutoUpdate bool   `json:"auto_update"`
					}

					if err := json.Unmarshal(respBody.Bytes(), &result); err != nil {
						return err
					}

					fmt.Printf("ID: %s\n", result.ID)
					fmt.Printf("Name: %s\n", result.Name)
					fmt.Printf("User ID: %s\n", result.UserID)
					fmt.Printf("Created At: %s\n", time.UnixMilli(result.CreatedAt).Format("2006-01-02 15:04:05"))
					fmt.Printf("Updated At: %s\n", time.UnixMilli(result.UpdatedAt).Format("2006-01-02 15:04:05"))
					fmt.Printf("Content: %s\n", result.Content)
					fmt.Printf("UI Meta: %s\n", result.UIMeta)
					if result.FilePath != "" {
						fmt.Printf("File Path: %s\n", result.FilePath)
						fmt.Printf("Auto-Update from File: %v\n", result.AutoUpdate)
					}
					return nil
				},
			},
			{
				Name:  "update",
				Usage: "Update a system prompt",
				Flags: []cli.Flag{
					&cli.StringFlag{
						Name:     "id",
						Usage:    "System prompt ID",
						Required: true,
					},
					&cli.StringFlag{
						Name:     "name",
						Usage:    "New system prompt name (optional)",
						Required: false,
					},
					&cli.StringFlag{
						Name:     "content",
						Usage:    "New system prompt content (optional)",
						Required: false,
					},
					&cli.StringFlag{
						Name:     "ui-meta",
						Usage:    "New system prompt UI meta (optional)",
						Required: false,
					},
					&cli.StringFlag{
						Name:     "is-default",
						Usage:    "Set system prompt as default (true/false)",
						Required: false,
					},
					&cli.StringFlag{
						Name:     "file-path",
						Usage:    "Path to file containing system prompt content (optional)",
						Required: false,
					},
					&cli.StringFlag{
						Name:     "auto-update",
						Usage:    "Enable/disable auto-update from file (true/false)",
						Required: false,
					},
				},
				Action: func(c *cli.Context) error {
					client := NewClient(c.String("base-url"), c.Int("port"), c.Int("timeout"))
					promptID := c.String("id")
					name := c.String("name")
					content := c.String("content")
					uiMeta := c.String("ui-meta")
					isDefault := c.Bool("is-default")
					filePath := c.String("file-path")
					autoUpdateStr := c.String("auto-update")

					nameIsSet := c.IsSet("name")
					contentIsSet := c.IsSet("content")
					filePathIsSet := c.IsSet("file-path")
					isDefaultIsSet := c.IsSet("is-default")
					autoUpdateIsSet := c.IsSet("auto-update")
					uiMetaIsSet := c.IsSet("ui-meta")

					// Validate that at least one field is provided
					if !nameIsSet && !contentIsSet && !uiMetaIsSet && !isDefaultIsSet && !autoUpdateIsSet && !filePathIsSet {
						return fmt.Errorf("at least one of --name, --content, --ui-meta, --is-default, --auto-update or --file-path must be provided")
					}

					body := map[string]interface{}{}
					if nameIsSet {
						body["name"] = name
					}
					if contentIsSet {
						body["content"] = content
					}
					if uiMetaIsSet {
						body["ui_meta"] = uiMeta
					}
					if filePathIsSet {
						body["file_path"] = filePath
					}
					if isDefaultIsSet {
						body["is_default"] = isDefault
					}

					// Handle update-from-file flag
					if autoUpdateIsSet {
						switch autoUpdateStr {
						case "true", "1", "yes", "on":
							body["auto_update"] = true
						case "false", "0", "no", "off":
							body["auto_update"] = false
						default:
							return fmt.Errorf("invalid value for --update-from-file: %s (use true/false)", autoUpdateStr)
						}
					}

					path := fmt.Sprintf("/chat/system-prompts/%s", promptID)
					resp, err := client.doRequest("PUT", path, body)
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
						ID         string `json:"id"`
						Name       string `json:"name"`
						Content    string `json:"content"`
						UserID     string `json:"user_id"`
						CreatedAt  int64  `json:"created_at"`
						UpdatedAt  int64  `json:"updated_at"`
						UIMeta     string `json:"ui_meta"`
						IsDefault  bool   `json:"is_default"`
						FilePath   string `json:"file_path"`
						AutoUpdate bool   `json:"auto_update"`
					}

					if err := json.Unmarshal(respBody.Bytes(), &result); err != nil {
						return err
					}

					fmt.Printf("System prompt updated with ID: %s\n", result.ID)
					fmt.Printf("Name: %s\n", result.Name)
					fmt.Printf("Updated At: %s\n", time.UnixMilli(result.UpdatedAt).Format("2006-01-02 15:04:05"))
					fmt.Printf("Content: %s\n", result.Content)
					fmt.Printf("UI Meta: %s\n", result.UIMeta)
					fmt.Printf("Is Default: %v\n", result.IsDefault)
					if result.FilePath != "" {
						fmt.Printf("File Path: %s\n", result.FilePath)
						fmt.Printf("Auto-Update from File: %v\n", result.AutoUpdate)
					}
					return nil
				},
			},
			{
				Name:  "delete",
				Usage: "Delete a system prompt",
				Flags: []cli.Flag{
					&cli.StringFlag{
						Name:     "id",
						Usage:    "System prompt ID",
						Required: true,
					},
				},
				Action: func(c *cli.Context) error {
					client := NewClient(c.String("base-url"), c.Int("port"), c.Int("timeout"))
					promptID := c.String("id")

					path := fmt.Sprintf("/chat/system-prompts/%s", promptID)
					resp, err := client.doRequest("DELETE", path, nil)
					if err != nil {
						return err
					}
					if resp.Body != nil {
						defer resp.Body.Close()
					}

					if resp.StatusCode == 200 || resp.StatusCode == 204 {
						fmt.Println("System prompt deleted successfully")
						return nil
					}

					respBody, err := processResponse(resp)
					if err != nil {
						return err
					}

					if c.Bool("json-output") {
						fmt.Println(respBody.String())
					}

					return nil
				},
			},
			{
				Name:  "attach",
				Usage: "Attach system prompts to a chat thread",
				Flags: []cli.Flag{
					&cli.StringFlag{
						Name:     "thread-id",
						Usage:    "Chat thread ID",
						Required: true,
					},
					&cli.StringSliceFlag{
						Name:     "prompt-ids",
						Usage:    "System prompt IDs to attach (comma-separated)",
						Required: true,
					},
				},
				Action: func(c *cli.Context) error {
					client := NewClient(c.String("base-url"), c.Int("port"), c.Int("timeout"))
					threadID := c.String("thread-id")
					promptIDs := c.StringSlice("prompt-ids")

					body := map[string]interface{}{
						"thread_id":         threadID,
						"system_prompt_ids": promptIDs,
					}

					resp, err := client.doRequest("POST", "/chat/system-prompts/attach", body)
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

					fmt.Printf("Successfully attached %d system prompts to thread %s\n", len(promptIDs), threadID)
					return nil
				},
			},
			{
				Name:  "detach",
				Usage: "Detach system prompts from a chat thread",
				Flags: []cli.Flag{
					&cli.StringFlag{
						Name:     "thread-id",
						Usage:    "Chat thread ID",
						Required: true,
					},
					&cli.StringSliceFlag{
						Name:     "prompt-ids",
						Usage:    "System prompt IDs to detach (comma-separated)",
						Required: true,
					},
				},
				Action: func(c *cli.Context) error {
					client := NewClient(c.String("base-url"), c.Int("port"), c.Int("timeout"))
					threadID := c.String("thread-id")
					promptIDs := c.StringSlice("prompt-ids")

					body := map[string]interface{}{
						"thread_id":         threadID,
						"system_prompt_ids": promptIDs,
					}

					resp, err := client.doRequest("POST", "/chat/system-prompts/detach", body)
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

					fmt.Printf("Successfully detached %d system prompts from thread %s\n", len(promptIDs), threadID)
					return nil
				},
			},
			{
				Name:  "detach-all",
				Usage: "Detach all system prompts from a chat thread",
				Flags: []cli.Flag{
					&cli.StringFlag{
						Name:     "thread-id",
						Usage:    "Chat thread ID",
						Required: true,
					},
				},
				Action: func(c *cli.Context) error {
					client := NewClient(c.String("base-url"), c.Int("port"), c.Int("timeout"))
					threadID := c.String("thread-id")

					body := map[string]interface{}{
						"thread_id": threadID,
					}

					resp, err := client.doRequest("POST", "/chat/system-prompts/detach-all", body)
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

					fmt.Printf("Successfully detached all system prompts from thread %s\n", threadID)
					return nil
				},
			},
			{
				Name:  "list-for-thread",
				Usage: "List all system prompts attached to a thread",
				Flags: []cli.Flag{
					&cli.StringFlag{
						Name:     "thread-id",
						Usage:    "Chat thread ID",
						Required: true,
					},
				},
				Action: func(c *cli.Context) error {
					client := NewClient(c.String("base-url"), c.Int("port"), c.Int("timeout"))
					threadID := c.String("thread-id")

					path := fmt.Sprintf("/chat/threads/%s/system-prompts", threadID)
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
						SystemPrompts []struct {
							ID         string `json:"id"`
							Name       string `json:"name"`
							Content    string `json:"content"`
							UserID     string `json:"user_id"`
							IsDefault  bool   `json:"is_default"`
							FilePath   string `json:"file_path"`
							AutoUpdate bool   `json:"auto_update"`
							CreatedAt  int64  `json:"created_at"`
							UpdatedAt  int64  `json:"updated_at"`
						} `json:"system_prompts"`
						Total int `json:"total"`
					}

					if err := json.Unmarshal(respBody.Bytes(), &body); err != nil {
						return err
					}

					table := tablewriter.NewWriter(os.Stdout)
					table.SetHeader([]string{"ID", "Name", "Is Default", "File Path", "Auto-Update", "Content Preview"})

					for _, prompt := range body.SystemPrompts {
						// Get a preview of the content (first 30 chars)
						contentPreview := prompt.Content
						if len(contentPreview) > 30 {
							contentPreview = contentPreview[:27] + "..."
						}

						table.Append([]string{
							prompt.ID,
							prompt.Name,
							fmt.Sprintf("%v", prompt.IsDefault),
							prompt.FilePath,
							fmt.Sprintf("%v", prompt.AutoUpdate),
							contentPreview,
						})
					}

					table.Render()
					fmt.Printf("Total system prompts for thread %s: %d\n", threadID, body.Total)
					return nil
				},
			},
			{
				Name:  "list-threads",
				Usage: "List all threads that have a specific system prompt attached",
				Flags: []cli.Flag{
					&cli.StringFlag{
						Name:     "prompt-id",
						Usage:    "System prompt ID",
						Required: true,
					},
				},
				Action: func(c *cli.Context) error {
					client := NewClient(c.String("base-url"), c.Int("port"), c.Int("timeout"))
					promptID := c.String("prompt-id")

					path := fmt.Sprintf("/chat/system-prompts/%s/threads", promptID)
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
						ChatThreads []struct {
							ID        string `json:"id"`
							Title     string `json:"title"`
							UserID    string `json:"user_id"`
							CreatedAt int64  `json:"created_at"`
							UpdatedAt int64  `json:"updated_at"`
						} `json:"threads"`
						Total int `json:"total"`
					}

					if err := json.Unmarshal(respBody.Bytes(), &body); err != nil {
						return err
					}

					table := tablewriter.NewWriter(os.Stdout)
					table.SetHeader([]string{"ID", "Title", "Created At"})

					for _, thread := range body.ChatThreads {
						table.Append([]string{
							thread.ID,
							thread.Title,
							time.UnixMilli(thread.CreatedAt).Format("2006-01-02 15:04:05"),
						})
					}

					table.Render()
					fmt.Printf("Total threads for system prompt %s: %d\n", promptID, body.Total)
					return nil
				},
			},
		},
	}
}
