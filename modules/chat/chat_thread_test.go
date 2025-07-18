package chat

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/hypernetix/hyperspot/libs/api"
	"github.com/hypernetix/hyperspot/libs/auth"
	"github.com/hypernetix/hyperspot/libs/db"
	"github.com/hypernetix/hyperspot/modules/llm"
)

// Helper functions are already defined in chat_msg_test.go, but we'll add a createTestGroup function
func createTestGroup(t *testing.T) *ChatThreadGroup {
	t.Helper()
	group := &ChatThreadGroup{
		ID:          uuid.New(),
		TenantID:    auth.GetTenantID(),
		UserID:      auth.GetUserID(),
		Title:       "Test Group",
		CreatedAtMs: time.Now().UnixMilli(),
		UpdatedAtMs: time.Now().UnixMilli(),
		IsActive:    true,
		IsDeleted:   false,
		IsPinned:    false,
		IsPublic:    true,
		IsTemporary: false,
	}

	result := db.DB().Create(group)
	require.NoError(t, result.Error, "Failed to create test group")
	return group
}

// Test thread object methods
func TestChatThreadDbSaveFields(t *testing.T) {
	setupTestDB(t)
	ctx := context.Background()

	thread, err := CreateChatThread(ctx, uuid.Nil, "Test Thread", nil, nil, nil, []uuid.UUID{})
	require.NoError(t, err, "Failed to create test thread")

	// Update fields
	newTitle := "Updated Title"
	thread.Title = newTitle
	thread.IsPinned = true
	thread.IsPublic = true
	thread.IsTemporary = true

	// Save specific fields
	err = thread.DbSaveFields(&thread.Title, &thread.IsPinned, &thread.IsPublic, &thread.IsTemporary)
	require.NoError(t, err, "DbSaveFields should not return an error")

	// Verify the changes were saved to the database
	updatedThread, err := GetChatThread(ctx, thread.ID)
	require.NoError(t, err, "Failed to get updated thread")
	assert.Equal(t, newTitle, updatedThread.Title, "Title should be updated")
	assert.True(t, updatedThread.IsPinned, "IsPinned should be updated")
	assert.True(t, updatedThread.IsPublic, "IsPublic should be updated")
	assert.True(t, updatedThread.IsTemporary, "IsTemporary should be updated")
}

func TestChatThreadSetLastMsgAt(t *testing.T) {
	setupTestDB(t)
	ctx := context.Background()

	thread, err := CreateChatThread(ctx, uuid.Nil, "Test Thread", nil, nil, nil, []uuid.UUID{})
	require.NoError(t, err, "Failed to create test thread")

	// Record the original timestamp
	originalLastMsgAt := thread.LastMsgAtMs

	// Wait a moment to ensure timestamp changes
	time.Sleep(10 * time.Millisecond)

	// Update the timestamp
	err = thread.SetLastMsgAt()
	require.NoError(t, err, "SetLastMsgAt should not return an error")

	// Verify the timestamp was updated
	assert.GreaterOrEqual(t, thread.LastMsgAtMs, originalLastMsgAt, "LastMsgAt should be updated")

	// Verify it was saved to the database
	updatedThread, err := GetChatThread(ctx, thread.ID)
	require.NoError(t, err, "Failed to get updated thread")
	assert.Equal(t, thread.LastMsgAtMs, updatedThread.LastMsgAtMs, "LastMsgAt should be updated in DB")
}

