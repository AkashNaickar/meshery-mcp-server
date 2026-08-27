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
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"time"

	"github.com/meshery-extensions/meshery-mcp-server/internal/topology"
)

func topologyHandlerHTTP(store *topology.Store) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		clusterID := r.URL.Query().Get("clusterId")
		if clusterID == "" {
			clusterID = r.URL.Query().Get("cluster_id")
		}
		if clusterID == "" {
			clusterID = "local"
		}
		topo := store.Get(clusterID)
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("Access-Control-Allow-Origin", "*")
		_ = json.NewEncoder(w).Encode(topo)
	}
}

func topologyStreamHandler(store *topology.Store) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		clusterID := r.URL.Query().Get("clusterId")
		if clusterID == "" {
			clusterID = r.URL.Query().Get("cluster_id")
		}
		if clusterID == "" {
			clusterID = "local"
		}

		w.Header().Set("Content-Type", "text/event-stream")
		w.Header().Set("Cache-Control", "no-cache")
		w.Header().Set("Connection", "keep-alive")
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("X-Accel-Buffering", "no")

		flusher, ok := w.(http.Flusher)
		if !ok {
			http.Error(w, "streaming unsupported", http.StatusInternalServerError)
			return
		}

		// Send initial snapshot.
		if err := writeSSE(w, "topology", store.Get(clusterID)); err != nil {
			return
		}
		flusher.Flush()

		ch := make(chan topology.Topology, 8)
		store.Subscribe(ch)
		defer store.Unsubscribe(ch)

		ticker := time.NewTicker(15 * time.Second)
		defer ticker.Stop()

		// Also poll for demoLoop changes that use notify via channel but also
		// via direct fingerprint comparison for clients that missed notify.
		pollTicker := time.NewTicker(2 * time.Second)
		defer pollTicker.Stop()

		lastFP := store.Get(clusterID).Fingerprint()

		for {
			select {
			case <-r.Context().Done():
				return
			case topo := <-ch:
				if topo.ClusterID != clusterID {
					continue
				}
				_ = writeSSE(w, "topology", &topo)
				flusher.Flush()
				lastFP = topo.Fingerprint()
			case <-pollTicker.C:
				current := store.Get(clusterID)
				fp := current.Fingerprint()
				if fp != lastFP {
					_ = writeSSE(w, "topology", current)
					flusher.Flush()
					lastFP = fp
				}
			case <-ticker.C:
				// Heartbeat comment to keep connection alive.
				_, _ = fmt.Fprintf(w, ": heartbeat %d\n\n", time.Now().Unix())
				flusher.Flush()
			}
		}
	}
}

func topologyScaleHandler(store *topology.Store) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Access-Control-Allow-Methods", "POST, OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type")
		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		if r.Method != http.MethodPost {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		var req struct {
			ClusterID string `json:"clusterId"`
			ClusterId string `json:"cluster_id"`
			Replicas  int    `json:"replicas"`
		}
		// Support both JSON body and query param for convenience.
		if r.Header.Get("Content-Type") == "application/json" || r.ContentLength > 0 {
			_ = json.NewDecoder(r.Body).Decode(&req)
		}
		if req.Replicas == 0 {
			if v := r.URL.Query().Get("replicas"); v != "" {
				if n, err := strconv.Atoi(v); err == nil {
					req.Replicas = n
				}
			}
		}
		clusterID := req.ClusterID
		if clusterID == "" {
			clusterID = req.ClusterId
		}
		if clusterID == "" {
			clusterID = r.URL.Query().Get("clusterId")
		}
		if clusterID == "" {
			clusterID = "local"
		}
		if req.Replicas == 0 {
			http.Error(w, `{"error":"replicas required (1-10)"}`, http.StatusBadRequest)
			return
		}
		topo, err := store.Scale(clusterID, req.Replicas)
		if err != nil {
			http.Error(w, fmt.Sprintf(`{"error":%q}`, err.Error()), http.StatusBadRequest)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(topo)
	}
}

func writeSSE(w http.ResponseWriter, event string, payload any) error {
	data, err := json.Marshal(payload)
	if err != nil {
		return err
	}
	_, err = fmt.Fprintf(w, "event: %s\ndata: %s\n\n", event, string(data))
	return err
}
