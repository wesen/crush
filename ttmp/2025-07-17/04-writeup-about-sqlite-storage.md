# SQLite Storage Architecture: Complete Database Guide

## Overview

The Crush TUI application uses SQLite as its primary storage backend, implementing a sophisticated schema designed for conversation management, session tracking, file storage, and message persistence. The database is optimized for real-time TUI interactions while maintaining data integrity and performance.

## Database Schema

### Core Tables

#### 1. Sessions Table

```sql
CREATE TABLE sessions (
    id TEXT PRIMARY KEY,
    title TEXT NOT NULL,
    description TEXT,
    provider TEXT NOT NULL,
    model TEXT NOT NULL,
    system_prompt TEXT,
    temperature REAL,
    max_tokens INTEGER,
    parent_id TEXT,
    tool_call_id TEXT,
    is_active BOOLEAN DEFAULT TRUE,
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    FOREIGN KEY (parent_id) REFERENCES sessions(id) ON DELETE SET NULL
);

CREATE INDEX idx_sessions_parent_id ON sessions(parent_id);
CREATE INDEX idx_sessions_created_at ON sessions(created_at);
```

**Purpose**: Stores AI conversation sessions with full configuration
**Key Features**:
- Hierarchical session relationships (parent/child)
- Tool call linking for nested operations
- Complete agent configuration storage
- Soft deletion support

#### 2. Messages Table

```sql
CREATE TABLE messages (
    id TEXT PRIMARY KEY,
    session_id TEXT NOT NULL,
    role TEXT NOT NULL CHECK (role IN ('system', 'user', 'assistant', 'tool')),
    content TEXT NOT NULL,
    provider TEXT NOT NULL,
    model TEXT NOT NULL,
    tool_calls TEXT, -- JSON array of tool calls
    tool_call_id TEXT,
    attachments TEXT, -- JSON array of attachments
    summary_message_id TEXT,
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    FOREIGN KEY (session_id) REFERENCES sessions(id) ON DELETE CASCADE,
    FOREIGN KEY (summary_message_id) REFERENCES messages(id) ON DELETE SET NULL
);

CREATE INDEX idx_messages_session_id ON messages(session_id);
CREATE INDEX idx_messages_created_at ON messages(created_at);
CREATE INDEX idx_messages_provider ON messages(provider);
```

**Purpose**: Stores all conversation messages with full metadata
**Key Features**:
- JSON storage for complex data (tool calls, attachments)
- Role-based message classification
- Provider/model tracking for multi-provider support
- Attachment support for files/images

#### 3. Files Table

```sql
CREATE TABLE files (
    id TEXT PRIMARY KEY,
    session_id TEXT NOT NULL,
    name TEXT NOT NULL,
    path TEXT NOT NULL,
    size INTEGER NOT NULL,
    mime_type TEXT,
    content BLOB, -- Binary content for small files
    content_preview TEXT, -- Text preview for display
    is_binary BOOLEAN DEFAULT FALSE,
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    FOREIGN KEY (session_id) REFERENCES sessions(id) ON DELETE CASCADE
);

CREATE INDEX idx_files_session_id ON files(session_id);
CREATE INDEX idx_files_created_at ON files(created_at);
```

**Purpose**: File storage with metadata and preview capabilities
**Key Features**:
- Binary storage for small files
- Content preview for TUI display
- MIME type detection
- Binary vs text file differentiation

#### 4. Attachments Table

```sql
CREATE TABLE attachments (
    id TEXT PRIMARY KEY,
    message_id TEXT NOT NULL,
    file_id TEXT NOT NULL,
    type TEXT NOT NULL CHECK (type IN ('image', 'file', 'code')),
    metadata TEXT, -- JSON metadata
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    FOREIGN KEY (message_id) REFERENCES messages(id) ON DELETE CASCADE,
    FOREIGN KEY (file_id) REFERENCES files(id) ON DELETE CASCADE
);

CREATE INDEX idx_attachments_message_id ON attachments(message_id);
CREATE INDEX idx_attachments_file_id ON attachments(file_id);
```

**Purpose**: Links files to messages as attachments
**Key Features**:
- Type classification for different attachment types
- JSON metadata for flexible extension
- Many-to-many relationship support

## Schema Evolution

### Migration History

#### Initial Schema (20250424200609_initial.sql)
```sql
-- Initial database creation
CREATE TABLE migrations (
    version TEXT PRIMARY KEY,
    applied_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
);

-- Core tables created above
```

