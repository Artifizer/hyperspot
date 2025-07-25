package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"os"
	"path/filepath"

	"github.com/google/uuid"
	"github.com/urfave/cli/v2"
)

// getChatAttachmentCommands creates the chat attachment command group
func getChatAttachmentCommands() *cli.Command {
	return &cli.Command{
		Name:  "attachment",
		Usage: "Chat attachment operations",
		Subcommands: []*cli.Command{
			{
				Name:  "attach-to-thread",
				Usage: "Attach a file to an existing chat thread",
				Flags: []cli.Flag{
					&cli.StringFlag{
						Name:     "thread-id",
						Aliases:  []string{"t"},
						Usage:    "Chat thread ID",
						Required: true,
					},
					&cli.StringFlag{
						Name:     "file",
						Aliases:  []string{"f"},
						Usage:    "Path to file to attach",
						Required: true,
					},
				},
				Action: func(c *cli.Context) error {
					client := NewClient(c.String("base-url"), c.Int("port"), c.Int("timeout"))

					// Convert to absolute path
					absPath, err := filepath.Abs(c.String("file"))
					if err != nil {
						return fmt.Errorf("failed to resolve file path: %v", err)
					}

					// Check if file exists
					if _, err := os.Stat(absPath); os.IsNotExist(err) {
						return fmt.Errorf("file not found: %s", absPath)
					}

					// Upload the file
					result, err := uploadFileToThread(client, c.String("thread-id"), absPath)
					if err != nil {
						return err
					}

					if c.Bool("json-output") {
						jsonBytes, _ := json.MarshalIndent(result, "", "  ")
						fmt.Println(string(jsonBytes))
					} else {
						fmt.Printf("OK, the file attached successfully to thread %s\n", c.String("thread-id"))
						fmt.Printf("Message ID: %s\n", result.ID.String())
						fmt.Printf("File: %s\n", filepath.Base(absPath))
						fmt.Printf("Original Size: %d bytes\n", result.OriginalFileSize)
						fmt.Printf("Full Content Length: %d bytes\n", result.FullContentLength)
						if result.IsTruncated {
							fmt.Printf("Truncated Content Length: %d bytes\n", result.TruncatedContentLength)
						}
					}

					return nil
				},
			},
			{
				Name:  "create-with-attachment",
				Usage: "Create a new chat thread with a file attachment",
				Flags: []cli.Flag{
					&cli.StringFlag{
						Name:    "group-id",
						Aliases: []string{"g"},
						Usage:   "Chat group ID (optional)",
					},
					&cli.StringFlag{
						Name:     "file",
						Aliases:  []string{"f"},
						Usage:    "Path to file to attach",
						Required: true,
					},
				},
				Action: func(c *cli.Context) error {
					client := NewClient(c.String("base-url"), c.Int("port"), c.Int("timeout"))

					// Convert to absolute path
					absPath, err := filepath.Abs(c.String("file"))
					if err != nil {
						return fmt.Errorf("failed to resolve file path: %v", err)
					}

					// Check if file exists
					if _, err := os.Stat(absPath); os.IsNotExist(err) {
						return fmt.Errorf("file not found: %s", absPath)
					}

					// Create new thread with attachment
					result, err := createThreadWithAttachment(client, c.String("group-id"), absPath)
					if err != nil {
						return err
					}

					if c.Bool("json-output") {
						jsonBytes, _ := json.MarshalIndent(result, "", "  ")
						fmt.Println(string(jsonBytes))
					} else {
						fmt.Printf("OK, a new thread created with attachment\n")
						fmt.Printf("Thread ID: %s\n", result.ThreadID.String())
						fmt.Printf("Message ID: %s\n", result.ID.String())
						fmt.Printf("File: %s\n", filepath.Base(absPath))
						fmt.Printf("Original Size: %d bytes\n", result.OriginalFileSize)
						fmt.Printf("Full Content Length: %d bytes\n", result.FullContentLength)
						if result.IsTruncated {
							fmt.Printf("Truncated Content Length: %d bytes\n", result.TruncatedContentLength)
						}
					}

					return nil
				},
			},
		},
	}
}

