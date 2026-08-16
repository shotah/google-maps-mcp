// Package tools registers link_resolve, place_resolve, place_search, and route_eta.
package tools

import (
	"github.com/mark3labs/mcp-go/mcp"
	mcpserver "github.com/mark3labs/mcp-go/server"
)

// Tool names — service_verb_object, no server-id prefix.
// Host mcp.toml name is maps → maps__link_resolve, maps__place_resolve, …
// See https://github.com/shotah/ai-gantry/blob/main/docs/mcp-naming.md
const (
	ToolLink   = "link_resolve"
	ToolPlace  = "place_resolve"
	ToolSearch = "place_search"
	ToolRoute  = "route_eta"
)

// Kind values returned by link_resolve.
const (
	KindPlace      = "place"
	KindDirections = "directions"
	KindSearch     = "search"
	KindView       = "view"
	KindUnknown    = "unknown"
)

const (
	nextLinkResolve  = `Next: link_resolve(url="https://maps.app.goo.gl/…")`
	nextPlaceResolve = `Next: place_resolve(query="Space Needle")`
	nextPlaceSearch  = `Next: place_search(query="sushi restaurants", near="Ballard")`
	nextRouteETA     = `Next: route_eta(origin="Seattle", destination="Portland")`
)

// ToolNames is the registered catalog (tests lock naming).
func ToolNames() []string {
	return []string{ToolLink, ToolPlace, ToolSearch, ToolRoute}
}

// Register attaches all tools to s.
func Register(s *mcpserver.MCPServer) {
	registerLink(s)
	registerPlace(s)
	registerSearch(s)
	registerRoute(s)
}

func registerTool(s *mcpserver.MCPServer, tool mcp.Tool, handler mcpserver.ToolHandlerFunc) {
	s.AddTool(tool, handler)
}
