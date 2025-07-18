package chat

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"math"
	"net/http"
	"strings"
	"time"

	"github.com/danielgtaylor/huma/v2"
	"github.com/google/uuid"
	"github.com/hypernetix/hyperspot/libs/api"
	"github.com/hypernetix/hyperspot/libs/config"
	"github.com/hypernetix/hyperspot/libs/logging"
	"github.com/hypernetix/hyperspot/modules/llm"
)

type ChatMessageAPIRequestBody struct {
	ModelName   string  `json:"model_name"`
	Temperature float32 `json:"temperature,omitempty" default:"0.5"`
	MaxTokens   int     `json:"max_tokens,omitempty" default:"2000"`
	Content     string  `json:"content"`
	Stream      bool    `json:"stream,omitempty" default:"false"`
	GroupID     string  `json:"group_id,omitempty" doc:"group id to create a new thread in"`
}

type ChatMessageAPIUpdateBody struct {
	Content string `json:"content"`
}

type ChatMessageAPIResponse struct {
	Body ChatMessage `json:"body"`
}

type ChatMessageAPIResponseList struct {
	Body struct {
		api.PageAPIResponse
		ChatMessages []ChatMessage `json:"messages"`
	} `json:"body"`
}

type ChatMessageSreamnAPIChunkResponse struct {
	Delta struct {
		ID           uuid.UUID `json:"id"`
		Content      string    `json:"content"`
		FinishReason string    `json:"finish_reason"`
		Error        string    `json:"error"`
	} `json:"delta"`
}

type ChatMessageSreamnAPIFinalChunkResponse struct {
	Patch ChatMessage `json:"patch"`
}

type ChatMessageSreamnAPIUpstreamChunkChoiceResponse struct {
	Delta struct {
		Content string `json:"content"`
	} `json:"delta"`
}

type ChatMessageSreamnAPIUpstreamChunkResponse struct {
	Choices      []ChatMessageSreamnAPIUpstreamChunkChoiceResponse `json:"choices"`
	FinishReason string                                            `json:"finish_reason"`
}

