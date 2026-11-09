// Package build holds version metadata injected at link time.
package build

// Version is overridden with -ldflags "-X .../internal/build.Version=...".
var (
	Version = "dev"
	Commit  = "none"
	Date    = "unknown"
)
