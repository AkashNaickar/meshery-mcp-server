package config

import "os"

const (
	// DefaultMeshServerURL is the default base URL of the Meshery Server REST API.
	DefaultMeshServerURL = "http://localhost:9081"
)

// Config holds runtime configuration for the Meshery MCP server.
type Config struct {
	// MeshServerURL is the base URL of the Meshery Server REST API.
	MeshServerURL string
	// MeshAPIToken is an optional token used to authenticate with the Meshery Server API.
	MeshAPIToken string
}

// Load reads configuration from the environment, applying defaults where unset.
func Load() *Config {
	return &Config{
		MeshServerURL: envOr("MESHERY_SERVER_URL", DefaultMeshServerURL),
		MeshAPIToken:  os.Getenv("MESHERY_API_TOKEN"),
	}
}

func envOr(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}