func TestListChatThreads(t *testing.T) {
	setupTestDB(t)
	ctx := context.Background()

	// Create a group
	group := createTestGroup(t)

	// Create multiple threads in the same group with different flag combinations
	threadCount := 5
	threads := make([]*ChatThread, threadCount)
	for i := 0; i < threadCount; i++ {
		// Set different combinations of flags for different threads
		isPinned := (i%2 == 0)
		isPublic := (i%3 == 0)
		isTemporary := (i%4 == 0)

		thread, err := CreateChatThread(ctx, group.ID, "Test Thread "+string(rune(i+65)), &isPinned, &isPublic, &isTemporary, []uuid.UUID{})
		require.NoError(t, err, "Failed to create test thread")

		// Update thread to assign to the group
		thread, err = UpdateChatThread(ctx, thread.ID, &thread.Title, &group.ID, &isPinned, &isPublic, &isTemporary, []uuid.UUID{})
		require.NoError(t, err, "Failed to update thread group")
		threads[i] = thread
		time.Sleep(10 * time.Millisecond)
	}

	// List threads with pagination
	pageRequest := &api.PageAPIRequest{
		PageSize:   3,
		PageNumber: 0,
		Order:      "-created_at",
	}

	// Get first page
	listedThreads, err := ListChatThreads(ctx, group.ID, pageRequest)
	require.NoError(t, err, "ListChatThreads should not return an error")
	assert.Len(t, listedThreads, 3, "Should return 3 threads for first page")

	// Get second page
	pageRequest.PageNumber = 1
	pageRequest.PageSize = 2
	listedThreads, err = ListChatThreads(ctx, group.ID, pageRequest)
	require.NoError(t, err, "ListChatThreads should not return an error")
	assert.Len(t, listedThreads, 2, "Should return 2 threads for second page")

	// Test with different sort order
	pageRequest.PageNumber = 0
	pageRequest.PageSize = 10
	pageRequest.Order = "created_at"
	listedThreadsAsc, err := ListChatThreads(ctx, group.ID, pageRequest)
	require.NoError(t, err, "ListChatThreads should not return an error")
	assert.Len(t, listedThreadsAsc, 5, "Should return 5 threads")
	assert.NotEqual(t, listedThreads[0].ID, listedThreadsAsc[0].ID, "Different sort order should return different first thread")
}

func TestGetChatThread2(t *testing.T) {
	setupTestDB(t)
	ctx := context.Background()

	// We'll use the default tenant and user IDs for verification
	expectedUserID := auth.GetUserID()
	expectedTenantID := auth.GetTenantID()

	// Create a thread
	title := "Test Thread for Get"
	thread, err := CreateChatThread(ctx, uuid.Nil, title, nil, nil, nil, []uuid.UUID{})
	require.NoError(t, err, "Failed to create test thread")

	// Get the thread
	retrievedThread, err := GetChatThread(ctx, thread.ID)
	require.NoError(t, err, "GetChatThread should not return an error")
	assert.Equal(t, thread.ID, retrievedThread.ID, "Retrieved thread ID should match")
	assert.Equal(t, title, retrievedThread.Title, "Retrieved thread title should match")
	assert.Equal(t, expectedUserID, retrievedThread.UserID, "User ID should match")
	assert.Equal(t, expectedTenantID, retrievedThread.TenantID, "Tenant ID should match")

	// Test with non-existent ID
	nonExistentID := uuid.New()
	_, err = GetChatThread(ctx, nonExistentID)
	assert.Error(t, err, "Should return error for non-existent thread")
}

func TestCreateChatThread(t *testing.T) {
	setupTestDB(t)
	ctx := context.Background()

	// Create a group
	group := createTestGroup(t)

	// Create a thread
	title := "Test Thread Creation"
	thread, err := CreateChatThread(ctx, group.ID, title, nil, nil, nil, []uuid.UUID{})
	require.NoError(t, err, "CreateChatThread should not return an error")

	// Verify expected values
	assert.Equal(t, title, thread.Title, "Title should match")
	assert.Equal(t, group.ID, thread.GroupID, "GroupID should match")
	assert.Equal(t, auth.GetTenantID(), thread.TenantID, "TenantID should match")
	assert.Equal(t, auth.GetUserID(), thread.UserID, "UserID should match")
	assert.True(t, thread.CreatedAtMs > 0, "CreatedAt should be set")
	assert.True(t, thread.UpdatedAtMs > 0, "UpdatedAt should be set")
	assert.True(t, thread.LastMsgAtMs > 0, "LastMsgAt should be set")
	assert.True(t, thread.IsActive, "New thread should be active")
	assert.False(t, thread.IsDeleted, "New thread should not be deleted")
	assert.False(t, thread.IsPinned, "New thread should not be pinned by default")
	assert.False(t, thread.IsPublic, "New thread should not be public by default")
	assert.False(t, thread.IsTemporary, "New thread should not be temporary by default")

	// Verify it was saved to the database
	retrievedThread, err := GetChatThread(ctx, thread.ID)
	require.NoError(t, err, "Failed to get created thread")
	assert.Equal(t, thread.ID, retrievedThread.ID, "Thread ID should match")
}

