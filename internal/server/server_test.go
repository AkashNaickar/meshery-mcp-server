package server

import (
	"context"
	"testing"

	"github.com/mark3labs/mcp-go/client"
	"github.com/mark3labs/mcp-go/mcp"
)

func TestServerInfoTool(t *testing.T) {
	s := New()

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
	if len(toolsResp.Tools) != 1 {
		t.Fatalf("expected 1 tool, got %d", len(toolsResp.Tools))
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
	if text.Text == "" {
		t.Fatal("expected non-empty text")
	}
}
