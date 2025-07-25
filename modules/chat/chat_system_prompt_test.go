package chat

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/hypernetix/hyperspot/libs/api"
	"github.com/hypernetix/hyperspot/libs/auth"
	"github.com/hypernetix/hyperspot/libs/db"
	"github.com/hypernetix/hyperspot/libs/errorx"
	"github.com/hypernetix/hyperspot/modules/llm"
)

// createTestThreadWithPrompt creates a test chat thread with the given system prompt IDs
func createTestThreadWithPrompt(t *testing.T, systemPromptIDs []uuid.UUID) (*ChatThread, context.Context, errorx.Error) {
	ctx := context.Background()
	thread, errx := CreateChatThread(ctx, uuid.Nil, "Test Thread", nil, nil, nil, systemPromptIDs)
	if errx != nil {
		return nil, ctx, errx
	}
	return thread, ctx, nil
}

// createDefaultSystemPrompt creates a default system prompt for testing
func createDefaultSystemPrompt(t *testing.T, name, content string) (*SystemPrompt, errorx.Error) {
	prompt, errx := createTestSystemPrompt(t, name, content, true)
	return prompt, errx
}

// createTestSystemPrompt creates a test system prompt with the given parameters
func createTestSystemPrompt(t *testing.T, name, content string, isDefault bool, uiMeta ...string) (*SystemPrompt, errorx.Error) {
	ctx := context.Background()
	uiMetaValue := ""
	if len(uiMeta) > 0 {
		uiMetaValue = uiMeta[0]
	}
	prompt, errx := CreateSystemPrompt(ctx, name, content, uiMetaValue, isDefault, "", false)
	require.Nil(t, errx, "Failed to create test system prompt")
	return prompt, errx
}

// TestSystemPromptDbSaveFields verifies the DbSaveFields method
func TestSystemPromptDbSaveFields(t *testing.T) {
	setupTestDB(t)
	ctx := context.Background()

	prompt, _ := createTestSystemPrompt(t, "Test Prompt", "Test content", false)

	// Update fields
	newName := "Updated Prompt"
	newContent := "Updated content"
	prompt.Name = newName
	prompt.Content = newContent

	// Save specific fields
	errx := prompt.DbSaveFields(&prompt.Name, &prompt.Content)
	require.Nil(t, errx, "DbSaveFields should not return an error")

	// Verify the changes were saved to the database
	updatedPrompt, errx := GetSystemPrompt(ctx, prompt.ID)
	require.Nil(t, errx, "Failed to get updated prompt")
	assert.Equal(t, newName, updatedPrompt.Name, "Name should be updated")
	assert.Equal(t, newContent, updatedPrompt.Content, "Content should be updated")
	assert.GreaterOrEqual(t, updatedPrompt.UpdatedAtMs, prompt.UpdatedAtMs, "UpdatedAtMs should be updated")
}

// TestCreateSystemPrompt verifies system prompt creation
func TestCreateSystemPrompt(t *testing.T) {
	setupTestDB(t)
	ctx := context.Background()

	name := "Test System Prompt"
	content := "You are a helpful assistant."

	prompt, errx := CreateSystemPrompt(ctx, name, content, "", false, "", false)
	require.Nil(t, errx, "CreateSystemPrompt should not return an error")
	assert.NotNil(t, prompt, "Prompt should be created")
	assert.NotEqual(t, uuid.Nil, prompt.ID, "ID should be set")
	assert.Equal(t, auth.GetTenantID(), prompt.TenantID, "TenantID should be set")
	assert.Equal(t, auth.GetUserID(), prompt.UserID, "UserID should be set")
	assert.Equal(t, name, prompt.Name, "Name should match")
	assert.Equal(t, content, prompt.Content, "Content should match")
	assert.False(t, prompt.IsDeleted, "IsDeleted should be false")
	assert.Greater(t, prompt.CreatedAtMs, int64(0), "CreatedAtMs should be set")
	assert.Greater(t, prompt.UpdatedAtMs, int64(0), "UpdatedAtMs should be set")
}

// TestGetSystemPrompt verifies system prompt retrieval
func TestGetSystemPrompt(t *testing.T) {
	setupTestDB(t)
	ctx := context.Background()

	// Create a system prompt
	originalPrompt, _ := createTestSystemPrompt(t, "Test Prompt", "Test content", false)

	// Test getting the prompt
	retrievedPrompt, errx := GetSystemPrompt(ctx, originalPrompt.ID)
	assert.Nil(t, errx, "GetSystemPrompt should not return an error")
	assert.NotNil(t, retrievedPrompt)
	assert.Equal(t, originalPrompt.ID, retrievedPrompt.ID)
	assert.Equal(t, originalPrompt.Name, retrievedPrompt.Name)
	assert.Equal(t, originalPrompt.Content, retrievedPrompt.Content)

	// Test with invalid prompt ID
	retrievedPrompt, errx = GetSystemPrompt(ctx, uuid.New())
	assert.NotNil(t, errx, "GetSystemPrompt should return an error for invalid prompt ID")
	assert.Nil(t, retrievedPrompt)
}

// TestListSystemPrompts verifies system prompt listing with pagination
func TestListSystemPrompts(t *testing.T) {
	setupTestDB(t)
	ctx := context.Background()

	// Create multiple system prompts
	prompts := make([]*SystemPrompt, 5)
	for i := 0; i < 5; i++ {
		name := fmt.Sprintf("Test Prompt %d", i+1)
		content := fmt.Sprintf("Content for prompt %d", i+1)
		prompts[i], _ = createTestSystemPrompt(t, name, content, false)
		// Add small delay to ensure different timestamps
		time.Sleep(1 * time.Millisecond)
	}

	// Test default listing (no pagination)
	listedPrompts, errx := ListSystemPrompts(ctx, nil, nil)
	assert.Nil(t, errx, "ListSystemPrompts should not return an error")
	assert.Equal(t, 5, len(listedPrompts), "Should return all 5 prompts")

	// Test with pagination
	pageRequest := &api.PageAPIRequest{PageSize: 2, PageNumber: 1}
	listedPrompts, errx = ListSystemPrompts(ctx, pageRequest, nil)
	assert.Nil(t, errx, "ListSystemPrompts should not return an error")
	assert.Equal(t, 2, len(listedPrompts), "Should return 2 prompts")

	// Test ordering (default ascending)
	pageRequest = &api.PageAPIRequest{Order: "updated_at"}
	listedPrompts, errx = ListSystemPrompts(ctx, pageRequest, nil)
	assert.Nil(t, errx, "ListSystemPrompts should not return an error")
	assert.Equal(t, 5, len(listedPrompts), "Should return all prompts")
	// First prompt should be the oldest
	assert.Equal(t, prompts[0].ID, listedPrompts[0].ID, "First prompt should be oldest")

	// Test descending order
	pageRequest = &api.PageAPIRequest{Order: "-updated_at"}
	listedPrompts, errx = ListSystemPrompts(ctx, pageRequest, nil)
	assert.Nil(t, errx, "ListSystemPrompts should not return an error")
	assert.Equal(t, 5, len(listedPrompts), "Should return all prompts")
	// First prompt should be the newest
	assert.Equal(t, prompts[4].ID, listedPrompts[0].ID, "First prompt should be newest")
}