func TestUpdateChatThread(t *testing.T) {
	setupTestDB(t)
	ctx := context.Background()

	// Create a thread
	thread, err := CreateChatThread(ctx, uuid.Nil, "Original Title", nil, nil, nil, []uuid.UUID{})
	require.NoError(t, err, "Failed to create test thread")

	// Create a new group
	group := createTestGroup(t)

	// Update the thread
	newTitle := "Updated Thread Title"
	isPinned := true
	isPublic := true
	isTemporary := true
	updatedThread, err := UpdateChatThread(ctx, thread.ID, &newTitle, &group.ID, &isPinned, &isPublic, &isTemporary, []uuid.UUID{})
	require.NoError(t, err, "UpdateChatThread should not return an error")
	assert.Equal(t, newTitle, updatedThread.Title, "Title should be updated")
	assert.Equal(t, group.ID, updatedThread.GroupID, "Group ID should be updated")
	assert.True(t, updatedThread.IsPinned, "IsPinned should be updated")
	assert.True(t, updatedThread.IsPublic, "IsPublic should be updated")
	assert.True(t, updatedThread.IsTemporary, "IsTemporary should be updated")

	// Verify the changes were saved to the database
	retrievedThread, err := GetChatThread(ctx, thread.ID)
	require.NoError(t, err, "Failed to get updated thread")
	assert.Equal(t, newTitle, retrievedThread.Title, "Title should be updated in DB")
	assert.Equal(t, group.ID, retrievedThread.GroupID, "Group ID should be updated in DB")
	assert.True(t, retrievedThread.IsPinned, "IsPinned should be updated in DB")
	assert.True(t, retrievedThread.IsPublic, "IsPublic should be updated in DB")
	assert.True(t, retrievedThread.IsTemporary, "IsTemporary should be updated in DB")

	// Test updating back to false
	isPinned = false
	isPublic = false
	isTemporary = false
	updatedThread, err = UpdateChatThread(ctx, thread.ID, &newTitle, &group.ID, &isPinned, &isPublic, &isTemporary, []uuid.UUID{})
	require.NoError(t, err, "UpdateChatThread should not return an error")
	assert.False(t, updatedThread.IsPinned, "IsPinned should be updated to false")
	assert.False(t, updatedThread.IsPublic, "IsPublic should be updated to false")
	assert.False(t, updatedThread.IsTemporary, "IsTemporary should be updated to false")

	// Test with non-existent ID
	nonExistentID := uuid.New()
	_, err = UpdateChatThread(ctx, nonExistentID, &newTitle, &group.ID, nil, nil, nil, []uuid.UUID{})
	assert.Error(t, err, "Should return error for non-existent thread")
}

func TestDeleteChatThread(t *testing.T) {
	setupTestDB(t)
	ctx := context.Background()

	// Create a thread
	thread, err := CreateChatThread(ctx, uuid.Nil, "Thread to Delete", nil, nil, nil, []uuid.UUID{})
	require.NoError(t, err, "Failed to create test thread")

	// Delete the thread
	err = DeleteChatThread(ctx, thread.ID)
	require.NoError(t, err, "DeleteChatThread should not return an error")

	// Verify the thread is marked as deleted in the database
	retrievedThread, err := GetChatThread(ctx, thread.ID)
	require.NoError(t, err, "Failed to get deleted thread")
	assert.True(t, retrievedThread.IsDeleted, "Thread should be marked as deleted")

	// Test with non-existent ID
	nonExistentID := uuid.New()
	err = DeleteChatThread(ctx, nonExistentID)
	assert.Error(t, err, "Should return error for non-existent thread")
}

