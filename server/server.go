// Package server initializes the MCP server.
package server

import (
	mcpserver "github.com/mark3labs/mcp-go/server"
)

// ServerName is the host mcp.toml id. Tools are exposed as maps__link_resolve.
const ServerName = "maps"

// ServerVersion is overwritten at link time by GoReleaser / `make cli`
// (-X github.com/shotah/google-maps-mcp/server.ServerVersion=...).
var ServerVersion = "0.1.0"

// New creates a stdio MCP server with tool capabilities.
func New() *mcpserver.MCPServer {
	return mcpserver.NewMCPServer(
		ServerName,
		ServerVersion,
		mcpserver.WithToolCapabilities(true),
	)
}