// TestListSystemPromptsWithFilters verifies system prompt listing with various filters
func TestListSystemPromptsWithFilters(t *testing.T) {
	setupTestDB(t)
	ctx := context.Background()

	// Create test prompts with different characteristics
	defaultPrompt1, _ := createTestSystemPrompt(t, "Default Assistant", "Default content 1", true)
	_, _ = createTestSystemPrompt(t, "Default Helper", "Default content 2", true)
	_, _ = createTestSystemPrompt(t, "Regular Prompt", "Regular content", false)
	_, _ = createTestSystemPrompt(t, "Another Prompt", "Another content", false)

	// Create file-based prompts (simulate by setting FilePath)
	filePrompt1, _ := createTestSystemPrompt(t, "File Prompt 1", "File content 1", false)
	filePrompt1.FilePath = "/path/to/file1.txt"
	filePrompt1.AutoUpdate = true
	errx := filePrompt1.DbSaveFields(&filePrompt1.FilePath, &filePrompt1.AutoUpdate)
	require.Nil(t, errx, "Failed to update file prompt 1")

	filePrompt2, _ := createTestSystemPrompt(t, "File Prompt 2", "File content 2", false)
	filePrompt2.FilePath = "/path/to/file2.txt"
	filePrompt2.AutoUpdate = false
	errx = filePrompt2.DbSaveFields(&filePrompt2.FilePath, &filePrompt2.AutoUpdate)
	require.Nil(t, errx, "Failed to update file prompt 2")

	// Test filter by is_default = true
	filter := &systemPromptFilterRequest{
		IsDefault: &[]bool{true}[0],
	}
	prompts, errx := ListSystemPrompts(ctx, nil, filter)
	assert.Nil(t, errx, "ListSystemPrompts should not return an error")
	assert.Equal(t, 2, len(prompts), "Should return 2 default prompts")
	for _, prompt := range prompts {
		assert.True(t, prompt.IsDefault, "All returned prompts should be default")
	}

	// Test filter by is_default = false
	filter = &systemPromptFilterRequest{
		IsDefault: &[]bool{false}[0],
	}
	prompts, errx = ListSystemPrompts(ctx, nil, filter)
	assert.Nil(t, errx, "ListSystemPrompts should not return an error")
	assert.Equal(t, 4, len(prompts), "Should return 4 non-default prompts")
	for _, prompt := range prompts {
		assert.False(t, prompt.IsDefault, "All returned prompts should be non-default")
	}

	// Test filter by is_file = true
	filter = &systemPromptFilterRequest{
		IsFile: &[]bool{true}[0],
	}
	prompts, errx = ListSystemPrompts(ctx, nil, filter)
	assert.Nil(t, errx, "ListSystemPrompts should not return an error")
	assert.Equal(t, 2, len(prompts), "Should return 2 file-based prompts")
	for _, prompt := range prompts {
		assert.NotEmpty(t, prompt.FilePath, "All returned prompts should have file paths")
	}

	// Test filter by is_file = false
	filter = &systemPromptFilterRequest{
		IsFile: &[]bool{false}[0],
	}
	prompts, errx = ListSystemPrompts(ctx, nil, filter)
	assert.Nil(t, errx, "ListSystemPrompts should not return an error")
	assert.Equal(t, 4, len(prompts), "Should return 4 content-based prompts")
	for _, prompt := range prompts {
		assert.Empty(t, prompt.FilePath, "All returned prompts should not have file paths")
	}

	// Test filter by auto_update = true
	filter = &systemPromptFilterRequest{
		AutoUpdate: &[]bool{true}[0],
	}
	prompts, errx = ListSystemPrompts(ctx, nil, filter)
	assert.Nil(t, errx, "ListSystemPrompts should not return an error")
	assert.Equal(t, 1, len(prompts), "Should return 1 auto-update prompt")
	assert.True(t, prompts[0].AutoUpdate, "Returned prompt should have auto-update enabled")
	assert.Equal(t, filePrompt1.ID, prompts[0].ID, "Should return the correct auto-update prompt")

	// Test filter by auto_update = false
	filter = &systemPromptFilterRequest{
		AutoUpdate: &[]bool{false}[0],
	}
	prompts, errx = ListSystemPrompts(ctx, nil, filter)
	assert.Nil(t, errx, "ListSystemPrompts should not return an error")
	assert.Equal(t, 5, len(prompts), "Should return 5 non-auto-update prompts")
	for _, prompt := range prompts {
		assert.False(t, prompt.AutoUpdate, "All returned prompts should not have auto-update enabled")
	}

	// Test filter by name (partial match)
	nameFilter := "Default"
	filter = &systemPromptFilterRequest{
		Name: &nameFilter,
	}
	prompts, errx = ListSystemPrompts(ctx, nil, filter)
	assert.Nil(t, errx, "ListSystemPrompts should not return an error")
	assert.Equal(t, 2, len(prompts), "Should return 2 prompts with 'Default' in name")
	for _, prompt := range prompts {
		assert.Contains(t, strings.ToLower(prompt.Name), strings.ToLower(nameFilter), "All returned prompts should contain 'Default' in name")
	}

	// Test filter by name (case insensitive)
	nameFilter = "file"
	filter = &systemPromptFilterRequest{
		Name: &nameFilter,
	}
	prompts, errx = ListSystemPrompts(ctx, nil, filter)
	assert.Nil(t, errx, "ListSystemPrompts should not return an error")
	assert.Equal(t, 2, len(prompts), "Should return 2 prompts with 'file' in name (case insensitive)")
	for _, prompt := range prompts {
		assert.Contains(t, strings.ToLower(prompt.Name), strings.ToLower(nameFilter), "All returned prompts should contain 'file' in name")
	}

	// Test combined filters (default=true AND name contains "Default")
	nameFilter = "Assistant"
	filter = &systemPromptFilterRequest{
		IsDefault: &[]bool{true}[0],
		Name:      &nameFilter,
	}
	prompts, errx = ListSystemPrompts(ctx, nil, filter)
	assert.Nil(t, errx, "ListSystemPrompts should not return an error")
	assert.Equal(t, 1, len(prompts), "Should return 1 prompt matching both filters")
	assert.True(t, prompts[0].IsDefault, "Returned prompt should be default")
	assert.Contains(t, strings.ToLower(prompts[0].Name), strings.ToLower(nameFilter), "Returned prompt should contain 'Assistant' in name")
	assert.Equal(t, defaultPrompt1.ID, prompts[0].ID, "Should return the correct prompt")

	// Test combined filters (is_file=true AND auto_update=false)
	filter = &systemPromptFilterRequest{
		IsFile:     &[]bool{true}[0],
		AutoUpdate: &[]bool{false}[0],
	}
	prompts, errx = ListSystemPrompts(ctx, nil, filter)
	assert.Nil(t, errx, "ListSystemPrompts should not return an error")
	assert.Equal(t, 1, len(prompts), "Should return 1 file-based prompt without auto-update")
	assert.NotEmpty(t, prompts[0].FilePath, "Returned prompt should have file path")
	assert.False(t, prompts[0].AutoUpdate, "Returned prompt should not have auto-update")
	assert.Equal(t, filePrompt2.ID, prompts[0].ID, "Should return the correct prompt")

	// Test filters with no matches
	nameFilter = "NonExistentName"
	filter = &systemPromptFilterRequest{
		Name: &nameFilter,
	}
	prompts, errx = ListSystemPrompts(ctx, nil, filter)
	assert.Nil(t, errx, "ListSystemPrompts should not return an error")
	assert.Equal(t, 0, len(prompts), "Should return 0 prompts for non-existent name")
}

