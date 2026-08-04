package main

import (
	"log"
	"os"

	"github.com/meshery-extensions/meshery-mcp-server/internal/config"
	"github.com/meshery-extensions/meshery-mcp-server/internal/server"
	"github.com/meshery-extensions/meshery-mcp-server/internal/version"
)

func main() {
	log.SetFlags(log.LstdFlags | log.LUTC)
	log.SetOutput(os.Stderr)

	cfg := config.Load()
	log.Printf("starting %s %s (Meshery Server: %s)", version.Name, version.Version, cfg.MeshServerURL)

	srv := server.New()
	if err := server.Serve(srv); err != nil {
		log.Fatalf("serve MCP server: %v", err)
	}
}
