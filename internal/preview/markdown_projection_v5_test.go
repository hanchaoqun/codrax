package preview

// markdown_projection_v5_test.go — v5 P0 pins (design
// causal_tree_v5_design_20260711.md 重-1/重-3/§C.3/§G-P0, user ruling
// 2026-07-11): typed info-token hard gate, textContent==fence byte
// invariant under the run-segmentation/envelope decoration layer, and the
// 档1 E# anchor pairing. Engine-minted positives live in internal/tool
// (answer_document_projection_uxg0_test.go); this file holds the synthetic
// worst-case and the negative/boundary lanes.
//
// MUTATION self-checks (recorded in the batch report):
//   - M-P1 typed gate comparison loosened/renamed →
//     TestTraceProjectionTypedInfoTokenIsHardGate red;
//   - M-P2 envelope stops eating the companion space →
//     TestTraceProjectionGridTextContentMatchesFenceBytes red (byte drift)
//     and TestTraceProjectionIconsUseEnvelopeSlots red;
//   - M-P3 anchor prefix/id drift → TestTraceProjectionEvidenceAnchor-
//     Pairing red.

import (
	"regexp"
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/tracefence"
)

func TestTraceProjectionTypedInfoTokenIsHardGate(t *testing.T) {
	// The typed token classifies REGARDLESS of body content — the hard gate
	// reads the precise signal only (no head, no scale note needed).
	tokenOnly := "```text trace-causal-projection\narbitrary body without any signature\n```\n"
	html, err := RenderMarkdownHTML([]byte(tokenOnly))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(html, `class="trace-projection-tree"`) {
		t.Fatalf("typed info token must classify without content sniffing:\n%s", html)
	}

	// Equality is EXACT on the second token and the first token must stay
	// "text": near-miss info strings never classify (and this body carries
	// no legacy signature either).
	for _, info := range []string{
		"text trace-causal-projectionx",
		"text trace-causal-projectio",
		"text Trace-Causal-Projection",
		"text xtrace-causal-projection",
		"go trace-causal-projection",
	} {
		page, err := RenderMarkdownHTML([]byte("```" + info + "\nplain body\n```\n"))
		if err != nil {
			t.Fatal(err)
		}
		if strings.Contains(page, "trace-projection-tree") {
			t.Fatalf("near-miss info %q must NOT classify (exact-match hard gate):\n%s", info, page)
		}
	}

	// Redundant whitespace between tokens is tolerated (field split), and
	// the first-token case-insensitivity ("Text") is unchanged behavior.
	for _, info := range []string{"text  trace-causal-projection", "Text trace-causal-projection"} {
		page, err := RenderMarkdownHTML([]byte("```" + info + "\nplain body\n```\n"))
		if err != nil {
			t.Fatal(err)
		}
		if !strings.Contains(page, "trace-projection-tree") {
			t.Fatalf("info %q must classify via the typed token:\n%s", info, page)
		}
	}
}

var v5TagPattern = regexp.MustCompile(`<[^>]*>`)

func v5PreTextContent(t *testing.T, html string) string {
	t.Helper()
	return preTextContentFrom(t, html, `<pre class="trace-projection-tree"`)
}

// preTextContentFrom extracts the decoded textContent of the first pre whose
// opener starts with marker (shared by the v5 tree pins and the ELIM-CHAN
// overview pins, whose pre carries an extra hook class).
func preTextContentFrom(t *testing.T, html, marker string) string {
	t.Helper()
	start := strings.Index(html, marker)
	if start < 0 {
		t.Fatalf("no projection pre:\n%s", html)
	}
	rest := html[start:]
	end := strings.Index(rest, "</code></pre>")
	if end < 0 {
		t.Fatalf("unterminated projection pre:\n%s", rest)
	}
	inner := rest[:end]
	inner = inner[strings.Index(inner, "<code"):]
	inner = inner[strings.Index(inner, ">")+1:]
	text := v5TagPattern.ReplaceAllString(inner, "")
	return strings.NewReplacer(
		"&lt;", "<", "&gt;", ">", "&#34;", `"`, "&#39;", "'", "&quot;", `"`, "&amp;", "&",
	).Replace(text)
}