#### Enhanced Schema (20250515105448_add_summary_message_id.sql)
```sql
-- Added summary message linking
ALTER TABLE messages ADD COLUMN summary_message_id TEXT;

-- Added foreign key constraint
ALTER TABLE messages ADD CONSTRAINT fk_summary_message 
    FOREIGN KEY (summary_message_id) REFERENCES messages(id) ON DELETE SET NULL;
```

#### Performance Optimization (20250624000000_add_created_at_indexes.sql)
```sql
-- Added composite indexes for common queries
CREATE INDEX idx_sessions_created_at_provider ON sessions(created_at, provider);
CREATE INDEX idx_messages_session_created ON messages(session_id, created_at);
CREATE INDEX idx_files_session_created ON files(session_id, created_at);
```

#### Provider Enhancement (20250627000000_add_provider_to_messages.sql)
```sql
-- Added provider information to messages for better tracking
ALTER TABLE messages ADD COLUMN provider TEXT NOT NULL DEFAULT 'openai';
ALTER TABLE messages ADD COLUMN model TEXT NOT NULL DEFAULT 'gpt-4';
```

## Data Relationships

### Entity Relationship Diagram

```
sessions
├── id (PK)
├── parent_id → sessions.id (FK)
└── 1:N messages

messages
├── id (PK)
├── session_id → sessions.id (FK)
├── summary_message_id → messages.id (FK)
└── 1:N attachments

files
├── id (PK)
├── session_id → sessions.id (FK)
└── 1:N attachments

attachments
├── id (PK)
├── message_id → messages.id (FK)
└── file_id → files.id (FK)
```

### Hierarchical Session Structure

```sql
-- Recursive query for session hierarchy
WITH RECURSIVE session_tree AS (
    SELECT id, parent_id, title, 0 as level
    FROM sessions
    WHERE parent_id IS NULL
    
    UNION ALL
    
    SELECT s.id, s.parent_id, s.title, st.level + 1
    FROM sessions s
    JOIN session_tree st ON s.parent_id = st.id
)
SELECT * FROM session_tree ORDER BY level, created_at;
```

## Query Patterns

### Common Queries

#### 1. Session Management

```sql
-- Get all sessions with message counts
SELECT 
    s.*,
    COUNT(m.id) as message_count,
    MAX(m.created_at) as last_message_at
FROM sessions s
LEFT JOIN messages m ON s.id = m.session_id
GROUP BY s.id
ORDER BY s.updated_at DESC;

-- Get session with full conversation
SELECT 
    s.*,
    json_group_array(
        json_object(
            'id', m.id,
            'role', m.role,
            'content', m.content,
            'created_at', m.created_at
        )
    ) as messages
FROM sessions s
JOIN messages m ON s.id = m.session_id
WHERE s.id = ?
GROUP BY s.id;
```

#### 2. Message Retrieval

```sql
-- Get messages for a session with pagination
SELECT * FROM messages
WHERE session_id = ?
ORDER BY created_at ASC
LIMIT ? OFFSET ?;

-- Get messages with attachments
SELECT 
    m.*,
    json_group_array(
        json_object(
            'id', a.id,
            'type', a.type,
            'name', f.name,
            'size', f.size
        )
    ) as attachments
FROM messages m
LEFT JOIN attachments a ON m.id = a.message_id
LEFT JOIN files f ON a.file_id = f.id
WHERE m.session_id = ?
GROUP BY m.id;
```

#### 3. File Management

```sql
-- Get files for a session
SELECT * FROM files
WHERE session_id = ?
ORDER BY created_at DESC;

-- Search files by name or content
SELECT * FROM files
WHERE session_id = ?
  AND (name LIKE ? OR content_preview LIKE ?)
ORDER BY created_at DESC;
```

#### 4. Complex Analytics

```sql
-- Session analytics
SELECT 
    provider,
    COUNT(*) as session_count,
    AVG(
        SELECT COUNT(*) FROM messages WHERE session_id = s.id
    ) as avg_messages_per_session,
    MAX(created_at) as latest_session
FROM sessions s
GROUP BY provider
ORDER BY session_count DESC;

-- Message type distribution
SELECT 
    role,
    COUNT(*) as count,
    ROUND(COUNT(*) * 100.0 / SUM(COUNT(*)) OVER(), 2) as percentage
FROM messages
WHERE session_id = ?
GROUP BY role;
```

