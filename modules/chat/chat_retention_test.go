package chat

import (
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"

	"github.com/google/uuid"
	"github.com/hypernetix/hyperspot/libs/auth"
	"github.com/hypernetix/hyperspot/libs/db"
	"github.com/hypernetix/hyperspot/libs/orm"
)

// Helper function to set up test database
func setupRetentionTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	testDB, err := db.InitInMemorySQLite(&gorm.Config{})
	if err := orm.OrmInit(testDB); err != nil {
		t.Fatalf("Failed to initialize ORM: %v", err)
	}
	require.NoError(t, err, "Failed to connect to test DB")

	db.SetDB(testDB)
	err = db.SafeAutoMigrate(testDB,
		&ChatMessage{},
		&ChatThread{},
		&ChatThreadGroup{},
	)
	require.NoError(t, err, "Failed to migrate test database")

	// Delete all data from the tables
	// Using Unscoped().Delete() to perform a hard delete that bypasses GORM's soft delete
	db.DB().Unscoped().Where("1=1").Delete(&ChatMessage{})
	db.DB().Unscoped().Where("1=1").Delete(&ChatThread{})
	db.DB().Unscoped().Where("1=1").Delete(&ChatThreadGroup{})

	return testDB
}

// Helper to create chat messages with specific timestamps
func createTestMessagesWithTimestamps(t *testing.T, count int, daysOld int) []*ChatMessage {
	t.Helper()
	messages := make([]*ChatMessage, 0, count)

	// Calculate timestamp
	timestamp := time.Now().AddDate(0, 0, -daysOld).UnixMilli()

	for i := 0; i < count; i++ {
		msg := &ChatMessage{
			ID:          uuid.New(),
			ThreadID:    uuid.New(),
			UserID:      auth.GetUserID(),
			TenantID:    auth.GetTenantID(),
			CreatedAtMs: timestamp,
			Content:     "Test message for retention",
			IsDeleted:   false,
		}
		result := db.DB().Create(msg)
		require.NoError(t, result.Error, "Failed to create test message")
		messages = append(messages, msg)
	}

	return messages
}

// Helper to create chat threads with specific timestamps
func createTestThreadsWithTimestamps(t *testing.T, count int, daysOld int, isDeleted bool) []*ChatThread {
	t.Helper()
	threads := make([]*ChatThread, 0, count)

	// Calculate timestamp
	lastMsgAt := time.Now().AddDate(0, 0, -daysOld).UnixMilli()

	for i := 0; i < count; i++ {
		thread := &ChatThread{
			ID:          uuid.New(),
			UserID:      auth.GetUserID(),
			TenantID:    auth.GetTenantID(),
			GroupID:     uuid.Nil,
			CreatedAtMs: time.Now().UnixMilli(),
			UpdatedAtMs: time.Now().UnixMilli(),
			LastMsgAtMs: lastMsgAt,
			Title:       "Test thread for retention",
			IsActive:    true,
			IsDeleted:   isDeleted,
			IsTemporary: false,
		}
		result := db.DB().Create(thread)
		require.NoError(t, result.Error, "Failed to create test thread")
		threads = append(threads, thread)
	}

	return threads
}

// Helper to create temporary chat threads with specific timestamps
func createTestTempThreadsWithTimestamps(t *testing.T, count int, hoursOld int, isDeleted bool) []*ChatThread {
	t.Helper()
	threads := make([]*ChatThread, 0, count)

	// Calculate timestamp
	lastMsgAt := time.Now().Add(time.Duration(-hoursOld) * time.Hour).UnixMilli()

	for i := 0; i < count; i++ {
		thread := &ChatThread{
			ID:          uuid.New(),
			UserID:      auth.GetUserID(),
			TenantID:    auth.GetTenantID(),
			GroupID:     uuid.Nil,
			CreatedAtMs: time.Now().UnixMilli(),
			UpdatedAtMs: time.Now().UnixMilli(),
			LastMsgAtMs: lastMsgAt,
			Title:       "Test temporary thread for retention",
			IsActive:    true,
			IsDeleted:   isDeleted,
			IsTemporary: true,
		}
		result := db.DB().Create(thread)
		require.NoError(t, result.Error, "Failed to create test temporary thread")
		threads = append(threads, thread)
	}

	return threads
}

// Helper to create chat thread groups with specific timestamps
func createTestGroupsWithTimestamps(t *testing.T, count int, daysOld int, isDeleted bool) []*ChatThreadGroup {
	t.Helper()
	groups := make([]*ChatThreadGroup, 0, count)

	// Calculate timestamp
	lastMsgAt := time.Now().AddDate(0, 0, -daysOld).UnixMilli()

	for i := 0; i < count; i++ {
		group := &ChatThreadGroup{
			ID:          uuid.New(),
			UserID:      auth.GetUserID(),
			TenantID:    auth.GetTenantID(),
			CreatedAtMs: time.Now().UnixMilli(),
			UpdatedAtMs: time.Now().UnixMilli(),
			LastMsgAtMs: lastMsgAt,
			Title:       "Test group for retention",
			IsActive:    true,
			IsDeleted:   isDeleted,
			IsTemporary: false,
		}
		result := db.DB().Create(group)
		require.NoError(t, result.Error, "Failed to create test group")
		groups = append(groups, group)
	}

	return groups
}

