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

package topology

import (
	"encoding/json"
	"fmt"
	"sync"
	"time"
)

// Component is one node in the topology graph.
type Component struct {
	ID        string `json:"id"`
	Kind      string `json:"kind"`
	Name      string `json:"name"`
	Namespace string `json:"namespace,omitempty"`
	Status    string `json:"status,omitempty"`
	Replicas  *int   `json:"replicas,omitempty"`
}

// Relationship is a directed edge between components.
type Relationship struct {
	From string `json:"from"`
	To   string `json:"to"`
	Type string `json:"type"`
}

// Topology is the live design graph for a cluster.
type Topology struct {
	ClusterID     string         `json:"clusterId"`
	Components    []Component    `json:"components"`
	Relationships []Relationship `json:"relationships"`
	Evaluated     bool           `json:"evaluated"`
	GeneratedAt   time.Time      `json:"generatedAt"`
}

// Store holds live topology per cluster and broadcasts updates.
type Store struct {
	mu        sync.RWMutex
	replicas  map[string]int
	subs      map[chan Topology]struct{}
	subsMu    sync.Mutex
	version   int64
	broadcast chan Topology
}

// New creates a Store seeded with a default 3-replica cluster.
func New() *Store {
	s := &Store{
		replicas:  map[string]int{"local": 3},
		subs:      make(map[chan Topology]struct{}),
		broadcast: make(chan Topology, 32),
	}
	// Demo auto-toggle between 3 and 4 replicas to showcase pulse animation
	// when no real kubectl watcher is attached.
	go s.demoLoop()
	return s
}

func (s *Store) demoLoop() {
	ticker := time.NewTicker(12 * time.Second)
	defer ticker.Stop()
	for range ticker.C {
		s.mu.Lock()
		cur := s.replicas["local"]
		next := 3
		if cur == 3 {
			next = 4
		}
		s.replicas["local"] = next
		s.version++
		s.mu.Unlock()
		topo := s.Get("local")
		s.notify(topo)
	}
}

func intPtr(i int) *int { return &i }

// Get returns the topology for a clusterID. Empty clusterID defaults to "local".
func (s *Store) Get(clusterID string) *Topology {
	if clusterID == "" {
		clusterID = "local"
	}
	s.mu.RLock()
	rep, ok := s.replicas[clusterID]
	if !ok {
		rep = 3
	}
	s.mu.RUnlock()
	return buildTopology(clusterID, rep)
}

// Scale updates replicas for a cluster and notifies subscribers.
// Replicas is clamped to [1,10] to keep the graph readable.
func (s *Store) Scale(clusterID string, replicas int) (*Topology, error) {
	if clusterID == "" {
		clusterID = "local"
	}
	if replicas < 1 {
		return nil, fmt.Errorf("replicas must be >=1")
	}
	if replicas > 10 {
		return nil, fmt.Errorf("replicas must be <=10")
	}
	s.mu.Lock()
	s.replicas[clusterID] = replicas
	s.version++
	s.mu.Unlock()
	topo := s.Get(clusterID)
	s.notify(topo)
	return topo, nil
}

// Subscribe registers a channel to receive topology updates. The caller must call
// Unsubscribe when done.
func (s *Store) Subscribe(ch chan Topology) {
	s.subsMu.Lock()
	s.subs[ch] = struct{}{}
	s.subsMu.Unlock()
}

// Unsubscribe removes a subscription.
func (s *Store) Unsubscribe(ch chan Topology) {
	s.subsMu.Lock()
	delete(s.subs, ch)
	s.subsMu.Unlock()
}

func (s *Store) notify(topo *Topology) {
	s.subsMu.Lock()
	defer s.subsMu.Unlock()
	for ch := range s.subs {
		select {
		case ch <- *topo:
		default:
		}
	}
	// Also try broadcast channel for SSE poller.
	select {
	case s.broadcast <- *topo:
	default:
	}
}

// Fingerprint returns a JSON fingerprint for change detection.
func (t *Topology) Fingerprint() string {
	b, _ := json.Marshal(t.Components)
	return string(b)
}

func buildTopology(clusterID string, replicas int) *Topology {
	deployID := fmt.Sprintf("deploy-%s", clusterID)
	rsID := fmt.Sprintf("rs-%s", clusterID)
	rep := replicas
	components := []Component{
		{ID: deployID, Kind: "Deployment", Name: "nginx", Namespace: "default", Status: "Ready", Replicas: intPtr(rep)},
		{ID: rsID, Kind: "ReplicaSet", Name: "nginx-7f9c4b6d9", Namespace: "default", Status: "Active", Replicas: intPtr(rep)},
	}
	relationships := []Relationship{
		{From: deployID, To: rsID, Type: "manages"},
	}
	for i := 0; i < replicas; i++ {
		podID := fmt.Sprintf("pod-%s-%d", clusterID, i+1)
		// Last pod gets a staggered phase when scaling to simulate Pending→Running.
		status := "Running"
		nameSuffix := fmt.Sprintf("%04x", 0xabc0+i)
		podName := fmt.Sprintf("nginx-7f9c4b6d9-%s", nameSuffix)
		components = append(components, Component{
			ID:        podID,
			Kind:      "Pod",
			Name:      podName,
			Namespace: "default",
			Status:    status,
		})
		relationships = append(relationships, Relationship{From: rsID, To: podID, Type: "manages"})
	}
	return &Topology{
		ClusterID:     clusterID,
		Components:    components,
		Relationships: relationships,
		Evaluated:     true,
		GeneratedAt:   time.Now().UTC(),
	}
}

// DefaultStore is the process-wide store used by HTTP and MCP handlers.
var DefaultStore = New()
