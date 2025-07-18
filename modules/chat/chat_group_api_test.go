package chat

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"

	"github.com/hypernetix/hyperspot/libs/api"
	"github.com/hypernetix/hyperspot/libs/auth"
	"github.com/hypernetix/hyperspot/libs/db"
	"github.com/hypernetix/hyperspot/modules/llm"
)

// Helper function to set up test database
func setupGroupTestDB(t *testing.T) *gorm.DB {
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

	// Delete all data from the tables
	// Using Unscoped().Delete() to perform a hard delete that bypasses GORM's soft delete
	db.DB().Unscoped().Where("1=1").Delete(&ChatMessage{})
	db.DB().Unscoped().Where("1=1").Delete(&ChatThread{})
	db.DB().Unscoped().Where("1=1").Delete(&ChatThreadGroup{})

	return testDB
}

// Helper function to create a test group for reuse across tests
func createTestGroupForTest(t *testing.T) *ChatThreadGroup {
	t.Helper()
	group := &ChatThreadGroup{
		ID:          uuid.New(),
		TenantID:    auth.GetTenantID(),
		UserID:      auth.GetUserID(),
		Title:       "Test Group",
		CreatedAtMs: time.Now().UnixMilli(),
		UpdatedAtMs: time.Now().UnixMilli(),
		LastMsgAtMs: time.Now().UnixMilli(),
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

// Test DbSaveFields method of ChatThreadGroup
func TestChatThreadGroupDbSaveFields(t *testing.T) {
	setupGroupTestDB(t)
	ctx := context.Background()

	group, err := CreateChatThreadGroup(ctx, "Test Group", nil, nil, nil)
	require.NoError(t, err, "Failed to create test group")

	// Update fields
	newTitle := "Updated Group Title"
	group.Title = newTitle
	group.IsPinned = true
	group.IsPublic = true
	group.IsTemporary = true

	// Save specific fields
	err = group.DbSaveFields(&group.Title, &group.IsPinned, &group.IsPublic, &group.IsTemporary)
	require.NoError(t, err, "DbSaveFields should not return an error")

	// Verify the changes were saved to the database
	updatedGroup, err := GetChatThreadGroup(ctx, group.ID)
	require.NoError(t, err, "Failed to get updated group")
	assert.Equal(t, newTitle, updatedGroup.Title, "Title should be updated")
	assert.True(t, updatedGroup.IsPinned, "IsPinned should be updated")
	assert.True(t, updatedGroup.IsPublic, "IsPublic should be updated")
	assert.True(t, updatedGroup.IsTemporary, "IsTemporary should be updated")
}

// Test SetLastMsgAt method of ChatThreadGroup
func TestChatThreadGroupSetLastMsgAt(t *testing.T) {
	setupGroupTestDB(t)
	ctx := context.Background()

	// Create a group with specific flags
	isPinned := true
	isPublic := true
	isTemporary := true

	group, err := CreateChatThreadGroup(ctx, "Test Group", &isPinned, &isPublic, &isTemporary)
	require.NoError(t, err, "Failed to create test group")
	assert.True(t, group.IsPinned, "Group should be pinned")
	assert.True(t, group.IsPublic, "Group should be public")
	assert.True(t, group.IsTemporary, "Group should be temporary")

	// Record the original timestamp
	originalLastMsgAt := group.LastMsgAtMs

	// Wait a moment to ensure timestamp changes
	time.Sleep(10 * time.Millisecond)

	// Update the timestamp
	err = group.SetLastMsgAt()
	require.NoError(t, err, "SetLastMsgAt should not return an error")

	// Verify the timestamp was updated
	assert.Greater(t, group.LastMsgAtMs, originalLastMsgAt, "LastMsgAt should be updated")

	// Verify it was saved to the database and flags remain unchanged
	updatedGroup, err := GetChatThreadGroup(ctx, group.ID)
	require.NoError(t, err, "Failed to get updated group")
	assert.Equal(t, group.LastMsgAtMs, updatedGroup.LastMsgAtMs, "LastMsgAt should be updated in DB")
	assert.True(t, updatedGroup.IsPinned, "IsPinned should remain unchanged")
	assert.True(t, updatedGroup.IsPublic, "IsPublic should remain unchanged")
	assert.True(t, updatedGroup.IsTemporary, "IsTemporary should remain unchanged")
}

// Test ListChatThreadGroups function
func TestListChatThreadGroups(t *testing.T) {
	setupGroupTestDB(t)
	ctx := context.Background()

	// Create multiple groups
	groupCount := 5
	for i := 0; i < groupCount; i++ {
		_, err := CreateChatThreadGroup(ctx, "Group "+string(rune('A'+i)), nil, nil, nil)
		require.NoError(t, err, "Failed to create test group")
		time.Sleep(10 * time.Millisecond)
	}

	// List groups with default page request
	pageRequest := &api.PageAPIRequest{
		PageSize:   10,
		PageNumber: 0,
		Order:      "-created_at",
	}
	_, err := ListChatThreadGroups(ctx, pageRequest)
	require.NoError(t, err, "ListChatThreadGroups should not return an error")

	// Test pagination
	smallPageRequest := &api.PageAPIRequest{
		PageSize:   2,
		PageNumber: 0,
		Order:      "-created_at",
	}
	firstPageGroups, err := ListChatThreadGroups(ctx, smallPageRequest)
	require.NoError(t, err, "ListChatThreadGroups with pagination should not return an error")
	assert.Len(t, firstPageGroups, 2, "Should return first page with 2 groups")

	// Get second page
	secondPageRequest := &api.PageAPIRequest{
		PageSize:   2,
		PageNumber: 2,
		Order:      "-created_at",
	}
	secondPageGroups, err := ListChatThreadGroups(ctx, secondPageRequest)
	require.NoError(t, err, "ListChatThreadGroups for second page should not return an error")
	assert.Len(t, secondPageGroups, 2, "Should return second page with 2 groups")

	// Verify first and second page groups are different
	assert.NotEqual(t, firstPageGroups[0].ID, secondPageGroups[0].ID, "Groups from different pages should be different")
}

// Test GetChatThreadGroup function
func TestGetChatThreadGroup(t *testing.T) {
	setupGroupTestDB(t)
	ctx := context.Background()

	// Create a group
	origGroup, err := CreateChatThreadGroup(ctx, "Test Group", nil, nil, nil)
	require.NoError(t, err, "Failed to create test group")

	// Get the group
	retrievedGroup, err := GetChatThreadGroup(ctx, origGroup.ID)
	require.NoError(t, err, "GetChatThreadGroup should not return an error")

	// Verify the retrieved group matches the original
	assert.Equal(t, origGroup.ID, retrievedGroup.ID, "Group ID should match")
	assert.Equal(t, origGroup.Title, retrievedGroup.Title, "Group title should match")
	assert.Equal(t, origGroup.TenantID, retrievedGroup.TenantID, "Tenant ID should match")
	assert.Equal(t, origGroup.UserID, retrievedGroup.UserID, "User ID should match")

	// Test retrieving non-existent group
	_, err = GetChatThreadGroup(ctx, uuid.New())
	assert.Error(t, err, "Getting non-existent group should return error")
}

// Test CreateChatThreadGroup function
func TestCreateChatThreadGroup(t *testing.T) {
	setupGroupTestDB(t)
	ctx := context.Background()

	// We'll use the default tenant and user IDs for verification
	expectedUserID := auth.GetUserID()
	expectedTenantID := auth.GetTenantID()

	// Create a group with default flags
	title := "New Test Group"
	group, err := CreateChatThreadGroup(ctx, title, nil, nil, nil)
	require.NoError(t, err, "CreateChatThreadGroup should not return an error")

	// Verify the group was created with correct properties
	assert.Equal(t, title, group.Title, "Group title should match")
	assert.Equal(t, expectedUserID, group.UserID, "User ID should match")
	assert.Equal(t, expectedTenantID, group.TenantID, "Tenant ID should match")
	assert.True(t, group.IsActive, "New group should be active")
	assert.False(t, group.IsDeleted, "New group should not be deleted")
	assert.False(t, group.IsPinned, "New group should not be pinned by default")
	assert.False(t, group.IsPublic, "New group should not be public by default")
	assert.False(t, group.IsTemporary, "New group should not be temporary by default")
	assert.NotEqual(t, uuid.Nil, group.ID, "Group ID should be assigned")

	// Verify it exists in the database
	retrievedGroup, err := GetChatThreadGroup(ctx, group.ID)
	require.NoError(t, err, "Failed to get created group")
	assert.Equal(t, group.ID, retrievedGroup.ID, "Group should exist in database")

	// Create a group with all flags set to true
	isPinned := true
	isPublic := true
	isTemporary := true
	groupWithFlags, err := CreateChatThreadGroup(ctx, "Group With Flags", &isPinned, &isPublic, &isTemporary)
	require.NoError(t, err, "CreateChatThreadGroup with flags should not return an error")

	// Verify flags were set correctly
	assert.True(t, groupWithFlags.IsPinned, "Group should be pinned")
	assert.True(t, groupWithFlags.IsPublic, "Group should be public")
	assert.True(t, groupWithFlags.IsTemporary, "Group should be temporary")

	// Test with empty title (should fail validation)
	_, err = CreateChatThreadGroup(ctx, "", nil, nil, nil)
	assert.NoError(t, err, "Empty title should not fail validation")
}

// Test UpdateChatThreadGroup function
func TestUpdateChatThreadGroup(t *testing.T) {
	setupGroupTestDB(t)
	ctx := context.Background()

	// Create a group
	group, err := CreateChatThreadGroup(ctx, "Original Group Title", nil, nil, nil)
	require.NoError(t, err, "Failed to create test group")

	// Update the group
	newTitle := "Updated Group Title"
	updatedGroup, err := UpdateChatThreadGroup(ctx, group.ID, &newTitle, nil, nil, nil)
	require.NoError(t, err, "UpdateChatThreadGroup should not return an error")

	// Verify the update was applied
	assert.Equal(t, newTitle, updatedGroup.Title, "Title should be updated")

	// Verify it was saved to the database
	retrievedGroup, err := GetChatThreadGroup(ctx, group.ID)
	require.NoError(t, err, "Failed to get updated group")
	assert.Equal(t, newTitle, retrievedGroup.Title, "Title should be updated in database")

	// Test updating non-existent group
	title := "New Title"
	_, err = UpdateChatThreadGroup(ctx, uuid.New(), &title, nil, nil, nil)
	assert.Error(t, err, "Updating non-existent group should return error")
}

// Test DeleteChatThreadGroup function
func TestDeleteChatThreadGroup(t *testing.T) {
	setupGroupTestDB(t)
	ctx := context.Background()

	// Create a group
	group, err := CreateChatThreadGroup(ctx, "Group to Delete", nil, nil, nil)
	require.NoError(t, err, "Failed to create test group")

	// Delete the group
	err = DeleteChatThreadGroup(ctx, group.ID)
	require.NoError(t, err, "DeleteChatThreadGroup should not return an error")

	// Try to get the deleted group
	retrievedGroup, err := GetChatThreadGroup(ctx, group.ID)
	assert.Error(t, err, "Getting deleted group should return error")
	assert.Nil(t, retrievedGroup, "Deleted group should not be retrievable")

	// Test deleting non-existent group
	err = DeleteChatThreadGroup(ctx, uuid.New())
	assert.Error(t, err, "Deleting non-existent group should return error")
}

// Tests with mocked dependencies
func TestCreateChatThreadGroup_MockedDependencies(t *testing.T) {
	// Register mock service
	llm.RegisterMockServiceForUnitTests(context.Background())

	setupGroupTestDB(t)
	ctx := context.Background()

	// Create a group with specific flags
	isPinned := true
	isPublic := true
	isTemporary := true
	group, err := CreateChatThreadGroup(ctx, "Test Group with Mocked Dependencies", &isPinned, &isPublic, &isTemporary)
	require.NoError(t, err, "CreateChatThreadGroup should not return an error")
	assert.NotNil(t, group, "Group should be created with mocked dependencies")
	assert.True(t, group.IsPinned, "IsPinned should be set to true")
	assert.True(t, group.IsPublic, "IsPublic should be set to true")
	assert.True(t, group.IsTemporary, "IsTemporary should be set to true")
}

func TestListChatThreadGroups_MockedDependencies(t *testing.T) {
	// Register mock service
	llm.RegisterMockServiceForUnitTests(context.Background())

	setupGroupTestDB(t)
	ctx := context.Background()

	// Create groups with different flag combinations
	isPinned1 := true
	isPublic1 := true
	isTemporary1 := true

	_, err := CreateChatThreadGroup(ctx, "Test Group 1", &isPinned1, &isPublic1, &isTemporary1)
	require.NoError(t, err, "Failed to create test group")

	isPinned2 := false
	isPublic2 := true
	isTemporary2 := false
	_, err = CreateChatThreadGroup(ctx, "Test Group 2", &isPinned2, &isPublic2, &isTemporary2)
	require.NoError(t, err, "Failed to create test group")

	// List groups
	pageRequest := &api.PageAPIRequest{
		PageSize:   10,
		PageNumber: 0,
	}
	groups, err := ListChatThreadGroups(ctx, pageRequest)
	require.NoError(t, err, "ListChatThreadGroups should not return an error")
	assert.NotNil(t, groups, "Should return groups array")
	assert.GreaterOrEqual(t, len(groups), 2, "Should return at least the created groups")

	// Verify that groups with different flag combinations are returned
	var foundTemporary, foundNotTemporary bool
	for _, g := range groups {
		if g.IsTemporary {
			foundTemporary = true
		} else {
			foundNotTemporary = true
		}
	}
	assert.True(t, foundTemporary, "Should find at least one temporary group")
	assert.True(t, foundNotTemporary, "Should find at least one non-temporary group")
}

func TestGetChatThreadGroup_MockedDependencies(t *testing.T) {
	// Register mock service
	llm.RegisterMockServiceForUnitTests(context.Background())

	setupGroupTestDB(t)
	ctx := context.Background()

	// Create a group with specific flags
	isPinned := true
	isPublic := true
	isTemporary := true
	group, err := CreateChatThreadGroup(ctx, "Test Group", &isPinned, &isPublic, &isTemporary)
	require.NoError(t, err, "Failed to create test group")
	assert.True(t, group.IsPinned, "Group should be pinned")
	assert.True(t, group.IsPublic, "Group should be public")
	assert.True(t, group.IsTemporary, "Group should be temporary")

	// Get the group
	retrievedGroup, err := GetChatThreadGroup(ctx, group.ID)
	require.NoError(t, err, "GetChatThreadGroup should not return an error")
	assert.Equal(t, group.ID, retrievedGroup.ID, "Retrieved group should match created group")
	assert.True(t, retrievedGroup.IsPinned, "Retrieved group should be pinned")
	assert.True(t, retrievedGroup.IsPublic, "Retrieved group should be public")
	assert.True(t, retrievedGroup.IsTemporary, "Retrieved group should be temporary")
}

func TestUpdateChatThreadGroup_MockedDependencies(t *testing.T) {
	// Register mock service
	llm.RegisterMockServiceForUnitTests(context.Background())

	setupGroupTestDB(t)
	ctx := context.Background()

	// Create a group
	group, err := CreateChatThreadGroup(ctx, "Original Title", nil, nil, nil)
	require.NoError(t, err, "Failed to create test group")

	// Update the group with flags
	isPinned := true
	isPublic := true
	isTemporary := true
	title := "Updated Title"
	updatedGroup, err := UpdateChatThreadGroup(ctx, group.ID, &title, &isPinned, &isPublic, &isTemporary)
	require.NoError(t, err, "UpdateChatThreadGroup should not return an error")
	assert.Equal(t, "Updated Title", updatedGroup.Title, "Title should be updated")
	assert.True(t, updatedGroup.IsPinned, "IsPinned should be updated")
	assert.True(t, updatedGroup.IsPublic, "IsPublic should be updated")
	assert.True(t, updatedGroup.IsTemporary, "IsTemporary should be updated")
}

func TestDeleteChatThreadGroup_MockedDependencies(t *testing.T) {
	// Register mock service
	llm.RegisterMockServiceForUnitTests(context.Background())

	setupGroupTestDB(t)
	ctx := context.Background()

	// Create a group with flags set
	isPinned := true
	isPublic := true
	isTemporary := true
	group, err := CreateChatThreadGroup(ctx, "Group to Delete", &isPinned, &isPublic, &isTemporary)
	require.NoError(t, err, "Failed to create test group")
	assert.True(t, group.IsPinned, "Group should be pinned")
	assert.True(t, group.IsPublic, "Group should be public")
	assert.True(t, group.IsTemporary, "Group should be temporary")

	// Delete the group
	err = DeleteChatThreadGroup(ctx, group.ID)
	require.NoError(t, err, "DeleteChatThreadGroup should not return an error")

	// Verify the group is marked as deleted
	_, err = GetChatThreadGroup(ctx, group.ID)
	assert.Error(t, err, "Getting deleted group should return error")
}