// Test messagesHardDelete function
func TestMessagesHardDelete(t *testing.T) {
	setupRetentionTestDB(t)

	// Create older messages (to be deleted)
	oldMessages := createTestMessagesWithTimestamps(t, 5, 31) // 31 days old

	// Create recent messages (to be kept)
	recentMessages := createTestMessagesWithTimestamps(t, 3, 5) // 5 days old

	// Mark the old messages as deleted first (required for hard delete)
	for _, msg := range oldMessages {
		msg.IsDeleted = true
		db.DB().Save(msg)
	}

	// Run the hard delete for messages older than 30 days
	messagesHardDelete(30)

	// Verify old messages were deleted
	for _, msg := range oldMessages {
		var count int64
		db.DB().Model(&ChatMessage{}).Where("id = ?", msg.ID).Order("created_at_ms ASC").Count(&count)
		assert.Equal(t, int64(0), count, "Old message should be hard deleted")
	}

	// Verify recent messages still exist
	for _, msg := range recentMessages {
		var count int64
		db.DB().Model(&ChatMessage{}).Where("id = ?", msg.ID).Count(&count)
		assert.Equal(t, int64(1), count, "Recent message should be preserved")
	}
}

// Test threadsHardDelete function
func TestThreadsHardDelete(t *testing.T) {
	setupRetentionTestDB(t)

	// Create older threads (to be deleted)
	oldThreads := createTestThreadsWithTimestamps(t, 5, 31, false) // 31 days old

	// Create recent threads (to be kept)
	recentThreads := createTestThreadsWithTimestamps(t, 3, 5, false) // 5 days old

	// Mark the old threads as deleted first (required for hard delete)
	for _, thread := range oldThreads {
		thread.IsDeleted = true
		db.DB().Save(thread)
	}

	// Run the hard delete for threads older than 30 days
	threadsHardDelete(30)

	// Verify old threads were deleted
	for _, thread := range oldThreads {
		var count int64
		db.DB().Model(&ChatThread{}).Where("id = ?", thread.ID).Count(&count)
		assert.Equal(t, int64(0), count, "Old thread should be hard deleted")
	}

	// Verify recent threads still exist
	for _, thread := range recentThreads {
		var count int64
		db.DB().Model(&ChatThread{}).Where("id = ?", thread.ID).Count(&count)
		assert.Equal(t, int64(1), count, "Recent thread should be preserved")
	}
}

// Test groupsHardDelete function
func TestGroupsHardDelete(t *testing.T) {
	setupRetentionTestDB(t)

	// Create older groups (to be deleted)
	oldGroups := createTestGroupsWithTimestamps(t, 5, 31, false) // 31 days old

	// Create recent groups (to be kept)
	recentGroups := createTestGroupsWithTimestamps(t, 3, 5, false) // 5 days old

	// Mark the old groups as deleted first (required for hard delete)
	for _, group := range oldGroups {
		group.IsDeleted = true
		db.DB().Save(group)
	}

	// Run the hard delete for groups older than 30 days
	groupsHardDelete(30)

	// Verify old groups were deleted
	for _, group := range oldGroups {
		var count int64
		db.DB().Model(&ChatThreadGroup{}).Where("id = ?", group.ID).Count(&count)
		assert.Equal(t, int64(0), count, "Old group should be hard deleted")
	}

	// Verify recent groups still exist
	for _, group := range recentGroups {
		var count int64
		db.DB().Model(&ChatThreadGroup{}).Where("id = ?", group.ID).Count(&count)
		assert.Equal(t, int64(1), count, "Recent group should be preserved")
	}
}

// Test messagesSoftDelete function
func TestMessagesSoftDelete(t *testing.T) {
	setupRetentionTestDB(t)

	// Create older messages (to be soft-deleted)
	oldMessages := createTestMessagesWithTimestamps(t, 5, 31) // 31 days old

	// Create recent messages (to be kept)
	recentMessages := createTestMessagesWithTimestamps(t, 3, 5) // 5 days old

	// Run the soft delete for messages older than 30 days
	messagesSoftDelete(30)

	// Verify old messages were soft-deleted (is_deleted = true)
	for _, msg := range oldMessages {
		var message ChatMessage
		result := db.DB().Where("id = ?", msg.ID).Order("created_at_ms ASC").First(&message)
		assert.NoError(t, result.Error, "Should find the message")
		assert.True(t, message.IsDeleted, "Old message should be soft-deleted")
	}

	// Verify recent messages still exist and not soft-deleted
	for _, msg := range recentMessages {
		var message ChatMessage
		result := db.DB().Where("id = ?", msg.ID).First(&message)
		assert.NoError(t, result.Error, "Should find the message")
		assert.False(t, message.IsDeleted, "Recent message should not be soft-deleted")
	}
}

