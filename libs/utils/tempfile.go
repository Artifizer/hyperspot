package utils

import (
	"fmt"
	"io"
	"mime/multipart"
	"os"
	"path/filepath"

	"github.com/hypernetix/hyperspot/libs/logging"
)

// SaveUploadedTempFile saves the uploaded file to a temporary location and returns the path
// If tempdir is provided, it uses that directory; otherwise uses the system default temp directory
func SaveUploadedTempFile(file multipart.File, tempdir string, filename string) (string, error) {
	// Create a temporary file with the original extension
	ext := filepath.Ext(filename)
	// Use the correct pattern for os.CreateTemp - put * where random chars should go
	tempFilePattern := "hyperspot_upload_*" + ext

	var tempFile *os.File
	var err error

	if tempdir != "" {
		// Use the provided temp directory
		tempFile, err = os.CreateTemp(tempdir, tempFilePattern)
	} else {
		// Use the system default temp directory
		tempFile, err = os.CreateTemp("", tempFilePattern)
	}

	if err != nil {
		return "", fmt.Errorf("failed to create temporary file: %v", err)
	}
	defer tempFile.Close()

	// Copy the uploaded file content to the temporary file
	size, err := io.Copy(tempFile, file)
	if err != nil {
		if removeErr := os.Remove(tempFile.Name()); removeErr != nil {
			logging.Warn("Failed to remove temp file %s: %v", tempFile.Name(), removeErr)
		}
		return "", fmt.Errorf("failed to copy file content: %v", err)
	}

	logging.Trace("Saved uploaded file '%s' to temporary location '%s' (%d bytes)",
		filename, tempFile.Name(), size)

	return tempFile.Name(), nil
}
