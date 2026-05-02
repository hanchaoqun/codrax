package render

import (
	"fmt"
	"strings"
	"testing"
)

// TestReproOriginalFailure is a one-shot repro of the user-reported
// failure case from the 2026-05-02 log: a flowchart with `<br/>`
// HTML and `→` arrow inside CJK labels that previously froze the
// CJK adapter. After L1+L2 we expect a fully-rendered ASCII grid.
//
// This test prints the output for visual inspection during a normal
// test run; failure conditions are explicit so a CI pass is
// meaningful too.
func TestReproOriginalFailure(t *testing.T) {
	in := "```mermaid\nflowchart LR\n" +
		"    Renderer --> dockRowState[\"dockRowState<br/>构建\"]\n" +
		"    dockRowState --> composeDockRows[\"composeDockRows\"]\n" +
		"    composeDockRows --> composeDockRow1[\"composeDockRow1<br/>活动行\"]\n" +
		"    commitToScrollback --> clearAndWrite[\"清除→写入→重绘\"]\n" +
		"```"
	ResetMermaidStats()
	out := RenderMermaidBlocks(in)
	if out == in {
		t.Fatalf("original failure case must now render; got unchanged source")
	}
	if !strings.Contains(out, "```text\n") {
		t.Errorf("output must rewrap fence as ```text```; got:\n%s", out)
	}
	if strings.Contains(out, "```mermaid") {
		t.Errorf("output must NOT keep a ```mermaid``` tag (chroma deception); got:\n%s", out)
	}
	for _, want := range []string{"清除", "写入", "重绘", "->"} {
		if !strings.Contains(out, want) {
			t.Errorf("expected token %q in rendered output; got:\n%s", want, out)
		}
	}
	s := GetMermaidStats()
	if s.Succeeded < 1 {
		t.Errorf("expected ≥1 success; got %d", s.Succeeded)
	}
	if s.FallbackSubstituted < 1 {
		t.Errorf("expected ≥1 fallback block (the → rune triggered narrow-fallback); got %d", s.FallbackSubstituted)
	}
	fmt.Println("=== rendered output ===")
	fmt.Println(out)
	fmt.Printf("=== stats === attempts=%d succeeded=%d fallback_blocks=%d fallback_runes=%d library_rejected=%d\n",
		s.Attempts, s.Succeeded, s.FallbackSubstituted, s.FallbackRunes, s.LibraryRejected)
}
