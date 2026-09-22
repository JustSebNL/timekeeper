// Package version reports the build identity of the running binary.
//
// It uses Go's embedded build info rather than a hardcoded string, so
// the reported version always matches the code that is actually running.
package version

import (
	_ "embed"
	"runtime/debug"
	"strings"
)

//go:embed VERSION
var versionFile []byte

// Info describes a build.
type Info struct {
	// Version is the semantic version, set at build time via
	// -ldflags "-X .../version.Version=x.y.z". Empty when not stamped.
	Version string
	// Commit is the git commit hash the binary was built from.
	Commit string
	// Date is the build time, from VCS info when available.
	Date string
	// GoVersion is the Go toolchain used.
	GoVersion string
}

// build is populated from ReadBuildInfo.
var build Info

func init() {
	info, ok := debug.ReadBuildInfo()
	if !ok {
		return
	}
	build.GoVersion = info.GoVersion
	for _, setting := range info.Settings {
		switch setting.Key {
		case "vcs.revision":
			build.Commit = setting.Value
		case "vcs.time":
			build.Date = setting.Value
		}
	}
}

// Get reports the build identity.
func Get() Info {
	// Prefer the build-time ldflags value; fall back to the embedded
	// VERSION file so a binary built without ldflags still reports
	// something useful.
	if build.Version == "" && len(versionFile) > 0 {
		build.Version = strings.TrimSpace(string(versionFile))
	}
	return build
}

// String renders a one-line summary.
func String() string {
	v := Get()
	if v.Version != "" && v.Commit != "" {
		return v.Version + " (" + short(v.Commit) + ")"
	}
	if v.Version != "" {
		return v.Version
	}
	if v.Commit != "" {
		return short(v.Commit)
	}
	return "unknown"
}

func short(commit string) string {
	if len(commit) >= 8 {
		return commit[:8]
	}
	return commit
}
