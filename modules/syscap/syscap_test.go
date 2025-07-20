package syscap

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestNewSysCapKeyAndParse(t *testing.T) {
	key := NewSysCapKey(CategoryHardware, "gpu")
	assert.Equal(t, SysCapKey("hardware:gpu"), key)

	cat, name, err := ParseSysCapKey(key)
	assert.NoError(t, err)
	assert.Equal(t, CategoryHardware, cat)
	assert.Equal(t, SysCapName("gpu"), name)

	_, _, err = ParseSysCapKey("")
	assert.Error(t, err)
	_, _, err = ParseSysCapKey("hardwaregpu")
	assert.Error(t, err)
	_, _, err = ParseSysCapKey(":gpu")
	assert.Error(t, err)
}

func TestSysCapSetters(t *testing.T) {
	c := NewSysCap(CategorySoftware, "python", "Python", false, 0)
	assert.False(t, c.Present)
	c.SetPresent(true)
	assert.True(t, c.Present)

	c.SetVersion("3.11")
	assert.NotNil(t, c.Version)
	assert.Equal(t, "3.11", *c.Version)

	c.SetAmount(42, DimensionCount)
	assert.NotNil(t, c.Amount)
	assert.Equal(t, 42.0, *c.Amount)
	assert.NotNil(t, c.AmountDimension)
	assert.Equal(t, DimensionCount, *c.AmountDimension)

	c.SetDetails("Installed via Homebrew")
	assert.NotNil(t, c.Details)
	assert.Equal(t, "Installed via Homebrew", *c.Details)
}

func TestRegisterAndGetSysCap(t *testing.T) {
	ClearCache()
	cap := NewSysCap(CategoryHardware, "test_hw", "Test HW", true, 0)
	RegisterSysCap(cap)
	key := NewSysCapKey(CategoryHardware, "test_hw")
	got, err := GetSysCap(key)
	assert.NoError(t, err)
	assert.Equal(t, cap, got)

	// Not found
	_, err = GetSysCap("hardware:doesnotexist")
	assert.Error(t, err)
	assert.IsType(t, &SysCapNotFoundError{}, err)
}

func TestListSysCapsAndSort(t *testing.T) {
	ClearCache()
	RegisterSysCap(NewSysCap(CategoryHardware, "aaa", "AAA", true, 0))
	RegisterSysCap(NewSysCap(CategoryHardware, "zzz", "ZZZ", true, 0))
	RegisterSysCap(NewSysCap(CategorySoftware, "bbb", "BBB", true, 0))

	all, err := ListSysCaps("")
	assert.NoError(t, err)
	assert.True(t, len(all) >= 3)

	hw, err := ListSysCaps(CategoryHardware)
	assert.NoError(t, err)
	for _, c := range hw {
		assert.Equal(t, CategoryHardware, c.Category)
	}
}

func TestCacheAndDetector(t *testing.T) {
	ClearCache()
	called := false
	cap := NewSysCap(CategoryHardware, "det", "Detector", false, 0)
	cap.CacheTTLMsec = 50
	cap.Detector = func(c *SysCap) error {
		called = true
		c.SetPresent(true)
		return nil
	}
	RegisterSysCap(cap)

	key := NewSysCapKey(CategoryHardware, "det")
	c, err := GetSysCap(key)
	assert.NoError(t, err)
	assert.False(t, c.Present)
	assert.False(t, called)

	// Expire cache
	time.Sleep(100 * time.Millisecond)
	called = false
	_, err = GetSysCap(key)
	assert.NoError(t, err)
	assert.True(t, cap.Present)
	assert.True(t, called)
}

func TestClearCacheForCategory(t *testing.T) {
	ClearCache()
	cap := NewSysCap(CategoryOS, "macos", "macOS", true, 0)
	cap.CacheTTLMsec = 100
	RegisterSysCap(cap)
	cap.CachedAtMs = time.Now().UnixMilli()
	ClearCacheForCategory(CategoryOS)
	assert.Equal(t, int64(0), cap.CachedAtMs)
}

func TestSysCapNotFoundError_Error(t *testing.T) {
	err := &SysCapNotFoundError{Key: "x:y"}
	assert.Contains(t, err.Error(), "x:y")
}
