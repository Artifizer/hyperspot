package utils

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	"github.com/hypernetix/hyperspot/libs/logging"
	"github.com/xitongsys/parquet-go-source/local"
	"github.com/xitongsys/parquet-go/reader"
)

func isParquetFile(filePath string) bool {
	if filepath.Ext(filePath) != ".parquet" {
		return false
	}

	// Open the file and read the first 4 bytes to check for PAR1 magic number
	file, err := os.Open(filePath)
	if err != nil {
		return false
	}
	defer file.Close()

	magicNumber := make([]byte, 4)
	_, err = file.Read(magicNumber)
	if err != nil {
		return false
	}

	// Check if file starts with PAR1 magic number
	return string(magicNumber) == "PAR1"
}

func UnmarshalParquetFile(filePath string, v interface{}) error {
	if !isParquetFile(filePath) {
		return fmt.Errorf("file is not a parquet file: %s", filePath)
	}

	fr, err := local.NewLocalFileReader(filePath)
	if err != nil {
		return fmt.Errorf("failed to open parquet file: %w", err)
	}
	defer fr.Close()

	pr, err := reader.NewParquetReader(fr, nil, 4)
	if err != nil {
		return fmt.Errorf("failed to create parquet reader: %w", err)
	}
	defer pr.ReadStop()

	num := int(pr.GetNumRows())
	res, err := pr.ReadByNumber(num)
	if err != nil {
		return fmt.Errorf("failed to read parquet file: %w", err)
	}

	jsonBs, err := json.MarshalIndent(res, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal to json: %w", err)
	}

	logging.Trace(fmt.Sprintf("unmarshalled '%s' parquet file with %d rows to: %s", filePath, len(res), string(jsonBs)))

	err = json.Unmarshal(jsonBs, v)
	if err != nil {
		return fmt.Errorf("failed to unmarshal from json: %w", err)
	}

	return nil
}
