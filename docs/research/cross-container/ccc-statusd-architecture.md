# ccc-statusd Architecture Analysis

## Overview

ccc-statusd is a daemon that enables communication between Claude Code sessions. It uses a hybrid architecture combining:
- Unix sockets for real-time bi-directional communication
- SQLite database for persistent state
- JSON file cache for session metadata

## Communication Mechanism

### Master Unix Socket

**Location**: `~/.cache/ccc-status/daemon.sock` (or `$CCC_CACHE_DIR/daemon.sock`)

**Permissions**: `0600` (owner read/write only)

The daemon creates a single master Unix socket that all clients connect to. This is implemented in `socket_server.go`.

### Protocol

All communication uses JSON-over-newline protocol:

```go
type SocketMessage struct {
    InternalID      string `json:"internal_id,omitempty"`       // Internal injection ID
    SessionID       string `json:"session_id,omitempty"`        // Claude session ID
    Type            string `json:"type"`                        // Message type
    Timestamp       int64  `json:"timestamp,omitempty"`
    ClaudeConfigDir string `json:"claude_config_dir,omitempty"` // Session config directory
    TranscriptPath  string `json:"transcript_path,omitempty"`   // Transcript file path
    Text            string `json:"text,omitempty"`              // For prompt messages
    Name            string `json:"name,omitempty"`              // For command messages
    Query           string `json:"query,omitempty"`             // For query messages
    Code            string `json:"code,omitempty"`              // For eval messages
    Data            interface{} `json:"data,omitempty"`         // Response data
    Error           string `json:"error,omitempty"`             // Error response
}
```

### Message Types

| Type | Direction | Purpose |
|------|-----------|---------|
| `register` | Injection → Daemon | Initial registration with internal ID |
| `associate` | Injection → Daemon | Associate internal ID with session ID |
| `heartbeat` | Injection → Daemon | Keep-alive with metadata updates |
| `prompt` | CLI → Daemon → Injection | Send text prompt to session |
| `command` | CLI → Daemon → Injection | Send slash command to session |
| `eval` | CLI → Daemon → Injection | Execute JavaScript in injection |
| `query` | CLI → Daemon | Query daemon state (sessions, paths) |
| `response` | Daemon → CLI | Response to any request |

### Connection Flow

1. **Injection Client Registration**:
   ```
   Injection → Daemon: {"type":"register", "internal_id":"uuid-1", "session_id":"uuid-2"}
   ```

2. **Heartbeat (every ~30s)**:
   ```
   Injection → Daemon: {
     "type":"heartbeat",
     "internal_id":"uuid-1",
     "session_id":"uuid-2",
     "claude_config_dir":"/path/to/.claude",
     "transcript_path":"/path/to/transcript.jsonl"
   }
   ```

3. **Sending Prompt (CLI to Session)**:
   ```
   CLI → Daemon: {"type":"prompt", "session_id":"uuid-2", "text":"Hello"}
   Daemon → CLI: {"type":"response", "session_id":"uuid-2"}  // or {"error":"..."}
   Daemon → Injection: {"type":"prompt", "session_id":"uuid-2", "text":"Hello"}
   ```

## State Storage

### 1. SQLite Database

**Location**: `~/.cache/ccc-status/scheduled.db`

**Tables**:

```sql
-- Scheduled messages for future delivery
CREATE TABLE scheduled_messages (
    id TEXT PRIMARY KEY,
    session_id TEXT NOT NULL,
    message TEXT NOT NULL,
    send_at INTEGER NOT NULL,      -- Unix timestamp
    created_at INTEGER NOT NULL
);

-- Session metadata (from heartbeats)
CREATE TABLE session_metadata (
    session_id TEXT PRIMARY KEY,
    claude_config_dir TEXT NOT NULL,
    transcript_path TEXT,
    first_seen INTEGER NOT NULL,
    last_seen INTEGER NOT NULL
);

-- Context window usage stats (from statusline)
CREATE TABLE context_window (
    session_id TEXT PRIMARY KEY,
    context_window_size INTEGER NOT NULL,
    used_percentage REAL NOT NULL,
    remaining_percentage REAL NOT NULL,
    total_input_tokens INTEGER NOT NULL,
    total_output_tokens INTEGER NOT NULL,
    cache_read_input_tokens INTEGER,
    cache_creation_input_tokens INTEGER,
    current_input_tokens INTEGER,
    current_output_tokens INTEGER,
    updated_at INTEGER NOT NULL
);
```

