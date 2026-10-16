package buildinfo

import (
	"runtime/debug"
	"testing"
)

func TestResolve(t *testing.T) {
	tests := []struct {
		name            string
		baseVersion     string
		injectedVersion string
		injectedCommit  string
		injectedDate    string
		build           *debug.BuildInfo
		want            Info
	}{
		{
			name:        "development",
			baseVersion: "0.6.0",
			want:        Info{Version: "v0.6.0-dev", Commit: "unknown", Date: "unknown"},
		},
		{
			name:        "development with v prefix in base",
			baseVersion: "v0.6.0",
			want:        Info{Version: "v0.6.0-dev", Commit: "unknown", Date: "unknown"},
		},
		{
			name:        "module and vcs",
			baseVersion: "0.6.0",
			build: &debug.BuildInfo{
				Main: debug.Module{Version: "v0.6.1"},
				Settings: []debug.BuildSetting{
					{Key: "vcs.revision", Value: "abc123"},
					{Key: "vcs.time", Value: "2026-10-13T00:00:00Z"},
				},
			},
			want: Info{Version: "v0.6.1", Commit: "abc123", Date: "2026-10-13T00:00:00Z"},
		},
		{
			name:            "injected wins",
			baseVersion:     "0.6.0",
			injectedVersion: "v0.6.0",
			injectedCommit:  "release",
			injectedDate:    "date",
			build: &debug.BuildInfo{
				Main: debug.Module{Version: "v0.1.0"},
				Settings: []debug.BuildSetting{
					{Key: "vcs.revision", Value: "other"},
					{Key: "vcs.time", Value: "other"},
				},
			},
			want: Info{Version: "v0.6.0", Commit: "release", Date: "date"},
		},
		{
			name:        "devel module falls back to base",
			baseVersion: "0.6.0",
			build:       &debug.BuildInfo{Main: debug.Module{Version: "(devel)"}},
			want:        Info{Version: "v0.6.0-dev", Commit: "unknown", Date: "unknown"},
		},
		{
			name:        "dirty checkout",
			baseVersion: "0.6.0",
			build: &debug.BuildInfo{
				Settings: []debug.BuildSetting{
					{Key: "vcs.revision", Value: "abc123"},
					{Key: "vcs.modified", Value: "true"},
				},
			},
			want: Info{Version: "v0.6.0-dev", Commit: "abc123-dirty", Date: "unknown"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := Resolve(tt.baseVersion, tt.injectedVersion, tt.injectedCommit, tt.injectedDate, tt.build)
			if got != tt.want {
				t.Fatalf("Resolve() = %+v, want %+v", got, tt.want)
			}
		})
	}
}

func TestFormat(t *testing.T) {
	info := Info{Version: "v0.6.0", Commit: "abc123", Date: "2026-10-13T00:00:00Z"}
	if got := info.Format("codex-proxy"); got != "codex-proxy v0.6.0 (commit=abc123, date=2026-10-13T00:00:00Z)" {
		t.Fatalf("unexpected output: %s", got)
	}
	if got := info.Format("antigravity-proxy"); got != "antigravity-proxy v0.6.0 (commit=abc123, date=2026-10-13T00:00:00Z)" {
		t.Fatalf("unexpected output: %s", got)
	}
}
