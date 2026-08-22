package main

import (
	"fmt"
	"log"
	"os"

	mcpserver "github.com/mark3labs/mcp-go/server"

	"github.com/shotah/google-maps-mcp/server"
	"github.com/shotah/google-maps-mcp/tools"
)

func main() {
	if len(os.Args) > 1 && os.Args[1] == "host-manifest" {
		if err := writeHostManifest(os.Stdout); err != nil {
			fmt.Fprintf(os.Stderr, "error: %v\n", err)
			os.Exit(1)
		}
		return
	}
	if err := run(); err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}
}

func run() error {
	s := server.New()
	tools.Register(s)
	errLogger := log.New(os.Stderr, "", log.LstdFlags)
	return mcpserver.ServeStdio(s, mcpserver.WithErrorLogger(errLogger))
}
