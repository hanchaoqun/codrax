package preview

// markdown_trace_lead_test.go — UX-ANCHOR pins (§29.61.7 customer feedback,
// 2026-07-14):
//
//   件a — lead-segment E# refs ([E#], [E#(+N)], the parenthesized bare "(E#)"
//        form and the ➋[E#] pair) link into the SAME per-fence anchor pairing
//        the fence writer consumes; same fail-closed discipline (count
//        identity bails the whole document, unclaimed ordinals stay plain,
//        non-lead prose is never decorated).
//   件b — ➊..➎ in lead prose wear the COMPACT body badge; the tree fence's
//        2ch envelope pill is untouched; backtick legend quotes stay verbatim.
//   件c — the lead anchor style block ships verbatim (link色/hover 可辨).
//
// MUTATION self-checks (recorded in the batch report):
//   - drop the heading-boundary closed-set check in traceProjectionLeadBlocks
//     → TestTraceLeadScopeRequiresProjectionHeading red (foreign-heading
//     paragraph gains a link);
//   - link unclaimed ordinals (F5 break) → TestTraceLeadEvidenceRefs… red
//     (dangling href assertion);
//   - decorate on count mismatch → TestTraceLeadFailClosedOnCountMismatch red.

import (
	"strings"
	"testing"
)

func traceLeadTestTree() string {
	return "```text trace-causal-projection\n" +
		"⊚ app-42 ‹用户关注线程› 满格=窗口10.000ms\n" +
		"│ ☾ 自身·sleep 5.000ms 50% [E1]\n" +
		"│ ⧖ worker-7 · runnable 2.000ms 20% [E2(+1)]\n" +
		"```\n"
}

func traceLeadTestSections() string {
	return "## 因果投影明细(逐节点完整属性)\n\n" +
		"每节点一块。\n\n" +
		"**[E1] 自身·sleep**\n\n- 层级: 自因\n\n" +
		"**[E2] worker-7**\n\n- 层级: 链上L1\n\n" +
		"## 证据索引\n\n" +
		"全部证据位于 `t.systrace`。\n\n" +
		"- **E1** — 定位: 行 5–9; 审计: kind=self\n" +
		"- **E2** — 定位: 行 12–20; 审计: kind=chain\n"
}

// traceLeadTestLead is the generator-shaped lead prose: conclusion line with a
// bracketed ref, running decomposition with the ➋[E#] pair, and the coverage
// residual note's parenthesized bare form "(E1)".
func traceLeadTestLead() string {
	return "## Trace 因果投影\n\n" +
		"**主根因:** worker-7 runnable 2.000ms，见 [E2(+1)]。\n\n" +
		"分析窗 100.000s → 100.010s，共 10.000ms。\n" +
		"- running 5.000ms: 确定性工作 1.000ms ➋[E2(+1)] · 供给折算影响 2.000ms 见 ➌[E1](折算,不计入四态合计) · 自身执行(无确定性可优化工作) 2.000ms。\n" +
		"- 未归因中最大 1.000ms 与自身 IO 口径行(E1)重叠解释,未计入链上归因以防双计。\n\n"
}

