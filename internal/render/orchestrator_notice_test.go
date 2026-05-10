package render

import (
	"strings"
	"testing"
)

// TestFormatOrchestratorNotice_NoLLMThinkingPrefix is the load-bearing
// regression test for the renderer/orchestrator UX-style separation.
// The whole reason EventOrchestratorNotice was carved out of
// EventAgentReasoning is that the legacy `formatReasoning` path
// prepends `  💭 [<agent>-N] ` — a visual signature reserved for
// genuine LLM reasoning text. When the orchestrator reused that path
// for its own scheduler decisions ("retry hint", "convergence stall",
// etc.) users could not distinguish "the LLM is thinking aloud" from
// "the orchestrator just decided to retry". This test pins the fix
// at the byte level: the formatter MUST NOT emit the 💭 character
// or the bracketed agent-iteration tag for any NoticeKind.
func TestFormatOrchestratorNotice_NoLLMThinkingPrefix(t *testing.T) {
	for kind := NoticeRetry; kind <= NoticeInvestigationReady; kind++ {
		out := formatOrchestratorNotice(kind, "⟳ test message")
		if strings.Contains(out, "💭") {
			t.Errorf("kind=%d: orchestrator notice MUST NOT carry the 💭 LLM-thinking icon; got %q", kind, out)
		}
		if strings.Contains(out, "[orchestrator-") || strings.Contains(out, "[orchestrator]") {
			t.Errorf("kind=%d: orchestrator notice MUST NOT carry an [agent-N] tag; got %q", kind, out)
		}
	}
}

// TestFormatOrchestratorNotice_BodyPreserved pins that the formatter
// passes the message body through verbatim — the body already carries
// its own kind glyph (⟳/·/–/›/⊘) chosen by user_messages.go and we
// must not strip or rewrite it. The test feeds each glyph variant
// and checks the output contains the same body.
func TestFormatOrchestratorNotice_BodyPreserved(t *testing.T) {
	cases := []struct {
		kind OrchestratorNoticeKind
		body string
	}{
		{NoticeRetry, "⟳ 正在补齐调查证据"},
		{NoticeForcedRead, "⟳ 正在补充关键信息"},
		{NoticeConvergenceStall, "– 根据已有线索作答"},
		{NoticeInvestigationReady, "› 调查就绪，准备作答"},
		{NoticeAbandonRead, "⊘ 跳过 `foo.go` (does not exist), 继续推进"},
		{NoticePlanReview, "· 方案已审阅, 未发现风险点"},
		{NoticeFinalizing, "⟳ 正在组织最终答案"},
	}
	for _, c := range cases {
		out := formatOrchestratorNotice(c.kind, c.body)
		plain := stripAnsiEscapes(out)
		if !strings.Contains(plain, c.body) {
			t.Errorf("kind=%d: body %q missing from output %q", c.kind, c.body, plain)
		}
	}
}

// TestFormatOrchestratorNotice_ColorByBucket pins the three-bucket
// classification: retry-class → statusRecoverable (yellow), info-class
// → statusMeta (gray), progress-class → statusObjective (cyan). The
// formatter wraps the body in exactly one of these styles; the test
// verifies the wrapping by re-styling the same body and matching the
// expected ANSI segment.
func TestFormatOrchestratorNotice_ColorByBucket(t *testing.T) {
	body := "msg"
	cases := []struct {
		name        string
		kind        OrchestratorNoticeKind
		wantSegment string
	}{
		// retry-class
		{"retry", NoticeRetry, statusRecoverable.Sprint(body)},
		{"forced-read", NoticeForcedRead, statusRecoverable.Sprint(body)},
		{"answer-check-retry", NoticeAnswerCheckRetry, statusRecoverable.Sprint(body)},
		{"fallback-finalizer", NoticeFallbackFinalizerOnly, statusRecoverable.Sprint(body)},
		{"fallback-extract", NoticeFallbackBackToExtract, statusRecoverable.Sprint(body)},
		{"fallback-explore", NoticeFallbackBackToExplore, statusRecoverable.Sprint(body)},
		{"finalizing", NoticeFinalizing, statusRecoverable.Sprint(body)},
		{"sc-start", NoticeSelfConsistencyStart, statusRecoverable.Sprint(body)},
		{"sc-rewriting", NoticeSelfConsistencyContradictionRewriting, statusRecoverable.Sprint(body)},
		{"no-tool-call", NoticeNoToolCall, statusRecoverable.Sprint(body)},

		// info-class
		{"convergence-stall", NoticeConvergenceStall, statusMeta.Sprint(body)},
		{"abandon-read", NoticeAbandonRead, statusMeta.Sprint(body)},
		{"upstream-cap", NoticeUpstreamCap, statusMeta.Sprint(body)},
		{"plan-review", NoticePlanReview, statusMeta.Sprint(body)},
		{"sc-logged", NoticeSelfConsistencyContradictionLogged, statusMeta.Sprint(body)},

		// fallback-terminal-class — glyph picks up muted yellow so a
		// "we stopped trying, finalising with what we have" notice
		// stands out from the regular gray info row.
		{"fallback-fail-loud", NoticeFallbackFailLoud, statusWarningMuted.Sprint(body)},
		{"yield-kill", NoticeYieldKill, statusWarningMuted.Sprint(body)},
		{"low-grounding", NoticeLowGrounding, statusWarningMuted.Sprint(body)},

		// progress-class
		{"investigation-ready", NoticeInvestigationReady, statusObjective.Sprint(body)},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			out := formatOrchestratorNotice(c.kind, body)
			if !strings.Contains(out, c.wantSegment) {
				t.Errorf("kind=%v expected styled segment %q in %q", c.kind, c.wantSegment, out)
			}
		})
	}
}

// TestFormatOrchestratorNotice_EmptyDegrades pins the silent-degrade
// contract: an empty / whitespace-only body returns "" so the dock
// can skip the commit without painting an empty row. The renderer
// dispatch relies on this to no-op nil-text emits cleanly.
func TestFormatOrchestratorNotice_EmptyDegrades(t *testing.T) {
	for _, body := range []string{"", "   ", "\n\t  "} {
		if got := formatOrchestratorNotice(NoticeRetry, body); got != "" {
			t.Errorf("empty/whitespace body %q must return empty output; got %q", body, got)
		}
	}
}

// TestRenderer_OrchestratorNoticeDispatchNonTTY locks that the non-TTY
// renderer branch picks up EventOrchestratorNotice (CI / piped stdout
// path) and emits it WITHOUT the 💭 LLM-thinking signature. Without
// this branch the event would silently drop on non-TTY runs.
func TestRenderer_OrchestratorNoticeDispatchNonTTY(t *testing.T) {
	var buf strings.Builder
	r := newTestRenderer("zh")
	r.SetOutput(&buf)
	emit := r.Emitter()
	emit(Event{
		Kind:       EventOrchestratorNotice,
		NoticeKind: NoticeRetry,
		Reasoning:  "⟳ 正在补齐调查证据",
	})
	out := buf.String()
	if !strings.Contains(out, "正在补齐调查证据") {
		t.Errorf("non-TTY orchestrator notice lost the body; got %q", out)
	}
	if strings.Contains(out, "💭") {
		t.Errorf("non-TTY orchestrator notice MUST NOT carry 💭; got %q", out)
	}
}