func ChatMessageAPIStream(ctx context.Context, groupID uuid.UUID, threadID uuid.UUID, input *ChatMessageAPIRequestBody) (*huma.StreamResponse, error) {
	if groupID != uuid.Nil {
		_, errx := GetChatThreadGroup(ctx, groupID)
		if errx != nil {
			return nil, errx.HumaError()
		}
	}

	if threadID != uuid.Nil {
		thread, errx := GetChatThread(ctx, threadID)
		if errx != nil {
			return nil, errx.HumaError()
		}
		if groupID != uuid.Nil && groupID != thread.GroupID {
			return nil, huma.Error400BadRequest(fmt.Sprintf("Group ID '%s' does not match thread group ID '%s'", groupID.String(), thread.GroupID.String()))
		}
	}

	// Add debugging for context state
	if ctx.Err() != nil {
		fmt.Printf("Context already canceled at ChatMessageAPIStream entry: %v\n", ctx.Err())
	}

	model, errx := llm.GetModelOrFirstLoaded(ctx, input.ModelName)
	if errx != nil {
		return nil, errx.HumaError()
	}

	// CreateMessage in PrepareLLMChatRequest will assign a thread ID if threadID is uuid.Nil
	// This ensures all messages are properly grouped by thread ID
	userMsg, upstreamRequest, errx := PrepareLLMChatRequest(ctx, groupID, threadID, model, input.Temperature, input.MaxTokens, input.Content, input.Stream)
	if errx != nil {
		return nil, errx.HumaError()
	}

	if input.Stream {
		startTime := time.Now()

		// Use the thread ID from the user message to ensure consistency
		assitantMsg, errx := CreateMessage(ctx, groupID, userMsg.ThreadID, ChatMessageRoleNameAssistant, model.Name, input.MaxTokens, input.Temperature, "", false, 0, 0)
		if errx != nil {
			return nil, errx.HumaError()
		}

		initialResponse := ChatMessageAPIResponseList{
			Body: struct {
				api.PageAPIResponse
				ChatMessages []ChatMessage `json:"messages"`
			}{
				PageAPIResponse: api.PageAPIResponse{
					PageNumber: 1,
					PageSize:   1,
					Total:      2,
				},
				ChatMessages: []ChatMessage{*userMsg, *assitantMsg},
			},
		}

		var upstreamResp *http.Response
		var err error
		if model.ServicePtr.GetCapabilities().Streaming {
			upstreamResp, err = model.ServicePtr.ChatCompletionsStreamRaw(ctx, model, upstreamRequest.ToMap())
		} else {
			logging.Debug("LLM service '%s' does not support streaming, using non-streaming mode", model.ServicePtr.GetName())
			upstreamRequest.Stream = false
			resp, err := model.ServicePtr.ChatCompletions(ctx, model, upstreamRequest)

			if err != nil {
				return nil, huma.Error502BadGateway(fmt.Sprintf("LLM service '%s' returned an error: %v",
					model.ServicePtr.GetName(), err))
			}
			upstreamResp = resp.HttpResp.UpstreamResponse

			var simulatedChatMessageSreamnAPIUpstreamChunkResponse ChatMessageSreamnAPIUpstreamChunkResponse

			for _, choice := range resp.Choices {
				c := ChatMessageSreamnAPIUpstreamChunkChoiceResponse{}
				c.Delta.Content = choice.Message.Content
				simulatedChatMessageSreamnAPIUpstreamChunkResponse.Choices = append(simulatedChatMessageSreamnAPIUpstreamChunkResponse.Choices, c)
			}

			simulatedChatMessageSreamnAPIUpstreamChunkResponse.FinishReason = "stop"

			jsonBytes, err := json.Marshal(simulatedChatMessageSreamnAPIUpstreamChunkResponse)
			if err != nil {
				return nil, huma.Error502BadGateway(fmt.Sprintf("Error marshaling simulated upstream response: %v", err))
			}

			simulatedBody := "data: " + string(jsonBytes) + "\n\n"
			upstreamResp.Body = io.NopCloser(bytes.NewBuffer([]byte(simulatedBody)))
		}

		if err != nil {
			return nil, huma.Error502BadGateway(fmt.Sprintf("LLM service '%s' returned an error: %v",
				model.ServicePtr.GetName(), err))
		}

		// We can't use 'defer upstreamResp.Body.Close()' here because:
		// 1. This is a streaming API where the response body is read asynchronously in the StreamResponse.Body function
		// 2. Closing the body here would terminate the stream before the client receives all data
		// 3. The Body.Close() responsibility is implicitly transferred to the StreamResponse handler
		// 4. The actual close happens after all data is read (at io.EOF) or when an error occurs

		// Check for non-200 responses.
		if upstreamResp.StatusCode != http.StatusOK {
			bodyBytes, _ := io.ReadAll(upstreamResp.Body)
			if err := upstreamResp.Body.Close(); err != nil {
				logging.Error("Failed to close upstream response body: %v", err)
			}
			return nil, fmt.Errorf("upstream error (%d): %s", upstreamResp.StatusCode, string(bodyBytes))
		}

		// Return a streaming response.
		return &huma.StreamResponse{
			Body: func(ctx huma.Context) {
				if err := ctx.SetReadDeadline(time.Now().Add(config.GetServerTimeout())); err != nil {
					logging.Error("Failed to set read deadline: %v", err)
				}
				// Note: We don't need to explicitly close upstreamResp.Body here
				// The body is automatically closed when we reach EOF in the read loop below
				// or when an error occurs during reading

				// Optionally copy some headers from the upstream response.
				ctx.SetHeader("Content-Type", upstreamResp.Header.Get("Content-Type"))

				initialResponseBytes, err := json.Marshal(initialResponse.Body)
				if err != nil {
					log.Printf("error marshaling initial response: %v", err)
					return
				}
				if _, err := ctx.BodyWriter().Write([]byte("data: " + string(initialResponseBytes) + "\n")); err != nil {
					logging.Error("Failed to write initial response: %v", err)
				}

				// Force the content to flush immediately
				if flusher, ok := ctx.BodyWriter().(http.Flusher); ok {
					flusher.Flush()
				}

				// Create a buffer and continuously stream data.
				buf := make([]byte, 1024*1024)
				for {
					n, upstreamReadErr := upstreamResp.Body.Read(buf)
					if n > 0 {
						lines := strings.Split(string(buf[:n]), "\n")

						for _, line := range lines {
							if line == "" {
								continue
							}

							if strings.HasPrefix(line, "event: ") {
								logging.Debug("Received chat streaming event: %s", strings.TrimPrefix(line, "event: "))
								continue
							}

							if !strings.HasPrefix(line, "data: ") {
								logging.Warn("Unsupported chat streaming content: %s", line)
								continue
							}

							data := strings.TrimPrefix(line, "data: ")

							if data == "" {
								continue
							}

							var responseBytes []byte
							done := false

							if strings.TrimSpace(strings.TrimSuffix(data, "\n")) == "[DONE]" {
								responseBytes = []byte("[DONE]\n\n")
								done = true
							} else {
								var upstreamStreamResponseChunk ChatMessageSreamnAPIUpstreamChunkResponse
								var marshalErr error

								if err := json.Unmarshal([]byte(data), &upstreamStreamResponseChunk); err != nil {
									chunkResponse := ChatMessageSreamnAPIChunkResponse{}
									msg := fmt.Sprintf("upstream response error: %s", data)
									logging.Warn(msg)
									assitantMsg.Error = msg
									chunkResponse.Delta.Error = msg
									chunkResponse.Delta.ID = assitantMsg.ID
									responseBytes, marshalErr = json.Marshal(chunkResponse)
								} else {
									chunkContent := upstreamStreamResponseChunk.Choices[0].Delta.Content
									assitantMsg.Content += chunkContent

									if upstreamStreamResponseChunk.FinishReason != "" {
										chunkResponse := ChatMessageSreamnAPIFinalChunkResponse{}
										chunkResponse.Patch = *assitantMsg
										responseBytes, marshalErr = json.Marshal(chunkResponse)
									} else {
										chunkResponse := ChatMessageSreamnAPIChunkResponse{}
										chunkResponse.Delta.Content = chunkContent
										chunkResponse.Delta.ID = assitantMsg.ID
										chunkResponse.Delta.FinishReason = upstreamStreamResponseChunk.FinishReason
										responseBytes, marshalErr = json.Marshal(chunkResponse)
									}
								}

								if marshalErr != nil {
									logging.Error("error marshaling chunk response: %v", marshalErr)
									continue
								}
							}

							// Write the actual response to the client and force flushing by sending extra dummy data
							writer := ctx.BodyWriter()
							_, writeErr := writer.Write([]byte("data: " + string(responseBytes) + "\n\n"))
							if writeErr != nil {
								logging.Error("error writing to response: %v", writeErr)
								return
							}

							// Force the content to flush immediately
							if flusher, ok := writer.(http.Flusher); ok {
								flusher.Flush()
							}

							if done {
								break
							}
						}
					}

					if upstreamReadErr != nil {
						assitantMsg.SizeChars = len(assitantMsg.Content)
						assitantMsg.SizeTokens = int(math.Ceil(float64(len(assitantMsg.Content)) / 4.0)) // TODO: need to get the actual number of tokens
						assitantMsg.TokensPerSec = float32(float64(assitantMsg.SizeTokens) / time.Since(startTime).Seconds())
						assitantMsg.CharsPerSec = float32(float64(assitantMsg.SizeChars) / time.Since(startTime).Seconds())
						assitantMsg.Completed = true

						assitantMsg.DbSaveFields(
							&assitantMsg.SizeChars,
							&assitantMsg.SizeTokens,
							&assitantMsg.TokensPerSec,
							&assitantMsg.CharsPerSec,
							&assitantMsg.Completed,
							&assitantMsg.Content,
						)

						chunkResponse := ChatMessageSreamnAPIFinalChunkResponse{
							Patch: *assitantMsg,
						}
						responseBytes, marshalErr := json.Marshal(chunkResponse)

						if marshalErr != nil {
							log.Printf("error marshaling chunk response: %v", marshalErr)
							continue
						}

						// Write the response to the client
						_, writeErr := ctx.BodyWriter().Write([]byte("data: " + string(responseBytes) + "\n\n"))
						if writeErr != nil {
							log.Printf("error writing to response: %v", writeErr)
							return
						}

						if upstreamReadErr == io.EOF {
							break
						}
						log.Printf("error reading from upstream: %v", upstreamReadErr)
						break
					}
				}
			},
		}, nil
	} else {
		// Proxy the request to the LLM service
		upstreamResp, err := model.ServicePtr.ChatCompletions(ctx, model, upstreamRequest)
		if err != nil {
			return nil, huma.Error502BadGateway(fmt.Sprintf("LLM service '%s' returned an error: %v",
				model.ServicePtr.GetName(), err))
		}

		// Use the thread ID from the user message to ensure consistency
		assitantMsg, errx := CreateMessage(ctx, groupID, userMsg.ThreadID, ChatMessageRoleNameAssistant, model.Name, input.MaxTokens, input.Temperature, upstreamResp.Choices[0].Message.Content, true, 0, 0)
		if errx != nil {
			return nil, errx.HumaError()
		}

		response := ChatMessageAPIResponseList{
			Body: struct {
				api.PageAPIResponse
				ChatMessages []ChatMessage `json:"messages"`
			}{
				PageAPIResponse: api.PageAPIResponse{
					PageNumber: 1,
					PageSize:   1,
					Total:      2,
				},
				ChatMessages: []ChatMessage{*userMsg, *assitantMsg},
			},
		}

		return &huma.StreamResponse{
			Body: func(ctx huma.Context) {
				if err := ctx.SetReadDeadline(time.Now().Add(config.GetServerTimeout())); err != nil {
					logging.Error("Failed to set read deadline: %v", err)
				}
				ctx.SetHeader("Content-Type", "application/json")
				responseBytes, err := json.Marshal(response.Body)
				if err != nil {
					log.Printf("error marshaling response: %v", err)
					return
				}
				if _, err := ctx.BodyWriter().Write(responseBytes); err != nil {
					logging.Error("Failed to write response: %v", err)
				}
			},
		}, nil
	}
	// return nil, nil -- unreachable
}

