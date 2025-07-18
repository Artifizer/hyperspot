package chat

import (
	"sync"
	"testing"

	"github.com/hypernetix/hyperspot/libs/config"
)

func TestChatConfig_WithConfig(t *testing.T) {
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
}
