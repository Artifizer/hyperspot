package main

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/hypernetix/hyperspot/libs/logging"
	"github.com/hypernetix/hyperspot/modules/llm"
	"github.com/olekukonko/tablewriter"
	"github.com/urfave/cli/v2"
)

type LLMServiceCapabilities struct {
	Streaming         bool `json:"streaming"`
	Embedding         bool `json:"embedding"`
	InstallModel      bool `json:"install_model"`
	ImportModel       bool `json:"import_model"`
	UpdateModel       bool `json:"update_model"`
	LoadModel         bool `json:"load_model"`
	LoadModelProgress bool `json:"load_model_progress"`
	LoadModelCancel   bool `json:"load_model_cancel"`
	LoadModelTimeout  bool `json:"load_model_timeout"`
	UnloadModel       bool `json:"unload_model"`
	DeleteModel       bool `json:"delete_model"`
}

func getLLMCommands() *cli.Command {
	return &cli.Command{
		Name:  "llm",
		Usage: "LLM service operations",
		Subcommands: []*cli.Command{
			{
				Name:  "list-services",
				Usage: "List available LLM services",
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
					path := fmt.Sprintf("/llm/services?page_size=%d&page=%d", pageSize, page)
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

					type LLMServiceDetails struct {
						Name         string                 `json:"name"`
						Capabilities LLMServiceCapabilities `json:"capabilities"`
					}

					body := struct {
						Services []LLMServiceDetails `json:"services"`
					}{}

					if err := json.Unmarshal(respBody.Bytes(), &body); err != nil {
						return err
					}

					table := tablewriter.NewWriter(os.Stdout)
					table.SetHeader([]string{"#", "Name", "Streaming", "Embedding", "Load/Unload models", "Install/remove models"})

					for n, service := range body.Services {
						table.Append([]string{fmt.Sprintf("%d", n+1),
							service.Name,
							fmt.Sprintf("%t", service.Capabilities.Streaming),
							fmt.Sprintf("%t", service.Capabilities.Embedding),
							fmt.Sprintf("%t", service.Capabilities.LoadModel),
							fmt.Sprintf("%t", service.Capabilities.InstallModel)})
					}

					table.Render()
					return nil
				},
			},
			{
				Name:  "list-models",
				Usage: "List available LLM models for a service",
				Flags: []cli.Flag{
					&cli.StringFlag{
						Name:     "service",
						Usage:    "Service name",
						Required: false,
					},
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
					service := c.String("service")
					pageSize := c.Int("page_size")
					page := c.Int("page")

					var path string

					if service == "" {
						path = fmt.Sprintf("/llm/models?page_size=%d&page=%d", pageSize, page)
					} else {
						path = fmt.Sprintf("/llm/services/%s/models?page_size=%d&page=%d", service, pageSize, page)
					}

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

					body := struct {
						Models []struct {
							Name        string `json:"name"`
							Description string `json:"description"`
							Size        int64  `json:"size"`
							State       string `json:"state"`
							Type        string `json:"type"`
						} `json:"models"`
					}{}

					if err := json.Unmarshal(respBody.Bytes(), &body); err != nil {
						return err
					}

					table := tablewriter.NewWriter(os.Stdout)
					table.SetHeader([]string{"Name", "Type", "MODEL SIZE (GB)", "State", "Description"})

					for _, model := range body.Models {
						// Convert size from bytes to GB with 2 decimal places
						sizeGB := float64(model.Size) / (1024 * 1024 * 1024)
						table.Append([]string{
							model.Name,
							model.Type,
							fmt.Sprintf("%.2f", sizeGB),
							model.State,
							model.Description,
						})
					}

					table.Render()
					return nil
				},
			},
			{
				Name:  "ping",
				Usage: "Ping a service",
				Flags: []cli.Flag{
					&cli.StringFlag{
						Name:     "service",
						Usage:    "Service name",
						Required: true,
					},
				},
				Action: func(c *cli.Context) error {
					client := NewClient(c.String("base-url"), c.Int("port"), c.Int("timeout"))
					path := fmt.Sprintf("/llm/services/%s/ping", c.String("service"))
					resp, err := client.doRequest("GET", path, nil)
					if err != nil {
						return err
					}
					if resp.Body != nil {
						defer resp.Body.Close()
					}

					_, err = processResponse(resp)
					if err != nil {
						return err
					}

					fmt.Printf("Service %s is alive\n", c.String("service"))
					return nil
				},
			},
			{
				Name:  "capabilities",
				Usage: "Get capabilities of a service",
				Flags: []cli.Flag{
					&cli.StringFlag{
						Name:     "service",
						Usage:    "Service name",
						Required: true,
					},
				},
				Action: func(c *cli.Context) error {
					client := NewClient(c.String("base-url"), c.Int("port"), c.Int("timeout"))
					path := fmt.Sprintf("/llm/services/%s/capabilities", c.String("service"))
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

					var capabilities LLMServiceCapabilities

					if err := json.Unmarshal(respBody.Bytes(), &capabilities); err != nil {
						return fmt.Errorf("error parsing capabilities info: %v", err)
					}

					// Display capabilities information in a nice format
					table := tablewriter.NewWriter(os.Stdout)
					table.SetHeader([]string{"Capability", "Supported"})

					table.Append([]string{"Load Model", fmt.Sprintf("%t", capabilities.LoadModel)})
					table.Append([]string{"- Load Model Progress", fmt.Sprintf("%t", capabilities.LoadModelProgress)})
					table.Append([]string{"- Load Model Cancel", fmt.Sprintf("%t", capabilities.LoadModelCancel)})
					table.Append([]string{"- Load Model Timeout", fmt.Sprintf("%t", capabilities.LoadModelTimeout)})
					table.Append([]string{"Unload Model", fmt.Sprintf("%t", capabilities.UnloadModel)})
					table.Append([]string{"Install Model", fmt.Sprintf("%t", capabilities.InstallModel)})
					table.Append([]string{"Delete Model", fmt.Sprintf("%t", capabilities.DeleteModel)})
					table.Append([]string{"Update Model", fmt.Sprintf("%t", capabilities.UpdateModel)})
					table.Append([]string{"Import Model", fmt.Sprintf("%t", capabilities.ImportModel)})

					table.Render()
					return nil
				},
			},
			{
				Name:  "prompt",
				Usage: "Send a single prompt to the LLM",
				Flags: []cli.Flag{
					&cli.StringFlag{
						Name:     "prompt",
						Aliases:  []string{"p"},
						Usage:    "Prompt message",
						Required: true,
					},
					&cli.StringFlag{
						Name:    "model",
						Aliases: []string{"m"},
						Usage:   "Model name",
						Value:   "qwen2.5",
					},
					&cli.Float64Flag{
						Name:    "temperature",
						Aliases: []string{"t"},
						Usage:   "Temperature (0.0-2.0)",
						Value:   0.2,
					},
					&cli.IntFlag{
						Name:    "max-tokens",
						Aliases: []string{"n"},
						Usage:   "Max completion tokens",
						Value:   2048,
					},
					&cli.BoolFlag{
						Name:    "stream",
						Aliases: []string{"s"},
						Usage:   "Enable streaming mode for the response",
						Value:   false,
					},
				},
				Action: func(c *cli.Context) error {
					client := NewClient(c.String("base-url"), c.Int("port"), c.Int("timeout"))
					req := map[string]interface{}{
						"model": c.String("model"),
						"messages": []map[string]string{
							{"role": "user", "content": c.String("prompt")},
						},
						"temperature": c.Float64("temperature"),
						"max_tokens":  c.Int("max-tokens"),
						"stream":      c.Bool("stream"),
					}

					t := time.Now()
					path := "/v1/chat/completions"

					// Handle streaming and non-streaming differently
					if c.Bool("stream") {
						// Streaming mode
						resp, err := client.doRequest("POST", path, req)
						if err != nil {
							return fmt.Errorf("error requesting POST %s json: %v", path, err)
						}
						if resp.Body == nil {
							return fmt.Errorf("no response body")
						}
						defer resp.Body.Close()

						if resp.StatusCode != http.StatusOK {
							bodyBytes, err := io.ReadAll(resp.Body)
							if err != nil {
								return fmt.Errorf("API error: status %d, unable to read body: %v", resp.StatusCode, err)
							}
							return fmt.Errorf("API error: status %d, body: %s", resp.StatusCode, string(bodyBytes))
						}

						fmt.Printf("\nUser: %s\n", c.String("prompt"))

						var assistantPrinted = false
						var modelName string
						var fullResponse strings.Builder

						reader := bufio.NewReader(resp.Body)

						isEOF := false
						isStreaming := true

						for {
							if isEOF {
								break
							}

							line, err := reader.ReadString('\n')
							if err != nil {
								if err == io.EOF {
									isEOF = true
								} else {
									return err
								}
							}

							logging.Debug("API response line: %s", line)

							if line == "\n" {
								continue
							}

							if strings.HasPrefix(line, "data: [DONE]") {
								break
							}

							if strings.HasPrefix(line, "{\"choices\":") {
								isStreaming = false
							}

							if strings.HasPrefix(line, "data: ") || strings.HasPrefix(line, "{\"choices\":") {
								data := strings.TrimPrefix(line, "data: ")

								if c.Bool("json-output") {
									fmt.Println(data)
									continue
								}

								var chunk struct {
									Model   string `json:"model"`
									Choices []struct {
										Delta struct { // used by LLMs that support streaming
											Content string `json:"content"`
										} `json:"delta"`
										Message struct { // alternative to delta, used by LLMs that do not support streaming
											Content string `json:"content"`
										} `json:"message"`
									} `json:"choices"`
								}

								if err := json.Unmarshal([]byte(data), &chunk); err != nil {
									continue
								}

								if len(chunk.Choices) > 0 {
									content := chunk.Choices[0].Delta.Content
									if content == "" {
										content = chunk.Choices[0].Message.Content
										if content != "" {
											isStreaming = false
										}
									}
									if modelName == "" && chunk.Model != "" {
										modelName = chunk.Model
									}

									w := bufio.NewWriter(os.Stdout)
									if len(content) > 0 {
										if !assistantPrinted {
											fmt.Printf("\nAssistant (%s):\n\n", modelName)
											assistantPrinted = true
										}
										if _, err := w.WriteString(content); err != nil {
											fmt.Fprintf(os.Stderr, "Failed to write content: %v\n", err)
										}
										if err := w.Flush(); err != nil {
											fmt.Fprintf(os.Stderr, "Failed to flush writer: %v\n", err)
										}
										fullResponse.WriteString(content)
									}
								}
							}
						}

						fmt.Printf("\n\n")
						elapsed := time.Since(t).Seconds()
						fmt.Printf("Time taken: %.1f sec\n", elapsed)
						if !isStreaming {
							fmt.Printf("\nWARNING: model replied in a non-streaming mode despite of the 'stream' flag\n")
						}
						return nil
					} else {
						// Non-streaming mode
						resp, err := client.doRequest("POST", path, req)
						if err != nil {
							return fmt.Errorf("error requesting POST %s json: %v", path, err)
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
							Error struct {
								Message string `json:"message,omitempty"`
							} `json:"error,omitempty"`
							Model   string `json:"model"`
							Choices []struct {
								Message struct {
									Content string `json:"content"`
								} `json:"message"`
							} `json:"choices"`
							Usage struct {
								PromptTokens     int `json:"prompt_tokens"`
								CompletionTokens int `json:"completion_tokens"`
								TotalTokens      int `json:"total_tokens"`
							} `json:"usage"`
						}

						if err := json.Unmarshal(respBody.Bytes(), &body); err != nil {
							return fmt.Errorf("error decoding response: %v %s", err, respBody.String())
						}

						if resp.StatusCode != http.StatusOK {
							return fmt.Errorf("API error: status %d, message: %s", resp.StatusCode, respBody.String())
						}

						if len(body.Choices) == 0 {
							return fmt.Errorf("no completions returned, response: %s", respBody.String())
						}

						elapsed := time.Since(t).Seconds()

						fmt.Printf("Response status: %s, Time taken: %.1f sec, completion tokens: %d, completion rate: %.1f tokens/s\n",
							resp.Status, elapsed, int64(body.Usage.CompletionTokens), float64(body.Usage.CompletionTokens)/elapsed)
						fmt.Printf("----------------------------------------\n")
						fmt.Printf("\nUser: %s\n", c.String("prompt"))
						fmt.Printf("\nAssistant (%s):\n\n%s\n\n", body.Model, body.Choices[0].Message.Content)
					}
					return nil
				},
			},
			{
				Name:  "chat",
				Usage: "Interactive chat session with streaming",
				Flags: []cli.Flag{
					&cli.StringFlag{
						Name:    "model",
						Aliases: []string{"m"},
						Usage:   "Model name",
						Value:   "qwen2.5",
					},
					&cli.Float64Flag{
						Name:    "temperature",
						Aliases: []string{"t"},
						Usage:   "Temperature (0.0-2.0)",
						Value:   0.7,
					},
					&cli.IntFlag{
						Name:    "max-tokens",
						Aliases: []string{"n"},
						Usage:   "Max completion tokens",
						Value:   2048,
					},
					&cli.StringFlag{
						Name:    "system",
						Aliases: []string{"s"},
						Usage:   "System prompt",
						Value:   "You are a helpful assistant",
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

					return session.RunRawChatSession(c.Bool("json-output"))
				},
			},
			{
				Name:  "models",
				Usage: "Model operations",
				Subcommands: []*cli.Command{
					{
						Name:  "get",
						Usage: "Get details about a specific model",
						Flags: []cli.Flag{
							&cli.StringFlag{
								Name:     "model_name",
								Usage:    "Model name",
								Required: true,
							},
							&cli.StringFlag{
								Name:     "service",
								Usage:    "Service name",
								Required: false,
							},
						},
						Action: func(c *cli.Context) error {
							client := NewClient(c.String("base-url"), c.Int("port"), c.Int("timeout"))
							modelName := c.String("model_name")
							serviceName := c.String("service")

							var path string

							if serviceName == "" {
								path = fmt.Sprintf("/llm/models/%s", modelName)
							} else {
								path = fmt.Sprintf("/llm/services/%s/models/%s", serviceName, modelName)
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

							// Parse the model information
							var modelInfo struct {
								ID          string   `json:"id"`
								Name        string   `json:"name"`
								Service     string   `json:"service"`
								Type        string   `json:"type"`
								Description string   `json:"description"`
								Context     int      `json:"context"`
								Size        int64    `json:"size"`
								Tags        []string `json:"tags"`
							}

							if err := json.Unmarshal(respBody.Bytes(), &modelInfo); err != nil {
								return fmt.Errorf("error parsing model info: %v", err)
							}

							// Display model information in a nice format
							table := tablewriter.NewWriter(os.Stdout)
							table.SetHeader([]string{"Property", "Value"})

							table.Append([]string{"Model name", modelInfo.Name})
							table.Append([]string{"Service", modelInfo.Service})
							table.Append([]string{"Model type", modelInfo.Type})
							table.Append([]string{"Upstream model name", strings.TrimPrefix(modelInfo.Name, fmt.Sprintf("%s%s", modelInfo.Service, llm.ServiceNameSeparator))})
							table.Append([]string{"Context size", fmt.Sprintf("%d tokens", modelInfo.Context)})
							table.Append([]string{"Model file size", fmt.Sprintf("%d bytes", modelInfo.Size)})
							table.Append([]string{"Description", modelInfo.Description})

							if len(modelInfo.Tags) > 0 {
								table.Append([]string{"Tags", strings.Join(modelInfo.Tags, ", ")})
							}

							table.Render()
							return nil
						},
					},
				},
			},
			{
				Name:  "embedding",
				Usage: "Get embeddings for input text",
				Flags: []cli.Flag{
					&cli.StringFlag{
						Name:    "model",
						Aliases: []string{"m"},
						Usage:   "Model name",
						Value:   "",
					},
					&cli.StringFlag{
						Name:     "input",
						Aliases:  []string{"i"},
						Usage:    "Input text to embed",
						Required: true,
					},
					&cli.StringFlag{
						Name:    "encoding-format",
						Aliases: []string{"e"},
						Usage:   "Encoding format (e.g., 'float')",
						Value:   "float",
					},
					&cli.IntFlag{
						Name:    "dimensions",
						Aliases: []string{"d"},
						Usage:   "Number of dimensions for the embedding vector",
						Value:   0,
					},
					&cli.BoolFlag{
						Name:    "full-vector",
						Aliases: []string{"f"},
						Usage:   "Display the full embedding vector",
						Value:   false,
					},
				},
				Action: func(c *cli.Context) error {
					client := NewClient(c.String("base-url"), c.Int("port"), c.Int("timeout"))

					// Prepare the request body
					request := map[string]interface{}{
						"model":           c.String("model"),
						"input":           c.String("input"),
						"encoding_format": c.String("encoding-format"),
					}

					// Add dimensions if specified
					if c.Int("dimensions") > 0 {
						request["dimensions"] = c.Int("dimensions")
					}

					startTime := time.Now()

					// Make the request
					resp, err := client.doRequest("POST", "/v1/embeddings", request)
					if err != nil {
						return fmt.Errorf("error requesting embeddings: %v", err)
					}
					if resp.Body != nil {
						defer resp.Body.Close()
					}

					// Process the response
					respBody, err := processResponse(resp)
					if err != nil {
						return err
					}

					if c.Bool("json-output") {
						fmt.Println(respBody.String())
						return nil
					}

					// Parse the response
					var embeddingResponse struct {
						Object string `json:"object"`
						Model  string `json:"model"`
						Data   []struct {
							Object    string    `json:"object"`
							Index     int       `json:"index"`
							Embedding []float64 `json:"embedding"`
						} `json:"data"`
						Usage struct {
							PromptTokens int `json:"prompt_tokens"`
							TotalTokens  int `json:"total_tokens"`
						} `json:"usage"`
					}

					if err := json.Unmarshal(respBody.Bytes(), &embeddingResponse); err != nil {
						return fmt.Errorf("error parsing embedding response: %v", err)
					}

					// Display embedding information
					fmt.Println("")
					fmt.Printf("Model:    %s\n", embeddingResponse.Model)
					fmt.Printf("Duration: %.3f sec\n", time.Since(startTime).Seconds())
					fmt.Printf("Tokens:   %d\n", embeddingResponse.Usage.TotalTokens)

					for _, data := range embeddingResponse.Data {
						fmt.Printf("\nEmbedding #%d:\n", data.Index)
						fmt.Printf(" - Dimensions: %d\n", len(data.Embedding))

						if c.Bool("full-vector") {
							fmt.Println(" - Vector:")

							// Print the embedding vector in a readable format
							for i, value := range data.Embedding {
								if i > 0 && i%5 == 0 {
									fmt.Println()
								}
								fmt.Printf("%10.6f ", value)
							}
							fmt.Println()
						} else {
							// Show just a preview of the vector
							previewSize := 5
							if len(data.Embedding) < previewSize {
								previewSize = len(data.Embedding)
							}

							fmt.Printf(" - Vector preview (first 5 dimensions): ")
							for i := 0; i < previewSize; i++ {
								fmt.Printf("%10.6f ", data.Embedding[i])
							}
							fmt.Println(" ...")
						}
					}

					return nil
				},
			},
		},
	}
}
