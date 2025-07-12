# Dataset Module

## Overview

The Dataset module provides comprehensive dataset management capabilities for the Hyperspot platform. It handles the registration, downloading, storage, and lifecycle management of datasets used in LLM benchmarks and other machine learning tasks. The module integrates with the job system to provide asynchronous dataset operations and includes built-in integrity verification through checksums and file size validation.

## Domain Model

### Dataset Entity
The core entity is the `Dataset` which represents a dataset with the following structure:

```markdown
<code_block_to_apply_changes_from>
```
Dataset
├── Static Fields (Immutable)
│   ├── Name (Primary Key)
│   ├── Description
│   ├── OriginalURL
│   └── LocalPath
├── Runtime Fields
│   ├── UpdatedAtMs
│   ├── DownloadedAtMs
│   ├── Size
│   ├── Checksum
│   ├── Status
│   ├── Error
│   ├── DownloadProgress
│   ├── Downloaded
│   └── DownloadJobId
└── Internal Fields
    ├── Mutex (Thread Safety)
    └── MutexTrace (Debugging)
```

### Job Integration
The module integrates with the job system through three job types:
- **Download Jobs**: Handle dataset downloads with progress tracking
- **Update Jobs**: Delete and re-download datasets
- **Delete Jobs**: Remove datasets from local storage

## Features

### Core Functionality
- **Dataset Registration**: Register datasets with metadata and download URLs
- **Asynchronous Downloads**: Background download processing with progress tracking
- **Integrity Verification**: Automatic checksum and file size validation
- **Concurrent Access Control**: Thread-safe operations with mutex locking
- **Status Management**: Comprehensive status tracking throughout the lifecycle
- **Error Handling**: Detailed error reporting and recovery mechanisms

### Advanced Features
- **Force Download**: Override existing downloads with fresh copies
- **Progress Tracking**: Real-time download progress monitoring
- **Job Integration**: Seamless integration with the job execution system
- **Database Persistence**: Automatic state persistence and recovery
- **File Management**: Automatic cleanup and storage management

## Configuration Settings

The dataset module uses the following configuration:

### Job Configuration
- **Download Timeout**: 10 hours maximum download time
- **Retry Policy**: 5 retries with 30-second delays
- **Job Queues**:
  - `download` queue for download operations
  - `maintenance` queue for delete operations

### Storage Configuration
- **Local Path**: Configurable local storage directory
- **File Validation**: Automatic checksum and size verification
- **Cleanup**: Automatic file removal on delete operations

## API Endpoints

### Get Dataset Job Parameters Schema
```bash
curl -X GET "http://localhost:8080/job_params/dataset" \
  -H "Content-Type: application/json"
```

### List All Datasets
```bash
curl -X GET "http://localhost:8080/datasets?page_size=10&page_number=1" \
  -H "Content-Type: application/json"
```

### Get Dataset by ID
```bash
curl -X GET "http://localhost:8080/datasets/humaneval" \
  -H "Content-Type: application/json"
```

## Database Schemas

### Dataset Table
```sql
CREATE TABLE datasets (
    name TEXT PRIMARY KEY,
    description TEXT,
    original_url TEXT,
    local_path TEXT,
    updated_at_ms BIGINT,
    downloaded_at_ms BIGINT,
    size BIGINT,
    checksum TEXT,
    status TEXT,
    error TEXT,
    download_progress REAL,
    downloaded BOOLEAN,
    download_job_id TEXT
);

CREATE INDEX idx_datasets_updated_at_ms ON datasets(updated_at_ms);
CREATE INDEX idx_datasets_downloaded_at_ms ON datasets(downloaded_at_ms);
```

## Dependencies

### Internal Module Dependencies
- **job**: Job execution and scheduling system
- **db**: Database operations and migrations
- **api**: API routing and pagination
- **core**: Module registration and initialization
- **logging**: Structured logging system
- **utils**: File operations and utilities

### External Dependencies
- **gorm**: ORM for database operations
- **huma**: API framework for HTTP endpoints
- **uuid**: Unique identifier generation

## Usage Examples

### Registering a Dataset
```bash
# Dataset registration is handled through code configuration
# Datasets are automatically registered during module initialization
```

### Downloading a Dataset
```bash
# Download jobs are automatically scheduled when datasets are accessed
# Progress can be monitored through the job system
```

### Checking Dataset Status
```bash
curl -X GET "http://localhost:8080/datasets/humaneval" | jq '.body.status'
```

### Force Re-download
```bash
# Force download is handled through job parameters
# Set force_download=true in job parameters
```
```