## Performance Optimization

### Indexing Strategy

#### Primary Indexes
```sql
-- Primary key indexes (automatic)
CREATE INDEX idx_sessions_pk ON sessions(id);
CREATE INDEX idx_messages_pk ON messages(id);
CREATE INDEX idx_files_pk ON files(id);
```

#### Query Optimization Indexes
```sql
-- Session queries
CREATE INDEX idx_sessions_active_created ON sessions(is_active, created_at DESC);
CREATE INDEX idx_sessions_provider_model ON sessions(provider, model, created_at DESC);

-- Message queries
CREATE INDEX idx_messages_role_created ON messages(role, created_at);
CREATE INDEX idx_messages_tool_call ON messages(tool_call_id) WHERE tool_call_id IS NOT NULL;

-- File queries
CREATE INDEX idx_files_mime_type ON files(mime_type);
CREATE INDEX idx_files_is_binary ON files(is_binary);
```

#### Composite Indexes
```sql
-- Session + message queries
CREATE INDEX idx_sessions_messages ON sessions(id, created_at DESC);
CREATE INDEX idx_messages_session_created ON messages(session_id, created_at ASC);

-- File + attachment queries
CREATE INDEX idx_files_attachments ON files(session_id, created_at DESC);
```

### Query Performance

#### Explain Plans

```sql
-- Analyze session retrieval
EXPLAIN QUERY PLAN
SELECT * FROM sessions
WHERE provider = 'openai' AND is_active = TRUE
ORDER BY created_at DESC
LIMIT 50;

-- Expected: Uses idx_sessions_provider_model
```

#### Size Optimization

```sql
-- Monitor database size
SELECT 
    name,
    SUM(pgsize) as size_bytes,
    ROUND(SUM(pgsize) / 1024.0 / 1024.0, 2) as size_mb
FROM dbstat
GROUP BY name
ORDER BY size_bytes DESC;

-- Vacuum and analyze
VACUUM;
ANALYZE;
```

## Data Access Patterns

### Repository Pattern

#### Session Repository

```go
type SessionRepository struct {
    db *sql.DB
}

func (r *SessionRepository) Create(ctx context.Context, session *Session) error {
    query := `
        INSERT INTO sessions (id, title, description, provider, model, 
                             system_prompt, temperature, max_tokens)
        VALUES (?, ?, ?, ?, ?, ?, ?, ?)
    `
    
    _, err := r.db.ExecContext(ctx, query,
        session.ID, session.Title, session.Description,
        session.Provider, session.Model, session.SystemPrompt,
        session.Temperature, session.MaxTokens,
    )
    return err
}

func (r *SessionRepository) FindByID(ctx context.Context, id string) (*Session, error) {
    query := `
        SELECT s.*, COUNT(m.id) as message_count
        FROM sessions s
        LEFT JOIN messages m ON s.id = m.session_id
        WHERE s.id = ?
        GROUP BY s.id
    `
    
    var session Session
    var messageCount int
    
    err := r.db.QueryRowContext(ctx, query, id).Scan(
        &session.ID, &session.Title, &session.Description,
        &session.Provider, &session.Model, &session.SystemPrompt,
        &session.Temperature, &session.MaxTokens, &messageCount,
    )
    
    return &session, err
}
```

#### Message Repository

```go
type MessageRepository struct {
    db *sql.DB
}

func (r *MessageRepository) Create(ctx context.Context, msg *Message) error {
    query := `
        INSERT INTO messages (id, session_id, role, content, provider, model, 
                           tool_calls, tool_call_id, attachments)
        VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)
    `
    
    toolCallsJSON, _ := json.Marshal(msg.ToolCalls)
    attachmentsJSON, _ := json.Marshal(msg.Attachments)
    
    _, err := r.db.ExecContext(ctx, query,
        msg.ID, msg.SessionID, msg.Role, msg.Content,
        msg.Provider, msg.Model, toolCallsJSON, msg.ToolCallID, attachmentsJSON,
    )
    return err
}

func (r *MessageRepository) FindBySession(ctx context.Context, 
    sessionID string, limit, offset int) ([]*Message, error) {
    
    query := `
        SELECT m.*, 
               json_group_array(
                   json_object('id', f.id, 'name', f.name, 'type', a.type)
               ) as files
        FROM messages m
        LEFT JOIN attachments a ON m.id = a.message_id
        LEFT JOIN files f ON a.file_id = f.id
        WHERE m.session_id = ?
        GROUP BY m.id
        ORDER BY m.created_at ASC
        LIMIT ? OFFSET ?
    `
    
    rows, err := r.db.QueryContext(ctx, query, sessionID, limit, offset)
    if err != nil {
        return nil, err
    }
    defer rows.Close()
    
    var messages []*Message
    for rows.Next() {
        var msg Message
        var filesJSON string
        
        if err := rows.Scan(/* ... */); err != nil {
            return nil, err
        }
        
        messages = append(messages, &msg)
    }
    
    return messages, nil
}
```

