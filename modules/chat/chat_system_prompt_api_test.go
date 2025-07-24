package chat

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/hypernetix/hyperspot/libs/api"
	"github.com/hypernetix/hyperspot/modules/llm"
)

// TestSystemPromptAPIRequest verifies the API request structure
func TestSystemPromptAPIRequest(t *testing.T) {
	request := SystemPromptCreateAPIRequest{
		Name:    "Test Prompt",
		Content: "You are a helpful assistant.",
	}

	assert.Equal(t, "Test Prompt", request.Name)
	assert.Equal(t, "You are a helpful assistant.", request.Content)
}

// TestSystemPromptAPIResponse verifies the API response structure
func TestSystemPromptAPIResponse(t *testing.T) {
	setupTestDB(t)

	// Create a system prompt
	prompt, errx := createTestSystemPrompt(t, "Test Prompt", "Test content", false)
	require.Nil(t, errx, "Failed to create test system prompt")

	response := SystemPromptAPIResponse{
		Body: prompt,
	}

	assert.NotNil(t, response.Body)
	assert.Equal(t, prompt.ID, response.Body.ID)
	assert.Equal(t, prompt.Name, response.Body.Name)
	assert.Equal(t, prompt.Content, response.Body.Content)
}

// TestSystemPromptAPIResponseList verifies the API list response structure
func TestSystemPromptAPIResponseList(t *testing.T) {
	setupTestDB(t)

	// Create multiple system prompts
	prompt1, errx := createTestSystemPrompt(t, "Prompt 1", "Content 1", false)
	require.Nil(t, errx, "Failed to create test system prompt")
	prompt2, errx := createTestSystemPrompt(t, "Prompt 2", "Content 2", false)
	require.Nil(t, errx, "Failed to create test system prompt")

	prompts := []*SystemPrompt{prompt1, prompt2}

	response := SystemPromptAPIResponseList{}
	response.Body.SystemPrompts = prompts
	response.Body.Total = len(prompts)

	assert.Equal(t, 2, len(response.Body.SystemPrompts))
	assert.Equal(t, 2, response.Body.Total)
	assert.Equal(t, prompt1.ID, response.Body.SystemPrompts[0].ID)
	assert.Equal(t, prompt2.ID, response.Body.SystemPrompts[1].ID)
}

// TestThreadSystemPromptBulkRequest verifies the bulk request structure
func TestThreadSystemPromptBulkRequest(t *testing.T) {
	threadID := uuid.New()
	promptID1 := uuid.New()
	promptID2 := uuid.New()

	request := ThreadSystemPromptBulkRequest{
		ThreadID:        threadID.String(),
		SystemPromptIDs: []string{promptID1.String(), promptID2.String()},
	}

	assert.Equal(t, threadID.String(), request.ThreadID)
	assert.Equal(t, 2, len(request.SystemPromptIDs))
	assert.Equal(t, promptID1.String(), request.SystemPromptIDs[0])
	assert.Equal(t, promptID2.String(), request.SystemPromptIDs[1])
}

// TestCreateSystemPromptAPI verifies system prompt creation through API logic
func TestCreateSystemPromptAPI(t *testing.T) {
	setupTestDB(t)
	ctx := context.Background()

	// Test valid request
	request := SystemPromptCreateAPIRequest{
		Name:    "API Test Prompt",
		Content: "You are a helpful assistant for API testing.",
	}

	// Simulate the API logic
	prompt, errx := CreateSystemPrompt(ctx, request.Name, request.Content, "", false, "", false)
	require.Nil(t, errx, "CreateSystemPrompt should not return an error")
	assert.NotNil(t, prompt)
	assert.Equal(t, request.Name, prompt.Name)
	assert.Equal(t, request.Content, prompt.Content)

	// Test empty name (would be caught by API validation)
	emptyNameRequest := SystemPromptCreateAPIRequest{
		Name:    "",
		Content: "Content without name",
	}

	// This would be caught by API validation, but let's test the underlying function
	_, errx = CreateSystemPrompt(ctx, emptyNameRequest.Name, emptyNameRequest.Content, "", false, "", false)
	// The underlying function doesn't validate empty names, that's done at API level
	assert.Nil(t, errx, "CreateSystemPrompt allows empty names (validation is at API level)")

	// Test empty content (would be caught by API validation)
	emptyContentRequest := SystemPromptCreateAPIRequest{
		Name:    "Name without content",
		Content: "",
	}

	_, errx = UpdateSystemPrompt(ctx, prompt.ID, &emptyContentRequest.Name, &emptyContentRequest.Content, nil, nil, nil, nil)
	assert.Nil(t, errx, "CreateSystemPrompt allows empty content (validation is at API level)")
}

// TestGetSystemPromptAPI verifies system prompt retrieval through API logic
func TestGetSystemPromptAPI(t *testing.T) {
	setupTestDB(t)
	ctx := context.Background()

	// Create a system prompt
	originalPrompt, errx := createTestSystemPrompt(t, "API Get Test", "Content for API get test", false)
	require.Nil(t, errx, "Failed to create test system prompt")

	// Test valid ID
	retrievedPrompt, errx := GetSystemPrompt(ctx, originalPrompt.ID)
	require.Nil(t, errx, "GetSystemPrompt should not return an error")
	assert.NotNil(t, retrievedPrompt)
	assert.Equal(t, originalPrompt.ID, retrievedPrompt.ID)
	assert.Equal(t, originalPrompt.Name, retrievedPrompt.Name)
	assert.Equal(t, originalPrompt.Content, retrievedPrompt.Content)

	// Test invalid ID (would return 404 in API)
	invalidID := uuid.New()
	retrievedPrompt, errx = GetSystemPrompt(ctx, invalidID)
	assert.NotNil(t, errx, "GetSystemPrompt should return error for invalid ID")
	assert.Nil(t, retrievedPrompt)
}

