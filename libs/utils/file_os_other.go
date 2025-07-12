//go:build !darwin && !linux && !windows

package utils

import (
	"os"
	"time"
)

// getFileCreationTime returns the file creation time for other platforms
// Falls back to modification time as a safe default
func GetFileCreationTime(fileInfo os.FileInfo) time.Time {
	// For platforms other than Darwin and Linux, fall back to modification time
	// This provides a consistent behavior across platforms
	return fileInfo.ModTime()
}