func registerChatMessageAPIRoutes(humaApi huma.API) {
	api.RegisterEndpoint(humaApi, huma.Operation{
		OperationID: "get-chat-message",
		Method:      http.MethodGet,
		Path:        "/chat/messages/{id}",
		Summary:     "Get a specific chat message by ID",
		Tags:        []string{"Chat Message"},
	}, func(ctx context.Context, input *struct {
		MessageID uuid.UUID `path:"id" doc:"The chat message UUID" example:"550e8400-e29b-41d4-a716-446655440000"`
	}) (*ChatMessageAPIResponse, error) {
		message, errx := GetMessage(ctx, input.MessageID)
		if errx != nil {
			return nil, errx.HumaError()
		}

		return &ChatMessageAPIResponse{
			Body: *message,
		}, nil
	})

	api.RegisterEndpoint(humaApi, huma.Operation{
		OperationID: "create-chat-message-in-thread",
		Method:      http.MethodPost,
		Path:        "/chat/threads/{thread_id}/messages",
		Summary:     "Create a new chat message in the specified thread",
		Tags:        []string{"Chat Thread", "Chat Message"},
	}, func(ctx context.Context, input *struct {
		ThreadID     string                    `path:"thread_id" doc:"The chat thread UUID" example:"550e8400-e29b-41d4-a716-446655440001"`
		Body         ChatMessageAPIRequestBody `json:"body"`
		ReplaceMsgID string                    `query:"replace_msg_id" doc:"replace given message in the thread and delete all messages after it"`
	}) (*huma.StreamResponse, error) {
		// Read the request body and store it in a local variable to ensure
		// the connection is released after processing
		requestBody := input.Body

		groupID := uuid.Nil

		threadID, err := uuid.Parse(input.ThreadID)
		if err != nil {
			return nil, huma.Error400BadRequest("Invalid chat ID format")
		}

		if input.ReplaceMsgID != "" {
			replaceMsgID, err := uuid.Parse(input.ReplaceMsgID)
			if err != nil {
				return nil, huma.Error400BadRequest("Invalid replace ID format")
			}

			errx := DeleteChatTail(ctx, threadID, replaceMsgID)
			if errx != nil {
				return nil, errx.HumaError()
			}
		}

		return ChatMessageAPIStream(ctx, groupID, threadID, &requestBody)
	})

	api.RegisterEndpoint(humaApi, huma.Operation{
		OperationID: "create-chat-message",
		Method:      http.MethodPost,
		Path:        "/chat/messages",
		Summary:     "Create a new chat message and start a new thread",
		Tags:        []string{"Chat Message"},
	}, func(ctx context.Context, input *struct {
		Body ChatMessageAPIRequestBody `json:"body"`
	}) (*huma.StreamResponse, error) {
		// Read the request body and store it in a local variable to ensure
		// the connection is released after processing
		requestBody := input.Body

		var groupID uuid.UUID
		var err error

		if requestBody.GroupID != "" {
			groupID, err = uuid.Parse(requestBody.GroupID)
			if err != nil {
				return nil, huma.Error400BadRequest("Invalid group ID format")
			}
		} else {
			groupID = uuid.Nil
		}

		return ChatMessageAPIStream(ctx, groupID, uuid.Nil, &requestBody)
	})

	api.RegisterEndpoint(humaApi, huma.Operation{
		OperationID: "update-chat-message",
		Method:      http.MethodPut,
		Path:        "/chat/messages/{id}",
		Summary:     "Update a chat message",
		Tags:        []string{"Chat Message"},
	}, func(ctx context.Context, input *struct {
		ID   string                   `path:"id" doc:"The chat message UUID" example:"550e8400-e29b-41d4-a716-446655440000"`
		Body ChatMessageAPIUpdateBody `json:"body"`
	}) (*ChatMessageAPIResponse, error) {
		messageID, err := uuid.Parse(input.ID)
		if err != nil {
			return nil, huma.Error400BadRequest("Invalid message ID format")
		}

		msg, errx := UpdateMessage(ctx, messageID, input.Body.Content)
		if errx != nil {
			return nil, errx.HumaError()
		}

		return &ChatMessageAPIResponse{
			Body: *msg,
		}, nil
	})

	api.RegisterEndpoint(humaApi, huma.Operation{
		OperationID: "delete-chat-message",
		Method:      http.MethodDelete,
		Path:        "/chat/messages/{id}",
		Summary:     "Delete a chat message",
		Tags:        []string{"Chat Message"},
	}, func(ctx context.Context, input *struct {
		ID string `path:"id" doc:"The chat message UUID" example:"550e8400-e29b-41d4-a716-446655440000"`
	}) (*struct{}, error) {
		messageID, err := uuid.Parse(input.ID)
		if err != nil {
			return nil, huma.Error400BadRequest("Invalid message ID format")
		}

		errx := DeleteMessage(ctx, messageID)
		if errx != nil {
			return nil, errx.HumaError()
		}

		return &struct{}{}, nil
	})
}