// TestListSystemPromptsWithPaginationAndFilters verifies system prompt listing with both pagination and filters
func TestListSystemPromptsWithPaginationAndFilters(t *testing.T) {
	setupTestDB(t)
	ctx := context.Background()

	// Create multiple prompts with same characteristics for pagination testing
	for i := 0; i < 5; i++ {
		name := fmt.Sprintf("Filter Test Prompt %d", i+1)
		content := fmt.Sprintf("Filter test content %d", i+1)
		_, _ = createTestSystemPrompt(t, name, content, false)
		time.Sleep(1 * time.Millisecond) // Ensure different timestamps
	}

	// Test pagination with name filter
	nameFilter := "Filter Test"
	filter := &systemPromptFilterRequest{
		Name: &nameFilter,
	}

	// Get first page
	pageRequest := &api.PageAPIRequest{
		PageSize:   2,
		PageNumber: 1,
		Order:      "name",
	}
	prompts, errx := ListSystemPrompts(ctx, pageRequest, filter)
	assert.Nil(t, errx, "ListSystemPrompts should not return an error")
	assert.Equal(t, 2, len(prompts), "Should return 2 prompts on first page")

	// Get second page
	pageRequest.PageNumber = 2
	prompts, errx = ListSystemPrompts(ctx, pageRequest, filter)
	assert.Nil(t, errx, "ListSystemPrompts should not return an error")
	assert.Equal(t, 2, len(prompts), "Should return 2 prompts on second page")

	// Get third page
	pageRequest.PageNumber = 3
	prompts, errx = ListSystemPrompts(ctx, pageRequest, filter)
	assert.Nil(t, errx, "ListSystemPrompts should not return an error")
	assert.Equal(t, 1, len(prompts), "Should return 1 prompt on third page")

	// Test ordering with filters
	pageRequest = &api.PageAPIRequest{
		Order: "name",
	}
	prompts, errx = ListSystemPrompts(ctx, pageRequest, filter)
	assert.Nil(t, errx, "ListSystemPrompts should not return an error")
	assert.Equal(t, 5, len(prompts), "Should return all 5 filtered prompts")

	// Verify ascending order by name
	for i := 1; i < len(prompts); i++ {
		assert.LessOrEqual(t, prompts[i-1].Name, prompts[i].Name, "Prompts should be in ascending order by name")
	}

	// Test descending order by name
	pageRequest.Order = "-name"
	prompts, errx = ListSystemPrompts(ctx, pageRequest, filter)
	assert.Nil(t, errx, "ListSystemPrompts should not return an error")
	assert.Equal(t, 5, len(prompts), "Should return all 5 filtered prompts")

	// Verify descending order by name
	for i := 1; i < len(prompts); i++ {
		assert.GreaterOrEqual(t, prompts[i-1].Name, prompts[i].Name, "Prompts should be in descending order by name")
	}
}

// TestListSystemPromptsOrderingOptions verifies all ordering options work correctly
func TestListSystemPromptsOrderingOptions(t *testing.T) {
	setupTestDB(t)
	ctx := context.Background()

	// Create prompts with specific names and timing
	prompt1, _ := createTestSystemPrompt(t, "Alpha Prompt", "Content 1", false)
	time.Sleep(10 * time.Millisecond)
	prompt2, _ := createTestSystemPrompt(t, "Beta Prompt", "Content 2", false)
	time.Sleep(10 * time.Millisecond)
	prompt3, _ := createTestSystemPrompt(t, "Gamma Prompt", "Content 3", false)

	// Test name ascending
	pageRequest := &api.PageAPIRequest{Order: "name"}
	prompts, errx := ListSystemPrompts(ctx, pageRequest, nil)
	assert.Nil(t, errx, "ListSystemPrompts should not return an error")
	assert.Equal(t, 3, len(prompts), "Should return all 3 prompts")
	assert.Equal(t, prompt1.ID, prompts[0].ID, "First prompt should be Alpha")
	assert.Equal(t, prompt2.ID, prompts[1].ID, "Second prompt should be Beta")
	assert.Equal(t, prompt3.ID, prompts[2].ID, "Third prompt should be Gamma")

	// Test name descending
	pageRequest = &api.PageAPIRequest{Order: "-name"}
	prompts, errx = ListSystemPrompts(ctx, pageRequest, nil)
	assert.Nil(t, errx, "ListSystemPrompts should not return an error")
	assert.Equal(t, 3, len(prompts), "Should return all 3 prompts")
	assert.Equal(t, prompt3.ID, prompts[0].ID, "First prompt should be Gamma")
	assert.Equal(t, prompt2.ID, prompts[1].ID, "Second prompt should be Beta")
	assert.Equal(t, prompt1.ID, prompts[2].ID, "Third prompt should be Alpha")

	// Test updated_at ascending
	pageRequest = &api.PageAPIRequest{Order: "updated_at"}
	prompts, errx = ListSystemPrompts(ctx, pageRequest, nil)
	assert.Nil(t, errx, "ListSystemPrompts should not return an error")
	assert.Equal(t, 3, len(prompts), "Should return all 3 prompts")
	assert.Equal(t, prompt1.ID, prompts[0].ID, "First prompt should be oldest")
	assert.Equal(t, prompt3.ID, prompts[2].ID, "Last prompt should be newest")

	// Test updated_at descending
	pageRequest = &api.PageAPIRequest{Order: "-updated_at"}
	prompts, errx = ListSystemPrompts(ctx, pageRequest, nil)
	assert.Nil(t, errx, "ListSystemPrompts should not return an error")
	assert.Equal(t, 3, len(prompts), "Should return all 3 prompts")
	assert.Equal(t, prompt3.ID, prompts[0].ID, "First prompt should be newest")
	assert.Equal(t, prompt1.ID, prompts[2].ID, "Last prompt should be oldest")

	// Test default ordering (should be name ascending)
	pageRequest = &api.PageAPIRequest{}
	prompts, errx = ListSystemPrompts(ctx, pageRequest, nil)
	assert.Nil(t, errx, "ListSystemPrompts should not return an error")
	assert.Equal(t, 3, len(prompts), "Should return all 3 prompts")
	assert.Equal(t, prompt1.ID, prompts[0].ID, "Default ordering should be name ascending")

	// Test invalid ordering (should default to name ascending)
	pageRequest = &api.PageAPIRequest{Order: "invalid_field"}
	prompts, errx = ListSystemPrompts(ctx, pageRequest, nil)
	assert.Error(t, errx, "ListSystemPrompts should return an error for invalid ordering")
}

// TestUpdateSystemPrompt verifies system prompt updates
func TestUpdateSystemPrompt(t *testing.T) {
	setupTestDB(t)
	ctx := context.Background()

	// Create a system prompt
	originalPrompt, _ := createTestSystemPrompt(t, "Original Name", "Original content", false)
	originalUpdatedAt := originalPrompt.UpdatedAtMs

	// Wait to ensure timestamp changes
	time.Sleep(10 * time.Millisecond)

	// Update the prompt
	newName := "Updated Name"
	newContent := "Updated content"
	updatedPrompt, errx := UpdateSystemPrompt(ctx, originalPrompt.ID, &newName, &newContent, nil, nil, nil, nil)
	require.Nil(t, errx, "UpdateSystemPrompt should not return an error")
	assert.NotNil(t, updatedPrompt)
	assert.Equal(t, originalPrompt.ID, updatedPrompt.ID)
	assert.Equal(t, newName, updatedPrompt.Name)
	assert.Equal(t, newContent, updatedPrompt.Content)
	assert.Greater(t, updatedPrompt.UpdatedAtMs, originalUpdatedAt, "UpdatedAtMs should be updated")

	// Verify the update persisted in the database
	retrievedPrompt, errx := GetSystemPrompt(ctx, originalPrompt.ID)
	require.Nil(t, errx, "Failed to get updated prompt")
	assert.Equal(t, newName, retrievedPrompt.Name)
	assert.Equal(t, newContent, retrievedPrompt.Content)

	// Test with invalid prompt ID
	invalidName := "New Name"
	invalidContent := "New content"
	updatedPrompt, errx = UpdateSystemPrompt(ctx, uuid.New(), &invalidName, &invalidContent, nil, nil, nil, nil)
	assert.NotNil(t, errx, "UpdateSystemPrompt should return an error for invalid prompt ID")
	assert.Nil(t, updatedPrompt)
}

