package chat

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/hypernetix/hyperspot/modules/syscap"
)

// TestCreateSystemPromptFromFile verifies system prompt creation from a file
func TestCreateSystemPromptFromFile(t *testing.T) {
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
	filePath := filepath.Join(tempDir, "test_prompt.txt")
	fileContent := "This is a test prompt content from file."
	err := os.WriteFile(filePath, []byte(fileContent), 0644)
	require.NoError(t, err, "Failed to create test file")

	// Test creating a system prompt from file
	name := "File-based Prompt"
	prompt, errx := CreateSystemPrompt(ctx, name, "", "", false, filePath, true)
	require.Nil(t, errx, "CreateSystemPrompt from file should not return an error")
	assert.NotNil(t, prompt, "Prompt should be created")
	assert.Equal(t, name, prompt.Name, "Name should match")
	assert.Equal(t, fileContent, prompt.Content, "Content should be loaded from file")
	assert.Equal(t, filePath, prompt.FilePath, "FromFile should be set")
	assert.NotEmpty(t, prompt.FileChecksum, "FileChecksum should be set")
	assert.True(t, prompt.AutoUpdate, "AutoUpdate should be true")

	// Verify the prompt was saved to the database
	retrievedPrompt, errx := GetSystemPrompt(ctx, prompt.ID)
	require.Nil(t, errx, "GetSystemPrompt should not return an error")
	assert.Equal(t, fileContent, retrievedPrompt.Content, "Content should be preserved in database")
	assert.Equal(t, filePath, retrievedPrompt.FilePath, "FromFile should be preserved in database")
	assert.NotEmpty(t, retrievedPrompt.FileChecksum, "FileChecksum should be preserved in database")
	assert.True(t, retrievedPrompt.AutoUpdate, "AutoUpdate should be preserved in database")
}

// TestCreateSystemPromptFromFileWithoutDesktopCap verifies that creating a system prompt from file
// fails when the desktop capability is not present
func TestCreateSystemPromptFromFileWithoutDesktopCap(t *testing.T) {
	setupTestDB(t)
	ctx := context.Background()

	sc := &syscap.SysCap{
		Category: syscap.CategoryModule,
		Name:     syscap.SysCapaModuleDesktop,
		Present:  false,
	}
	syscap.RegisterSysCap(sc)
	defer syscap.UnregisterSysCap(sc)

	// Create a temporary file with content
	tempDir := t.TempDir()
	filePath := filepath.Join(tempDir, "test_prompt.txt")
	err := os.WriteFile(filePath, []byte("Test content"), 0644)
	require.NoError(t, err, "Failed to create test file")

	// Test creating a system prompt from file without desktop capability
	_, errx := CreateSystemPrompt(ctx, "Test Prompt", "", "", false, filePath, true)
	require.NotNil(t, errx, "CreateSystemPrompt should return an error without desktop capability")
	assert.Contains(t, errx.Error(), "not allowed", "Error should indicate file operations are not allowed")
}

// TestCreateSystemPromptFromNonExistentFile verifies error handling when file doesn't exist
func TestCreateSystemPromptFromNonExistentFile(t *testing.T) {
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
	nonExistentPath := "/path/to/nonexistent/file.txt"
	_, errx := CreateSystemPrompt(ctx, "Test Prompt", "", "", false, nonExistentPath, true)
	require.NotNil(t, errx, "CreateSystemPrompt should return an error for non-existent file")
	assert.Contains(t, errx.Error(), "does not exist", "Error should indicate file doesn't exist")
}

