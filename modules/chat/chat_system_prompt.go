package chat

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"

	"github.com/hypernetix/hyperspot/libs/api"
	"github.com/hypernetix/hyperspot/libs/auth"
	"github.com/hypernetix/hyperspot/libs/db"
	"github.com/hypernetix/hyperspot/libs/errorx"
	"github.com/hypernetix/hyperspot/libs/orm"
	"github.com/hypernetix/hyperspot/libs/utils"
	"github.com/hypernetix/hyperspot/modules/file_parser"
	"github.com/hypernetix/hyperspot/modules/syscap"
)

// SystemPrompt represents a system prompt that can be attached to chat threads
type SystemPrompt struct {
	SequenceID   int64     `json:"-" gorm:"primaryKey;autoIncrement:true"`
	ID           uuid.UUID `json:"id" gorm:"index"`
	TenantID     uuid.UUID `json:"-" gorm:"index"`
	UserID       uuid.UUID `json:"user_id" gorm:"index"`
	Name         string    `json:"name"`
	Content      string    `json:"content"`
	CreatedAtMs  int64     `json:"created_at" gorm:"index" doc:"unix timestamp in milliseconds"`
	UpdatedAtMs  int64     `json:"updated_at" gorm:"index" doc:"unix timestamp in milliseconds"`
	UIMeta       string    `json:"ui_meta" doc:"UI json metadata, not used by the backend"`
	IsDeleted    bool      `json:"-" gorm:"index"`
	IsDefault    bool      `json:"is_default" gorm:"index" doc:"if true, this prompt will be automatically attached to every thread"`
	FilePath     string    `json:"file_path" doc:"path to the file that contains the system prompt"`
	FileChecksum string    `json:"-"`
	AutoUpdate   bool      `json:"auto_update" doc:"if true, system prompt needs to be updated on every chat message"`
}

// ChatThreadSystemPrompt represents the many-to-many relationship between ChatThread and SystemPrompt
type ChatThreadSystemPrompt struct {
	SequenceID     int64     `json:"-" gorm:"primaryKey;autoIncrement:true"`
	TenantID       uuid.UUID `json:"-" gorm:"index"`
	UserID         uuid.UUID `json:"-" gorm:"index"`
	ThreadID       uuid.UUID `json:"thread_id" gorm:"index"`
	SystemPromptID uuid.UUID `json:"system_prompt_id" gorm:"index"`
	CreatedAtMs    int64     `json:"created_at" gorm:"index" doc:"unix timestamp in milliseconds"`
	IsDeleted      bool      `json:"-" gorm:"index"`
}

func (p *SystemPrompt) DbSaveFields(fields ...interface{}) errorx.Error {
	p.UpdatedAtMs = time.Now().UnixMilli()
	fields = append(fields, &p.UpdatedAtMs)

	pkFields := map[string]interface{}{
		"id":        p.ID,
		"tenant_id": p.TenantID,
		"user_id":   p.UserID,
	}

	errx := orm.OrmUpdateObjFields(p, pkFields, fields...)
	if errx != nil {
		msg := fmt.Sprintf("failed to update system prompt: %s", errx.Error())
		return errorx.NewErrInternalServerError(msg)
	}

	return nil
}

func (p *SystemPrompt) GetContent() string {
	if p.AutoUpdate && p.FilePath != "" {
		fields := []interface{}{}
		if errx := p.updateSystemPromptFromFile(&fields); errx != nil {
			return ""
		}

		if len(fields) > 0 {
			p.DbSaveFields(fields...)
		}
	}
	return p.Content
}

// updateSystemPromptFromFile() updates the system prompt from a file if needed
// i.e. if file checksum changed. The fields parameter is used to return the fields that
// were updated for later saving to the database
func (p *SystemPrompt) updateSystemPromptFromFile(fields *[]interface{}) errorx.Error {
	if !syscap.SysCapIsPresent(syscap.CategoryModule, syscap.SysCapaModuleDesktop) {
		return errorx.NewErrBadRequest("System prompt creation from file is not allowed")
	}

	if p.FilePath == "" {
		return errorx.NewErrBadRequest("System prompt file is not set, update from file is not allowed")
	}

	if !utils.FileExists(p.FilePath) {
		return errorx.NewErrNotFound("file does not exist or can't be accessed: %s", p.FilePath)
	}

	fileChecksum, err := utils.GetFileChecksum(p.FilePath)
	if err != nil {
		return errorx.NewErrInternalServerError("Failed to get file checksum: " + err.Error())
	}

	if fileChecksum != p.FileChecksum {
		doc, errx := file_parser.ParseLocalDocument(p.FilePath)
		if errx != nil {
			return errx
		}

		p.Content = doc.GetTextContent()
		p.FileChecksum = doc.FileChecksum
		if fields != nil {
			*fields = append(*fields, &p.Content)
			*fields = append(*fields, &p.FileChecksum)
		}
	}

	return nil
}

