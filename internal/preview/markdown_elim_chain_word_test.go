package preview

// markdown_elim_chain_word_test.go — ELIM-CHAN pins (user ruling 2026-07-14:
// the ◎ 窗内可消除量总览 board's ⛓ 链上 vs ◇ 邻近 regions read too alike;
// the ⛓ chain channel word gains ONE understated dark-green ink — color-only
// single encoding, no weight change — on the HTML face only). Contract under
// test:
//
//   - the ⛓ channel word (glyph + noun, zh 链上 / en on-chain) wraps in ONE
//     <span class="elim-chain-word"> whose INNER markup is byte-identical to
//     the unwrapped grid form (envelope slot + CJK cells / ASCII run) — the
//     wrapper is pure ink, textContent and grid geometry untouched;
//   - the header promise line (⛓ 链上块先…) colors by the same token hit;
//   - ◇ 邻近 stays default ink (zero wrappers);
//   - scope is the typed trace-elim-overview fence ONLY: the projection tree
//     fence, plain text fences and the user-request fence never wrap, even
//     when their bodies carry the token bytes;
//   - escape safety: fence bytes containing <&> still escape inside and
//     around the wrapper, and pre textContent == fence bytes.
//
// MUTATION self-checks (recorded in the batch report):
//   - M-E1 token arm unplugged (elimChain flag never passes true / the
//     traceElimChainWordToken arm removed) →
//     TestElimOverviewChainWordWearsColorSpan red (zero wrappers);
//   - M-E2 scope widened (elimChain forced true for the projection tree) →
//     TestElimChainWordScopedToElimOverviewFence red.

import (
	"strings"
	"testing"
)

// elimChainBoardZH transcribes the engine emitters' zh line shapes
// (runtimeTraceProjElimHead two-line wrap form, runtimeTraceProjElimRowLine
// `%9.3fms ` + 12-cell bar + channel word, the 件⑥ composition-note indent) —
// fixture 取产线实铸形, not an invented layout.
const elimChainBoardZH = "```text trace-elim-overview\n" +
	"◎ 窗内可消除量总览 · 尺=com.example.app-42 窗内墙钟ms\n" +
	"⛓ 链上块先·块内值降序·零序数·零佩戴 · 定位走 [E#] · 满格=本区TOP1\n" +
	"   26.392ms ████████████ ⛓ 链上 · CookieMonsterCl-59843 · 调度压力候选 [E7]\n" +
	"    7.081ms ███░░░░░░░░░ ⛓ 链上 · binder:42591_4 · 优先级反转候选·供给缺口主导 [E3]\n" +
	"            · 可消除构成: 调度修复 4.200ms + 频点/热策略 2.881ms\n" +
	"    6.000ms ██░░░░░░░░░░ ◇ 邻近 · app-20 · IO等待 [E9]\n" +
	"```\n"

// elimChainWrappedZH is the exact wrapped form of one zh channel-word hit:
// the same envelope slot + CJK cells the unwrapped grid emits, inside the
// single ink wrapper.
const elimChainWrappedZH = `<span class="elim-chain-word">` +
	`<span class="trace-slot trace-icon trace-icon-io"><span class="trace-ink">⛓ </span></span>` +
	`<span class="trace-cell trace-cell-2">链</span>` +
	`<span class="trace-cell trace-cell-2">上</span>` +
	`</span>`

