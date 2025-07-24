package chat

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/hypernetix/hyperspot/libs/api"
	"github.com/hypernetix/hyperspot/libs/auth"
	"github.com/hypernetix/hyperspot/libs/db"
	"github.com/hypernetix/hyperspot/libs/errorx"
	"github.com/hypernetix/hyperspot/libs/openapi_client"
	"github.com/hypernetix/hyperspot/libs/orm"
	"github.com/hypernetix/hyperspot/modules/llm"

	"runtime/debug"

	"github.com/google/uuid"
)

type ChatMessageRole int

const (
	ChatMessageRoleIdUser      ChatMessageRole = 0
	ChatMessageRoleIdAssistant ChatMessageRole = 1
	ChatMessageRoleIdSystem    ChatMessageRole = 2
)

const (
	ChatMessageRoleNameUser      = "user"
	ChatMessageRoleNameAssistant = "assistant"
	ChatMessageRoleNameSystem    = "system"
)

var ChatMessageRoleNames = map[ChatMessageRole]string{
	ChatMessageRoleIdUser:      ChatMessageRoleNameUser,
	ChatMessageRoleIdAssistant: ChatMessageRoleNameAssistant,
	ChatMessageRoleIdSystem:    ChatMessageRoleNameSystem,
}

var ChatMessageRoleIds = map[string]ChatMessageRole{
	ChatMessageRoleNameUser:      ChatMessageRoleIdUser,
	ChatMessageRoleNameAssistant: ChatMessageRoleIdAssistant,
	ChatMessageRoleNameSystem:    ChatMessageRoleIdSystem,
}

type ChatMessage struct {
	SequenceID    int64           `json:"-" gorm:"primaryKey;autoIncrement:true"`
	ID            uuid.UUID       `json:"id" gorm:"index"`
	TenantID      uuid.UUID       `json:"-" gorm:"index"`
	UserID        uuid.UUID       `json:"user_id" gorm:"index"`
	ThreadID      uuid.UUID       `json:"thread_id" gorm:"index"`
	Role          string          `json:"role" gorm:"-" doc:"user, assistant, system"`
	RoleId        ChatMessageRole `json:"-" gorm:"index" doc:"0 - user, 1 - assistant, 2 - system"`
	ModelName     string          `json:"model_name"`
	MaxTokens     int             `json:"max_tokens"`
	Temperature   float32         `json:"temperature"`
	CreatedAtMs   int64           `json:"created_at" gorm:"index" doc:"unix timestamp in milliseconds"`
	UpdatedAtMs   int64           `json:"updated_at" gorm:"index" doc:"unix timestamp in milliseconds"`
	Content       string          `json:"content"`
	Error         string          `json:"error" gorm:"-"`
	Likes         int             `json:"like" doc:"1 - like, 0 - not set, -1 - dislike"`
	IsDeleted     bool            `json:"-" gorm:"index"`
	Completed     bool            `json:"completed"`
	SizeChars     int             `json:"size_chars"`
	SizeTokens    int             `json:"size_tokens"`
	TokensPerSec  float32         `json:"tokens_per_sec"`
	CharsPerSec   float32         `json:"chars_per_sec"`
	ChatThreadPtr *ChatThread     `json:"-" gorm:"-"`
	FinishReason  string          `json:"finish_reason"`
	// Optional file attachement fields. Set only if IsFileAttachment is true. We interpret
	// the file attachment is just another user message with IsFileAttachment = true
	IsFileAttachment       bool   `json:"is_file_attachment,omitempty" doc:"true if the message is a file attachment and not user prompt"`
	OriginalFileSize       int64  `json:"original_file_size,omitempty" doc:"original binaryfile size in bytes"`
	FullContentLength      int64  `json:"full_content_length,omitempty" doc:"full parsed content length in characters"`
	TruncatedContentLength int64  `json:"truncated_content_length,omitempty" doc:"truncated parsed content length in characters"`
	IsTruncated            bool   `json:"is_truncated,omitempty" doc:"true if the file content was truncated"`
	AttachedFileName       string `json:"attached_file_name,omitempty" doc:"original file name"`
	AttachedFileExtension  string `json:"attached_file_extension,omitempty" doc:"original file extension"`
}

func (m *ChatMessage) DbSaveFields(fields ...interface{}) errorx.Error {
	m.UpdatedAtMs = time.Now().UnixMilli()
	fields = append(fields, &m.UpdatedAtMs)

	pkFields := map[string]interface{}{
		"id":        m.ID,
		"tenant_id": m.TenantID,
		"user_id":   m.UserID,
		"thread_id": m.ThreadID,
	}

	errx := orm.OrmUpdateObjFields(m, pkFields, fields...)
	if errx != nil {
		msg := fmt.Sprintf("failed to update message: %s", errx.Error())
		if m.ChatThreadPtr != nil {
			m.ChatThreadPtr.LogError(msg)
		}
		return errx
	}

	return nil
}