// TestTraceProjectionGridTextContentMatchesFenceBytes — v5 P0 acceptance ③
// on the synthetic worst case: every run class (ASCII run / rails / CJK /
// envelope slots / bars), both chip families, the action token, plain and
// compound E# refs, a space-less keep-mark glyph, stanza heads and an empty
// line — the decorated pre's textContent must equal the fence body byte for
// byte.
func TestTraceProjectionGridTextContentMatchesFenceBytes(t *testing.T) {
	fenceBody := strings.Join([]string{
		"⊚ com.example.app-42 ‹用户关注线程›       满格=窗口114.940ms",
		"│      ☾ 自身·sleep                       65.527ms  57%  [E1(+8)]",
		"├─下钻─ ❶ ⧖ CookieMonsterCl-59843 · runnable ██░░░░░░░░ 25.847ms 22% ⚠实际6.936ms · [E7(+1)+E8]",
		"│           · 调度压力候选 · [状态runnable] · 根因排序#1 · 置信中",
		"│      ◌ 自身  █░░░░ 16.164ms 11% 无唤醒记录·⊘链止 [E4]",
		"└─语义─ ✦ VerifyClass · 优化点 optimization point",
		"",
		"◇ 邻近区段",
		"       ⧗ app-20 · IO等待 ▒▒▒░░ 6.000ms 33% 邻近影响#2 [E9]",
		"",
		"▒ 背景压力",
		"       ⧖ rival-300 · runnable ▒▒▒░░░░░░░ 6.000ms 33% [E10]",
	}, "\n") + "\n"
	html, err := RenderMarkdownHTML([]byte("```text trace-causal-projection\n" + fenceBody + "```\n"))
	if err != nil {
		t.Fatal(err)
	}
	if got := v5PreTextContent(t, html); got != fenceBody {
		t.Fatalf("textContent drifted from fence bytes\n--- fence ---\n%q\n--- textContent ---\n%q", fenceBody, got)
	}
	// 档1 structure spot checks: physical lines wrap in .trace-line with the
	// newline OUTSIDE the span; ◇/▒ stanza heads add .trace-stanza-head.
	if !strings.Contains(html, "</span>\n<span class=\"trace-line") {
		t.Fatalf("newlines must stay outside the line spans:\n%s", html)
	}
	for _, head := range []string{"◇ 邻近区段", "▒ 背景压力"} {
		idx := strings.Index(html, `<span class="trace-line trace-stanza-head">`)
		if idx < 0 {
			t.Fatalf("stanza head %q missing its sticky line class:\n%s", head, html)
		}
	}
	if strings.Count(html, `<span class="trace-line trace-stanza-head">`) != 2 {
		t.Fatalf("exactly the two stanza heads may wear trace-stanza-head:\n%s", html)
	}
}

// TestTraceProjectionEvidenceAnchorPairing — 档1 (T-5): [E#] fence tokens
// link to the paired detail stanza / evidence roster entry; pairing is
// ordinal per fence and bails out whole on any count mismatch.
func TestTraceProjectionEvidenceAnchorPairing(t *testing.T) {
	tree := "```text trace-causal-projection\n" +
		"⊚ app-42 ‹用户关注线程› 满格=窗口10.000ms\n" +
		"│ ☾ 自身·sleep 5.000ms 50% [E1]\n" +
		"│ ⧖ worker-7 · runnable 2.000ms 20% [E2(+1)]\n" +
		"```\n"
	detail := "## 因果投影明细(逐节点完整属性)\n\n" +
		"每节点一块。\n\n" +
		"**[E1] 自身·sleep**\n\n- 层级: 自因\n\n" +
		"**[E2] worker-7**\n\n- 层级: 链上L1\n"
	evidence := "## 证据索引\n\n" +
		"全部证据位于 `t.systrace`。\n\n" +
		"- **E1** — 定位: 行 5–9; 审计: kind=self\n" +
		"- **E2** — 定位: 行 12–20; 审计: kind=chain\n"

	html, err := RenderMarkdownHTML([]byte(tree + "\n" + detail + "\n" + evidence))
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{
		`<a class="trace-eref" style="width:4ch" href="#trace-e1">[E1]</a>`,
		`<a class="trace-eref" style="width:8ch" href="#trace-e2">[E2(+1)]</a>`,
		// Detail stanza headings win the id (first occurrence in doc order).
		`<p id="trace-e1">`,
		`<p id="trace-e2">`,
	} {
		if !strings.Contains(html, want) {
			t.Fatalf("anchor pairing missing %q:\n%s", want, html)
		}
	}
	// The evidence roster must NOT duplicate the claimed ids.
	if strings.Count(html, `id="trace-e1"`) != 1 || strings.Count(html, `id="trace-e2"`) != 1 {
		t.Fatalf("anchor ids must be unique:\n%s", html)
	}

	// Evidence-only document: the roster list items carry the ids.
	html, err = RenderMarkdownHTML([]byte(tree + "\n" + evidence))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(html, `<li id="trace-e1">`) || !strings.Contains(html, `href="#trace-e1"`) {
		t.Fatalf("evidence roster fallback must carry the anchor ids:\n%s", html)
	}

	// Count mismatch (two fences, one detail family): NO anchor decoration —
	// a wrong link is worse than none.
	html, err = RenderMarkdownHTML([]byte(tree + "\n" + tree + "\n" + detail))
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(html, "trace-eref") || strings.Contains(html, `id="trace-e1"`) {
		t.Fatalf("ambiguous pairing must bail out whole:\n%s", html)
	}

	// Matched multi-artifact layout: per-group prefixes, in document order.
	html, err = RenderMarkdownHTML([]byte(tree + "\n" + detail + "\n" + tree + "\n" + detail))
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{
		`href="#trace-g1-e1"`, `<p id="trace-g1-e1">`,
		`href="#trace-g2-e1"`, `<p id="trace-g2-e1">`,
	} {
		if !strings.Contains(html, want) {
			t.Fatalf("multi-group pairing missing %q:\n%s", want, html)
		}
	}

	// A lone fence (no detail/evidence sections at all) renders plain runs.
	html, err = RenderMarkdownHTML([]byte(tree))
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(html, "trace-eref") {
		t.Fatalf("fence without paired sections must not mint links:\n%s", html)
	}
	if !strings.Contains(html, `>[E1]</span>`) {
		t.Fatalf("unpaired [E1] must stay a plain pinned run:\n%s", html)
	}
}

