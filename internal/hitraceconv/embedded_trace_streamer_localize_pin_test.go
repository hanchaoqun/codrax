package hitraceconv

// No build tag: the lockstep pin runs in default and slim_streamer test
// passes alike because the English producer and Chinese mapping are
// always compiled.

import (
	"fmt"
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
	// (1) Default unbundled-platform gap from the production formatter.
	gap := EmbeddedTraceStreamerPlatformGapMessage("darwin", "arm64")
	zhGap := LocalizeEmbeddedTraceStreamerCaveatZh(gap)
	if zhGap == gap {
		t.Fatalf("zh mapping did not fire on the production gap wording — English producer and Chinese mapping drifted apart:\n%s", gap)
	}
	for _, want := range []string{
		"默认内嵌 trace_streamer 层",
		"darwin/arm64",
		"未内嵌 darwin/arm64 平台 payload",
		"配置外部 trace_streamer",
		"slim_streamer 会显式禁用内嵌 payload",
	} {
		if !strings.Contains(zhGap, want) {
			t.Fatalf("translated gap caveat missing %q:\n%s", want, zhGap)
		}
	}
	for _, leftover := range []string{
		"has no bundled payload",
		"configure an external",
		"install a distribution",
		"explicitly disables embedded payloads",
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

	// (2b) Verified-child runtime failure is a separate producer and source
	// identity; neither may silently fall through to the integrity wording.
	runtimeFailure := EmbeddedTraceStreamerRuntimeIncompatibleMessage("2.34", fmt.Errorf("loader_missing: /lib64/ld-linux-x86-64.so.2"))
	zhRuntimeFailure := LocalizeEmbeddedTraceStreamerCaveatZh(runtimeFailure)
	for _, want := range []string{"子工具运行时不兼容", "Codrax 父程序", "glibc/共享库", "loader_missing"} {
		if !strings.Contains(zhRuntimeFailure, want) {
			t.Fatalf("translated runtime caveat missing %q:\n%s", want, zhRuntimeFailure)
		}
	}
	if got := LocalizeEmbeddedTraceStreamerSourceZh("embedded_runtime_incompatible"); got != "内嵌 trace_streamer 子工具运行时不兼容" {
		t.Fatalf("runtime source identity translation mismatch: %q", got)
	}

	// (3) Source label: production labeler output.
	src := embeddedTraceStreamerSource(embeddedTraceStreamerManifest{UpstreamRef: "7fb4eab"})
	zhSrc := LocalizeEmbeddedTraceStreamerSourceZh(src)
	if zhSrc == src || !strings.Contains(zhSrc, "内嵌 trace_streamer 7fb4eab") {
		t.Fatalf("source label translation failed: %q -> %q", src, zhSrc)
	}

	// (4) Failure provenance is produced by the shared typed lane
	// formatter. It is a user-visible caveat in CLI/REPL failure paths,
	// so the embedded source label and all surrounding field labels must
	// remain localized as one unit.
	resolutionCaveats := traceStreamerFailureCaveats(traceProviderLanePlan{
		Path:      "/tmp/embedded/trace_streamer",
		Source:    src,
		Available: true,
	}, "conversion failed")
	var resolution string
	for _, caveat := range resolutionCaveats {
		if strings.Contains(caveat, "trace_streamer provider resolution:") {
			resolution = caveat
			break
		}
	}
	if resolution == "" {
		t.Fatalf("production failure caveats did not include provider resolution: %v", resolutionCaveats)
	}
	zhResolution := LocalizeConvertMessage("zh", resolution)
	for _, want := range []string{"trace_streamer 提供方解析：", "来源=内嵌 trace_streamer 7fb4eab", "路径=/tmp/embedded/trace_streamer", "可用=是"} {
		if !strings.Contains(zhResolution, want) {
			t.Fatalf("translated provider resolution missing %q:\n%s", want, zhResolution)
		}
	}
	for _, leftover := range []string{"provider resolution:", "source=", " path=", " available=", "embedded trace_streamer"} {
		if strings.Contains(zhResolution, leftover) {
			t.Fatalf("English provider-resolution fragment %q survived translation:\n%s", leftover, zhResolution)
		}
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
