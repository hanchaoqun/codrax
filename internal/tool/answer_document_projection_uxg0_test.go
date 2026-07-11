package tool

// answer_document_projection_uxg0_test.go — UXG-0 push-gate pins (2026-07-11):
// the UXR-1 batch's self-inflicted display regressions, closed with
// ENGINE-MINTED fixtures only (runtimeTraceProjTreeFence /
// runtimeTraceProjDetailFullText real output; handwritten forms live only as
// counterexamples in internal/preview).
//
//   - D1: every current-generator fence HEAD form (⊚×4 target heads + ⊘×6
//     flat banner heads) must classify as a trace-projection tree in the
//     preview HTML face — the UXR-1 ⊘ rewrite had disconnected the classifier
//     arm (archived old-wording heads keep an ADDITIVE archive arm, pinned in
//     internal/preview with an archive-verbatim quote).
//   - D2: the §29.36.2 ◇ channel ordinal 邻近影响#N / adjacent-impact #N is
//     chip-styled with the NEUTRAL channel class trace-rank-adjacent (user
//     ruling: 同样式、中性色区分通道); on-chain 根因排序 #1..#5 keep their
//     per-rank colored classes.
//   - D11: an unconsumed primary-tier record rendered in the ◇ stanza demotes
//     its 因果位置 detail cell to the adjacent channel word (邻近(参考) /
//     adjacent (reference)) — never 主根因(优先处理) beside a chainless or
//     differently-consumed conclusion; the audit fact stays on the channel's
//     邻近影响#N seat line.
//
// MUTATION self-checks (recorded in the batch report):
//   - M-G1 drop the six ⊘ prefixes from isTraceCausalProjectionFence →
//     TestUXG0FenceHeadsClassifiedByPreview red (⊘ arms);
//   - M-G2 drop the 邻近影响/adjacent-impact arm from
//     traceProjectionRankToken → TestUXG0AdjacentOrdinalChipNeutralClass red;
//   - M-G3 drop the ⊘ icon-directory entry → the undrillable-box assertion in
//     TestUXG0FenceHeadsClassifiedByPreview red;
//   - M-G11 restore the Background-only demotion gate →
//     TestUXG0AdjacentPrimaryTierDetailDemotion red.

import (
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/preview"
	"github.com/hanchaoqun/codrax/internal/types"
)

// uxg0RenderFenceHTML pushes one engine-minted fence through the real preview
// renderer — the same cross-package path the standalone HTML report takes.
func uxg0RenderFenceHTML(t *testing.T, fence string) string {
	t.Helper()
	if !strings.HasPrefix(fence, "```text\n") {
		t.Fatalf("engine fence drifted (no ```text head):\n%s", fence)
	}
	html, err := preview.RenderMarkdownHTML([]byte(fence + "\n"))
	if err != nil {
		t.Fatal(err)
	}
	return html
}

