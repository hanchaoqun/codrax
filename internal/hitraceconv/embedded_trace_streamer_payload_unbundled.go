//go:build embed_streamer && !(windows && amd64) && !(linux && amd64)

package hitraceconv

// embed_streamer build on a platform without a bundled payload.
// The first wave bundles windows-amd64 and linux-amd64 only (HED-59
// ruling 2026-07-05; darwin is excluded because the reference
// darwin-aarch64 asset is a mislabeled x86_64 Mach-O). Such builds
// compile, but the discovery chain reports an explicit structured
// unavailability caveat instead of silently behaving like a slim
// build — external discovery tiers keep working unchanged.

import "runtime"

func init() {
	embeddedTraceStreamerTagEnabled = true
	// Wording comes from the single shared producer so the Chinese
	// mapping and the lockstep pin stay in sync with what real builds
	// emit — never inline the sentence here.
	embeddedTraceStreamerPlatformGap = EmbeddedTraceStreamerPlatformGapMessage(runtime.GOOS, runtime.GOARCH)
}
