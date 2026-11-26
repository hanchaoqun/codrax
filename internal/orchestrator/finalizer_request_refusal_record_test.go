package orchestrator

import (
	"errors"
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/agent"
	"github.com/hanchaoqun/codrax/internal/types"
)

// Every answer dispatch leaves the run's refusal record in step with how it
// returned: a typed record when the byte guard refused the request, none
// otherwise. The record's kind says whether anything of the answer stage
// reached the model.
func TestRefusedAnswerRequestIsRecordedWithWhatReachedTheModel(t *testing.T) {
	logLine := regexp.MustCompile(`answer request refused before sending: kind=first_request request_bytes=742912 allocation_bytes=700000 requests_sent=0 input_digest=[0-9a-f]{64}\b`)

	t.Run("first request refused, nothing sent", func(t *testing.T) {
		run := runRefusalScenario(t, refusalRunOptions{})
		record := run.bus.Mutable.FinalizerRequestRefusal()
		if !record.NothingSent() || record.Kind != types.FinalizerRefusalFirstRequest ||
			record.RequestBytes != 742912 || record.AllocationBytes != 700000 || record.RequestsSent != 0 || len(record.InputDigest) != 64 {
			t.Fatalf("the run must keep the typed record of the refusal: %+v", record)
		}
		if !logLine.MatchString(run.log) {
			t.Errorf("the operator log must carry the record:\n%s", lastLogLines(run.log, 8))
		}
	})

	t.Run("repair request refused", func(t *testing.T) {
		run := runRefusalScenario(t, refusalRunOptions{
			finalizer: func(call int, _ *types.AgentContext) (*agent.StageOutput, error) {
				if call == 1 {
					return nil, &agent.AnswerContextByteBudgetError{ActualBytes: 777916, AllocatedBytes: 700000, RequestsSent: 1}
				}
				return nil, refusedAnswerRequest()
			},
		})
		if !strings.Contains(run.log, "kind=repair request_bytes=777916 allocation_bytes=700000 requests_sent=1") {
			t.Errorf("the refused repair request must be recorded as one:\n%s", lastLogLines(run.log, 12))
		}
		// The forced dispatch was refused at its own first request, but
		// requests of this run had reached the model.
		record := run.bus.Mutable.FinalizerRequestRefusal()
		if record == nil || record.Kind != types.FinalizerRefusalLaterRequest || record.NothingSent() || record.RequestsSent != 0 {
			t.Fatalf("a run that sent answer requests has no nothing-sent refusal: %+v", record)
		}
	})

	t.Run("refusal after another failed dispatch", func(t *testing.T) {
		run := runRefusalScenario(t, refusalRunOptions{
			finalizer: func(call int, _ *types.AgentContext) (*agent.StageOutput, error) {
				if call == 1 {
					return nil, errors.New("provider closed connection after the answer prompt")
				}
				return nil, refusedAnswerRequest()
			},
		})
		record := run.bus.Mutable.FinalizerRequestRefusal()
		if record == nil || record.NothingSent() {
			t.Fatalf("a dispatch that failed otherwise may have sent a request: %+v", record)
		}
	})

	t.Run("answered after a refusal", func(t *testing.T) {
		run := runRefusalScenario(t, refusalRunOptions{
			repoFiles: lateReadRepo,
			finalizer: func(call int, ctx *types.AgentContext) (*agent.StageOutput, error) {
				if call == 1 {
					pendingLateRead(ctx)
					return nil, refusedAnswerRequest()
				}
				return &agent.StageOutput{MissingPiece: types.MissingNone, FinalAnswer: "Source 在 src.go:1 定义。"}, nil
			},
		})
		if got := run.bus.Mutable.Result(); !strings.Contains(got, "Source") {
			t.Fatalf("fixture must deliver the second dispatch's answer: %q", got)
		}
		if record := run.bus.Mutable.FinalizerRequestRefusal(); record != nil {
			t.Fatalf("the record describes the run's last answer dispatch only: %+v", record)
		}
	})
}