// TestTraceProjectionAnchorLinksOnlyClaimedOrdinals — F5 (fix round
// 2026-07-11): a merged detail heading "[E7] [E9] name" claims ONE id (its
// first unseen ordinal); with no evidence roster to pick up E9, the fence's
// [E9] token must stay a plain pinned run — the writer never mints a
// dangling href.
func TestTraceProjectionAnchorLinksOnlyClaimedOrdinals(t *testing.T) {
	doc := "```text trace-causal-projection\n" +
		"⊚ app-42 ‹用户关注线程› 满格=窗口10.000ms\n" +
		"│ ⧖ worker-7 · runnable 2.000ms 20% [E7]\n" +
		"│ ⧖ worker-9 · runnable 1.000ms 10% [E9]\n" +
		"```\n\n" +
		"## 因果投影明细(逐节点完整属性)\n\n" +
		"每节点一块。\n\n" +
		"**[E7] [E9] worker**\n\n- 层级: 链上L1\n"
	html, err := RenderMarkdownHTML([]byte(doc))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(html, `<a class="trace-eref" style="width:4ch" href="#trace-e7">[E7]</a>`) {
		t.Fatalf("claimed ordinal E7 must link:\n%s", html)
	}
	if !strings.Contains(html, `<p id="trace-e7">`) {
		t.Fatalf("merged heading must claim its first unseen ordinal:\n%s", html)
	}
	if strings.Contains(html, `href="#trace-e9"`) {
		t.Fatalf("unclaimed ordinal E9 must not mint a dangling link:\n%s", html)
	}
	if !strings.Contains(html, `>[E9]</span>`) {
		t.Fatalf("unclaimed [E9] must stay a plain pinned run:\n%s", html)
	}
}

// TestTraceProjectionLegacyFlatHeadPrefixCutsBothParenFamilies — F6 (fix
// round 2026-07-11): the legacy ⊘-head prefix cut recognizes BOTH opening
// paren families. The current closed set spells its parens ASCII (verbatim
// generator bytes); the fullwidth arm future-proofs a zh wording evolution
// — previously the set spelled two ASCII parens, leaving a fullwidth head
// uncut (whole-string match only).
func TestTraceProjectionLegacyFlatHeadPrefixCutsBothParenFamilies(t *testing.T) {
	if got := legacyFlatHeadPrefix("⊘ 唤醒链无法上溯（旧短因词面）"); got != "⊘ 唤醒链无法上溯（" {
		t.Fatalf("fullwidth-paren head must cut after （, got %q", got)
	}
	if got := legacyFlatHeadPrefix(tracefence.FlatHeadMissingWakeupZH); got != "⊘ 唤醒链无法上溯(" {
		t.Fatalf("ASCII-paren closed-set head must cut after (, got %q", got)
	}
	if got := legacyFlatHeadPrefix(tracefence.FlatHeadUnresolvedZH); got != tracefence.FlatHeadUnresolvedZH {
		t.Fatalf("paren-less head must match whole, got %q", got)
	}
	// End-to-end: an ARCHIVED fence whose 短因 wording differs from today's
	// closed set still classifies via the prefix cut (the fallback lane's
	// whole point).
	archived := "```text\n⊘ 唤醒链无法上溯(某个历史短因词面)  满格=窗口2.000ms\n```\n"
	html, err := RenderMarkdownHTML([]byte(archived))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(html, `class="trace-projection-tree"`) {
		t.Fatalf("archived evolved-wording ⊘ head must classify via prefix cut:\n%s", html)
	}
}
