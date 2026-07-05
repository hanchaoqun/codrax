package render

import (
	"os"
	"strings"
	"testing"

	"github.com/charmbracelet/glamour"
	"github.com/yuin/goldmark/parser"
)

// ---------------------------------------------------------------------------
// RFH #66 residual ① — terminal face: single "~" must never strike.
// ---------------------------------------------------------------------------

// Customer-shaped regression: prose uses a lone "~" as a range connector
// (Chinese numeric-interval convention). glamour v1.0.0's built-in GFM
// strikethrough opened SGR 9 on a single tilde, so two ranges in one
// paragraph struck through everything between them AND consumed the "~"
// bytes (same defect the HTML face fixed in internal/preview; specimen:
// .codrax/output/20260705-181128.544-80798.html).
const terminalTildeRangeProse = "com.baidu.tieba 59566 主线程在 34579.472865s~34579.587805s 的 114.940ms 帧窗口内共出现 4 个卡顿子窗口,唤醒链每层延迟 6~11ms。"

func TestTerminalMarkdownSingleTildeRangeStaysLiteral(t *testing.T) {
	r := New(nil, true) // forceColor: ANSI on even on non-TTY test runner
	out := r.RenderMarkdown(terminalTildeRangeProse)
	// No SGR 9 (crossed-out) may appear anywhere in the render.
	// termenv emits the attribute either standalone ("\x1b[9m") or as
	// a ";9m" suffix on a combined sequence; assert both spellings.
	if strings.Contains(out, "\x1b[9m") || strings.Contains(out, ";9m") {
		t.Fatalf("single-tilde range connectors must never render struck-through (SGR 9):\n%q", out)
	}
	plain := stripAnsiEscapes(out)
	if !strings.Contains(plain, "34579.472865s~34579.587805s") ||
		!strings.Contains(plain, "6~11ms") {
		t.Fatalf("tilde ranges must survive literally in terminal prose:\n%q", plain)
	}
}

// ~~ behavior recorded as-is: double tilde KEEPS its GFM strikethrough
// semantics on the terminal face — the markers are consumed and the
// span renders with the crossed-out attribute (Strikethrough.CrossedOut
// stays true; see style_palette_test.go). This pin also blocks the
// rejected "CrossedOut=false" fix direction, which would have silently
// degraded ~~ while STILL eating single-tilde bytes at the parser.
func TestTerminalMarkdownDoubleTildeStillStruck(t *testing.T) {
	r := New(nil, true)
	out := r.RenderMarkdown("前一版结论 ~~全帧丢失~~ 已被修正。")
	if !strings.Contains(out, "\x1b[9m") && !strings.Contains(out, ";9m") {
		t.Fatalf("double-tilde must keep GFM strikethrough (SGR 9) on the terminal face:\n%q", out)
	}
	plain := stripAnsiEscapes(out)
	if strings.Contains(plain, "~~") {
		t.Fatalf("double-tilde markers must be consumed by the renderer:\n%q", plain)
	}
	if !strings.Contains(plain, "全帧丢失") {
		t.Fatalf("struck content must stay visible:\n%q", plain)
	}
}

func TestTerminalMarkdownMixedSingleAndDoubleTilde(t *testing.T) {
	r := New(nil, true)
	out := r.RenderMarkdown("区间 3~5ms 与 ~~已废弃~~ 并存,另一区间 7~9ms。")
	plain := stripAnsiEscapes(out)
	if !strings.Contains(plain, "3~5ms") || !strings.Contains(plain, "7~9ms") {
		t.Fatalf("single-tilde ranges corrupted in mixed prose:\n%q", plain)
	}
	if !strings.Contains(out, "\x1b[9m") && !strings.Contains(out, ";9m") {
		t.Fatalf("double-tilde span lost in mixed prose:\n%q", out)
	}
}

// ---------------------------------------------------------------------------
// RFH #66 residual ② — terminal face: raw-HTML-shaped tokens stay literal.
// ---------------------------------------------------------------------------

