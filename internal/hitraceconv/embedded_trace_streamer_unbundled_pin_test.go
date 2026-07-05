//go:build embed_streamer && !(windows && amd64) && !(linux && amd64)

package hitraceconv

import (
	"runtime"
	"strings"
	"testing"
)

// Unbundled-platform pin for embed_streamer builds (e.g. darwin, which
// is excluded from the first wave because the reference darwin-aarch64
// asset is a mislabeled x86_64 Mach-O): the build compiles, embeds
// nothing, and the embedded tier fail-louds with a structured
// platform-gap caveat instead of silently acting like a slim build.
func TestEmbedStreamerBuildOnUnbundledPlatformFailsLoud(t *testing.T) {
	if !embeddedTraceStreamerTagEnabled {
		t.Fatal("embed_streamer build must enable the embedded payload tier even without a bundled binary")
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
	if len(caveats) != 1 || !strings.Contains(caveats[0], "embed_streamer build tag") {
		t.Fatalf("unbundled platform must fail loud with the structured gap caveat, got %v", caveats)
	}
}
