package llm

import (
	"context"

	"github.com/hypernetix/hyperspot/libs/utils"
	"github.com/hypernetix/hyperspot/modules/syscap"
)

var llmServicesSyscapMu utils.DebugMutex
var llmServicesInstances = make(map[string][]LLMService)
var llmServicesSyscaps = make(map[string]*syscap.SysCap)

func registerLLMServiceSysCap(llmService LLMService, name string, displayName string) *syscap.SysCap {
	llmServicesSyscapMu.Lock()
	defer llmServicesSyscapMu.Unlock()

	// Add instance to the list for this service name
	llmServicesInstances[name] = append(llmServicesInstances[name], llmService)

	// If a capability already exists, return it
	if cap, exists := llmServicesSyscaps[name]; exists {
		return cap
	}

	// Create a new capability if it's the first time we see this service name
	newCap := syscap.NewSysCap(
		syscap.CategoryLLMService,
		syscap.SysCapName(name),
		displayName,
		false, // Initially not present
		5000,  // Cache TTL in milliseconds (5 seconds)
	)

	// Assign the detector function
	newCap.Detector = func(c *syscap.SysCap) error {
		llmServicesSyscapMu.Lock()
		instances := llmServicesInstances[name]
		llmServicesSyscapMu.Unlock()

		c.Present = false
		for _, instance := range instances {
			if err := instance.Ping(context.Background()); err == nil {
				c.Present = true
				break // One is enough to consider the service present
			}
		}
		return nil
	}

	// Store and register the new capability
	llmServicesSyscaps[name] = newCap
	syscap.RegisterSysCap(newCap)

	return newCap
}

// resetLLMServiceSysCap resets the global state for syscap tests.
func resetLLMServiceSysCap() {
	llmServicesSyscapMu.Lock()
	defer llmServicesSyscapMu.Unlock()
	llmServicesInstances = make(map[string][]LLMService)
	llmServicesSyscaps = make(map[string]*syscap.SysCap)
}
