package llm

import (
	"testing"

	"github.com/hypernetix/hyperspot/libs/config"
	"github.com/hypernetix/hyperspot/libs/logging"
	"github.com/stretchr/testify/assert"
)

func TestRegisterLLMServiceSysCap(t *testing.T) {
	t.Run("single successful instance", func(t *testing.T) {
		resetLLMServiceSysCap()
		cfg := config.ConfigLLMService{}
		logger := logging.NewLogger(logging.DebugLevel, "", logging.NoneLevel, 0, 0, 0)
		mockService := NewMockService("", cfg, logger)
		mockService.PingShouldFail = false

		cap := registerLLMServiceSysCap(mockService, "test-service", "Test Service")
		assert.NotNil(t, cap)

		// Trigger detector
		err := cap.Detector(cap)
		assert.NoError(t, err)
		assert.True(t, cap.Present)
		assert.Equal(t, 1, mockService.PingCount)
	})

	t.Run("single failing instance", func(t *testing.T) {
		resetLLMServiceSysCap()
		cfg := config.ConfigLLMService{}
		logger := logging.NewLogger(logging.DebugLevel, "", logging.NoneLevel, 0, 0, 0)
		mockService := NewMockService("", cfg, logger)
		mockService.PingShouldFail = true

		cap := registerLLMServiceSysCap(mockService, "test-service", "Test Service")
		assert.NotNil(t, cap)

		// Trigger detector
		err := cap.Detector(cap)
		assert.Nil(t, err)
		assert.False(t, cap.Present)
		assert.Equal(t, 1, mockService.PingCount)
	})

	t.Run("multiple instances, one successful", func(t *testing.T) {
		resetLLMServiceSysCap()
		cfg := config.ConfigLLMService{}
		logger := logging.NewLogger(logging.DebugLevel, "", logging.NoneLevel, 0, 0, 0)
		mockService1 := NewMockService("", cfg, logger)
		mockService1.PingShouldFail = false
		mockService2 := NewMockService("", cfg, logger)
		mockService2.PingShouldFail = true

		cap1 := registerLLMServiceSysCap(mockService1, "test-service", "Test Service")
		cap2 := registerLLMServiceSysCap(mockService2, "test-service", "Test Service")
		assert.Same(t, cap1, cap2, "should return the same capability instance")

		// Trigger detector
		err := cap1.Detector(cap1)
		assert.NoError(t, err)
		assert.True(t, cap1.Present)
		assert.True(t, cap2.Present)
		assert.Equal(t, 1, mockService1.PingCount)
		assert.Equal(t, 0, mockService2.PingCount)
	})

	t.Run("multiple instances, all failing", func(t *testing.T) {
		resetLLMServiceSysCap()
		cfg := config.ConfigLLMService{}
		logger := logging.NewLogger(logging.DebugLevel, "", logging.NoneLevel, 0, 0, 0)
		mockService1 := NewMockService("", cfg, logger)
		mockService1.PingShouldFail = true
		mockService2 := NewMockService("", cfg, logger)
		mockService2.PingShouldFail = true

		cap1 := registerLLMServiceSysCap(mockService1, "test-service", "Test Service")
		cap2 := registerLLMServiceSysCap(mockService2, "test-service", "Test Service")
		assert.Same(t, cap1, cap2)

		// Trigger detector
		err := cap1.Detector(cap1)
		assert.Nil(t, err)
		assert.False(t, cap1.Present)
		assert.False(t, cap2.Present)
		assert.Equal(t, 1, mockService1.PingCount)
		assert.Equal(t, 1, mockService2.PingCount)
	})

	t.Run("multiple different services", func(t *testing.T) {
		resetLLMServiceSysCap()
		cfg := config.ConfigLLMService{}
		logger := logging.NewLogger(logging.DebugLevel, "", logging.NoneLevel, 0, 0, 0)
		mockService1 := NewMockService("", cfg, logger)
		mockService1.PingShouldFail = false
		mockService2 := NewMockService("", cfg, logger)
		mockService2.PingShouldFail = false

		cap1 := registerLLMServiceSysCap(mockService1, "test-service-1", "Test Service 1")
		cap2 := registerLLMServiceSysCap(mockService2, "test-service-2", "Test Service 2")
		assert.NotSame(t, cap1, cap2)

		// Trigger detector for cap1
		err := cap1.Detector(cap1)
		assert.NoError(t, err)
		assert.True(t, cap1.Present)
		assert.Equal(t, 1, mockService1.PingCount)

		// Trigger detector for cap2
		err = cap2.Detector(cap2)
		assert.NoError(t, err)
		assert.True(t, cap2.Present)
		assert.Equal(t, 1, mockService2.PingCount)
	})
}
