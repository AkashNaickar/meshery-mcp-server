package version

const (
	// Name is the name of the MCP server.
	Name = "meshery-mcp-server"
	// Version is the semantic version of the MCP server.
	Version = "v0.1.0"
)

// CommitSHA is the git commit the binary was built from. It is injected at
// build time via ldflags (see Makefile and Dockerfile).
var CommitSHA = "unknown"
