package chat

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/hypernetix/hyperspot/libs/api"
	"github.com/hypernetix/hyperspot/modules/syscap"
)

// TestSystemPromptFileAPIRequest verifies the API request structure for file-based prompts
func TestSystemPromptFileAPIRequest(t *testing.T) {
	request := SystemPromptCreateAPIRequest{
		Name:       "File Prompt",
		Content:    "",
		FilePath:   "/path/to/file.txt",
		AutoUpdate: true,
	}

	assert.Equal(t, "File Prompt", request.Name)
	assert.Equal(t, "", request.Content)
	assert.Equal(t, "/path/to/file.txt", request.FilePath)
	assert.True(t, request.AutoUpdate)
}

// TestSystemPromptFileUpdateAPIRequest verifies the API update request structure for file-based prompts
func TestSystemPromptFileUpdateAPIRequest(t *testing.T) {
	AutoUpdate := true
	request := SystemPromptUpdateAPIRequest{
		Name:       nil,
		Content:    nil,
		UIMeta:     nil,
		AutoUpdate: &AutoUpdate,
	}

	assert.Nil(t, request.Name)
	assert.Nil(t, request.Content)
	assert.Nil(t, request.UIMeta)
	assert.NotNil(t, request.AutoUpdate)
	assert.True(t, *request.AutoUpdate)
}

// TestCreateSystemPromptFromFileAPI verifies system prompt creation from a file through API
func TestCreateSystemPromptFromFileAPI(t *testing.T) {
	setupTestDB(t)
	ctx := context.Background()

	// Register desktop syscap to allow file operations
	sc := &syscap.SysCap{
		Category: syscap.CategoryModule,
		Name:     syscap.SysCapaModuleDesktop,
		Present:  true,
	}
	syscap.RegisterSysCap(sc)
	defer syscap.UnregisterSysCap(sc)

	// Create a temporary file with content
	tempDir := t.TempDir()
	filePath := filepath.Join(tempDir, "api_test_prompt.txt")
	fileContent := "API test prompt content from file."
	err := os.WriteFile(filePath, []byte(fileContent), 0644)
	require.NoError(t, err, "Failed to create test file")

	// Create API request
	request := SystemPromptCreateAPIRequest{
		Name:       "API File Prompt",
		Content:    "",
		FilePath:   filePath,
		AutoUpdate: true,
	}

	// Simulate API handler logic
	prompt, errx := CreateSystemPrompt(
		ctx,
		request.Name,
		request.Content,
		request.UIMeta,
		false,
		request.FilePath,
		request.AutoUpdate,
	)
	require.Nil(t, errx, "CreateSystemPrompt should not return an error")

	// Verify response
	response := SystemPromptAPIResponse{
		Body: prompt,
	}

	assert.Equal(t, request.Name, response.Body.Name)
	assert.Equal(t, fileContent, response.Body.Content)
	assert.Equal(t, filePath, response.Body.FilePath)
	assert.True(t, response.Body.AutoUpdate)
}

// TestUpdateSystemPromptFromFileAPI verifies system prompt updates from a file through API
func TestUpdateSystemPromptFromFileAPI(t *testing.T) {
	setupTestDB(t)
	ctx := context.Background()

	// Register desktop syscap to allow file operations
	sc := &syscap.SysCap{
		Category: syscap.CategoryModule,
		Name:     syscap.SysCapaModuleDesktop,
		Present:  true,
	}
	syscap.RegisterSysCap(sc)
	defer syscap.UnregisterSysCap(sc)

	// Create a temporary file with content
	tempDir := t.TempDir()
	filePath := filepath.Join(tempDir, "api_update_test.txt")
	initialContent := "Initial content for API update test"
	err := os.WriteFile(filePath, []byte(initialContent), 0644)
	require.NoError(t, err, "Failed to create test file")

	// Create a system prompt with file but without auto-update
	prompt, errx := CreateSystemPrompt(ctx, "API Update Test", "", "", false, filePath, false)
	require.Nil(t, errx, "CreateSystemPrompt should not return an error")

	// Update the file content
	updatedContent := "Updated content for API update test"
	err = os.WriteFile(filePath, []byte(updatedContent), 0644)
	require.NoError(t, err, "Failed to update test file")

	// Create API update request to enable auto-update
	AutoUpdate := true
	request := SystemPromptUpdateAPIRequest{
		AutoUpdate: &AutoUpdate,
	}

	// Simulate API handler logic
	updatedPrompt, errx := UpdateSystemPrompt(
		ctx,
		prompt.ID,
		request.Name,
		request.Content,
		request.UIMeta,
		request.IsDefault,
		request.FilePath,
		request.AutoUpdate,
	)
	require.Nil(t, errx, "UpdateSystemPrompt should not return an error")

	// Verify response
	response := SystemPromptAPIResponse{
		Body: updatedPrompt,
	}

	assert.Equal(t, updatedContent, updatedPrompt.GetContent())
	assert.True(t, response.Body.AutoUpdate)
}

