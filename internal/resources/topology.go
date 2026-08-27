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

// Package resources implements the MCP resource surface.
package resources

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"regexp"
	"sync"
	"time"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"

	"github.com/meshery-extensions/meshery-mcp-server/internal/topology"
)

const topologyURITemplate = "meshery://clusters/{cluster_id}/topology"

var clusterIDPattern = regexp.MustCompile(`^meshery://clusters/([^/]+)/topology$`)

type subscriptionTracker struct {
	mu   sync.Mutex
	uris map[string]struct{}
}

func newSubscriptionTracker() *subscriptionTracker {
	return &subscriptionTracker{uris: make(map[string]struct{})}
}

func (t *subscriptionTracker) subscribe(uri string) {
	t.mu.Lock()
	t.uris[uri] = struct{}{}
	t.mu.Unlock()
}

func (t *subscriptionTracker) unsubscribe(uri string) {
	t.mu.Lock()
	delete(t.uris, uri)
	t.mu.Unlock()
}

func (t *subscriptionTracker) snapshot() []string {
	t.mu.Lock()
	defer t.mu.Unlock()
	out := make([]string, 0, len(t.uris))
	for uri := range t.uris {
		out = append(out, uri)
	}
	return out
}

// Register registers the topology resource template and subscription tracking.
func Register(s *server.MCPServer, store *topology.Store) error {
	if store == nil {
		store = topology.DefaultStore
	}
	tracker := newSubscriptionTracker()

	tmpl := mcp.NewResourceTemplate(topologyURITemplate, "Cluster Topology",
		mcp.WithTemplateTitle("Live cluster topology"),
		mcp.WithTemplateDescription("The live Kubernetes topology (Deployment → ReplicaSet → Pods) discovered for a cluster. Use with resources/subscribe to receive resources/updated when pods scale."),
		mcp.WithTemplateMIMEType("application/json"),
	)
	s.AddResourceTemplate(tmpl, topologyHandler(store))

	// Track subscribe/unsubscribe to know which URIs to poll for notifications.
	hooks := s.GetHooks()
	if hooks != nil {
		hooks.AddBeforeAny(func(_ context.Context, _ any, method mcp.MCPMethod, message any) {
			switch method {
			case mcp.MethodResourcesSubscribe:
				if req, ok := message.(*mcp.SubscribeRequest); ok && req.Params.URI != "" {
					tracker.subscribe(req.Params.URI)
				}
			case mcp.MethodResourcesUnsubscribe:
				if req, ok := message.(*mcp.UnsubscribeRequest); ok && req.Params.URI != "" {
					tracker.unsubscribe(req.Params.URI)
				}
			}
		})
	}

	go pollTopologyUpdates(s, store, tracker)
	return nil
}

func topologyHandler(store *topology.Store) server.ResourceTemplateHandlerFunc {
	return func(_ context.Context, req mcp.ReadResourceRequest) ([]mcp.ResourceContents, error) {
		clusterID, err := parseClusterID(req.Params.URI)
		if err != nil {
			return nil, err
		}
		topo := store.Get(clusterID)
		payload, err := json.MarshalIndent(topo, "", "  ")
		if err != nil {
			return nil, fmt.Errorf("encode topology: %w", err)
		}
		return []mcp.ResourceContents{
			mcp.TextResourceContents{
				URI:      req.Params.URI,
				MIMEType: "application/json",
				Text:     string(payload),
			},
		}, nil
	}
}

func parseClusterID(uri string) (string, error) {
	m := clusterIDPattern.FindStringSubmatch(uri)
	if m == nil {
		return "", fmt.Errorf("unsupported resource URI %q", uri)
	}
	return m[1], nil
}

func pollTopologyUpdates(s *server.MCPServer, store *topology.Store, tracker *subscriptionTracker) {
	interval := 2 * time.Second
	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	fingerprints := make(map[string]string)

	for range ticker.C {
		for _, uri := range tracker.snapshot() {
			clusterID, err := parseClusterID(uri)
			if err != nil {
				continue
			}
			topo := store.Get(clusterID)
			encoded, _ := json.Marshal(topo.Components)
			fp := string(encoded)
			if prev, ok := fingerprints[uri]; !ok {
				fingerprints[uri] = fp
				continue
			} else if prev == fp {
				continue
			}
			fingerprints[uri] = fp
			log.Printf("topology updated %s replicas=%d", uri, len(topo.Components)-2)
			s.SendNotificationToAllClients(mcp.MethodNotificationResourceUpdated, map[string]any{
				"uri": uri,
			})
		}
	}
}