// CreateSystemPrompt creates a new system prompt
func CreateSystemPrompt(
	ctx context.Context,
	name string,
	content string,
	uiMeta string,
	isDefault bool,
	fromFile string,
	AutoUpdate bool,
) (*SystemPrompt, errorx.Error) {
	nowMs := time.Now().UnixMilli()

	if AutoUpdate && fromFile == "" {
		return nil, errorx.NewErrBadRequest("System prompt file is not set, update from file is not allowed")
	}

	prompt := &SystemPrompt{
		ID:          uuid.New(),
		TenantID:    auth.GetTenantID(),
		UserID:      auth.GetUserID(),
		Name:        name,
		Content:     content,
		UIMeta:      uiMeta,
		CreatedAtMs: nowMs,
		UpdatedAtMs: nowMs,
		IsDeleted:   false,
		IsDefault:   isDefault,
		FilePath:    fromFile,
		AutoUpdate:  AutoUpdate,
	}

	if fromFile != "" {
		errx := prompt.updateSystemPromptFromFile(nil)
		if errx != nil {
			return nil, errx
		}
	}

	err := db.DB().Create(&prompt).Error
	if err != nil {
		return nil, errorx.NewErrInternalServerError("Failed to create system prompt: " + err.Error())
	}

	return prompt, nil
}

// GetSystemPrompt retrieves a system prompt by ID
func GetSystemPrompt(ctx context.Context, promptID uuid.UUID) (*SystemPrompt, errorx.Error) {
	query, errx := orm.GetBaseQueryTUU(&SystemPrompt{}, nil)
	if errx != nil {
		return nil, errx
	}

	var prompt SystemPrompt
	err := query.Where("id = ?", promptID).First(&prompt).Error
	if err != nil {
		return nil, errorx.NewErrNotFound(fmt.Sprintf("System prompt '%s' not found", promptID.String()))
	}

	// Update content on any prompt fetch
	prompt.Content = prompt.GetContent()

	return &prompt, nil
}

// ListSystemPrompts retrieves system prompts for the current user with pagination support
func ListSystemPrompts(ctx context.Context, pageRequest *api.PageAPIRequest) ([]*SystemPrompt, errorx.Error) {
	query, errx := orm.GetBaseQueryTUU(&SystemPrompt{}, pageRequest)
	if errx != nil {
		return nil, errx
	}

	if pageRequest != nil && strings.HasPrefix(pageRequest.Order, "-") {
		query = query.Order("updated_at_ms desc")
	} else {
		query = query.Order("updated_at_ms asc")
	}

	var prompts []*SystemPrompt

	if err := query.Find(&prompts).Error; err != nil {
		return nil, errorx.NewErrInternalServerError("Failed to list system prompts: " + err.Error())
	}

	return prompts, nil
}