## Transaction Management

### ACID Compliance

```go
func (r *Repository) CreateSessionWithMessages(ctx context.Context, 
    session *Session, messages []*Message) error {
    
    tx, err := r.db.BeginTx(ctx, nil)
    if err != nil {
        return err
    }
    defer tx.Rollback()
    
    // Create session
    if err := r.createSessionTx(tx, session); err != nil {
        return err
    }
    
    // Create messages
    for _, msg := range messages {
        if err := r.createMessageTx(tx, msg); err != nil {
            return err
        }
    }
    
    return tx.Commit()
}
```

### Concurrent Access

```sql
-- Enable WAL mode for better concurrency
PRAGMA journal_mode = WAL;

-- Set synchronous mode for performance
PRAGMA synchronous = NORMAL;

-- Set cache size
PRAGMA cache_size = 10000;
```

## Backup and Maintenance

### Automated Maintenance

```sql
-- Daily maintenance tasks
-- 1. Clean old sessions (configurable retention)
DELETE FROM sessions 
WHERE updated_at < datetime('now', '-30 days')
  AND is_active = FALSE;

-- 2. Clean orphaned files
DELETE FROM files 
WHERE session_id NOT IN (SELECT id FROM sessions);

-- 3. Vacuum if needed
PRAGMA incremental_vacuum;
```

### Backup Strategy

```go
type BackupManager struct {
    db *sql.DB
}

func (b *BackupManager) CreateBackup(ctx context.Context, path string) error {
    backupDB, err := sql.Open("sqlite3", path)
    if err != nil {
        return err
    }
    defer backupDB.Close()
    
    _, err = b.db.ExecContext(ctx, "VACUUM INTO ?", path)
    return err
}

func (b *BackupManager) RestoreBackup(ctx context.Context, path string) error {
    // Close current DB
    // Copy backup file
    // Reopen database
    return nil
}
```

## Configuration and Setup

### Database Initialization

```go
func InitializeDatabase(dbPath string) (*sql.DB, error) {
    db, err := sql.Open("sqlite3", fmt.Sprintf("%s?_fk=true&_cache_size=10000", dbPath))
    if err != nil {
        return nil, err
    }
    
    // Enable foreign keys
    if _, err := db.Exec("PRAGMA foreign_keys = ON"); err != nil {
        return nil, err
    }
    
    // Run migrations
    if err := runMigrations(db); err != nil {
        return nil, err
    }
    
    return db, nil
}
```

### Connection Pooling

```go
func SetupConnectionPool(db *sql.DB) {
    db.SetMaxOpenConns(25)
    db.SetMaxIdleConns(25)
    db.SetConnMaxLifetime(5 * time.Minute)
}
```

## Security Considerations

### Data Protection

```sql
-- Use encryption for sensitive data
-- Store API keys encrypted
-- File content encryption for sensitive files
-- Access logging for audit trails
```

### Access Control

```go
type AccessController struct {
    db *sql.DB
}

func (a *AccessController) CheckSessionAccess(ctx context.Context, 
    sessionID, userID string) (bool, error) {
    
    // Implement user-based access control
    // For now, all sessions are accessible
    return true, nil
}
```

## Conclusion

The SQLite storage system in Crush provides a robust, performant foundation for the TUI application with:

- **Normalized schema** for data integrity
- **Optimized indexes** for query performance
- **JSON storage** for flexible metadata
- **Hierarchical relationships** for complex workflows
- **Migration system** for schema evolution
- **Concurrent access** with WAL mode
- **Comprehensive backup** and maintenance tools

The design balances simplicity with functionality, providing a solid foundation for the AI conversation management system while maintaining excellent performance characteristics for interactive TUI usage.