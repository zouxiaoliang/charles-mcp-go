package main

import (
	"runtime/debug"
	"testing"
)

func TestResolveVersion(t *testing.T) {
	for _, tt := range []struct {
		name, injected, module, want string
	}{
		{"release linker flag", "v1.2.3", "(devel)", "v1.2.3"},
		{"linker flag takes priority", "v1.2.3-rc.1", "v1.2.2", "v1.2.3-rc.1"},
		{"explicit dev overrides module metadata", "dev", "v1.2.3", "dev"},
		{"go install release", "", "v1.2.3", "v1.2.3"},
		{"go install commit", "", "v0.0.0-20260910000000-abcdef123456", "v0.0.0-20260910000000-abcdef123456"},
		{"source checkout", "", "(devel)", "dev"},
		{"empty metadata", "", "", "dev"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			info := &debug.BuildInfo{Main: debug.Module{Version: tt.module}}
			if got := resolveVersion(tt.injected, info); got != tt.want {
				t.Fatalf("version = %q, want %q", got, tt.want)
			}
		})
	}
	if got := resolveVersion("dev", nil); got != "dev" {
		t.Fatalf("missing build info: %q", got)
	}
}
