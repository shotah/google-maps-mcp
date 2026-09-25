package tools

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/mark3labs/mcp-go/mcp"
	mcpserver "github.com/mark3labs/mcp-go/server"

	"github.com/shotah/google-maps-mcp/server"
)

func TestMCPCallLinkResolve(t *testing.T) {
	t.Parallel()
	s := newToolServer(t)
	text, isErr := callTool(t, s, ToolLink, map[string]any{
		"urls": []any{"https://www.google.com/maps/place/Space+Needle/@47.6205,-122.3493,17z"},
	})
	if isErr {
		t.Fatalf("link_resolve error: %s", text)
	}
	if !strings.Contains(text, `"results"`) || !strings.Contains(text, `"kind":"place"`) || !strings.Contains(text, "Space Needle") {
		t.Fatalf("payload = %s", text)
	}
}

func TestMCPCallPlaceResolveMissingKey(t *testing.T) {
	t.Setenv("GOOGLE_MAPS_API_KEY", "")
	old := newClient
	t.Cleanup(func() { newClient = old })
	newClient = clientFromEnv

	s := newToolServer(t)
	text, isErr := callTool(t, s, ToolPlace, map[string]any{"queries": []any{"Seattle"}})
	if !isErr || !strings.Contains(text, "GOOGLE_MAPS_API_KEY") {
		t.Fatalf("expected missing-key teach-in, got err=%v text=%s", isErr, text)
	}
}

func newToolServer(t *testing.T) *mcpserver.MCPServer {
	t.Helper()
	s := server.New()
	Register(s)
	return s
}

func callHandlerOK(t *testing.T, handler func(context.Context, mcp.CallToolRequest) (*mcp.CallToolResult, error), args map[string]any) string {
	t.Helper()
	req := mcp.CallToolRequest{Params: mcp.CallToolParams{Arguments: args}}
	result, err := handler(context.Background(), req)
	if err != nil {
		t.Fatalf("handler returned error: %v", err)
	}
	if result.IsError {
		tc, _ := result.Content[0].(mcp.TextContent)
		t.Fatalf("handler returned tool error: %s", tc.Text)
	}
	if len(result.Content) == 0 {
		t.Fatal("handler returned empty content")
	}
	tc, ok := result.Content[0].(mcp.TextContent)
	if !ok {
		t.Fatalf("expected TextContent, got %T", result.Content[0])
	}
	return tc.Text
}

func callHandlerErr(t *testing.T, handler func(context.Context, mcp.CallToolRequest) (*mcp.CallToolResult, error), args map[string]any) string {
	t.Helper()
	req := mcp.CallToolRequest{Params: mcp.CallToolParams{Arguments: args}}
	result, err := handler(context.Background(), req)
	if err != nil {
		t.Fatalf("handler returned unexpected Go error: %v", err)
	}
	if !result.IsError {
		tc := result.Content[0].(mcp.TextContent)
		t.Fatalf("expected tool error, got success: %s", tc.Text)
	}
	tc := result.Content[0].(mcp.TextContent)
	return tc.Text
}

func callTool(t *testing.T, s *mcpserver.MCPServer, toolName string, args map[string]any) (text string, isError bool) {
	t.Helper()
	params := map[string]any{"name": toolName, "arguments": args}
	msg := map[string]any{"jsonrpc": "2.0", "id": 1, "method": "tools/call", "params": params}
	raw, err := json.Marshal(msg)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	resp := s.HandleMessage(context.Background(), raw)
	switch r := resp.(type) {
	case mcp.JSONRPCResponse:
		result, ok := r.Result.(*mcp.CallToolResult)
		if !ok {
			t.Fatalf("expected *CallToolResult, got %T", r.Result)
		}
		if len(result.Content) == 0 {
			return "", result.IsError
		}
		tc, ok := result.Content[0].(mcp.TextContent)
		if !ok {
			t.Fatalf("expected TextContent, got %T", result.Content[0])
		}
		return tc.Text, result.IsError
	case mcp.JSONRPCError:
		t.Fatalf("protocol error %d: %s", r.Error.Code, r.Error.Message)
		return "", true
	default:
		t.Fatalf("unexpected response type %T", resp)
		return "", true
	}
}
