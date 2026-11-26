package orchestrator

import (
	"errors"
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"io"
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/agent"
	"github.com/hanchaoqun/codrax/internal/types"
)

// A forced dispatch that is not run is reported as not run: the log counts
// zero attempts, the step line carries the reader's notice instead of a
// failed model response, and no answer cue is shown for a dispatch that did
// not happen.
func TestForcedDispatchThatIsNotRunIsReportedAsNotRun(t *testing.T) {
	for _, tc := range []struct {
		lang, stepNotice, failedLabel, retryLabel, cue string
	}{
		{"zh", refusalStepNoticeZH, "未能生成最终答案", "模型响应出错", "4/4 正在生成最终答案"},
		{"en", refusalStepNoticeEN, "Could not compose answer", "Model response error", "4/4 Composing the final answer"},
	} {
		t.Run(tc.lang, func(t *testing.T) {
			run := runRefusalScenario(t, refusalRunOptions{language: tc.lang})
			if run.finalizerCalls != 1 {
				t.Fatalf("fixture: answer stage dispatched %d times, want 1", run.finalizerCalls)
			}
			if strings.Contains(run.log, "failed after 3 attempts") || strings.Contains(run.log, "dispatched attempt(s)") {
				t.Errorf("the log claims attempts although none was dispatched after the refusal:\n%s", lastLogLines(run.log, 12))
			}
			// The log keeps the raw error with its figures.
			if !strings.Contains(run.log, "forced finalize not dispatched (0 attempts)") || !strings.Contains(run.log, "messages and tool schemas=742912 bytes; allocation=700000 bytes") {
				t.Errorf("the log must say the forced dispatch was not run, and why:\n%s", lastLogLines(run.log, 12))
			}

			if !strings.Contains(run.rendered, tc.failedLabel+" · "+tc.stepNotice) {
				t.Errorf("the step must read as failed with the reader's notice:\n%s", run.rendered)
			}
			if strings.Contains(run.rendered, tc.retryLabel) {
				t.Errorf("no model response existed, yet the output says %q:\n%s", tc.retryLabel, run.rendered)
			}
			if got := strings.Count(run.rendered, tc.cue); got != 1 {
				t.Errorf("the answer cue appears %d times; only the one dispatch that ran may be announced:\n%s", got, run.rendered)
			}
			for _, leaked := range []string{"allocation", "content-byte", "agent finalizer", "742912", "700000"} {
				if strings.Contains(run.rendered, leaked) {
					t.Errorf("reader-facing output contains %q:\n%s", leaked, run.rendered)
				}
			}
		})
	}
}

// A forced dispatch that ran and failed is reported with the attempts that
// were dispatched, not with the size of its attempt budget.
func TestForcedDispatchFailureCountsTheAttemptsDispatched(t *testing.T) {
	t.Run("rebuilt after a drained read, refused again", func(t *testing.T) {
		run := runRefusalScenario(t, refusalRunOptions{
			repoFiles: lateReadRepo,
			finalizer: func(call int, ctx *types.AgentContext) (*agent.StageOutput, error) {
				if call == 1 {
					pendingLateRead(ctx)
				}
				return nil, refusedAnswerRequest()
			},
		})
		if run.finalizerCalls != 2 {
			t.Fatalf("fixture: answer stage dispatched %d times, want 2", run.finalizerCalls)
		}
		if !strings.Contains(run.log, "forced finalize failed after 1 dispatched attempt(s)") || strings.Contains(run.log, "not dispatched") {
			t.Errorf("one attempt was dispatched of a budget of three:\n%s", lastLogLines(run.log, 6))
		}
		if got := strings.Count(run.rendered, "4/4 正在生成最终答案"); got != 2 {
			t.Errorf("both dispatches ran and both must be announced, got %d cue(s):\n%s", got, run.rendered)
		}
		if strings.Contains(run.rendered, "模型响应出错") {
			t.Errorf("neither refusal was a failed model response:\n%s", run.rendered)
		}
	})

	t.Run("a dropped stream is retried once, then another failure ends it", func(t *testing.T) {
		run := runRefusalScenario(t, refusalRunOptions{
			finalizer: func(call int, _ *types.AgentContext) (*agent.StageOutput, error) {
				switch call {
				case 1:
					return nil, errors.New("provider rejected the answer request")
				case 2:
					return nil, fmt.Errorf("read stream: %w", io.ErrUnexpectedEOF)
				}
				return nil, errors.New("provider rejected the answer request")
			},
		})
		if run.finalizerCalls != 3 {
			t.Fatalf("fixture: answer stage dispatched %d times, want the failed one and two forced attempts\n%s", run.finalizerCalls, lastLogLines(run.log, 12))
		}
		if !strings.Contains(run.log, "forced finalize failed after 2 dispatched attempt(s): ") {
			t.Errorf("two attempts were dispatched of a budget of three:\n%s", lastLogLines(run.log, 6))
		}
		// A failure that is not a refused request keeps today's step line:
		// the error text, read as a failed model response.
		if !strings.Contains(run.rendered, "模型响应出错") || !strings.Contains(run.rendered, "provider rejected the answer request") {
			t.Errorf("a failed dispatch keeps its own step line:\n%s", run.rendered)
		}
		if strings.Contains(run.rendered, refusalStepNoticeZH) || run.bus.TaskState.LastError == refusalNoticeZH {
			t.Errorf("nothing was refused in this run:\n%s", run.rendered)
		}
	})
}

