package chat

import (
	"github.com/hypernetix/hyperspot/libs/core"
)

func InitModule() {
	core.RegisterModule(&core.Module{
		Migrations:    []interface{}{&ChatMessage{}},
		InitAPIRoutes: registerChatMessageAPIRoutes,
		Name:          "chat_message",
	})
	core.RegisterModule(&core.Module{
		Migrations:    []interface{}{&ChatThread{}},
		InitAPIRoutes: registerChatThreadAPIRoutes,
		Name:          "chat_thread",
	})
	core.RegisterModule(&core.Module{
		Migrations:    []interface{}{&ChatThreadGroup{}},
		InitAPIRoutes: registerChatThreadGroupAPIRoutes,
		Name:          "chat_thread_group",
	})
	core.RegisterModule(&core.Module{
		Migrations:    []interface{}{&SystemPrompt{}, &ChatThreadSystemPrompt{}},
		InitAPIRoutes: registerSystemPromptAPIRoutes,
		Name:          "system_prompt",
	})
	core.RegisterModule(&core.Module{
		Migrations:    []interface{}{}, // No new migrations for attachments
		InitAPIRoutes: registerChatAttachmentAPIRoutes,
		Name:          "chat_attachment",
	})
}
