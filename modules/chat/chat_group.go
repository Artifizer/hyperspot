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

type ChatThreadGroup struct {
	SequenceID  int64     `json:"-" gorm:"primaryKey;autoIncrement:true"`
	TenantID    uuid.UUID `json:"tenant_id" gorm:"index"`
	UserID      uuid.UUID `json:"user_id" gorm:"index"`
	ID          uuid.UUID `json:"id" gorm:"primary_key"`
	CreatedAtMs int64     `json:"created_at" doc:"unix timestamp in milliseconds" gorm:"index"`
	UpdatedAtMs int64     `json:"updated_at" doc:"unix timestamp in milliseconds" gorm:"index"`
	LastMsgAtMs int64     `json:"last_msg_at" doc:"unix timestamp in milliseconds" gorm:"index"`
	Title       string    `json:"title"`
	IsActive    bool      `json:"is_active"`
	IsDeleted   bool      `json:"is_deleted"`
	IsPinned    bool      `json:"is_pinned"`
	IsPublic    bool      `json:"is_public"`
	IsTemporary bool      `json:"is_temporary" gorm:"index"`
}

func (g *ChatThreadGroup) DbSaveFields(fields ...interface{}) errorx.Error {
	g.UpdatedAtMs = time.Now().UnixMilli()
	fields = append(fields, &g.UpdatedAtMs)
	pkFields := map[string]interface{}{
		"id":        g.ID,
		"tenant_id": auth.GetTenantID(),
		"user_id":   auth.GetUserID(),
	}

	errx := orm.OrmUpdateObjFields(g, pkFields, fields...)
	if errx != nil {
		msg := fmt.Sprintf("failed to update chat thread group '%s': %s", g.ID.String(), errx.Error())
		logger.Error(msg)
		return errx
	}

	return nil
}

func (g *ChatThreadGroup) SetLastMsgAt() errorx.Error {
	g.LastMsgAtMs = time.Now().UnixMilli()
	return g.DbSaveFields(&g.LastMsgAtMs)
}

func ListChatThreadGroups(ctx context.Context, pageRequest *api.PageAPIRequest) ([]*ChatThreadGroup, errorx.Error) {
	query, errx := orm.GetBaseQuery(&ChatThreadGroup{}, auth.GetTenantID(), auth.GetUserID(), pageRequest)
	if errx != nil {
		return nil, errx
	}

	query = query.Where("is_deleted = ?", false)
	if pageRequest != nil && strings.HasPrefix(pageRequest.Order, "-") {
		query = query.Order("sequence_id desc")
	} else {
		query = query.Order("sequence_id asc")
	}

	var groups []*ChatThreadGroup
	if err := query.Find(&groups).Error; err != nil {
		return nil, errorx.NewErrInternalServerError("Failed to list groups: " + err.Error())
	}

	return groups, nil
}

func GetChatThreadGroup(ctx context.Context, groupID uuid.UUID) (*ChatThreadGroup, errorx.Error) {
	query, errx := orm.GetBaseQuery(&ChatThreadGroup{}, auth.GetTenantID(), auth.GetUserID(), nil)
	if errx != nil {
		return nil, errx
	}

	var group ChatThreadGroup
	err := query.Where("id = ? AND is_deleted = ?", groupID, false).First(&group).Error
	if err != nil {
		return nil, errorx.NewErrNotFound(fmt.Sprintf("Group '%s' not found", groupID.String()))
	}

	return &group, nil
}

func CreateChatThreadGroup(
	ctx context.Context,
	title string,
	isPinned *bool,
	isPublic *bool,
	isTemporary *bool,
) (*ChatThreadGroup, errorx.Error) {
	group := ChatThreadGroup{
		ID:          uuid.New(),
		TenantID:    auth.GetTenantID(),
		UserID:      auth.GetUserID(),
		CreatedAtMs: time.Now().UnixMilli(),
		UpdatedAtMs: time.Now().UnixMilli(),
		LastMsgAtMs: time.Now().UnixMilli(),
		Title:       title,
		IsActive:    true,
		IsDeleted:   false,
		IsPinned:    false,
		IsPublic:    false,
		IsTemporary: false,
	}

	if isPublic != nil {
		group.IsPublic = *isPublic
	}
	if isPinned != nil {
		group.IsPinned = *isPinned
	}
	if isTemporary != nil {
		group.IsTemporary = *isTemporary
	}

	err := db.DB().Create(&group).Error
	if err != nil {
		return nil, errorx.NewErrInternalServerError("Failed to create group: " + err.Error())
	}

	return &group, nil
}

func UpdateChatThreadGroup(
	ctx context.Context,
	groupID uuid.UUID,
	title *string,
	isPinned *bool,
	isPublic *bool,
	isTemporary *bool,
) (*ChatThreadGroup, errorx.Error) {
	group, errx := GetChatThreadGroup(ctx, groupID)
	if errx != nil {
		return nil, errx
	}

	if title != nil {
		group.Title = *title
	}
	if isPinned != nil {
		group.IsPinned = *isPinned
	}
	if isPublic != nil {
		group.IsPublic = *isPublic
	}
	if isTemporary != nil {
		group.IsTemporary = *isTemporary
	}

	err := group.DbSaveFields(&group.Title, &group.IsPinned, &group.IsPublic, &group.IsTemporary)
	if err != nil {
		return nil, errorx.NewErrInternalServerError("Failed to update group: " + err.Error())
	}

	return group, nil
}

func DeleteChatThreadGroup(ctx context.Context, groupID uuid.UUID) errorx.Error {
	group, errx := GetChatThreadGroup(ctx, groupID)
	if errx != nil {
		return errx
	}

	group.IsDeleted = true
	return group.DbSaveFields(&group.IsDeleted)
}