// glamour's ANSI renderer feeds RawHTML/HTMLBlock nodes through
// bluemonday.StrictPolicy, which silently DELETED prose tokens like
// "<anonymous>" (JS stack frames) and "Vec<int>" (generics) — the
// terminal twin of the HTML face's "raw HTML omitted" defect. The
// terminal pipeline therefore does not parse raw HTML at all: the
// bytes stay Text nodes and render verbatim. A terminal has no HTML
// execution surface, so literal display is exactly as safe as
// deletion — minus the information loss.
func TestTerminalMarkdownRawHTMLTokensStayLiteral(t *testing.T) {
	r := New(nil, true)
	out := r.RenderMarkdown("崩溃栈顶帧 <anonymous> 出现在 Vec<int> 的泛型实例化路径。")
	plain := stripAnsiEscapes(out)
	for _, want := range []string{"<anonymous>", "Vec<int>"} {
		if !strings.Contains(plain, want) {
			t.Fatalf("terminal face must keep raw-HTML-shaped token %q literal (was silently stripped pre-RFH#66):\n%q", want, plain)
		}
	}
}

func TestTerminalMarkdownHTMLBlockStaysLiteral(t *testing.T) {
	r := New(nil, true)
	out := r.RenderMarkdown("前置说明。\n\n<script>alert(1)</script>\n\n后置说明。")
	plain := stripAnsiEscapes(out)
	if !strings.Contains(plain, "<script>alert(1)</script>") {
		t.Fatalf("block-level raw HTML must display literally on the terminal face (info preserved, no execution surface):\n%q", plain)
	}
}

// ---------------------------------------------------------------------------
// Parity pins: the shadow goldmark instance must not drift from glamour.
// ---------------------------------------------------------------------------

// terminalParityCorpus exercises every element family the terminal
// renderer styles. It deliberately contains NO single-tilde connector
// and NO raw-HTML-shaped token — outside the two ruled deviations the
// shadow pipeline must be byte-identical to stock glamour.
const terminalParityCorpus = `# Title H1

## Section H2 标题

### Sub H3

#### H4

##### H5

###### H6

**Strong** with *emphasis*, ~~struck~~, ` + "`inline code`" + `, [a link](https://example.com), <https://example.com/auto>, <user@example.com>, www.example.com, an entity &amp; and CJK 中文混排。

> Block quote with ` + "`code`" + ` inside.

- Unordered one
- Unordered two

1. Ordered first
2. Ordered second

- [x] Task done
- [ ] Task pending

| Col A | Col B |
| ----- | ----- |
| a1    | b1 中文 |
| a2    | b2    |

` + "```go" + `
func main() { fmt.Println("hi") }
` + "```" + `

` + "```" + `
plain fence body
` + "```" + `

Term
: definition body

---

![alt text](https://example.com/img.png)

Hard break line one` + "  " + `
line two after hard break.
`

func TestTerminalMarkdownByteParityWithGlamour(t *testing.T) {
	cfg := codraxStyleConfig()
	gr, err := glamour.NewTermRenderer(
		glamour.WithStyles(cfg),
		glamour.WithWordWrap(0),
	)
	if err != nil {
		t.Fatalf("glamour.NewTermRenderer: %v", err)
	}
	want, err := gr.Render(terminalParityCorpus)
	if err != nil {
		t.Fatalf("glamour render: %v", err)
	}
	got, err := newTerminalMarkdown(cfg).Render(terminalParityCorpus)
	if err != nil {
		t.Fatalf("terminal markdown render: %v", err)
	}
	if got != want {
		t.Fatalf("terminal markdown output drifted from glamour for the parity corpus\n--- glamour (%d bytes) ---\n%q\n--- shadow (%d bytes) ---\n%q",
			len(want), want, len(got), got)
	}
}