// The step line's detail: the reader's notice for a request that was never
// sent, the error text for everything else.
func TestAnswerStepFailureTextKeepsTheRawErrorOutOfTheRefusalCase(t *testing.T) {
	wrapped := fmt.Errorf("agent finalizer execution: %w", refusedAnswerRequest())
	if got := finalizeNodeFailureText(wrapped, "zh"); got != refusalStepNoticeZH {
		t.Errorf("zh step notice\n got: %s\nwant: %s", got, refusalStepNoticeZH)
	}
	if got := finalizeNodeFailureText(wrapped, "en"); got != refusalStepNoticeEN {
		t.Errorf("en step notice\n got: %s\nwant: %s", got, refusalStepNoticeEN)
	}
	other := errors.New("read stream: unexpected EOF")
	if got := finalizeNodeFailureText(other, "zh"); got != other.Error() {
		t.Errorf("another failure keeps its own text: %s", got)
	}
}

// The forced dispatch that is not run takes the same failure path as one that
// ran and failed: the standing refusal is the loop's error, so it reaches the
// no-visible-output fallback, the log and the failure-message chooser like
// any dispatch error. The arm that decides not to dispatch therefore only
// empties the attempt budget; it never reports or returns on its own.
func TestForcedDispatchThatIsNotRunTakesTheSharedFailurePath(t *testing.T) {
	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, "orchestrator.go", nil, 0)
	if err != nil {
		t.Fatal(err)
	}
	var (
		decision   *ast.IfStmt
		fallbacks  []token.Pos
		failureLog []token.Pos
		loop       token.Pos
	)
	ast.Inspect(file, func(n ast.Node) bool {
		switch node := n.(type) {
		case *ast.IfStmt:
			if assign, ok := node.Init.(*ast.AssignStmt); ok && len(assign.Rhs) == 1 && callsMethod(assign.Rhs[0], "repeatedAnswerRequestRefusal") {
				if decision != nil {
					t.Fatal("the standing refusal is consulted at one place")
				}
				decision = node
			}
		case *ast.ForStmt:
			if cond, ok := node.Cond.(*ast.BinaryExpr); ok {
				if limit, ok := cond.Y.(*ast.Ident); ok && limit.Name == "forceFinalizeMaxAttempts" {
					loop = node.Pos()
				}
			}
		case *ast.CallExpr:
			switch {
			case callsMethod(node, "finalizerNoVisibleOutputFallback"):
				fallbacks = append(fallbacks, node.Pos())
			case callsFunc(node, "logForcedFinalizeFailure"):
				failureLog = append(failureLog, node.Pos())
			}
		}
		return true
	})
	if decision == nil || loop == token.NoPos || len(failureLog) != 1 {
		t.Fatalf("vacuous: decision=%v loop=%v failure log sites=%d", decision != nil, loop != token.NoPos, len(failureLog))
	}
	if assign, ok := decision.Init.(*ast.AssignStmt); !ok || len(assign.Lhs) != 1 || identName(assign.Lhs[0]) != "err" || assign.Tok != token.ASSIGN {
		t.Fatal("the standing refusal must be assigned to the attempt loop's own err")
	}
	if len(decision.Body.List) != 1 {
		t.Fatalf("the not-dispatched arm only empties the attempt budget, found %d statements", len(decision.Body.List))
	}
	budget, ok := decision.Body.List[0].(*ast.AssignStmt)
	if !ok || len(budget.Lhs) != 1 || identName(budget.Lhs[0]) != "forceFinalizeMaxAttempts" || len(budget.Rhs) != 1 {
		t.Fatal("the not-dispatched arm must set the attempt budget and do nothing else")
	}
	if lit, ok := budget.Rhs[0].(*ast.BasicLit); !ok || lit.Value != "0" {
		t.Fatal("the not-dispatched arm must set the attempt budget to zero")
	}
	// decision → attempt loop → fallback → failure log, in source order.
	var fallbackAfterLoop token.Pos
	for _, pos := range fallbacks {
		if pos > loop && pos < failureLog[0] {
			fallbackAfterLoop = pos
		}
	}
	if !(decision.End() < loop && fallbackAfterLoop != token.NoPos) {
		t.Fatalf("order must be decision, attempt loop, no-visible-output fallback, failure log: decision=%s loop=%s fallback=%s log=%s",
			fset.Position(decision.Pos()), fset.Position(loop), fset.Position(fallbackAfterLoop), fset.Position(failureLog[0]))
	}
}

func callsMethod(expr ast.Expr, name string) bool {
	call, ok := expr.(*ast.CallExpr)
	if !ok {
		return false
	}
	sel, ok := call.Fun.(*ast.SelectorExpr)
	return ok && sel.Sel.Name == name
}

func callsFunc(expr ast.Expr, name string) bool {
	call, ok := expr.(*ast.CallExpr)
	if !ok {
		return false
	}
	return identName(call.Fun) == name
}

func identName(expr ast.Expr) string {
	if id, ok := expr.(*ast.Ident); ok {
		return id.Name
	}
	return ""
}
