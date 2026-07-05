//go:build !embed_streamer

package hitraceconv

import "testing"

// Compile-level slim pin: builds without -tags embed_streamer compile
// none of the payload stubs, so the embedded tier is inert — no assets,
// no tag flag, no payload bytes, and the discovery chain's final tier
// returns silence rather than a caveat. Together with the
// embed_streamer counterpart pins this guards the per-platform
// distribution ruling: slim builds must stay byte-lean.
func TestSlimBuildEmbedsNoTraceStreamerPayload(t *testing.T) {
	if embeddedTraceStreamerTagEnabled {
		t.Fatal("slim build must not enable the embed_streamer payload tier")
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
