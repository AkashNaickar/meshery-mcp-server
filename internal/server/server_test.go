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

package server

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/mark3labs/mcp-go/client"
	"github.com/mark3labs/mcp-go/mcp"

	"github.com/meshery-extensions/meshery-mcp-server/internal/config"
	"github.com/meshery-extensions/meshery-mcp-server/internal/version"
)

func TestServerInfoTool(t *testing.T) {
	s := New(&config.Config{})

	mcpClient, err := client.NewInProcessClient(s)
	if err != nil {
		t.Fatalf("new in-process client: %v", err)
	}
	defer func() {
		if err := mcpClient.Close(); err != nil {
			t.Errorf("close in-process client: %v", err)
		}
	}()

	ctx := context.Background()

	_, err = mcpClient.Initialize(ctx, mcp.InitializeRequest{
		Params: mcp.InitializeParams{
			ProtocolVersion: mcp.LATEST_PROTOCOL_VERSION,
			ClientInfo:      mcp.Implementation{Name: "test-client", Version: "0.0.1"},
		},
	})
	if err != nil {
		t.Fatalf("initialize: %v", err)
	}

	toolsResp, err := mcpClient.ListTools(ctx, mcp.ListToolsRequest{})
	if err != nil {
		t.Fatalf("list tools: %v", err)
	}
	if len(toolsResp.Tools) != 2 {
		t.Fatalf("expected 2 tools, got %d", len(toolsResp.Tools))
	}

	result, err := mcpClient.CallTool(ctx, mcp.CallToolRequest{
		Params: mcp.CallToolParams{
			Name: "server_info",
		},
	})
	if err != nil {
		t.Fatalf("call tool: %v", err)
	}
	if len(result.Content) == 0 {
		t.Fatal("expected content in result")
	}
	text, ok := mcp.AsTextContent(result.Content[0])
	if !ok || text == nil {
		t.Fatalf("expected text content, got %T", result.Content[0])
	}
	expected := fmt.Sprintf("%s %s (commit %s)", version.Name, version.Version, version.CommitSHA)
	if text.Text != expected {
		t.Fatalf("expected %q, got %q", expected, text.Text)
	}
}

func TestMesheryPingTool(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"build":"v0.8.36"}`))
	}))
	defer srv.Close()

	s := New(&config.Config{MeshServerURL: srv.URL})

	mcpClient, err := client.NewInProcessClient(s)
	if err != nil {
		t.Fatalf("new in-process client: %v", err)
	}
	defer func() {
		if err := mcpClient.Close(); err != nil {
			t.Errorf("close in-process client: %v", err)
		}
	}()

	ctx := context.Background()
	_, err = mcpClient.Initialize(ctx, mcp.InitializeRequest{
		Params: mcp.InitializeParams{
			ProtocolVersion: mcp.LATEST_PROTOCOL_VERSION,
			ClientInfo:      mcp.Implementation{Name: "test-client", Version: "0.0.1"},
		},
	})
	if err != nil {
		t.Fatalf("initialize: %v", err)
	}

	result, err := mcpClient.CallTool(ctx, mcp.CallToolRequest{
		Params: mcp.CallToolParams{
			Name: "meshery_ping",
		},
	})
	if err != nil {
		t.Fatalf("call tool: %v", err)
	}
	if len(result.Content) == 0 {
		t.Fatal("expected content in result")
	}
	text, ok := mcp.AsTextContent(result.Content[0])
	if !ok || text == nil {
		t.Fatalf("expected text content, got %T", result.Content[0])
	}
	if text.Text != "Meshery Server reachable (build v0.8.36)" {
		t.Errorf("unexpected ping output: %q", text.Text)
	}
}
