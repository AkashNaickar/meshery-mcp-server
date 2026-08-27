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
	"embed"
	"errors"
	"fmt"
	"io/fs"
	"log"
	"net/http"
	"time"

	"github.com/mark3labs/mcp-go/server"

	"github.com/meshery-extensions/meshery-mcp-server/internal/config"
	"github.com/meshery-extensions/meshery-mcp-server/internal/resources"
	"github.com/meshery-extensions/meshery-mcp-server/internal/tools"
	"github.com/meshery-extensions/meshery-mcp-server/internal/topology"
	"github.com/meshery-extensions/meshery-mcp-server/internal/version"
)

//go:embed web
var webFS embed.FS

// New creates an MCP server with all registered MCP surfaces.
func New() (*server.MCPServer, error) {
	s := server.NewMCPServer(version.Name, version.Version,
		server.WithHooks(&server.Hooks{}),
		server.WithResourceCapabilities(true, false),
	)

	store := topology.DefaultStore
	registry := NewRegistry(
		Named("tools", RegistrantFunc(func(server *server.MCPServer) error {
			tools.Register(server)
			return nil
		})),
		Named("resources", RegistrantFunc(func(server *server.MCPServer) error {
			return resources.Register(server, store)
		})),
	)

	if err := registry.RegisterAll(s); err != nil {
		return nil, err
	}

	return s, nil
}

// Serve runs the MCP server over the transport selected in cfg.
func Serve(s *server.MCPServer, cfg *config.Config) error {
	switch cfg.Transport {
	case "stdio":
		return server.ServeStdio(s)
	case "http", "sse":
		return serveHTTP(s, cfg)
	default:
		return fmt.Errorf("unsupported transport %q (want stdio, http, or sse)", cfg.Transport)
	}
}

func serveHTTP(s *server.MCPServer, cfg *config.Config) error {
	mcpHandler := server.NewStreamableHTTPServer(s)
	store := topology.DefaultStore

	// mux serves health, bench dashboard, and MCP at /mcp
	mux := http.NewServeMux()
	mux.Handle("/mcp", mcpHandler)
	mux.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"status":"ok"}`))
	})
	// Topology REST + SSE for the live graph.
	mux.HandleFunc("/api/topology", topologyHandlerHTTP(store))
	mux.HandleFunc("/api/topology/stream", topologyStreamHandler(store))
	mux.HandleFunc("/api/topology/scale", topologyScaleHandler(store))
	mux.HandleFunc("/api/topology/", topologyHandlerHTTP(store))
	// Serve bench dashboard from embedded web/bench when present, fallback to filesystem for local dev.
	if benchFS, err := fs.Sub(webFS, "web/bench"); err == nil {
		mux.Handle("/bench/", http.StripPrefix("/bench/", http.FileServer(http.FS(benchFS))))
	} else {
		mux.Handle("/bench/", http.StripPrefix("/bench/", http.FileServer(http.Dir("web/bench"))))
	}
	// Serve the playground/topology graph at /.
	if rootFS, err := fs.Sub(webFS, "web"); err == nil {
		mux.Handle("/", http.FileServer(http.FS(rootFS)))
	} else {
		mux.Handle("/", http.FileServer(http.Dir("web")))
	}

	httpServer := &http.Server{
		Addr:    cfg.HTTPAddr,
		Handler: mux,
	}

	errCh := make(chan error, 1)
	go func() {
		log.Printf("serving MCP over streamable HTTP on %s", cfg.HTTPAddr)
		errCh <- httpServer.ListenAndServe()
	}()

	ctx, stop := signalContext()
	defer stop()

	select {
	case err := <-errCh:
		if errors.Is(err, context.Canceled) {
			return nil
		}
		return err
	case <-ctx.Done():
		log.Printf("shutting down MCP HTTP server")
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		return httpServer.Shutdown(shutdownCtx)
	}
}
