// Package buildinfo exposes the version metadata of the running binary.
package buildinfo

import (
	"fmt"
	"runtime/debug"
)

// Set by ldflags at release time.
var (
	Version = "dev"
	Commit  = "none"
	Date    = "unknown"
)

// IsRelease reports whether the binary was built with a release version.
func IsRelease() bool {
	return Version != "dev"
}

// String returns a one-line description of the build. For plain `dev` builds
// it falls back to the module version, so `go install` builds report theirs.
func String() string {
	version := Version
	if version == "dev" {
		if info, ok := debug.ReadBuildInfo(); ok && info.Main.Version != "" && info.Main.Version != "(devel)" {
			version = info.Main.Version
		}
	}
	return fmt.Sprintf("tuipe %s (commit %s, built %s)", version, Commit, Date)
}