func TestElimOverviewChainWordWearsColorSpan(t *testing.T) {
	html, err := RenderMarkdownHTML([]byte(elimChainBoardZH))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(html, `<pre class="trace-projection-tree trace-elim-overview"`) {
		t.Fatalf("the ◎ fence must classify on its typed token:\n%s", html)
	}
	// Exactly the three ⛓ token hits wrap: header promise line + two chain
	// member rows. The ◇ row contributes zero.
	if got := strings.Count(html, `<span class="elim-chain-word">`); got != 3 {
		t.Fatalf("want exactly 3 elim-chain-word wrappers (header + 2 rows), got %d:\n%s", got, html)
	}
	if got := strings.Count(html, elimChainWrappedZH); got != 3 {
		t.Fatalf("wrapped token inner markup must stay the exact unwrapped grid form (slot + 2ch cells), got %d of:\n%s\n---\n%s", got, elimChainWrappedZH, html)
	}
	// ◇ 邻近 stays default ink: its plain cells render outside any wrapper
	// (◇ is deliberately no state mark — UXG-0 D3 — so glyph and noun stay
	// ordinary cells with the companion space as a pinned 1ch run).
	adjacent := `<span class="trace-cell trace-cell-1">◇</span>` +
		`<span class="trace-run" style="width:1ch"> </span>` +
		`<span class="trace-cell trace-cell-2">邻</span>` +
		`<span class="trace-cell trace-cell-2">近</span>`
	at := strings.Index(html, adjacent)
	if at < 0 {
		t.Fatalf("adjacent channel word must keep its plain grid cells:\n%s", html)
	}
	if window := html[max(0, at-len(`<span class="elim-chain-word">`)):at]; strings.Contains(window, "elim-chain-word") {
		t.Fatalf("◇ 邻近 must not wear the chain ink wrapper:\n%s", html)
	}
	// Decoration only: pre textContent == fence bytes, byte for byte.
	body := strings.TrimPrefix(elimChainBoardZH, "```text trace-elim-overview\n")
	body = strings.TrimSuffix(body, "```\n")
	if got := preTextContentFrom(t, html, `<pre class="trace-projection-tree trace-elim-overview"`); got != body {
		t.Fatalf("textContent drifted from fence bytes\n--- fence ---\n%q\n--- textContent ---\n%q", body, got)
	}
}

func TestElimOverviewChainWordENForm(t *testing.T) {
	// EN word face transcribed from runtimeTraceProjElimChannelWord
	// (`⛓ on-chain`) and the en head composer (`⛓ on-chain block first …`).
	board := "```text trace-elim-overview\n" +
		"◎ Eliminable-in-window overview · ruler = app-42 in-window wall-clock ms\n" +
		"⛓ on-chain block first · value desc within block · zero ordinals · zero wear · locate via [E#] · bar full = section TOP1\n" +
		"   26.392ms ████████████ ⛓ on-chain · CookieMonsterCl-59843 · scheduling-pressure candidate [E7]\n" +
		"    6.000ms ██░░░░░░░░░░ ◇ adjacent · app-20 · IO wait [E9]\n" +
		"```\n"
	html, err := RenderMarkdownHTML([]byte(board))
	if err != nil {
		t.Fatal(err)
	}
	if got := strings.Count(html, `<span class="elim-chain-word">`); got != 2 {
		t.Fatalf("want exactly 2 elim-chain-word wrappers (header + 1 row), got %d:\n%s", got, html)
	}
	wrapped := `<span class="elim-chain-word">` +
		`<span class="trace-slot trace-icon trace-icon-io"><span class="trace-ink">⛓ </span></span>` +
		`<span class="trace-run" style="width:8ch">on-chain</span>` +
		`</span>`
	if got := strings.Count(html, wrapped); got != 2 {
		t.Fatalf("EN wrapped token must keep the slot + pinned ASCII run form, got %d of:\n%s\n---\n%s", got, wrapped, html)
	}
	if strings.Contains(html, `elim-chain-word"><span class="trace-cell trace-cell-1">◇`) {
		t.Fatalf("◇ adjacent must stay default ink:\n%s", html)
	}
}

func TestElimOverviewChainWordEscapeSafety(t *testing.T) {
	// A subject carrying raw <&> beside the token: everything still escapes
	// (all text leaves pass stdhtml.EscapeString) and textContent round-trips.
	board := "```text trace-elim-overview\n" +
		"◎ 窗内可消除量总览 · 尺=app 窗内墙钟ms\n" +
		"    5.000ms █░░░░░░░░░░░ ⛓ 链上 · a<b&c> · 调度压力候选 [E2]\n" +
		"```\n"
	html, err := RenderMarkdownHTML([]byte(board))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(html, "a&lt;b&amp;c&gt;") {
		t.Fatalf("raw <&> beside the wrapped token must stay escaped:\n%s", html)
	}
	if strings.Contains(html, "<b&c>") {
		t.Fatalf("unescaped fence bytes leaked into markup:\n%s", html)
	}
	if got := strings.Count(html, `<span class="elim-chain-word">`); got != 1 {
		t.Fatalf("want exactly 1 wrapper, got %d:\n%s", got, html)
	}
	body := "◎ 窗内可消除量总览 · 尺=app 窗内墙钟ms\n" +
		"    5.000ms █░░░░░░░░░░░ ⛓ 链上 · a<b&c> · 调度压力候选 [E2]\n"
	if got := preTextContentFrom(t, html, `<pre class="trace-projection-tree trace-elim-overview"`); got != body {
		t.Fatalf("textContent drifted\n--- fence ---\n%q\n--- textContent ---\n%q", body, got)
	}
}

