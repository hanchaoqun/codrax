//go:build slim_streamer

package hitraceconv

import "testing"

// Compile-level slim pin: the explicit slim_streamer opt-out excludes
// every payload and the unbundled-platform gap stub. The legacy
// embed_streamer tag cannot override slim_streamer when both are set.
func TestSlimBuildEmbedsNoTraceStreamerPayload(t *testing.T) {
	if embeddedTraceStreamerTagEnabled {
		t.Fatal("slim_streamer build must not enable the embedded payload tier")
	}
	if embeddedTraceStreamerPlatformGap != "" {
		t.Fatalf("slim build must not carry a platform gap message, got %q", embeddedTraceStreamerPlatformGap)
	}
	if fsys := embeddedTraceStreamerAssetsFS(); fsys != nil {
		t.Fatal("slim build must expose no embedded assets FS")
	}
	path, source, caveats := resolveEmbeddedTraceStreamerTool()
	if path != "" || source != "" || len(caveats) != 0 {
		t.Fatalf("slim embedded tier must resolve to silence, got path=%q source=%q caveats=%v", path, source, caveats)
	}
}
