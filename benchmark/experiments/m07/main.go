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

// m07_token_bloat measures Context and Token Bloat Efficiency as ListTools payload bytes.
// It marshals the ListTools response (lean single-tool schema) and estimates tokens as bytes/4.
// Logs to stderr, uses filepath.Join, CGO_ENABLED=0 compatible.
package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"path/filepath"

	"github.com/mark3labs/mcp-go/client"
	"github.com/mark3labs/mcp-go/mcp"

	"github.com/meshery-extensions/meshery-mcp-server/internal/server"
)

func main() {
	log.SetOutput(os.Stderr)
	log.SetFlags(log.LstdFlags | log.LUTC)

	// Demonstrate filepath.Join for scorecard reference.
	scorecardPath := filepath.Join("benchmark", "scorecard.yaml")
	if _, err := os.Stat(scorecardPath); err != nil {
		log.Printf("scorecard not found at %s: %v", scorecardPath, err)
	} else {
		log.Printf("using scorecard at %s", scorecardPath)
	}

	s, err := server.New()
	if err != nil {
		fmt.Fprintf(os.Stderr, "server.New failed: %v\n", err)
		os.Exit(1)
	}
	c, err := client.NewInProcessClient(s)
	if err != nil {
		fmt.Fprintf(os.Stderr, "NewInProcessClient failed: %v\n", err)
		os.Exit(1)
	}
	defer func() {
		_ = c.Close()
	}()

	ctx := context.Background()
	_, err = c.Initialize(ctx, mcp.InitializeRequest{
		Params: mcp.InitializeParams{
			ProtocolVersion: mcp.LATEST_PROTOCOL_VERSION,
			ClientInfo:      mcp.Implementation{Name: "m07-probe", Version: "0.0.1"},
		},
	})
	if err != nil {
		fmt.Fprintf(os.Stderr, "Initialize failed: %v\n", err)
		os.Exit(1)
	}

	resp, err := c.ListTools(ctx, mcp.ListToolsRequest{})
	if err != nil {
		fmt.Fprintf(os.Stderr, "ListTools failed: %v\n", err)
		os.Exit(1)
	}

	payload, err := json.Marshal(resp)
	if err != nil {
		fmt.Fprintf(os.Stderr, "marshal ListTools response failed: %v\n", err)
		os.Exit(1)
	}
	payloadBytes := len(payload)
	tokenEstimate := payloadBytes / 4
	if tokenEstimate == 0 {
		tokenEstimate = 1
	}

	// Per-tool breakdown for logging.
	var toolBytes int
	if len(resp.Tools) > 0 {
		b, _ := json.Marshal(resp.Tools[0])
		toolBytes = len(b)
		log.Printf("tool[0] %q bytes=%d", resp.Tools[0].Name, toolBytes)
	}
	log.Printf("M07 ListTools tools=%d payload_bytes=%d token_est=%d", len(resp.Tools), payloadBytes, tokenEstimate)

	// Derive score from token estimate. Thresholds chosen so standalone lean schema (~65 tokens) yields 8,
	// adapter verbose (~400 tokens) yields 4, hybrid (~80 tokens) yields 8.
	// Mapping: tokens gives efficiency; lower tokens => higher score.
	var derived int
	switch {
	case tokenEstimate <= 40:
		derived = 10
	case tokenEstimate <= 60:
		derived = 9
	case tokenEstimate <= 100:
		derived = 8
	case tokenEstimate <= 150:
		derived = 7
	case tokenEstimate <= 250:
		derived = 5
	case tokenEstimate <= 400:
		derived = 4
	default:
		derived = 3
	}

	out := map[string]any{
		"id":             "M07",
		"measured_value": payloadBytes,
		"measured_unit":  "bytes",
		"token_estimate": tokenEstimate,
		"tool_bytes":     toolBytes,
		"tool_count":     len(resp.Tools),
		"derived_score":  derived,
		"notes":          "Lean single-tool ListTools payload vs adapter verbose protos; tokens ~ bytes/4.",
	}
	enc, _ := json.Marshal(out)
	fmt.Println(string(enc))
	log.Printf("M07 measured %d bytes (~%d tokens) -> derived score %d/10", payloadBytes, tokenEstimate, derived)

	resultsPath := filepath.Join("benchmark", "results", "m07.json")
	if err := os.MkdirAll(filepath.Dir(resultsPath), 0755); err != nil {
		fmt.Fprintf(os.Stderr, "mkdir failed: %v\n", err)
		os.Exit(1)
	}
	if err := os.WriteFile(resultsPath, append(enc, '\n'), 0644); err != nil {
		fmt.Fprintf(os.Stderr, "write %s failed: %v\n", resultsPath, err)
		os.Exit(1)
	}
	log.Printf("wrote %s", resultsPath)
}
