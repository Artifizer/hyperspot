package syscap

import (
	"fmt"
	"sort"
	"sync"
	"time"

	"github.com/hypernetix/hyperspot/libs/core"
	"github.com/hypernetix/hyperspot/libs/errorx"
	"github.com/hypernetix/hyperspot/libs/logging"
)

// SysCapCategory represents the category of a capability
type SysCapCategory string

const (
	CategoryHardware   SysCapCategory = "hardware"
	CategorySoftware   SysCapCategory = "software"
	CategoryOS         SysCapCategory = "os"
	CategoryLLMService SysCapCategory = "llm_service"
	CategoryModule     SysCapCategory = "module"
)

// Predefined syscap names
const (
	SysCapaModuleDesktop SysCapName = "desktop"
	SysCapaModuleServer  SysCapName = "server"
)

func isValidSysCapCategory(category SysCapCategory) errorx.Error {
	if category == CategoryHardware || category == CategorySoftware || category == CategoryOS || category == CategoryLLMService || category == CategoryModule {
		return nil
	}
	return errorx.NewErrBadRequest("Invalid category. Must be one of: %s, %s, %s, %s, %s", CategoryHardware, CategorySoftware, CategoryOS, CategoryLLMService, CategoryModule)
}

// SysCapName represents the machine-friendly name of a capability (lowercase, underscores)
type SysCapName string

// SysCapDetector is a function that detects a specific capability
type SysCapDetector func(*SysCap) error

type SysCap struct {
	Key             SysCapKey              `json:"key"`
	Category        SysCapCategory         `json:"category" `
	Name            SysCapName             `json:"name"`
	DisplayName     string                 `json:"display_name"`
	Present         bool                   `json:"present"`
	Version         *string                `json:"version,omitempty"`
	Amount          *float64               `json:"amount,omitempty"`
	AmountDimension *SysCapAmountDimension `json:"amount_dimension,omitempty"`
	Details         *string                `json:"details,omitempty"`
	Detector        SysCapDetector         `json:"-"`
	CacheTTLMsec    uint                   `json:"cache_ttl_msec" doc:"cache timeout in milliseconds"`
	CachedAtMs      int64                  `json:"cached_at_ms" doc:"unix timestamp in milliseconds"`
}

// SysCapAmountDimension represents the unit/dimension of a capability amount
type SysCapAmountDimension string

const (
	DimensionCount     SysCapAmountDimension = "count"
	DimensionGB        SysCapAmountDimension = "GB"
	DimensionMB        SysCapAmountDimension = "MB"
	DimensionPercent   SysCapAmountDimension = "percent"
	DimensionBytes     SysCapAmountDimension = "bytes"
	DimensionCores     SysCapAmountDimension = "cores"
	DimensionMHz       SysCapAmountDimension = "MHz"
	DimensionInstances SysCapAmountDimension = "instances"
)

// SysCapKey represents a unique key for a capability in format "category:name"
type SysCapKey string

// NewSysCapKey creates a capability key from category and name
func NewSysCapKey(category SysCapCategory, name SysCapName) SysCapKey {
	return SysCapKey(fmt.Sprintf("%s:%s", string(category), string(name)))
}

// ParseSysCapKey parses a capability key into category and name
func ParseSysCapKey(key SysCapKey) (SysCapCategory, SysCapName, error) {
	keyStr := string(key)
	for i, char := range keyStr {
		if char == ':' {
			if i == 0 || i == len(keyStr)-1 {
				return "", "", fmt.Errorf("invalid capability key format: %s", keyStr)
			}
			category := SysCapCategory(keyStr[:i])
			name := SysCapName(keyStr[i+1:])
			return category, name, nil
		}
	}
	return "", "", fmt.Errorf("invalid capability key format, missing ':': %s", keyStr)
}

// sysCapRegistry holds all registered capability detectors
var sysCapRegistry = make(map[SysCapKey]*SysCap)
var sysCapMutex = sync.RWMutex{}

// capabilityCache holds cached capabilities with expiration
var cacheMutex = sync.RWMutex{}

// RegisterSysCap registers a capability detector.
func RegisterSysCap(capability *SysCap) {
	capability.Key = NewSysCapKey(capability.Category, capability.Name)

	sysCapMutex.Lock()
	defer sysCapMutex.Unlock()

	sysCapRegistry[capability.Key] = capability
	logging.Debug("Registered capability: %s (display: %s)", string(capability.Key), capability.DisplayName)
}

func (c *SysCap) IsCacheValid() bool {
	return time.Now().UnixMilli()-c.CachedAtMs < int64(c.CacheTTLMsec)
}

