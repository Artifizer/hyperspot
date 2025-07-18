# Chat Module

The Chat module provides comprehensive chat functionality for HyperSpot, enabling users to interact with various LLM services through a structured chat interface. It supports thread-based conversations, message management, file attachments, system prompts, retention policies, and both streaming and non-streaming responses.

## Overview

The chat module is built around a hierarchical structure:

```
Chat Groups (optional)
└── Chat Threads
    ├── System Prompts (optional)
    └── Chat Messages
        ├── User Messages
        ├── Assistant Messages
        ├── System Messages (optional)
        └── File Attachment Messages (optional)
```

## Features

### Core Functionality
- **Multi-tenant and multi-user support** with proper isolation
- **Thread-based conversations** with optional grouping
- **Real-time streaming responses** for interactive chat experiences
- **File attachment support** with intelligent parsing and content truncation
- **System prompt management** for customizing AI behavior
- **Message retention policies** with configurable soft/hard deletion
- **Comprehensive API** with REST endpoints for all operations

### Message Types
- **User Messages**: Human input to the conversation
- **Assistant Messages**: AI responses from LLM services
- **System Messages**: System-level instructions
- **File Attachment Messages**: Special message with parsed content from uploaded files

### File Attachment Support
- **Supported formats**: PDF, DOCX, HTML, TXT, MD (via file_parser module)
- **Content parsing**: Automatic text extraction from documents
- **Size limiting**: Configurable maximum file size (default: 16MB, see the config file options `chat.uploaded_file_max_size_kb`)
- **Content truncation**: Intelligent truncation at whitespace boundaries (16KB by default, see the config file options `chat.uploaded_file_max_content_length`)
- **Metadata preservation**: Original file size, content length, file extension

## Configuration

The chat module supports extensive configuration through the `config.yaml` file under the `chat` section:

### Message Retention Settings

```yaml
chat:
  # Soft delete messages after this many days (marks as deleted but keeps in DB)
  soft_delete_messages_after_days: 90

  # Hard delete messages after this many days (permanently removes from DB)
  hard_delete_messages_after_days: 120

  # Soft delete temporary threads after this many hours
  soft_delete_temp_threads_after_hours: 24

  # Hard delete temporary threads after this many hours
  hard_delete_temp_threads_after_hours: 48
```

### File Upload Settings

```yaml
chat:
  # Directory for temporary file storage during upload processing
  uploaded_file_temp_dir: ""  # empty means system default temp dir

  # Maximum content length in characters before truncation
  uploaded_file_max_content_length: 16384  # 16K characters

  # Maximum file size in KB for uploads
  uploaded_file_max_size_kb: 16384  # 16MB
```

### Logging Configuration

```yaml
chat_logger:
  console_level: "info"     # Console log level
  file_level: "debug"       # File log level
  file: "logs/chat.log"     # Log file path
  max_size_mb: 1000         # Max log file size before rotation
  max_backups: 3            # Number of backup log files to keep
  max_age_days: 28          # Max age of log files before deletion
```

## API Endpoints

### Chat Messages

#### Create Message
- **POST** `/chat/messages` - Create a new message and start a new thread
- **POST** `/chat/threads/{thread_id}/messages` - Create a message in an existing thread

**Request Body:**
```json
{
  "model_name": "string",
  "temperature": 0.5,
  "max_tokens": 2000,
  "content": "string",
  "stream": false,
  "group_id": "string (optional)"
}
```

**Query Parameters:**
- `replace_msg_id`: Replace a message and delete all subsequent messages

#### Message Management
- **GET** `/chat/messages/{id}` - Get a specific message by ID
- **PUT** `/chat/messages/{id}` - Update a message's content
- **DELETE** `/chat/messages/{id}` - Delete a message

### Chat Threads

#### Thread Management
- **GET** `/chat/groups/{group_id}/threads` - List threads in a group
- **GET** `/chat/threads/{id}` - Get a specific thread
- **POST** `/chat/threads` - Create a new thread
- **PUT** `/chat/threads/{id}` - Update thread properties
- **DELETE** `/chat/threads/{id}` - Delete a thread

**Thread Properties:**
```json
{
  "title": "string",
  "group_id": "uuid",
  "is_pinned": false,
  "is_public": false,
  "is_temporary": false,
  "system_prompt_ids": ["uuid1", "uuid2"]
}
```

### Chat Groups

