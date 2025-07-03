package job

import (
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/hypernetix/hyperspot/libs/config"
)

func TestJobConfig_WithConfig(t *testing.T) {
	// Save original config to restore after test
	mu := &sync.Mutex{}
	origConfig := config.Get()
	defer func() {
		mu.Lock()
		config.Reset()
		if origConfig != nil {
			config.GetDefault()
		}
		mu.Unlock()
	}()

	// Create test config
	config.GetDefault()

	// Test that our job functionality works with the config
	// This is mostly a sanity check to ensure job code can use config properly
	timeout := config.GetServerTimeout()
	assert.GreaterOrEqual(t, int64(timeout.Seconds()), int64(60), "Default timeout should be at least 60 seconds")
}