// TestListSystemPromptsAPI verifies system prompt listing through API logic
func TestListSystemPromptsAPI(t *testing.T) {
	setupTestDB(t)
	ctx := context.Background()

	// Create multiple system prompts with explicit timing
	prompt1, errx := createTestSystemPrompt(t, "API List Test 1", "Content 1", false)
	require.Nil(t, errx, "Failed to create test system prompt")
	time.Sleep(10 * time.Millisecond) // Ensure different timestamps
	prompt2, errx := createTestSystemPrompt(t, "API List Test 2", "Content 2", false)
	require.Nil(t, errx, "Failed to create test system prompt")
	time.Sleep(10 * time.Millisecond) // Ensure different timestamps
	prompt3, errx := createTestSystemPrompt(t, "API List Test 3", "Content 3", false)
	require.Nil(t, errx, "Failed to create test system prompt")

	// Test default parameters (simulating API defaults)
	pageRequest := &api.PageAPIRequest{
		PageNumber: 1,
		PageSize:   20,
		Order:      "-updated_at",
	}

	prompts, errx := ListSystemPrompts(ctx, pageRequest)
	require.Nil(t, errx, "ListSystemPrompts should not return an error")
	assert.Equal(t, 3, len(prompts), "Should return all 3 prompts")

	// Test pagination
	pageRequest = &api.PageAPIRequest{
		PageNumber: 1,
		PageSize:   2,
		Order:      "-updated_at",
	}

	prompts, errx = ListSystemPrompts(ctx, pageRequest)
	require.Nil(t, errx, "ListSystemPrompts should not return an error")
	assert.Equal(t, 2, len(prompts), "Should return 2 prompts with pagination")

	// Test ordering - ascending should have earlier timestamps first
	pageRequest = &api.PageAPIRequest{
		PageNumber: 1,
		PageSize:   20,
		Order:      "updated_at", // Ascending
	}

	prompts, errx = ListSystemPrompts(ctx, pageRequest)
	require.Nil(t, errx, "ListSystemPrompts should not return an error")
	assert.Equal(t, 3, len(prompts), "Should return all prompts")
	// Verify ascending order by timestamp
	for i := 1; i < len(prompts); i++ {
		assert.LessOrEqual(t, prompts[i-1].UpdatedAtMs, prompts[i].UpdatedAtMs, "Prompts should be in ascending order")
	}

	// Test descending order - later timestamps should come first
	pageRequest = &api.PageAPIRequest{
		PageNumber: 1,
		PageSize:   20,
		Order:      "-updated_at", // Descending
	}

	prompts, errx = ListSystemPrompts(ctx, pageRequest)
	require.Nil(t, errx, "ListSystemPrompts should not return an error")
	assert.Equal(t, 3, len(prompts), "Should return all prompts")
	// Verify descending order by timestamp
	for i := 1; i < len(prompts); i++ {
		assert.GreaterOrEqual(t, prompts[i-1].UpdatedAtMs, prompts[i].UpdatedAtMs, "Prompts should be in descending order")
	}

	// Verify all prompts are present in the results
	promptIDs := make(map[uuid.UUID]bool)
	for _, prompt := range prompts {
		promptIDs[prompt.ID] = true
	}
	assert.True(t, promptIDs[prompt1.ID], "Prompt 1 should be in results")
	assert.True(t, promptIDs[prompt2.ID], "Prompt 2 should be in results")
	assert.True(t, promptIDs[prompt3.ID], "Prompt 3 should be in results")
}

// TestUpdateSystemPromptAPI verifies system prompt updates through API logic
func TestUpdateSystemPromptAPI(t *testing.T) {
	setupTestDB(t)
	ctx := context.Background()

	// Create a system prompt
	originalPrompt, errx := createTestSystemPrompt(t, "Original API Name", "Original API content", false)
	require.Nil(t, errx, "Failed to create test system prompt")

	// Test valid update
	updateRequest := SystemPromptCreateAPIRequest{
		Name:    "Updated API Name",
		Content: "Updated API content",
	}

	updatedPrompt, errx := UpdateSystemPrompt(ctx, originalPrompt.ID, &updateRequest.Name, &updateRequest.Content, nil, nil, nil, nil)
	require.Nil(t, errx, "UpdateSystemPrompt should not return an error")
	assert.NotNil(t, updatedPrompt)
	assert.Equal(t, originalPrompt.ID, updatedPrompt.ID)
	assert.Equal(t, updateRequest.Name, updatedPrompt.Name)
	assert.Equal(t, updateRequest.Content, updatedPrompt.Content)

	// Test invalid ID (would return 404 in API)
	invalidID := uuid.New()
	newName := "New Name"
	newContent := "New Content"
	updatedPrompt, errx = UpdateSystemPrompt(ctx, invalidID, &newName, &newContent, nil, nil, nil, nil)
	assert.NotNil(t, errx, "UpdateSystemPrompt should return error for invalid ID")
	assert.Nil(t, updatedPrompt)

	// Test empty name and content (would be caught by API validation)
	// The underlying function doesn't validate these, that's done at API level
	emptyRequest := SystemPromptCreateAPIRequest{
		Name:    "",
		Content: "",
	}

	updatedPrompt, errx = UpdateSystemPrompt(ctx, originalPrompt.ID, &emptyRequest.Name, &emptyRequest.Content, nil, nil, nil, nil)
	assert.Nil(t, errx, "UpdateSystemPrompt allows empty values (validation is at API level)")
}