// TestCreateSystemPromptFromFileAPIValidation verifies API-level validation for file-based prompts
func TestCreateSystemPromptFromFileAPIValidation(t *testing.T) {
	setupTestDB(t)
	ctx := context.Background()

	// Register desktop syscap to allow file operations
	sc := &syscap.SysCap{
		Category: syscap.CategoryModule,
		Name:     syscap.SysCapaModuleDesktop,
		Present:  true,
	}
	syscap.RegisterSysCap(sc)
	defer syscap.UnregisterSysCap(sc)

	// Test with non-existent file
	request := SystemPromptCreateAPIRequest{
		Name:       "Invalid File Prompt",
		Content:    "",
		FilePath:   "/path/to/nonexistent/file.txt",
		AutoUpdate: true,
	}

	// Simulate API handler logic
	_, errx := CreateSystemPrompt(
		ctx,
		request.Name,
		request.Content,
		request.UIMeta,
		request.IsDefault,
		request.FilePath,
		request.AutoUpdate,
	)
	require.NotNil(t, errx, "CreateSystemPrompt should return an error for non-existent file")
	assert.Contains(t, errx.Error(), "does not exist", "Error should indicate file doesn't exist")
}

// TestUpdateSystemPromptFromFileAPIValidation verifies API-level validation for file-based prompt updates
func TestUpdateSystemPromptFromFileAPIValidation(t *testing.T) {
	setupTestDB(t)
	ctx := context.Background()

	// Register desktop syscap to allow file operations
	sc := &syscap.SysCap{
		Category: syscap.CategoryModule,
		Name:     syscap.SysCapaModuleDesktop,
		Present:  true,
	}
	syscap.RegisterSysCap(sc)
	defer syscap.UnregisterSysCap(sc)

	// Create a regular system prompt without file
	prompt, errx := CreateSystemPrompt(ctx, "Regular API Prompt", "Regular content", "", false, "", false)
	require.Nil(t, errx, "CreateSystemPrompt should not return an error")

	// Try to enable auto-update without setting a file
	AutoUpdate := true
	request := SystemPromptUpdateAPIRequest{
		AutoUpdate: &AutoUpdate,
	}

	// Simulate API handler logic
	_, errx = UpdateSystemPrompt(
		ctx,
		prompt.ID,
		request.Name,
		request.Content,
		request.UIMeta,
		request.IsDefault,
		request.FilePath,
		request.AutoUpdate,
	)
	require.NotNil(t, errx, "UpdateSystemPrompt should return an error when enabling auto-update without a file")
	assert.Contains(t, errx.Error(), "file is not set", "Error should indicate file is not set")
}

// TestSystemPromptFileAPIResponseSerialization verifies API response serialization for file-based prompts
func TestSystemPromptFileAPIResponseSerialization(t *testing.T) {
	setupTestDB(t)
	ctx := context.Background()

	// Register desktop syscap to allow file operations
	sc := &syscap.SysCap{
		Category: syscap.CategoryModule,
		Name:     syscap.SysCapaModuleDesktop,
		Present:  true,
	}
	syscap.RegisterSysCap(sc)
	defer syscap.UnregisterSysCap(sc)

	// Create a temporary file with content
	tempDir := t.TempDir()
	filePath := filepath.Join(tempDir, "serialization_test.txt")
	fileContent := "Content for serialization test"
	err := os.WriteFile(filePath, []byte(fileContent), 0644)
	require.NoError(t, err, "Failed to create test file")

	// Create a system prompt with file
	prompt, errx := CreateSystemPrompt(ctx, "Serialization Test", "", "", false, filePath, true)
	require.Nil(t, errx, "CreateSystemPrompt should not return an error")

	// Create API response
	response := SystemPromptAPIResponse{
		Body: prompt,
	}

	// Serialize to JSON
	jsonData, err := json.Marshal(response)
	require.NoError(t, err, "Failed to marshal response to JSON")

	// Deserialize from JSON
	var deserializedResponse SystemPromptAPIResponse
	err = json.Unmarshal(jsonData, &deserializedResponse)
	require.NoError(t, err, "Failed to unmarshal response from JSON")

	// Verify fields are correctly serialized and deserialized
	assert.Equal(t, prompt.ID, deserializedResponse.Body.ID)
	assert.Equal(t, prompt.Name, deserializedResponse.Body.Name)
	assert.Equal(t, prompt.Content, deserializedResponse.Body.Content)
	assert.Equal(t, prompt.FilePath, deserializedResponse.Body.FilePath)
	assert.Equal(t, prompt.AutoUpdate, deserializedResponse.Body.AutoUpdate)
}

