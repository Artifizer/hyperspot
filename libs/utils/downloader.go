package utils

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"time"

	"github.com/hashicorp/go-getter"
	"github.com/hypernetix/hyperspot/libs/logging"
)

type ProgressTracker struct {
	Total                 int64
	Current               int64
	LastLog               int
	URL                   string
	Dest                  string
	Callback              func(progress float32)
	LastLogTime           time.Time
	ProgressThresholdMsec int64

	stream io.ReadCloser
}

// ProgressTracker has to implement ReadCloser interface
func (pt *ProgressTracker) Close() error {
	return pt.stream.Close()
}

// ProgressTracker has to implement ReadCloser interface
func (pt *ProgressTracker) Read(p []byte) (int, error) {
	n, err := pt.stream.Read(p)
	pt.Current += int64(n)

	now := time.Now()
	elapsed := now.Sub(pt.LastLogTime)
	if pt.Total > 0 && elapsed > time.Duration(pt.ProgressThresholdMsec)*time.Millisecond {
		progress := float32(pt.Current) / float32(pt.Total) * 100
		logging.Debug("Downloading %s: %.1f%% complete", filepath.Base(pt.Dest), progress)
		pt.Callback(progress)
		pt.LastLogTime = now
	}

	return n, err
}

func (pt *ProgressTracker) TrackProgress(src string, currentSize, totalSize int64, stream io.ReadCloser) io.ReadCloser {
	pt.Total = totalSize
	pt.Current = currentSize
	pt.LastLogTime = time.Now()
	pt.stream = stream

	logging.Debug("TOTAL SIZE: " + strconv.FormatInt(pt.Total, 10))
	logging.Debug("CURRENT SIZE: " + strconv.FormatInt(pt.Current, 10))
	logging.Debug("Downloading %s: %.2f%% complete", filepath.Base(pt.Dest), float64(pt.Current)/float64(pt.Total)*100)
	return pt
}

func DownloadWithProgress(sourceURL, destination string, callback func(progress float32), resume bool) error {
	// Create destination directory if it doesn't exist
	destDir := filepath.Dir(destination)
	if err := os.MkdirAll(destDir, 0755); err != nil {
		return fmt.Errorf("failed to create destination directory: %w", err)
	}

	// Initialize progress tracker
	pt := &ProgressTracker{
		URL:                   sourceURL,
		Dest:                  destination,
		Callback:              callback,
		LastLogTime:           time.Now(),
		ProgressThresholdMsec: 2000, // update progress every 2 seconds
	}

	// Configure go-getter client
	client := &getter.Client{
		Src:              sourceURL,
		Dst:              destination,
		Pwd:              destDir,
		Mode:             getter.ClientModeFile,
		ProgressListener: pt,
		Getters: map[string]getter.Getter{
			"http":  &getter.HttpGetter{},
			"https": &getter.HttpGetter{},
		},
	}

	// Check for existing file to resume download if enabled
	if resume {
		if fi, err := os.Stat(destination); err == nil {
			pt.Current = fi.Size()
			client.Mode = getter.ClientModeAny
			logging.Info(fmt.Sprintf("Resuming download from %d bytes", pt.Current))
		}
	}
	// Always use the progress tracker option
	//	client.Options = []getter.ClientOption{
	//		getter.WithProgress(pt),
	//	}

	if err := client.Get(); err != nil {
		return fmt.Errorf("download failed: %w", err)
	}

	logging.Info(fmt.Sprintf("Download complete: %s", destination))
	return nil
}
