package chat

import (
	"context"
	"fmt"
	"math"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/hypernetix/hyperspot/libs/api"
	"github.com/hypernetix/hyperspot/libs/auth"
	"github.com/hypernetix/hyperspot/libs/db"
	"github.com/hypernetix/hyperspot/modules/llm"
	"gorm.io/gorm"
)

// Setup test database
func setupTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	testDB, err := db.InitInMemorySQLite(nil)
	require.NoError(t, err, "Failed to connect to test DB")

	db.SetDB(testDB)
	err = db.SafeAutoMigrate(testDB,
		&ChatMessage{},
		&ChatThread{},
		&ChatThreadGroup{},
		&SystemPrompt{},
		&ChatThreadSystemPrompt{},
	)
	require.NoError(t, err, "Failed to migrate test database")

	return testDB
}

// Create test thread for reuse across tests
func createTestThread(t *testing.T) *ChatThread {
	ctx := context.Background()
	thread, errx := CreateChatThread(ctx, uuid.Nil, "Test Thread", nil, nil, nil, []uuid.UUID{})
	require.Nil(t, errx, "Failed to create test thread")
	return thread
}

// TestChatMessageRoles ensures role constants and mappings are correct
func TestChatMessageRoles(t *testing.T) {
	// Test role constants
	assert.Equal(t, ChatMessageRole(0), ChatMessageRoleIdUser)
	assert.Equal(t, ChatMessageRole(1), ChatMessageRoleIdAssistant)
	assert.Equal(t, ChatMessageRole(2), ChatMessageRoleIdSystem)

	// Test role name constants
	assert.Equal(t, "user", ChatMessageRoleNameUser)
	assert.Equal(t, "assistant", ChatMessageRoleNameAssistant)
	assert.Equal(t, "system", ChatMessageRoleNameSystem)

	// Test role mappings
	assert.Equal(t, ChatMessageRoleNameUser, ChatMessageRoleNames[ChatMessageRoleIdUser])
	assert.Equal(t, ChatMessageRoleNameAssistant, ChatMessageRoleNames[ChatMessageRoleIdAssistant])
	assert.Equal(t, ChatMessageRoleNameSystem, ChatMessageRoleNames[ChatMessageRoleIdSystem])

	assert.Equal(t, ChatMessageRoleIdUser, ChatMessageRoleIds[ChatMessageRoleNameUser])
	assert.Equal(t, ChatMessageRoleIdAssistant, ChatMessageRoleIds[ChatMessageRoleNameAssistant])
	assert.Equal(t, ChatMessageRoleIdSystem, ChatMessageRoleIds[ChatMessageRoleNameSystem])
}

// TestDbSaveFields verifies that field updates work correctly
func TestDbSaveFields(t *testing.T) {
	setupTestDB(t)

	// Create a message
	msg := &ChatMessage{
		ID:          uuid.New(),
		TenantID:    auth.GetTenantID(),
		UserID:      auth.GetUserID(),
		ThreadID:    uuid.New(),
		RoleId:      ChatMessageRoleIdUser,
		Content:     "Original content",
		CreatedAtMs: time.Now().UnixMilli(),
	}

	// Save the message to the database
	err := db.DB().Create(msg).Error
	require.NoError(t, err, "Failed to create message")

	// Update a field
	msg.Content = "Updated content"
	errx := msg.DbSaveFields(&msg.Content)
	assert.Nil(t, errx, "DbSaveFields should not return an error")

	// Verify the update
	var updatedMsg ChatMessage
	err = db.DB().Where("id = ?", msg.ID).First(&updatedMsg).Error
	require.NoError(t, err, "Failed to retrieve updated message")
	assert.Equal(t, "Updated content", updatedMsg.Content)
}

