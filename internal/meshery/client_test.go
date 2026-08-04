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

package meshery

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

const testToken = "test-token"

func TestPingSuccess(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != systemVersionPath {
			t.Errorf("unexpected path: %q", r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"build":"v0.8.36","commitsha":"abc123","releaseChannel":"stable","latest":"v0.8.36","outdated":false}`))
	}))
	defer srv.Close()

	c := New(srv.URL, "")
	v, err := c.Ping(context.Background())
	if err != nil {
		t.Fatalf("Ping() error: %v", err)
	}
	if v.Build == nil || *v.Build != "v0.8.36" {
		t.Errorf("Build = %v, want v0.8.36", v.Build)
	}
	if v.Commitsha == nil || *v.Commitsha != "abc123" {
		t.Errorf("Commitsha = %v, want abc123", v.Commitsha)
	}
	if v.ReleaseChannel == nil || *v.ReleaseChannel != "stable" {
		t.Errorf("ReleaseChannel = %v, want stable", v.ReleaseChannel)
	}
	if v.Outdated == nil || *v.Outdated {
		t.Errorf("Outdated = %v, want false", v.Outdated)
	}
}

func TestPingServerUnreachable(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {}))
	url := srv.URL
	srv.Close()

	c := New(url, "", WithMaxRetries(0))
	_, err := c.Ping(context.Background())
	if err == nil {
		t.Fatal("Ping() expected error for unreachable server, got nil")
	}
	if !strings.Contains(err.Error(), url) {
		t.Errorf("expected error to reference server URL %q, got: %v", url, err)
	}
}

func TestPingNon2xxStatus(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, `{"error":"unauthorized"}`, http.StatusUnauthorized)
	}))
	defer srv.Close()

	c := New(srv.URL, "")
	_, err := c.Ping(context.Background())
	if err == nil {
		t.Fatal("Ping() expected error for 401, got nil")
	}
	var apiErr *APIError
	if !errors.As(err, &apiErr) {
		t.Fatalf("expected *APIError, got %T", err)
	}
	if apiErr.StatusCode != http.StatusUnauthorized {
		t.Errorf("StatusCode = %d, want 401", apiErr.StatusCode)
	}
}

func TestPingRetriesOn5xx(t *testing.T) {
	var attempts atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if attempts.Add(1) < 3 {
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"build":"v0.8.36"}`))
	}))
	defer srv.Close()

	c := New(srv.URL, "", WithMaxRetries(3))
	_, err := c.Ping(context.Background())
	if err != nil {
		t.Fatalf("Ping() error after retries: %v", err)
	}
	if got := attempts.Load(); got != 3 {
		t.Errorf("attempts = %d, want 3", got)
	}
}

func TestPingDoesNotRetry4xx(t *testing.T) {
	var attempts atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		attempts.Add(1)
		http.Error(w, "not found", http.StatusNotFound)
	}))
	defer srv.Close()

	c := New(srv.URL, "", WithMaxRetries(3))
	_, err := c.Ping(context.Background())
	if err == nil {
		t.Fatal("Ping() expected error for 404, got nil")
	}
	if got := attempts.Load(); got != 1 {
		t.Errorf("attempts = %d, want 1 (no retry on 4xx)", got)
	}
}

func TestAuthorizationHeader(t *testing.T) {
	var mu sync.Mutex
	var gotAuth string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		gotAuth = r.Header.Get("Authorization")
		mu.Unlock()
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{}`))
	}))
	defer srv.Close()

	c := New(srv.URL, testToken)
	if _, err := c.Ping(context.Background()); err != nil {
		t.Fatalf("Ping() error: %v", err)
	}

	mu.Lock()
	defer mu.Unlock()
	if gotAuth != "Bearer "+testToken {
		t.Errorf("Authorization = %q, want %q", gotAuth, "Bearer "+testToken)
	}
}

func TestNoAuthorizationHeaderWithoutToken(t *testing.T) {
	var mu sync.Mutex
	var gotAuth string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		gotAuth = r.Header.Get("Authorization")
		mu.Unlock()
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	c := New(srv.URL, "")
	if _, err := c.Ping(context.Background()); err != nil {
		t.Fatalf("Ping() error: %v", err)
	}

	mu.Lock()
	defer mu.Unlock()
	if gotAuth != "" {
		t.Errorf("Authorization = %q, want empty", gotAuth)
	}
}

func TestPingRespectsTimeout(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(2 * time.Second)
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	c := New(srv.URL, "", WithTimeout(50*time.Millisecond), WithMaxRetries(0))
	start := time.Now()
	_, err := c.Ping(context.Background())
	elapsed := time.Since(start)
	if err == nil {
		t.Fatal("Ping() expected timeout error, got nil")
	}
	if elapsed > 1*time.Second {
		t.Errorf("Ping() took %v, expected timeout much sooner", elapsed)
	}
}

func TestDefaultBaseURL(t *testing.T) {
	c := New("", "")
	if c.baseURL != DefaultServerURL {
		t.Errorf("baseURL = %q, want %q", c.baseURL, DefaultServerURL)
	}
}