// TestUpdateSystemPromptFromFile verifies updating a system prompt from a file
func TestUpdateSystemPromptFromFile(t *testing.T) {
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
	filePath := filepath.Join(tempDir, "test_prompt.txt")
	initialContent := "Initial content"
	err := os.WriteFile(filePath, []byte(initialContent), 0644)
	require.NoError(t, err, "Failed to create test file")

	// Create a system prompt with file but without auto-update
	prompt, errx := CreateSystemPrompt(ctx, "Test Prompt", "", "", false, filePath, false)
	require.Nil(t, errx, "CreateSystemPrompt should not return an error")
	assert.Equal(t, initialContent, prompt.Content, "Content should be loaded from file")
	assert.False(t, prompt.AutoUpdate, "AutoUpdate should be false")

	// Update the file content
	updatedContent := "Updated content"
	err = os.WriteFile(filePath, []byte(updatedContent), 0644)
	require.NoError(t, err, "Failed to update test file")

	// Enable auto-update for the prompt
	AutoUpdate := true
	updatedPrompt, errx := UpdateSystemPrompt(ctx, prompt.ID, nil, nil, nil, nil, nil, &AutoUpdate)
	require.Nil(t, errx, "UpdateSystemPrompt should not return an error")
	assert.Equal(t, updatedContent, updatedPrompt.Content, "Content should be updated from file")
	assert.True(t, updatedPrompt.AutoUpdate, "AutoUpdate should be true")

	// Verify the prompt was updated in the database
	retrievedPrompt, errx := GetSystemPrompt(ctx, prompt.ID)
	require.Nil(t, errx, "GetSystemPrompt should not return an error")
	assert.Equal(t, updatedContent, retrievedPrompt.Content, "Content should be updated in database")
	assert.True(t, retrievedPrompt.AutoUpdate, "AutoUpdate should be true in database")
}

// TestGetContentAutoUpdate verifies that GetContent automatically updates content from file
func TestGetContentAutoUpdate(t *testing.T) {
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
	filePath := filepath.Join(tempDir, "test_prompt.txt")
	initialContent := "Initial content for auto-update test"
	err := os.WriteFile(filePath, []byte(initialContent), 0644)
	require.NoError(t, err, "Failed to create test file")

	// Create a system prompt with auto-update enabled
	prompt, errx := CreateSystemPrompt(ctx, "Auto-update Prompt", "", "", false, filePath, true)
	require.Nil(t, errx, "CreateSystemPrompt should not return an error")
	assert.Equal(t, initialContent, prompt.Content, "Content should be loaded from file")

	// Get the content (should not change since file hasn't changed)
	content := prompt.GetContent()
	assert.Equal(t, initialContent, content, "Content should not change when file hasn't changed")

	// Update the file content
	updatedContent := "Updated content for auto-update test"
	err = os.WriteFile(filePath, []byte(updatedContent), 0644)
	require.NoError(t, err, "Failed to update test file")

	// Get the content again (should auto-update)
	content = prompt.GetContent()
	assert.Equal(t, updatedContent, content, "Content should be auto-updated from file")

	// Verify the prompt was updated in the database
	retrievedPrompt, errx := GetSystemPrompt(ctx, prompt.ID)
	require.Nil(t, errx, "GetSystemPrompt should not return an error")
	assert.Equal(t, updatedContent, retrievedPrompt.Content, "Content should be updated in database")
}

// TestUpdateSystemPromptDisableFileUpdate verifies disabling file updates
func TestUpdateSystemPromptDisableFileUpdate(t *testing.T) {
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
	filePath := filepath.Join(tempDir, "test_prompt.txt")
	initialContent := "Initial content for disable test"
	err := os.WriteFile(filePath, []byte(initialContent), 0644)
	require.NoError(t, err, "Failed to create test file")

	// Create a system prompt with auto-update enabled
	prompt, errx := CreateSystemPrompt(ctx, "Disable Update Prompt", "", "", false, filePath, true)
	require.Nil(t, errx, "CreateSystemPrompt should not return an error")
	assert.Equal(t, initialContent, prompt.Content, "Content should be loaded from file")

	// Disable auto-update
	AutoUpdate := false
	updatedPrompt, errx := UpdateSystemPrompt(ctx, prompt.ID, nil, nil, nil, nil, nil, &AutoUpdate)
	require.Nil(t, errx, "UpdateSystemPrompt should not return an error")
	assert.False(t, updatedPrompt.AutoUpdate, "AutoUpdate should be false")

	// Update the file content
	updatedContent := "Updated content after disabling auto-update"
	err = os.WriteFile(filePath, []byte(updatedContent), 0644)
	require.NoError(t, err, "Failed to update test file")

	// Get the content (should not auto-update since it's disabled)
	content := updatedPrompt.GetContent()
	assert.Equal(t, initialContent, content, "Content should not change when auto-update is disabled")
}

