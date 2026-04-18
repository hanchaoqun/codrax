package agent

import (
	"strings"
	"testing"
)

// TestExtractConcreteValues_CommentFilter exercises the comment-line
// pre-strip added on top of extractConcreteValues after the 2026-04-13
// baseline regression proved that comment text was being parsed as a
// binding call (see memory/project_baseline_2026_04_13_post_phase4.md).
//
// Invariants:
//  1. The exact comment from sub_explorer.go:94-96 must NOT emit a
//     `binds ONLY` entry.
//  2. A real `r.Register(NewFoo(deps))` call must still emit one.
//  3. An inline `// Real comment about Register(NewFoo())` must NOT.
//  4. Interior `/* Register(New... */` block comments must NOT.
//  5. `# comment with Register(NewFoo())` (shell/Python shape) must NOT.
//  6. Python `"""` docstring blocks containing binding-shaped text
//     must NOT.
//  7. `* continuation line inside block comment` must NOT.
func TestExtractConcreteValues_CommentFilter(t *testing.T) {
	type tc struct {
		name        string
		source      string
		wantBinds   bool // expect at least one "binds"/"binds ONLY" entry
		wantContain string
	}

	tests := []tc{
		{
			name: "phantom from sub_explorer.go comment must be filtered",
			source: `
// Cross-Run reset — sub_explorer is a process-lifetime singleton
// (NewSubExplorer called once from RegisterDefaults). Each
// SubAgentRequest is an independent scoped investigation, so
`,
			wantBinds: false,
		},
		{
			name: "real Register(NewFoo(deps)) still emits binds ONLY",
			source: `func RegisterDefaultSubAgents(r *Registry, deps *Deps) {
	r.Register(NewSubExplorer(deps))
}`,
			wantBinds:   true,
			wantContain: "NewSubExplorer",
		},
		{
			name: "inline // comment containing Register(New...) must not fire",
			source: `func noop() {
	// Register(NewFoo(deps)) — this is a comment, not a call
}`,
			wantBinds: false,
		},
		{
			name: "block comment /* Register(New... */ must not fire",
			source: `func noop() {
	/* Register(NewFoo(deps)) block comment */
}`,
			wantBinds: false,
		},
		{
			name: "multi-line block comment with continuation must not fire",
			source: `func noop() {
	/*
	 * Register(NewFoo(deps))
	 * continuation line
	 */
}`,
			wantBinds: false,
		},
		{
			name: "# shell/python comment must not fire",
			source: `def register():
    # Register(NewFoo(deps))
    pass`,
			wantBinds: false,
		},
		{
			name: "python docstring must not fire",
			source: `def register():
    """
    Register(NewFoo(deps)) described here.
    """
    pass`,
			wantBinds: false,
		},
		{
			name: "real call adjacent to a leading comment still fires",
			source: `func Register(r *R, d *D) {
	// bind the default sub-agent
	r.Register(NewSubExplorer(d))
}`,
			wantBinds:   true,
			wantContain: "NewSubExplorer",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := extractConcreteValues(tt.source, "go")
			hasBinds := false
			var values []string
			for _, e := range got {
				if strings.Contains(e.kind, "binds") {
					hasBinds = true
					values = append(values, e.value)
				}
			}
			if hasBinds != tt.wantBinds {
				t.Fatalf("hasBinds=%v, want=%v (entries=%+v)", hasBinds, tt.wantBinds, got)
			}
			if tt.wantContain != "" {
				joined := strings.Join(values, " | ")
				if !strings.Contains(joined, tt.wantContain) {
					t.Fatalf("binds value %q does not contain %q", joined, tt.wantContain)
				}
			}
		})
	}
}

// TestStripCommentLines_BlankingBehaviour exercises the helper directly
// to pin down the corner cases (block-state across lines, triple-quote
// entry/exit, preserving real code).
func TestStripCommentLines_BlankingBehaviour(t *testing.T) {
	in := []string{
		`x := 1`,                          // 0: code — keep
		`// comment`,                      // 1: strip
		`/* one-line block */`,            // 2: strip, no state
		`/* open`,                         // 3: strip, enter block
		` * continuation`,                 // 4: strip (in block)
		` */ y := 2`,                      // 5: strip, exit block (best-effort)
		`# shell`,                         // 6: strip
		`z := 3`,                          // 7: code — keep
		`"""`,                             // 8: strip, enter triple
		`inside docstring NewFoo(deps)`,   // 9: strip
		`"""`,                             // 10: strip, exit triple
		`w := 4`,                          // 11: code — keep
	}
	out := stripCommentLines(in)
	if len(out) != len(in) {
		t.Fatalf("length changed: got %d want %d", len(out), len(in))
	}
	keepIdx := map[int]string{0: `x := 1`, 7: `z := 3`, 11: `w := 4`}
	for i, got := range out {
		want, keep := keepIdx[i]
		if keep {
			if got != want {
				t.Errorf("line %d: got %q want %q", i, got, want)
			}
			continue
		}
		if got != "" {
			t.Errorf("line %d: expected blank, got %q", i, got)
		}
	}
}
