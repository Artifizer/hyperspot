package main

import (
	"encoding/json"
	"fmt"
	"os"
	"time"

	"github.com/olekukonko/tablewriter"
	"github.com/urfave/cli/v2"
)

func getChatCommands() *cli.Command {
	return &cli.Command{
		Name:  "chat",
		Usage: "Chat operations",
		Subcommands: []*cli.Command{
			getSystemPromptCommands(),
			getChatAttachmentCommands(),
			{
				Name:  "interactive",
				Usage: "Start an interactive chat session with optional thread history",
				Flags: []cli.Flag{
					&cli.StringFlag{
						Name:     "thread-id",
						Aliases:  []string{"t"},
						Usage:    "Chat thread ID to load history from",
						Required: true,
					},
					&cli.StringFlag{
						Name:    "model",
						Aliases: []string{"m"},
						Usage:   "Model name",
						Value:   "qwen2.5",
					},
					&cli.Float64Flag{
						Name:  "temperature",
						Usage: "Temperature (0.0-2.0)",
						Value: 0.7,
					},
					&cli.IntFlag{
						Name:  "max-tokens",
						Usage: "Max completion tokens",
						Value: 2048,
					},
					&cli.StringFlag{
						Name:  "system",
						Usage: "System message for the chat",
						Value: "You are a helpful assistant",
					},
					&cli.StringSliceFlag{
						Name:  "system-prompt-id",
						Usage: "System prompt ID to attach to the chat thread (can be specified multiple times)",
					},
					&cli.BoolFlag{
						Name:  "hide-history",
						Usage: "Don't show message history at start",
						Value: false,
					},
				},
				Action: func(c *cli.Context) error {
					client := NewClient(c.String("base-url"), c.Int("port"), c.Int("timeout"))

					session := NewChatSession(
						client,
						c.String("model"),
						c.Float64("temperature"),
						c.Int("max-tokens"),
						c.String("system"),
					)

					session.ShowMessages = !c.Bool("hide-history")

					// Load the thread if provided
					if err := session.LoadChatThread(c.String("thread-id"), c.Bool("json-output")); err != nil {
						return err
					}

					// Attach system prompts if specified
					systemPromptIDs := c.StringSlice("system-prompt-id")
					if len(systemPromptIDs) > 0 {
						threadID := c.String("thread-id")
						body := map[string]interface{}{
							"thread_id":         threadID,
							"system_prompt_ids": systemPromptIDs,
						}

						resp, err := client.doRequest("POST", "/chat/system-prompts/attach", body)
						if err != nil {
							return fmt.Errorf("failed to attach system prompts: %w", err)
						}
						if resp.Body != nil {
							defer resp.Body.Close()
						}

						if resp.StatusCode >= 400 {
							return fmt.Errorf("failed to attach system prompts: status code %d", resp.StatusCode)
						}

						if !c.Bool("json-output") {
							fmt.Printf("Attached %d system prompt(s) to the chat thread\n", len(systemPromptIDs))
						}

						// Reload the thread to get the updated system prompts
						if err := session.LoadChatThread(threadID, c.Bool("json-output")); err != nil {
							return err
						}
					}

					return session.RunChatSession(c.Bool("json-output"))
				},
			},
			{
				Name:  "groups",
				Usage: "Chat group management",
				Subcommands: []*cli.Command{
					{
						Name:  "list",
						Usage: "List chat groups",
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

							path := fmt.Sprintf("/chat/groups?page=%d&page_size=%d&order=%s", page, pageSize, order)
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
								ChatThreadGroups []struct {
									ID        string `json:"id"`
									Title     string `json:"title"`
									CreatedAt int64  `json:"created_at"`
									UpdatedAt int64  `json:"updated_at"`
									UserID    string `json:"user_id"`
								} `json:"groups"`
								Total int `json:"total"`
							}

							if err := json.Unmarshal(respBody.Bytes(), &body); err != nil {
								return err
							}

							table := tablewriter.NewWriter(os.Stdout)
							table.SetHeader([]string{"ID", "Title", "Created At", "Updated At", "User ID"})

							for _, group := range body.ChatThreadGroups {
								table.Append([]string{
									group.ID,
									group.Title,
									time.UnixMilli(group.CreatedAt).Format("2006-01-02 15:04:05"),
									time.UnixMilli(group.UpdatedAt).Format("2006-01-02 15:04:05"),
									group.UserID,
								})
							}

							table.Render()
							fmt.Printf("Total groups: %d\n", body.Total)
							return nil
						},
					},
					{
						Name:  "create",
						Usage: "Create a new chat group",
						Flags: []cli.Flag{
							&cli.StringFlag{
								Name:     "title",
								Usage:    "Group title",
								Required: true,
							},
							&cli.BoolFlag{
								Name:  "pinned",
								Usage: "Mark group as pinned",
							},
							&cli.BoolFlag{
								Name:  "public",
								Usage: "Mark group as public",
							},
							&cli.BoolFlag{
								Name:  "temporary",
								Usage: "Mark group as temporary",
							},
						},
						Action: func(c *cli.Context) error {
							client := NewClient(c.String("base-url"), c.Int("port"), c.Int("timeout"))
							title := c.String("title")

							body := map[string]interface{}{
								"title": title,
							}

							if c.IsSet("pinned") {
								body["is_pinned"] = c.Bool("pinned")
							}

							if c.IsSet("public") {
								body["is_public"] = c.Bool("public")
							}

							if c.IsSet("temporary") {
								body["is_temporary"] = c.Bool("temporary")
							}

							resp, err := client.doRequest("POST", "/chat/groups", body)
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
								ID string `json:"id"`
							}

							if err := json.Unmarshal(respBody.Bytes(), &result); err != nil {
								return err
							}

							fmt.Printf("Group created with ID: %s\n", result.ID)
							return nil
						},
					},
					{
						Name:  "get",
						Usage: "Get chat group details",
						Flags: []cli.Flag{
							&cli.StringFlag{
								Name:     "id",
								Usage:    "Group ID",
								Required: true,
							},
						},
						Action: func(c *cli.Context) error {
							client := NewClient(c.String("base-url"), c.Int("port"), c.Int("timeout"))
							groupID := c.String("id")

							path := fmt.Sprintf("/chat/groups/%s", groupID)
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
								Title       string `json:"title"`
								CreatedAt   int64  `json:"created_at"`
								UpdatedAt   int64  `json:"updated_at"`
								UserID      string `json:"user_id"`
								IsPinned    *bool  `json:"is_pinned,omitempty"`
								IsPublic    *bool  `json:"is_public,omitempty"`
								IsTemporary *bool  `json:"is_temporary,omitempty"`
							}

							if err := json.Unmarshal(respBody.Bytes(), &result); err != nil {
								return err
							}

							fmt.Printf("ID: %s\n", result.ID)
							fmt.Printf("Title: %s\n", result.Title)
							fmt.Printf("Created At: %s\n", time.UnixMilli(result.CreatedAt).Format("2006-01-02 15:04:05"))
							fmt.Printf("Updated At: %s\n", time.UnixMilli(result.UpdatedAt).Format("2006-01-02 15:04:05"))
							fmt.Printf("User ID: %s\n", result.UserID)
							if result.IsPinned != nil {
								fmt.Printf("Pinned: %t\n", *result.IsPinned)
							}
							if result.IsPublic != nil {
								fmt.Printf("Public: %t\n", *result.IsPublic)
							}
							if result.IsTemporary != nil {
								fmt.Printf("Temporary: %t\n", *result.IsTemporary)
							}
							return nil
						},
					},
					{
						Name:  "update",
						Usage: "Update a chat group",
						Flags: []cli.Flag{
							&cli.StringFlag{
								Name:     "id",
								Usage:    "Group ID",
								Required: true,
							},
							&cli.StringFlag{
								Name:     "title",
								Usage:    "New group title",
								Required: true,
							},
							&cli.BoolFlag{
								Name:  "pinned",
								Usage: "Mark group as pinned",
							},
							&cli.BoolFlag{
								Name:  "public",
								Usage: "Mark group as public",
							},
							&cli.BoolFlag{
								Name:  "temporary",
								Usage: "Mark group as temporary",
							},
						},
						Action: func(c *cli.Context) error {
							client := NewClient(c.String("base-url"), c.Int("port"), c.Int("timeout"))
							groupID := c.String("id")
							title := c.String("title")

							body := map[string]interface{}{}
							if title != "" {
								body["title"] = title
							}

							if c.IsSet("pinned") {
								body["is_pinned"] = c.Bool("pinned")
							}

							if c.IsSet("public") {
								body["is_public"] = c.Bool("public")
							}

							if c.IsSet("temporary") {
								body["is_temporary"] = c.Bool("temporary")
							}

							path := fmt.Sprintf("/chat/groups/%s", groupID)
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
							} else {
								fmt.Println("Group update request sent")
							}
							return nil
						},
					},
					{
						Name:  "delete",
						Usage: "Delete a chat group",
						Flags: []cli.Flag{
							&cli.StringFlag{
								Name:     "id",
								Usage:    "Group ID",
								Required: true,
							},
						},
						Action: func(c *cli.Context) error {
							client := NewClient(c.String("base-url"), c.Int("port"), c.Int("timeout"))
							groupID := c.String("id")

							path := fmt.Sprintf("/chat/groups/%s", groupID)
							resp, err := client.doRequest("DELETE", path, nil)
							if err != nil {
								return err
							}
							if resp.Body != nil {
								defer resp.Body.Close()
							}

							if resp.StatusCode == 200 || resp.StatusCode == 204 {
								fmt.Println("Group deleted successfully")
								return nil
							}

							respBody, err := processResponse(resp)
							if err != nil {
								return err
							}

							if c.Bool("json-output") {
								fmt.Println(respBody.String())
							} else {
								fmt.Println("Group deleted successfully")
							}
							return nil
						},
					},
					{
						Name:  "list-threads",
						Usage: "List threads in a group",
						Flags: []cli.Flag{
							&cli.StringFlag{
								Name:     "id",
								Usage:    "Group ID",
								Required: true,
							},
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
							groupID := c.String("id")
							page := c.Int("page")
							pageSize := c.Int("page-size")
							order := c.String("order")

							path := fmt.Sprintf("/chat/groups/%s/threads?page=%d&page_size=%d&order=%s",
								groupID, page, pageSize, order)
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
									CreatedAt int64  `json:"created_at"`
									UpdatedAt int64  `json:"updated_at"`
									UserID    string `json:"user_id"`
								} `json:"threads"`
								Total int `json:"total"`
							}

							if err := json.Unmarshal(respBody.Bytes(), &body); err != nil {
								return err
							}

							table := tablewriter.NewWriter(os.Stdout)
							table.SetHeader([]string{"ID", "Title", "Created At", "Updated At", "User ID"})

							for _, thread := range body.ChatThreads {
								table.Append([]string{
									thread.ID,
									thread.Title,
									time.UnixMilli(thread.CreatedAt).Format("2006-01-02 15:04:05"),
									time.UnixMilli(thread.UpdatedAt).Format("2006-01-02 15:04:05"),
									thread.UserID,
								})
							}

							table.Render()
							fmt.Printf("Total threads: %d\n", body.Total)
							return nil
						},
					},
				},
			},
			{
				Name:  "threads",
				Usage: "Chat thread management",
				Subcommands: []*cli.Command{
					{
						Name:  "list",
						Usage: "List chat threads",
						Flags: []cli.Flag{
							&cli.StringFlag{
								Name:     "group-id",
								Usage:    "Group ID (optional)",
								Required: false,
							},
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
							groupID := c.String("group-id")
							page := c.Int("page")
							pageSize := c.Int("page-size")
							order := c.String("order")

							var path string
							if groupID != "" {
								path = fmt.Sprintf("/chat/groups/%s/threads?page=%d&page_size=%d&order=%s",
									groupID, page, pageSize, order)
							} else {
								path = fmt.Sprintf("/chat/threads?page=%d&page_size=%d&order=%s",
									page, pageSize, order)
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

							var body struct {
								ChatThreads []struct {
									ID        string `json:"id"`
									Title     string `json:"title"`
									GroupID   string `json:"group_id"`
									CreatedAt int64  `json:"created_at"`
									UpdatedAt int64  `json:"updated_at"`
									UserID    string `json:"user_id"`
								} `json:"threads"`
								Total int `json:"total"`
							}

							if err := json.Unmarshal(respBody.Bytes(), &body); err != nil {
								return err
							}

							table := tablewriter.NewWriter(os.Stdout)
							table.SetHeader([]string{"ID", "Title", "Group ID", "Created At", "Updated At"})

							for _, thread := range body.ChatThreads {
								table.Append([]string{
									thread.ID,
									thread.Title,
									thread.GroupID,
									time.UnixMilli(thread.CreatedAt).Format("2006-01-02 15:04:05"),
									time.UnixMilli(thread.UpdatedAt).Format("2006-01-02 15:04:05"),
								})
							}

							table.Render()
							fmt.Printf("Total threads: %d\n", body.Total)
							return nil
						},
					},
					{
						Name:  "create",
						Usage: "Create a new chat thread",
						Flags: []cli.Flag{
							&cli.StringFlag{
								Name:     "title",
								Usage:    "Thread title",
								Required: true,
							},
							&cli.StringFlag{
								Name:  "group-id",
								Usage: "Group ID to add thread to",
							},
							&cli.BoolFlag{
								Name:  "pinned",
								Usage: "Mark thread as pinned",
							},
							&cli.BoolFlag{
								Name:  "public",
								Usage: "Mark thread as public",
							},
							&cli.BoolFlag{
								Name:  "temporary",
								Usage: "Mark thread as temporary",
							},
						},
						Action: func(c *cli.Context) error {
							client := NewClient(c.String("base-url"), c.Int("port"), c.Int("timeout"))
							title := c.String("title")
							groupID := c.String("group-id")

							body := map[string]interface{}{
								"title": title,
							}
							if groupID != "" {
								body["group_id"] = groupID
							}

							if c.IsSet("pinned") {
								body["is_pinned"] = c.Bool("pinned")
							}

							if c.IsSet("public") {
								body["is_public"] = c.Bool("public")
							}

							if c.IsSet("temporary") {
								body["is_temporary"] = c.Bool("temporary")
							}

							resp, err := client.doRequest("POST", "/chat/threads", body)
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
								Title       string `json:"title"`
								IsPinned    *bool  `json:"is_pinned,omitempty"`
								IsPublic    *bool  `json:"is_public,omitempty"`
								IsTemporary *bool  `json:"is_temporary,omitempty"`
							}

							if err := json.Unmarshal(respBody.Bytes(), &result); err != nil {
								return err
							}

							fmt.Printf("Thread created with ID: %s\n", result.ID)
							fmt.Printf("Title: %s\n", result.Title)
							if result.IsPinned != nil {
								fmt.Printf("Pinned: %t\n", *result.IsPinned)
							}
							if result.IsPublic != nil {
								fmt.Printf("Public: %t\n", *result.IsPublic)
							}
							if result.IsTemporary != nil {
								fmt.Printf("Temporary: %t\n", *result.IsTemporary)
							}
							return nil
						},
					},
					{
						Name:  "update",
						Usage: "Update a chat thread",
						Flags: []cli.Flag{
							&cli.StringFlag{
								Name:     "id",
								Usage:    "Thread ID",
								Required: true,
							},
							&cli.StringFlag{
								Name:  "title",
								Usage: "New thread title",
							},
							&cli.StringFlag{
								Name:  "group-id",
								Usage: "New group ID (use 'none' to remove from group)",
							},
							&cli.BoolFlag{
								Name:  "pinned",
								Usage: "Mark thread as pinned",
							},
							&cli.BoolFlag{
								Name:  "public",
								Usage: "Mark thread as public",
							},
							&cli.BoolFlag{
								Name:  "temporary",
								Usage: "Mark thread as temporary",
							},
						},
						Action: func(c *cli.Context) error {
							client := NewClient(c.String("base-url"), c.Int("port"), c.Int("timeout"))
							threadID := c.String("id")
							title := c.String("title")
							groupID := c.String("group-id")

							// At least one field must be provided
							if title == "" && groupID == "" && !c.IsSet("pinned") && !c.IsSet("public") && !c.IsSet("temporary") {
								return fmt.Errorf("at least one of --title, --group-id, --pinned, --public, or --temporary must be provided")
							}

							body := map[string]interface{}{}

							if title != "" {
								body["title"] = title
							}

							if groupID != "" {
								if groupID == "none" {
									body["group_id"] = ""
								} else {
									body["group_id"] = groupID
								}
							}

							if c.IsSet("pinned") {
								body["is_pinned"] = c.Bool("pinned")
							}

							if c.IsSet("public") {
								body["is_public"] = c.Bool("public")
							}

							if c.IsSet("temporary") {
								body["is_temporary"] = c.Bool("temporary")
							}

							path := fmt.Sprintf("/chat/threads/%s", threadID)
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
								ID          string `json:"id"`
								Title       string `json:"title"`
								GroupID     string `json:"group_id"`
								UpdatedAt   int64  `json:"updated_at"`
								IsPinned    *bool  `json:"is_pinned,omitempty"`
								IsPublic    *bool  `json:"is_public,omitempty"`
								IsTemporary *bool  `json:"is_temporary,omitempty"`
							}

							if err := json.Unmarshal(respBody.Bytes(), &result); err != nil {
								return err
							}

							fmt.Printf("Thread updated successfully:\n")
							fmt.Printf("ID: %s\n", result.ID)
							fmt.Printf("Title: %s\n", result.Title)
							if result.GroupID != "" {
								fmt.Printf("Group ID: %s\n", result.GroupID)
							} else {
								fmt.Printf("Group ID: none\n")
							}
							fmt.Printf("Updated At: %s\n", time.UnixMilli(result.UpdatedAt).Format("2006-01-02 15:04:05"))
							return nil
						},
					},
					{
						Name:  "get",
						Usage: "Get chat thread details",
						Flags: []cli.Flag{
							&cli.StringFlag{
								Name:     "id",
								Usage:    "Thread ID",
								Required: true,
							},
						},
						Action: func(c *cli.Context) error {
							client := NewClient(c.String("base-url"), c.Int("port"), c.Int("timeout"))
							threadID := c.String("id")

							path := fmt.Sprintf("/chat/threads/%s", threadID)
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
								Title       string `json:"title"`
								Description string `json:"description"`
								GroupID     string `json:"group_id"`
								CreatedAt   int64  `json:"created_at"`
								UpdatedAt   int64  `json:"updated_at"`
								UserID      string `json:"user_id"`
							}

							if err := json.Unmarshal(respBody.Bytes(), &result); err != nil {
								return err
							}

							fmt.Printf("ID: %s\n", result.ID)
							fmt.Printf("Title: %s\n", result.Title)
							fmt.Printf("Description: %s\n", result.Description)
							fmt.Printf("Group ID: %s\n", result.GroupID)
							fmt.Printf("Created At: %s\n", time.UnixMilli(result.CreatedAt).Format("2006-01-02 15:04:05"))
							fmt.Printf("Updated At: %s\n", time.UnixMilli(result.UpdatedAt).Format("2006-01-02 15:04:05"))
							fmt.Printf("User ID: %s\n", result.UserID)
							return nil
						},
					},
					{
						Name:  "delete",
						Usage: "Delete a chat thread",
						Flags: []cli.Flag{
							&cli.StringFlag{
								Name:     "id",
								Usage:    "Thread ID",
								Required: true,
							},
						},
						Action: func(c *cli.Context) error {
							client := NewClient(c.String("base-url"), c.Int("port"), c.Int("timeout"))
							threadID := c.String("id")

							path := fmt.Sprintf("/chat/threads/%s", threadID)
							resp, err := client.doRequest("DELETE", path, nil)
							if err != nil {
								return err
							}
							if resp.Body != nil {
								defer resp.Body.Close()
							}

							if resp.StatusCode == 200 || resp.StatusCode == 204 {
								fmt.Println("Thread deleted successfully")
								return nil
							}

							respBody, err := processResponse(resp)
							if err != nil {
								return err
							}

							if c.Bool("json-output") {
								fmt.Println(respBody.String())
							} else {
								fmt.Println("Thread deleted successfully")
							}
							return nil
						},
					},
					{
						Name:  "list-messages",
						Usage: "List messages in a thread",
						Flags: []cli.Flag{
							&cli.StringFlag{
								Name:     "id",
								Usage:    "Thread ID",
								Required: true,
							},
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
								Value: "-created_at",
							},
						},
						Action: func(c *cli.Context) error {
							client := NewClient(c.String("base-url"), c.Int("port"), c.Int("timeout"))
							threadID := c.String("id")
							page := c.Int("page")
							pageSize := c.Int("page-size")
							order := c.String("order")

							path := fmt.Sprintf("/chat/threads/%s/messages?page=%d&page_size=%d&order=%s",
								threadID, page, pageSize, order)
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
								Messages []struct {
									ID          string  `json:"id"`
									ThreadID    string  `json:"thread_id"`
									Role        string  `json:"role"`
									Content     string  `json:"content"`
									CreatedAtMs int64   `json:"created_at"`
									UpdatedAtMs int64   `json:"updated_at"`
									ModelName   string  `json:"model_name"`
									Temperature float32 `json:"temperature"`
									UserID      string  `json:"user_id"`
								} `json:"messages"`
								Total int `json:"total"`
							}

							if err := json.Unmarshal(respBody.Bytes(), &body); err != nil {
								return err
							}

							table := tablewriter.NewWriter(os.Stdout)
							table.SetHeader([]string{"Message ID", "Created At", "Model", "Temp", "Role", "Content"})

							for _, message := range body.Messages {
								table.Append([]string{
									message.ID,
									time.UnixMilli(message.CreatedAtMs).Format("2006-01-02 15:04:05.123"),
									message.ModelName,
									fmt.Sprintf("%.1f", message.Temperature),
									message.Role,
									message.Content,
								})
							}

							table.Render()
							fmt.Printf("Total messages: %d\n", body.Total)

							return nil
						},
					},
				},
			},
			{
				Name:  "messages",
				Usage: "Chat message management",
				Subcommands: []*cli.Command{
					{
						Name:  "send",
						Usage: "Send a message to a thread",
						Flags: []cli.Flag{
							&cli.StringFlag{
								Name:     "thread-id",
								Usage:    "Thread ID",
								Required: true,
							},
							&cli.StringFlag{
								Name:     "content",
								Usage:    "Message content",
								Required: true,
							},
							&cli.StringFlag{
								Name:     "model",
								Usage:    "Model to use for assistant response",
								Value:    "",
								Required: true,
							},
							&cli.Float64Flag{
								Name:     "temperature",
								Usage:    "Temperature for generation",
								Value:    0.5,
								Required: false,
							},
							&cli.BoolFlag{
								Name:    "stream",
								Usage:   "Stream the response",
								Value:   false,
								Aliases: []string{"s"},
							},
							&cli.IntFlag{
								Name:    "max-tokens",
								Usage:   "Max tokens",
								Value:   2048,
								Aliases: []string{"mt"},
							},
						},
						Action: func(c *cli.Context) error {
							client := NewClient(c.String("base-url"), c.Int("port"), c.Int("timeout"))
							threadID := c.String("thread-id")
							content := c.String("content")
							model := c.String("model")
							temperature := c.Float64("temperature")
							stream := c.Bool("stream")
							maxTokens := c.Int("max-tokens")

							body := map[string]interface{}{
								"content":     content,
								"model_name":  model,
								"temperature": temperature,
								"stream":      stream,
								"max_tokens":  maxTokens,
							}

							path := fmt.Sprintf("/chat/threads/%s/messages", threadID)
							resp, err := client.doRequest("POST", path, body)
							if err != nil {
								return err
							}
							if resp.Body != nil {
								defer resp.Body.Close()
							}

							if stream {
								// TODO: stream the response
								return nil
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
								ID          string  `json:"id"`
								ThreadID    string  `json:"thread_id"`
								CreatedAtMs int64   `json:"created_at"`
								UpdatedAtMs int64   `json:"updated_at"`
								ModelName   string  `json:"model_name"`
								Temperature float32 `json:"temperature"`
								Content     string  `json:"content"`
								Role        string  `json:"role"`
							}

							if err := json.Unmarshal(respBody.Bytes(), &result); err != nil {
								return err
							}

							fmt.Printf("Message sent with ID: %s\n", result.ID)
							fmt.Printf("Thread ID: %s\n", result.ThreadID)
							fmt.Printf("Model: %s\n", result.ModelName)
							fmt.Printf("Temperature: %.1f\n", result.Temperature)
							fmt.Printf("Created At: %s\n", time.UnixMilli(result.CreatedAtMs).Format("2006-01-02 15:04:05.123"))
							fmt.Printf("Updated At: %s\n", time.UnixMilli(result.UpdatedAtMs).Format("2006-01-02 15:04:05.123"))
							fmt.Printf("Role: %s\n", result.Role)
							fmt.Printf("Content: %s\n", result.Content)
							return nil
						},
					},
					{
						Name:  "update-and-reset-tail",
						Usage: "Update and reset the chat tail",
						Flags: []cli.Flag{
							&cli.StringFlag{
								Name:     "thread-id",
								Usage:    "Thread ID",
								Required: true,
							},
							&cli.StringFlag{
								Name:     "content",
								Usage:    "Message content",
								Required: true,
							},
							&cli.StringFlag{
								Name:     "model",
								Usage:    "Model to use for assistant response",
								Value:    "",
								Required: true,
							},
							&cli.Float64Flag{
								Name:     "temperature",
								Usage:    "Temperature for generation",
								Value:    0.5,
								Required: false,
							},
							&cli.BoolFlag{
								Name:    "stream",
								Usage:   "Stream the response",
								Value:   false,
								Aliases: []string{"s"},
							},
							&cli.IntFlag{
								Name:    "max-tokens",
								Usage:   "Max tokens",
								Value:   2048,
								Aliases: []string{"mt"},
							},
							&cli.StringFlag{
								Name:     "message-id",
								Usage:    "Message ID",
								Required: true,
							},
						},
						Action: func(c *cli.Context) error {
							client := NewClient(c.String("base-url"), c.Int("port"), c.Int("timeout"))
							threadID := c.String("thread-id")
							content := c.String("content")
							model := c.String("model")
							temperature := c.Float64("temperature")
							stream := c.Bool("stream")
							maxTokens := c.Int("max-tokens")
							messageID := c.String("message-id")

							body := map[string]interface{}{
								"content":     content,
								"model_name":  model,
								"temperature": temperature,
								"stream":      stream,
								"max_tokens":  maxTokens,
							}

							path := fmt.Sprintf("/chat/threads/%s/messages/?replace_msg_id=%s", threadID, messageID)
							resp, err := client.doRequest("POST", path, body)
							if err != nil {
								return err
							}
							if resp.Body != nil {
								defer resp.Body.Close()
							}

							if stream {
								// TODO: stream the response
								return nil
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
								ID          string  `json:"id"`
								ThreadID    string  `json:"thread_id"`
								CreatedAtMs int64   `json:"created_at"`
								UpdatedAtMs int64   `json:"updated_at"`
								ModelName   string  `json:"model_name"`
								Temperature float32 `json:"temperature"`
								Content     string  `json:"content"`
								Role        string  `json:"role"`
							}

							if err := json.Unmarshal(respBody.Bytes(), &result); err != nil {
								return err
							}

							fmt.Printf("Message sent with ID: %s\n", result.ID)
							fmt.Printf("Thread ID: %s\n", result.ThreadID)
							fmt.Printf("Model: %s\n", result.ModelName)
							fmt.Printf("Temperature: %.1f\n", result.Temperature)
							fmt.Printf("Created At: %s\n", time.UnixMilli(result.CreatedAtMs).Format("2006-01-02 15:04:05.123"))
							fmt.Printf("Updated At: %s\n", time.UnixMilli(result.UpdatedAtMs).Format("2006-01-02 15:04:05.123"))
							fmt.Printf("Role: %s\n", result.Role)
							fmt.Printf("Content: %s\n", result.Content)
							return nil
						},
					},
					{
						Name:  "create",
						Usage: "Create a new message and start a new thread",
						Flags: []cli.Flag{
							&cli.StringFlag{
								Name:     "content",
								Usage:    "Message content",
								Required: true,
							},
							&cli.StringFlag{
								Name:    "model",
								Usage:   "Model name to use",
								Value:   "qwen2.5",
								Aliases: []string{"model-name"},
							},
							&cli.StringFlag{
								Name:    "temperature",
								Usage:   "Temperature for generation",
								Value:   "0.5",
								Aliases: []string{"temp"},
							},
							&cli.BoolFlag{
								Name:    "stream",
								Usage:   "Stream the response",
								Value:   false,
								Aliases: []string{"s"},
							},
							&cli.IntFlag{
								Name:  "max-tokens",
								Usage: "Max tokens",
								Value: 2048,
							},
						},
						Action: func(c *cli.Context) error {
							client := NewClient(c.String("base-url"), c.Int("port"), c.Int("timeout"))
							content := c.String("content")
							model := c.String("model")
							temperature := c.Float64("temperature")
							stream := c.Bool("stream")
							maxTokens := c.Int("max-tokens")
							body := map[string]interface{}{
								"content":     content,
								"model_name":  model,
								"temperature": temperature,
								"stream":      stream,
								"max_tokens":  maxTokens,
							}

							resp, err := client.doRequest("POST", "/chat/messages", body)
							if err != nil {
								return err
							}
							if resp.Body != nil {
								defer resp.Body.Close()
							}

							if stream {
								// TODO: stream the response
								return nil
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
								ID          string  `json:"id"`
								ThreadID    string  `json:"thread_id"`
								CreatedAtMs int64   `json:"created_at"`
								UpdatedAtMs int64   `json:"updated_at"`
								ModelName   string  `json:"model_name"`
								Temperature float32 `json:"temperature"`
								Content     string  `json:"content"`
								Role        string  `json:"role"`
							}

							if err := json.Unmarshal(respBody.Bytes(), &result); err != nil {
								return err
							}

							fmt.Printf("Message sent with ID: %s\n", result.ID)
							fmt.Printf("Thread ID: %s\n", result.ThreadID)
							fmt.Printf("Model: %s\n", result.ModelName)
							fmt.Printf("Temperature: %.1f\n", result.Temperature)
							fmt.Printf("Created At: %s\n", time.UnixMilli(result.CreatedAtMs).Format("2006-01-02 15:04:05.123"))
							fmt.Printf("Updated At: %s\n", time.UnixMilli(result.UpdatedAtMs).Format("2006-01-02 15:04:05.123"))
							fmt.Printf("Role: %s\n", result.Role)
							fmt.Printf("Content: %s\n", result.Content)
							return nil
						},
					},
					{
						Name:  "get",
						Usage: "Get a specific message",
						Flags: []cli.Flag{
							&cli.StringFlag{
								Name:     "id",
								Usage:    "Message ID",
								Required: true,
							},
						},
						Action: func(c *cli.Context) error {
							client := NewClient(c.String("base-url"), c.Int("port"), c.Int("timeout"))
							messageID := c.String("id")

							path := fmt.Sprintf("/chat/messages/%s", messageID)
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
								ThreadID    string `json:"thread_id"`
								Role        string `json:"role"`
								Content     string `json:"content"`
								CreatedAtMs int64  `json:"created_at"`
								UpdatedAtMs int64  `json:"updated_at"`
								UserID      string `json:"user_id"`
							}

							if err := json.Unmarshal(respBody.Bytes(), &result); err != nil {
								return err
							}

							fmt.Printf("ID: %s\n", result.ID)
							fmt.Printf("Thread ID: %s\n", result.ThreadID)
							fmt.Printf("Role: %s\n", result.Role)
							fmt.Printf("Created At: %s\n", time.UnixMilli(result.CreatedAtMs).Format("2006-01-02 15:04:05.123"))
							fmt.Printf("Updated At: %s\n", time.UnixMilli(result.UpdatedAtMs).Format("2006-01-02 15:04:05.123"))
							fmt.Printf("User ID: %s\n", result.UserID)
							fmt.Println("\nContent:")
							fmt.Println(result.Content)
							return nil
						},
					},
					{
						Name:  "update",
						Usage: "Update a message",
						Flags: []cli.Flag{
							&cli.StringFlag{
								Name:     "id",
								Usage:    "Message ID",
								Required: true,
							},
							&cli.StringFlag{
								Name:     "content",
								Usage:    "New message content",
								Required: true,
							},
						},
						Action: func(c *cli.Context) error {
							client := NewClient(c.String("base-url"), c.Int("port"), c.Int("timeout"))
							messageID := c.String("id")
							content := c.String("content")

							body := map[string]interface{}{
								"content": content,
							}

							path := fmt.Sprintf("/chat/messages/%s", messageID)
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
								ID          string  `json:"id"`
								ThreadID    string  `json:"thread_id"`
								CreatedAtMs int64   `json:"created_at"`
								UpdatedAtMs int64   `json:"updated_at"`
								ModelName   string  `json:"model_name"`
								Temperature float32 `json:"temperature"`
								Content     string  `json:"content"`
								Role        string  `json:"role"`
							}

							if err := json.Unmarshal(respBody.Bytes(), &result); err != nil {
								return err
							}

							fmt.Printf("Message updated with ID: %s\n", result.ID)
							fmt.Printf("Thread ID: %s\n", result.ThreadID)
							fmt.Printf("Model: %s\n", result.ModelName)
							fmt.Printf("Temperature: %.1f\n", result.Temperature)
							fmt.Printf("Created At: %s\n", time.UnixMilli(result.CreatedAtMs).Format("2006-01-02 15:04:05.123"))
							fmt.Printf("Updated At: %s\n", time.UnixMilli(result.UpdatedAtMs).Format("2006-01-02 15:04:05.123"))
							fmt.Printf("Role: %s\n", result.Role)
							fmt.Printf("Content: %s\n", result.Content)
							return nil
						},
					},
					{
						Name:  "delete",
						Usage: "Delete a message",
						Flags: []cli.Flag{
							&cli.StringFlag{
								Name:     "id",
								Usage:    "Message ID",
								Required: true,
							},
						},
						Action: func(c *cli.Context) error {
							client := NewClient(c.String("base-url"), c.Int("port"), c.Int("timeout"))
							messageID := c.String("id")

							path := fmt.Sprintf("/chat/messages/%s", messageID)
							resp, err := client.doRequest("DELETE", path, nil)
							if err != nil {
								return err
							}
							if resp.Body != nil {
								defer resp.Body.Close()
							}

							if resp.StatusCode == 200 || resp.StatusCode == 204 {
								fmt.Println("Message deleted successfully")
								return nil
							}

							respBody, err := processResponse(resp)
							if err != nil {
								return err
							}

							if c.Bool("json-output") {
								fmt.Println(respBody.String())
								return nil
							}

							return nil
						},
					},
				},
			},
		},
	}
}
