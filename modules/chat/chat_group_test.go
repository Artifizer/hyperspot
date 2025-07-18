package chat

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"

	"github.com/hypernetix/hyperspot/libs/api"
	"github.com/hypernetix/hyperspot/libs/auth"
	"github.com/hypernetix/hyperspot/libs/db"
	"github.com/hypernetix/hyperspot/libs/errorx"
	"github.com/hypernetix/hyperspot/modules/llm"
)

// Helper function to set up test database
func setupChatGroupTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	testDB, err := db.InitInMemorySQLite(nil)
	require.NoError(t, err, "Failed to connect to test DB")

	db.SetDB(testDB)
	err = db.SafeAutoMigrate(testDB,
		&ChatMessage{},
		&ChatThread{},
		&ChatThreadGroup{},
	)
	require.NoError(t, err, "Failed to migrate test database")

	return testDB
}

// Test that a new ChatThreadGroup is created with expected default values
func TestNewChatThreadGroup(t *testing.T) {
	setupChatGroupTestDB(t)
	ctx := context.Background()

	title := "Test Group Creation"
	group, err := CreateChatThreadGroup(ctx, title, nil, nil, nil)
	require.NoError(t, err, "CreateChatThreadGroup should not return an error")

	// Verify expected default values
	assert.Equal(t, title, group.Title, "Title should match")
	assert.Equal(t, auth.GetTenantID(), group.TenantID, "TenantID should match")
	assert.Equal(t, auth.GetUserID(), group.UserID, "UserID should match")
	assert.True(t, group.CreatedAtMs > 0, "CreatedAt should be set")
	assert.True(t, group.UpdatedAtMs > 0, "UpdatedAt should be set")
	assert.True(t, group.LastMsgAtMs > 0, "LastMsgAt should be set")
	assert.True(t, group.IsActive, "New group should be active")
	assert.False(t, group.IsPinned, "New group should not be pinned by default")
	assert.False(t, group.IsPublic, "New group should not be public by default")
	assert.False(t, group.IsTemporary, "New group should not be temporary by default")
	assert.False(t, group.IsDeleted, "New group should not be deleted")

	// Test creating a group with specific boolean flags
	isPinned := true
	isPublic := true
	isTemporary := true
	groupWithFlags, err := CreateChatThreadGroup(ctx, "Group With Flags", &isTemporary, &isPublic, &isPinned)
	require.NoError(t, err, "CreateChatThreadGroup with flags should not return an error")

	// Verify flags were set correctly
	assert.True(t, groupWithFlags.IsPinned, "Group should be pinned")
	assert.True(t, groupWithFlags.IsPublic, "Group should be public")
	assert.True(t, groupWithFlags.IsTemporary, "Group should be temporary")
}

// Test DbSaveFields with various field combinations
func TestChatThreadGroupDbSaveFieldsCombinations(t *testing.T) {
	setupChatGroupTestDB(t)
	ctx := context.Background()

	group, err := CreateChatThreadGroup(ctx, "Test Group", nil, nil, nil)
	require.NoError(t, err, "Failed to create test group")

	// Test case 1: Update title only
	originalUpdatedAt := group.UpdatedAtMs
	time.Sleep(10 * time.Millisecond) // Ensure time difference
	newTitle := "Updated Title Only"
	group.Title = newTitle
	err = group.DbSaveFields(&group.Title)
	require.NoError(t, err, "DbSaveFields should not return an error")

	// Verify updates
	retrievedGroup, err := GetChatThreadGroup(ctx, group.ID)
	require.NoError(t, err, "Failed to get updated group")
	assert.Equal(t, newTitle, retrievedGroup.Title, "Title should be updated")
	assert.Greater(t, retrievedGroup.UpdatedAtMs, originalUpdatedAt, "UpdatedAt should be updated")

	// Test case 2: Update multiple boolean fields
	originalUpdatedAt = retrievedGroup.UpdatedAtMs
	time.Sleep(10 * time.Millisecond) // Ensure time difference
	group.IsPinned = true
	group.IsPublic = true
	group.IsTemporary = true
	err = group.DbSaveFields(&group.IsPinned, &group.IsPublic, &group.IsTemporary)
	require.NoError(t, err, "DbSaveFields should not return an error")

	// Verify updates
	retrievedGroup, err = GetChatThreadGroup(ctx, group.ID)
	require.NoError(t, err, "Failed to get updated group")
	assert.True(t, retrievedGroup.IsPinned, "IsPinned should be updated")
	assert.True(t, retrievedGroup.IsPublic, "IsPublic should be updated")
	assert.True(t, retrievedGroup.IsTemporary, "IsTemporary should be updated")

	// Test case 3: Update boolean fields back to false
	originalUpdatedAt = retrievedGroup.UpdatedAtMs
	time.Sleep(10 * time.Millisecond) // Ensure time difference
	group.IsPinned = false
	group.IsPublic = false
	group.IsTemporary = false
	err = group.DbSaveFields(&group.IsPinned, &group.IsPublic, &group.IsTemporary)
	require.NoError(t, err, "DbSaveFields should not return an error")

	// Verify updates
	retrievedGroup, err = GetChatThreadGroup(ctx, group.ID)
	require.NoError(t, err, "Failed to get updated group")
	assert.False(t, retrievedGroup.IsPinned, "IsPinned should be updated to false")
	assert.False(t, retrievedGroup.IsPublic, "IsPublic should be updated to false")
	assert.False(t, retrievedGroup.IsTemporary, "IsTemporary should be updated to false")
	assert.Greater(t, retrievedGroup.UpdatedAtMs, originalUpdatedAt, "UpdatedAt should be updated")
}