// TestUXG0FenceHeadsClassifiedByPreview — D1: all ten current-generator head
// forms classify. The ⊚×4 target heads (zh/en × user-focused/anchor-only) and
// the ⊘×6 flat banner heads (zh/en × missing-wakeup / not-drilled /
// unresolved) are minted one by one; each must reach the preview face as
// <pre class="trace-projection-tree">.
func TestUXG0FenceHeadsClassifiedByPreview(t *testing.T) {
	targetProjection := func() types.TraceCausalProjection {
		return types.TraceCausalProjection{WakeupPath: []string{"tppmgr-300", "VSyncGenerator-2270"}}
	}
	missingWakeupProjection := func() types.TraceCausalProjection {
		p := uxr1FourOneSixFiveProjection()
		p.OnChainCauses = append(p.OnChainCauses, types.TraceCausalProjectionNode{
			Role: types.TraceCausalRoleRootCauseContext, EvidenceID: "E-ud",
			Subject: "sleeper-1", Object: "missing_wakeup", TypeToken: "missing_wakeup",
			Predicate: "root_cause_context", UndrillableReason: "missing_wakeup",
			ImpactMS: 1.0, CumulativeImpactMS: 1.0,
			LineStart: 600, LineEnd: 610, Confidence: 0.5,
		})
		return p
	}
	notDrilledProjection := func() types.TraceCausalProjection {
		p := uxr1FourOneSixFiveProjection()
		p.WakeupChainRecommendedNotRun = true
		return p
	}
	cases := []struct {
		name   string
		zh     bool
		anchor bool
		proj   types.TraceCausalProjection
		head   string
	}{
		{"target zh focused", true, false, targetProjection(), "⊚ VSyncGenerator-2270 ‹用户关注线程›"},
		{"target zh anchor", true, true, targetProjection(), "⊚ VSyncGenerator-2270 ‹分析锚点线程›"},
		{"target en focused", false, false, targetProjection(), "⊚ VSyncGenerator-2270 <user-focused thread>"},
		{"target en anchor", false, true, targetProjection(), "⊚ VSyncGenerator-2270 <analysis anchor thread>"},
		{"flat zh missing wakeup", true, false, missingWakeupProjection(), "⊘ 唤醒链无法上溯(窗内无唤醒记录)"},
		{"flat en missing wakeup", false, false, missingWakeupProjection(), "⊘ wakeup chain not traceable (no sched_wakeup record in the window)"},
		{"flat zh not drilled", true, false, notDrilledProjection(), "⊘ 唤醒链未下钻(本报告未运行 wakeup_chain,可追问补齐)"},
		{"flat en not drilled", false, false, notDrilledProjection(), "⊘ wakeup chain not drilled (wakeup_chain was not run for this report; ask a follow-up to fill it in)"},
		{"flat zh unresolved", true, false, uxr1FourOneSixFiveProjection(), "⊘ 唤醒链路径未解析"},
		{"flat en unresolved", false, false, uxr1FourOneSixFiveProjection(), "⊘ wakeup path unresolved"},
	}
	for _, tc := range cases {
		model := buildRuntimeTraceProjTreeModel(tc.proj, newRuntimeTraceCausalProjectionEvidenceIndex(), tc.zh)
		if tc.anchor {
			runtimeTraceProjApplyUserFocus(&model, runtimeTraceProjUserFocus{Entities: []string{"42591"}})
		}
		fence := runtimeTraceProjTreeFence(model, tc.zh)
		if !strings.Contains(fence, tc.head) {
			t.Fatalf("%s: engine head drifted, want %q:\n%s", tc.name, tc.head, fence)
		}
		html := uxg0RenderFenceHTML(t, fence)
		if !strings.Contains(html, `<pre class="trace-projection-tree"`) {
			t.Fatalf("%s: head form not classified as a projection tree:\n%s", tc.name, html)
		}
		if strings.HasPrefix(tc.head, "⊘") {
			// D3: the flat banner's ⊘ joins the one-cell optical-box family
			// (same circled family as ⊚/⊗) on the HTML face.
			if !strings.Contains(html, `trace-icon trace-icon-undrillable"><span class="trace-icon-glyph">⊘</span>`) {
				t.Fatalf("%s: ⊘ must wear its one-cell optical box:\n%s", tc.name, html)
			}
		}
	}
}

