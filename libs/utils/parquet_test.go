package utils

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"

	// These packages are used for writing a valid parquet file for testing.
	"github.com/xitongsys/parquet-go-source/local"
	"github.com/xitongsys/parquet-go/writer"
)

// createTempFile creates a temporary file with the given extension,
// writes the provided header and extra content, and returns the file name.
func createTempFile(t *testing.T, ext string, header []byte, content []byte) string {
	tmpfile, err := os.CreateTemp("", "testfile_*"+ext)
	assert.NoError(t, err)
	// Write header bytes first.
	_, err = tmpfile.Write(header)
	assert.NoError(t, err)
	// Write additional content if provided.
	if content != nil {
		_, err = tmpfile.Write(content)
		assert.NoError(t, err)
	}
	err = tmpfile.Close()
	assert.NoError(t, err)
	return tmpfile.Name()
}

//
// Tests for isParquetFile
//

func TestIsParquetFile_WrongExtension(t *testing.T) {
	// Create a temporary file with a wrong extension.
	filename := createTempFile(t, ".txt", []byte("PAR1"), nil)
	defer os.Remove(filename)

	res := isParquetFile(filename)
	assert.False(t, res, "File with wrong extension should return false")
}

func TestIsParquetFile_InvalidHeader(t *testing.T) {
	// Create a file with a .parquet extension but an invalid header.
	filename := createTempFile(t, ".parquet", []byte("WRNG"), nil)
	defer os.Remove(filename)

	res := isParquetFile(filename)
	assert.False(t, res, "File with invalid header should return false")
}

func TestIsParquetFile_Valid(t *testing.T) {
	// Create a file with proper .parquet extension and header "PAR1".
	filename := createTempFile(t, ".parquet", []byte("PAR1"), []byte("some extra content"))
	defer os.Remove(filename)

	res := isParquetFile(filename)
	assert.True(t, res, "File with correct extension and header should return true")
}

//
// Tests for UnmarshalParquetFile
//

// Dummy type used for testing UnmarshalParquetFile.
type ParquetDummy struct {
	Field string `parquet:"name=field, type=BYTE_ARRAY, convertedtype=UTF8, encoding=PLAIN_DICTIONARY"`
}

func TestUnmarshalParquetFile_NotParquetExtension(t *testing.T) {
	// Create a file with a wrong extension.
	filename := createTempFile(t, ".txt", []byte("PAR1"), nil)
	defer os.Remove(filename)

	var dummy []ParquetDummy
	err := UnmarshalParquetFile(filename, &dummy)
	assert.Error(t, err, "Expected an error if file does not have .parquet extension")
}

func TestUnmarshalParquetFile_NonExistent(t *testing.T) {
	// Provide a filename that does not exist.
	filename := filepath.Join(os.TempDir(), "nonexistent.parquet")
	var dummy []ParquetDummy
	err := UnmarshalParquetFile(filename, &dummy)
	assert.Error(t, err, "Expected an error when attempting to unmarshal non-existent file")
}

func TestUnmarshalParquetFile_InvalidParquet(t *testing.T) {
	// Create a .parquet file that passes the magic number test but contains invalid parquet content.
	filename := createTempFile(t, ".parquet", []byte("PAR1"), []byte("invalid parquet content"))
	defer os.Remove(filename)

	var dummy []ParquetDummy
	err := UnmarshalParquetFile(filename, &dummy)
	assert.Error(t, err, "Expected an error when unmarshalling an invalid parquet file")
}

func TestUnmarshalParquetFile_Valid(t *testing.T) {
	// This test generates a valid parquet file using the parquet writer and then uses
	// UnmarshalParquetFile to read it back.

	// Create a temporary file for parquet output.
	tmpFile, err := os.CreateTemp("", "test_valid_*.parquet")
	assert.NoError(t, err)
	filename := tmpFile.Name()
	tmpFile.Close()
	defer os.Remove(filename)

	// Create a parquet file writer.
	fw, err := local.NewLocalFileWriter(filename)
	assert.NoError(t, err)

	// Create a parquet writer with one parallel writer.
	pw, err := writer.NewParquetWriter(fw, new(ParquetDummy), 1)
	if err != nil {
		// If a parquet writer can not be created, skip this test.
		t.Skipf("Skipping valid parquet test: failed to create writer: %v", err)
	}

	// Write one record.
	dummyRecord := ParquetDummy{Field: "testvalue"}
	err = pw.Write(dummyRecord)
	assert.NoError(t, err)
	err = pw.WriteStop()
	assert.NoError(t, err)
	fw.Close()

	// Unmarshal the parquet file into a slice.
	var result []ParquetDummy
	err = UnmarshalParquetFile(filename, &result)
	assert.NoError(t, err, "Expected valid parsing of parcel file")
	assert.Equal(t, 1, len(result), "Expected one row in the parquet file")
	assert.Equal(t, "testvalue", result[0].Field, "Record field value does not match expected")
}