// TestElimChainWordScopedToElimOverviewFence — the ink wrapper exists ONLY
// inside the typed ◎ fence. The projection tree fence, a plain text fence and
// the user-request fence render the very same token bytes with ZERO wrappers
// (their output is byte-independent of the ELIM-CHAN arm).
func TestElimChainWordScopedToElimOverviewFence(t *testing.T) {
	// Projection tree fence (typed token): grid rendering, no chain ink.
	tree := "```text trace-causal-projection\n" +
		"⊚ app-42 ‹用户关注线程› 满格=窗口10.000ms\n" +
		"│ ⧖ worker-7 · runnable 2.000ms 20% 根因排序#1 [E7]\n" +
		"│ ⛓ 链上 · 借词面一行\n" +
		"```\n"
	html, err := RenderMarkdownHTML([]byte(tree))
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(html, "elim-chain-word") {
		t.Fatalf("projection tree fence must not wear elim-chain-word:\n%s", html)
	}
	if !strings.Contains(html, `<span class="trace-slot trace-icon trace-icon-io"><span class="trace-ink">⛓ </span></span><span class="trace-cell trace-cell-2">链</span>`) {
		t.Fatalf("tree fence keeps the plain grid form for the same bytes:\n%s", html)
	}

	// Plain text fence (no typed token, no legacy signature): ordinary
	// escaped pre, untouched bytes.
	plain := "```text\n⛓ 链上 · 普通代码块一行\n```\n"
	html, err = RenderMarkdownHTML([]byte(plain))
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(html, "elim-chain-word") || strings.Contains(html, "trace-projection-tree") {
		t.Fatalf("plain text fence must stay an ordinary code block:\n%s", html)
	}
	if !strings.Contains(html, "⛓ 链上 · 普通代码块一行") {
		t.Fatalf("plain fence bytes must render verbatim-escaped:\n%s", html)
	}

	// User-request fence arm regression: wrap-enabled pre, no grid, no ink.
	userReq := "```text codrax-user-request\n请分析 ⛓ 链上 这行的含义\n```\n"
	html, err = RenderMarkdownHTML([]byte(userReq))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(html, `<pre class="user-request"><code`) {
		t.Fatalf("user-request fence arm regressed:\n%s", html)
	}
	if strings.Contains(html, "elim-chain-word") || strings.Contains(html, "trace-projection-tree") {
		t.Fatalf("user-request fence must not grid-render or ink the token:\n%s", html)
	}
}

// TestElimChainInkCSSDefinedForBothThemes — the ink variable is declared for
// the light :root, the dark scheme and the print palette, and the scoped rule
// reads it; color is the ONLY property (single encoding — no font-weight may
// ride this class, per the 2026-07-14 ruling correction).
func TestElimChainInkCSSDefinedForBothThemes(t *testing.T) {
	page, err := RenderStandaloneMarkdownHTML("t", []byte(elimChainBoardZH))
	if err != nil {
		t.Fatal(err)
	}
	if got := strings.Count(page, "--elim-chain-fg:"); got != 3 {
		t.Fatalf("want the ink variable declared exactly 3x (:root light, dark scheme, print), got %d", got)
	}
	rule := `pre.trace-elim-overview .elim-chain-word { color: var(--elim-chain-fg); }`
	if !strings.Contains(page, rule) {
		t.Fatalf("scoped color-only rule missing (want %q)", rule)
	}
	if strings.Contains(page, `.elim-chain-word { font-weight`) {
		t.Fatalf("single-encoding ruling: no font-weight on .elim-chain-word")
	}
}