// TestListSystemPromptsAPIWithFilePrompts verifies listing system prompts that include file-based prompts
func TestListSystemPromptsAPIWithFilePrompts(t *testing.T) {
	setupTestDB(t)
	ctx := context.Background()

	// Register desktop syscap to allow file operations
	sc := &syscap.SysCap{
		Category: syscap.CategoryModule,
		Name:     syscap.SysCapaModuleDesktop,
		Present:  true,
	}
	syscap.RegisterSysCap(sc)
	defer syscap.UnregisterSysCap(sc)

	// Create a temporary file with content
	tempDir := t.TempDir()
	filePath := filepath.Join(tempDir, "list_test.txt")
	fileContent := "Content for list test"
	err := os.WriteFile(filePath, []byte(fileContent), 0644)
	require.NoError(t, err, "Failed to create test file")

	// Create multiple system prompts, including file-based ones
	regularPrompt, _ := createTestSystemPrompt(t, "Regular Prompt", "Regular content", false)
	filePrompt, errx := CreateSystemPrompt(ctx, "File Prompt", "", "", false, filePath, true)
	require.Nil(t, errx, "CreateSystemPrompt should not return an error")

	// Create page request
	pageRequest := &api.PageAPIRequest{
		PageNumber: 1,
		PageSize:   10,
	}

	// List system prompts
	prompts, errx := ListSystemPrompts(ctx, pageRequest, nil)
	require.Nil(t, errx, "ListSystemPrompts should not return an error")

	// Verify both prompts are in the list
	foundRegular := false
	foundFile := false
	for _, p := range prompts {
		if p.ID == regularPrompt.ID {
			foundRegular = true
			assert.Equal(t, "Regular Prompt", p.Name)
			assert.Equal(t, "Regular content", p.Content)
			assert.Empty(t, p.FilePath)
			assert.False(t, p.AutoUpdate)
		}
		if p.ID == filePrompt.ID {
			foundFile = true
			assert.Equal(t, "File Prompt", p.Name)
			assert.Equal(t, fileContent, p.Content)
			assert.Equal(t, filePath, p.FilePath)
			assert.True(t, p.AutoUpdate)
		}
	}

	assert.True(t, foundRegular, "Regular prompt should be in the list")
	assert.True(t, foundFile, "File-based prompt should be in the list")
}

// TestGetSystemPromptAPIWithFilePrompt verifies getting a file-based system prompt by ID
func TestGetSystemPromptAPIWithFilePrompt(t *testing.T) {
	setupTestDB(t)
	ctx := context.Background()

	// Register desktop syscap to allow file operations
	sc := &syscap.SysCap{
		Category: syscap.CategoryModule,
		Name:     syscap.SysCapaModuleDesktop,
		Present:  true,
	}
	syscap.RegisterSysCap(sc)
	defer syscap.UnregisterSysCap(sc)

	// Create a temporary file with content
	tempDir := t.TempDir()
	filePath := filepath.Join(tempDir, "get_test.txt")
	fileContent := "Content for get test"
	err := os.WriteFile(filePath, []byte(fileContent), 0644)
	require.NoError(t, err, "Failed to create test file")

	// Create a file-based system prompt
	prompt, errx := CreateSystemPrompt(ctx, "Get Test Prompt", "", "", false, filePath, true)
	require.Nil(t, errx, "CreateSystemPrompt should not return an error")

	// Get the prompt by ID
	retrievedPrompt, errx := GetSystemPrompt(ctx, prompt.ID)
	require.Nil(t, errx, "GetSystemPrompt should not return an error")

	// Verify the prompt fields
	assert.Equal(t, prompt.ID, retrievedPrompt.ID)
	assert.Equal(t, "Get Test Prompt", retrievedPrompt.Name)
	assert.Equal(t, fileContent, retrievedPrompt.Content)
	assert.Equal(t, filePath, retrievedPrompt.FilePath)
	assert.True(t, retrievedPrompt.AutoUpdate)
}

