package hitraceconv

// No build tag: the lockstep pin runs in slim and embed_streamer test
// passes alike, because the wording pair (English producers + Chinese
// mapping) is always compiled.

import (
	"path"
	"runtime"
	"strings"
	"testing"
)

// Lockstep pin for the embedded-tier wording pairs. The English inputs
// are taken from the PRODUCTION producers (never re-typed literals), so
// changing a single word on either side — producer sentence or Chinese
// mapping — makes at least one assertion here go red instead of
// silently degrading to English passthrough in the CLI/REPL.
func TestEmbeddedTraceStreamerZhLocalizationLockstep(t *testing.T) {
	// (1) Unbundled-platform gap: production formatter (the payload stub
	// for non-bundled embed_streamer builds emits exactly this).
	gap := EmbeddedTraceStreamerPlatformGapMessage("darwin", "arm64")
	zhGap := LocalizeEmbeddedTraceStreamerCaveatZh(gap)
	if zhGap == gap {
		t.Fatalf("zh mapping did not fire on the production gap wording — English producer and Chinese mapping drifted apart:\n%s", gap)
	}
	for _, want := range []string{
		"embed_streamer 构建已启用内嵌 trace_streamer",
		"darwin/arm64",
		"首批仅内嵌 linux-amd64 与 windows-amd64",
		"外部 trace_streamer",
	} {
		if !strings.Contains(zhGap, want) {
			t.Fatalf("translated gap caveat missing %q:\n%s", want, zhGap)
		}
	}
	for _, leftover := range []string{
		"payload is enabled",
		"no binary is bundled",
		"first wave bundles",
		"install or configure",
	} {
		if strings.Contains(zhGap, leftover) {
			t.Fatalf("English fragment %q survived translation (partial replace = wording drift):\n%s", leftover, zhGap)
		}
	}

	// (2) Extraction-failure caveat: production resolver output on a
	// deliberately broken payload, not a re-typed literal.
	binaryRel := path.Join(runtime.GOOS+"-"+runtime.GOARCH, traceStreamerBinaryName())
	binaryBody := []byte("embedded-trace-streamer-binary")
	manifest := embeddedTraceStreamerTestManifest(binaryRel, binaryBody)
	manifest.Platforms[0].SHA256 = strings.Repeat("0", 64)
	fsys := embeddedTraceStreamerTestFS(t, manifest, binaryRel, binaryBody, 0o444)
	withEmbeddedTraceStreamerState(t, fsys, true, "")
	t.Setenv("CODRAX_TRACE_STREAMER_CACHE", t.TempDir())
	_, _, caveats := resolveEmbeddedTraceStreamerTool()
	if len(caveats) != 1 {
		t.Fatalf("expected exactly one extraction-failure caveat, got %v", caveats)
	}
	zhNotUsable := LocalizeEmbeddedTraceStreamerCaveatZh(caveats[0])
	if zhNotUsable == caveats[0] {
		t.Fatalf("zh mapping did not fire on the production not-usable wording:\n%s", caveats[0])
	}
	if !strings.Contains(zhNotUsable, "内嵌 trace_streamer 不可用") {
		t.Fatalf("translated not-usable caveat missing Chinese lead:\n%s", zhNotUsable)
	}
	if strings.Contains(zhNotUsable, "is not usable") {
		t.Fatalf("English fragment survived not-usable translation:\n%s", zhNotUsable)
	}

	// (3) Source label: production labeler output.
	src := embeddedTraceStreamerSource(embeddedTraceStreamerManifest{UpstreamRef: "7fb4eab"})
	zhSrc := LocalizeEmbeddedTraceStreamerSourceZh(src)
	if zhSrc == src || !strings.Contains(zhSrc, "内嵌 trace_streamer 7fb4eab") {
		t.Fatalf("source label translation failed: %q -> %q", src, zhSrc)
	}

	// Foreign inputs pass through untouched — the shared mapping owns
	// only the embedded-tier wording.
	if got := LocalizeEmbeddedTraceStreamerSourceZh("trace_streamer on PATH"); got != "trace_streamer on PATH" {
		t.Fatalf("foreign source label must pass through, got %q", got)
	}
	if got := LocalizeEmbeddedTraceStreamerCaveatZh("some unrelated caveat"); got != "some unrelated caveat" {
		t.Fatalf("foreign caveat must pass through, got %q", got)
	}
}