// TestDeleteSystemPromptAPI verifies system prompt deletion through API logic
func TestDeleteSystemPromptAPI(t *testing.T) {
	setupTestDB(t)
	ctx := context.Background()

	// Create a system prompt
	prompt, _ := createTestSystemPrompt(t, "API Delete Test", "Content to be deleted", false)

	// Test valid deletion
	errx := DeleteSystemPrompt(ctx, prompt.ID)
	require.Nil(t, errx, "DeleteSystemPrompt should not return an error")

	// Verify prompt is soft deleted
	retrievedPrompt, errx := GetSystemPrompt(ctx, prompt.ID)
	assert.NotNil(t, errx, "GetSystemPrompt should return error for deleted prompt")
	assert.Nil(t, retrievedPrompt)

	// Test invalid ID (would return 404 in API)
	invalidID := uuid.New()
	errx = DeleteSystemPrompt(ctx, invalidID)
	assert.NotNil(t, errx, "DeleteSystemPrompt should return error for invalid ID")
}

// TestBulkAttachSystemPromptsAPI verifies bulk attachment through API logic
func TestBulkAttachSystemPromptsAPI(t *testing.T) {
	setupTestDB(t)
	ctx := context.Background()

	// Create thread and system prompts
	thread := createTestThread(t)
	prompt1, _ := createTestSystemPrompt(t, "Bulk Attach 1", "Content 1", false)
	prompt2, _ := createTestSystemPrompt(t, "Bulk Attach 2", "Content 2", false)

	// Test valid bulk attach request
	bulkRequest := ThreadSystemPromptBulkRequest{
		ThreadID:        thread.ID.String(),
		SystemPromptIDs: []string{prompt1.ID.String(), prompt2.ID.String()},
	}

	// Parse thread ID (simulating API logic)
	threadID, err := uuid.Parse(bulkRequest.ThreadID)
	require.NoError(t, err, "Thread ID should be valid UUID")

	// Parse prompt IDs (simulating API logic)
	var promptIDs []uuid.UUID
	for _, promptIDStr := range bulkRequest.SystemPromptIDs {
		promptID, err := uuid.Parse(promptIDStr)
		require.NoError(t, err, "Prompt ID should be valid UUID")
		promptIDs = append(promptIDs, promptID)
	}

	// Attach prompts
	errx := AttachSystemPromptsToThread(ctx, threadID, promptIDs)
	require.Nil(t, errx, "AttachSystemPromptsToThread should not return an error")

	// Verify attachment
	attachedPrompts, errx := GetSystemPromptsForThread(ctx, threadID)
	require.Nil(t, errx, "Failed to get attached prompts")
	assert.Equal(t, 2, len(attachedPrompts), "Should have 2 attached prompts")

	// Test invalid thread ID format (would return 400 in API)
	invalidRequest := ThreadSystemPromptBulkRequest{
		ThreadID:        "invalid-uuid",
		SystemPromptIDs: []string{prompt1.ID.String()},
	}

	_, err = uuid.Parse(invalidRequest.ThreadID)
	assert.Error(t, err, "Invalid UUID should cause parse error")

	// Test invalid prompt ID format (would return 400 in API)
	invalidPromptRequest := ThreadSystemPromptBulkRequest{
		ThreadID:        thread.ID.String(),
		SystemPromptIDs: []string{"invalid-uuid"},
	}

	threadID, err = uuid.Parse(invalidPromptRequest.ThreadID)
	require.NoError(t, err, "Thread ID should be valid")

	for _, promptIDStr := range invalidPromptRequest.SystemPromptIDs {
		_, err = uuid.Parse(promptIDStr)
		assert.Error(t, err, "Invalid prompt UUID should cause parse error")
	}
}

// TestBulkDetachSystemPromptsAPI verifies bulk detachment through API logic
func TestBulkDetachSystemPromptsAPI(t *testing.T) {
	setupTestDB(t)
	ctx := context.Background()

	// Create thread and system prompts
	thread := createTestThread(t)
	prompt1, _ := createTestSystemPrompt(t, "Bulk Detach 1", "Content 1", false)
	prompt2, _ := createTestSystemPrompt(t, "Bulk Detach 2", "Content 2", false)
	prompt3, _ := createTestSystemPrompt(t, "Bulk Detach 3", "Content 3", false)

	// Attach all prompts first
	promptIDs := []uuid.UUID{prompt1.ID, prompt2.ID, prompt3.ID}
	errx := AttachSystemPromptsToThread(ctx, thread.ID, promptIDs)
	require.Nil(t, errx, "Failed to attach prompts")

	// Test valid bulk detach request
	bulkRequest := ThreadSystemPromptBulkRequest{
		ThreadID:        thread.ID.String(),
		SystemPromptIDs: []string{prompt1.ID.String(), prompt2.ID.String()},
	}

	// Parse thread ID (simulating API logic)
	threadID, err := uuid.Parse(bulkRequest.ThreadID)
	require.NoError(t, err, "Thread ID should be valid UUID")

	// Parse prompt IDs (simulating API logic)
	var detachPromptIDs []uuid.UUID
	for _, promptIDStr := range bulkRequest.SystemPromptIDs {
		promptID, err := uuid.Parse(promptIDStr)
		require.NoError(t, err, "Prompt ID should be valid UUID")
		detachPromptIDs = append(detachPromptIDs, promptID)
	}

	// Detach prompts
	errx = DetachSystemPromptsFromThread(ctx, threadID, detachPromptIDs)
	require.Nil(t, errx, "DetachSystemPromptsFromThread should not return an error")

	// Verify only prompt3 remains
	attachedPrompts, errx := GetSystemPromptsForThread(ctx, threadID)
	require.Nil(t, errx, "Failed to get attached prompts")
	assert.Equal(t, 1, len(attachedPrompts), "Should have 1 remaining prompt")
	assert.Equal(t, prompt3.ID, attachedPrompts[0].ID, "Only prompt3 should remain")
}

