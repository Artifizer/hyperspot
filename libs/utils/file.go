package utils

import (
	"crypto/sha256"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
)

func validateFilePath(filename string) error {
	if filepath.IsAbs(filename) {
		return fmt.Errorf("absolute paths are not allowed: %s", filename)
	}
	if filepath.Clean(filename) != filename {
		return fmt.Errorf("invalid path (potential traversal attack): %s", filename)
	}
	if matched, _ := regexp.MatchString(`^[a-zA-Z0-9\-_\\\/.~]+$`, filename); !matched {
		return fmt.Errorf("the path contains invalid characters: %s", filename)
	}

	return nil
}

func PrepareFilePath(filename string) error {
	// Normalize path separators for the current OS
	filename = filepath.FromSlash(filename)

	if err := validateFilePath(filename); err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(filename), 0755); err != nil {
		return err
	}
	return nil
}

func GetFileSize(filename string) (int64, error) {
	fi, err := os.Stat(filename)
	if err != nil {
		return 0, err
	}
	return fi.Size(), nil
}

func GetFileChecksum(filename string) (string, error) {
	file, err := os.Open(filename)
	if err != nil {
		return "", err
	}
	defer file.Close()

	hasher := sha256.New()
	if _, err := io.Copy(hasher, file); err != nil {
		return "", err
	}

	checksum := fmt.Sprintf("%x", hasher.Sum(nil))
	return checksum, nil
}

func FileExists(path string) bool {
	f, err := os.Open(path)
	if err != nil {
		return false
	}
	f.Close()
	return true
}

func IsPipe(file *os.File) bool {
	fi, err := file.Stat()
	if err != nil {
		return false
	}
	return (fi.Mode() & os.ModeCharDevice) == 0
}