// Drift guard against goldmark upgrades: the local parser lists must be
// exactly goldmark's defaults minus the HTML block parser (block
// priority 900) and the raw HTML inline parser (inline priority 400).
// If a goldmark upgrade adds/removes/renumbers default parsers this
// fails loudly instead of silently diverging from stock parsing.
func TestTerminalParserListsTrackGoldmarkDefaults(t *testing.T) {
	defBlock := parser.DefaultBlockParsers()
	gotBlock := terminalBlockParsers()
	var wantBlock []int
	for _, v := range defBlock {
		if v.Priority == 900 { // NewHTMLBlockParser — intentionally removed
			continue
		}
		wantBlock = append(wantBlock, v.Priority)
	}
	var haveBlock []int
	for _, v := range gotBlock {
		haveBlock = append(haveBlock, v.Priority)
	}
	if len(haveBlock) != len(wantBlock) {
		t.Fatalf("terminalBlockParsers must be goldmark defaults minus priority 900; got %v, want %v", haveBlock, wantBlock)
	}
	for i := range wantBlock {
		if haveBlock[i] != wantBlock[i] {
			t.Fatalf("terminalBlockParsers priority[%d] = %d, want %d (defaults drifted?)", i, haveBlock[i], wantBlock[i])
		}
	}

	defInline := parser.DefaultInlineParsers()
	gotInline := terminalInlineParsers()
	var wantInline []int
	for _, v := range defInline {
		if v.Priority == 400 { // NewRawHTMLParser — intentionally removed
			continue
		}
		wantInline = append(wantInline, v.Priority)
	}
	var haveInline []int
	for _, v := range gotInline {
		haveInline = append(haveInline, v.Priority)
	}
	if len(haveInline) != len(wantInline) {
		t.Fatalf("terminalInlineParsers must be goldmark defaults minus priority 400; got %v, want %v", haveInline, wantInline)
	}
	for i := range wantInline {
		if haveInline[i] != wantInline[i] {
			t.Fatalf("terminalInlineParsers priority[%d] = %d, want %d (defaults drifted?)", i, haveInline[i], wantInline[i])
		}
	}
}

// Source guard: names the exact regression vectors so a failure is
// self-explaining (mirrors internal/preview's guard for the HTML face).
func TestTerminalMarkdownSourceGuards(t *testing.T) {
	stripComments := func(src string) string {
		var code []string
		for _, line := range strings.Split(src, "\n") {
			if i := strings.Index(line, "//"); i >= 0 {
				line = line[:i]
			}
			code = append(code, line)
		}
		return strings.Join(code, "\n")
	}

	mt, err := os.ReadFile("markdown_terminal.go")
	if err != nil {
		t.Fatalf("read markdown_terminal.go: %v", err)
	}
	mtCode := stripComments(string(mt))
	for _, banned := range []string{
		"extension.GFM",           // bundles single-tilde Strikethrough
		"extension.Strikethrough", // single-tilde <del>/SGR 9
		"NewRawHTMLParser",        // re-swallows <anonymous>/Vec<int>
		"NewHTMLBlockParser",      // re-swallows block-level raw HTML
	} {
		if strings.Contains(mtCode, banned) {
			t.Fatalf("markdown_terminal.go must not use %s (RFH #66 rulings; see file header)", banned)
		}
	}
	if !strings.Contains(mtCode, "markdownext.StrikethroughDoubleTildeOnly") {
		t.Fatalf("markdown_terminal.go lost markdownext.StrikethroughDoubleTildeOnly (ruling 2026-07-05: single ~ never strikes)")
	}

	rd, err := os.ReadFile("renderer.go")
	if err != nil {
		t.Fatalf("read renderer.go: %v", err)
	}
	rdCode := stripComments(string(rd))
	if strings.Contains(rdCode, "glamour.NewTermRenderer") {
		t.Fatalf("renderer.go must not construct glamour.NewTermRenderer: its hardcoded extension.GFM re-enables single-tilde strikethrough and bluemonday raw-HTML stripping; use newTerminalMarkdown (markdown_terminal.go)")
	}
}
