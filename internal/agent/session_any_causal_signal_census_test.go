package agent

import (
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// session_any_causal_signal_census_test.go — SIDECAR-Q1 (user ruling
// 2026-09-02, colleague_merge_audit §40.28 ②) turns the T3-1 ruling into a
// SIGNAL-LEVEL prohibition: the session-wide ANY aggregate
// (answerDocRuntimeTraceGuidanceView.CausalUnproven / FrameFlowUnproven — one
// exploratory zero-row probe sets it for the whole run) may feed ADVISORY prompt
// text only. Every hard decision and every public artifact reads the seat-level
// evidence-ID authority (tracefinding.SeatFrameCausalityIndex) instead. The
// PR23 sidecar contract had re-consumed the ANY signal one month after the
// crown face was fixed; this census makes the next such consumer red.
//
// 复核收编 (batch-one adversarial review, 2026-09-02): the allowlist is
// FUNCTION-scoped (file + enclosing top-level function), never file-scoped —
// answer_document_evaluator.go also hosts hard emit checks, so a new reject
// arm in an allowlisted file must still go red. The census also proves its
// own coverage: every allowlisted reader must be found (a stale allowlist is
// red, not silently green) and a walk/parse failure is a failure.
func TestSessionAnyCausalSignalFeedsAdvisoryLanesOnly(t *testing.T) {
	type reader struct{ file, fn string }
	allowed := map[reader]bool{
		// advisory prompt hint renderer + the view populator itself
		{"internal/agent/answer_document_evaluator.go", "renderAnswerDocRuntimeTraceAnswerGuidance"}: true,
		{"internal/agent/answer_document_evaluator.go", "answerDocRuntimeTraceGuidanceView"}:         true,
		// finalizer decision-boundary prose (advisory teaching, no gate)
		{"internal/agent/answer_document_final_decision_boundary.go", "renderAnswerDocTraceFinalDecisionBoundary"}: true,
		// A→B handoff prose (advisory)
		{"internal/agent/answer_document_trace_decision_handoff.go", "renderAnswerDocTraceDecisionHandoffSetWithAggregateFacts"}: true,
	}
	signals := map[string]bool{"CausalUnproven": true, "FrameFlowUnproven": true}
	roots := []string{"internal/agent", "internal/tool", "internal/orchestrator", "internal/analysis", "internal/outputdump", "internal/repl"}
	repo := filepath.Join("..", "..")
	var offenders []string
	seen := map[reader]bool{}
	filesWalked := 0
	fset := token.NewFileSet()
	for _, root := range roots {
		walkErr := filepath.WalkDir(filepath.Join(repo, root), func(path string, d os.DirEntry, err error) error {
			if err != nil {
				return err
			}
			if d.IsDir() || !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
				return nil
			}
			filesWalked++
			rel, relErr := filepath.Rel(repo, path)
			if relErr != nil {
				return relErr
			}
			rel = filepath.ToSlash(rel)
			body, readErr := os.ReadFile(path)
			if readErr != nil {
				return readErr
			}
			src := string(body)
			if !strings.Contains(src, ".CausalUnproven") && !strings.Contains(src, ".FrameFlowUnproven") {
				return nil
			}
			file, parseErr := parser.ParseFile(fset, path, src, 0)
			if parseErr != nil {
				return parseErr
			}
			for _, decl := range file.Decls {
				fn, ok := decl.(*ast.FuncDecl)
				if !ok || fn.Body == nil {
					continue
				}
				found := false
				ast.Inspect(fn.Body, func(n ast.Node) bool {
					if sel, ok := n.(*ast.SelectorExpr); ok && signals[sel.Sel.Name] {
						found = true
					}
					return !found
				})
				if !found {
					continue
				}
				key := reader{rel, fn.Name.Name}
				if allowed[key] {
					seen[key] = true
					continue
				}
				offenders = append(offenders, rel+":"+fn.Name.Name)
			}
			return nil
		})
		if walkErr != nil {
			t.Fatalf("census walk over %s failed (a silent green here would defeat the tripwire): %v", root, walkErr)
		}
	}
	if filesWalked < 100 {
		t.Fatalf("census walked only %d files — the repo root resolution (%q) is wrong", filesWalked, repo)
	}
	for key := range allowed {
		if !seen[key] {
			t.Fatalf("allowlisted reader %s:%s no longer reads the session-ANY signal — prune the allowlist instead of leaving a stale door", key.file, key.fn)
		}
	}
	if len(offenders) > 0 {
		t.Fatalf("session-ANY causal signal read outside the advisory allowlist (use tracefinding.SeatFrameCausalityIndex for any hard decision or public artifact): %v", offenders)
	}
}