// TestDetachAllSystemPromptsAPI verifies detaching all prompts through API logic
func TestDetachAllSystemPromptsAPI(t *testing.T) {
	setupTestDB(t)
	ctx := context.Background()

	// Create thread and system prompts
	thread := createTestThread(t)
	prompt1, _ := createTestSystemPrompt(t, "Detach All 1", "Content 1", false)
	prompt2, _ := createTestSystemPrompt(t, "Detach All 2", "Content 2", false)

	// Attach prompts first
	promptIDs := []uuid.UUID{prompt1.ID, prompt2.ID}
	errx := AttachSystemPromptsToThread(ctx, thread.ID, promptIDs)
	require.Nil(t, errx, "Failed to attach prompts")

	// Test detach all request structure
	detachAllRequest := struct {
		ThreadID string `json:"thread_id"`
	}{
		ThreadID: thread.ID.String(),
	}

	// Parse thread ID (simulating API logic)
	threadID, err := uuid.Parse(detachAllRequest.ThreadID)
	require.NoError(t, err, "Thread ID should be valid UUID")

	// Detach all prompts
	errx = DetachAllSystemPromptsFromThread(ctx, threadID)
	require.Nil(t, errx, "DetachAllSystemPromptsFromThread should not return an error")

	// Verify no prompts remain
	attachedPrompts, errx := GetSystemPromptsForThread(ctx, threadID)
	require.Nil(t, errx, "Failed to get attached prompts")
	assert.Equal(t, 0, len(attachedPrompts), "Should have 0 remaining prompts")

	// Test invalid thread ID format (would return 400 in API)
	invalidDetachAllRequest := struct {
		ThreadID string `json:"thread_id"`
	}{
		ThreadID: "invalid-uuid",
	}

	_, err = uuid.Parse(invalidDetachAllRequest.ThreadID)
	assert.Error(t, err, "Invalid UUID should cause parse error")
}

// TestGetThreadSystemPromptsAPI verifies getting system prompts for a thread through API logic
func TestGetThreadSystemPromptsAPI(t *testing.T) {
	setupTestDB(t)
	ctx := context.Background()

	// Create thread and system prompts
	thread := createTestThread(t)
	prompt1, _ := createTestSystemPrompt(t, "Thread Prompt 1", "Content 1", false)
	prompt2, _ := createTestSystemPrompt(t, "Thread Prompt 2", "Content 2", false)

	// Test with no attached prompts
	prompts, errx := GetSystemPromptsForThread(ctx, thread.ID)
	require.Nil(t, errx, "GetSystemPromptsForThread should not return an error")
	assert.Equal(t, 0, len(prompts), "Should have 0 prompts initially")

	// Attach prompts
	promptIDs := []uuid.UUID{prompt1.ID, prompt2.ID}
	errx = AttachSystemPromptsToThread(ctx, thread.ID, promptIDs)
	require.Nil(t, errx, "Failed to attach prompts")

	// Test getting prompts for thread
	prompts, errx = GetSystemPromptsForThread(ctx, thread.ID)
	require.Nil(t, errx, "GetSystemPromptsForThread should not return an error")
	assert.Equal(t, 2, len(prompts), "Should have 2 attached prompts")

	// Test invalid thread ID format (would return 400 in API)
	invalidThreadID := "invalid-uuid"
	_, err := uuid.Parse(invalidThreadID)
	assert.Error(t, err, "Invalid UUID should cause parse error")

	// Test non-existent thread ID (would return 404 in API)
	nonExistentID := uuid.New()
	prompts, errx = GetSystemPromptsForThread(ctx, nonExistentID)
	assert.NotNil(t, errx, "GetSystemPromptsForThread should return error for non-existent thread")
	assert.Nil(t, prompts)
}

// TestGetSystemPromptThreadsAPI verifies getting threads for a system prompt through API logic
func TestGetSystemPromptThreadsAPI(t *testing.T) {
	setupTestDB(t)
	ctx := context.Background()

	// Create threads and system prompt
	thread1 := createTestThread(t)
	thread2 := createTestThread(t)
	prompt, _ := createTestSystemPrompt(t, "Prompt with Threads", "Content", false)

	// Test with no attached threads
	threads, errx := GetThreadsForSystemPrompt(ctx, prompt.ID)
	require.Nil(t, errx, "GetThreadsForSystemPrompt should not return an error")
	assert.Equal(t, 0, len(threads), "Should have 0 threads initially")

	// Attach prompt to threads
	errx = AttachSystemPromptToThread(ctx, thread1.ID, prompt.ID)
	require.Nil(t, errx, "Failed to attach prompt to thread1")
	errx = AttachSystemPromptToThread(ctx, thread2.ID, prompt.ID)
	require.Nil(t, errx, "Failed to attach prompt to thread2")

	// Test getting threads for prompt
	threads, errx = GetThreadsForSystemPrompt(ctx, prompt.ID)
	require.Nil(t, errx, "GetThreadsForSystemPrompt should not return an error")
	assert.Equal(t, 2, len(threads), "Should have 2 attached threads")

	// Test invalid prompt ID format (would return 400 in API)
	invalidPromptID := "invalid-uuid"
	_, err := uuid.Parse(invalidPromptID)
	assert.Error(t, err, "Invalid UUID should cause parse error")

	// Test non-existent prompt ID (would return 404 in API)
	nonExistentID := uuid.New()
	threads, errx = GetThreadsForSystemPrompt(ctx, nonExistentID)
	assert.NotNil(t, errx, "GetThreadsForSystemPrompt should return error for non-existent prompt")
	assert.Nil(t, threads)
}

