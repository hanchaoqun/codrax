package preview

// markdown_proj_channel_word_test.go — RNB-5B 件⑪ pins (§29.96.2 终判⑪,
// 2026-07-15: the projection TREE fence's channel seat words 「根因排序#N」/
// 「邻近影响#N」 gain HTML-layer ink via the ELIM-CHAN token-arm mechanism).
// Contract under test:
//
//   - the channel word half of a seat chip (noun immediately followed by its
//     #N ordinal — zh joined, en space-joined) wraps in ONE
//     <span class="proj-chain-word"> / <span class="proj-adjacent-word">
//     whose INNER markup is byte-identical to the unwrapped grid form —
//     pure ink, textContent and grid geometry untouched;
//   - prose/action-word mentions WITHOUT a following ordinal (未入根因排序前5,
//     bare channel nouns) never wear ink (座次词面 only);
//   - scope is the projection tree fence ONLY: the ◎ elim fence, plain text
//     fences and the user-request fence never mint these spans (非目标 fence
//     零染), and the elim empty-chain honest line keeps its existing
//     no-ink behavior (ELIM-CHAN pins);
//   - escape safety: textContent == fence bytes.
//
// MUTATION self-checks:
//   - unplugging the traceProjectionChannelWordToken arm (channelWords never
//     true) reds TestProjChannelWordWearsInk (zero wrappers);
//   - widening the scope to the elim fence reds
//     TestProjChannelWordScopedToTreeFence.

import (
	"strings"
	"testing"
)

const projChannelTreeZH = "```text trace-causal-projection\n" +
	"⊚ app-42 ‹用户关注线程› 满格=窗口10.000ms\n" +
	"│ ⧖ worker-7 · runnable 2.000ms 20% [E7]\n" +
	"│ · 调度压力候选·根因排序#1·置信中\n" +
	"    ⧖ logd-9 · runnable 1.000ms 10% [E9]\n" +
	"      · 调度压力候选·邻近影响#2·置信中\n" +
	"    ✦ jit-3 · 语义 0.500ms 可优化·未入根因排序前5 [E11]\n" +
	"```\n"

func TestProjChannelWordWearsInk(t *testing.T) {
	html, err := RenderMarkdownHTML([]byte(projChannelTreeZH))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(html, `<pre class="trace-projection-tree"`) {
		t.Fatalf("the tree fence must classify on its typed token:\n%s", html)
	}
	// Exactly ONE chain-word wrapper: the seat chip 根因排序#1. The
	// action-word mention 未入根因排序前5 has no following #N and stays bare.
	if got := strings.Count(html, `<span class="proj-chain-word">`); got != 1 {
		t.Fatalf("want exactly 1 proj-chain-word wrapper (the seat chip), got %d:\n%s", got, html)
	}
	// The wrapper's inner markup is the exact unwrapped CJK grid form.
	chainWrapped := `<span class="proj-chain-word">` +
		`<span class="trace-cell trace-cell-2">根</span>` +
		`<span class="trace-cell trace-cell-2">因</span>` +
		`<span class="trace-cell trace-cell-2">排</span>` +
		`<span class="trace-cell trace-cell-2">序</span>` +
		`</span>`
	if !strings.Contains(html, chainWrapped) {
		t.Fatalf("wrapped chain word must keep the exact grid cells:\n%s", html)
	}
	// The #1 ordinal keeps its existing chip span AFTER the word wrapper.
	if !strings.Contains(html, chainWrapped+`<span class="trace-rank-ordinal trace-rank-1 trace-rank-width-2">#1</span>`) {
		t.Fatalf("the ordinal chip must survive beside the inked word:\n%s", html)
	}
	// Exactly ONE adjacent-word wrapper (邻近影响#2) with the neutral class.
	if got := strings.Count(html, `<span class="proj-adjacent-word">`); got != 1 {
		t.Fatalf("want exactly 1 proj-adjacent-word wrapper, got %d:\n%s", got, html)
	}
	// textContent round-trips (decoration only).
	body := strings.TrimPrefix(projChannelTreeZH, "```text trace-causal-projection\n")
	body = strings.TrimSuffix(body, "```\n")
	if got := preTextContentFrom(t, html, `<pre class="trace-projection-tree"`); got != body {
		t.Fatalf("textContent drifted from fence bytes\n--- fence ---\n%q\n--- textContent ---\n%q", body, got)
	}
}

