//go:build linux

package utils

import (
	"os"
	"time"
)

// getFileCreationTime returns the file creation time on Linux systems
// Note: Linux doesn't reliably support file creation time (birth time) in all filesystems,
// so we fall back to modification time
func GetFileCreationTime(fileInfo os.FileInfo) time.Time {
	// On Linux, file creation time is not consistently available across all filesystems
	// Fall back to modification time
	return fileInfo.ModTime()
}