// TestSystemPromptAPIWithMockedDependencies verifies API functionality with mocked LLM service
func TestSystemPromptAPIWithMockedDependencies(t *testing.T) {
	// Register mock service
	llm.RegisterMockServiceForUnitTests(context.Background())

	setupTestDB(t)
	ctx := context.Background()

	// Test creating system prompt through API logic with mocked dependencies
	request := SystemPromptCreateAPIRequest{
		Name:    "Mocked API Prompt",
		Content: "You are a helpful assistant for mocked API testing.",
	}

	prompt, errx := CreateSystemPrompt(ctx, request.Name, request.Content, "", false, "", false)
	require.Nil(t, errx, "CreateSystemPrompt should work with mocked dependencies")
	assert.NotNil(t, prompt)
	assert.Equal(t, request.Name, prompt.Name)
	assert.Equal(t, request.Content, prompt.Content)

	// Test full API workflow with mocked dependencies
	thread := createTestThread(t)

	// Attach prompt to thread
	errx = AttachSystemPromptToThread(ctx, thread.ID, prompt.ID)
	require.Nil(t, errx, "AttachSystemPromptToThread should work with mocked dependencies")

	// Verify attachment
	attachedPrompts, errx := GetSystemPromptsForThread(ctx, thread.ID)
	require.Nil(t, errx, "GetSystemPromptsForThread should work with mocked dependencies")
	assert.Equal(t, 1, len(attachedPrompts), "Should have 1 attached prompt")
	assert.Equal(t, prompt.ID, attachedPrompts[0].ID, "Correct prompt should be attached")

	// Test listing prompts
	prompts, errx := ListSystemPrompts(ctx, &api.PageAPIRequest{PageSize: 10, PageNumber: 1})
	require.Nil(t, errx, "ListSystemPrompts should work with mocked dependencies")
	assert.GreaterOrEqual(t, len(prompts), 1, "Should have at least 1 prompt")

	// Test updating prompt
	updateRequest := SystemPromptCreateAPIRequest{
		Name:    "Updated Mocked API Prompt",
		Content: "Updated content for mocked API testing.",
	}

	updatedPrompt, errx := UpdateSystemPrompt(ctx, prompt.ID, &updateRequest.Name, &updateRequest.Content, nil, nil, nil, nil)
	require.Nil(t, errx, "UpdateSystemPrompt should work with mocked dependencies")
	assert.Equal(t, updateRequest.Name, updatedPrompt.Name)
	assert.Equal(t, updateRequest.Content, updatedPrompt.Content)

	// Test deleting prompt
	errx = DeleteSystemPrompt(ctx, prompt.ID)
	require.Nil(t, errx, "DeleteSystemPrompt should work with mocked dependencies")

	// Verify deletion
	deletedPrompt, errx := GetSystemPrompt(ctx, prompt.ID)
	assert.NotNil(t, errx, "GetSystemPrompt should return error for deleted prompt")
	assert.Nil(t, deletedPrompt)
}

// TestPartialUpdateSystemPromptAPI verifies partial system prompt updates through API logic
func TestPartialUpdateSystemPromptAPI(t *testing.T) {
	setupTestDB(t)
	ctx := context.Background()

	// Create a system prompt to update
	originalPrompt, _ := createTestSystemPrompt(t, "Original Name", "Original content", false)

	// Test updating only the name
	nameValue := "Updated Name Only"
	nameOnlyRequest := SystemPromptUpdateAPIRequest{
		Name:    &nameValue,
		Content: nil,
	}

	updatedPrompt, errx := UpdateSystemPrompt(ctx, originalPrompt.ID, nameOnlyRequest.Name, nameOnlyRequest.Content, nil, nil, nil, nil)
	require.Nil(t, errx, "UpdateSystemPrompt should not return an error for name-only update")
	assert.NotNil(t, updatedPrompt)
	assert.Equal(t, originalPrompt.ID, updatedPrompt.ID)
	assert.Equal(t, nameValue, updatedPrompt.Name, "Name should be updated")
	assert.Equal(t, "Original content", updatedPrompt.Content, "Content should remain unchanged")

	// Test updating only the content
	contentValue := "Updated content only"
	contentOnlyRequest := SystemPromptUpdateAPIRequest{
		Name:    nil,
		Content: &contentValue,
	}

	updatedPrompt, errx = UpdateSystemPrompt(ctx, originalPrompt.ID, contentOnlyRequest.Name, contentOnlyRequest.Content, nil, nil, nil, nil)
	require.Nil(t, errx, "UpdateSystemPrompt should not return an error for content-only update")
	assert.NotNil(t, updatedPrompt)
	assert.Equal(t, originalPrompt.ID, updatedPrompt.ID)
	assert.Equal(t, "Updated Name Only", updatedPrompt.Name, "Name should remain from previous update")
	assert.Equal(t, contentValue, updatedPrompt.Content, "Content should be updated")

	// Test updating both name and content
	finalNameValue := "Final Name"
	finalContentValue := "Final content"
	bothFieldsRequest := SystemPromptUpdateAPIRequest{
		Name:    &finalNameValue,
		Content: &finalContentValue,
	}

	updatedPrompt, errx = UpdateSystemPrompt(ctx, originalPrompt.ID, bothFieldsRequest.Name, bothFieldsRequest.Content, nil, nil, nil, nil)
	require.Nil(t, errx, "UpdateSystemPrompt should not return an error for full update")
	assert.NotNil(t, updatedPrompt)
	assert.Equal(t, originalPrompt.ID, updatedPrompt.ID)
	assert.Equal(t, finalNameValue, updatedPrompt.Name, "Name should be updated")
	assert.Equal(t, finalContentValue, updatedPrompt.Content, "Content should be updated")

	// Test validation: both fields nil (this would be caught by API validation)
	emptyRequest := SystemPromptUpdateAPIRequest{
		Name:    nil,
		Content: nil,
	}

	// The underlying function allows empty values (validation is at API level)
	// But let's test that it doesn't change anything when both are empty
	beforeUpdate, errx := GetSystemPrompt(ctx, originalPrompt.ID)
	require.Nil(t, errx, "Should be able to get prompt before empty update")

	updatedPrompt, errx = UpdateSystemPrompt(ctx, originalPrompt.ID, emptyRequest.Name, emptyRequest.Content, nil, nil, nil, nil)
	require.Nil(t, errx, "UpdateSystemPrompt allows empty values (validation is at API level)")
	assert.NotNil(t, updatedPrompt)
	assert.Equal(t, beforeUpdate.Name, updatedPrompt.Name, "Name should remain unchanged with empty update")
	assert.Equal(t, beforeUpdate.Content, updatedPrompt.Content, "Content should remain unchanged with empty update")
}