func TestTraceLeadEvidenceRefsLinkAndBadgeCompact(t *testing.T) {
	doc := traceLeadTestLead() + traceLeadTestTree() + "\n" + traceLeadTestSections()
	html, err := RenderMarkdownHTML([]byte(doc))
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{
		// 件a: bracketed lead ref links on the shared pairing (claimed E2).
		`<a class="trace-eref-lead" href="#trace-e2">[E2(+1)]</a>`,
		// 件a: the parenthesized bare form links, parens stay plain text.
		`(<a class="trace-eref-lead" href="#trace-e1">E1</a>)重叠解释`,
		// 件b: the badge wears the COMPACT body form with its rank color.
		`<span class="trace-lead-badge trace-rank-2">➋</span>`,
		// The fence's own anchor machinery is untouched beside the lead lane.
		`<a class="trace-eref" style="width:8ch" href="#trace-e2">[E2(+1)]</a>`,
		`<p id="trace-e1">`,
	} {
		if !strings.Contains(html, want) {
			t.Fatalf("lead decoration missing %q:\n%s", want, html)
		}
	}
	// 件b: the badge pairs directly with its linked ref (➋[E2(+1)] form).
	if !strings.Contains(html, `</span><a class="trace-eref-lead" href="#trace-e2">[E2(+1)]</a>`) {
		t.Fatalf("badge+ref pair must decorate adjacently:\n%s", html)
	}
	// 件a accidental-link repair: 「见 ➌[E1](折算,不计入四态合计)」 markdown-
	// parses as a LINK (bogus href, swallowed note) — the lead lane rebuilds
	// it as the anchored ref plus the VISIBLE verbatim parenthetical.
	if !strings.Contains(html, `<a class="trace-eref-lead" href="#trace-e1">[E1]</a>(折算,不计入四态合计)`) {
		t.Fatalf("accidental ref-link must repair into anchored ref + visible note:\n%s", html)
	}
	if strings.Contains(html, `href="%E6%8A%98%E7%AE%97`) {
		t.Fatalf("the bogus relative href must not survive:\n%s", html)
	}
	// textContent discipline: stripping tags restores the lead line verbatim
	// (the swallowed parenthetical is BACK as text — the pre-repair face lost
	// it into the href).
	stripped := v5TagPattern.ReplaceAllString(html, "")
	if !strings.Contains(stripped, "确定性工作 1.000ms ➋[E2(+1)] · 供给折算影响 2.000ms 见 ➌[E1](折算,不计入四态合计) · 自身执行(无确定性可优化工作) 2.000ms。") {
		t.Fatalf("lead textContent drifted:\n%s", stripped)
	}
	if !strings.Contains(stripped, "与自身 IO 口径行(E1)重叠解释") {
		t.Fatalf("bare-form textContent drifted:\n%s", stripped)
	}
}

// TestTraceLeadRealLinksStayUntouched — the accidental-link repair is grammar
// gated: a REAL markdown link in lead prose (URL / fragment destination, or a
// non-E# label) keeps its normal rendering.
func TestTraceLeadRealLinksStayUntouched(t *testing.T) {
	doc := "## Trace 因果投影\n\n" +
		"参考 [E1](https://example.com/doc) 与 [说明](note.md)。\n\n" +
		traceLeadTestTree() + "\n" + traceLeadTestSections()
	html, err := RenderMarkdownHTML([]byte(doc))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(html, `<a href="https://example.com/doc">E1</a>`) {
		t.Fatalf("a real URL link must stay a normal link:\n%s", html)
	}
	if !strings.Contains(html, `>说明</a>`) {
		t.Fatalf("a non-E# label link must stay untouched:\n%s", html)
	}
}

// TestTraceLeadCompactBadgeAndLinkStyleVerbatim — 件b/件c style pins: the
// compact badge rule (0.85em, unbolded, line-height 1 — visually subordinate
// to the body line box) and the lead link rule (dotted underline, hover
// solid; link color rides the page-global a rule) ship verbatim in the
// standalone page CSS. The fence pill rule stays untouched beside them.
func TestTraceLeadCompactBadgeAndLinkStyleVerbatim(t *testing.T) {
	page, err := RenderStandaloneMarkdownHTML("t", []byte("x\n"))
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{
		`.trace-lead-badge { display: inline-block; font-family: var(--font-symbols); font-size: .85em; font-weight: 400; line-height: 1; padding: .06em .16em; border-radius: .28em; vertical-align: baseline; font-synthesis: none; font-variant-emoji: text; print-color-adjust: exact; -webkit-print-color-adjust: exact; }`,
		`.trace-lead-badge.trace-rank-2 { color: var(--rank-2-fg); background: var(--rank-2-bg); }`,
		`a.trace-eref-lead { font-family: var(--font-mono); font-size: .92em; white-space: nowrap; text-decoration: underline dotted; text-underline-offset: .18em; }`,
		`a.trace-eref-lead:hover { text-decoration-style: solid; }`,
		// fence pill zero-change sentinel (件b scope: 树 fence 内 pill 零改).
		`pre.trace-projection-tree .trace-rank-pill { border-radius: .22em; font-weight: 750; print-color-adjust: exact; -webkit-print-color-adjust: exact; }`,
	} {
		if !strings.Contains(page, want) {
			t.Fatalf("standalone CSS missing verbatim rule %q", want)
		}
	}
}

