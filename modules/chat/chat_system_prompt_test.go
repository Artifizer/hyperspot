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
	"github.com/hypernetix/hyperspot/modules/llm"
)

// Create test system prompt for reuse across tests
func createTestSystemPrompt(t *testing.T, name, content string, uiMeta ...string) *SystemPrompt {
	ctx := context.Background()
	uiMetaValue := ""
	if len(uiMeta) > 0 {
		uiMetaValue = uiMeta[0]
	}
	prompt, errx := CreateSystemPrompt(ctx, name, content, uiMetaValue)
	require.Nil(t, errx, "Failed to create test system prompt")
	return prompt
}

// TestSystemPromptDbSaveFields verifies the DbSaveFields method
func TestSystemPromptDbSaveFields(t *testing.T) {
	setupTestDB(t)
	ctx := context.Background()

	prompt := createTestSystemPrompt(t, "Test Prompt", "Test content")

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

	prompt, errx := CreateSystemPrompt(ctx, name, content, "")
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
	originalPrompt := createTestSystemPrompt(t, "Test Prompt", "Test content")

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
		prompts[i] = createTestSystemPrompt(t, name, content)
		// Add small delay to ensure different timestamps
		time.Sleep(1 * time.Millisecond)
	}

	// Test default listing (no pagination)
	listedPrompts, errx := ListSystemPrompts(ctx, nil)
	assert.Nil(t, errx, "ListSystemPrompts should not return an error")
	assert.Equal(t, 5, len(listedPrompts), "Should return all 5 prompts")

	// Test with pagination
	pageRequest := &api.PageAPIRequest{PageSize: 2, PageNumber: 1}
	listedPrompts, errx = ListSystemPrompts(ctx, pageRequest)
	assert.Nil(t, errx, "ListSystemPrompts should not return an error")
	assert.Equal(t, 2, len(listedPrompts), "Should return 2 prompts")

	// Test ordering (default ascending)
	pageRequest = &api.PageAPIRequest{Order: "updated_at"}
	listedPrompts, errx = ListSystemPrompts(ctx, pageRequest)
	assert.Nil(t, errx, "ListSystemPrompts should not return an error")
	assert.Equal(t, 5, len(listedPrompts), "Should return all prompts")
	// First prompt should be the oldest
	assert.Equal(t, prompts[0].ID, listedPrompts[0].ID, "First prompt should be oldest")

	// Test descending order
	pageRequest = &api.PageAPIRequest{Order: "-updated_at"}
	listedPrompts, errx = ListSystemPrompts(ctx, pageRequest)
	assert.Nil(t, errx, "ListSystemPrompts should not return an error")
	assert.Equal(t, 5, len(listedPrompts), "Should return all prompts")
	// First prompt should be the newest
	assert.Equal(t, prompts[4].ID, listedPrompts[0].ID, "First prompt should be newest")
}

// TestUpdateSystemPrompt verifies system prompt updates
func TestUpdateSystemPrompt(t *testing.T) {
	setupTestDB(t)
	ctx := context.Background()

	// Create a system prompt
	originalPrompt := createTestSystemPrompt(t, "Original Name", "Original content")
	originalUpdatedAt := originalPrompt.UpdatedAtMs

	// Wait to ensure timestamp changes
	time.Sleep(10 * time.Millisecond)

	// Update the prompt
	newName := "Updated Name"
	newContent := "Updated content"
	updatedPrompt, errx := UpdateSystemPrompt(ctx, originalPrompt.ID, &newName, &newContent, nil)
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
	updatedPrompt, errx = UpdateSystemPrompt(ctx, uuid.New(), &invalidName, &invalidContent, nil)
	assert.NotNil(t, errx, "UpdateSystemPrompt should return an error for invalid prompt ID")
	assert.Nil(t, updatedPrompt)
}

