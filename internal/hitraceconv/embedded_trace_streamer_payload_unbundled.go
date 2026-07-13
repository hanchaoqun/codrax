//go:build !slim_streamer && !(windows && amd64) && !(linux && amd64)

package hitraceconv

// Default distribution on a platform without a bundled payload. The
// first wave bundles windows-amd64 and linux-amd64 only (HED-59 ruling
// 2026-07-05; default-embed ruling 2026-07-13); darwin is excluded
// because the reference darwin-aarch64 asset is a mislabeled x86_64
// Mach-O. Such builds report an explicit structured platform gap.
// `-tags slim_streamer` excludes this stub as well as every payload,
// producing the deliberate external-only distribution.

import "runtime"

func init() {
	embeddedTraceStreamerTagEnabled = true
	// Wording comes from the single shared producer so the Chinese
	// mapping and the lockstep pin stay in sync with what real builds
	// emit — never inline the sentence here.
	embeddedTraceStreamerPlatformGap = EmbeddedTraceStreamerPlatformGapMessage(runtime.GOOS, runtime.GOARCH)
}
