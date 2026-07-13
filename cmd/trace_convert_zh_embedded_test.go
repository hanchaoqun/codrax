package cmd

import (
	"errors"
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/hitraceconv"
)

// Pins that the CLI zh switch arms actually delegate embedded-tier
// wording to the shared hitraceconv mapping. English inputs come from
// the production producers, so trigger-substring drift (arm no longer
// matching what real builds emit) also turns this red instead of
// silently passing English through.
func TestTraceConvertZhDelegatesEmbeddedTraceStreamerWording(t *testing.T) {
	gap := hitraceconv.EmbeddedTraceStreamerPlatformGapMessage("darwin", "arm64")
	if got := traceConvertTraceMessageZh(gap); got == gap || !strings.Contains(got, "默认内嵌 trace_streamer 层") || !strings.Contains(got, "slim_streamer 会显式禁用内嵌 payload") || strings.Contains(got, "has no bundled payload") {
		t.Fatalf("gap caveat not localized through shared mapping:\nin:  %s\nout: %s", gap, got)
	}
	notUsable := hitraceconv.EmbeddedTraceStreamerNotUsableMessage(errors.New("sha256 mismatch"))
	if got := traceConvertTraceMessageZh(notUsable); got == notUsable || !strings.Contains(got, "内嵌 trace_streamer 不可用") {
		t.Fatalf("not-usable caveat not localized through shared mapping:\nin:  %s\nout: %s", notUsable, got)
	}
	// Source label arm; exact producer wording is pinned in
	// hitraceconv's lockstep test, this guards the switch arm.
	src := "embedded trace_streamer 7fb4eab"
	if got := traceConvertTraceSourceZh(src); !strings.Contains(got, "内嵌 trace_streamer 7fb4eab") {
		t.Fatalf("embedded source label not localized: %q -> %q", src, got)
	}
}