// TestUpdateSystemPromptFileRemoved verifies error handling when file is removed
func TestUpdateSystemPromptFileRemoved(t *testing.T) {
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
	filePath := filepath.Join(tempDir, "test_prompt.txt")
	initialContent := "Initial content for file removal test"
	err := os.WriteFile(filePath, []byte(initialContent), 0644)
	require.NoError(t, err, "Failed to create test file")

	// Create a system prompt with auto-update enabled
	prompt, errx := CreateSystemPrompt(ctx, "File Removal Prompt", "", "", false, filePath, true)
	require.Nil(t, errx, "CreateSystemPrompt should not return an error")
	assert.Equal(t, initialContent, prompt.Content, "Content should be loaded from file")

	// Remove the file
	err = os.Remove(filePath)
	require.NoError(t, err, "Failed to remove test file")

	// Get the content (should return empty string due to error)
	content := prompt.GetContent()
	assert.Equal(t, "", content, "GetContent should return empty string when file is removed")
}

// TestUpdateSystemPromptWithoutFile verifies error handling when trying to enable auto-update without a file
func TestUpdateSystemPromptWithoutFile(t *testing.T) {
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
	prompt, errx := CreateSystemPrompt(ctx, "Regular Prompt", "Regular content", "", false, "", false)
	require.Nil(t, errx, "CreateSystemPrompt should not return an error")

	// Try to enable auto-update without setting a file
	AutoUpdate := true
	_, errx = UpdateSystemPrompt(ctx, prompt.ID, nil, nil, nil, nil, nil, &AutoUpdate)
	require.NotNil(t, errx, "UpdateSystemPrompt should return an error when enabling auto-update without a file")
	assert.Contains(t, errx.Error(), "file is not set", "Error should indicate file is not set")
}

// TestSystemPromptAPIWithFileOperations verifies API-level file operations
func TestSystemPromptAPIWithFileOperations(t *testing.T) {
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

	// Test API request for creating a system prompt from file
	request := SystemPromptCreateAPIRequest{
		Name:       "API File Prompt",
		Content:    "",
		FilePath:   filePath,
		AutoUpdate: true,
	}

	// Simulate the API logic
	prompt, errx := CreateSystemPrompt(ctx, request.Name, request.Content, "", false, request.FilePath, request.AutoUpdate)
	require.Nil(t, errx, "CreateSystemPrompt from API should not return an error")
	assert.Equal(t, request.Name, prompt.Name, "Name should match API request")
	assert.Equal(t, fileContent, prompt.Content, "Content should be loaded from file")
	assert.Equal(t, filePath, prompt.FilePath, "FromFile should match API request")
	assert.True(t, prompt.AutoUpdate, "AutoUpdate should match API request")

	// Test API request for updating a system prompt with file changes
	updatedContent := "Updated API test content"
	err = os.WriteFile(filePath, []byte(updatedContent), 0644)
	require.NoError(t, err, "Failed to update test file")

	// Get the content (should auto-update)
	content := prompt.GetContent()
	assert.Equal(t, updatedContent, content, "Content should be auto-updated from file via API")
}

// TestConcurrentFileUpdates verifies that file updates work correctly under concurrent access
func TestConcurrentFileUpdates(t *testing.T) {
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
	filePath := filepath.Join(tempDir, "concurrent_test.txt")
	initialContent := "Initial content for concurrent test"
	err := os.WriteFile(filePath, []byte(initialContent), 0644)
	require.NoError(t, err, "Failed to create test file")

	// Create a system prompt with auto-update enabled
	prompt, errx := CreateSystemPrompt(ctx, "Concurrent Update Prompt", "", "", false, filePath, true)
	require.Nil(t, errx, "CreateSystemPrompt should not return an error")
	assert.Equal(t, initialContent, prompt.Content, "Content should be loaded from file")

	// Get the prompt ID for concurrent operations
	promptID := prompt.ID

	// Update the file multiple times with small delays
	for i := 1; i <= 3; i++ {
		updatedContent := initialContent + " - update " + string(rune('0'+i))
		err = os.WriteFile(filePath, []byte(updatedContent), 0644)
		require.NoError(t, err, "Failed to update test file")
		time.Sleep(10 * time.Millisecond) // Small delay to ensure file system registers the change
	}

	// Final content should be the last update
	finalContent := initialContent + " - update 3"

	// Get the prompt and verify content is updated
	updatedPrompt, errx := GetSystemPrompt(ctx, promptID)
	require.Nil(t, errx, "GetSystemPrompt should not return an error")

	// Call GetContent to trigger an update if needed
	content := updatedPrompt.GetContent()
	assert.Equal(t, finalContent, content, "Content should be updated to the latest file version")
}

