package tools

import (
	"context"
	"regexp"
	"strings"
	"testing"

	"github.com/mark3labs/mcp-go/mcp"
	mcpserver "github.com/mark3labs/mcp-go/server"

	"github.com/shotah/google-maps-mcp/server"
)

func TestToolNamesLocked(t *testing.T) {
	t.Parallel()
	re := regexp.MustCompile(`^[a-z]+_[a-z]+`)
	names := ToolNames()
	if len(names) != 3 {
		t.Fatalf("ToolNames() len = %d, want 3: %v", len(names), names)
	}
	want := map[string]bool{
		"link_resolve":  true,
		"place_resolve": true,
		"route_eta":     true,
	}
	for _, name := range names {
		if !re.MatchString(name) {
			t.Errorf("tool %q does not match ^[a-z]+_[a-z]+", name)
		}
		if strings.HasPrefix(name, "maps") {
			t.Errorf("tool %q starts with server id maps", name)
		}
		if !want[name] {
			t.Errorf("unexpected tool %q", name)
		}
		delete(want, name)
	}
	for missing := range want {
		t.Errorf("missing tool %q", missing)
	}
}

func TestRegisteredToolNamesMatchCatalog(t *testing.T) {
	t.Parallel()
	s := server.New()
	Register(s)
	got := registeredToolNames(t, s)
	want := ToolNames()
	if len(got) != len(want) {
		t.Fatalf("registered %d tools %v, catalog %v", len(got), keys(got), want)
	}
	for _, name := range want {
		if !got[name] {
			t.Errorf("catalog tool %q is not registered", name)
		}
	}
}

func TestToolConstants(t *testing.T) {
	t.Parallel()
	if ToolLink != "link_resolve" {
		t.Fatalf("ToolLink = %q, want link_resolve", ToolLink)
	}
	if ToolPlace != "place_resolve" {
		t.Fatalf("ToolPlace = %q, want place_resolve", ToolPlace)
	}
	if ToolRoute != "route_eta" {
		t.Fatalf("ToolRoute = %q, want route_eta", ToolRoute)
	}
}

func registeredToolNames(t *testing.T, s *mcpserver.MCPServer) map[string]bool {
	t.Helper()
	resp := s.HandleMessage(context.Background(), []byte(`{"jsonrpc":"2.0","id":1,"method":"tools/list"}`))
	result, ok := resp.(mcp.JSONRPCResponse)
	if !ok {
		t.Fatalf("expected JSONRPCResponse, got %T", resp)
	}
	listResult, ok := result.Result.(mcp.ListToolsResult)
	if !ok {
		t.Fatalf("expected ListToolsResult, got %T", result.Result)
	}
	names := make(map[string]bool, len(listResult.Tools))
	for _, tool := range listResult.Tools {
		names[tool.Name] = true
	}
	return names
}

func keys(m map[string]bool) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	return out
}