// TestDeleteSystemPrompt verifies system prompt soft deletion
func TestDeleteSystemPrompt(t *testing.T) {
	setupTestDB(t)
	ctx := context.Background()

	// Create a system prompt
	prompt, _ := createTestSystemPrompt(t, "Test Prompt", "Test content", false)

	// Delete the prompt
	errx := DeleteSystemPrompt(ctx, prompt.ID)
	require.Nil(t, errx, "DeleteSystemPrompt should not return an error")

	// Verify the prompt is marked as deleted but still exists in DB
	var deletedPrompt SystemPrompt
	err := db.DB().Where("id = ?", prompt.ID).First(&deletedPrompt).Error
	require.NoError(t, err, "Prompt should still exist in database")
	assert.True(t, deletedPrompt.IsDeleted, "IsDeleted should be true")

	// Verify GetSystemPrompt no longer returns the deleted prompt
	retrievedPrompt, errx := GetSystemPrompt(ctx, prompt.ID)
	assert.NotNil(t, errx, "GetSystemPrompt should return an error for deleted prompt")
	assert.Nil(t, retrievedPrompt)

	// Test with invalid prompt ID
	errx = DeleteSystemPrompt(ctx, uuid.New())
	assert.NotNil(t, errx, "DeleteSystemPrompt should return an error for invalid prompt ID")
}

// TestDefaultSystemPrompts verifies default system prompts are automatically attached to threads
func TestDefaultSystemPrompts(t *testing.T) {
	setupTestDB(t)

	// Create default prompts
	defaultPrompt1, errx := createDefaultSystemPrompt(t, "Default 1", "Default content 1")
	require.Nil(t, errx, "Failed to create default prompt 1")

	defaultPrompt2, errx := createDefaultSystemPrompt(t, "Default 2", "Default content 2")
	require.Nil(t, errx, "Failed to create default prompt 2")

	// Create a non-default prompt that shouldn't be attached by default
	nonDefaultPrompt, _ := createTestSystemPrompt(t, "Non-default", "Non-default content", false)
	nonDefaultPrompt.IsDefault = false
	errx = nonDefaultPrompt.DbSaveFields(&nonDefaultPrompt.IsDefault)
	require.Nil(t, errx, "Failed to update prompt to non-default")

	// Create a thread with no prompts initially
	thread, threadCtx, errx := createTestThreadWithPrompt(t, nil)
	require.Nil(t, errx, "Failed to create thread")

	// Manually attach default prompts to simulate the default behavior
	errx = AttachSystemPromptsToThread(threadCtx, thread.ID, []uuid.UUID{defaultPrompt1.ID, defaultPrompt2.ID})
	require.Nil(t, errx, "Failed to attach default prompts to thread")

	// Create a thread that should get the default prompts
	thread1, threadCtx1, errx := createTestThreadWithPrompt(t, nil)
	require.Nil(t, errx, "Failed to create thread 1")

	// Manually attach the default prompts to simulate the default behavior
	errx = AttachSystemPromptsToThread(threadCtx1, thread1.ID, []uuid.UUID{defaultPrompt1.ID, defaultPrompt2.ID})
	require.Nil(t, errx, "Failed to attach default prompts to thread 1")

	// Verify default prompts are attached
	attachedPrompts, errx := GetSystemPromptsForThread(threadCtx1, thread1.ID)
	require.Nil(t, errx, "Failed to get attached prompts")
	require.Len(t, attachedPrompts, 2, "Should have 2 default prompts attached")

	// Verify the correct prompts are attached
	attachedIDs := make(map[uuid.UUID]bool)
	for _, p := range attachedPrompts {
		attachedIDs[p.ID] = true
	}
	assert.True(t, attachedIDs[defaultPrompt1.ID], "Default prompt 1 should be attached")
	assert.True(t, attachedIDs[defaultPrompt2.ID], "Default prompt 2 should be attached")
	assert.False(t, attachedIDs[nonDefaultPrompt.ID], "Non-default prompt should not be attached")

	// Create another thread that won't get default prompts
	thread2, threadCtx2, errx := createTestThreadWithPrompt(t, nil)
	require.Nil(t, errx, "Failed to create thread 2")

	// Verify no prompts are attached by default
	attachedPrompts, errx = GetSystemPromptsForThread(threadCtx2, thread2.ID)
	require.Nil(t, errx, "Failed to get attached prompts")
	assert.Len(t, attachedPrompts, 2, "Should have 2 default prompts attached")
}

// TestDefaultAndManualPrompts verifies that default and manually attached prompts work together
func TestDefaultAndManualPrompts(t *testing.T) {
	setupTestDB(t)

	// Create default prompts
	_, errx := createDefaultSystemPrompt(t, "Default 1", "Default content 1")
	require.Nil(t, errx, "Failed to create default prompt 1")

	defaultPrompt2, errx := createDefaultSystemPrompt(t, "Default 2", "Default content 2")
	require.Nil(t, errx, "Failed to create default prompt 2")

	// Create a non-default prompt
	nonDefaultPrompt, _ := createTestSystemPrompt(t, "Manual Prompt", "Manual content", false)

	// Create a thread
	thread, threadCtx, errx := createTestThreadWithPrompt(t, nil)
	require.Nil(t, errx, "Failed to create thread")

	// Verify default prompts are attached
	attachedPrompts, errx := GetSystemPromptsForThread(threadCtx, thread.ID)
	require.Nil(t, errx, "Failed to get attached prompts")
	require.Len(t, attachedPrompts, 2, "Should have 2 default prompts attached")

	// Manually attach another prompt
	errx = AttachSystemPromptToThread(threadCtx, thread.ID, nonDefaultPrompt.ID)
	require.Nil(t, errx, "Failed to attach manual prompt")

	// Verify all prompts are attached
	attachedPrompts, errx = GetSystemPromptsForThread(threadCtx, thread.ID)
	require.Nil(t, errx, "Failed to get attached prompts")
	require.Len(t, attachedPrompts, 3, "Should have 3 prompts attached (2 default + 1 manual)")

	// Detach a default prompt
	errx = DetachSystemPromptFromThread(threadCtx, thread.ID, nonDefaultPrompt.ID)
	require.Nil(t, errx, "Failed to detach manual prompt")

	// Verify only the remaining prompts are attached
	attachedPrompts, errx = GetSystemPromptsForThread(threadCtx, thread.ID)
	require.Nil(t, errx, "Failed to get attached prompts")
	require.Len(t, attachedPrompts, 2, "Should have 2 prompts attached after detach")

	// Create another thread - won't have any prompts by default
	thread2, threadCtx2, errx := createTestThreadWithPrompt(t, nil)
	require.Nil(t, errx, "Failed to create thread 2")

	attachedPrompts, errx = GetSystemPromptsForThread(threadCtx2, thread2.ID)
	require.Nil(t, errx, "Failed to get attached prompts")
	assert.Len(t, attachedPrompts, 2, "New thread should have 2 prompts by default")

	// Delete defaultPrompt2
	errx = DeleteSystemPrompt(threadCtx, defaultPrompt2.ID)
	require.Nil(t, errx, "Failed to delete default prompt 2")

	// Verify only the remaining prompts are attached
	attachedPrompts, errx = GetSystemPromptsForThread(threadCtx, thread.ID)
	require.Nil(t, errx, "Failed to get attached prompts after delete")
	require.Len(t, attachedPrompts, 1, "Should have 1 prompt attached after delete")
}

// TestUpdateDefaultPrompt verifies that updating a default prompt affects existing threads
func TestUpdateDefaultPrompt(t *testing.T) {
	setupTestDB(t)

	// Create a default prompt
	prompt, errx := createDefaultSystemPrompt(t, "Test Prompt", "Initial content")
	require.Nil(t, errx, "Failed to create default prompt")

	// Create a thread that will get the default prompt
	thread, threadCtx, errx := createTestThreadWithPrompt(t, nil)
	require.Nil(t, errx, "Failed to create thread")

	// Verify the prompt is attached
	attachedPrompts, errx := GetSystemPromptsForThread(threadCtx, thread.ID)
	require.Nil(t, errx, "Failed to get attached prompts")
	require.Len(t, attachedPrompts, 1, "Should have 1 default prompt attached")
	assert.Equal(t, "Initial content", attachedPrompts[0].Content, "Incorrect initial content")

	// Update the default prompt
	updatedContent := "Updated content"
	_, errx = UpdateSystemPrompt(threadCtx, prompt.ID, nil, &updatedContent, nil, nil, nil, nil)
	require.Nil(t, errx, "Failed to update default prompt")

	// Verify the thread's prompt was updated
	attachedPrompts, errx = GetSystemPromptsForThread(threadCtx, thread.ID)
	require.Nil(t, errx, "Failed to get attached prompts after update")
	require.Len(t, attachedPrompts, 1, "Should still have 1 prompt attached")
	assert.Equal(t, updatedContent, attachedPrompts[0].Content, "Prompt content should be updated")
}