func (c *SysCap) IsPresent() bool {
	if !c.IsCacheValid() && c.Detector != nil {
		if c.Detector(c) != nil {
			return false
		}
		c.CachedAtMs = time.Now().UnixMilli()
	}

	return c.Present
}

// GetSysCap gets a specific capability by key, using cache if available
func GetSysCap(key SysCapKey) (*SysCap, error) {
	// Get detector
	sysCapMutex.RLock()
	c, exists := sysCapRegistry[key]
	sysCapMutex.RUnlock()

	if !exists {
		return nil, &SysCapNotFoundError{Key: string(key)}
	}

	if c.IsCacheValid() {
		return c, nil
	}

	if c.Detector == nil {
		return c, nil
	}

	// Detect capability
	err := c.Detector(c)
	if err != nil {
		return nil, err
	}

	// Update cache
	c.CachedAtMs = time.Now().UnixMilli()

	return c, nil
}

// ListSysCaps returns all capabilities, optionally filtered by category, ordered by category:name
func ListSysCaps(category SysCapCategory) ([]*SysCap, error) {
	sysCapMutex.RLock()
	capabilitiesByKey := make(map[SysCapKey]*SysCap)
	for key, capability := range sysCapRegistry {
		if category != "" && capability.Category != category {
			continue
		}
		capabilitiesByKey[key] = capability
	}
	sysCapMutex.RUnlock()

	var wg sync.WaitGroup
	for key, capability := range capabilitiesByKey {
		if !capability.IsCacheValid() && capability.Detector != nil {
			wg.Add(1)
			go func(key SysCapKey, capability *SysCap) {
				defer wg.Done()
				err := capability.Detector(capability)
				if err != nil {
					logging.Warn("Failed to detect capability %s: %v", string(key), err)
					return
				}
				capability.CachedAtMs = time.Now().UnixMilli()
			}(key, capability)
		}
	}
	wg.Wait()

	capabilities := make([]*SysCap, 0, len(capabilitiesByKey))
	for _, capability := range capabilitiesByKey {
		capabilities = append(capabilities, capability)
	}

	// Sort capabilities by category:name
	sort.Slice(capabilities, func(i, j int) bool {
		keyI := NewSysCapKey(capabilities[i].Category, capabilities[i].Name)
		keyJ := NewSysCapKey(capabilities[j].Category, capabilities[j].Name)
		return string(keyI) < string(keyJ)
	})

	return capabilities, nil
}

// ClearCache clears the capability cache
func ClearCache() {
	cacheMutex.Lock()
	defer cacheMutex.Unlock()

	for _, c := range sysCapRegistry {
		c.CachedAtMs = 0
	}

	logging.Debug("SysCap cache cleared")
}

// ClearCacheForCategory clears the cache for a specific category
func ClearCacheForCategory(category SysCapCategory) {
	cacheMutex.Lock()
	defer cacheMutex.Unlock()

	for _, c := range sysCapRegistry {
		if c.Category == category {
			c.CachedAtMs = 0
		}
	}
	logging.Debug("SysCap cache cleared for category: %s", category)
}

// SysCapNotFoundError represents an error when a capability is not found
type SysCapNotFoundError struct {
	Key string
}

func (e *SysCapNotFoundError) Error() string {
	return "capability not found: " + e.Key
}

// newSysCap creates a new capability with timestamp and ID
func NewSysCap(
	category SysCapCategory,
	name SysCapName,
	displayName string,
	present bool,
	cacheTTLSec uint,
) *SysCap {
	now := time.Now().UnixMilli()

	return &SysCap{
		Category:     category,
		Name:         name,
		DisplayName:  displayName,
		Present:      present,
		CachedAtMs:   now,
		CacheTTLMsec: cacheTTLSec,
	}
}

func (c *SysCap) SetPresent(present bool) *SysCap {
	c.Present = present
	return c
}

// SetVersion sets the version field of a capability
func (c *SysCap) SetVersion(version string) *SysCap {
	c.Version = &version
	return c
}

// SetAmount sets the amount and dimension fields of a capability
func (c *SysCap) SetAmount(amount float64, dimension SysCapAmountDimension) *SysCap {
	c.Amount = &amount
	c.AmountDimension = &dimension
	return c
}

// SetDetails sets the details field of a capability
func (c *SysCap) SetDetails(details string) *SysCap {
	c.Details = &details
	return c
}

// InitModule initializes the capabilities module
func InitModule() {
	core.RegisterModule(&core.Module{
		Name:          "syscaps",
		InitAPIRoutes: initSysCapsAPIRoutes,
	})
}