func listMessages(ctx context.Context, threadID uuid.UUID, pageRequest *api.PageAPIRequest) ([]*ChatMessage, errorx.Error) {
	var systemPrompts []*SystemPrompt
	var messages []*ChatMessage
	var errx errorx.Error

	// 1. Get system prompts for the thread

	chatThread, errx := GetChatThread(ctx, threadID)
	if errx != nil {
		return nil, errx
	}

	if len(chatThread.SystemPrompts) > 0 {
		systemPrompts = append(systemPrompts, chatThread.SystemPrompts...)
	}

	// 2. Merge system prompts and convert it to the first chat message

	systemMsg := systemPromptsToChatMessage(systemPrompts)
	if systemMsg != nil {
		messages = append(messages, systemMsg)
	}

	// 3. Get messages for the thread

	var chatMessages []*ChatMessage

	query, errx := orm.GetBaseQuery(&ChatMessage{}, auth.GetTenantID(), auth.GetUserID(), pageRequest)
	if errx != nil {
		return nil, errx
	}

	query = query.Where("thread_id = ? AND is_deleted = ?", threadID, false)

	if pageRequest != nil && strings.HasPrefix(pageRequest.Order, "-") {
		query = query.Order("sequence_id desc")
	} else {
		query = query.Order("sequence_id asc")
	}

	if err := query.Find(&chatMessages).Error; err != nil {
		return nil, errorx.NewErrInternalServerError("Failed to list messages: " + err.Error())
	}

	for _, message := range chatMessages {
		message.ChatThreadPtr = chatThread
		roleName, ok := ChatMessageRoleNames[message.RoleId]
		if !ok {
			return nil, errorx.NewErrBadRequest("Invalid role")
		}
		message.Role = roleName
	}

	messages = append(messages, chatMessages...)
	return messages, nil
}

func GetMessage(ctx context.Context, messageID uuid.UUID) (*ChatMessage, errorx.Error) {
	query, errx := orm.GetBaseQuery(&ChatMessage{}, auth.GetTenantID(), auth.GetUserID(), nil)
	if errx != nil {
		return nil, errx
	}

	var message ChatMessage
	err := query.Where("id = ?", messageID).First(&message).Error
	if err != nil {
		return nil, errorx.NewErrNotFound(fmt.Sprintf("Message '%s' not found", messageID.String()))
	}

	thread, errx := GetChatThread(ctx, message.ThreadID)
	if errx != nil {
		return nil, errx
	}
	message.ChatThreadPtr = thread

	roleName, ok := ChatMessageRoleNames[message.RoleId]
	if !ok {
		return nil, errorx.NewErrBadRequest("Invalid role")
	}
	message.Role = roleName

	return &message, nil
}

func shortenMessage(message string, maxLength int) string {
	if len(message) <= maxLength {
		return message
	}
	return message[:maxLength] + "..."
}

// PrepareLLMChatRequest creates a new message and prepares the request payload for the LLM API
// If threadID is uuid.Nil, a new thread will be created with the first message conten
func PrepareLLMChatRequest(
	ctx context.Context,
	groupID uuid.UUID,
	threadID uuid.UUID,
	model *llm.LLMModel,
	temperature float32,
	maxTokens int,
	content string,
	stream bool,
) (*ChatMessage, *openapi_client.OpenAICompletionRequest, errorx.Error) {
	// Debug context state
	if ctx.Err() != nil {
		fmt.Printf("Context already canceled at PrepareLLMChatRequest entry: %v\n", ctx.Err())
		debug.PrintStack()
	}

	if threadID == uuid.Nil {
		thread, errx := CreateChatThread(ctx, groupID, shortenMessage(content, 128), nil, nil, nil, []uuid.UUID{})
		if errx != nil {
			return nil, nil, errx
		}
		threadID = thread.ID
	}

	var limit int = 100 // TODO: limit to 100 last messages, need to be smarter!

	var msgs []*ChatMessage
	var errx errorx.Error

	msgs, errx = listMessages(ctx, threadID, &api.PageAPIRequest{
		PageNumber: 1,
		PageSize:   limit,
	})

	if errx != nil {
		return nil, nil, errx
	}

	if len(msgs) == limit {
		return nil, nil, errorx.NewErrBadRequest(fmt.Sprintf("Too many messages in the thread '%s', limit is %d", threadID.String(), limit))
	}

	msg, errx := CreateMessage(ctx, groupID, threadID, ChatMessageRoleNameUser, model.Name, maxTokens, temperature, content, true, 0, 0)
	if errx != nil {
		return nil, nil, errx
	}

	msgs = append(msgs, msg)

	// Convert chat messages to the format expected by the LLM API
	var openAIMessages []openapi_client.OpenAICompletionMessage

	// Add the conversation messages
	for _, msg := range msgs {
		openAIMessages = append(openAIMessages, openapi_client.OpenAICompletionMessage{
			Role:    msg.Role,
			Content: msg.Content,
		})
	}

	// Prepare the request payload
	requestPayload := &openapi_client.OpenAICompletionRequest{
		ModelName:   model.UpstreamModelName,
		Messages:    openAIMessages,
		Temperature: float64(temperature),
		MaxTokens:   msg.MaxTokens,
		Stream:      stream,
	}

	return msg, requestPayload, nil
}

