package tools

import (
	"context"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"

	"github.com/meshery-extensions/meshery-mcp-server/internal/version"
)

// Register registers all tools exposed by the Meshery MCP server.
func Register(s *server.MCPServer) {
	serverInfo := mcp.NewTool("server_info",
		mcp.WithDescription("Return metadata about the Meshery MCP server."),
	)
	s.AddTool(serverInfo, serverInfoHandler)
}

func serverInfoHandler(_ context.Context, _ mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	return mcp.NewToolResultText(version.Name + " " + version.Version), nil
}
