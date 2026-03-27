package version

import "fmt"

// Version information, set via ldflags at build time.
var (
	Version   = "dev"
	BuildDate = "unknown"
	GitCommit = "unknown"
)

// Repository information for update checks.
const (
	RepoOwner = "young1lin"
	RepoName  = "port-bridge"
)

// ShortVersion returns the version with a "v" prefix (e.g. "v1.0.0").
func ShortVersion() string {
	return "v" + Version
}

// FullVersionString returns a detailed version string.
func FullVersionString() string {
	return fmt.Sprintf("PortBridge %s\nCommit: %s\nBuilt: %s", ShortVersion(), GitCommit, BuildDate)
}