func ValidateMessage(
	ctx context.Context,
	content string,
	modelName string,
	maxTokens int,
	temperature float32,
) errorx.Error {
	if modelName == "" {
		return errorx.NewErrBadRequest("Model name is required")
	}

	if temperature < 0 || temperature > 1 {
		return errorx.NewErrBadRequest("Temperature must be between 0.0 and 1.0")
	}

	if maxTokens < 0 {
		return errorx.NewErrBadRequest("Max tokens must be greater than 0")
	}

	return nil
}

func CreateMessage(
	ctx context.Context,
	groupID uuid.UUID,
	threadID uuid.UUID,
	role string,
	modelName string,
	maxTokens int,
	temperature float32,
	content string,
	completed bool,
	sizeTokens int,
	seconds float32,
) (*ChatMessage, errorx.Error) {
	if threadID == uuid.Nil {
		return nil, errorx.NewErrBadRequest("Thread ID is required")
	}

	errx := ValidateMessage(ctx, content, modelName, maxTokens, temperature)
	if errx != nil {
		return nil, errx
	}

	modelPtr := llm.GetModel(ctx, modelName, false, llm.ModelStateLoaded)
	if modelPtr == nil {
		modelPtr = llm.GetModel(ctx, modelName, false, llm.ModelStateAny)
		if modelPtr == nil {
			return nil, errorx.NewErrBadRequest(fmt.Sprintf("Model '%s' not found", modelName))
		}
		return nil, errorx.NewErrBadRequest(fmt.Sprintf("Model '%s' is not loaded", modelName))
	}

	roleId, ok := ChatMessageRoleIds[role]
	if !ok {
		return nil, errorx.NewErrBadRequest("Invalid role")
	}

	var thread *ChatThread

	thread, errx = GetChatThread(ctx, threadID)
	if errx != nil {
		return nil, errx
	}
	if groupID != uuid.Nil && groupID != thread.GroupID {
		return nil, errorx.NewErrBadRequest(fmt.Sprintf("Group ID '%s' does not match thread group ID '%s'", groupID.String(), thread.GroupID.String()))
	}

	if maxTokens == 0 {
		maxTokens = 2048
	}

	if modelPtr.MaxTokens > 0 && maxTokens > modelPtr.MaxTokens {
		maxTokens = modelPtr.MaxTokens
	}

	var TokensPerSec float32
	var CharsPerSec float32

	if seconds > 0 {
		TokensPerSec = float32(sizeTokens) / seconds
		CharsPerSec = float32(len(content)) / seconds
	}

	t := time.Now().UnixMilli()

	msg := ChatMessage{
		ID:            uuid.New(),
		TenantID:      auth.GetTenantID(),
		UserID:        auth.GetUserID(),
		ThreadID:      threadID,
		RoleId:        roleId,
		Role:          role,
		ModelName:     modelName,
		MaxTokens:     maxTokens,
		Temperature:   float32(int(temperature*100)) / 100.0,
		CreatedAtMs:   t,
		UpdatedAtMs:   t,
		Content:       content,
		Likes:         0,
		IsDeleted:     false,
		Completed:     completed,
		SizeChars:     len(content),
		SizeTokens:    sizeTokens,
		TokensPerSec:  TokensPerSec,
		CharsPerSec:   CharsPerSec,
		ChatThreadPtr: nil,
	}

	err := db.DB().Create(&msg).Error
	if err != nil {
		return nil, errorx.NewErrInternalServerError("Failed to create message: " + err.Error())
	}

	msg.ChatThreadPtr = thread

	errx = msg.ChatThreadPtr.SetLastMsgAt()
	if errx != nil {
		return nil, errx
	}

	return &msg, nil
}