// TestTraceLeadFailClosedOnCountMismatch — the count-identity break kills
// EVERY link in the document (fence and lead alike, no dangling href), while
// the lead badge styling — presentation only, no target to dangle — persists.
func TestTraceLeadFailClosedOnCountMismatch(t *testing.T) {
	doc := traceLeadTestLead() + traceLeadTestTree() + "\n" + traceLeadTestTree() + "\n" +
		"## 因果投影明细(逐节点完整属性)\n\n每节点一块。\n\n**[E1] 自身·sleep**\n\n- 层级: 自因\n"
	html, err := RenderMarkdownHTML([]byte(doc))
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(html, "trace-eref-lead") || strings.Contains(html, `href="#trace-`) {
		t.Fatalf("count mismatch must kill every anchor link (lead included):\n%s", html)
	}
	if !strings.Contains(html, `<span class="trace-lead-badge trace-rank-2">➋</span>`) {
		t.Fatalf("compact badge styling is target-free and must survive the link bail:\n%s", html)
	}
}

// TestTraceLeadUnclaimedOrdinalStaysPlain — F5 on the lead lane: an ordinal
// without a claimed target renders as plain text, never a dangling link.
func TestTraceLeadUnclaimedOrdinalStaysPlain(t *testing.T) {
	doc := "## Trace 因果投影\n\n" +
		"引用一个已认领 [E1] 和一个未认领 [E9] 的例子。\n\n" +
		traceLeadTestTree() + "\n" +
		"## 因果投影明细(逐节点完整属性)\n\n每节点一块。\n\n**[E1] 自身·sleep**\n\n- 层级: 自因\n\n**[E2] worker-7**\n\n- 层级: 链上L1\n"
	html, err := RenderMarkdownHTML([]byte(doc))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(html, `<a class="trace-eref-lead" href="#trace-e1">[E1]</a>`) {
		t.Fatalf("claimed lead ordinal must link:\n%s", html)
	}
	if strings.Contains(html, `href="#trace-e9"`) {
		t.Fatalf("unclaimed lead ordinal must not mint a dangling link:\n%s", html)
	}
	if !strings.Contains(html, "未认领 [E9] 的例子") {
		t.Fatalf("unclaimed lead ordinal must stay verbatim plain text:\n%s", html)
	}
}

// TestTraceLeadScopeRequiresProjectionHeading — the lead scope is bounded by
// the tracefence projection H2 closed set: prose under a FOREIGN heading
// (even directly above the fence) and prose ABOVE the projection heading are
// never decorated; the artifact-suffixed multi-group form still is.
func TestTraceLeadScopeRequiresProjectionHeading(t *testing.T) {
	foreign := "## 其他分析\n\n结论提及 [E1] 但这不是投影 lead。\n\n" +
		traceLeadTestTree() + "\n" + traceLeadTestSections()
	html, err := RenderMarkdownHTML([]byte(foreign))
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(html, "trace-eref-lead") || strings.Contains(html, "trace-lead-badge") {
		t.Fatalf("foreign-heading prose must not be decorated:\n%s", html)
	}
	// The fence's own links are pairing-driven and stay live.
	if !strings.Contains(html, `<a class="trace-eref" style="width:4ch" href="#trace-e1">[E1]</a>`) {
		t.Fatalf("fence links must stay live under a foreign heading:\n%s", html)
	}

	above := "上文提及 [E1] 不属于投影节。\n\n" + traceLeadTestLead() +
		traceLeadTestTree() + "\n" + traceLeadTestSections()
	html, err = RenderMarkdownHTML([]byte(above))
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(html, `上文提及 <a`) {
		t.Fatalf("prose above the projection heading must stay plain:\n%s", html)
	}
	if !strings.Contains(html, `<a class="trace-eref-lead" href="#trace-e2">[E2(+1)]</a>`) {
		t.Fatalf("the real lead below the heading must still decorate:\n%s", html)
	}

	// Multi-artifact: per-group prefixes reach the lead refs, and the
	// generator's " — <artifact>" title suffix stays in the closed set.
	multi := "## Trace 因果投影 — a.systrace\n\n结论见 [E1]。\n\n" + traceLeadTestTree() + "\n" +
		"## Trace 因果投影 — b.systrace\n\n结论见 [E1]。\n\n" + traceLeadTestTree() + "\n" +
		traceLeadTestSections() + "\n" + traceLeadTestSections()
	html, err = RenderMarkdownHTML([]byte(multi))
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{
		`<a class="trace-eref-lead" href="#trace-g1-e1">[E1]</a>`,
		`<a class="trace-eref-lead" href="#trace-g2-e1">[E1]</a>`,
	} {
		if !strings.Contains(html, want) {
			t.Fatalf("multi-artifact lead pairing missing %q:\n%s", want, html)
		}
	}
}

