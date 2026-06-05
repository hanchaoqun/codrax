package types

import "testing"

func TestStripConversationPrefix(t *testing.T) {
	cases := []struct {
		name string
		in   string
		want string
	}{
		{"single-shot passes through", "how many agents", "how many agents"},
		{
			"REPL marker returns current only",
			"## Prior conversation\n### Recent conversation\n- You: old\n\n## Current request\nrepomap的作用",
			"repomap的作用",
		},
		{
			"trailing whitespace trimmed",
			"## Prior conversation\nx\n\n## Current request\nfoo\n\n",
			"foo",
		},
		{"empty input", "", ""},
		{"marker without trailing newline is not matched", "## Current request", "## Current request"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := StripConversationPrefix(c.in); got != c.want {
				t.Errorf("StripConversationPrefix(%q) = %q, want %q", c.in, got, c.want)
			}
		})
	}
}

func TestSplitConversation(t *testing.T) {
	cases := []struct {
		name        string
		in          string
		wantPrior   string
		wantCurrent string
	}{
		{
			"single-shot: no marker",
			"repomap的作用",
			"",
			"repomap的作用",
		},
		{
			"REPL with prior and current",
			"## Prior conversation\n### Recent conversation\n- You: old Q\n  Codrax: old A\n\n## Current request\nrepomap的作用",
			"## Prior conversation\n### Recent conversation\n- You: old Q\n  Codrax: old A",
			"repomap的作用",
		},
		{"empty input", "", "", ""},
		{
			"marker only with empty tail",
			"## Prior conversation\nx\n\n## Current request\n",
			"## Prior conversation\nx",
			"",
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			gotPrior, gotCurrent := SplitConversation(c.in)
			if gotPrior != c.wantPrior {
				t.Errorf("prior: got %q, want %q", gotPrior, c.wantPrior)
			}
			if gotCurrent != c.wantCurrent {
				t.Errorf("current: got %q, want %q", gotCurrent, c.wantCurrent)
			}
		})
	}
}

// TestIsContinuation pins the post-commit-48 contract: the function
// is a stable signature for back-compat, but ALL keyword + regex
// rules are gone. The LLM continuation_classifier (in package
// orchestrator) is authoritative. IsContinuation now returns:
//   - false when prior is empty (no continuation possible)
//   - false when current is empty (degenerate input)
//   - false otherwise (LLM classifier decides; this function is a
//     no-keyword-table back-compat shim)
//
// Pre-commit-48 a 30+ row truth table tested zh/en prefixes +
// pronoun regex + identifier-window rules. Operator feedback
// (2026-05-01) flagged the keyword list as a red-line violation
// AND dead-code-vs-LLM-classifier; commit 48 simplified.
func TestIsContinuation(t *testing.T) {
	prior := "Q: prior turn\nA: prior answer."
	cases := []struct {
		name    string
		current string
		prior   string
		want    bool
	}{
		{"non-empty inputs default to fresh", "再详细讲讲那个流程", prior, false},
		{"non-empty self-contained also fresh", "explorer 是怎么调用 sub-agent 的", prior, false},
		{"empty prior degenerate", "再详细讲讲", "", false},
		{"empty current degenerate", "", prior, false},
		{"both empty degenerate", "", "", false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := IsContinuation(c.current, c.prior); got != c.want {
				t.Errorf("IsContinuation(%q, %q) = %v, want %v", c.current, c.prior, got, c.want)
			}
		})
	}
}

func TestNormalizeREPLCommandAlias(t *testing.T) {
	cases := []struct {
		in   string
		want string
	}{
		{"/quit", "/quit"},
		{" /exit ", "/exit"},
		{"\\q", "/quit"},
		{"\\help", "/help"},
		{"/history now", "/history now"},
		{"how does explorer work", ""},
		// User-mode and write commands. /mode accepts only the new
		// user-facing values at execution time; prefix normalization
		// still keeps invalid /mode payloads out of the pipeline.
		{"/mode", "/mode"},
		{"/mode write", "/mode write"},
		{"/write fix typo", "/write fix typo"},
		{"/plan", "/plan"},
		{"/plan show", "/plan show"},
		{"/approve", "/approve"},
		{"/reject", "/reject"},
		{"/reject too narrow", "/reject too narrow"},
		{"/verify", "/verify"},
		{"/verify plan-123", "/verify plan-123"},
		{"/worktree", "/worktree"},
		{"/worktree list", "/worktree list"},
		{"\\plan", "/plan"},
		{"\\approve", "/approve"},
		{"\\worktree discard plan-1", "/worktree discard plan-1"},
		// Multi-repo /repos command (Phase 2).
		{"/repos", "/repos"},
		{"/repos focus my-repo-1a2b3c4d", "/repos focus my-repo-1a2b3c4d"},
		{"/repos refresh", "/repos refresh"},
		{"\\repos cap 5", "/repos cap 5"},
		{"/workflow", "/workflow"},
		{"/workflow show", "/workflow show"},
		{"\\workflow cancel", "/workflow cancel"},
	}
	for _, c := range cases {
		if got := NormalizeREPLCommandAlias(c.in); got != c.want {
			t.Errorf("NormalizeREPLCommandAlias(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}

func TestIsREPLControlInput(t *testing.T) {
	if !IsREPLControlInput("\\q") {
		t.Error(`\q must be treated as a REPL control input`)
	}
	if !IsREPLControlInput("/help") {
		t.Error(`/help must be treated as a REPL control input`)
	}
	if IsREPLControlInput("buildOrLoadGraph 是如何构建图的？") {
		t.Error("normal user question must not be treated as a control input")
	}
}