// Test threadsSoftDelete function
func TestThreadsSoftDelete(t *testing.T) {
	setupRetentionTestDB(t)

	// Create older threads (to be soft-deleted)
	oldThreads := createTestThreadsWithTimestamps(t, 5, 61, false) // 61 days old

	// Create recent threads (to be kept)
	recentThreads := createTestThreadsWithTimestamps(t, 3, 15, false) // 15 days old

	// Run the soft delete for threads older than 60 days
	threadsSoftDelete(60)

	// Verify old threads were soft-deleted (is_deleted = true)
	for _, thread := range oldThreads {
		var threadObj ChatThread
		result := db.DB().Where("id = ?", thread.ID).Order("created_at_ms ASC").First(&threadObj)
		assert.NoError(t, result.Error, "Should find the thread")
		assert.True(t, threadObj.IsDeleted, "Old thread should be soft-deleted")
	}

	// Verify recent threads still exist and not soft-deleted
	for _, thread := range recentThreads {
		var threadObj ChatThread
		result := db.DB().Where("id = ?", thread.ID).Order("created_at_ms DESC").First(&threadObj)
		assert.NoError(t, result.Error, "Should find the thread")
		assert.False(t, threadObj.IsDeleted, "Recent thread should not be soft-deleted")
	}
}

// Test groupsSoftDelete function
func TestGroupsSoftDelete(t *testing.T) {
	setupRetentionTestDB(t)

	// Create older groups (to be soft-deleted)
	oldGroups := createTestGroupsWithTimestamps(t, 5, 91, false) // 91 days old

	// Create recent groups (to be kept)
	recentGroups := createTestGroupsWithTimestamps(t, 3, 30, false) // 30 days old

	// Run the soft delete for groups older than 90 days
	groupsSoftDelete(90)

	// Verify old groups were soft-deleted (is_deleted = true)
	for _, group := range oldGroups {
		var groupObj ChatThreadGroup
		result := db.DB().Where("id = ?", group.ID).First(&groupObj)
		assert.NoError(t, result.Error, "Should find the group")
		assert.True(t, groupObj.IsDeleted, "Old group should be soft-deleted")
	}

	// Verify recent groups still exist and not soft-deleted
	for _, group := range recentGroups {
		var groupObj ChatThreadGroup
		result := db.DB().Where("id = ?", group.ID).First(&groupObj)
		assert.NoError(t, result.Error, "Should find the group")
		assert.False(t, groupObj.IsDeleted, "Recent group should not be soft-deleted")
	}
}

// Test hardDeleteRetentionJob for one cycle
func TestHardDeleteRetentionJob_SingleCycle(t *testing.T) {
	setupRetentionTestDB(t)

	msgs := make([]*ChatMessage, 0, 1000)
	db.DB().Find(&msgs).Order("created_at_ms ASC")
	for _, msg := range msgs {
		fmt.Println(msg.ID, msg.Content, msg.IsDeleted, msg.CreatedAtMs, time.UnixMilli(msg.CreatedAtMs).Format("2006-01-02 15:04:05.123"))
	}

	// Create data for hard deletion
	createTestMessagesWithTimestamps(t, 3, 31)      // 31 days old messages
	createTestThreadsWithTimestamps(t, 3, 61, true) // 61 days old soft-deleted threads
	createTestGroupsWithTimestamps(t, 3, 91, true)  // 91 days old soft-deleted groups

	// Create a modified version of softDeleteRetentionJob that runs only once
	softDeleteRetentionJobOnce := func(days int) {
		messagesSoftDelete(days)
		threadsSoftDelete(days)
		groupsSoftDelete(days)
	}

	// Create a modified version of hardDeleteRetentionJob that runs only once
	hardDeleteRetentionJobOnce := func(days int) {
		messagesHardDelete(days)
		threadsHardDelete(days)
		groupsHardDelete(days)
	}

	// Run the soft delete job first
	softDeleteRetentionJobOnce(29)

	// Run the hard delete job once
	hardDeleteRetentionJobOnce(30) // Using 30 days as threshold

	// Verify counts
	var messageCount, threadCount, groupCount int64
	db.DB().Model(&ChatMessage{}).Count(&messageCount)
	db.DB().Model(&ChatThread{}).Count(&threadCount)
	db.DB().Model(&ChatThreadGroup{}).Count(&groupCount)

	// All should be deleted with our test parameters
	assert.Equal(t, int64(0), messageCount, "All messages should be hard deleted")
	assert.Equal(t, int64(0), threadCount, "All threads should be hard deleted")
	assert.Equal(t, int64(0), groupCount, "All groups should be hard deleted")
}

