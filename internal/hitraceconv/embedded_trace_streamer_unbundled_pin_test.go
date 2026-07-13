//go:build !slim_streamer && !(windows && amd64) && !(linux && amd64)

package hitraceconv

import (
	"runtime"
	"strings"
	"testing"
)

// Default unbundled-platform pin (e.g. darwin, whose upstream asset is
// mislabeled): the build embeds nothing and fail-louds with a
// structured platform gap. Only explicit slim_streamer suppresses it.
func TestDefaultBuildOnUnbundledPlatformFailsLoud(t *testing.T) {
	if !embeddedTraceStreamerTagEnabled {
		t.Fatal("default unbundled-platform build must enable the structured embedded tier")
	}
	if fsys := embeddedTraceStreamerAssetsFS(); fsys != nil {
		t.Fatal("unbundled platform must not expose an embedded assets FS")
	}
	hostPlatform := runtime.GOOS + "/" + runtime.GOARCH
	if !strings.Contains(embeddedTraceStreamerPlatformGap, hostPlatform) {
		t.Fatalf("platform gap message must name the host platform %s, got %q", hostPlatform, embeddedTraceStreamerPlatformGap)
	}
	path, source, caveats := resolveEmbeddedTraceStreamerTool()
	if path != "" || source != "" {
		t.Fatalf("unbundled platform must not resolve a tool, got path=%q source=%q", path, source)
	}
	if len(caveats) != 1 || !strings.Contains(caveats[0], hostPlatform) || !strings.Contains(caveats[0], "external trace_streamer") {
		t.Fatalf("unbundled platform must fail loud with the structured gap caveat, got %v", caveats)
	}
}
