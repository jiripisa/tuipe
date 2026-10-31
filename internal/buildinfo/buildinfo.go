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

// Effective returns the version to show and compare. For plain `dev` builds
// it falls back to the module version, so `go install` builds report theirs.
func Effective() string {
	if Version == "dev" {
		if info, ok := debug.ReadBuildInfo(); ok && info.Main.Version != "" && info.Main.Version != "(devel)" {
			return info.Main.Version
		}
	}
	return Version
}

// String returns a one-line description of the build.
func String() string {
	return fmt.Sprintf("tuipe %s (commit %s, built %s)", Effective(), Commit, Date)
}