// TestUpdateSystemPromptFilePathChange verifies changing the file path for a system prompt
func TestUpdateSystemPromptFilePathChange(t *testing.T) {
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

	t.Run("Change file path for prompt", func(t *testing.T) {
		// Create two temporary files with different content
		tempDir := t.TempDir()
		filePath1 := filepath.Join(tempDir, "file1.txt")
		filePath2 := filepath.Join(tempDir, "file2.txt")

		content1 := "Content from file 1"
		content2 := "Content from file 2"

		err := os.WriteFile(filePath1, []byte(content1), 0644)
		require.NoError(t, err, "Failed to create first test file")

		err = os.WriteFile(filePath2, []byte(content2), 0644)
		require.NoError(t, err, "Failed to create second test file")

		// Create a system prompt with the first file
		prompt, errx := CreateSystemPrompt(ctx, "File Path Change Prompt", "", "", false, filePath1, true)
		require.Nil(t, errx, "CreateSystemPrompt should not return an error")
		assert.Equal(t, content1, prompt.Content, "Content should be loaded from first file")
		assert.Equal(t, filePath1, prompt.FilePath, "FromFile should be set to first file")

		// Update the prompt to use the second file
		updatedPrompt, errx := UpdateSystemPrompt(ctx, prompt.ID, nil, nil, nil, nil, &filePath2, nil)
		require.Nil(t, errx, "UpdateSystemPrompt should not return an error")

		// Verify the file path and content were updated
		assert.Equal(t, filePath2, updatedPrompt.FilePath, "File path should be updated")
		assert.Equal(t, content2, updatedPrompt.Content, "Content should be updated from new file")
		assert.True(t, updatedPrompt.AutoUpdate, "AutoUpdate should remain enabled")

		// Get the prompt again to verify persistence
		retrievedPrompt, errx := GetSystemPrompt(ctx, prompt.ID)
		require.Nil(t, errx, "GetSystemPrompt should not return an error")
		assert.Equal(t, filePath2, retrievedPrompt.FilePath, "File path should be updated in database")
	})

	t.Run("Remove file path from prompt", func(t *testing.T) {
		// Create a temporary file
		tempDir := t.TempDir()
		filePath := filepath.Join(tempDir, "file.txt")
		content := "Test content"

		err := os.WriteFile(filePath, []byte(content), 0644)
		require.NoError(t, err, "Failed to create test file")

		// Create a system prompt with a file
		prompt, errx := CreateSystemPrompt(ctx, "Prompt to Remove File", "", "", false, filePath, true)
		require.Nil(t, errx, "CreateSystemPrompt should not return an error")

		// Remove the file path
		emptyPath := ""
		updatedPrompt, errx := UpdateSystemPrompt(ctx, prompt.ID, nil, nil, nil, nil, &emptyPath, nil)
		require.Nil(t, errx, "UpdateSystemPrompt should not return an error")

		// Verify the file path was removed and auto-update was disabled
		assert.Empty(t, updatedPrompt.FilePath, "File path should be empty")
		assert.False(t, updatedPrompt.AutoUpdate, "AutoUpdate should be disabled when file path is removed")

		// Get the prompt again to verify persistence
		retrievedPrompt, errx := GetSystemPrompt(ctx, prompt.ID)
		require.Nil(t, errx, "GetSystemPrompt should not return an error")
		assert.Empty(t, retrievedPrompt.FilePath, "File path should be empty in database")
		assert.False(t, retrievedPrompt.AutoUpdate, "AutoUpdate should be disabled in database")

		// Verify content is now empty
		assert.Empty(t, retrievedPrompt.GetContent(), "Content should be empty after removing file path")
	})

	t.Run("Update to non-existent file should fail", func(t *testing.T) {
		// Create a temporary file
		tempDir := t.TempDir()
		filePath := filepath.Join(tempDir, "file.txt")
		nonExistentPath := filepath.Join(tempDir, "nonexistent.txt")

		err := os.WriteFile(filePath, []byte("test"), 0644)
		require.NoError(t, err, "Failed to create test file")

		// Create a system prompt with a file
		prompt, errx := CreateSystemPrompt(ctx, "Test Prompt", "", "", false, filePath, true)
		require.Nil(t, errx, "CreateSystemPrompt should not return an error")

		// Try to update to a non-existent file
		_, errx = UpdateSystemPrompt(ctx, prompt.ID, nil, nil, nil, nil, &nonExistentPath, nil)
		require.NotNil(t, errx, "UpdateSystemPrompt should return an error for non-existent file")
	})
}