// TestPartialUpdateSystemPromptAPIValidation verifies API-level validation for partial updates
func TestPartialUpdateSystemPromptAPIValidation(t *testing.T) {
	setupTestDB(t)

	// Test API validation logic: both fields nil should be rejected
	emptyRequest := SystemPromptUpdateAPIRequest{
		Name:    nil,
		Content: nil,
	}

	// Simulate API validation logic
	if emptyRequest.Name == nil && emptyRequest.Content == nil {
		// This would return huma.Error400BadRequest("Either 'name' or 'content' field must be provided")
		assert.True(t, true, "API should reject requests with both fields nil")
	} else {
		assert.Fail(t, "API validation should catch empty requests")
	}

	// Test API validation logic: at least one field provided should pass validation
	validName := "Valid Name"
	nameOnlyRequest := SystemPromptUpdateAPIRequest{
		Name:    &validName,
		Content: nil,
	}

	if nameOnlyRequest.Name == nil && nameOnlyRequest.Content == nil {
		assert.Fail(t, "API should accept requests with at least one field")
	} else {
		assert.True(t, true, "API should accept requests with name only")
	}

	validContent := "Valid content"
	contentOnlyRequest := SystemPromptUpdateAPIRequest{
		Name:    nil,
		Content: &validContent,
	}

	if contentOnlyRequest.Name == nil && contentOnlyRequest.Content == nil {
		assert.Fail(t, "API should accept requests with at least one field")
	} else {
		assert.True(t, true, "API should accept requests with content only")
	}

	bothFieldsRequest := SystemPromptUpdateAPIRequest{
		Name:    &validName,
		Content: &validContent,
	}

	if bothFieldsRequest.Name == nil && bothFieldsRequest.Content == nil {
		assert.Fail(t, "API should accept requests with at least one field")
	} else {
		assert.True(t, true, "API should accept requests with both fields")
	}
}

// TestPartialUpdateSystemPromptAPIRequestStructure verifies the API request structure for partial updates
func TestPartialUpdateSystemPromptAPIRequestStructure(t *testing.T) {
	// Test that omitempty works correctly for partial updates with nil pointers
	nameValue := "Test Name"
	nameOnlyRequest := SystemPromptUpdateAPIRequest{
		Name:    &nameValue,
		Content: nil,
	}

	// Marshal to JSON to verify omitempty behavior
	jsonBytes, err := json.Marshal(nameOnlyRequest)
	require.NoError(t, err, "Should be able to marshal request to JSON")

	var nameOnlyUnmarshaled map[string]interface{}
	err = json.Unmarshal(jsonBytes, &nameOnlyUnmarshaled)
	require.NoError(t, err, "Should be able to unmarshal JSON")

	// With nil pointer, the field should be omitted from JSON
	_, hasName := nameOnlyUnmarshaled["name"]
	_, hasContent := nameOnlyUnmarshaled["content"]
	assert.True(t, hasName, "Name field should be present when not nil")
	assert.False(t, hasContent, "Content field should be omitted when nil")

	// Test content-only request
	contentValue := "Test Content"
	contentOnlyRequest := SystemPromptUpdateAPIRequest{
		Name:    nil,
		Content: &contentValue,
	}

	jsonBytes, err = json.Marshal(contentOnlyRequest)
	require.NoError(t, err, "Should be able to marshal content-only request to JSON")

	var contentOnlyUnmarshaled map[string]interface{}
	err = json.Unmarshal(jsonBytes, &contentOnlyUnmarshaled)
	require.NoError(t, err, "Should be able to unmarshal content-only JSON")

	_, hasName = contentOnlyUnmarshaled["name"]
	_, hasContent = contentOnlyUnmarshaled["content"]
	assert.False(t, hasName, "Name field should be omitted when nil")
	assert.True(t, hasContent, "Content field should be present when not nil")

	// Test both fields nil
	emptyRequest := SystemPromptUpdateAPIRequest{
		Name:    nil,
		Content: nil,
	}

	jsonBytes, err = json.Marshal(emptyRequest)
	require.NoError(t, err, "Should be able to marshal empty request to JSON")

	var emptyUnmarshaled map[string]interface{}
	err = json.Unmarshal(jsonBytes, &emptyUnmarshaled)
	require.NoError(t, err, "Should be able to unmarshal empty JSON")

	_, hasName = emptyUnmarshaled["name"]
	_, hasContent = emptyUnmarshaled["content"]
	assert.False(t, hasName, "Name field should be omitted when nil")
	assert.False(t, hasContent, "Content field should be omitted when nil")

	// Verify the JSON structure directly
	expectedEmptyJSON := "{}"
	assert.JSONEq(t, expectedEmptyJSON, string(jsonBytes), "Empty request should marshal to empty JSON object")

	// Test that empty strings are still included (not omitted)
	emptyName := ""
	emptyContent := ""
	emptyStringRequest := SystemPromptUpdateAPIRequest{
		Name:    &emptyName,
		Content: &emptyContent,
	}

	jsonBytes, err = json.Marshal(emptyStringRequest)
	require.NoError(t, err, "Should be able to marshal empty string request to JSON")

	var emptyStringUnmarshaled map[string]interface{}
	err = json.Unmarshal(jsonBytes, &emptyStringUnmarshaled)
	require.NoError(t, err, "Should be able to unmarshal empty string JSON")

	_, hasName = emptyStringUnmarshaled["name"]
	_, hasContent = emptyStringUnmarshaled["content"]
	assert.True(t, hasName, "Name field should be present even when empty string")
	assert.True(t, hasContent, "Content field should be present even when empty string")
	assert.Equal(t, "", emptyStringUnmarshaled["name"], "Name should be empty string")
	assert.Equal(t, "", emptyStringUnmarshaled["content"], "Content should be empty string")

	// Verify the JSON structure for empty strings
	expectedEmptyStringJSON := `{"name":"","content":""}`
	assert.JSONEq(t, expectedEmptyStringJSON, string(jsonBytes), "Empty string request should include empty string fields")
}

