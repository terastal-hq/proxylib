// Package buildinfo resolves executable version, commit, and build date metadata
// from release link flags, embedded base versions, and runtime debug build information.
package buildinfo

import (
	"fmt"
	"runtime/debug"
	"strings"
)

// Info holds non-secret executable identity.
type Info struct {
	Version string
	Commit  string
	Date    string
}

// Current returns executable identity by combining injected variables with debug.ReadBuildInfo().
func Current(baseVersion, injectedVersion, injectedCommit, injectedDate string) Info {
	info, _ := debug.ReadBuildInfo()
	return Resolve(baseVersion, injectedVersion, injectedCommit, injectedDate, info)
}

// Resolve determines executable identity by evaluating injected flags against runtime build settings.
func Resolve(baseVersion, injectedVersion, injectedCommit, injectedDate string, build *debug.BuildInfo) Info {
	info := Info{Version: injectedVersion, Commit: injectedCommit, Date: injectedDate}
	if info.Version == "" {
		v := strings.TrimSpace(baseVersion)
		if !strings.HasPrefix(v, "v") {
			v = "v" + v
		}
		info.Version = v + "-dev"
		if build != nil && build.Main.Version != "" && build.Main.Version != "(devel)" {
			info.Version = build.Main.Version
		}
	}
	dirty := false
	if build != nil {
		for _, setting := range build.Settings {
			switch setting.Key {
			case "vcs.revision":
				if info.Commit == "" {
					info.Commit = setting.Value
				}
			case "vcs.modified":
				dirty = setting.Value == "true"
			case "vcs.time":
				if info.Date == "" {
					info.Date = setting.Value
				}
			}
		}
	}
	if info.Commit == "" {
		info.Commit = "unknown"
	}
	if dirty && injectedCommit == "" && info.Commit != "unknown" {
		info.Commit += "-dirty"
	}
	if info.Date == "" {
		info.Date = "unknown"
	}
	return info
}

// Format formats the executable identity with the specified binary/application name.
func (i Info) Format(appName string) string {
	return fmt.Sprintf("%s %s (commit=%s, date=%s)", appName, i.Version, i.Commit, i.Date)
}