// TestAttachSystemPromptsToThread verifies bulk attachment functionality
func TestAttachSystemPromptsToThread(t *testing.T) {
	setupTestDB(t)
	ctx := context.Background()

	// Create test prompts
	prompt1, _ := createTestSystemPrompt(t, "Prompt 1", "Content 1", false)
	prompt2, _ := createTestSystemPrompt(t, "Prompt 2", "Content 2", false)
	prompt3, _ := createTestSystemPrompt(t, "Prompt 3", "Content 3", false)

	// Create a test thread
	thread, threadCtx, errx := createTestThreadWithPrompt(t, nil)
	require.Nil(t, errx, "CreateThread should not return an error")

	// Attach multiple prompts
	promptIDs := []uuid.UUID{prompt1.ID, prompt2.ID, prompt3.ID}
	errx = AttachSystemPromptsToThread(threadCtx, thread.ID, promptIDs)
	require.Nil(t, errx, "AttachSystemPromptsToThread should not return an error")

	// Verify the attachments
	attachedPrompts, errx := GetSystemPromptsForThread(ctx, thread.ID)
	require.Nil(t, errx, "GetSystemPromptsForThread should not return an error")
	require.Len(t, attachedPrompts, 3, "Should have 3 prompts attached")

	// Verify the order matches the input order
	for i, prompt := range attachedPrompts {
		require.Equal(t, promptIDs[i], prompt.ID, "Prompt IDs should match in order")
	}

	// Test with empty prompt IDs

	// Test with empty prompt list
	errx = AttachSystemPromptsToThread(ctx, thread.ID, []uuid.UUID{})
	assert.Nil(t, errx, "AttachSystemPromptsToThread should handle empty list")

	// Test with invalid thread ID
	errx = AttachSystemPromptsToThread(ctx, uuid.New(), promptIDs)
	assert.NotNil(t, errx, "AttachSystemPromptsToThread should return error for invalid thread")

	// Test with invalid prompt ID
	invalidPromptIDs := []uuid.UUID{uuid.New()}
	errx = AttachSystemPromptsToThread(ctx, thread.ID, invalidPromptIDs)
	assert.NotNil(t, errx, "AttachSystemPromptsToThread should return error for invalid prompt")
}

// TestDetachSystemPromptsFromThread verifies bulk detachment functionality
func TestDetachSystemPromptsFromThread(t *testing.T) {
	setupTestDB(t)
	ctx := context.Background()

	// Create a thread and system prompts
	thread, _, errx := createTestThreadWithPrompt(t, nil)
	require.Nil(t, errx, "Failed to create thread")
	prompt1, _ := createTestSystemPrompt(t, "Prompt 1", "Content 1", false)
	prompt2, _ := createTestSystemPrompt(t, "Prompt 2", "Content 2", false)
	prompt3, _ := createTestSystemPrompt(t, "Prompt 3", "Content 3", false)

	// Attach all prompts first
	promptIDs := []uuid.UUID{prompt1.ID, prompt2.ID, prompt3.ID}
	errx = AttachSystemPromptsToThread(ctx, thread.ID, promptIDs)
	require.Nil(t, errx, "Failed to attach prompts")

	// Verify all prompts are attached
	attachedPrompts, errx := GetSystemPromptsForThread(ctx, thread.ID)
	require.Nil(t, errx, "Failed to get attached prompts")
	assert.Equal(t, 3, len(attachedPrompts), "Should have 3 attached prompts")

	// Test detaching some prompts
	detachIDs := []uuid.UUID{prompt1.ID, prompt2.ID}
	errx = DetachSystemPromptsFromThread(ctx, thread.ID, detachIDs)
	require.Nil(t, errx, "DetachSystemPromptsFromThread should not return an error")

	// Verify only prompt3 remains attached
	attachedPrompts, errx = GetSystemPromptsForThread(ctx, thread.ID)
	require.Nil(t, errx, "Failed to get attached prompts")
	assert.Equal(t, 1, len(attachedPrompts), "Should have 1 attached prompt")
	assert.Equal(t, prompt3.ID, attachedPrompts[0].ID, "Only prompt3 should remain")

	// Test detaching non-attached prompts (should not error)
	errx = DetachSystemPromptsFromThread(ctx, thread.ID, detachIDs)
	assert.Nil(t, errx, "DetachSystemPromptsFromThread should handle non-attached prompts gracefully")

	// Test with empty prompt list
	errx = DetachSystemPromptsFromThread(ctx, thread.ID, []uuid.UUID{})
	assert.Nil(t, errx, "DetachSystemPromptsFromThread should handle empty list")

	// Test with invalid thread ID
	errx = DetachSystemPromptsFromThread(ctx, uuid.New(), promptIDs)
	assert.NotNil(t, errx, "DetachSystemPromptsFromThread should return error for invalid thread")
}

// TestAttachDetachSystemPromptToThread verifies single prompt wrapper functions
func TestAttachDetachSystemPromptToThread(t *testing.T) {
	setupTestDB(t)
	ctx := context.Background()

	// Create a thread and system prompt
	thread, _, errx := createTestThreadWithPrompt(t, nil)
	require.Nil(t, errx, "Failed to create thread")
	prompt, _ := createTestSystemPrompt(t, "Test Prompt", "Test content", false)

	// Test single attach
	errx = AttachSystemPromptToThread(ctx, thread.ID, prompt.ID)
	require.Nil(t, errx, "AttachSystemPromptToThread should not return an error")

	// Verify prompt is attached
	attachedPrompts, errx := GetSystemPromptsForThread(ctx, thread.ID)
	require.Nil(t, errx, "Failed to get attached prompts")
	assert.Equal(t, 1, len(attachedPrompts), "Should have 1 attached prompt")
	assert.Equal(t, prompt.ID, attachedPrompts[0].ID, "Correct prompt should be attached")

	// Test single detach
	errx = DetachSystemPromptFromThread(ctx, thread.ID, prompt.ID)
	require.Nil(t, errx, "DetachSystemPromptFromThread should not return an error")

	// Verify prompt is detached
	attachedPrompts, errx = GetSystemPromptsForThread(ctx, thread.ID)
	require.Nil(t, errx, "Failed to get attached prompts")
	assert.Equal(t, 0, len(attachedPrompts), "Should have 0 attached prompts")
}

// TestDetachAllSystemPromptsFromThread verifies detaching all prompts
func TestDetachAllSystemPromptsFromThread(t *testing.T) {
	setupTestDB(t)
	ctx := context.Background()

	// Create a thread and system prompts
	thread, _, errx := createTestThreadWithPrompt(t, nil)
	require.Nil(t, errx, "Failed to create thread")
	prompt1, _ := createTestSystemPrompt(t, "Prompt 1", "Content 1", false)
	prompt2, _ := createTestSystemPrompt(t, "Prompt 2", "Content 2", false)
	prompt3, _ := createTestSystemPrompt(t, "Prompt 3", "Content 3", false)

	// Attach all prompts
	promptIDs := []uuid.UUID{prompt1.ID, prompt2.ID, prompt3.ID}
	errx = AttachSystemPromptsToThread(ctx, thread.ID, promptIDs)
	require.Nil(t, errx, "Failed to attach prompts")

	// Verify all prompts are attached
	attachedPrompts, errx := GetSystemPromptsForThread(ctx, thread.ID)
	require.Nil(t, errx, "Failed to get attached prompts")
	assert.Equal(t, 3, len(attachedPrompts), "Should have 3 attached prompts")

	// Detach all prompts
	errx = DetachAllSystemPromptsFromThread(ctx, thread.ID)
	require.Nil(t, errx, "DetachAllSystemPromptsFromThread should not return an error")

	// Verify no prompts are attached
	attachedPrompts, errx = GetSystemPromptsForThread(ctx, thread.ID)
	require.Nil(t, errx, "Failed to get attached prompts")
	assert.Equal(t, 0, len(attachedPrompts), "Should have 0 attached prompts")

	// Test with invalid thread ID
	errx = DetachAllSystemPromptsFromThread(ctx, uuid.New())
	assert.NotNil(t, errx, "DetachAllSystemPromptsFromThread should return error for invalid thread")
}