// UpdateSystemPrompt updates an existing system prompt
func UpdateSystemPrompt(
	ctx context.Context,
	promptID uuid.UUID,
	name *string,
	content *string,
	uiMeta *string,
	isDefault *bool,
	filePath *string,
	autoUpdate *bool,
) (*SystemPrompt, errorx.Error) {
	prompt, errx := GetSystemPrompt(ctx, promptID)
	if errx != nil {
		return nil, errx
	}

	var fields []interface{}

	if isDefault != nil {
		if *isDefault != prompt.IsDefault {
			prompt.IsDefault = *isDefault
			fields = append(fields, &prompt.IsDefault)
		}
	}

	if filePath != nil {
		if *filePath == "" && prompt.FilePath != "" {
			// If file path is removed, also remove content
			prompt.Content = ""
			fields = append(fields, &prompt.Content)
		}
		prompt.FilePath = *filePath
		fields = append(fields, &prompt.FilePath)
		if *filePath == "" {
			prompt.AutoUpdate = false
			fields = append(fields, &prompt.AutoUpdate)
			autoUpdate = &prompt.AutoUpdate
		}
	}

	if prompt.FilePath != "" {
		if errx := prompt.updateSystemPromptFromFile(&fields); errx != nil {
			return nil, errx
		}
	}

	if autoUpdate != nil {
		if *autoUpdate && prompt.FilePath == "" {
			return nil, errorx.NewErrBadRequest("System prompt file is not set, update from file is not allowed")
		}
		if prompt.AutoUpdate != *autoUpdate {
			prompt.AutoUpdate = *autoUpdate
			fields = append(fields, &prompt.AutoUpdate)
		}
	}

	// Only save fields that were provided in the request
	if name != nil {
		prompt.Name = *name
		fields = append(fields, &prompt.Name)
	}
	if content != nil {
		prompt.Content = *content
		fields = append(fields, &prompt.Content)
	}
	if uiMeta != nil {
		prompt.UIMeta = *uiMeta
		fields = append(fields, &prompt.UIMeta)
	}

	if len(fields) == 0 {
		// No fields to update
		return prompt, nil
	}

	errx = prompt.DbSaveFields(fields...)
	if errx != nil {
		return nil, errx
	}

	return prompt, nil
}

// DeleteSystemPrompt soft deletes a system prompt
func DeleteSystemPrompt(ctx context.Context, promptID uuid.UUID) errorx.Error {
	prompt, errx := GetSystemPrompt(ctx, promptID)
	if errx != nil {
		return errx
	}

	prompt.IsDeleted = true
	return prompt.DbSaveFields(&prompt.IsDeleted)
}

// AttachSystemPromptsToThread attaches multiple system prompts to a chat thread atomically
func AttachSystemPromptsToThread(ctx context.Context, threadID uuid.UUID, systemPromptIDs []uuid.UUID) errorx.Error {
	if len(systemPromptIDs) == 0 {
		return nil // Nothing to attach
	}

	// Verify the thread exists and belongs to the user (direct check to avoid recursion)
	query, errx := orm.GetBaseQueryTUU(&ChatThread{}, nil)
	if errx != nil {
		return errx
	}

	var thread ChatThread
	err := query.Where("id = ?", threadID).First(&thread).Error
	if err != nil {
		return errorx.NewErrNotFound(fmt.Sprintf("Chat thread '%s' not found", threadID.String()))
	}

	// Verify all system prompts exist and belong to the user
	for _, systemPromptID := range systemPromptIDs {
		_, errx = GetSystemPrompt(ctx, systemPromptID)
		if errx != nil {
			return errx
		}
	}

	// Check which relationships already exist to avoid duplicates
	relationQuery, errx := orm.GetBaseQueryTUU(&ChatThreadSystemPrompt{}, nil)
	if errx != nil {
		return errx
	}

	var existingRelationships []ChatThreadSystemPrompt
	err = relationQuery.Where(
		"thread_id = ? AND system_prompt_id IN ? AND is_deleted = ?",
		threadID, systemPromptIDs, false,
	).Find(&existingRelationships).Error

	if err != nil {
		return errorx.NewErrInternalServerError("Failed to check existing relationships: " + err.Error())
	}

	// Create a map of existing prompt IDs for quick lookup
	existingPromptIDs := make(map[uuid.UUID]bool)
	for _, rel := range existingRelationships {
		existingPromptIDs[rel.SystemPromptID] = true
	}

	// Filter out already attached prompts
	var newPromptIDs []uuid.UUID
	for _, promptID := range systemPromptIDs {
		if !existingPromptIDs[promptID] {
			newPromptIDs = append(newPromptIDs, promptID)
		}
	}

	if len(newPromptIDs) == 0 {
		return nil // All prompts are already attached
	}

	// Start a transaction for atomic operation
	tx := db.DB().Begin()
	if tx.Error != nil {
		return errorx.NewErrInternalServerError("Failed to start transaction: " + tx.Error.Error())
	}
	defer tx.Rollback()

	// Create all new relationships
	now := time.Now().UnixMilli()
	for _, promptID := range newPromptIDs {
		relationship := ChatThreadSystemPrompt{
			TenantID:       auth.GetTenantID(),
			UserID:         auth.GetUserID(),
			ThreadID:       threadID,
			SystemPromptID: promptID,
			CreatedAtMs:    now,
			IsDeleted:      false,
		}

		err = tx.Create(&relationship).Error
		if err != nil {
			return errorx.NewErrInternalServerError("Failed to attach system prompt to thread: " + err.Error())
		}
	}

	// Commit the transaction
	if err = tx.Commit().Error; err != nil {
		return errorx.NewErrInternalServerError("Failed to commit transaction: " + err.Error())
	}

	return nil
}

