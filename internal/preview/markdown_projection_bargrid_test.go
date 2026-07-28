package preview

import (
	"strings"
	"testing"

	"github.com/mattn/go-runewidth"
)

// markdown_projection_bargrid_test.go — BARGRID-1 (customer report
// 2026-07-28, HTML 场景): the projection tree's bar blocks rendered with
// visible gaps between the SOLID cells (实体块没有紧紧挨着). Root cause: the
// grid cell width digit came from runewidth's package-level DefaultCondition,
// which follows the SERVER's locale — under a zh_CN environment the
// East-Asian-AMBIGUOUS blocks █ (U+2588) and ▒ (U+2592) measure 2 while
// ░ (U+2591) measures 1, so the HTML face emitted trace-cell-2 boxes whose
// 1ch of block ink left a blank column after every solid cell. The grid
// ruler is the FENCE GEOMETRY and must be locale-INVARIANT: the C.3/C-9
// design says bar blocks are per-rune 1ch cells, always.
func TestTraceProjectionGridGeometryIsLocaleInvariant(t *testing.T) {
	saved := runewidth.DefaultCondition
	defer func() { runewidth.DefaultCondition = saved }()
	// Simulate the customer's zh_CN server environment: ambiguous-width
	// runes measure 2 under the package-level default condition.
	runewidth.DefaultCondition = &runewidth.Condition{EastAsianWidth: true}

	fenceBody := "⧖ app-20 · runnable ██▒▒░░░░ 6.000ms 33% [E1]\n"
	html, err := RenderMarkdownHTML([]byte("```text trace-causal-projection\n" + fenceBody + "```\n"))
	if err != nil {
		t.Fatal(err)
	}
	for _, cell := range []string{"█", "▒", "░"} {
		want := `<span class="trace-cell trace-cell-1 trace-bar">` + cell + `</span>`
		if !strings.Contains(html, want) {
			t.Fatalf("bar cell %q must stay a 1ch box regardless of server locale (blank columns between solid blocks otherwise):\n%s", cell, html)
		}
		if strings.Contains(html, `trace-cell-2 trace-bar">`+cell) {
			t.Fatalf("bar cell %q leaked a locale-dependent 2ch box:\n%s", cell, html)
		}
	}
	// True CJK wides keep their 2ch cells — the invariant ruler is the
	// narrow condition, not a flatten-everything-to-1ch rewrite.
	cjk, err := RenderMarkdownHTML([]byte("```text trace-causal-projection\n⧖ 线程 ██░░ 1.000ms\n```\n"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(cjk, `trace-cell-2">线</span>`) {
		t.Fatalf("true CJK runes must keep 2ch cells:\n%s", cjk)
	}
}