// TestGetSystemPromptsForThread verifies getting prompts for a thread
func TestGetSystemPromptsForThread(t *testing.T) {
	setupTestDB(t)
	ctx := context.Background()

	// Create a thread and system prompts
	thread := createTestThread(t)
	prompt1, _ := createTestSystemPrompt(t, "Prompt 1", "Content 1", false)
	prompt2, _ := createTestSystemPrompt(t, "Prompt 2", "Content 2", false)

	// Test with no attached prompts and no default prompts
	attachedPrompts, errx := GetSystemPromptsForThread(ctx, thread.ID)
	require.Nil(t, errx, "GetSystemPromptsForThread should not return an error")
	assert.Equal(t, 0, len(attachedPrompts), "Should have 0 prompts when no defaults exist")

	// Create a default system prompt
	defaultPrompt, _ := createTestSystemPrompt(t, "Default Prompt", "Default Content", true)

	// Test with no attached prompts but with default prompts
	promptsWithDefault, errx := GetSystemPromptsForThread(ctx, thread.ID)
	require.Nil(t, errx, "GetSystemPromptsForThread should not return an error")
	assert.Equal(t, 1, len(promptsWithDefault), "Should have 1 prompt (the default one)")
	assert.Equal(t, defaultPrompt.ID, promptsWithDefault[0].ID, "Default prompt should be returned")

	// Attach prompts
	promptIDs := []uuid.UUID{prompt1.ID, prompt2.ID}
	errx = AttachSystemPromptsToThread(ctx, thread.ID, promptIDs)
	require.Nil(t, errx, "Failed to attach prompts")

	// Test getting attached prompts and default prompts
	allPrompts, errx := GetSystemPromptsForThread(ctx, thread.ID)
	require.Nil(t, errx, "GetSystemPromptsForThread should not return an error")
	assert.Equal(t, 3, len(allPrompts), "Should have 3 prompts (2 attached + 1 default)")

	// Verify correct prompts are returned
	promptMap := make(map[uuid.UUID]bool)
	for _, prompt := range allPrompts {
		promptMap[prompt.ID] = true
	}
	assert.True(t, promptMap[prompt1.ID], "Prompt 1 should be included")
	assert.True(t, promptMap[prompt2.ID], "Prompt 2 should be included")
	assert.True(t, promptMap[defaultPrompt.ID], "Default prompt should be included")

	// Test with invalid thread ID
	invalidPrompts, errx := GetSystemPromptsForThread(ctx, uuid.New())
	assert.NotNil(t, errx, "GetSystemPromptsForThread should return error for invalid thread")
	assert.Nil(t, invalidPrompts)
}

// TestGetThreadsForSystemPrompt verifies getting threads for a prompt
func TestGetThreadsForSystemPrompt(t *testing.T) {
	setupTestDB(t)
	ctx := context.Background()

	// Create threads and a system prompt
	thread1 := createTestThread(t)
	thread2 := createTestThread(t)
	prompt, _ := createTestSystemPrompt(t, "Test Prompt", "Test content", false)

	// Test with no attached threads
	attachedThreads, errx := GetThreadsForSystemPrompt(ctx, prompt.ID)
	require.Nil(t, errx, "GetThreadsForSystemPrompt should not return an error")
	assert.Equal(t, 0, len(attachedThreads), "Should have 0 attached threads")

	// Attach prompt to threads
	errx = AttachSystemPromptToThread(ctx, thread1.ID, prompt.ID)
	require.Nil(t, errx, "Failed to attach prompt to thread1")
	errx = AttachSystemPromptToThread(ctx, thread2.ID, prompt.ID)
	require.Nil(t, errx, "Failed to attach prompt to thread2")

	// Test getting attached threads
	attachedThreads, errx = GetThreadsForSystemPrompt(ctx, prompt.ID)
	require.Nil(t, errx, "GetThreadsForSystemPrompt should not return an error")
	assert.Equal(t, 2, len(attachedThreads), "Should have 2 attached threads")

	// Verify correct threads are returned
	attachedIDs := make(map[uuid.UUID]bool)
	for _, thread := range attachedThreads {
		attachedIDs[thread.ID] = true
	}
	assert.True(t, attachedIDs[thread1.ID], "Thread 1 should be attached")
	assert.True(t, attachedIDs[thread2.ID], "Thread 2 should be attached")

	// Test with invalid prompt ID
	attachedThreads, errx = GetThreadsForSystemPrompt(ctx, uuid.New())
	assert.NotNil(t, errx, "GetThreadsForSystemPrompt should return error for invalid prompt")
	assert.Nil(t, attachedThreads)
}

// TestSystemPromptWithMockedDependencies verifies functionality with mocked LLM service
func TestSystemPromptWithMockedDependencies(t *testing.T) {
	// Register mock service
	llm.RegisterMockServiceForUnitTests(context.Background())

	setupTestDB(t)
	ctx := context.Background()

	// Test creating system prompt with mocked dependencies
	prompt, errx := CreateSystemPrompt(ctx, "Mocked Test Prompt", "You are a helpful assistant.", "", false, "", false)
	require.Nil(t, errx, "CreateSystemPrompt should work with mocked dependencies")
	assert.NotNil(t, prompt, "Prompt should be created with mocked dependencies")

	// Test creating thread with system prompt
	thread, errx := CreateChatThread(ctx, uuid.Nil, "Thread with System Prompt", nil, nil, nil, []uuid.UUID{prompt.ID})
	require.Nil(t, errx, "CreateChatThread should work with system prompts and mocked dependencies")
	assert.NotNil(t, thread, "Thread should be created")
	assert.Equal(t, 1, len(thread.SystemPrompts), "Thread should have 1 system prompt")
	assert.Equal(t, prompt.ID, thread.SystemPrompts[0].ID, "Correct system prompt should be attached")
}

// TestSystemPromptTransactionRollback verifies transaction rollback on errors
func TestSystemPromptTransactionRollback(t *testing.T) {
	setupTestDB(t)
	ctx := context.Background()

	// Create a thread
	thread := createTestThread(t)

	// Test with mix of valid and invalid prompt IDs
	validPrompt, _ := createTestSystemPrompt(t, "Valid Prompt", "Valid content", false)
	invalidPromptID := uuid.New() // This doesn't exist

	promptIDs := []uuid.UUID{validPrompt.ID, invalidPromptID}

	// This should fail and rollback
	errx := AttachSystemPromptsToThread(ctx, thread.ID, promptIDs)
	assert.NotNil(t, errx, "AttachSystemPromptsToThread should fail with invalid prompt ID")

	// Verify no prompts were attached (transaction rolled back)
	attachedPrompts, errx := GetSystemPromptsForThread(ctx, thread.ID)
	require.Nil(t, errx, "Failed to get attached prompts")
	assert.Equal(t, 0, len(attachedPrompts), "No prompts should be attached after rollback")
}