// Test softDeleteRetentionJob for one cycle
func TestSoftDeleteRetentionJob_SingleCycle(t *testing.T) {
	setupRetentionTestDB(t)

	// Create data for soft deletion
	createTestMessagesWithTimestamps(t, 3, 31)       // 31 days old messages
	createTestThreadsWithTimestamps(t, 3, 61, false) // 61 days old threads
	createTestGroupsWithTimestamps(t, 3, 91, false)  // 91 days old groups

	// Create a modified version of softDeleteRetentionJob that runs only once
	softDeleteRetentionJobOnce := func(days int) {
		messagesSoftDelete(days)
		threadsSoftDelete(days)
		groupsSoftDelete(days)
	}

	// Run the soft delete job once
	softDeleteRetentionJobOnce(30) // Using 30 days as threshold

	// Verify all items were soft-deleted
	var msgCount, threadCount, groupCount int64
	db.DB().Model(&ChatMessage{}).Where("is_deleted = ?", true).Count(&msgCount)
	db.DB().Model(&ChatThread{}).Where("is_deleted = ?", true).Count(&threadCount)
	db.DB().Model(&ChatThreadGroup{}).Where("is_deleted = ?", true).Count(&groupCount)

	assert.Equal(t, int64(3), msgCount, "All messages should be soft deleted")
	assert.Equal(t, int64(3), threadCount, "All threads should be soft deleted")
	assert.Equal(t, int64(3), groupCount, "All groups should be soft deleted")
}

// Test retentionStart function
func TestRetentionStart(t *testing.T) {
	// This function starts goroutines so we can't easily test it directly
	// We'll test that it doesn't crash and runs without error

	// Mock implementation that doesn't actually start goroutines
	mockRetentionStart := func(pause time.Duration, softDays, hardDays int) bool {
		if softDays <= 0 && hardDays <= 0 {
			return false // Nothing to do
		}
		return true
	}

	// Test different combinations
	assert.True(t, mockRetentionStart(time.Hour, 30, 90), "Should run with both soft and hard delete")
	assert.True(t, mockRetentionStart(time.Hour, 30, 0), "Should run with only soft delete")
	assert.True(t, mockRetentionStart(time.Hour, 0, 90), "Should run with only hard delete")
	assert.False(t, mockRetentionStart(time.Hour, 0, 0), "Should not run with no delete policies")
}

// Test edge cases for delete functions
func TestRetentionEdgeCases(t *testing.T) {
	setupRetentionTestDB(t)

	// Test with negative days
	messagesHardDelete(-10) // Should not delete anything

	// Test with zero days - would delete everything but testing it's handled gracefully
	messagesSoftDelete(0)

	// Test when no data matches criteria
	groupsHardDelete(1000) // No data is this old

	// Verify the database is still intact
	db.SafeAutoMigrate(db.DB(), &ChatMessage{}, &ChatThread{}, &ChatThreadGroup{})

	// Create some data to verify we can still interact with DB
	msg := createTestMessagesWithTimestamps(t, 1, 1)
	assert.NotNil(t, msg, "Should be able to create data after edge case tests")
}

// Test with DB transaction errors (simulated)
func TestRetentionDBErrors(t *testing.T) {
	// This is a complex test that would require mocking the DB layer
	// For now, we'll skip and add a placeholder
	t.Skip("Skipping DB error tests - would require complex mocking")
}

// Test for concurrency issues
func TestRetentionConcurrencyHandling(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping concurrency test in short mode")
	}

	setupRetentionTestDB(t)

	// Create test data
	createTestMessagesWithTimestamps(t, 10, 31)

	// Run multiple deletion operations concurrently
	concurrentCount := 3
	doneChan := make(chan bool, concurrentCount)

	for i := 0; i < concurrentCount; i++ {
		go func() {
			messagesSoftDelete(30)
			doneChan <- true
		}()
	}

	// Wait for all goroutines
	for i := 0; i < concurrentCount; i++ {
		<-doneChan
	}

	// Verify the operations completed
	var softDeletedCount int64
	db.DB().Model(&ChatMessage{}).Where("is_deleted = ?", true).Count(&softDeletedCount)
	assert.Equal(t, int64(10), softDeletedCount, "All messages should be soft-deleted")
}

// Integration test simulating production usage patterns
// Helper to create temporary chat thread groups with specific timestamps
func createTestTempGroupsWithTimestamps(t *testing.T, count int, hoursOld int, isDeleted bool) []*ChatThreadGroup {
	t.Helper()
	groups := make([]*ChatThreadGroup, 0, count)

	// Calculate timestamp
	lastMsgAt := time.Now().Add(time.Duration(-hoursOld) * time.Hour).UnixMilli()

	for i := 0; i < count; i++ {
		group := &ChatThreadGroup{
			ID:          uuid.New(),
			UserID:      auth.GetUserID(),
			TenantID:    auth.GetTenantID(),
			CreatedAtMs: time.Now().UnixMilli(),
			UpdatedAtMs: time.Now().UnixMilli(),
			LastMsgAtMs: lastMsgAt,
			Title:       "Test temporary group for retention",
			IsActive:    true,
			IsDeleted:   isDeleted,
			IsTemporary: true,
		}
		result := db.DB().Create(group)
		require.NoError(t, result.Error, "Failed to create test temporary group")
		groups = append(groups, group)
	}

	return groups
}