// Tests with mocked dependencies
func TestCreateChatThread_MockedDependencies(t *testing.T) {
	// Register mock service
	llm.RegisterMockServiceForUnitTests(context.Background())

	setupTestDB(t)
	ctx := context.Background()

	// Create a thread with specific boolean flags
	isPinned := true
	isPublic := true
	isTemporary := true
	thread, err := CreateChatThread(ctx, uuid.Nil, "Test Thread with Mocked Dependencies", &isPinned, &isPublic, &isTemporary, []uuid.UUID{})
	require.NoError(t, err, "CreateChatThread should not return an error")
	assert.NotNil(t, thread, "Thread should be created with mocked dependencies")
	assert.True(t, thread.IsPinned, "IsPinned should be set to true")
	assert.True(t, thread.IsPublic, "IsPublic should be set to true")
	assert.True(t, thread.IsTemporary, "IsTemporary should be set to true")
}

func TestListChatThreads_MockedDependencies(t *testing.T) {
	// Register mock service
	llm.RegisterMockServiceForUnitTests(context.Background())

	setupTestDB(t)
	ctx := context.Background()

	group := createTestGroup(t)

	// Create a thread
	_, err := CreateChatThread(ctx, uuid.Nil, "Test Thread", nil, nil, nil, []uuid.UUID{})
	require.NoError(t, err, "Failed to create test thread")

	// List threads
	pageRequest := &api.PageAPIRequest{
		PageSize:   10,
		PageNumber: 0,
	}
	threads, err := ListChatThreads(ctx, group.ID, pageRequest)
	require.NoError(t, err, "ListChatThreads should not return an error")
	assert.NotNil(t, threads, "Should return threads array (might be empty)")
}

func TestGetChatThread_MockedDependencies(t *testing.T) {
	// Register mock service
	llm.RegisterMockServiceForUnitTests(context.Background())

	setupTestDB(t)
	ctx := context.Background()

	// Create a thread
	thread, err := CreateChatThread(ctx, uuid.Nil, "Test Thread for Get", nil, nil, nil, []uuid.UUID{})
	require.NoError(t, err, "Failed to create test thread")

	// Get the thread
	retrievedThread, err := GetChatThread(ctx, thread.ID)
	require.NoError(t, err, "GetChatThread should not return an error")
	assert.Equal(t, thread.ID, retrievedThread.ID, "Retrieved thread ID should match")
}

func TestUpdateChatThread_MockedDependencies(t *testing.T) {
	// Register mock service
	llm.RegisterMockServiceForUnitTests(context.Background())

	setupTestDB(t)
	ctx := context.Background()

	// Create a thread
	thread, err := CreateChatThread(ctx, uuid.Nil, "Original Title", nil, nil, nil, []uuid.UUID{})
	require.NoError(t, err, "Failed to create test thread")

	// Create a new group
	group := createTestGroup(t)

	// Update the thread with boolean flags
	isPinned := true
	isPublic := true
	isTemporary := true
	title := "Updated Thread Title"
	updatedThread, err := UpdateChatThread(ctx, thread.ID, &title, &group.ID, &isPinned, &isPublic, &isTemporary, []uuid.UUID{})
	require.NoError(t, err, "UpdateChatThread should not return an error")
	assert.Equal(t, title, updatedThread.Title, "Title should be updated")
	assert.Equal(t, group.ID, updatedThread.GroupID, "Group ID should be updated")
	assert.True(t, updatedThread.IsPinned, "IsPinned should be updated")
	assert.True(t, updatedThread.IsPublic, "IsPublic should be updated")
	assert.True(t, updatedThread.IsTemporary, "IsTemporary should be updated")

	updatedThread, err = UpdateChatThread(ctx, thread.ID, nil, nil, &isPinned, &isPublic, &isTemporary, []uuid.UUID{})
	require.NoError(t, err, "UpdateChatThread should not return an error")
	assert.Equal(t, title, updatedThread.Title, "Title should be updated")
	assert.Equal(t, group.ID, updatedThread.GroupID, "Group ID should be updated")
}

func TestDeleteChatThread_MockedDependencies(t *testing.T) {
	// Register mock service
	llm.RegisterMockServiceForUnitTests(context.Background())

	setupTestDB(t)
	ctx := context.Background()

	// Create a thread
	thread, err := CreateChatThread(ctx, uuid.Nil, "Thread to Delete", nil, nil, nil, []uuid.UUID{})
	require.NoError(t, err, "Failed to create test thread")

	// Delete the thread
	err = DeleteChatThread(ctx, thread.ID)
	require.NoError(t, err, "DeleteChatThread should not return an error")
}
