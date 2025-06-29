package auth

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestGetTenantID(t *testing.T) {
	// Test that GetTenantID returns the expected default tenant ID
	tenantID := GetTenantID()
	assert.Equal(t, DEFAULT_TENANT_ID, tenantID)
	// Compare only first part of the UUID
	assert.True(t, strings.HasPrefix(tenantID.String(), "00000000"))
}

func TestGetUserID(t *testing.T) {
	// Test that GetUserID returns the expected default user ID
	userID := GetUserID()
	assert.Equal(t, DEFAULT_USER_ID, userID)
	// Compare only first part of the UUID
	assert.True(t, strings.HasPrefix(userID.String(), "11111111"))
}

func TestGetNodeID(t *testing.T) {
	// Test that GetNodeID returns the expected default node ID
	nodeID := GetNodeID()
	assert.Equal(t, DEFAULT_NODE_ID, nodeID)
	// Compare only first part of the UUID
	assert.True(t, strings.HasPrefix(nodeID.String(), "22222222"))
}

func TestDefaultIDs(t *testing.T) {
	// Test that the default IDs are valid UUIDs and have the expected values
	// Compare only first part of the UUIDs
	assert.True(t, strings.HasPrefix(DEFAULT_TENANT_ID.String(), "00000000"))
	assert.True(t, strings.HasPrefix(DEFAULT_USER_ID.String(), "11111111"))
	assert.True(t, strings.HasPrefix(DEFAULT_NODE_ID.String(), "22222222"))
}
