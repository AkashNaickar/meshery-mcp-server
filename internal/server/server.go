package server

import (
	"github.com/mark3labs/mcp-go/server"

	"github.com/meshery-extensions/meshery-mcp-server/internal/tools"
	"github.com/meshery-extensions/meshery-mcp-server/internal/version"
)

// New creates an MCP server with all registered tools.
func New() *server.MCPServer {
	s := server.NewMCPServer(version.Name, version.Version)
	tools.Register(s)
	return s
}

// Serve runs the MCP server over stdio until a client disconnects or the
// process is interrupted.
func Serve(s *server.MCPServer) error {
	return server.ServeStdio(s)
}