// TestDeleteSystemPromptAPIWithFilePrompt verifies deleting a file-based system prompt
func TestDeleteSystemPromptAPIWithFilePrompt(t *testing.T) {
	setupTestDB(t)
	ctx := context.Background()

	// Register desktop syscap to allow file operations
	sc := &syscap.SysCap{
		Category: syscap.CategoryModule,
		Name:     syscap.SysCapaModuleDesktop,
		Present:  true,
	}
	syscap.RegisterSysCap(sc)
	defer syscap.UnregisterSysCap(sc)

	// Create a temporary file with content
	tempDir := t.TempDir()
	filePath := filepath.Join(tempDir, "delete_test.txt")
	fileContent := "Content for delete test"
	err := os.WriteFile(filePath, []byte(fileContent), 0644)
	require.NoError(t, err, "Failed to create test file")

	// Create a file-based system prompt
	prompt, errx := CreateSystemPrompt(ctx, "Delete Test Prompt", "", "", false, filePath, true)
	require.Nil(t, errx, "CreateSystemPrompt should not return an error")

	// Delete the prompt
	errx = DeleteSystemPrompt(ctx, prompt.ID)
	require.Nil(t, errx, "DeleteSystemPrompt should not return an error")

	// Try to get the deleted prompt
	_, errx = GetSystemPrompt(ctx, prompt.ID)
	require.NotNil(t, errx, "GetSystemPrompt should return an error for deleted prompt")
	assert.Contains(t, errx.Error(), "not found", "Error should indicate prompt is not found")
}

// TestUpdateSystemPromptFileOperations verifies file path update and removal operations through the API
func TestUpdateSystemPromptFileOperations(t *testing.T) {
	setupTestDB(t)
	ctx := context.Background()

	// Register desktop syscap to allow file operations
	sc := &syscap.SysCap{
		Category: syscap.CategoryModule,
		Name:     syscap.SysCapaModuleDesktop,
		Present:  true,
	}
	syscap.RegisterSysCap(sc)
	defer syscap.UnregisterSysCap(sc)

	t.Run("Change file path through API", func(t *testing.T) {
		// Create test files
		tempDir := t.TempDir()
		file1 := filepath.Join(tempDir, "file1.txt")
		file2 := filepath.Join(tempDir, "file2.txt")

		err := os.WriteFile(file1, []byte("First file content"), 0644)
		require.NoError(t, err)

		err = os.WriteFile(file2, []byte("Second file content"), 0644)
		require.NoError(t, err)

		// Create initial prompt
		prompt, errx := CreateSystemPrompt(ctx, "API Update Test", "", "", false, file1, true)
		require.Nil(t, errx)

		// Create API request to update file path
		autoUpdate := true
		req := SystemPromptUpdateAPIRequest{
			FilePath:   &file2,
			AutoUpdate: &autoUpdate,
		}

		// Convert to JSON
		reqBody, err := json.Marshal(req)
		require.NoError(t, err)

		// Simulate API call
		var resp SystemPromptAPIResponse
		err = json.Unmarshal(reqBody, &resp)
		require.NoError(t, err)

		// Update the prompt
		updatedPrompt, errx := UpdateSystemPrompt(
			ctx,
			prompt.ID,
			nil, // name
			nil, // content
			nil, // uiMeta
			nil, // isDefault
			req.FilePath,
			req.AutoUpdate,
		)
		require.Nil(t, errx)

		// Verify the response
		assert.Equal(t, file2, updatedPrompt.FilePath)
		assert.Equal(t, "Second file content", updatedPrompt.Content)
		assert.True(t, updatedPrompt.AutoUpdate)

		// Verify database state
		dbPrompt, errx := GetSystemPrompt(ctx, prompt.ID)
		require.Nil(t, errx)
		assert.Equal(t, file2, dbPrompt.FilePath)
	})

	t.Run("Remove file path through API", func(t *testing.T) {
		// Create test file
		tempDir := t.TempDir()
		filePath := filepath.Join(tempDir, "file.txt")

		err := os.WriteFile(filePath, []byte("Test content"), 0644)
		require.NoError(t, err)

		// Create initial prompt with file
		prompt, errx := CreateSystemPrompt(ctx, "API Remove Test", "", "", false, filePath, true)
		require.Nil(t, errx)

		// Create API request to remove file path
		emptyPath := ""
		autoUpdate := false
		req := SystemPromptUpdateAPIRequest{
			FilePath:   &emptyPath,
			AutoUpdate: &autoUpdate,
		}

		// Convert to JSON
		reqBody, err := json.Marshal(req)
		require.NoError(t, err)

		// Simulate API call
		var resp SystemPromptAPIResponse
		err = json.Unmarshal(reqBody, &resp)
		require.NoError(t, err)

		// Update the prompt
		updatedPrompt, errx := UpdateSystemPrompt(
			ctx,
			prompt.ID,
			nil, // name
			nil, // content
			nil, // uiMeta
			nil, // isDefault
			req.FilePath,
			req.AutoUpdate,
		)
		require.Nil(t, errx)

		// Verify the response
		assert.Empty(t, updatedPrompt.FilePath)
		assert.Empty(t, updatedPrompt.Content)
		assert.False(t, updatedPrompt.AutoUpdate)

		// Verify database state
		dbPrompt, errx := GetSystemPrompt(ctx, prompt.ID)
		require.Nil(t, errx)
		assert.Empty(t, dbPrompt.FilePath)
		assert.False(t, dbPrompt.AutoUpdate)
	})

	t.Run("Update to non-existent file should fail", func(t *testing.T) {
		// Create initial prompt
		tempDir := t.TempDir()
		filePath := filepath.Join(tempDir, "file.txt")

		err := os.WriteFile(filePath, []byte("Test content"), 0644)
		require.NoError(t, err)

		prompt, errx := CreateSystemPrompt(ctx, "API Invalid Update", "", "", false, filePath, true)
		require.Nil(t, errx)

		// Try to update to non-existent file
		nonExistentPath := filepath.Join(tempDir, "nonexistent.txt")
		autoUpdate := true
		req := SystemPromptUpdateAPIRequest{
			FilePath:   &nonExistentPath,
			AutoUpdate: &autoUpdate,
		}

		// This should fail
		_, errx = UpdateSystemPrompt(
			ctx,
			prompt.ID,
			nil, // name
			nil, // content
			nil, // uiMeta
			nil, // isDefault
			req.FilePath,
			req.AutoUpdate,
		)
		require.NotNil(t, errx)
	})
}