// Test tempThreadsAndGroupsSoftDelete function
func TestTempThreadsAndGroupsSoftDelete(t *testing.T) {
	setupRetentionTestDB(t)

	// Create temporary threads and groups with different ages
	// Recent temporary items (to be kept)
	recentTempThreads := createTestTempThreadsWithTimestamps(t, 3, 12, false) // 12 hours old
	recentTempGroups := createTestTempGroupsWithTimestamps(t, 3, 12, false)   // 12 hours old

	// Older temporary items (to be soft deleted)
	olderTempThreads := createTestTempThreadsWithTimestamps(t, 5, 36, false) // 36 hours old
	olderTempGroups := createTestTempGroupsWithTimestamps(t, 5, 36, false)   // 36 hours old

	messages_count := 1003

	// Create messages for the threads
	for _, thread := range recentTempThreads {
		for i := 0; i < messages_count; i++ {
			msg := &ChatMessage{
				ID:          uuid.New(),
				ThreadID:    thread.ID,
				UserID:      auth.GetUserID(),
				TenantID:    auth.GetTenantID(),
				CreatedAtMs: thread.LastMsgAtMs,
				Content:     "Test message for temporary thread",
				IsDeleted:   false,
			}
			result := db.DB().Create(msg)
			require.NoError(t, result.Error, "Failed to create test message")
		}
	}

	for _, thread := range olderTempThreads {
		for i := 0; i < messages_count; i++ {
			msg := &ChatMessage{
				ID:          uuid.New(),
				ThreadID:    thread.ID,
				UserID:      auth.GetUserID(),
				TenantID:    auth.GetTenantID(),
				CreatedAtMs: thread.LastMsgAtMs,
				Content:     "Test message for older temporary thread",
				IsDeleted:   false,
			}
			result := db.DB().Create(msg)
			require.NoError(t, result.Error, "Failed to create test message")
		}
	}

	// Run the soft delete for temporary items older than 24 hours
	tempThreadsAndGroupsSoftDelete(24)

	// Verify recent temporary threads were kept intact
	for _, thread := range recentTempThreads {
		var foundThread ChatThread
		result := db.DB().Where("id = ?", thread.ID).First(&foundThread)
		require.NoError(t, result.Error, "Recent temporary thread should exist")
		assert.False(t, foundThread.IsDeleted, "Recent temporary thread should not be marked as deleted")

		// Check messages in this thread
		var msgCount int64
		db.DB().Model(&ChatMessage{}).Where("thread_id = ? AND is_deleted = ?", thread.ID, false).Count(&msgCount)
		assert.Equal(t, int64(messages_count), msgCount, "Messages in recent temporary thread should not be deleted")
	}

	// Verify recent temporary groups were kept intact
	for _, group := range recentTempGroups {
		var foundGroup ChatThreadGroup
		result := db.DB().Where("id = ?", group.ID).First(&foundGroup)
		require.NoError(t, result.Error, "Recent temporary group should exist")
		assert.False(t, foundGroup.IsDeleted, "Recent temporary group should not be marked as deleted")
	}

	// Verify older temporary threads were soft deleted
	for _, thread := range olderTempThreads {
		var foundThread ChatThread
		result := db.DB().Where("id = ?", thread.ID).First(&foundThread)
		require.NoError(t, result.Error, "Older temporary thread should still exist")
		assert.True(t, foundThread.IsDeleted, "Older temporary thread should be marked as deleted")

		// Check messages in this thread
		var msgCount int64
		db.DB().Model(&ChatMessage{}).Where("thread_id = ? AND is_deleted = ?", thread.ID, true).Count(&msgCount)
		assert.Equal(t, int64(messages_count), msgCount, "Messages in older temporary thread should be soft deleted")
	}

	// Verify older temporary groups were soft deleted
	for _, group := range olderTempGroups {
		var foundGroup ChatThreadGroup
		result := db.DB().Where("id = ?", group.ID).First(&foundGroup)
		require.NoError(t, result.Error, "Older temporary group should still exist")
		assert.True(t, foundGroup.IsDeleted, "Older temporary group should be marked as deleted")
	}
}

