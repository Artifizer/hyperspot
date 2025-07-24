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
)

type ChatThread struct {
	SequenceID    int64            `json:"-" gorm:"primaryKey;autoIncrement:true"`
	TenantID      uuid.UUID        `json:"tenant_id" gorm:"index"`
	UserID        uuid.UUID        `json:"user_id" gorm:"index"`
	ID            uuid.UUID        `json:"id" gorm:"index"`
	GroupID       uuid.UUID        `json:"group_id" gorm:"index"`
	CreatedAtMs   int64            `json:"created_at" gorm:"autoCreateTime;index" doc:"unix timestamp in milliseconds"`
	UpdatedAtMs   int64            `json:"updated_at" gorm:"autoUpdateTime;index" doc:"unix timestamp in milliseconds"`
	LastMsgAtMs   int64            `json:"last_msg_at" gorm:"index" doc:"unix timestamp in milliseconds"`
	SizeChars     int              `json:"size_chars"`
	SizeTokens    int              `json:"size_tokens"`
	Title         string           `json:"title"`
	IsActive      bool             `json:"is_active"`
	IsDeleted     bool             `json:"is_deleted"`
	IsPinned      bool             `json:"is_pinned"`
	IsPublic      bool             `json:"is_public"`
	IsTemporary   bool             `json:"is_temporary" gorm:"index"`
	GroupPtr      *ChatThreadGroup `json:"-" gorm:"-"`
	SystemPrompts []*SystemPrompt  `json:"system_prompts,omitempty" gorm:"-"`
}

func (t *ChatThread) DbSaveFields(fields ...interface{}) errorx.Error {
	t.UpdatedAtMs = time.Now().UnixMilli()
	fields = append(fields, &t.UpdatedAtMs)
	pkFields := map[string]interface{}{
		"id":        t.ID,
		"tenant_id": auth.GetTenantID(),
		"user_id":   auth.GetUserID(),
	}

	errx := orm.OrmUpdateObjFields(t, pkFields, fields...)
	if errx != nil {
		msg := fmt.Sprintf("failed to update chat thread: %s", errx.Error())
		t.LogError(msg)
		return errx
	}

	return nil
}

func (t *ChatThread) SetLastMsgAt() errorx.Error {
	t.LastMsgAtMs = time.Now().UnixMilli()
	group := t.GroupPtr
	if group != nil {
		errx := group.SetLastMsgAt()
		if errx != nil {
			return errx
		}
	}
	return t.DbSaveFields(&t.LastMsgAtMs)
}

func ListChatThreads(ctx context.Context, groupID uuid.UUID, pageRequest *api.PageAPIRequest) ([]ChatThread, errorx.Error) {
	var group *ChatThreadGroup
	var errx errorx.Error
	if groupID != uuid.Nil {
		group, errx = GetChatThreadGroup(ctx, groupID)
		if errx != nil {
			return nil, errx
		}
	} else {
		group = nil
	}

	query, errx := orm.GetBaseQuery(&ChatThread{}, auth.GetTenantID(), auth.GetUserID(), pageRequest)
	if errx != nil {
		return nil, errx
	}

	query = query.Where("group_id = ? AND is_deleted = ?", groupID, false)

	if pageRequest != nil && strings.HasPrefix(pageRequest.Order, "-") {
		query = query.Order("sequence_id desc")
	} else {
		query = query.Order("sequence_id asc")
	}

	threads := []ChatThread{}
	err := query.Find(&threads).Error
	if err != nil {
		return nil, errorx.NewErrInternalServerError("Failed to list chat threads: " + err.Error())
	}

	for i, thread := range threads {
		if group != nil && thread.GroupID == group.ID {
			threads[i].GroupPtr = group
		} else if group != nil {
			panic("thread group id mismatch")
		}
	}

	return threads, nil
}

func GetChatThread(ctx context.Context, threadID uuid.UUID) (*ChatThread, errorx.Error) {
	if threadID == uuid.Nil {
		return nil, errorx.NewErrNotFound("Chat thread not found")
	}

	query, errx := orm.GetBaseQuery(&ChatThread{}, auth.GetTenantID(), auth.GetUserID(), nil)
	if errx != nil {
		return nil, errx
	}

	var thread ChatThread
	err := query.Where("id = ?", threadID).First(&thread).Error
	if err != nil {
		return nil, errorx.NewErrNotFound(fmt.Sprintf("Chat thread '%s' not found", threadID.String()))
	}

	if thread.GroupID != uuid.Nil {
		group, errx := GetChatThreadGroup(ctx, thread.GroupID)
		if errx != nil {
			return nil, errx
		}
		thread.GroupPtr = group
	}

	// Load system prompts attached to this thread
	systemPrompts, errx := GetSystemPromptsForThread(ctx, threadID)
	if errx != nil {
		// Log error but don't fail - system prompts are optional
		fmt.Printf("Warning: Failed to load system prompts for thread %s: %v\n", threadID, errx)
	} else {
		thread.SystemPrompts = systemPrompts
	}

	return &thread, nil
}