#### Group Management
- **GET** `/chat/groups` - List all groups
- **GET** `/chat/groups/{id}` - Get a specific group
- **POST** `/chat/groups` - Create a new group
- **PUT** `/chat/groups/{id}` - Update group properties
- **DELETE** `/chat/groups/{id}` - Delete a group

### File Attachments

#### Upload Endpoints
- **POST** `/chat/attachments` - Create new thread with file attachment
- **POST** `/chat/threads/{thread_id}/attachment` - Add file to existing thread

**Upload Requirements:**
- **Content-Type**: `multipart/form-data`
- **Field Name**: `file`
- **Max Size**: Configured via `uploaded_file_max_size_kb`

**Query Parameters:**
- `group_id`: Optional group ID for new thread creation

### System Prompts

#### Prompt Management
- **GET** `/chat/system-prompts` - List all system prompts
- **GET** `/chat/system-prompts/{id}` - Get a specific prompt
- **POST** `/chat/system-prompts` - Create a new system prompt
- **PUT** `/chat/system-prompts/{id}` - Update a system prompt
- **DELETE** `/chat/system-prompts/{id}` - Delete a system prompt

#### Thread-Prompt Association
- **POST** `/chat/threads/{thread_id}/system-prompts` - Attach prompts to thread
- **DELETE** `/chat/threads/{thread_id}/system-prompts` - Detach prompts from thread

## Database Schema

### Core Tables

**ChatMessage**
- `id`: Unique message identifier
- `thread_id`: Reference to parent thread
- `user_id`: Message author
- `role`: Message role (user/assistant/system)
- `content`: Message text content
- `model_name`: LLM model used for generation
- `temperature`: Model temperature parameter
- `max_tokens`: Token limit for generation
- `created_at_ms`: Creation timestamp
- `completed`: Whether message generation is complete
- `size_chars`: Content size in characters
- `size_tokens`: Content size in tokens
- `tokens_per_sec`: Generation rate
- `finish_reason`: Completion reason
- File attachment fields (when applicable):
  - `is_file_attachment`: Boolean flag
  - `original_file_size`: File size in bytes
  - `full_content_length`: Full parsed content length
  - `truncated_content_length`: Truncated content length
  - `is_truncated`: Whether content was truncated
  - `attached_file_name`: Original filename
  - `attached_file_extension`: File extension

**ChatThread**
- `id`: Unique thread identifier
- `group_id`: Optional parent group
- `user_id`: Thread owner
- `title`: Thread title
- `created_at_ms`: Creation timestamp
- `last_msg_at_ms`: Last message timestamp
- `is_active`: Active status
- `is_deleted`: Soft deletion flag
- `is_pinned`: Pinned status
- `is_public`: Public visibility
- `is_temporary`: Temporary thread flag
- `size_chars`: Total content size
- `size_tokens`: Total token count

**ChatThreadGroup**
- `id`: Unique group identifier
- `user_id`: Group owner
- `title`: Group title
- `created_at_ms`: Creation timestamp
- `last_msg_at_ms`: Last activity timestamp
- `is_active`: Active status
- `is_deleted`: Soft deletion flag
- `is_pinned`: Pinned status
- `is_public`: Public visibility
- `is_temporary`: Temporary group flag

**SystemPrompt**
- `id`: Unique prompt identifier
- `user_id`: Prompt owner
- `name`: Prompt name
- `content`: Prompt text
- `ui_meta`: UI metadata (JSON)
- `created_at_ms`: Creation timestamp
- `updated_at_ms`: Last update timestamp

**ChatThreadSystemPrompt** (Junction Table)
- `thread_id`: Reference to thread
- `system_prompt_id`: Reference to system prompt
- `created_at_ms`: Association timestamp

## Streaming Support

The chat module supports real-time streaming responses for interactive chat experiences:

### Streaming Flow
1. Client sends message with `"stream": true`
2. Server immediately creates user message
3. Server proxies request to LLM service
4. Server streams response chunks back to client
5. Server creates final assistant message when complete

### Streaming Response Format
```json
{
  "delta": {
    "id": "message-uuid",
    "content": "partial content",
    "finish_reason": "",
    "error": ""
  }
}
```

### Final Response
```json
{
  "patch": {
    // Complete ChatMessage object
  }
}
```

## Retention Policy

The chat module implements automated retention policies to manage storage and privacy:

### Retention Jobs
- **Soft Delete Job**: Runs every 5 minutes, marks old content as deleted
- **Hard Delete Job**: Runs every 5 minutes, permanently removes old content
- **Temporary Content**: Special handling for temporary threads/groups

### Retention Stages
1. **Active**: Normal operation
2. **Soft Deleted**: Marked as deleted but data preserved
3. **Hard Deleted**: Permanently removed from database

### Configuration Impact
- Messages older than `soft_delete_messages_after_days` are soft deleted
- Messages older than `hard_delete_messages_after_days` are hard deleted
- Temporary content follows accelerated `temp_*_hours` schedules

## Integration Points

### LLM Services
- Supports all configured LLM services (Ollama, LM Studio, OpenAI, etc.)
- Automatic model selection and routing
- Transparent streaming support
- Error handling and fallback mechanisms

### File Parser Module
- Automatic content extraction from supported file formats
- Intelligent content truncation
- Metadata preservation
- Error handling for unsupported formats

### Authentication & Authorization
- Multi-tenant isolation
- User-based access control
- Automatic tenant/user context injection

## Performance Considerations

### Pagination
- All list endpoints support pagination via `PageAPIRequest`
- Configurable page sizes with reasonable defaults
- Efficient database queries with proper indexing

### Content Limits
- Configurable content truncation prevents memory issues
- Streaming reduces client memory usage
- Temporary file cleanup prevents disk bloat

### Database Optimization
- Proper indexing on frequently queried fields
- Soft deletion for better performance
- Background retention jobs minimize impact

## Error Handling

The chat module provides comprehensive error handling:

### Error Types
- **400 Bad Request**: Invalid input parameters
- **404 Not Found**: Requested resource doesn't exist
- **413 Payload Too Large**: File size exceeds limits
- **415 Unsupported Media Type**: Unsupported file format
- **500 Internal Server Error**: System errors

### Error Response Format
```json
{
  "error": {
    "code": "error_code",
    "message": "Human readable message",
    "details": {}
  }
}
```

## Usage Examples

### Basic Chat Flow
```bash
# Create a new message (starts new thread)
curl -X POST http://localhost:8087/chat/messages \
  -H "Content-Type: application/json" \
  -d '{
    "model_name": "llama3:8b",
    "content": "Hello, how are you?",
    "temperature": 0.7
  }'

# Add message to existing thread
curl -X POST http://localhost:8087/chat/threads/{thread_id}/messages \
  -H "Content-Type: application/json" \
  -d '{
    "model_name": "llama3:8b",
    "content": "Follow up question",
    "stream": true
  }'
```

### File Upload
```bash
# Upload file to new thread
curl -X POST http://localhost:8087/chat/attachments \
  -F "file=@document.pdf"

# Upload file to existing thread
curl -X POST http://localhost:8087/chat/threads/{thread_id}/attachment \
  -F "file=@document.docx"
```

### Thread Management
```bash
# Create thread with system prompt
curl -X POST http://localhost:8087/chat/threads \
  -H "Content-Type: application/json" \
  -d '{
    "title": "Code Review Session",
    "system_prompt_ids": ["prompt-uuid"],
    "is_pinned": true
  }'
```

## Testing

The chat module includes comprehensive test coverage:

- **Unit Tests**: Core functionality and edge cases
- **Integration Tests**: API endpoints and database operations
- **Retention Tests**: Background job functionality
- **Attachment Tests**: File upload and parsing

Run tests:
```bash
# Unit tests
go test ./modules/chat/...

# API tests
python tests/api_tests.py
```

## Dependencies

- **Database**: GORM for ORM operations
- **File Parser**: Module for document parsing
- **LLM Services**: Integration with various AI providers
- **Authentication**: User and tenant management
- **Logging**: Structured logging with rotation

## Troubleshooting

### Common Issues

**File Upload Failures**
- Check file size limits in configuration
- Verify supported file formats
- Ensure proper multipart/form-data encoding

**Streaming Issues**
- Verify LLM service connectivity
- Check network timeouts
- Monitor server logs for errors

**Retention Problems**
- Validate retention configuration values
- Check background job status in logs
- Verify database connectivity

### Debug Logging
Enable debug logging in configuration:
```yaml
chat_logger:
  console_level: "debug"
  file_level: "trace"
```

### Health Checks
Monitor chat functionality:
- Database connectivity
- LLM service availability
- File system permissions
- Background job execution