// TestPartialUpdateSystemPrompt verifies partial system prompt updates
func TestPartialUpdateSystemPrompt(t *testing.T) {
	setupTestDB(t)
	ctx := context.Background()

	// Create a system prompt to update
	originalPrompt, _ := createTestSystemPrompt(t, "Original Name", "Original content", false)
	originalUpdatedAt := originalPrompt.UpdatedAtMs

	// Test updating only the name
	time.Sleep(10 * time.Millisecond) // Ensure timestamp difference
	nameValue := "Updated Name Only"
	updatedPrompt, errx := UpdateSystemPrompt(ctx, originalPrompt.ID, &nameValue, nil, nil, nil, nil, nil)
	require.Nil(t, errx, "UpdateSystemPrompt should not return an error for name-only update")
	assert.NotNil(t, updatedPrompt)
	assert.Equal(t, originalPrompt.ID, updatedPrompt.ID)
	assert.Equal(t, "Updated Name Only", updatedPrompt.Name, "Name should be updated")
	assert.Equal(t, "Original content", updatedPrompt.Content, "Content should remain unchanged")
	assert.Greater(t, updatedPrompt.UpdatedAtMs, originalUpdatedAt, "UpdatedAtMs should be updated")
	assert.Equal(t, originalPrompt.CreatedAtMs, updatedPrompt.CreatedAtMs, "CreatedAtMs should remain unchanged")

	// Test updating only the content
	time.Sleep(10 * time.Millisecond) // Ensure timestamp difference
	previousUpdatedAt := updatedPrompt.UpdatedAtMs
	contentValue := "Updated content only"
	updatedPrompt, errx = UpdateSystemPrompt(ctx, originalPrompt.ID, nil, &contentValue, nil, nil, nil, nil)
	require.Nil(t, errx, "UpdateSystemPrompt should not return an error for content-only update")
	assert.NotNil(t, updatedPrompt)
	assert.Equal(t, originalPrompt.ID, updatedPrompt.ID)
	assert.Equal(t, "Updated Name Only", updatedPrompt.Name, "Name should remain from previous update")
	assert.Equal(t, "Updated content only", updatedPrompt.Content, "Content should be updated")
	assert.Greater(t, updatedPrompt.UpdatedAtMs, previousUpdatedAt, "UpdatedAtMs should be updated")
	assert.Equal(t, originalPrompt.CreatedAtMs, updatedPrompt.CreatedAtMs, "CreatedAtMs should remain unchanged")

	// Test updating both name and content
	time.Sleep(10 * time.Millisecond) // Ensure timestamp difference
	previousUpdatedAt = updatedPrompt.UpdatedAtMs
	finalName := "Final Name"
	finalContent := "Final content"
	updatedPrompt, errx = UpdateSystemPrompt(ctx, originalPrompt.ID, &finalName, &finalContent, nil, nil, nil, nil)
	require.Nil(t, errx, "UpdateSystemPrompt should not return an error for full update")
	assert.NotNil(t, updatedPrompt)
	assert.Equal(t, originalPrompt.ID, updatedPrompt.ID)
	assert.Equal(t, "Final Name", updatedPrompt.Name, "Name should be updated")
	assert.Equal(t, "Final content", updatedPrompt.Content, "Content should be updated")
	assert.Greater(t, updatedPrompt.UpdatedAtMs, previousUpdatedAt, "UpdatedAtMs should be updated")
	assert.Equal(t, originalPrompt.CreatedAtMs, updatedPrompt.CreatedAtMs, "CreatedAtMs should remain unchanged")
}

// TestPartialUpdateSystemPromptEmptyFields verifies behavior with empty fields
func TestPartialUpdateSystemPromptEmptyFields(t *testing.T) {
	setupTestDB(t)
	ctx := context.Background()

	// Create a system prompt to update
	originalPrompt, _ := createTestSystemPrompt(t, "Original Name", "Original content", false)

	// Test that nil fields don't change existing values
	beforeUpdate, errx := GetSystemPrompt(ctx, originalPrompt.ID)
	require.Nil(t, errx, "Should be able to get prompt before empty update")

	updatedPrompt, errx := UpdateSystemPrompt(ctx, originalPrompt.ID, nil, nil, nil, nil, nil, nil)
	require.Nil(t, errx, "UpdateSystemPrompt should not return an error for nil update")
	assert.NotNil(t, updatedPrompt)
	assert.Equal(t, beforeUpdate.Name, updatedPrompt.Name, "Name should remain unchanged with nil update")
	assert.Equal(t, beforeUpdate.Content, updatedPrompt.Content, "Content should remain unchanged with nil update")

	// Test that empty string pointers DO update the fields (this is the expected behavior)
	emptyName := ""
	emptyContent := ""
	updatedPrompt, errx = UpdateSystemPrompt(ctx, originalPrompt.ID, &emptyName, &emptyContent, nil, nil, nil, nil)
	require.Nil(t, errx, "UpdateSystemPrompt should not return an error for empty string update")
	assert.NotNil(t, updatedPrompt)
	assert.Equal(t, "", updatedPrompt.Name, "Name should be updated to empty string")
	assert.Equal(t, "", updatedPrompt.Content, "Content should be updated to empty string")
}

// TestPartialUpdateSystemPromptWithInvalidID verifies error handling for partial updates with invalid IDs
func TestPartialUpdateSystemPromptWithInvalidID(t *testing.T) {
	setupTestDB(t)
	ctx := context.Background()

	// Test partial update with non-existent ID
	nonExistentID := uuid.New()

	// Test name-only update with invalid ID
	nameValue := "Updated Name"
	updatedPrompt, errx := UpdateSystemPrompt(ctx, nonExistentID, &nameValue, nil, nil, nil, nil, nil)
	assert.NotNil(t, errx, "UpdateSystemPrompt should return error for non-existent ID")
	assert.Nil(t, updatedPrompt)

	// Test content-only update with invalid ID
	contentValue := "Updated Content"
	updatedPrompt, errx = UpdateSystemPrompt(ctx, nonExistentID, nil, &contentValue, nil, nil, nil, nil)
	assert.NotNil(t, errx, "UpdateSystemPrompt should return error for non-existent ID")
	assert.Nil(t, updatedPrompt)

	// Test both fields update with invalid ID
	bothName := "Updated Name"
	bothContent := "Updated Content"
	updatedPrompt, errx = UpdateSystemPrompt(ctx, nonExistentID, &bothName, &bothContent, nil, nil, nil, nil)
	assert.NotNil(t, errx, "UpdateSystemPrompt should return error for non-existent ID")
	assert.Nil(t, updatedPrompt)
}

// TestPartialUpdateSystemPromptPersistence verifies that partial updates are properly persisted
func TestPartialUpdateSystemPromptPersistence(t *testing.T) {
	setupTestDB(t)
	ctx := context.Background()

	// Create a system prompt
	originalPrompt, _ := createTestSystemPrompt(t, "Original Name", "Original content", false)

	// Update only the name
	nameValue := "Persisted Name"
	_, errx := UpdateSystemPrompt(ctx, originalPrompt.ID, &nameValue, nil, nil, nil, nil, nil)
	require.Nil(t, errx, "UpdateSystemPrompt should not return an error")

	// Retrieve the prompt from database to verify persistence
	retrievedPrompt, errx := GetSystemPrompt(ctx, originalPrompt.ID)
	require.Nil(t, errx, "Should be able to retrieve updated prompt")
	assert.Equal(t, "Persisted Name", retrievedPrompt.Name, "Name update should be persisted")
	assert.Equal(t, "Original content", retrievedPrompt.Content, "Content should remain unchanged")

	// Update only the content
	contentValue := "Persisted content"
	_, errx = UpdateSystemPrompt(ctx, originalPrompt.ID, nil, &contentValue, nil, nil, nil, nil)
	require.Nil(t, errx, "UpdateSystemPrompt should not return an error")

	// Retrieve again to verify content persistence
	retrievedPrompt, errx = GetSystemPrompt(ctx, originalPrompt.ID)
	require.Nil(t, errx, "Should be able to retrieve updated prompt")
	assert.Equal(t, "Persisted Name", retrievedPrompt.Name, "Name should remain from previous update")
	assert.Equal(t, "Persisted content", retrievedPrompt.Content, "Content update should be persisted")
}