func CreateChatThread(
	ctx context.Context,
	groupID uuid.UUID,
	title string,
	isPinned *bool,
	isPublic *bool,
	isTemporary *bool,
	systemPromptIDs []uuid.UUID,
) (*ChatThread, errorx.Error) {
	if groupID != uuid.Nil {
		_, errx := GetChatThreadGroup(ctx, groupID)
		if errx != nil {
			return nil, errx
		}
	}

	thread := ChatThread{
		ID:          uuid.New(),
		TenantID:    auth.GetTenantID(),
		UserID:      auth.GetUserID(),
		GroupID:     groupID,
		Title:       title,
		CreatedAtMs: time.Now().UnixMilli(),
		UpdatedAtMs: time.Now().UnixMilli(),
		LastMsgAtMs: time.Now().UnixMilli(),
		IsActive:    true,
		IsDeleted:   false,
		IsPinned:    false,
		IsPublic:    false,
		IsTemporary: false,
	}

	if isTemporary != nil {
		thread.IsTemporary = *isTemporary
	}
	if isPublic != nil {
		thread.IsPublic = *isPublic
	}
	if isPinned != nil {
		thread.IsPinned = *isPinned
	}

	err := db.DB().Create(&thread).Error
	if err != nil {
		return nil, errorx.NewErrInternalServerError("Failed to create chat thread: " + err.Error())
	}

	// Attach system prompts if provided
	if len(systemPromptIDs) > 0 {
		// Filter out nil UUIDs
		var validPromptIDs []uuid.UUID
		for _, promptID := range systemPromptIDs {
			if promptID != uuid.Nil {
				validPromptIDs = append(validPromptIDs, promptID)
			}
		}

		if len(validPromptIDs) > 0 {
			errx := AttachSystemPromptsToThread(ctx, thread.ID, validPromptIDs)
			if errx != nil {
				// If we fail to attach system prompts, we should still return the thread
				// but log the error
				fmt.Printf("Warning: Failed to attach system prompts to thread %s: %v\n", thread.ID, errx)
			}
		}

		// Reload the thread to get the attached system prompts
		updatedThread, errx := GetChatThread(ctx, thread.ID)
		if errx != nil {
			return &thread, nil // Return the thread even if we can't reload it
		}
		return updatedThread, nil
	}

	return &thread, nil
}

func UpdateChatThread(
	ctx context.Context,
	threadID uuid.UUID,
	title *string,
	groupID *uuid.UUID,
	isPinned *bool,
	isPublic *bool,
	isTemporary *bool,
	systemPromptIDs []uuid.UUID,
) (*ChatThread, errorx.Error) {
	thread, errx := GetChatThread(ctx, threadID)
	if errx != nil {
		return nil, errorx.NewErrNotFound("Chat thread not found")
	}

	if thread.TenantID != auth.GetTenantID() || thread.UserID != auth.GetUserID() {
		return nil, errorx.NewErrForbidden("You are not allowed to update this thread")
	}

	if groupID != nil {
		if *groupID != uuid.Nil {
			group, errx := GetChatThreadGroup(ctx, *groupID)
			if errx != nil {
				return nil, errx
			}

			thread.GroupID = group.ID
			thread.GroupPtr = group
		} else {
			thread.GroupID = uuid.Nil
		}
	}

	// Update fields
	if title != nil {
		thread.Title = *title
	}

	if isPinned != nil {
		thread.IsPinned = *isPinned
	}
	if isPublic != nil {
		thread.IsPublic = *isPublic
	}
	if isTemporary != nil {
		thread.IsTemporary = *isTemporary
	}

	thread.LastMsgAtMs = time.Now().UnixMilli()

	// Handle system prompts update
	if systemPromptIDs != nil {
		// First, detach all existing system prompts
		errx := DetachAllSystemPromptsFromThread(ctx, threadID)
		if errx != nil {
			return nil, errx
		}

		// Filter out nil UUIDs and attach the new ones
		var validPromptIDs []uuid.UUID
		for _, promptID := range systemPromptIDs {
			if promptID != uuid.Nil {
				validPromptIDs = append(validPromptIDs, promptID)
			}
		}

		if len(validPromptIDs) > 0 {
			errx := AttachSystemPromptsToThread(ctx, threadID, validPromptIDs)
			if errx != nil {
				return nil, errx
			}
		}
	}

	errx = thread.DbSaveFields(&thread.Title, &thread.GroupID, &thread.LastMsgAtMs, &thread.IsPinned, &thread.IsPublic, &thread.IsTemporary)
	if errx != nil {
		return nil, errx
	}

	// Reload the thread to get the updated system prompts
	return GetChatThread(ctx, threadID)
}

func DeleteChatThread(ctx context.Context, threadID uuid.UUID) errorx.Error {
	thread, errx := GetChatThread(ctx, threadID)
	if errx != nil {
		return errx
	}

	thread.IsDeleted = true
	return thread.DbSaveFields(&thread.IsDeleted)
}