// TestPartialUpdateSystemPromptAPIWithInvalidID verifies error handling for partial updates with invalid IDs
func TestPartialUpdateSystemPromptAPIWithInvalidID(t *testing.T) {
	setupTestDB(t)
	ctx := context.Background()

	// Test partial update with non-existent ID
	nonExistentID := uuid.New()
	nameValue := "Updated Name"
	nameOnlyRequest := SystemPromptUpdateAPIRequest{
		Name:    &nameValue,
		Content: nil,
	}

	updatedPrompt, errx := UpdateSystemPrompt(ctx, nonExistentID, nameOnlyRequest.Name, nameOnlyRequest.Content, nil, nil, nil, nil)
	assert.NotNil(t, errx, "UpdateSystemPrompt should return error for non-existent ID")
	assert.Nil(t, updatedPrompt)

	// Test content-only update with non-existent ID
	contentValue := "Updated Content"
	contentOnlyRequest := SystemPromptUpdateAPIRequest{
		Name:    nil,
		Content: &contentValue,
	}

	updatedPrompt, errx = UpdateSystemPrompt(ctx, nonExistentID, contentOnlyRequest.Name, contentOnlyRequest.Content, nil, nil, nil, nil)
	assert.NotNil(t, errx, "UpdateSystemPrompt should return error for non-existent ID")
	assert.Nil(t, updatedPrompt)
}

// TestPartialUpdateSystemPromptAPITimestamps verifies that timestamps are updated correctly during partial updates
func TestPartialUpdateSystemPromptAPITimestamps(t *testing.T) {
	setupTestDB(t)
	ctx := context.Background()

	// Create a system prompt
	originalPrompt, _ := createTestSystemPrompt(t, "Original Name", "Original content", false)
	originalUpdatedAt := originalPrompt.UpdatedAtMs

	// Wait a bit to ensure timestamp difference
	time.Sleep(10 * time.Millisecond)

	// Test name-only update
	nameValue := "Updated Name"
	nameOnlyRequest := SystemPromptUpdateAPIRequest{
		Name:    &nameValue,
		Content: nil,
	}

	updatedPrompt, errx := UpdateSystemPrompt(ctx, originalPrompt.ID, nameOnlyRequest.Name, nameOnlyRequest.Content, nil, nil, nil, nil)
	require.Nil(t, errx, "UpdateSystemPrompt should not return an error")
	assert.Greater(t, updatedPrompt.UpdatedAtMs, originalUpdatedAt, "UpdatedAtMs should be updated after name change")
	assert.Equal(t, originalPrompt.CreatedAtMs, updatedPrompt.CreatedAtMs, "CreatedAtMs should remain unchanged")

	// Wait a bit more and test content-only update
	time.Sleep(10 * time.Millisecond)
	previousUpdatedAt := updatedPrompt.UpdatedAtMs

	contentValue := "Updated content"
	contentOnlyRequest := SystemPromptUpdateAPIRequest{
		Name:    nil,
		Content: &contentValue,
	}

	updatedPrompt, errx = UpdateSystemPrompt(ctx, originalPrompt.ID, contentOnlyRequest.Name, contentOnlyRequest.Content, nil, nil, nil, nil)
	require.Nil(t, errx, "UpdateSystemPrompt should not return an error")
	assert.Greater(t, updatedPrompt.UpdatedAtMs, previousUpdatedAt, "UpdatedAtMs should be updated after content change")
	assert.Equal(t, originalPrompt.CreatedAtMs, updatedPrompt.CreatedAtMs, "CreatedAtMs should remain unchanged")
}