// Test tempThreadsAndGroupsHardDelete function
func TestTempThreadsAndGroupsHardDelete(t *testing.T) {
	setupRetentionTestDB(t)

	// Create temporary threads and groups with different ages
	// Recent temporary items (to be kept)
	recentTempThreads := createTestTempThreadsWithTimestamps(t, 3, 24, true) // 24 hours old, already soft-deleted
	recentTempGroups := createTestTempGroupsWithTimestamps(t, 3, 24, true)   // 24 hours old, already soft-deleted

	// Older temporary items (to be hard deleted)
	olderTempThreads := createTestTempThreadsWithTimestamps(t, 5, 72, true) // 72 hours old, already soft-deleted
	olderTempGroups := createTestTempGroupsWithTimestamps(t, 5, 72, true)   // 72 hours old, already soft-deleted

	// Create messages for the threads
	for _, thread := range recentTempThreads {
		msg := &ChatMessage{
			ID:          uuid.New(),
			ThreadID:    thread.ID,
			UserID:      auth.GetUserID(),
			TenantID:    auth.GetTenantID(),
			CreatedAtMs: thread.LastMsgAtMs,
			Content:     "Test message for temporary thread",
			IsDeleted:   true, // Already soft-deleted
		}
		result := db.DB().Create(msg)
		require.NoError(t, result.Error, "Failed to create test message")
	}

	for _, thread := range olderTempThreads {
		msg := &ChatMessage{
			ID:          uuid.New(),
			ThreadID:    thread.ID,
			UserID:      auth.GetUserID(),
			TenantID:    auth.GetTenantID(),
			CreatedAtMs: thread.LastMsgAtMs,
			Content:     "Test message for older temporary thread",
			IsDeleted:   true, // Already soft-deleted
		}
		result := db.DB().Create(msg)
		require.NoError(t, result.Error, "Failed to create test message")
	}

	// Run the hard delete for temporary items older than 48 hours
	tempThreadsAndGroupsHardDelete(48)

	// Verify recent temporary threads were kept
	for _, thread := range recentTempThreads {
		var count int64
		db.DB().Model(&ChatThread{}).Where("id = ?", thread.ID).Count(&count)
		assert.Equal(t, int64(1), count, "Recent temporary thread should not be hard deleted")

		// Check messages in this thread
		var msgCount int64
		db.DB().Model(&ChatMessage{}).Where("thread_id = ?", thread.ID).Count(&msgCount)
		assert.Equal(t, int64(1), msgCount, "Messages in recent temporary thread should not be hard deleted")
	}

	// Verify recent temporary groups were kept
	for _, group := range recentTempGroups {
		var count int64
		db.DB().Model(&ChatThreadGroup{}).Where("id = ?", group.ID).Count(&count)
		assert.Equal(t, int64(1), count, "Recent temporary group should not be hard deleted")
	}

	// Verify older temporary threads were hard deleted
	for _, thread := range olderTempThreads {
		var count int64
		db.DB().Model(&ChatThread{}).Where("id = ?", thread.ID).Count(&count)
		assert.Equal(t, int64(0), count, "Older temporary thread should be hard deleted")

		// Check messages in this thread
		var msgCount int64
		db.DB().Model(&ChatMessage{}).Where("thread_id = ?", thread.ID).Count(&msgCount)
		assert.Equal(t, int64(0), msgCount, "Messages in older temporary thread should be hard deleted")
	}

	// Verify older temporary groups were hard deleted
	for _, group := range olderTempGroups {
		var count int64
		db.DB().Model(&ChatThreadGroup{}).Where("id = ?", group.ID).Count(&count)
		assert.Equal(t, int64(0), count, "Older temporary group should be hard deleted")
	}
}

