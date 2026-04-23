package mcpserver

// This file previously contained the Session type, sessionState enum, and SessionStore.
// Those types have been moved to internal/sessionstore:
//   - sessionstore.Session   (the session struct)
//   - sessionstore.State     (string enum: loaded, scored, tailored, finalized)
//   - sessionstore.Store     (interface: Create, Get, Update, Delete)
//   - sessionstore.MemoryStore (in-memory LRU, used by the MCP server)
//   - sessionstore.DiskStore   (file-per-session JSON, used by CLI subcommands)
//
// The package-level sessions variable and handler functions are in session_tools.go.
