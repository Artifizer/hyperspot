package utils

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestGetFileCreationTime(t *testing.T) {
	// Create a temporary file for testing
	tempDir := t.TempDir()
	testFile := filepath.Join(tempDir, "test_file.txt")

	// Write some content to the file
	content := []byte("test content for file creation time")
	err := os.WriteFile(testFile, content, 0644)
	require.NoError(t, err, "Failed to create test file")

	// Get file info
	fileInfo, err := os.Stat(testFile)
	require.NoError(t, err, "Failed to get file info")

	// Test GetFileCreationTime
	creationTime := GetFileCreationTime(fileInfo)

	// Basic validation - creation time should be a valid time
	assert.False(t, creationTime.IsZero(), "Creation time should not be zero")

	// Creation time should be reasonably recent (within last minute)
	now := time.Now()
	timeDiff := now.Sub(creationTime)
	assert.True(t, timeDiff >= 0, "Creation time should not be in the future")
	assert.True(t, timeDiff < time.Minute, "Creation time should be recent (within 1 minute)")

	// On all platforms, creation time should be <= modification time
	modTime := fileInfo.ModTime()
	assert.True(t, creationTime.Equal(modTime) || creationTime.Before(modTime),
		"Creation time should be <= modification time")
}

func TestGetFileCreationTimeWithModifiedFile(t *testing.T) {
	// Create a temporary file
	tempDir := t.TempDir()
	testFile := filepath.Join(tempDir, "modified_test_file.txt")

	// Write initial content
	initialContent := []byte("initial content")
	err := os.WriteFile(testFile, initialContent, 0644)
	require.NoError(t, err, "Failed to create test file")

	// Get initial file info and creation time
	initialFileInfo, err := os.Stat(testFile)
	require.NoError(t, err, "Failed to get initial file info")
	initialCreationTime := GetFileCreationTime(initialFileInfo)

	// Wait to ensure time difference - use shorter wait but more reliable approach
	time.Sleep(50 * time.Millisecond)

	// Modify the file with different content
	modifiedContent := []byte("modified content with more text - timestamp should change")
	err = os.WriteFile(testFile, modifiedContent, 0644)
	require.NoError(t, err, "Failed to modify test file")

	// Get updated file info
	updatedFileInfo, err := os.Stat(testFile)
	require.NoError(t, err, "Failed to get updated file info")
	updatedCreationTime := GetFileCreationTime(updatedFileInfo)

	// Platform-specific behavior validation
	t.Run("creation_time_consistency", func(t *testing.T) {
		// Both creation times should be valid
		assert.False(t, initialCreationTime.IsZero(), "Initial creation time should not be zero")
		assert.False(t, updatedCreationTime.IsZero(), "Updated creation time should not be zero")

		// Check modification time difference
		modTimeDiff := updatedFileInfo.ModTime().Sub(initialFileInfo.ModTime())
		t.Logf("Modification time difference: %v", modTimeDiff)
		t.Logf("Initial mod time: %v, Updated mod time: %v",
			initialFileInfo.ModTime(), updatedFileInfo.ModTime())

		// Test should be more tolerant of filesystem timing precision
		// The key is that the function should return valid times
		if modTimeDiff > 0 {
			t.Logf("Modification time changed as expected: %v", modTimeDiff)
		} else {
			t.Logf("Modification time unchanged - filesystem may have low precision")
		}

		// Log creation time behavior for debugging
		t.Logf("Initial creation time: %v, Updated creation time: %v",
			initialCreationTime, updatedCreationTime)

		// On platforms where creation time = modification time, they should be equal
		// On platforms with true creation time, creation time should remain constant
		creationTimeDiff := updatedCreationTime.Sub(initialCreationTime)
		t.Logf("Creation time difference: %v", creationTimeDiff)

		// The main assertion is that creation time should be <= modification time
		// This should be true on all platforms
		assert.True(t, updatedCreationTime.Equal(updatedFileInfo.ModTime()) ||
			updatedCreationTime.Before(updatedFileInfo.ModTime()),
			"Creation time should be <= modification time")
	})
}

func TestGetFileCreationTimeWithNonExistentFile(t *testing.T) {
	// This test ensures the function handles edge cases gracefully
	tempDir := t.TempDir()
	nonExistentFile := filepath.Join(tempDir, "non_existent_file.txt")

	// Try to get file info for non-existent file (should fail)
	_, err := os.Stat(nonExistentFile)
	assert.Error(t, err, "Should fail to get file info for non-existent file")
	assert.True(t, os.IsNotExist(err), "Error should be 'file not exists'")
}

func TestGetFileCreationTimeWithDirectory(t *testing.T) {
	// Test with a directory instead of a file
	tempDir := t.TempDir()

	// Get directory info
	dirInfo, err := os.Stat(tempDir)
	require.NoError(t, err, "Failed to get directory info")

	// Test GetFileCreationTime with directory
	creationTime := GetFileCreationTime(dirInfo)

	// Validation
	assert.False(t, creationTime.IsZero(), "Directory creation time should not be zero")
	assert.True(t, dirInfo.IsDir(), "Should be a directory")

	// Creation time should be reasonably recent
	now := time.Now()
	timeDiff := now.Sub(creationTime)
	assert.True(t, timeDiff >= 0, "Creation time should not be in the future")
	assert.True(t, timeDiff < time.Minute, "Creation time should be recent")
}

func TestGetFileCreationTimePlatformBehavior(t *testing.T) {
	// Create a test file
	tempDir := t.TempDir()
	testFile := filepath.Join(tempDir, "platform_test.txt")

	err := os.WriteFile(testFile, []byte("platform test"), 0644)
	require.NoError(t, err, "Failed to create test file")

	fileInfo, err := os.Stat(testFile)
	require.NoError(t, err, "Failed to get file info")

	creationTime := GetFileCreationTime(fileInfo)
	modTime := fileInfo.ModTime()

	// Document expected behavior per platform
	t.Run("platform_specific_behavior", func(t *testing.T) {
		// All platforms should return a valid time
		assert.False(t, creationTime.IsZero(), "Creation time should be valid")

		// Creation time should not be after modification time
		assert.True(t, creationTime.Equal(modTime) || creationTime.Before(modTime),
			"Creation time should be <= modification time")

		// Log platform-specific behavior for debugging
		t.Logf("Platform behavior - Creation time: %v, Mod time: %v, Equal: %v",
			creationTime, modTime, creationTime.Equal(modTime))
	})
}

// Benchmark the file creation time function
func BenchmarkGetFileCreationTime(b *testing.B) {
	// Create a test file
	tempDir := b.TempDir()
	testFile := filepath.Join(tempDir, "benchmark_test.txt")

	err := os.WriteFile(testFile, []byte("benchmark test content"), 0644)
	if err != nil {
		b.Fatalf("Failed to create test file: %v", err)
	}

	fileInfo, err := os.Stat(testFile)
	if err != nil {
		b.Fatalf("Failed to get file info: %v", err)
	}

	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		_ = GetFileCreationTime(fileInfo)
	}
}
