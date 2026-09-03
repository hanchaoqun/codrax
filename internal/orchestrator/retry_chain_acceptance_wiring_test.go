package orchestrator

import (
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"testing"
)

// retry_chain_acceptance_wiring_test.go — §40.14 V7-2 复核: the production
// reset pair is wired, not merely defined. acceptFinalizeNode must call
// closeFinalizeRetryChain, and the scheduler's finalize acceptance exits must
// go through acceptFinalizeNode (the census only proves a caller exists).
func TestFinalizeAcceptanceExitsCloseTheRetryChain(t *testing.T) {
	calls := func(file, fn string) map[string]int {
		t.Helper()
		src, err := os.ReadFile(file)
		if err != nil {
			t.Fatal(err)
		}
		f, err := parser.ParseFile(token.NewFileSet(), file, src, 0)
		if err != nil {
			t.Fatal(err)
		}
		out := map[string]int{}
		for _, decl := range f.Decls {
			fd, ok := decl.(*ast.FuncDecl)
			if !ok || fd.Body == nil || (fn != "" && fd.Name.Name != fn) {
				continue
			}
			ast.Inspect(fd.Body, func(n ast.Node) bool {
				call, ok := n.(*ast.CallExpr)
				if !ok {
					return true
				}
				if sel, ok := call.Fun.(*ast.SelectorExpr); ok {
					out[sel.Sel.Name]++
				} else if id, ok := call.Fun.(*ast.Ident); ok {
					out[id.Name]++
				}
				return true
			})
		}
		return out
	}
	if calls("retry_state.go", "acceptFinalizeNode")["closeFinalizeRetryChain"] != 1 {
		t.Fatal("acceptFinalizeNode must close the finalize retry chain exactly once")
	}
	if n := calls("orchestrator.go", "")["acceptFinalizeNode"]; n < 4 {
		t.Fatalf("the scheduler's finalize acceptance exits must go through acceptFinalizeNode (found %d calls, want ≥ 4)", n)
	}
}