// TestPartialUpdateSystemPromptWithSpecialCharacters verifies partial updates with special characters
func TestPartialUpdateSystemPromptWithSpecialCharacters(t *testing.T) {
	setupTestDB(t)
	ctx := context.Background()

	// Create a system prompt
	originalPrompt, _ := createTestSystemPrompt(t, "Original Name", "Original content", false)

	// Test name update with special characters
	specialName := "Special Name with émojis 🚀 and symbols @#$%"
	updatedPrompt, errx := UpdateSystemPrompt(ctx, originalPrompt.ID, &specialName, nil, nil, nil, nil, nil)
	require.Nil(t, errx, "UpdateSystemPrompt should handle special characters in name")
	assert.Equal(t, specialName, updatedPrompt.Name, "Name with special characters should be updated")
	assert.Equal(t, "Original content", updatedPrompt.Content, "Content should remain unchanged")

	// Test content update with special characters and newlines
	specialContent := "Special content with\nnewlines and\ttabs\nand émojis 🎯\nand symbols: @#$%^&*()"
	updatedPrompt, errx = UpdateSystemPrompt(ctx, originalPrompt.ID, nil, &specialContent, nil, nil, nil, nil)
	require.Nil(t, errx, "UpdateSystemPrompt should handle special characters in content")
	assert.Equal(t, specialName, updatedPrompt.Name, "Name should remain from previous update")
	assert.Equal(t, specialContent, updatedPrompt.Content, "Content with special characters should be updated")
}

// TestPartialUpdateSystemPromptWithLongStrings verifies partial updates with very long strings
func TestPartialUpdateSystemPromptWithLongStrings(t *testing.T) {
	setupTestDB(t)
	ctx := context.Background()

	// Create a system prompt
	originalPrompt, _ := createTestSystemPrompt(t, "Original Name", "Original content", false)

	// Test name update with long string
	longName := strings.Repeat("Very Long Name ", 50) // 750 characters
	updatedPrompt, errx := UpdateSystemPrompt(ctx, originalPrompt.ID, &longName, nil, nil, nil, nil, nil)
	require.Nil(t, errx, "UpdateSystemPrompt should handle long names")
	assert.Equal(t, longName, updatedPrompt.Name, "Long name should be updated")
	assert.Equal(t, "Original content", updatedPrompt.Content, "Content should remain unchanged")

	// Test content update with very long string
	longContent := strings.Repeat("This is a very long content string that repeats many times to test the system's ability to handle large text content. ", 100) // ~11,000 characters
	updatedPrompt, errx = UpdateSystemPrompt(ctx, originalPrompt.ID, nil, &longContent, nil, nil, nil, nil)
	require.Nil(t, errx, "UpdateSystemPrompt should handle long content")
	assert.Equal(t, longName, updatedPrompt.Name, "Name should remain from previous update")
	assert.Equal(t, longContent, updatedPrompt.Content, "Long content should be updated")
}

// TestPartialUpdateSystemPromptConcurrency verifies partial updates work correctly under concurrent access
func TestPartialUpdateSystemPromptConcurrency(t *testing.T) {
	setupTestDB(t)
	ctx := context.Background()

	// Create a system prompt
	originalPrompt, _ := createTestSystemPrompt(t, "Original Name", "Original content", false)

	// Test concurrent partial updates with reduced concurrency for SQLite
	var wg sync.WaitGroup
	errors := make(chan error, 4)
	successCount := make(chan int, 4)

	// Reduced concurrent operations for SQLite compatibility
	// Concurrent name updates
	for i := 0; i < 2; i++ {
		wg.Add(1)
		go func(index int) {
			defer wg.Done()
			name := fmt.Sprintf("Concurrent Name %d", index)
			_, errx := UpdateSystemPrompt(ctx, originalPrompt.ID, &name, nil, nil, nil, nil, nil)
			if errx != nil {
				// SQLite may have locking issues under high concurrency, which is expected
				if strings.Contains(errx.Error(), "database table is locked") || strings.Contains(errx.Error(), "not found") {
					// This is acceptable for SQLite under concurrent load
					return
				}
				errors <- fmt.Errorf("concurrent name update %d failed: %v", index, errx)
			} else {
				successCount <- 1
			}
		}(i)
	}

	// Concurrent content updates
	for i := 0; i < 2; i++ {
		wg.Add(1)
		go func(index int) {
			defer wg.Done()
			content := fmt.Sprintf("Concurrent content %d", index)
			_, errx := UpdateSystemPrompt(ctx, originalPrompt.ID, nil, &content, nil, nil, nil, nil)
			if errx != nil {
				// SQLite may have locking issues under high concurrency, which is expected
				if strings.Contains(errx.Error(), "database table is locked") || strings.Contains(errx.Error(), "not found") {
					// This is acceptable for SQLite under concurrent load
					return
				}
				errors <- fmt.Errorf("concurrent content update %d failed: %v", index, errx)
			} else {
				successCount <- 1
			}
		}(i)
	}

	wg.Wait()
	close(errors)
	close(successCount)

	// Check for unexpected errors (database locking is expected and acceptable)
	for err := range errors {
		assert.NoError(t, err, "Unexpected concurrent update errors")
	}

	// Count successful operations
	successfulOps := 0
	for range successCount {
		successfulOps++
	}

	// At least some operations should succeed
	assert.Greater(t, successfulOps, 0, "At least some concurrent operations should succeed")

	// Verify the prompt still exists and is valid
	finalPrompt, errx := GetSystemPrompt(ctx, originalPrompt.ID)
	require.Nil(t, errx, "Should be able to retrieve prompt after concurrent updates")
	assert.NotNil(t, finalPrompt)
	assert.Equal(t, originalPrompt.ID, finalPrompt.ID, "ID should remain unchanged")
	assert.NotEmpty(t, finalPrompt.Name, "Name should not be empty after concurrent updates")
	assert.NotEmpty(t, finalPrompt.Content, "Content should not be empty after concurrent updates")
}

// TestPartialUpdateSystemPromptWithMockedDependencies verifies partial updates work with mocked dependencies
func TestPartialUpdateSystemPromptWithMockedDependencies(t *testing.T) {
	// Register mock service
	llm.RegisterMockServiceForUnitTests(context.Background())

	setupTestDB(t)
	ctx := context.Background()

	// Create a system prompt
	originalPrompt, _ := createTestSystemPrompt(t, "Original Name", "Original content", false)

	// Test name-only update with mocked dependencies
	nameValue := "Mocked Name Update"
	updatedPrompt, errx := UpdateSystemPrompt(ctx, originalPrompt.ID, &nameValue, nil, nil, nil, nil, nil)
	require.Nil(t, errx, "UpdateSystemPrompt should work with mocked dependencies")
	assert.Equal(t, "Mocked Name Update", updatedPrompt.Name, "Name should be updated")
	assert.Equal(t, "Original content", updatedPrompt.Content, "Content should remain unchanged")

	// Test content-only update with mocked dependencies
	contentValue := "Mocked content update"
	updatedPrompt, errx = UpdateSystemPrompt(ctx, originalPrompt.ID, nil, &contentValue, nil, nil, nil, nil)
	require.Nil(t, errx, "UpdateSystemPrompt should work with mocked dependencies")
	assert.Equal(t, "Mocked Name Update", updatedPrompt.Name, "Name should remain from previous update")
	assert.Equal(t, "Mocked content update", updatedPrompt.Content, "Content should be updated")

	// Test both fields update with mocked dependencies
	finalName := "Final Mocked Name"
	finalContent := "Final mocked content"
	updatedPrompt, errx = UpdateSystemPrompt(ctx, originalPrompt.ID, &finalName, &finalContent, nil, nil, nil, nil)
	require.Nil(t, errx, "UpdateSystemPrompt should work with mocked dependencies")
	assert.Equal(t, "Final Mocked Name", updatedPrompt.Name, "Name should be updated")
	assert.Equal(t, "Final mocked content", updatedPrompt.Content, "Content should be updated")
}