func TestProjChannelWordENForm(t *testing.T) {
	tree := "```text trace-causal-projection\n" +
		"⊚ app-42 <user-focused thread> bar full = window 10.000ms\n" +
		"│ ⧖ worker-7 · runnable 2.000ms 20% [E7]\n" +
		"│ · scheduling-pressure candidate · root-cause rank #1 · confidence mid\n" +
		"    ⧖ logd-9 · runnable 1.000ms [E9]\n" +
		"      · scheduling-pressure candidate · adjacent-impact #2 · confidence mid\n" +
		"```\n"
	html, err := RenderMarkdownHTML([]byte(tree))
	if err != nil {
		t.Fatal(err)
	}
	if got := strings.Count(html, `<span class="proj-chain-word">`); got != 1 {
		t.Fatalf("want exactly 1 EN chain-word wrapper, got %d:\n%s", got, html)
	}
	wrapped := `<span class="proj-chain-word">` +
		`<span class="trace-run" style="width:15ch">root-cause rank</span>` +
		`</span>`
	if !strings.Contains(html, wrapped) {
		t.Fatalf("EN wrapped word must keep the pinned ASCII run form:\n%s", html)
	}
	if got := strings.Count(html, `<span class="proj-adjacent-word">`); got != 1 {
		t.Fatalf("want exactly 1 EN adjacent-word wrapper, got %d:\n%s", got, html)
	}
}

// TestProjChannelWordScopedToTreeFence — 非目标 fence 零染: the ◎ elim fence,
// a plain text fence and the user-request fence render the same token bytes
// with ZERO proj-*-word wrappers.
func TestProjChannelWordScopedToTreeFence(t *testing.T) {
	elim := "```text trace-elim-overview\n" +
		"◎ 窗内可消除量总览 · 尺=app 窗内墙钟ms\n" +
		"    5.000ms █░░░░░░░░░░░ ⛓ 链上 · worker-7 · 调度压力候选 [E2]\n" +
		"· 词面对照:根因排序#1·邻近影响#2\n" +
		"```\n"
	html, err := RenderMarkdownHTML([]byte(elim))
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(html, "proj-chain-word") || strings.Contains(html, "proj-adjacent-word") {
		t.Fatalf("the elim fence must not mint proj channel-word spans:\n%s", html)
	}
	plain := "```text\n根因排序#1·邻近影响#2\n```\n"
	html, err = RenderMarkdownHTML([]byte(plain))
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(html, "proj-chain-word") || strings.Contains(html, "proj-adjacent-word") {
		t.Fatalf("a plain text fence must stay uninked:\n%s", html)
	}
	userReq := "```text codrax-user-request\n请解释 根因排序#1 的含义\n```\n"
	html, err = RenderMarkdownHTML([]byte(userReq))
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(html, "proj-chain-word") {
		t.Fatalf("the user-request fence must stay uninked:\n%s", html)
	}
}

// TestProjChannelWordCSSDeclared — the scoped rules exist, color is the ONLY
// property (single encoding, same discipline as the ELIM-CHAN rule).
func TestProjChannelWordCSSDeclared(t *testing.T) {
	page, err := RenderStandaloneMarkdownHTML("t", []byte(projChannelTreeZH))
	if err != nil {
		t.Fatal(err)
	}
	for _, rule := range []string{
		`pre.trace-projection-tree .proj-chain-word { color: var(--elim-chain-fg); }`,
		`pre.trace-projection-tree .proj-adjacent-word { color: var(--rank-adjacent-fg); }`,
	} {
		if !strings.Contains(page, rule) {
			t.Fatalf("scoped color-only rule missing (want %q)", rule)
		}
	}
	if strings.Contains(page, `.proj-chain-word { font-weight`) ||
		strings.Contains(page, `.proj-adjacent-word { font-weight`) {
		t.Fatalf("single encoding: no font-weight on the channel-word classes")
	}
}