func TestAnswerDispatchRecordingToleratesAMissingRun(t *testing.T) {
	wrapped := fmt.Errorf("agent finalizer execution: %w", refusedAnswerRequest())
	var none *Orchestrator
	none.recordFinalizeDispatchResult(wrapped)
	(&Orchestrator{}).recordFinalizeDispatchResult(wrapped)
	(&Orchestrator{busCtx: &types.BusContext{}}).recordFinalizeDispatchResult(wrapped)
	if finalizeRefusalOf(nil) != nil || finalizeRefusalOf(&types.BusContext{}) != nil {
		t.Fatal("a run without state has no refusal record")
	}
	o := &Orchestrator{busCtx: &types.BusContext{Mutable: types.NewMutableState("question")}}
	o.recordFinalizeDispatchResult(wrapped)
	if record := finalizeRefusalOf(o.busCtx); !record.NothingSent() || record.RequestBytes != 742912 {
		t.Fatalf("the wrapped typed error must be recorded: %+v", record)
	}
	o.recordFinalizeDispatchResult(nil)
	if finalizeRefusalOf(o.busCtx) != nil {
		t.Fatal("a dispatch that was not refused clears the record")
	}
}

// A state the digest cannot read still gets its record; the log says the
// digest is unavailable and names what could not be read.
func TestRefusalOverAStateThatCannotBeSampledIsRecordedWithoutADigest(t *testing.T) {
	readLog := captureOperatorLog(t)
	o := &Orchestrator{busCtx: unsampleableBus()}
	o.recordFinalizeDispatchResult(refusedAnswerRequest())
	record := finalizeRefusalOf(o.busCtx)
	if !record.NothingSent() || record.InputDigest != "" || record.RequestBytes != 742912 {
		t.Fatalf("the refusal is recorded without a digest: %+v", record)
	}
	if log := readLog(); !strings.Contains(log, "requests_sent=0 input_digest=unavailable (state.repairExecutionPlan: unsupported type") {
		t.Errorf("the log must say the digest is unavailable and why:\n%s", lastLogLines(log, 4))
	}
}

// The record is only as good as its coverage: an answer dispatch that does
// not go through the recording entry could send requests the record never
// hears of. The entry is the one place that builds an answer dispatch.
func TestEveryAnswerDispatchGoesThroughTheRecordingEntry(t *testing.T) {
	entries, err := filepath.Glob("*.go")
	if err != nil {
		t.Fatal(err)
	}
	fset := token.NewFileSet()
	builders, entryCalls := map[string]int{}, 0
	var direct []string
	for _, name := range entries {
		if strings.HasSuffix(name, "_test.go") {
			continue
		}
		src, err := os.ReadFile(name)
		if err != nil {
			t.Fatal(err)
		}
		file, err := parser.ParseFile(fset, name, src, 0)
		if err != nil {
			t.Fatal(err)
		}
		ast.Inspect(file, func(n ast.Node) bool {
			call, ok := n.(*ast.CallExpr)
			if !ok {
				return true
			}
			switch fn := call.Fun.(type) {
			case *ast.Ident:
				if fn.Name == "newFinalizeStageExecutionRequest" {
					builders[name]++
				}
			case *ast.SelectorExpr:
				switch fn.Sel.Name {
				case "executeFinalizeStageRequest":
					entryCalls++
				case "dispatchStage":
					if len(call.Args) == 1 {
						if stage, ok := call.Args[0].(*ast.SelectorExpr); ok && stage.Sel.Name == "StageFinalize" {
							direct = append(direct, fset.Position(call.Pos()).String())
						}
					}
				}
			}
			return true
		})
	}
	if len(direct) != 0 {
		t.Fatalf("the answer stage is dispatched past the recording entry at %v", direct)
	}
	if len(builders) != 1 || builders["finalizer_request_refusal.go"] != 1 {
		t.Fatalf("answer dispatches must be built only inside executeFinalizeStageRequest, found builders in %v", builders)
	}
	if entryCalls < 2 {
		t.Fatalf("vacuous: the scheduler dispatches the answer stage through the entry at %d site(s)", entryCalls)
	}
}