// TestUXG0AdjacentOrdinalChipNeutralClass — D2: the engine-minted ◇ ordinal
// (92300-isomorphic: a ◇ row wearing 邻近影响#1) reaches the HTML face as a
// chip with the NEUTRAL channel class; the on-chain 根因排序 ordinals keep
// their per-rank colored classes (no regression).
func TestUXG0AdjacentOrdinalChipNeutralClass(t *testing.T) {
	for _, tc := range []struct {
		name string
		zh   bool
		word string
	}{
		{"zh", true, "邻近影响#1"},
		{"en", false, "adjacent-impact #1"},
	} {
		model := buildRuntimeTraceProjTreeModel(uxr1FourOneSixFiveProjection(),
			newRuntimeTraceCausalProjectionEvidenceIndex(), tc.zh)
		fence := runtimeTraceProjTreeFence(model, tc.zh)
		if !strings.Contains(fence, tc.word) {
			t.Fatalf("%s: engine ◇ seat chip drifted, want %q:\n%s", tc.name, tc.word, fence)
		}
		html := uxg0RenderFenceHTML(t, fence)
		if !strings.Contains(html, `<span class="trace-rank-ordinal trace-rank-adjacent trace-rank-width-2">#1</span>`) {
			t.Fatalf("%s: ◇ ordinal must wear the neutral channel chip class:\n%s", tc.name, html)
		}
		// The chainless fixture publishes NO 根因排序 seats: any per-rank
		// colored ordinal here would be a channel leak.
		for rank := 1; rank <= 5; rank++ {
			leak := `trace-rank-ordinal trace-rank-` + string(rune('0'+rank))
			if strings.Contains(html, leak) {
				t.Fatalf("%s: adjacent ordinal leaked into colored class %q:\n%s", tc.name, leak, html)
			}
		}
	}
	// No-regress half: an engine-minted ON-CHAIN board keeps the per-rank
	// colored ordinal class for its 根因排序#1.
	chainFence, _ := rcrOpendirFence(t, true)
	if !strings.Contains(chainFence, "根因排序#1") {
		t.Fatalf("opendir engine fence drifted (no 根因排序#1):\n%s", chainFence)
	}
	chainHTML := uxg0RenderFenceHTML(t, chainFence)
	if !strings.Contains(chainHTML, `<span class="trace-rank-ordinal trace-rank-1 trace-rank-width-2">#1</span>`) {
		t.Fatalf("on-chain 根因排序#1 lost its colored ordinal chip:\n%s", chainHTML)
	}
	if strings.Contains(chainHTML, "trace-rank-adjacent") {
		t.Fatalf("an on-chain board must not mint neutral adjacent chips:\n%s", chainHTML)
	}
}

// uxg0AdjacentPrimaryProjection mints the D11 shape: an engine primary-tier
// record whose chain relevance is adjacent — rendered in the ◇ stanza, never
// consumed by any conclusion lead.
func uxg0AdjacentPrimaryProjection() types.TraceCausalProjection {
	node := uxr1AdjacentNode("E-pri", "runnable_wait", "runnable_wait", 1, 0.198, 110)
	node.Role = types.TraceCausalRolePrimaryRootCause
	node.Predicate = "root_cause_primary"
	node.Tier = "primary"
	return types.TraceCausalProjection{
		WindowStartTs: 2942.298, WindowEndTs: 2942.300,
		AdjacentCauses: []types.TraceCausalProjectionNode{node},
	}
}

// TestUXG0AdjacentPrimaryTierDetailDemotion — D11: the ◇ primary-tier
// non-lead row's 因果位置 detail cell speaks the §29.36.2 adjacent channel
// word, zero 主根因 wording; the channel seat line keeps the audit fact.
func TestUXG0AdjacentPrimaryTierDetailDemotion(t *testing.T) {
	for _, tc := range []struct {
		name      string
		zh        bool
		want      string
		seat      string
		forbidden []string
	}{
		{"zh", true, "邻近(参考)", "邻近影响", []string{"主根因"}},
		{"en", false, "adjacent (reference)", "adjacent-impact", []string{"primary (handle first)", "主根因"}},
	} {
		model := buildRuntimeTraceProjTreeModel(uxg0AdjacentPrimaryProjection(),
			newRuntimeTraceCausalProjectionEvidenceIndex(), tc.zh)
		if len(model.Adjacent) == 0 {
			t.Fatalf("%s: fixture drifted — primary-tier adjacent node left the ◇ stanza", tc.name)
		}
		detail := runtimeTraceProjDetailFullText(model, tc.zh)
		if !strings.Contains(detail, tc.want) {
			t.Fatalf("%s: demoted ◇ primary-tier row must wear the channel word %q:\n%s", tc.name, tc.want, detail)
		}
		if !strings.Contains(detail, tc.seat) {
			t.Fatalf("%s: the channel seat line (audit fact) must stay, want %q:\n%s", tc.name, tc.seat, detail)
		}
		for _, word := range tc.forbidden {
			if strings.Contains(detail, word) {
				t.Fatalf("%s: unconsumed ◇ primary-tier row leaked %q into the detail face:\n%s", tc.name, word, detail)
			}
		}
	}
}
