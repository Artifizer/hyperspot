//go:build darwin

package utils

import (
	"os"
	"syscall"
	"time"
)

// getFileCreationTime returns the file creation time on Darwin/macOS systems
func GetFileCreationTime(fileInfo os.FileInfo) time.Time {
	if stat, ok := fileInfo.Sys().(*syscall.Stat_t); ok {
		// Try to get birth time (creation time on macOS)
		if stat.Birthtimespec.Sec != 0 {
			return time.Unix(stat.Birthtimespec.Sec, stat.Birthtimespec.Nsec)
		}
	}
	// Fall back to modification time if birth time is not available
	return fileInfo.ModTime()
}