// TestDeleteSystemPrompt verifies system prompt soft deletion
func TestDeleteSystemPrompt(t *testing.T) {
	setupTestDB(t)
	ctx := context.Background()

	// Create a system prompt
	prompt := createTestSystemPrompt(t, "Test Prompt", "Test content")

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

// TestAttachSystemPromptsToThread verifies bulk attachment functionality
func TestAttachSystemPromptsToThread(t *testing.T) {
	setupTestDB(t)
	ctx := context.Background()

	// Create a thread and system prompts
	thread := createTestThread(t)
	prompt1 := createTestSystemPrompt(t, "Prompt 1", "Content 1")
	prompt2 := createTestSystemPrompt(t, "Prompt 2", "Content 2")
	prompt3 := createTestSystemPrompt(t, "Prompt 3", "Content 3")

	// Test attaching multiple prompts
	promptIDs := []uuid.UUID{prompt1.ID, prompt2.ID, prompt3.ID}
	errx := AttachSystemPromptsToThread(ctx, thread.ID, promptIDs)
	require.Nil(t, errx, "AttachSystemPromptsToThread should not return an error")

	// Verify prompts are attached
	attachedPrompts, errx := GetSystemPromptsForThread(ctx, thread.ID)
	require.Nil(t, errx, "Failed to get attached prompts")
	assert.Equal(t, 3, len(attachedPrompts), "Should have 3 attached prompts")

	// Verify the correct prompts are attached
	attachedIDs := make(map[uuid.UUID]bool)
	for _, prompt := range attachedPrompts {
		attachedIDs[prompt.ID] = true
	}
	assert.True(t, attachedIDs[prompt1.ID], "Prompt 1 should be attached")
	assert.True(t, attachedIDs[prompt2.ID], "Prompt 2 should be attached")
	assert.True(t, attachedIDs[prompt3.ID], "Prompt 3 should be attached")

	// Test attaching duplicate prompts (should be ignored)
	duplicateIDs := []uuid.UUID{prompt1.ID, prompt2.ID} // Already attached
	errx = AttachSystemPromptsToThread(ctx, thread.ID, duplicateIDs)
	require.Nil(t, errx, "AttachSystemPromptsToThread should handle duplicates gracefully")

	// Verify no additional prompts were attached
	attachedPrompts, errx = GetSystemPromptsForThread(ctx, thread.ID)
	require.Nil(t, errx, "Failed to get attached prompts")
	assert.Equal(t, 3, len(attachedPrompts), "Should still have 3 attached prompts")

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
	thread := createTestThread(t)
	prompt1 := createTestSystemPrompt(t, "Prompt 1", "Content 1")
	prompt2 := createTestSystemPrompt(t, "Prompt 2", "Content 2")
	prompt3 := createTestSystemPrompt(t, "Prompt 3", "Content 3")

	// Attach all prompts first
	promptIDs := []uuid.UUID{prompt1.ID, prompt2.ID, prompt3.ID}
	errx := AttachSystemPromptsToThread(ctx, thread.ID, promptIDs)
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
	thread := createTestThread(t)
	prompt := createTestSystemPrompt(t, "Test Prompt", "Test content")

	// Test single attach
	errx := AttachSystemPromptToThread(ctx, thread.ID, prompt.ID)
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
	thread := createTestThread(t)
	prompt1 := createTestSystemPrompt(t, "Prompt 1", "Content 1")
	prompt2 := createTestSystemPrompt(t, "Prompt 2", "Content 2")
	prompt3 := createTestSystemPrompt(t, "Prompt 3", "Content 3")

	// Attach all prompts
	promptIDs := []uuid.UUID{prompt1.ID, prompt2.ID, prompt3.ID}
	errx := AttachSystemPromptsToThread(ctx, thread.ID, promptIDs)
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
	prompt1 := createTestSystemPrompt(t, "Prompt 1", "Content 1")
	prompt2 := createTestSystemPrompt(t, "Prompt 2", "Content 2")

	// Test with no attached prompts
	attachedPrompts, errx := GetSystemPromptsForThread(ctx, thread.ID)
	require.Nil(t, errx, "GetSystemPromptsForThread should not return an error")
	assert.Equal(t, 0, len(attachedPrompts), "Should have 0 attached prompts")

	// Attach prompts
	promptIDs := []uuid.UUID{prompt1.ID, prompt2.ID}
	errx = AttachSystemPromptsToThread(ctx, thread.ID, promptIDs)
	require.Nil(t, errx, "Failed to attach prompts")

	// Test getting attached prompts
	attachedPrompts, errx = GetSystemPromptsForThread(ctx, thread.ID)
	require.Nil(t, errx, "GetSystemPromptsForThread should not return an error")
	assert.Equal(t, 2, len(attachedPrompts), "Should have 2 attached prompts")

	// Verify correct prompts are returned
	attachedIDs := make(map[uuid.UUID]bool)
	for _, prompt := range attachedPrompts {
		attachedIDs[prompt.ID] = true
	}
	assert.True(t, attachedIDs[prompt1.ID], "Prompt 1 should be attached")
	assert.True(t, attachedIDs[prompt2.ID], "Prompt 2 should be attached")

	// Test with invalid thread ID
	attachedPrompts, errx = GetSystemPromptsForThread(ctx, uuid.New())
	assert.NotNil(t, errx, "GetSystemPromptsForThread should return error for invalid thread")
	assert.Nil(t, attachedPrompts)
}

// TestGetThreadsForSystemPrompt verifies getting threads for a prompt
func TestGetThreadsForSystemPrompt(t *testing.T) {
	setupTestDB(t)
	ctx := context.Background()

	// Create threads and a system prompt
	thread1 := createTestThread(t)
	thread2 := createTestThread(t)
	prompt := createTestSystemPrompt(t, "Test Prompt", "Test content")

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
	prompt, errx := CreateSystemPrompt(ctx, "Mocked Test Prompt", "You are a helpful assistant.", "")
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
	validPrompt := createTestSystemPrompt(t, "Valid Prompt", "Valid content")
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
	originalPrompt := createTestSystemPrompt(t, "Original Name", "Original content")
	originalUpdatedAt := originalPrompt.UpdatedAtMs

	// Test updating only the name
	time.Sleep(10 * time.Millisecond) // Ensure timestamp difference
	nameValue := "Updated Name Only"
	updatedPrompt, errx := UpdateSystemPrompt(ctx, originalPrompt.ID, &nameValue, nil, nil)
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
	updatedPrompt, errx = UpdateSystemPrompt(ctx, originalPrompt.ID, nil, &contentValue, nil)
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
	updatedPrompt, errx = UpdateSystemPrompt(ctx, originalPrompt.ID, &finalName, &finalContent, nil)
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
	originalPrompt := createTestSystemPrompt(t, "Original Name", "Original content")

	// Test that nil fields don't change existing values
	beforeUpdate, errx := GetSystemPrompt(ctx, originalPrompt.ID)
	require.Nil(t, errx, "Should be able to get prompt before empty update")

	updatedPrompt, errx := UpdateSystemPrompt(ctx, originalPrompt.ID, nil, nil, nil)
	require.Nil(t, errx, "UpdateSystemPrompt should not return an error for nil update")
	assert.NotNil(t, updatedPrompt)
	assert.Equal(t, beforeUpdate.Name, updatedPrompt.Name, "Name should remain unchanged with nil update")
	assert.Equal(t, beforeUpdate.Content, updatedPrompt.Content, "Content should remain unchanged with nil update")

	// Test that empty string pointers DO update the fields (this is the expected behavior)
	emptyName := ""
	emptyContent := ""
	updatedPrompt, errx = UpdateSystemPrompt(ctx, originalPrompt.ID, &emptyName, &emptyContent, nil)
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
	updatedPrompt, errx := UpdateSystemPrompt(ctx, nonExistentID, &nameValue, nil, nil)
	assert.NotNil(t, errx, "UpdateSystemPrompt should return error for non-existent ID")
	assert.Nil(t, updatedPrompt)

	// Test content-only update with invalid ID
	contentValue := "Updated Content"
	updatedPrompt, errx = UpdateSystemPrompt(ctx, nonExistentID, nil, &contentValue, nil)
	assert.NotNil(t, errx, "UpdateSystemPrompt should return error for non-existent ID")
	assert.Nil(t, updatedPrompt)

	// Test both fields update with invalid ID
	bothName := "Updated Name"
	bothContent := "Updated Content"
	updatedPrompt, errx = UpdateSystemPrompt(ctx, nonExistentID, &bothName, &bothContent, nil)
	assert.NotNil(t, errx, "UpdateSystemPrompt should return error for non-existent ID")
	assert.Nil(t, updatedPrompt)
}

// TestPartialUpdateSystemPromptPersistence verifies that partial updates are properly persisted
func TestPartialUpdateSystemPromptPersistence(t *testing.T) {
	setupTestDB(t)
	ctx := context.Background()

	// Create a system prompt
	originalPrompt := createTestSystemPrompt(t, "Original Name", "Original content")

	// Update only the name
	nameValue := "Persisted Name"
	_, errx := UpdateSystemPrompt(ctx, originalPrompt.ID, &nameValue, nil, nil)
	require.Nil(t, errx, "UpdateSystemPrompt should not return an error")

	// Retrieve the prompt from database to verify persistence
	retrievedPrompt, errx := GetSystemPrompt(ctx, originalPrompt.ID)
	require.Nil(t, errx, "Should be able to retrieve updated prompt")
	assert.Equal(t, "Persisted Name", retrievedPrompt.Name, "Name update should be persisted")
	assert.Equal(t, "Original content", retrievedPrompt.Content, "Content should remain unchanged")

	// Update only the content
	contentValue := "Persisted content"
	_, errx = UpdateSystemPrompt(ctx, originalPrompt.ID, nil, &contentValue, nil)
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
	originalPrompt := createTestSystemPrompt(t, "Original Name", "Original content")

	// Test name update with special characters
	specialName := "Special Name with émojis 🚀 and symbols @#$%"
	updatedPrompt, errx := UpdateSystemPrompt(ctx, originalPrompt.ID, &specialName, nil, nil)
	require.Nil(t, errx, "UpdateSystemPrompt should handle special characters in name")
	assert.Equal(t, specialName, updatedPrompt.Name, "Name with special characters should be updated")
	assert.Equal(t, "Original content", updatedPrompt.Content, "Content should remain unchanged")

	// Test content update with special characters and newlines
	specialContent := "Special content with\nnewlines and\ttabs\nand émojis 🎯\nand symbols: @#$%^&*()"
	updatedPrompt, errx = UpdateSystemPrompt(ctx, originalPrompt.ID, nil, &specialContent, nil)
	require.Nil(t, errx, "UpdateSystemPrompt should handle special characters in content")
	assert.Equal(t, specialName, updatedPrompt.Name, "Name should remain from previous update")
	assert.Equal(t, specialContent, updatedPrompt.Content, "Content with special characters should be updated")
}

// TestPartialUpdateSystemPromptWithLongStrings verifies partial updates with very long strings
func TestPartialUpdateSystemPromptWithLongStrings(t *testing.T) {
	setupTestDB(t)
	ctx := context.Background()

	// Create a system prompt
	originalPrompt := createTestSystemPrompt(t, "Original Name", "Original content")

	// Test name update with long string
	longName := strings.Repeat("Very Long Name ", 50) // 750 characters
	updatedPrompt, errx := UpdateSystemPrompt(ctx, originalPrompt.ID, &longName, nil, nil)
	require.Nil(t, errx, "UpdateSystemPrompt should handle long names")
	assert.Equal(t, longName, updatedPrompt.Name, "Long name should be updated")
	assert.Equal(t, "Original content", updatedPrompt.Content, "Content should remain unchanged")

	// Test content update with very long string
	longContent := strings.Repeat("This is a very long content string that repeats many times to test the system's ability to handle large text content. ", 100) // ~11,000 characters
	updatedPrompt, errx = UpdateSystemPrompt(ctx, originalPrompt.ID, nil, &longContent, nil)
	require.Nil(t, errx, "UpdateSystemPrompt should handle long content")
	assert.Equal(t, longName, updatedPrompt.Name, "Name should remain from previous update")
	assert.Equal(t, longContent, updatedPrompt.Content, "Long content should be updated")
}

// TestPartialUpdateSystemPromptConcurrency verifies partial updates work correctly under concurrent access
func TestPartialUpdateSystemPromptConcurrency(t *testing.T) {
	setupTestDB(t)
	ctx := context.Background()

	// Create a system prompt
	originalPrompt := createTestSystemPrompt(t, "Original Name", "Original content")

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
			_, errx := UpdateSystemPrompt(ctx, originalPrompt.ID, &name, nil, nil)
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
			_, errx := UpdateSystemPrompt(ctx, originalPrompt.ID, nil, &content, nil)
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
	originalPrompt := createTestSystemPrompt(t, "Original Name", "Original content")

	// Test name-only update with mocked dependencies
	nameValue := "Mocked Name Update"
	updatedPrompt, errx := UpdateSystemPrompt(ctx, originalPrompt.ID, &nameValue, nil, nil)
	require.Nil(t, errx, "UpdateSystemPrompt should work with mocked dependencies")
	assert.Equal(t, "Mocked Name Update", updatedPrompt.Name, "Name should be updated")
	assert.Equal(t, "Original content", updatedPrompt.Content, "Content should remain unchanged")

	// Test content-only update with mocked dependencies
	contentValue := "Mocked content update"
	updatedPrompt, errx = UpdateSystemPrompt(ctx, originalPrompt.ID, nil, &contentValue, nil)
	require.Nil(t, errx, "UpdateSystemPrompt should work with mocked dependencies")
	assert.Equal(t, "Mocked Name Update", updatedPrompt.Name, "Name should remain from previous update")
	assert.Equal(t, "Mocked content update", updatedPrompt.Content, "Content should be updated")

	// Test both fields update with mocked dependencies
	finalName := "Final Mocked Name"
	finalContent := "Final mocked content"
	updatedPrompt, errx = UpdateSystemPrompt(ctx, originalPrompt.ID, &finalName, &finalContent, nil)
	require.Nil(t, errx, "UpdateSystemPrompt should work with mocked dependencies")
	assert.Equal(t, "Final Mocked Name", updatedPrompt.Name, "Name should be updated")
	assert.Equal(t, "Final mocked content", updatedPrompt.Content, "Content should be updated")
}
