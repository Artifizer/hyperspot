package auth

import (
	"github.com/google/uuid"
)

var DEFAULT_TENANT_ID = uuid.MustParse("00000000-4f26-4798-b655-feedc1cfe5e3")
var DEFAULT_USER_ID = uuid.MustParse("11111111-6a88-4768-9dfc-6bcd5187d9ed")
var DEFAULT_NODE_ID = uuid.MustParse("22222222-7b99-4879-a0fe-1234567890ab")

func GetTenantID() uuid.UUID {
	return DEFAULT_TENANT_ID
}

func GetUserID() uuid.UUID {
	return DEFAULT_USER_ID
}

func GetNodeID() uuid.UUID {
	return DEFAULT_NODE_ID
}