// TestListMessages verifies the ListMessages function
func TestListMessages(t *testing.T) {
	setupTestDB(t)
	ctx := context.Background()

	llm.RegisterMockServiceForUnitTests(ctx)

	// Create a thread
	thread := createTestThread(t)

	// Create some messages
	for i := 0; i < 5; i++ {
		_, errx := CreateMessage(
			ctx,
			uuid.Nil,
			thread.ID,
			"user",
			"mock-model",
			100,
			0.5,
			"Test message "+string(rune(i+65)), // A, B, C, D, E
			true,
			10,
			0.1,
		)
		require.Nil(t, errx, fmt.Sprintf("Failed to create test message: %v", errx))
	}

	// Test default paging
	messages, errx := listMessages(ctx, thread.ID, &api.PageAPIRequest{})
	assert.Nil(t, errx, "ListMessages should not return an error")
	assert.Equal(t, 5, len(messages), "Should return all 5 messages")

	// Test with paging
	messages, errx = listMessages(ctx, thread.ID, &api.PageAPIRequest{PageSize: 2, PageNumber: 1})
	assert.Nil(t, errx, "ListMessages should not return an error")
	assert.Equal(t, 2, len(messages), "Should return 2 messages")

	// Test with invalid thread ID
	messages, errx = listMessages(ctx, uuid.New(), &api.PageAPIRequest{})
	assert.NotNil(t, errx, "ListMessages should return an error for invalid thread ID")
	assert.Nil(t, messages)
}

// TestGetMessage verifies the GetMessage function
func TestGetMessage(t *testing.T) {
	setupTestDB(t)
	ctx := context.Background()

	llm.RegisterMockServiceForUnitTests(ctx)
	// Create a thread
	thread := createTestThread(t)

	// Create a message
	msg, errx := CreateMessage(
		ctx,
		uuid.Nil,
		thread.ID,
		"user",
		"mock-model",
		100,
		0.5,
		"Test message content",
		true,
		10,
		0.1,
	)
	require.Nil(t, errx, "Failed to create test message")

	// Test getting the message
	retrievedMsg, errx := GetMessage(ctx, msg.ID)
	assert.Nil(t, errx, "GetMessage should not return an error")
	assert.NotNil(t, retrievedMsg)
	assert.Equal(t, msg.ID, retrievedMsg.ID)
	assert.Equal(t, "user", retrievedMsg.Role)
	assert.Equal(t, msg.Content, retrievedMsg.Content)

	// Test with invalid message ID
	retrievedMsg, errx = GetMessage(ctx, uuid.New())
	assert.NotNil(t, errx, "GetMessage should return an error for invalid message ID")
	assert.Nil(t, retrievedMsg)
}

// TestShortenMessage verifies the shortenMessage function
func TestShortenMessage(t *testing.T) {
	longMessage := "This is a very long message that needs to be shortened for display purposes"

	// Test with message shorter than max length
	result := shortenMessage(longMessage, 100)
	assert.Equal(t, longMessage, result)

	// Test with message longer than max length
	result = shortenMessage(longMessage, 20)
	assert.Equal(t, "This is a very long ...", result)
	assert.Equal(t, 23, len(result)) // 20 + 3 chars for "..."
}

// TestPrepareLLMChatRequest verifies the PrepareLLMChatRequest function
func TestPrepareLLMChatRequest(t *testing.T) {
	setupTestDB(t)
	ctx := context.Background()

	llm.RegisterMockServiceForUnitTests(ctx)

	// Create a thread
	thread := createTestThread(t)

	// Create a previous message for chat history
	_, errx := CreateMessage(
		ctx,
		uuid.Nil,
		thread.ID,
		"user",
		"mock-model",
		100,
		0.5,
		"Hello, assistant",
		true,
		10,
		0.1,
	)
	require.Nil(t, errx, "Failed to create test message")

	// Mock an LLMModel
	model := &llm.LLMModel{
		Name:              "mock-model",
		UpstreamModelName: "mock-model",
		MaxTokens:         4000,
	}

	// Test preparing a chat request
	msg, req, errx := PrepareLLMChatRequest(
		ctx,
		uuid.Nil,
		thread.ID,
		model,
		0.7,
		200,
		"How can I help you today?",
		false,
	)

	assert.Nil(t, errx, "PrepareLLMChatRequest should not return an error")
	assert.NotNil(t, msg)
	assert.NotNil(t, req)

	// Verify message fields
	assert.Equal(t, "user", msg.Role)
	assert.Equal(t, ChatMessageRoleIdUser, msg.RoleId)
	assert.Equal(t, "How can I help you today?", msg.Content)
	assert.Equal(t, 0.7, math.Round(float64(msg.Temperature)*10)/10)
	assert.Equal(t, 200, msg.MaxTokens)
	assert.Equal(t, "mock-model", msg.ModelName)

	// Verify request fields
	assert.Equal(t, "mock-model", req.ModelName)
	assert.Equal(t, 0.7, math.Round(float64(req.Temperature)*10)/10)
	assert.Equal(t, 200, req.MaxTokens)
	assert.Equal(t, 2, len(req.Messages))

	// Test with invalid thread ID
	msg, req, errx = PrepareLLMChatRequest(
		ctx,
		uuid.Nil,
		uuid.New(),
		model,
		0.7,
		200,
		"This should fail",
		false,
	)

	assert.NotNil(t, errx, "PrepareLLMChatRequest should return an error for invalid thread ID")
	assert.Nil(t, msg)
	assert.Nil(t, req)
}