**Configuration**:
- WAL mode for concurrent reads during writes
- Busy timeout: 5000ms
- Max 1 connection (serialized writes)

### 2. JSON File Cache

**Location**: `~/.cache/ccc-status/<session-id>.json`

**Purpose**: Fast session metadata access without database queries

**Structure** (in `internal/cache/models.go`):
- Session ID, working directory, git branch
- Model info, cost, message counts
- Transcript path, config directory
- Timestamps (created, updated, last user submit)

### 3. In-Memory State

In `socket_server.go`:
```go
type SocketServer struct {
    clients           map[string]*ClientConnection // internalID -> connection
    sessionToInternal map[string]string            // sessionID -> internalID mapping
}
```

## Key Code Paths

### Session Send/Receive

**CLI sends prompt** (`control_api.go:SendPromptViaSocket`):
1. Resolve short session ID to full UUID via cache file lookup
2. Connect to `daemon.sock` Unix socket
3. Send JSON: `{"type":"prompt", "session_id":"...", "text":"..."}`
4. Wait for response JSON
5. Handle error or success

**Daemon routes message** (`socket_server.go:handleConnection`):
1. Parse incoming JSON message
2. For `prompt`/`command`/`eval` types:
   - Look up `internalID` from `sessionToInternal` map
   - Find `ClientConnection` from `clients` map
   - Write JSON to injection's connection
   - Return response to CLI

**Injection receives** (JavaScript injection client):
- Reads JSON from socket
- For `prompt`: simulates user input
- For `command`: executes slash command
- For `eval`: runs JavaScript code

### Session Discovery

**Path resolution priority** (`internal/sessions/discovery.go:FindSessionPath`):
1. Check database `transcript_path` (fastest, most reliable)
2. Check `CCC_PATH_HOME` env for recursive search
3. Check cache file for `ClaudeConfigDir`
4. Check database for `ClaudeConfigDir`
5. Fall back to `CLAUDE_CONFIG_DIR` or `~/.claude`

### Daemon Startup (`daemon.go:Run`)

1. Create cache directory
2. Write PID file to `daemon.pid`
3. Set up signal handlers (SIGTERM, SIGINT, SIGHUP)
4. Initialize socket server at `daemon.sock`
5. Initialize watchers (transcript, git, tmux, config)
6. Initialize scheduler worker
7. Discover existing sessions
8. Start background cleanup goroutines
9. Enter main event loop

## Socket Path and Permissions

| Path | Purpose | Permissions |
|------|---------|-------------|
| `~/.cache/ccc-status/daemon.sock` | Master communication socket | 0600 |
| `~/.cache/ccc-status/daemon.pid` | Process ID file | default |
| `~/.cache/ccc-status/daemon.log` | Daemon log file | default |
| `~/.cache/ccc-status/scheduled.db` | SQLite database | default |
| `~/.cache/ccc-status/*.json` | Session cache files | default |

**Path Resolution** (`internal/util/paths.go`):
1. `$CCC_CACHE_DIR` environment variable (if set)
2. `$XDG_CACHE_HOME/ccc-status` (XDG standard)
3. `~/.cache/ccc-status` (XDG default)
4. `/tmp/ccc-status` (fallback)

## Session Management

### Stale Connection Cleanup

- Periodic cleanup runs every 60 seconds
- Sessions without heartbeat for 90 seconds are considered stale
- Stale connections are closed and removed from maps

### Message Routing

Daemon maintains two mappings:
- `sessionToInternal`: Maps Claude session UUID to internal injection UUID
- `clients`: Maps internal UUID to socket connection

This double-mapping allows:
- CLI to send using session ID (user-facing)
- Internal routing using stable injection ID
- Sessions can reconnect with same session ID

## Query Handlers

The daemon supports queries via socket (`daemon.go:handleQuery`):

| Query | Purpose |
|-------|---------|
| `session_path` | Get transcript path for session ID |
| `list_sessions` | List all sessions from transcript watcher |
| `active_sessions` | List sessions with active socket connections |
| `active_connections` | List all connections with metadata |
| `reload_schedule` | Reload scheduled messages from DB |

## Summary

ccc-statusd enables inter-session communication through:
1. **Unix socket** for real-time bidirectional messaging
2. **SQLite** for persistent scheduling and session metadata
3. **JSON cache** for fast session lookups
4. **In-memory maps** for connection routing

The architecture supports:
- Multiple concurrent sessions
- Session reconnection
- Message scheduling
- Multi-profile configurations
- Graceful degradation when daemon is unavailable