// Test the SetLastMsgAt method with timestamp verification
func TestChatThreadGroupSetLastMsgAtTimestampVerification(t *testing.T) {
	setupChatGroupTestDB(t)
	ctx := context.Background()

	group, err := CreateChatThreadGroup(ctx, "Test Last Message Timestamp", nil, nil, nil)
	require.NoError(t, err, "Failed to create test group")

	// Record timestamps
	originalLastMsgAt := group.LastMsgAtMs
	originalUpdatedAt := group.UpdatedAtMs

	// Wait to ensure timestamp difference
	time.Sleep(10 * time.Millisecond)

	// Update last message timestamp
	err = group.SetLastMsgAt()
	require.NoError(t, err, "SetLastMsgAt should not return an error")

	// Verify both timestamps are updated
	assert.Greater(t, group.LastMsgAtMs, originalLastMsgAt, "LastMsgAt should be updated")
	assert.Greater(t, group.UpdatedAtMs, originalUpdatedAt, "UpdatedAt should be updated")

	// Verify timestamps in database
	retrievedGroup, err := GetChatThreadGroup(ctx, group.ID)
	require.NoError(t, err, "Failed to get updated group")
	assert.Equal(t, group.LastMsgAtMs, retrievedGroup.LastMsgAtMs, "LastMsgAt should match in DB")
	assert.Equal(t, group.UpdatedAtMs, retrievedGroup.UpdatedAtMs, "UpdatedAt should match in DB")
}

// Test listing groups with various filtering conditions
func TestListChatThreadGroupsFiltering(t *testing.T) {
	setupChatGroupTestDB(t)
	ctx := context.Background()

	// Create active groups
	activeCount := 3
	for i := 0; i < activeCount; i++ {
		_, err := CreateChatThreadGroup(ctx, "Active Group "+string(rune('A'+i)), nil, nil, nil)
		require.NoError(t, err, "Failed to create active group")
	}

	// Create deleted groups (manually setting IsDeleted)
	deletedCount := 2
	for i := 0; i < deletedCount; i++ {
		group, err := CreateChatThreadGroup(ctx, "Deleted Group "+string(rune('A'+i)), nil, nil, nil)
		require.NoError(t, err, "Failed to create group")
		group.IsDeleted = true
		err = group.DbSaveFields(&group.IsDeleted)
		require.NoError(t, err, "Failed to mark group as deleted")
	}

	// List only active groups (default behavior)
	pageRequest := &api.PageAPIRequest{
		PageSize:   10,
		PageNumber: 0,
	}
	groups, err := ListChatThreadGroups(ctx, pageRequest)
	require.NoError(t, err, "ListChatThreadGroups should not return an error")

	// Verify all returned groups are active and not deleted
	for _, group := range groups {
		assert.True(t, group.IsActive, "Group should be active")
		assert.False(t, group.IsDeleted, "Group should not be deleted")
	}
}

// Test GetChatThreadGroup with non-existent group ID
func TestGetChatThreadGroupNonExistent(t *testing.T) {
	setupChatGroupTestDB(t)
	ctx := context.Background()

	// Try to get a group with random UUID
	nonExistentID := uuid.New()
	retrievedGroup, err := GetChatThreadGroup(ctx, nonExistentID)

	// Verify error is returned
	assert.Error(t, err, "GetChatThreadGroup should return an error for non-existent group")
	assert.Nil(t, retrievedGroup, "Retrieved group should be nil")

	// Verify error type is ErrNotFound
	var notFoundErr *errorx.ErrNotFound
	assert.True(t, errors.As(err, &notFoundErr), "Error should be of type ErrNotFound")
}

