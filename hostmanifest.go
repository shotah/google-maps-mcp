package main

import (
	"encoding/json"
	"io"
)

func writeHostManifest(w io.Writer) error {
	return json.NewEncoder(w).Encode(map[string]any{
		"name":     "maps",
		"command":  "google-maps-mcp",
		"env_keys": []string{"GOOGLE_MAPS_API_KEY"},
		"blurb":    "Places / ETA. One Maps key.",
	})
}
