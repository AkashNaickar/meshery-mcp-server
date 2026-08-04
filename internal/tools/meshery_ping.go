// Copyright Meshery Authors
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package tools

import (
	"context"
	"fmt"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"

	"github.com/meshery-extensions/meshery-mcp-server/internal/meshery"
)

// Register registers all tools exposed by the Meshery MCP server.
// The provided client is shared by every tool that talks to the Meshery Server.
func Register(s *server.MCPServer, mc *meshery.Client) {
	serverInfo := mcp.NewTool("server_info",
		mcp.WithDescription("Return metadata about the Meshery MCP server."),
	)
	s.AddTool(serverInfo, serverInfoHandler)

	ping := mcp.NewTool("meshery_ping",
		mcp.WithDescription("Check connectivity to the configured Meshery Server and return its version."),
	)
	s.AddTool(ping, mesheryPingHandler(mc))
}

func mesheryPingHandler(mc *meshery.Client) server.ToolHandlerFunc {
	return func(ctx context.Context, _ mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		v, err := mc.Ping(ctx)
		if err != nil {
			return nil, err
		}
		build := "unknown"
		if v.Build != nil {
			build = *v.Build
		}
		return mcp.NewToolResultText(fmt.Sprintf("Meshery Server reachable (build %s)", build)), nil
	}
}