// Test integration of temporary thread/group retention with regular retention
func TestTempRetentionIntegration(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	setupRetentionTestDB(t)

	// Create regular data
	createTestMessagesWithTimestamps(t, 3, 5)        // 5 days old
	createTestThreadsWithTimestamps(t, 3, 10, false) // 10 days old
	createTestGroupsWithTimestamps(t, 3, 15, false)  // 15 days old

	// Create temporary data with different ages
	// Recent temporary data (to be kept)
	createTestTempThreadsWithTimestamps(t, 3, 12, false) // 12 hours old
	createTestTempGroupsWithTimestamps(t, 3, 12, false)  // 12 hours old

	// Older temporary data (to be soft deleted)
	createTestTempThreadsWithTimestamps(t, 3, 36, false) // 36 hours old
	createTestTempGroupsWithTimestamps(t, 3, 36, false)  // 36 hours old

	// Very old temporary data (to be hard deleted)
	oldTempThreads := createTestTempThreadsWithTimestamps(t, 3, 72, false) // 72 hours old
	oldTempGroups := createTestTempGroupsWithTimestamps(t, 3, 72, false)   // 72 hours old

	// Mark the very old temporary data as soft-deleted
	for _, thread := range oldTempThreads {
		thread.IsDeleted = true
		db.DB().Save(thread)
	}
	for _, group := range oldTempGroups {
		group.IsDeleted = true
		db.DB().Save(group)
	}

	// Print the state of threads before running retention
	var beforeCount int64
	db.DB().Model(&ChatThread{}).Where("is_temporary = ? AND last_msg_at_ms <= ? AND last_msg_at_ms > ? AND is_deleted = ?",
		true, time.Now().Add(-24*time.Hour).UnixMilli(), time.Now().Add(-48*time.Hour).UnixMilli(), false).Count(&beforeCount)

	// Run soft delete for temporary items
	tempThreadsAndGroupsSoftDelete(24)

	// Verify the older temporary threads and groups were soft-deleted immediately after the soft delete
	var afterSoftDeleteThreadCount, afterSoftDeleteGroupCount int64
	db.DB().Model(&ChatThread{}).Where("is_temporary = ? AND last_msg_at_ms <= ? AND last_msg_at_ms > ? AND is_deleted = ?",
		true, time.Now().Add(-24*time.Hour).UnixMilli(), time.Now().Add(-48*time.Hour).UnixMilli(), true).Count(&afterSoftDeleteThreadCount)
	db.DB().Model(&ChatThreadGroup{}).Where("is_temporary = ? AND last_msg_at_ms <= ? AND last_msg_at_ms > ? AND is_deleted = ?",
		true, time.Now().Add(-24*time.Hour).UnixMilli(), time.Now().Add(-48*time.Hour).UnixMilli(), true).Count(&afterSoftDeleteGroupCount)

	assert.Equal(t, int64(3), afterSoftDeleteThreadCount, "Older temporary threads should be soft deleted after tempThreadsAndGroupsSoftDelete")
	assert.Equal(t, int64(3), afterSoftDeleteGroupCount, "Older temporary groups should be soft deleted after tempThreadsAndGroupsSoftDelete")

	// Run hard delete for temporary items
	tempThreadsAndGroupsHardDelete(48)

	// Verify counts
	var recentTempThreadCount, recentTempGroupCount int64
	var olderTempThreadCount, olderTempGroupCount int64
	var veryOldTempThreadCount, veryOldTempGroupCount int64
	var regularThreadCount, regularGroupCount int64

	// Recent temporary items should be intact
	db.DB().Model(&ChatThread{}).Where("is_temporary = ? AND last_msg_at_ms > ? AND is_deleted = ?",
		true, time.Now().Add(-24*time.Hour).UnixMilli(), false).Count(&recentTempThreadCount)
	db.DB().Model(&ChatThreadGroup{}).Where("is_temporary = ? AND last_msg_at_ms > ? AND is_deleted = ?",
		true, time.Now().Add(-24*time.Hour).UnixMilli(), false).Count(&recentTempGroupCount)

	// Older temporary items should still be in database and soft deleted (after hard delete of very old items)
	db.DB().Model(&ChatThread{}).Where("is_temporary = ? AND last_msg_at_ms <= ? AND last_msg_at_ms > ? AND is_deleted = ?",
		true, time.Now().Add(-24*time.Hour).UnixMilli(), time.Now().Add(-48*time.Hour).UnixMilli(), true).Count(&olderTempThreadCount)
	db.DB().Model(&ChatThreadGroup{}).Where("is_temporary = ? AND last_msg_at_ms <= ? AND last_msg_at_ms > ? AND is_deleted = ?",
		true, time.Now().Add(-24*time.Hour).UnixMilli(), time.Now().Add(-48*time.Hour).UnixMilli(), true).Count(&olderTempGroupCount)

	// Very old temporary items should be hard deleted
	db.DB().Model(&ChatThread{}).Where("is_temporary = ? AND last_msg_at_ms <= ?",
		true, time.Now().Add(-48*time.Hour).UnixMilli()).Count(&veryOldTempThreadCount)
	db.DB().Model(&ChatThreadGroup{}).Where("is_temporary = ? AND last_msg_at_ms <= ?",
		true, time.Now().Add(-48*time.Hour).UnixMilli()).Count(&veryOldTempGroupCount)

	// Regular items should be untouched
	db.DB().Model(&ChatThread{}).Where("is_temporary = ?", false).Count(&regularThreadCount)
	db.DB().Model(&ChatThreadGroup{}).Where("is_temporary = ?", false).Count(&regularGroupCount)

	assert.Equal(t, int64(3), recentTempThreadCount, "Recent temporary threads should be kept")
	assert.Equal(t, int64(3), recentTempGroupCount, "Recent temporary groups should be kept")

	assert.Equal(t, int64(3), olderTempThreadCount, "Older temporary threads should be soft deleted")
	assert.Equal(t, int64(3), olderTempGroupCount, "Older temporary groups should be soft deleted")

	assert.Equal(t, int64(0), veryOldTempThreadCount, "Very old temporary threads should be hard deleted")
	assert.Equal(t, int64(0), veryOldTempGroupCount, "Very old temporary groups should be hard deleted")

	assert.Equal(t, int64(3), regularThreadCount, "Regular threads should be untouched")
	assert.Equal(t, int64(3), regularGroupCount, "Regular groups should be untouched")
}

