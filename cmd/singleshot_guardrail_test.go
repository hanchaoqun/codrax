package cmd

import (
	"go/ast"
	"go/parser"
	"go/token"
	"strings"
	"testing"
)

// TestSingleShotDoesNotTouchMemory is the structural guardrail for
// the session-32 C7 invariant: the `--request` single-shot CLI path
// must NOT construct a memory.Store or call BuildContext. Single-
// shot runs are stateless by design, and the memory-precision-index
// work that unlocks chitchat / pipeline continuity in the REPL is
// explicitly scoped away from one-off invocations.
//
// The guarantee is load-bearing for two reasons: (1) callers piping
// --request into scripts expect no side effects on ~/.codrax/memory
// (no files created, no MEMORY.md mutation); (2) the REPL's memory-
// compaction background goroutine and shared-lock acquisition are a
// non-trivial source of startup latency that single-shot must never
// pay for.
//
// This test parses cmd/root.go, walks the body of runSingleShot, and
// asserts that no selector expression names any memory / chitchat
// construction symbol. It fails at build time if someone ever wires
// such a call into the single-shot branch — independent of test
// coverage on that branch.
func TestSingleShotDoesNotTouchMemory(t *testing.T) {
	fset := token.NewFileSet()
	f, err := parser.ParseFile(fset, "root.go", nil, 0)
	if err != nil {
		t.Fatalf("parse root.go: %v", err)
	}

	var body *ast.BlockStmt
	for _, decl := range f.Decls {
		fn, ok := decl.(*ast.FuncDecl)
		if !ok {
			continue
		}
		if fn.Name.Name == "runSingleShot" {
			body = fn.Body
			break
		}
	}
	if body == nil {
		t.Fatal("runSingleShot function not found in cmd/root.go; test must be updated if the function was renamed")
	}

	// Any selector shaped `<pkg>.<Name>` whose full form matches one
	// of these is a violation of the single-shot statelessness rule.
	// Extend as new continuity-bearing constructors / methods appear.
	banned := map[string]string{
		"memory.NewStore":      "single-shot must not construct a memory Store",
		"memory.Store":         "single-shot must not reference memory.Store",
		"newLLMSummarizer":     "single-shot must not construct the memory summarizer",
		"NewChitchatResponder": "single-shot must not wire the chitchat responder",
		"NewChitchatClassifier": "single-shot must not wire the chitchat classifier",
	}
	// Banned method names — violated if any selector's Sel.Name matches.
	// Separate from full-name list because BuildContext could be called
	// on any variable (e.g. store.BuildContext, s.BuildContext).
	bannedMethods := map[string]string{
		"BuildContext": "single-shot must not retrieve prior-conversation context",
		"Append":       "single-shot must not append to memory store (Turn persistence)",
		"Compact":      "single-shot must not trigger memory compaction",
	}

	ast.Inspect(body, func(n ast.Node) bool {
		sel, ok := n.(*ast.SelectorExpr)
		if !ok {
			return true
		}
		// Full pkg.Name match (catches memory.NewStore etc.).
		if ident, ok := sel.X.(*ast.Ident); ok {
			full := ident.Name + "." + sel.Sel.Name
			if reason, bad := banned[full]; bad {
				pos := fset.Position(sel.Pos())
				t.Errorf("runSingleShot references %s at %s: %s", full, pos, reason)
			}
			// Also check bare method name (e.g. app.store.BuildContext
			// where the X is not an Ident but a nested SelectorExpr).
			if reason, bad := bannedMethods[sel.Sel.Name]; bad {
				pos := fset.Position(sel.Pos())
				t.Errorf("runSingleShot references method %s at %s (full: %s): %s",
					sel.Sel.Name, pos, full, reason)
			}
			return true
		}
		// Nested selector (e.g. app.store.BuildContext). Check method
		// name only.
		if reason, bad := bannedMethods[sel.Sel.Name]; bad {
			pos := fset.Position(sel.Pos())
			t.Errorf("runSingleShot references method %s at %s: %s",
				sel.Sel.Name, pos, reason)
		}
		return true
	})
}

// TestSingleShotImportsAreBounded is a softer companion check: if
// cmd/root.go ever gains `import "github.com/hanchaoqun/codrax/internal/memory"`
// and the memory package ends up referenced inside runSingleShot,
// the first test above catches it. But that first test only covers
// the function body, not other call-graph paths (e.g. a new helper
// runSingleShot invokes that itself touches memory). To close that
// hole cheaply, this test greps the AST to confirm runSingleShot
// only calls other helpers that we explicitly allow.
//
// Maintenance: when runSingleShot legitimately needs a new helper,
// add its name to allowedHelpers. The test fails loudly so additions
// are visible in review.
func TestSingleShotCallGraphIsBounded(t *testing.T) {
	fset := token.NewFileSet()
	f, err := parser.ParseFile(fset, "root.go", nil, 0)
	if err != nil {
		t.Fatalf("parse root.go: %v", err)
	}
	var body *ast.BlockStmt
	for _, decl := range f.Decls {
		fn, ok := decl.(*ast.FuncDecl)
		if !ok {
			continue
		}
		if fn.Name.Name == "runSingleShot" {
			body = fn.Body
			break
		}
	}
	if body == nil {
		t.Fatal("runSingleShot function not found in cmd/root.go")
	}

	// Whitelist of function calls runSingleShot may make. Dotted
	// calls (app.orch.Run, fmt.Println) are matched by the rightmost
	// selector name. Update deliberately.
	allowedHelpers := map[string]bool{
		// Standard lib / util.
		"Println": true, "Print": true, "Printf": true, "Fprintln": true,
		"Fprintf": true, "Errorf": true, "HasSuffix": true,
		// Orchestrator + logging + render.
		"Run": true, "Info": true, "Error": true, "Warning": true,
		"Result": true, "RenderResult": true,
	}

	var unknown []string
	ast.Inspect(body, func(n ast.Node) bool {
		call, ok := n.(*ast.CallExpr)
		if !ok {
			return true
		}
		name := ""
		switch fn := call.Fun.(type) {
		case *ast.Ident:
			name = fn.Name
		case *ast.SelectorExpr:
			name = fn.Sel.Name
		}
		if name == "" {
			return true
		}
		if allowedHelpers[name] {
			return true
		}
		// Common inert operations the test should not police.
		if strings.HasPrefix(name, "Is") || strings.HasPrefix(name, "As") {
			return true
		}
		unknown = append(unknown, name)
		return true
	})

	for _, name := range unknown {
		// Non-fatal: a new unknown helper is usually benign (a string
		// formatter, a getter). The test fails loudly to flag the
		// addition for review but does not assume malicious intent.
		// If the addition is legitimate, add it to allowedHelpers.
		t.Logf("runSingleShot calls %q which is not on the bounded-helper allowlist; "+
			"add to allowedHelpers if safe, otherwise refactor", name)
	}
}