// TestValidateMessage verifies the ValidateMessage function
func TestValidateMessage(t *testing.T) {
	ctx := context.Background()

	// Test with valid values
	errx := ValidateMessage(ctx, "Valid content", "mock-model", 100, 0.5)
	assert.Nil(t, errx, "ValidateMessage should not return an error for valid values")

	// Test with empty content
	errx = ValidateMessage(ctx, "", "mock-model", 100, 0.5)
	assert.Nil(t, errx, "ValidateMessage should accept empty content")

	// Test with invalid temperature
	errx = ValidateMessage(ctx, "Content", "mock-model", 100, 1.5)
	assert.NotNil(t, errx, "ValidateMessage should return an error for temperature > 1.0")
	assert.Contains(t, errx.Error(), "Temperature must be between 0.0 and 1.0")

	errx = ValidateMessage(ctx, "Content", "mock-model", 100, -0.5)
	assert.NotNil(t, errx, "ValidateMessage should return an error for temperature < 0.0")
	assert.Contains(t, errx.Error(), "Temperature must be between 0.0 and 1.0")
}

// TestCreateMessage verifies the CreateMessage function
func TestCreateMessage(t *testing.T) {
	setupTestDB(t)
	ctx := context.Background()

	llm.RegisterMockServiceForUnitTests(ctx)

	// Create a thread
	thread := createTestThread(t)

	// Test creating a message
	msg, errx := CreateMessage(
		ctx,
		uuid.Nil,
		thread.ID,
		"user",
		"mock-model",
		100,
		0.5,
		"Test create message",
		true,
		10,
		0.1,
	)

	assert.Nil(t, errx, "CreateMessage should not return an error")
	assert.NotNil(t, msg)
	assert.Equal(t, "user", msg.Role)
	assert.Equal(t, ChatMessageRoleIdUser, msg.RoleId)
	assert.Equal(t, "Test create message", msg.Content)
	assert.Equal(t, "mock-model", msg.ModelName)
	assert.Equal(t, 0.5, float64(msg.Temperature))
	assert.Equal(t, 100, msg.MaxTokens)
	assert.True(t, msg.Completed)
	assert.Equal(t, 10, msg.SizeTokens)

	// Test creating a message with invalid thread ID
	msg, errx = CreateMessage(
		ctx,
		uuid.Nil,
		uuid.New(),
		"user",
		"mock-model",
		100,
		0.5,
		"This should fail",
		true,
		10,
		0.1,
	)

	assert.NotNil(t, errx, "CreateMessage should return an error for invalid thread ID")
	assert.Nil(t, msg)

	// Test creating a message with invalid role
	msg, errx = CreateMessage(
		ctx,
		uuid.Nil,
		thread.ID,
		"invalid-role",
		"mock-model",
		100,
		0.5,
		"This should fail",
		true,
		10,
		0.1,
	)

	assert.NotNil(t, errx, "CreateMessage should return an error for invalid role")
	assert.Nil(t, msg)
}

// TestUpdateMessage verifies the UpdateMessage function
func TestUpdateMessage(t *testing.T) {
	setupTestDB(t)
	ctx := context.Background()

	llm.RegisterMockServiceForUnitTests(ctx)

	// Create a thread
	thread := createTestThread(t)

	// Create a message
	msg, errx := CreateMessage(
		ctx,
		uuid.Nil,
		thread.ID,
		"user",
		"mock-model",
		100,
		0.5,
		"Original content",
		true,
		10,
		0.1,
	)
	require.Nil(t, errx, "Failed to create test message")
	assert.Equal(t, msg.CreatedAtMs, msg.UpdatedAtMs, "Timestamp should be equal to updated at")

	ts := msg.CreatedAtMs

	time.Sleep(10 * time.Millisecond)

	// Test updating the message
	updatedMsg, errx := UpdateMessage(ctx, msg.ID, "Updated content")
	assert.Nil(t, errx, "UpdateMessage should not return an error")

	assert.Equal(t, ts, updatedMsg.CreatedAtMs, "Timestamp should not change")
	assert.Greater(t, updatedMsg.UpdatedAtMs, ts, "Updated at should change")

	assert.NotNil(t, updatedMsg)
	assert.Equal(t, "Updated content", updatedMsg.Content)

	// Verify the update in the database
	retrievedMsg, errx := GetMessage(ctx, msg.ID)
	assert.Nil(t, errx)
	assert.Equal(t, "Updated content", retrievedMsg.Content)

	// Test updating a non-existent message
	updatedMsg, errx = UpdateMessage(ctx, uuid.New(), "This should fail")
	assert.NotNil(t, errx, "UpdateMessage should return an error for invalid message ID")
	assert.Nil(t, updatedMsg)
}