func CreateFileAttachmentMessage(
	ctx context.Context,
	groupID uuid.UUID,
	threadID uuid.UUID,
	content string,
	fileName string,
	fileExt string,
	originalFileSize int64,
	fullContentLength int64,
	truncatedContentLength int64,
	isTruncated bool,
) (*ChatMessage, errorx.Error) {
	if fullContentLength > int64(chatConfigInstance.UploadedFileMaxContentLength) {
		return nil, errorx.NewErrBadRequest("File content is too long")
	}

	if fullContentLength == 0 {
		return nil, errorx.NewErrBadRequest("File text content is empty")
	}

	var thread *ChatThread
	var errx errorx.Error

	if threadID == uuid.Nil {
		thread, errx = CreateChatThread(ctx, groupID, shortenMessage(content, 128), nil, nil, nil, []uuid.UUID{})
		if errx != nil {
			return nil, errx
		}
		threadID = thread.ID
	} else {
		thread, errx = GetChatThread(ctx, threadID)
		if errx != nil {
			return nil, errx
		}
		if groupID != uuid.Nil && groupID != thread.GroupID {
			return nil, errorx.NewErrBadRequest(fmt.Sprintf("Group ID '%s' does not match thread group ID '%s'", groupID.String(), thread.GroupID.String()))
		}
	}

	msg := ChatMessage{
		ID:            uuid.New(),
		TenantID:      auth.GetTenantID(),
		UserID:        auth.GetUserID(),
		ThreadID:      threadID,
		RoleId:        ChatMessageRoleIdUser,
		Role:          ChatMessageRoleNameUser,
		ModelName:     "",
		MaxTokens:     0,
		Temperature:   0,
		CreatedAtMs:   time.Now().UnixMilli(),
		UpdatedAtMs:   time.Now().UnixMilli(),
		Content:       content,
		Likes:         0,
		IsDeleted:     false,
		Completed:     true,
		SizeChars:     len(content),
		SizeTokens:    0,
		TokensPerSec:  0,
		CharsPerSec:   0,
		ChatThreadPtr: thread,
		// Store the attachment specific fields
		IsFileAttachment:       true,
		AttachedFileName:       fileName,
		AttachedFileExtension:  fileExt,
		FullContentLength:      fullContentLength,
		TruncatedContentLength: truncatedContentLength,
		OriginalFileSize:       originalFileSize,
		IsTruncated:            isTruncated,
	}

	err := db.DB().Create(&msg).Error
	if err != nil {
		return nil, errorx.NewErrInternalServerError("Failed to create special file attachment message: " + err.Error())
	}

	errx = msg.ChatThreadPtr.SetLastMsgAt()
	if errx != nil {
		return nil, errx
	}

	return &msg, nil
}

func UpdateMessage(ctx context.Context, messageID uuid.UUID, content string) (*ChatMessage, errorx.Error) {
	msg, errx := GetMessage(ctx, messageID)
	if errx != nil {
		return nil, errx
	}

	msg.Content = content

	errx = msg.ChatThreadPtr.SetLastMsgAt()
	if errx != nil {
		return nil, errx
	}

	errx = msg.DbSaveFields(&msg.Content)
	if errx != nil {
		return nil, errx
	}

	return msg, nil
}

func DeleteMessage(ctx context.Context, messageID uuid.UUID) errorx.Error {
	// Actual delete will be done by retention job
	message, errx := GetMessage(ctx, messageID)
	if errx != nil {
		return errx
	}

	message.IsDeleted = true
	return message.DbSaveFields(&message.IsDeleted)
}

func DeleteChatTail(ctx context.Context, threadID uuid.UUID, messageID uuid.UUID) errorx.Error {
	msg, errx := GetMessage(ctx, messageID)
	if errx != nil {
		return errx
	}

	// Use base query approach for the update operation
	updateQuery, errx := orm.GetBaseQuery(&ChatMessage{}, auth.GetTenantID(), auth.GetUserID(), nil)
	if errx != nil {
		return errx
	}

	result := updateQuery.Where(
		"thread_id = ? AND created_at_ms >= ? AND sequence_id >= ? AND is_deleted = ?",
		threadID, msg.CreatedAtMs, msg.SequenceID, false,
	).Update("is_deleted", true)

	if result.Error != nil {
		return errorx.NewErrInternalServerError("Failed to delete chat tail: " + result.Error.Error())
	}

	return nil
}
