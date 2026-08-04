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

// Package meshery provides a client for the Meshery Server REST API.
package meshery

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

const (
	// DefaultServerURL is the default base URL of the Meshery Server REST API.
	DefaultServerURL = "http://localhost:9081"
	// DefaultTimeout is the default timeout for API requests.
	DefaultTimeout = 30 * time.Second
	// DefaultMaxRetries is the default number of retries for transient failures.
	DefaultMaxRetries = 3
	// systemVersionPath is the health-check endpoint of the Meshery Server.
	systemVersionPath = "/api/system/version"
)

// VersionInfo mirrors the Meshery Server version response
// (SystemVersion in github.com/meshery/schemas/models/v1beta1/system).
type VersionInfo struct {
	// Build is the build identifier (typically the git tag of the running binary).
	Build *string `json:"build,omitempty"`
	// Commitsha is the git commit SHA of the running service.
	Commitsha *string `json:"commitsha,omitempty"`
	// Latest is the latest available Meshery release tag.
	Latest *string `json:"latest,omitempty"`
	// Outdated is true when the running build is older than the latest release.
	Outdated *bool `json:"outdated,omitempty"`
	// ReleaseChannel is the release channel of the running binary.
	ReleaseChannel *string `json:"releaseChannel,omitempty"`
	// Version is the Meshery Cloud deployment version.
	Version *string `json:"version,omitempty"`
}

// Client is a client for the Meshery Server REST API.
type Client struct {
	baseURL    string
	apiToken   string
	httpClient *http.Client
	maxRetries int
}

// Option configures a Client.
type Option func(*Client)

// WithHTTPClient overrides the underlying HTTP client used for requests.
func WithHTTPClient(c *http.Client) Option {
	return func(cl *Client) {
		if c != nil {
			cl.httpClient = c
		}
	}
}

// WithTimeout sets the per-request timeout.
func WithTimeout(d time.Duration) Option {
	return func(cl *Client) {
		cl.httpClient.Timeout = d
	}
}

// WithMaxRetries sets the number of retries for transient failures.
func WithMaxRetries(n int) Option {
	return func(cl *Client) {
		cl.maxRetries = n
	}
}

// New creates a Client for the given Meshery Server base URL. An empty
// apiToken omits the Authorization header.
func New(baseURL, apiToken string, opts ...Option) *Client {
	if baseURL == "" {
		baseURL = DefaultServerURL
	}

	c := &Client{
		baseURL:    strings.TrimRight(baseURL, "/"),
		apiToken:   apiToken,
		httpClient: &http.Client{Timeout: DefaultTimeout},
		maxRetries: DefaultMaxRetries,
	}
	for _, opt := range opts {
		opt(c)
	}
	return c
}

// Ping verifies connectivity to the Meshery Server and returns its version
// information. It returns an error when the server is unreachable or responds
// with a non-2xx status.
func (c *Client) Ping(ctx context.Context) (*VersionInfo, error) {
	var v VersionInfo
	if err := c.do(ctx, http.MethodGet, systemVersionPath, nil, &v); err != nil {
		return nil, err
	}
	return &v, nil
}

// do performs a JSON request, retrying on transient failures. The response body
// is decoded into out when out is non-nil and the status is 2xx.
func (c *Client) do(ctx context.Context, method, path string, body, out any) error {
	var reqBody io.Reader
	if body != nil {
		b, err := json.Marshal(body)
		if err != nil {
			return fmt.Errorf("encode request body: %w", err)
		}
		reqBody = bytes.NewReader(b)
	}

	var lastErr error
	for attempt := 0; attempt <= c.maxRetries; attempt++ {
		lastErr = c.doOnce(ctx, method, path, reqBody, out)
		if lastErr == nil {
			return nil
		}
		// Do not retry client errors (4xx) or context errors.
		if !isRetryable(lastErr) {
			return lastErr
		}
		if attempt == c.maxRetries {
			break
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(backoff(attempt)):
		}
	}
	return lastErr
}

func (c *Client) doOnce(ctx context.Context, method, path string, reqBody io.Reader, out any) error {
	u, err := url.JoinPath(c.baseURL, path)
	if err != nil {
		return fmt.Errorf("build request URL: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, method, u, reqBody)
	if err != nil {
		return fmt.Errorf("create request: %w", err)
	}
	req.Header.Set("Accept", "application/json")
	if c.apiToken != "" {
		req.Header.Set("Authorization", "Bearer "+c.apiToken)
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("%s %s: %w", method, u, err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices {
		b, readErr := io.ReadAll(resp.Body)
		if readErr != nil {
			return fmt.Errorf("read error response body: %w", readErr)
		}
		return &APIError{
			StatusCode: resp.StatusCode,
			Status:     resp.Status,
			Method:     method,
			URL:        u,
			Body:       string(b),
		}
	}

	if out != nil {
		b, err := io.ReadAll(resp.Body)
		if err != nil {
			return fmt.Errorf("read response body: %w", err)
		}
		if len(b) == 0 {
			return nil
		}
		if err := json.Unmarshal(b, out); err != nil {
			return fmt.Errorf("decode response body: %w", err)
		}
	}
	return nil
}

// isRetryable reports whether err is a transient failure that can be retried.
// Transient failures are network errors and 5xx responses; 4xx errors and
// context cancellations are returned immediately.
func isRetryable(err error) bool {
	if err == nil {
		return false
	}
	if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
		return false
	}
	var apiErr *APIError
	if errors.As(err, &apiErr) {
		return apiErr.StatusCode >= http.StatusInternalServerError
	}
	return true
}

// backoff returns the wait duration for the given retry attempt.
func backoff(attempt int) time.Duration {
	return time.Duration(1<<attempt) * 100 * time.Millisecond
}

// APIError represents an unexpected (non-2xx) response from the Meshery Server.
type APIError struct {
	StatusCode int
	Status     string
	Method     string
	URL        string
	Body       string
}

// Error implements the error interface.
func (e *APIError) Error() string {
	return fmt.Sprintf("meshery api error: %s %s returned %s", e.Method, e.URL, e.Status)
}