// TestTraceLeadBacktickLegendQuotesStayVerbatim — the 树读法 legend quotes
// badge glyphs inside backticks (`➊..➎`); code spans are teaching text and
// must not grow badge spans (nor may any lead code span be restyled).
func TestTraceLeadBacktickLegendQuotesStayVerbatim(t *testing.T) {
	doc := "## Trace 因果投影\n\n" +
		"树读法:\n" +
		"- `➊..➎` = 根因排序前五(依有效归因)。\n\n" +
		traceLeadTestTree() + "\n" + traceLeadTestSections()
	html, err := RenderMarkdownHTML([]byte(doc))
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(html, "trace-lead-badge") {
		t.Fatalf("backtick legend quote must not grow a badge span:\n%s", html)
	}
}

// TestTraceLeadBareFormBoundaries — the parenthesized bare grammar is exact:
// pid parentheticals ("E2(1234)"), hyphenated names ("Thread-E5") and
// unwrapped bare tags never link; only the generator's "(E#)"/"(E#(+N))"
// forms do.
func TestTraceLeadBareFormBoundaries(t *testing.T) {
	doc := "## Trace 因果投影\n\n" +
		"线程 E2(1234) 与 Thread-E5 出现在名称里;残差与(E1(+2))重叠;裸写 E2 不链接。\n\n" +
		traceLeadTestTree() + "\n" + traceLeadTestSections()
	html, err := RenderMarkdownHTML([]byte(doc))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(html, `(<a class="trace-eref-lead" href="#trace-e1">E1(+2)</a>)重叠`) {
		t.Fatalf("the wrapped bare merge form must link:\n%s", html)
	}
	if strings.Contains(html, `>E2</a>`) || strings.Contains(html, `>E5</a>`) {
		t.Fatalf("name-embedded / unwrapped bare tags must never link:\n%s", html)
	}
	if !strings.Contains(html, "线程 E2(1234) 与 Thread-E5 出现在名称里") {
		t.Fatalf("undedecorated prose must stay verbatim:\n%s", html)
	}
}

// TestQuestionFenceNeverClassifiesAsTraceOrMermaid — UX-ANCHOR 件d⑤: the
// output-dump 问题节 verbatim fence carries its own typed second token; a
// customer question that PASTES a projection-tree fragment (legacy-sniffable
// head + scale note) or a mermaid-looking body must render as one plain
// escaped <pre>, never re-classified by the content-sniffing lanes.
func TestQuestionFenceNeverClassifiesAsTraceOrMermaid(t *testing.T) {
	pasted := "```text codrax-user-request\n" +
		"请看我上次报告里的这棵树:\n" +
		"⊚ app-42 ‹用户关注线程› 满格=窗口10.000ms\n" +
		"│ ☾ 自身·sleep 5.000ms 50% [E1]\n" +
		"```\n"
	html, err := RenderMarkdownHTML([]byte(pasted))
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(html, "trace-projection-tree") {
		t.Fatalf("question fence must not classify as a projection tree:\n%s", html)
	}
	// EVOLUTION RECORD (§29.61.8a, 2026-07-14): the question pre wears the
	// wrap-enabled user-request class; still an escaped non-grid pre.
	if !strings.Contains(html, `<pre class="user-request"><code class="language-text">`) {
		t.Fatalf("question fence must render as the wrap-enabled escaped pre:\n%s", html)
	}

	mermaidish := "```text codrax-user-request\ngraph TD\nA-->B\n```\n"
	html, err = RenderMarkdownHTML([]byte(mermaidish))
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(html, `<div class="mermaid">`) {
		t.Fatalf("question fence must not fall into mermaid body sniffing:\n%s", html)
	}

	// The archive lane is untouched: a BARE ```text fence with the same tree
	// body still classifies through the demoted legacy sniff.
	archive := "```text\n⊚ app-42 ‹用户关注线程› 满格=窗口10.000ms\n```\n"
	html, err = RenderMarkdownHTML([]byte(archive))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(html, "trace-projection-tree") {
		t.Fatalf("bare-text archive fence must keep its legacy classification:\n%s", html)
	}
}