// TestSystemPromptWithThreadsAndFilePrompt verifies attaching file-based system prompts to threads
func TestSystemPromptWithThreadsAndFilePrompt(t *testing.T) {
	setupTestDB(t)
	ctx := context.Background()

	// Register desktop syscap to allow file operations
	sc := &syscap.SysCap{
		Category: syscap.CategoryModule,
		Name:     syscap.SysCapaModuleDesktop,
		Present:  true,
	}
	syscap.RegisterSysCap(sc)
	defer syscap.UnregisterSysCap(sc)

	// Create a temporary file with content
	tempDir := t.TempDir()
	filePath := filepath.Join(tempDir, "thread_test.txt")
	fileContent := "Content for thread test"
	err := os.WriteFile(filePath, []byte(fileContent), 0644)
	require.NoError(t, err, "Failed to create test file")

	// Create a file-based system prompt
	prompt, errx := CreateSystemPrompt(ctx, "Thread Test Prompt", "", "", false, filePath, true)
	require.Nil(t, errx, "CreateSystemPrompt should not return an error")

	// Create a chat thread
	thread, errx := CreateChatThread(ctx, uuid.Nil, "Thread with File Prompt", nil, nil, nil, []uuid.UUID{})
	require.Nil(t, errx, "CreateChatThread should not return an error")

	// Attach the prompt to the thread
	errx = AttachSystemPromptToThread(ctx, thread.ID, prompt.ID)
	require.Nil(t, errx, "AttachSystemPromptToThread should not return an error")

	// Get system prompts for the thread
	prompts, errx := GetSystemPromptsForThread(ctx, thread.ID)
	require.Nil(t, errx, "GetSystemPromptsForThread should not return an error")
	require.Equal(t, 1, len(prompts), "Thread should have one system prompt")
	assert.Equal(t, prompt.ID, prompts[0].ID)
	assert.Equal(t, "Thread Test Prompt", prompts[0].Name)
	assert.Equal(t, fileContent, prompts[0].Content)
	assert.Equal(t, filePath, prompts[0].FilePath)
	assert.True(t, prompts[0].AutoUpdate)

	// Update the file content
	updatedContent := "Updated content for thread test"
	err = os.WriteFile(filePath, []byte(updatedContent), 0644)
	require.NoError(t, err, "Failed to update test file")

	// Get system prompts for the thread again
	updatedPrompts, errx := GetSystemPromptsForThread(ctx, thread.ID)
	require.Nil(t, errx, "GetSystemPromptsForThread should not return an error")
	require.Equal(t, 1, len(updatedPrompts), "Thread should have one system prompt")

	// Call GetContent to trigger an update if needed
	content := updatedPrompts[0].GetContent()
	assert.Equal(t, updatedContent, content, "Content should be updated from file")
}