// DetachSystemPromptsFromThread detaches multiple system prompts from a chat thread atomically
func DetachSystemPromptsFromThread(ctx context.Context, threadID uuid.UUID, systemPromptIDs []uuid.UUID) errorx.Error {
	if len(systemPromptIDs) == 0 {
		return nil // Nothing to detach
	}

	// Verify the thread exists and belongs to the user (direct check to avoid recursion)
	query, errx := orm.GetBaseQueryTUU(&ChatThread{}, nil)
	if errx != nil {
		return errx
	}

	var thread ChatThread
	err := query.Where("id = ?", threadID).First(&thread).Error
	if err != nil {
		return errorx.NewErrNotFound(fmt.Sprintf("Chat thread '%s' not found", threadID.String()))
	}

	// Start a transaction for atomic operation
	tx := db.DB().Begin()
	if tx.Error != nil {
		return errorx.NewErrInternalServerError("Failed to start transaction: " + tx.Error.Error())
	}
	defer tx.Rollback()

	// Use the transaction connection for the update
	result := tx.Model(&ChatThreadSystemPrompt{}).Where(
		"thread_id = ? AND system_prompt_id IN ? AND tenant_id = ? AND user_id = ? AND is_deleted = ?",
		threadID, systemPromptIDs, auth.GetTenantID(), auth.GetUserID(), false,
	).Update("is_deleted", true)

	if result.Error != nil {
		return errorx.NewErrInternalServerError("Failed to detach system prompts from thread: " + result.Error.Error())
	}

	// Commit the transaction
	if err = tx.Commit().Error; err != nil {
		return errorx.NewErrInternalServerError("Failed to commit transaction: " + err.Error())
	}

	return nil
}

// AttachSystemPromptToThread attaches a single system prompt to a chat thread (wrapper for backward compatibility)
func AttachSystemPromptToThread(ctx context.Context, threadID uuid.UUID, systemPromptID uuid.UUID) errorx.Error {
	return AttachSystemPromptsToThread(ctx, threadID, []uuid.UUID{systemPromptID})
}

// DetachSystemPromptFromThread detaches a single system prompt from a chat thread (wrapper for backward compatibility)
func DetachSystemPromptFromThread(ctx context.Context, threadID uuid.UUID, systemPromptID uuid.UUID) errorx.Error {
	return DetachSystemPromptsFromThread(ctx, threadID, []uuid.UUID{systemPromptID})
}

// DetachAllSystemPromptsFromThread detaches all system prompts from a chat thread
func DetachAllSystemPromptsFromThread(ctx context.Context, threadID uuid.UUID) errorx.Error {
	// Verify the thread exists and belongs to the user (direct check to avoid recursion)
	query, errx := orm.GetBaseQueryTUU(&ChatThread{}, nil)
	if errx != nil {
		return errx
	}

	var thread ChatThread
	err := query.Where("id = ?", threadID).First(&thread).Error
	if err != nil {
		return errorx.NewErrNotFound(fmt.Sprintf("Chat thread '%s' not found", threadID.String()))
	}

	// Soft delete all relationships for this thread using base query approach
	updateQuery, errx := orm.GetBaseQueryTUU(&ChatThreadSystemPrompt{}, nil)
	if errx != nil {
		return errx
	}

	result := updateQuery.Where(
		"thread_id = ?",
		threadID,
	).Update("is_deleted", true)

	if result.Error != nil {
		return errorx.NewErrInternalServerError("Failed to detach system prompts from thread: " + result.Error.Error())
	}

	return nil
}

