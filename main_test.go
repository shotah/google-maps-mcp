package main

import (
	"testing"

	"github.com/shotah/google-maps-mcp/server"
	"github.com/shotah/google-maps-mcp/tools"
)

func TestRegister(t *testing.T) {
	t.Parallel()
	s := server.New()
	tools.Register(s)
	if s == nil {
		t.Fatal("server is nil")
	}
}