// ChatAttachmentResponse represents the response from attachment API
type ChatAttachmentResponse struct {
	ID                     uuid.UUID `json:"id"`
	UserID                 uuid.UUID `json:"user_id"`
	ThreadID               uuid.UUID `json:"thread_id"`
	Role                   string    `json:"role"`
	ModelName              string    `json:"model_name"`
	MaxTokens              int       `json:"max_tokens"`
	Temperature            float32   `json:"temperature"`
	CreatedAt              int64     `json:"created_at"`
	UpdatedAt              int64     `json:"updated_at"`
	Content                string    `json:"content"`
	Error                  string    `json:"error"`
	Like                   int       `json:"like"`
	Completed              bool      `json:"completed"`
	SizeChars              int       `json:"size_chars"`
	SizeTokens             int       `json:"size_tokens"`
	TokensPerSec           float32   `json:"tokens_per_sec"`
	CharsPerSec            float32   `json:"chars_per_sec"`
	FinishReason           string    `json:"finish_reason"`
	IsFileAttachment       bool      `json:"is_file_attachment,omitempty"`
	OriginalFileSize       int64     `json:"original_file_size,omitempty"`
	FullContentLength      int64     `json:"full_content_length,omitempty"`
	TruncatedContentLength int64     `json:"truncated_content_length,omitempty"`
	IsTruncated            bool      `json:"is_truncated,omitempty"`
	AttachedFileName       string    `json:"attached_file_name,omitempty"`
	AttachedFileExtension  string    `json:"attached_file_extension,omitempty"`
}

// uploadFileToThread uploads a file to an existing chat thread
func uploadFileToThread(client *Client, threadID string, filePath string) (*ChatAttachmentResponse, error) {
	// Open the file
	file, err := os.Open(filePath)
	if err != nil {
		return nil, fmt.Errorf("failed to open file: %v", err)
	}
	defer file.Close()

	// Create multipart form data
	var buf bytes.Buffer
	writer := multipart.NewWriter(&buf)

	// Create form file field
	filename := filepath.Base(filePath)
	part, err := writer.CreateFormFile("file", filename)
	if err != nil {
		return nil, fmt.Errorf("failed to create form file: %v", err)
	}

	// Copy file content to form
	_, err = io.Copy(part, file)
	if err != nil {
		return nil, fmt.Errorf("failed to copy file content: %v", err)
	}

	// Close the writer to finalize the form
	err = writer.Close()
	if err != nil {
		return nil, fmt.Errorf("failed to close multipart writer: %v", err)
	}

	// Create HTTP request
	url := fmt.Sprintf("/chat/threads/%s/attachment", threadID)
	req, err := http.NewRequest("POST", client.client.GetBaseURL()+url, &buf)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %v", err)
	}

	// Set headers
	req.Header.Set("Content-Type", writer.FormDataContentType())
	// Note: API key is handled per request in this client architecture

	// Send request
	httpClient := &http.Client{}
	resp, err := httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to send request: %v", err)
	}
	defer resp.Body.Close()

	// Process response
	respBody, err := processResponse(resp)
	if err != nil {
		return nil, err
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("API error: status %d, body: %s", resp.StatusCode, respBody.String())
	}

	// Parse response
	var result ChatAttachmentResponse
	if err := json.Unmarshal(respBody.Bytes(), &result); err != nil {
		return nil, fmt.Errorf("failed to parse response: %v", err)
	}

	return &result, nil
}

// createThreadWithAttachment creates a new thread with a file attachment
func createThreadWithAttachment(client *Client, groupID string, filePath string) (*ChatAttachmentResponse, error) {
	// Open the file
	file, err := os.Open(filePath)
	if err != nil {
		return nil, fmt.Errorf("failed to open file: %v", err)
	}
	defer file.Close()

	// Create multipart form data
	var buf bytes.Buffer
	writer := multipart.NewWriter(&buf)

	// Create form file field
	filename := filepath.Base(filePath)
	part, err := writer.CreateFormFile("file", filename)
	if err != nil {
		return nil, fmt.Errorf("failed to create form file: %v", err)
	}

	// Copy file content to form
	_, err = io.Copy(part, file)
	if err != nil {
		return nil, fmt.Errorf("failed to copy file content: %v", err)
	}

	// Close the writer to finalize the form
	err = writer.Close()
	if err != nil {
		return nil, fmt.Errorf("failed to close multipart writer: %v", err)
	}

	// Create HTTP request
	url := "/chat/attachments"
	if groupID != "" {
		url += "?group_id=" + groupID
	}

	req, err := http.NewRequest("POST", client.client.GetBaseURL()+url, &buf)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %v", err)
	}

	// Set headers
	req.Header.Set("Content-Type", writer.FormDataContentType())
	// Note: API key is handled per request in this client architecture

	// Send request
	httpClient := &http.Client{}
	resp, err := httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to send request: %v", err)
	}
	defer resp.Body.Close()

	// Process response
	respBody, err := processResponse(resp)
	if err != nil {
		return nil, err
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("API error: status %d, body: %s", resp.StatusCode, respBody.String())
	}

	// Parse response
	var result ChatAttachmentResponse
	if err := json.Unmarshal(respBody.Bytes(), &result); err != nil {
		return nil, fmt.Errorf("failed to parse response: %v", err)
	}

	return &result, nil
}
