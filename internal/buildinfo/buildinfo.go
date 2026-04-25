// Package buildinfo exposes metadata injected at build time.
package buildinfo

// Build metadata defaults. Release builds override these with -ldflags.
var (
	Version   = "dev"
	Commit    = "unknown"
	BuildDate = "unknown"
)