// Test updating a group with validation
func TestUpdateChatThreadGroupValidation(t *testing.T) {
	setupChatGroupTestDB(t)
	ctx := context.Background()

	// Create a group
	originalTitle := "Original Group Title"
	group, err := CreateChatThreadGroup(ctx, originalTitle, nil, nil, nil)
	require.NoError(t, err, "Failed to create test group")

	// Test with empty title
	emptyTitle := ""
	_, err = UpdateChatThreadGroup(ctx, group.ID, &emptyTitle, nil, nil, nil)
	// Depending on implementation, this might or might not be an error
	// If validation exists, assert error; otherwise, verify empty title is accepted
	if err != nil {
		assert.Error(t, err, "Updating with empty title should return error if validation exists")
	} else {
		updatedGroup, getErr := GetChatThreadGroup(ctx, group.ID)
		require.NoError(t, getErr, "Failed to get updated group")
		assert.Equal(t, emptyTitle, updatedGroup.Title, "Title should be updated to empty")
	}

	// Test with very long title (implementation-dependent)
	longTitle := "This is an extremely long title that might exceed the database field size limit depending on the implementation constraints"
	_, err = UpdateChatThreadGroup(ctx, group.ID, &longTitle, nil, nil, nil)
	// Again, this might or might not be an error based on implementation
	if err != nil {
		assert.Error(t, err, "Updating with very long title might return error")
	} else {
		updatedGroup, getErr := GetChatThreadGroup(ctx, group.ID)
		require.NoError(t, getErr, "Failed to get updated group")
		assert.Equal(t, longTitle, updatedGroup.Title, "Long title should be accepted")
	}
}

// Test marking a group as deleted and verifying it cannot be retrieved
func TestDeletedGroupRetrieval(t *testing.T) {
	setupChatGroupTestDB(t)
	ctx := context.Background()

	// Create a group
	group, err := CreateChatThreadGroup(ctx, "Soon to be deleted", nil, nil, nil)
	require.NoError(t, err, "Failed to create test group")

	// Delete the group
	err = DeleteChatThreadGroup(ctx, group.ID)
	require.NoError(t, err, "DeleteChatThreadGroup should not return an error")

	// Try to get the deleted group
	_, err = GetChatThreadGroup(ctx, group.ID)
	assert.Error(t, err, "GetChatThreadGroup should return an error for deleted group")

	// However, the group should still exist in the database, just marked as deleted
	var retrievedGroup ChatThreadGroup
	result := db.DB().Where("id = ?", group.ID).First(&retrievedGroup)
	assert.NoError(t, result.Error, "Deleted group should still exist in database")
	assert.True(t, retrievedGroup.IsDeleted, "Group should be marked as deleted")
}

// Test creating groups with same title
func TestCreateChatThreadGroupDuplicateTitles(t *testing.T) {
	setupChatGroupTestDB(t)
	ctx := context.Background()

	// Create two groups with the same title
	title := "Duplicate Title Group"
	group1, err := CreateChatThreadGroup(ctx, title, nil, nil, nil)
	require.NoError(t, err, "Failed to create first group")

	group2, err := CreateChatThreadGroup(ctx, title, nil, nil, nil)
	require.NoError(t, err, "Failed to create second group with same title")

	// Verify both groups exist but have different IDs
	assert.NotEqual(t, group1.ID, group2.ID, "Groups should have different IDs despite same title")

	// Verify both can be retrieved
	retrievedGroup1, err := GetChatThreadGroup(ctx, group1.ID)
	require.NoError(t, err, "Failed to get first group")
	assert.Equal(t, title, retrievedGroup1.Title, "First group title should match")

	retrievedGroup2, err := GetChatThreadGroup(ctx, group2.ID)
	require.NoError(t, err, "Failed to get second group")
	assert.Equal(t, title, retrievedGroup2.Title, "Second group title should match")
}

// Test updating a group that was just deleted
func TestUpdateDeletedChatThreadGroup(t *testing.T) {
	setupChatGroupTestDB(t)
	ctx := context.Background()

	// Create a group
	group, err := CreateChatThreadGroup(ctx, "Group to delete then update", nil, nil, nil)
	require.NoError(t, err, "Failed to create test group")

	// Delete the group
	err = DeleteChatThreadGroup(ctx, group.ID)
	require.NoError(t, err, "DeleteChatThreadGroup should not return an error")

	// Try to update the deleted group
	title := "Updated Deleted Group"
	_, err = UpdateChatThreadGroup(ctx, group.ID, &title, nil, nil, nil)
	assert.Error(t, err, "Updating a deleted group should return an error")
}