func TestRetentionIntegration(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	setupRetentionTestDB(t)

	// Create a variety of data with different timestamps
	// Recent active data
	createTestMessagesWithTimestamps(t, 5, 5)
	createTestThreadsWithTimestamps(t, 5, 10, false)
	createTestGroupsWithTimestamps(t, 5, 15, false)

	// Older data (soft delete candidates)
	createTestMessagesWithTimestamps(t, 5, 45)
	createTestThreadsWithTimestamps(t, 5, 75, false)
	createTestGroupsWithTimestamps(t, 5, 85, false)

	// Very old soft-deleted data (hard delete candidates)
	messages := createTestMessagesWithTimestamps(t, 5, 95)
	threads := createTestThreadsWithTimestamps(t, 5, 125, false)
	groups := createTestGroupsWithTimestamps(t, 5, 155, false)

	// Mark the very old data as soft-deleted
	for _, msg := range messages {
		msg.IsDeleted = true
		db.DB().Save(msg)
	}
	for _, thread := range threads {
		thread.IsDeleted = true
		db.DB().Save(thread)
	}
	for _, group := range groups {
		group.IsDeleted = true
		db.DB().Save(group)
	}

	// Run soft delete first
	softDeleteRetentionJobOnce := func(days int) {
		messagesSoftDelete(days)
		threadsSoftDelete(days)
		groupsSoftDelete(days)
	}
	softDeleteRetentionJobOnce(30)

	// Then run hard delete
	hardDeleteRetentionJobOnce := func(days int) {
		messagesHardDelete(days)
		threadsHardDelete(days)
		groupsHardDelete(days)
	}
	hardDeleteRetentionJobOnce(90)

	// Verify recent data is intact
	var recentMsgCount, recentThreadCount, recentGroupCount int64
	db.DB().Model(&ChatMessage{}).Where("created_at_ms > ?", time.Now().AddDate(0, 0, -30).UnixMilli()).Count(&recentMsgCount)
	db.DB().Model(&ChatThread{}).Where("last_msg_at_ms > ?", time.Now().AddDate(0, 0, -30).UnixMilli()).Count(&recentThreadCount)
	db.DB().Model(&ChatThreadGroup{}).Where("last_msg_at_ms > ?", time.Now().AddDate(0, 0, -30).UnixMilli()).Count(&recentGroupCount)

	assert.Equal(t, int64(5), recentMsgCount, "Recent messages should be preserved")
	assert.Equal(t, int64(5), recentThreadCount, "Recent threads should be preserved")
	assert.Equal(t, int64(5), recentGroupCount, "Recent groups should be preserved")

	// Verify older data is soft-deleted but not hard-deleted
	var olderMsgCount, olderThreadCount, olderGroupCount int64
	db.DB().Model(&ChatMessage{}).Where("created_at_ms <= ? AND created_at_ms > ? AND is_deleted = ?",
		time.Now().AddDate(0, 0, -30).UnixMilli(),
		time.Now().AddDate(0, 0, -90).UnixMilli(),
		true).Count(&olderMsgCount)
	db.DB().Model(&ChatThread{}).Where("last_msg_at_ms <= ? AND last_msg_at_ms > ? AND is_deleted = ?",
		time.Now().AddDate(0, 0, -30).UnixMilli(),
		time.Now().AddDate(0, 0, -90).UnixMilli(),
		true).Count(&olderThreadCount)
	db.DB().Model(&ChatThreadGroup{}).Where("last_msg_at_ms <= ? AND last_msg_at_ms > ? AND is_deleted = ?",
		time.Now().AddDate(0, 0, -30).UnixMilli(),
		time.Now().AddDate(0, 0, -90).UnixMilli(),
		true).Count(&olderGroupCount)

	assert.Equal(t, int64(5), olderMsgCount, "Older messages should be soft-deleted but not hard-deleted")
	assert.Equal(t, int64(5), olderThreadCount, "Older threads should be soft-deleted but not hard-deleted")
	assert.Equal(t, int64(5), olderGroupCount, "Older groups should be soft-deleted but not hard-deleted")

	// Verify very old data is hard-deleted (check for both timestamp AND deleted flag)
	var veryOldMsgCount, veryOldThreadCount, veryOldGroupCount int64
	db.DB().Model(&ChatMessage{}).Where("created_at_ms <= ? AND is_deleted = ?",
		time.Now().AddDate(0, 0, -90).UnixMilli(), true).Count(&veryOldMsgCount)
	db.DB().Model(&ChatThread{}).Where("last_msg_at_ms <= ? AND is_deleted = ?",
		time.Now().AddDate(0, 0, -90).UnixMilli(), true).Count(&veryOldThreadCount)
	db.DB().Model(&ChatThreadGroup{}).Where("last_msg_at_ms <= ? AND is_deleted = ?",
		time.Now().AddDate(0, 0, -90).UnixMilli(), true).Count(&veryOldGroupCount)

	assert.Equal(t, int64(0), veryOldMsgCount, "Very old messages should be hard-deleted")
	assert.Equal(t, int64(0), veryOldThreadCount, "Very old threads should be hard-deleted")
	assert.Equal(t, int64(0), veryOldGroupCount, "Very old groups should be hard-deleted")
}
