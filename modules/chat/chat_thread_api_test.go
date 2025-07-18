package chat

import (
	"context"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/hypernetix/hyperspot/modules/llm"
)

func TestChatThreadAPI_FlagOperations(t *testing.T) {
	// Register mock service
	llm.RegisterMockServiceForUnitTests(context.Background())

	setupTestDB(t)
	ctx := context.Background()

	// Create a thread with default flags (all false)
	title := "Thread API Test"
	thread, err := CreateChatThread(ctx, uuid.Nil, title, nil, nil, nil, []uuid.UUID{})
	require.NoError(t, err, "Failed to create thread")

	// Verify default flags
	assert.False(t, thread.IsPinned, "IsPinned should be false by default")
	assert.False(t, thread.IsPublic, "IsPublic should be false by default")
	assert.False(t, thread.IsTemporary, "IsTemporary should be false by default")

	// Update all flags to true
	isPinned := true
	isPublic := true
	isTemporary := true
	updatedThread, err := UpdateChatThread(ctx, thread.ID, &thread.Title, &thread.GroupID, &isPinned, &isPublic, &isTemporary, []uuid.UUID{})
	require.NoError(t, err, "Failed to update thread flags")

	// Verify flags were updated
	assert.True(t, updatedThread.IsPinned, "IsPinned should be updated to true")
	assert.True(t, updatedThread.IsPublic, "IsPublic should be updated to true")
	assert.True(t, updatedThread.IsTemporary, "IsTemporary should be updated to true")

	// Retrieve the thread and verify flags persist in DB
	retrievedThread, err := GetChatThread(ctx, thread.ID)
	require.NoError(t, err, "Failed to get thread")
	assert.True(t, retrievedThread.IsPinned, "IsPinned should persist in DB")
	assert.True(t, retrievedThread.IsPublic, "IsPublic should persist in DB")
	assert.True(t, retrievedThread.IsTemporary, "IsTemporary should persist in DB")

	// Update only isPinned to false
	isPinned = false
	updatedThread, err = UpdateChatThread(ctx, thread.ID, &thread.Title, &thread.GroupID, &isPinned, nil, nil, []uuid.UUID{})
	require.NoError(t, err, "Failed to update isPinned flag")

	// Verify only isPinned was updated
	assert.False(t, updatedThread.IsPinned, "IsPinned should be updated to false")
	assert.True(t, updatedThread.IsPublic, "IsPublic should remain true")
	assert.True(t, updatedThread.IsTemporary, "IsTemporary should remain true")

	// Create a new thread with all flags set to true
	newThread, err := CreateChatThread(ctx, uuid.Nil, "New Thread With Flags", &isPinned, &isPublic, &isTemporary, []uuid.UUID{})
	require.NoError(t, err, "Failed to create thread with flags")

	// Verify flags were set correctly on creation
	assert.False(t, newThread.IsPinned, "IsPinned should be set to false")
	assert.True(t, newThread.IsPublic, "IsPublic should be set to true")
	assert.True(t, newThread.IsTemporary, "IsTemporary should be set to true")
}