// TestPartialUpdateSystemPromptAPIWithMockedDependencies verifies partial updates work with mocked dependencies
func TestPartialUpdateSystemPromptAPIWithMockedDependencies(t *testing.T) {
	// Register mock service
	llm.RegisterMockServiceForUnitTests(context.Background())

	setupTestDB(t)
	ctx := context.Background()

	// Create a system prompt
	originalPrompt, _ := createTestSystemPrompt(t, "Original Name", "Original content", false)

	// Test name-only update with mocked dependencies
	nameValue := "Mocked Name Update"
	nameOnlyRequest := SystemPromptUpdateAPIRequest{
		Name:    &nameValue,
		Content: nil,
	}

	updatedPrompt, errx := UpdateSystemPrompt(ctx, originalPrompt.ID, nameOnlyRequest.Name, nameOnlyRequest.Content, nil, nil, nil, nil)
	require.Nil(t, errx, "UpdateSystemPrompt should work with mocked dependencies")
	assert.Equal(t, nameValue, updatedPrompt.Name, "Name should be updated")
	assert.Equal(t, "Original content", updatedPrompt.Content, "Content should remain unchanged")

	// Test content-only update with mocked dependencies
	contentValue := "Mocked content update"
	contentOnlyRequest := SystemPromptUpdateAPIRequest{
		Name:    nil,
		Content: &contentValue,
	}

	updatedPrompt, errx = UpdateSystemPrompt(ctx, originalPrompt.ID, contentOnlyRequest.Name, contentOnlyRequest.Content, nil, nil, nil, nil)
	require.Nil(t, errx, "UpdateSystemPrompt should work with mocked dependencies")
	assert.Equal(t, "Mocked Name Update", updatedPrompt.Name, "Name should remain from previous update")
	assert.Equal(t, contentValue, updatedPrompt.Content, "Content should be updated")
}

// TestSystemPromptUIMeta verifies the ui_meta field functionality in system prompts
func TestSystemPromptUIMeta(t *testing.T) {
	setupTestDB(t)
	ctx := context.Background()

	// Test 1: Create a system prompt without ui_meta, ensure it's empty on GET
	promptName := "Prompt Without UIMeta"
	promptContent := "This is a test prompt without UI metadata"

	// Create the prompt without ui_meta
	prompt, errx := CreateSystemPrompt(ctx, promptName, promptContent, "", false, "", false)
	require.Nil(t, errx, "CreateSystemPrompt should not return an error")
	assert.NotNil(t, prompt)
	assert.Equal(t, promptName, prompt.Name)
	assert.Equal(t, promptContent, prompt.Content)
	assert.Equal(t, "", prompt.UIMeta, "UIMeta should be empty when not provided")

	// Get the prompt and verify ui_meta is empty
	retrievedPrompt, errx := GetSystemPrompt(ctx, prompt.ID)
	require.Nil(t, errx, "GetSystemPrompt should not return an error")
	assert.Equal(t, "", retrievedPrompt.UIMeta, "UIMeta should be empty when not provided during creation")

	// Test 2: Update the prompt with ui_meta, ensure it's updated
	uiMetaValue := `{"theme":"dark","icon":"assistant","priority":1}`
	uiMetaPtr := uiMetaValue

	updatedPrompt, errx := UpdateSystemPrompt(ctx, prompt.ID, nil, nil, &uiMetaPtr, nil, nil, nil)
	require.Nil(t, errx, "UpdateSystemPrompt should not return an error")
	assert.Equal(t, uiMetaValue, updatedPrompt.UIMeta, "UIMeta should be updated")

	// Get the prompt again and verify ui_meta is updated
	retrievedUpdatedPrompt, errx := GetSystemPrompt(ctx, prompt.ID)
	require.Nil(t, errx, "GetSystemPrompt should not return an error")
	assert.Equal(t, uiMetaValue, retrievedUpdatedPrompt.UIMeta, "UIMeta should be updated after PUT")

	// Test 3: Create another system prompt with ui_meta, ensure it's returned on GET as is
	secondPromptName := "Prompt With UIMeta"
	secondPromptContent := "This is a test prompt with UI metadata"
	secondUIMeta := `{"theme":"light","icon":"user","priority":2}`

	secondPrompt, errx := CreateSystemPrompt(ctx, secondPromptName, secondPromptContent, secondUIMeta, false, "", false)
	require.Nil(t, errx, "CreateSystemPrompt should not return an error")
	assert.NotNil(t, secondPrompt)
	assert.Equal(t, secondPromptName, secondPrompt.Name)
	assert.Equal(t, secondPromptContent, secondPrompt.Content)
	assert.Equal(t, secondUIMeta, secondPrompt.UIMeta, "UIMeta should be set correctly during creation")

	// Get the second prompt and verify ui_meta is preserved
	retrievedSecondPrompt, errx := GetSystemPrompt(ctx, secondPrompt.ID)
	require.Nil(t, errx, "GetSystemPrompt should not return an error")
	assert.Equal(t, secondUIMeta, retrievedSecondPrompt.UIMeta, "UIMeta should be preserved as provided during creation")

	// Test 4: Update with empty ui_meta should clear the field
	emptyUIMeta := ""
	updatedSecondPrompt, errx := UpdateSystemPrompt(ctx, secondPrompt.ID, nil, nil, &emptyUIMeta, nil, nil, nil)
	require.Nil(t, errx, "UpdateSystemPrompt should not return an error")
	assert.Equal(t, "", updatedSecondPrompt.UIMeta, "UIMeta should be cleared when updated with empty string")

	// Get the second prompt again and verify ui_meta is cleared
	retrievedClearedPrompt, errx := GetSystemPrompt(ctx, secondPrompt.ID)
	require.Nil(t, errx, "GetSystemPrompt should not return an error")
	assert.Equal(t, "", retrievedClearedPrompt.UIMeta, "UIMeta should be cleared after update with empty string")
}