func TestDeleteAndResetTail(t *testing.T) {
	setupTestDB(t)
	ctx := context.Background()

	llm.RegisterMockServiceForUnitTests(ctx)

	// Create a thread
	thread := createTestThread(t)

	// Create a message
	msg1, errx := CreateMessage(
		ctx,
		uuid.Nil,
		thread.ID,
		"user",
		"mock-model",
		100,
		0.5,
		"Message to be updated later",
		true,
		10,
		0.1,
	)
	require.Nil(t, errx, "Failed to create test message #1")

	time.Sleep(10 * time.Millisecond)

	// Create another message
	msg2, errx := CreateMessage(
		ctx,
		uuid.Nil,
		thread.ID,
		"user",
		"mock-model",
		100,
		0.5,
		"Message to be deleted if prev one is updated",
		true,
		10,
		0.1,
	)
	require.Nil(t, errx, "Failed to create test message #2")

	// Create another message
	msg3, errx := CreateMessage(
		ctx,
		uuid.Nil,
		thread.ID,
		"user",
		"mock-model",
		100,
		0.5,
		"Message to be deleted if prev one is updated",
		true,
		10,
		0.1,
	)
	require.Nil(t, errx, "Failed to create test message #3")

	errx = DeleteChatTail(ctx, thread.ID, msg2.ID)
	require.Nil(t, errx, "Failed to delete chat tail: %v", errx)

	msg2, errx = GetMessage(ctx, msg2.ID)
	require.Nil(t, errx, "Failed to retrieve test message #2")

	msg3, errx = GetMessage(ctx, msg3.ID)
	require.Nil(t, errx, "Failed to retrieve test message #3")

	assert.Equal(t, false, msg1.IsDeleted, "Message #1 should not be deleted")
	assert.Equal(t, true, msg2.IsDeleted, "Message #2 should be deleted")
	assert.Equal(t, true, msg3.IsDeleted, "Message #3 should be deleted")
}

// TestDeleteMessage verifies the DeleteMessage function
func TestDeleteMessage(t *testing.T) {
	setupTestDB(t)
	ctx := context.Background()

	llm.RegisterMockServiceForUnitTests(ctx)

	// Create a thread
	thread := createTestThread(t)

	// Create a message
	msg, errx := CreateMessage(
		ctx,
		uuid.Nil,
		thread.ID,
		"user",
		"mock-model",
		100,
		0.5,
		"Test delete message",
		true,
		10,
		0.1,
	)
	require.Nil(t, errx, "Failed to create test message")

	// Test deleting the message
	errx = DeleteMessage(ctx, msg.ID)
	assert.Nil(t, errx, "DeleteMessage should not return an error")

	// Verify the message is marked as deleted
	var deletedMsg ChatMessage
	err := db.DB().Where("id = ?", msg.ID).First(&deletedMsg).Error
	require.NoError(t, err, "Failed to retrieve deleted message")
	assert.True(t, deletedMsg.IsDeleted, "Message should be marked as deleted")

	// Test deleting a non-existent message
	errx = DeleteMessage(ctx, uuid.New())
	assert.NotNil(t, errx, "DeleteMessage should return an error for invalid message ID")
}

// The following tests require more mocking of dependencies to fully test.
// In a real environment, we would mock the LLM module and other services.

// Skipping complex integration tests for now as they require more extensive mocking
func TestCreateMessage_MockedDependencies(t *testing.T) {
	// Register mock service
	llm.RegisterMockServiceForUnitTests(context.Background())

	setupTestDB(t)
	ctx := context.Background()

	// Create a thread
	_, errx := CreateChatThread(ctx, uuid.Nil, "Test Thread", nil, nil, nil, []uuid.UUID{})
	require.Nil(t, errx, "Failed to create test thread")
}

