package utils

import (
	"crypto/sha256"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestPrepareFilePath_InvalidPaths(t *testing.T) {
	// Test that absolute paths are rejected.
	var err error
	if runtime.GOOS == "windows" {
		err = PrepareFilePath("C:\\tmp\\test.txt")
	} else {
		err = PrepareFilePath("/tmp/test.txt")
	}
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "absolute paths are not allowed")

	// Test that paths which aren't clean (potential traversal attack) are rejected.
	invalidPath := "folder/../file.txt"
	err = PrepareFilePath(invalidPath)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "invalid path")

	// Test that paths with invalid characters are rejected.
	err = PrepareFilePath("test<>.txt")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "invalid characters")
}

func TestPrepareFilePath_ValidPath(t *testing.T) {
	// Use a temporary directory as the working directory to avoid side effects.
	tempDir := t.TempDir()
	origWD, err := os.Getwd()
	assert.NoError(t, err)
	defer os.Chdir(origWD)
	err = os.Chdir(tempDir)
	assert.NoError(t, err)

	// Using a valid relative path should succeed; it will create the required directory.
	validPath := filepath.Join("subdir", "test.txt")
	err = PrepareFilePath(validPath)
	assert.NoError(t, err)

	// Now "subdir" should exist.
	_, err = os.Stat("subdir")
	assert.NoError(t, err)
}

func TestGetFileSize(t *testing.T) {
	// Create a temporary file with known content.
	content := "Hello, world!"
	tmpFile, err := os.CreateTemp("", "filesize_test_*.txt")
	assert.NoError(t, err)
	defer os.Remove(tmpFile.Name())

	_, err = tmpFile.WriteString(content)
	assert.NoError(t, err)
	tmpFile.Close()

	size, err := GetFileSize(tmpFile.Name())
	assert.NoError(t, err)
	assert.Equal(t, int64(len(content)), size)
}

func TestGetFileChecksum(t *testing.T) {
	// Create a temporary file with known content.
	content := "ChecksumTestContent"
	tmpFile, err := os.CreateTemp("", "checksum_test_*.txt")
	assert.NoError(t, err)
	defer os.Remove(tmpFile.Name())

	_, err = tmpFile.WriteString(content)
	assert.NoError(t, err)
	tmpFile.Close()

	checksum, err := GetFileChecksum(tmpFile.Name())
	assert.NoError(t, err)

	// Compute expected checksum using sha256 directly.
	expected := fmt.Sprintf("%x", sha256.Sum256([]byte(content)))
	assert.Equal(t, expected, checksum)
}

func TestFileExists(t *testing.T) {
	// Create a temporary file.
	tmpFile, err := os.CreateTemp("", "exists_test_*.txt")
	assert.NoError(t, err)
	filename := tmpFile.Name()
	tmpFile.Close()
	// Ensure file exists.
	exists := FileExists(filename)
	assert.True(t, exists)

	// Remove the file and check that FileExists returns false.
	err = os.Remove(filename)
	assert.NoError(t, err)
	exists = FileExists(filename)
	assert.False(t, exists)
}

func TestIsPipe(t *testing.T) {
	t.Run("pipe", func(t *testing.T) {
		r, w, err := os.Pipe()
		assert.NoError(t, err)
		defer r.Close()
		defer w.Close()

		// For a pipe, IsPipe should return true.
		assert.True(t, IsPipe(r), "Expected the read end of a pipe to be detected as a pipe")
		assert.True(t, IsPipe(w), "Expected the write end of a pipe to be detected as a pipe")
	})

	t.Run("terminal", func(t *testing.T) {
		// Attempt to open /dev/tty to simulate a terminal.
		tty, err := os.Open("/dev/tty")
		if err != nil {
			t.Skip("Skipping terminal test; /dev/tty not available")
		}
		defer tty.Close()

		// For a terminal, IsPipe should return false.
		assert.False(t, IsPipe(tty), "Expected /dev/tty (a terminal) not to be detected as a pipe")
	})

	t.Run("closed file", func(t *testing.T) {
		// Create a temporary file and close it to simulate a file with an invalid descriptor.
		tmpFile, err := os.CreateTemp("", "ispipe_test_*.txt")
		assert.NoError(t, err)
		filename := tmpFile.Name()
		tmpFile.Close()
		defer os.Remove(filename)

		// When Stat fails on a closed file, IsPipe should return false.
		assert.False(t, IsPipe(tmpFile), "Expected a closed file not to be detected as a pipe")
	})
}
