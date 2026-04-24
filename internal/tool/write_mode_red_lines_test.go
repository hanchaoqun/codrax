package tool

import (
	"go/ast"
	"go/parser"
	"go/token"
	"testing"
)

// TestWriteModeToolsDontGroundCitations is the session-33 R1
// structural red-line test. Locks the L3 invariant:
//
//   B0 write-mode tools MUST NOT call ground.BuildContext or
//   ground.GroundItem (or any other ground.* symbol).
//
// Why: those helpers canonicalise paths against ctx.RepoRoot and
// reconstruct citation line-indexes from tool-result history. In
// write mode RepoRoot may be a worktree path whose TurnAArtifacts
// were indexed under the main-repo root; running the grounder on
// those artifacts would silently drop or mis-anchor citations. The
// correct design — and the one B0 ships — keeps write tools
// orthogonal to the read-mode citation pipeline. ChangePlan /
// TestResult / ApplyPatch payloads are structured proposals and
// verdicts, not evidence that needs grounding.
//
// Enforcement shape: parse each write-mode tool file as Go AST,
// walk every SelectorExpr, and fail if any reference resolves to
// the ground package. go/ast gives us zero runtime cost and
// zero dependency on whether the ground package is even imported
// (the test reads source strings).
//
// When the B2 / B3 tools land for real (apply_patch, run_tests,
// emit_test_results), this test must continue to pass — it is
// enforcement, not documentation, of the L3 red line. If a future
// change genuinely needs grounding inside a write-mode tool, the
// first step is a design document explaining why the L3 boundary
// moves.
func TestWriteModeToolsDontGroundCitations(t *testing.T) {
	// The four B0 write-mode tool files. Adding a new write-mode
	// tool means extending this slice — a conscious step for the
	// code author that signals "my new tool also falls under L3".
	writeModeFiles := []string{
		"emit_change_plan.go",
		"apply_patch.go",
		"run_tests.go",
		"emit_test_results.go",
	}

	// Banned identifier names on the ground package's side of a
	// SelectorExpr. Keep this list tight: only symbols that
	// perform the grounding (canonicalise + lookup) belong here,
	// not helper utilities that happen to live in package
	// ground. If the ground package grows new public symbols
	// that must stay off-limits to write-mode tools, add them
	// here.
	bannedGroundSymbols := map[string]bool{
		"BuildContext": true,
		"GroundItem":   true,
	}

	for _, filename := range writeModeFiles {
		t.Run(filename, func(t *testing.T) {
			fset := token.NewFileSet()
			file, err := parser.ParseFile(fset, filename, nil, parser.ParseComments)
			if err != nil {
				t.Fatalf("parse %s: %v", filename, err)
			}
			// Guard: the import list itself must not pull in
			// ground. Even if none of its symbols are referenced,
			// an import is a signal that the author was at least
			// considering grounding — fail loudly to surface the
			// intent review.
			for _, imp := range file.Imports {
				// Path is quoted, e.g. `"github.com/.../internal/tool/ground"`.
				if imp.Path == nil {
					continue
				}
				p := imp.Path.Value
				if p == `"github.com/hanchaoqun/codrax/internal/tool/ground"` {
					t.Errorf("%s imports internal/tool/ground (L3 red line: write-mode tools must not depend on grounding)",
						filename)
				}
			}

			// Walk the AST for any SelectorExpr whose left side
			// is the identifier `ground`. This catches calls like
			// ground.BuildContext(ctx) / ground.GroundItem(...)
			// without needing type resolution. Zero false-negatives
			// because a raw string match on "ground.BuildContext"
			// elsewhere in the file would still require an import
			// to compile — and we already flagged the import.
			ast.Inspect(file, func(n ast.Node) bool {
				sel, ok := n.(*ast.SelectorExpr)
				if !ok {
					return true
				}
				ident, ok := sel.X.(*ast.Ident)
				if !ok {
					return true
				}
				if ident.Name != "ground" {
					return true
				}
				if bannedGroundSymbols[sel.Sel.Name] {
					pos := fset.Position(sel.Pos())
					t.Errorf("%s references ground.%s at %s (L3 red line: write-mode tools must not call grounding helpers)",
						filename, sel.Sel.Name, pos)
				} else {
					// Any other ground.* reference is also
					// suspicious — flag at lower severity.
					pos := fset.Position(sel.Pos())
					t.Errorf("%s references ground.%s at %s (L3 red line: write-mode tools must not reference the ground package at all; extend bannedGroundSymbols if this was intentional)",
						filename, sel.Sel.Name, pos)
				}
				return true
			})
		})
	}
}