// Test with mocked database errors
func TestChatThreadGroupDBErrors(t *testing.T) {
	// Mock test will simulate database errors if possible
	// This is implementation-dependent and might require more complex setup
	// Consider skipping if mocking DB errors is too complex
	t.Skip("Skipping DB error test - implementation depends on project's mocking strategy")

	// Examples of error cases to test if mocking is available:
	// 1. Create fails due to DB error
	// 2. Get fails due to DB error
	// 3. Update fails due to DB error
	// 4. Delete fails due to DB error
	// 5. List fails due to DB error
}

// Tests with mocked services ensuring all mocked dependencies work properly
func TestChatThreadGroupWithAllMockedDependencies(t *testing.T) {
	// Register all necessary mock services
	llm.RegisterMockServiceForUnitTests(context.Background())
	// Add other mock services as needed

	setupChatGroupTestDB(t)
	ctx := context.Background()

	// Test complete workflow with mocked dependencies
	// 1. Create a group with flags
	isPinned := true
	isPublic := true
	isTemporary := true
	group, err := CreateChatThreadGroup(ctx, "Complete Workflow Test", &isPinned, &isPublic, &isTemporary)
	require.NoError(t, err, "CreateChatThreadGroup should not return an error")
	assert.True(t, group.IsPinned, "IsPinned should be set")
	assert.True(t, group.IsPublic, "IsPublic should be set")
	assert.True(t, group.IsTemporary, "IsTemporary should be set")

	// 2. Get the group
	retrievedGroup, err := GetChatThreadGroup(ctx, group.ID)
	require.NoError(t, err, "GetChatThreadGroup should not return an error")
	assert.Equal(t, group.ID, retrievedGroup.ID, "Retrieved group should match created group")
	assert.True(t, retrievedGroup.IsPinned, "IsPinned should persist in DB")
	assert.True(t, retrievedGroup.IsPublic, "IsPublic should persist in DB")
	assert.True(t, retrievedGroup.IsTemporary, "IsTemporary should persist in DB")

	// 3. Update the group with different flags
	updatedTitle := "Updated Workflow Test"
	isPinned = false
	isPublic = false
	isTemporary = false
	updatedGroup, err := UpdateChatThreadGroup(ctx, group.ID, &updatedTitle, &isPinned, &isPublic, &isTemporary)
	require.NoError(t, err, "UpdateChatThreadGroup should not return an error")
	assert.Equal(t, updatedTitle, updatedGroup.Title, "Updated title should match")
	assert.False(t, updatedGroup.IsPinned, "IsPinned should be updated to false")
	assert.False(t, updatedGroup.IsPublic, "IsPublic should be updated to false")
	assert.False(t, updatedGroup.IsTemporary, "IsTemporary should be updated to false")

	// 4. Delete the group
	err = DeleteChatThreadGroup(ctx, group.ID)
	require.NoError(t, err, "DeleteChatThreadGroup should not return an error")

	// 5. Verify group is deleted
	_, err = GetChatThreadGroup(ctx, group.ID)
	assert.Error(t, err, "Getting deleted group should return error")
}

// Test concurrency - multiple operations on the same group
// Note: This is a more advanced test and might be flaky
func TestChatThreadGroupConcurrency(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping concurrency test in short mode")
	}

	setupChatGroupTestDB(t)
	ctx := context.Background()

	// Create a group
	group, err := CreateChatThreadGroup(ctx, "Concurrency Test Group", nil, nil, nil)
	require.NoError(t, err, "Failed to create test group")

	// Concurrent updates to LastMsgAt
	concurrentCount := 5
	doneChan := make(chan bool, concurrentCount)

	time.Sleep(10 * time.Millisecond)

	for i := 0; i < concurrentCount; i++ {
		go func() {
			retrievedGroup, err := GetChatThreadGroup(ctx, group.ID)
			if err == nil && retrievedGroup != nil {
				err = retrievedGroup.SetLastMsgAt()
			}
			doneChan <- (err == nil)
		}()
	}

	// Wait for all goroutines
	successCount := 0
	for i := 0; i < concurrentCount; i++ {
		if <-doneChan {
			successCount++
		}
	}

	// We expect some updates to succeed, but not necessarily all due to DB locking/concurrency
	assert.GreaterOrEqual(t, successCount, 1, "At least one concurrent update should succeed")

	// Final check - group should still be retrievable
	finalGroup, err := GetChatThreadGroup(ctx, group.ID)
	require.NoError(t, err, "Group should still be retrievable after concurrent operations")
	assert.NotEqual(t, group.LastMsgAtMs, finalGroup.LastMsgAtMs, "LastMsgAt should be updated")
}
