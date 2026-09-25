package tools

import (
	"encoding/json"
	"fmt"

	"github.com/mark3labs/mcp-go/mcp"
)

const maxBatch = 8

func rejectSingular(request mcp.CallToolRequest, singular, plural, next string) error {
	if _, ok := request.GetArguments()[singular]; ok {
		return fmt.Errorf("%s is not a parameter. Pass %s (1–%d). %s", singular, plural, maxBatch, next)
	}
	return nil
}

func requireStrings(request mcp.CallToolRequest, key, next string) ([]string, error) {
	items, err := request.RequireStringSlice(key)
	if err != nil || len(items) == 0 {
		return nil, fmt.Errorf("%s is required. %s", key, next)
	}
	if len(items) > maxBatch {
		return nil, fmt.Errorf("%s accepts at most %d items. %s", key, maxBatch, next)
	}
	return items, nil
}

func requireObjects(request mcp.CallToolRequest, key, next string) ([]map[string]any, error) {
	args := request.GetArguments()
	val, ok := args[key]
	if !ok {
		return nil, fmt.Errorf("%s is required. %s", key, next)
	}
	var raw []any
	switch v := val.(type) {
	case []any:
		raw = v
	case []map[string]any:
		out := make([]map[string]any, len(v))
		copy(out, v)
		return checkBatchLen(key, next, out)
	default:
		return nil, fmt.Errorf("%s must be a list of objects. %s", key, next)
	}
	if len(raw) == 0 {
		return nil, fmt.Errorf("%s is required. %s", key, next)
	}
	out := make([]map[string]any, 0, len(raw))
	for i, item := range raw {
		obj, ok := item.(map[string]any)
		if !ok {
			return nil, fmt.Errorf("%s item %d must be an object. %s", key, i, next)
		}
		out = append(out, obj)
	}
	return checkBatchLen(key, next, out)
}

func checkBatchLen(key, next string, items []map[string]any) ([]map[string]any, error) {
	if len(items) == 0 {
		return nil, fmt.Errorf("%s is required. %s", key, next)
	}
	if len(items) > maxBatch {
		return nil, fmt.Errorf("%s accepts at most %d items. %s", key, maxBatch, next)
	}
	return items, nil
}

func strField(obj map[string]any, key string) string {
	s, _ := obj[key].(string)
	return s
}

func batchText(results any) (*mcp.CallToolResult, error) {
	b, err := json.Marshal(struct {
		Results any `json:"results"`
	}{Results: results})
	if err != nil {
		return mcp.NewToolResultError(err.Error()), nil
	}
	return mcp.NewToolResultText(string(b)), nil
}