func TestListMessages_MockedDependencies(t *testing.T) {
	t.Skip("Requires more complete mocking of dependencies")
}

func TestGetMessage_MockedDependencies(t *testing.T) {
	t.Skip("Requires more complete mocking of dependencies")
}

func TestPrepareLLMChatRequest_MockedDependencies(t *testing.T) {
	// Register mock service
	llm.RegisterMockServiceForUnitTests(context.Background())

	setupTestDB(t)
	ctx := context.Background()

	// Create a thread
	_, errx := CreateChatThread(ctx, uuid.Nil, "Test Thread", nil, nil, nil, []uuid.UUID{})
	require.Nil(t, errx, "Failed to create test thread")
}

func TestUpdateMessage_MockedDependencies(t *testing.T) {
	t.Skip("Requires more complete mocking of dependencies")
}

func TestDeleteMessage_MockedDependencies(t *testing.T) {
	t.Skip("Requires more complete mocking of dependencies")
}

// TestLastMsgAtUpdates verifies that LastMsgAt is properly updated in both thread and group
func TestLastMsgAtUpdates(t *testing.T) {
	setupTestDB(t)
	ctx := context.Background()

	llm.RegisterMockServiceForUnitTests(ctx)

	// 1. Create a chat group
	group, errx := CreateChatThreadGroup(ctx, "Test Group", nil, nil, nil)
	require.Nil(t, errx, "Failed to create chat group")
	require.NotNil(t, group)

	// 2. Create a chat thread in this group
	thread, errx := CreateChatThread(ctx, group.ID, "Test Thread", nil, nil, nil, []uuid.UUID{})
	require.Nil(t, errx, "Failed to create chat thread")
	require.NotNil(t, thread)
	require.Equal(t, group.ID, thread.GroupID)

	// 3. Write a message to this thread
	msg1, errx := CreateMessage(
		ctx,
		group.ID,
		thread.ID,
		"user",
		"mock-model",
		100,
		0.5,
		"First test message",
		true,
		10,
		0.1,
	)
	require.Nil(t, errx, "Failed to create first message")
	require.NotNil(t, msg1)

	// 4. Save thread.LastMsgAt and group.LastMsgAt to variables
	// Refresh thread and group to get updated timestamps
	thread, errx = GetChatThread(ctx, thread.ID)
	require.Nil(t, errx, "Failed to get thread")
	group, errx = GetChatThreadGroup(ctx, group.ID)
	require.Nil(t, errx, "Failed to get group")

	firstThreadLastMsgAt := thread.LastMsgAtMs
	firstGroupLastMsgAt := group.LastMsgAtMs

	// 5. Sleep 0.3 sec
	time.Sleep(300 * time.Millisecond)

	// 6. Write new message to the thread
	msg2, errx := CreateMessage(
		ctx,
		group.ID,
		thread.ID,
		"user",
		"mock-model",
		100,
		0.5,
		"Second test message",
		true,
		10,
		0.1,
	)
	require.Nil(t, errx, "Failed to create second message")
	require.NotNil(t, msg2)

	// Refresh thread and group to get updated timestamps
	thread, errx = GetChatThread(ctx, thread.ID)
	require.Nil(t, errx, "Failed to get thread")
	group, errx = GetChatThreadGroup(ctx, group.ID)
	require.Nil(t, errx, "Failed to get group")

	// 7. Ensure group.LastMsgAt and thread.LastMsgAt are greater than previous ones
	assert.Greater(t, thread.LastMsgAtMs, firstThreadLastMsgAt, "Thread LastMsgAt should be updated")
	assert.Greater(t, group.LastMsgAtMs, firstGroupLastMsgAt, "Group LastMsgAt should be updated")

	// 8. Ensure that the new LastMsgAt values are within 0.1 sec from now
	now := time.Now().UnixMilli()
	maxDiff := int64(100) // 0.1 sec in milliseconds

	threadDiff := now - thread.LastMsgAtMs
	groupDiff := now - group.LastMsgAtMs

	assert.LessOrEqual(t, threadDiff, maxDiff, "Thread LastMsgAt should be within 0.1 sec from now")
	assert.LessOrEqual(t, groupDiff, maxDiff, "Group LastMsgAt should be within 0.1 sec from now")
}