// GetSystemPromptsForThread retrieves all system prompts attached to a thread
// and all global system prompts with is_default=true
func GetSystemPromptsForThread(ctx context.Context, threadID uuid.UUID) ([]*SystemPrompt, errorx.Error) {
	// Verify the thread exists and belongs to the user (without loading system prompts to avoid recursion)
	query, errx := orm.GetBaseQueryTUU(&ChatThread{}, nil)
	if errx != nil {
		return nil, errx
	}

	var thread ChatThread
	err := query.Where("id = ? AND is_deleted = ?", threadID, false).First(&thread).Error
	if err != nil {
		return nil, errorx.NewErrNotFound(fmt.Sprintf("Chat thread '%s' not found", threadID.String()))
	}

	// Get attached system prompt IDs
	var promptIDs []uuid.UUID
	relationQuery, errx := orm.GetBaseQueryTUU(&ChatThreadSystemPrompt{}, nil)
	if errx != nil {
		return nil, errx
	}

	var relationships []ChatThreadSystemPrompt
	err = relationQuery.Where(
		"thread_id = ? AND is_deleted = ?",
		threadID,
		false,
	).Find(&relationships).Error

	if err != nil {
		return nil, errorx.NewErrInternalServerError("Failed to get system prompts for thread: " + err.Error())
	}

	// Get the system prompt IDs from relationships
	for _, rel := range relationships {
		promptIDs = append(promptIDs, rel.SystemPromptID)
	}

	// Get the system prompts using orm.GetBaseQuery
	promptQuery, errx := orm.GetBaseQueryTUU(&SystemPrompt{}, nil)
	if errx != nil {
		return nil, errx
	}

	var whereClause string
	var queryParams []interface{}

	if len(promptIDs) > 0 {
		// Get both attached prompts and default prompts
		whereClause = "((id IN ?) OR (is_default = ? AND tenant_id = ? AND user_id = ?)) AND is_deleted = ?"
		queryParams = []interface{}{promptIDs, true, auth.GetTenantID(), auth.GetUserID(), false}
	} else {
		// Get only default prompts when no prompts are attached
		whereClause = "is_default = ? AND tenant_id = ? AND user_id = ? AND is_deleted = ?"
		queryParams = []interface{}{true, auth.GetTenantID(), auth.GetUserID(), false}
	}

	var prompts []*SystemPrompt
	err = promptQuery.Where(whereClause, queryParams...).Find(&prompts).Error

	if err != nil {
		return nil, errorx.NewErrInternalServerError("Failed to get system prompts: " + err.Error())
	}

	return prompts, nil
}

// GetThreadsForSystemPrompt retrieves all threads that have a specific system prompt attached
func GetThreadsForSystemPrompt(ctx context.Context, systemPromptID uuid.UUID) ([]*ChatThread, errorx.Error) {
	// Verify the system prompt exists and belongs to the user
	_, errx := GetSystemPrompt(ctx, systemPromptID)
	if errx != nil {
		return nil, errx
	}

	relationQuery, errx := orm.GetBaseQueryTUU(&ChatThreadSystemPrompt{}, nil)
	if errx != nil {
		return nil, errx
	}

	var relationships []ChatThreadSystemPrompt
	err := relationQuery.Where(
		"system_prompt_id = ?",
		systemPromptID,
	).Find(&relationships).Error

	if err != nil {
		return nil, errorx.NewErrInternalServerError("Failed to get threads for system prompt: " + err.Error())
	}

	if len(relationships) == 0 {
		return []*ChatThread{}, nil
	}

	// Get the thread IDs
	var threadIDs []uuid.UUID
	for _, rel := range relationships {
		threadIDs = append(threadIDs, rel.ThreadID)
	}

	// Get the threads using orm.GetBaseQuery
	threadQuery, errx := orm.GetBaseQueryTUU(&ChatThread{}, nil)
	if errx != nil {
		return nil, errx
	}

	var threads []*ChatThread
	err = threadQuery.Where(
		"id IN ?",
		threadIDs,
	).Find(&threads).Error

	if err != nil {
		return nil, errorx.NewErrInternalServerError("Failed to get threads: " + err.Error())
	}

	return threads, nil
}

func systemPromptsToChatMessage(systemPrompts []*SystemPrompt) *ChatMessage {
	var systemContents []string

	if len(systemPrompts) == 0 {
		return nil
	}

	for _, systemPrompt := range systemPrompts {
		systemContents = append(systemContents, systemPrompt.GetContent())
	}
	msg := ChatMessage{
		Role:    ChatMessageRoleNameSystem,
		Content: strings.Join(systemContents, "\n\n"),
	}
	return &msg
}
