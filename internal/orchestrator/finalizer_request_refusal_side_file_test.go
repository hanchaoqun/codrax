package orchestrator

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/agent"
	"github.com/hanchaoqun/codrax/internal/outputdump"
	"github.com/hanchaoqun/codrax/internal/types"
)

// The root-cause side file of a run without an answer names the capacity
// reason only when the answer stage's first request was refused and nothing
// of the stage reached the model. Every other run keeps the reason it has
// today. The frontend's reason is read from the same record.
func TestSideFileNamesTheCapacityReasonOnlyWhenNothingWasSent(t *testing.T) {
	const capacity, transcript = "final_answer_request_exceeds_capacity", "final_answer_transcript_not_available"
	for _, tc := range []struct {
		name       string
		opts       refusalRunOptions
		dispatches int
		want       string
	}{
		{"first request refused, forced dispatch not run", refusalRunOptions{}, 1, capacity},
		{"rebuilt after a drained read and refused again", refusalRunOptions{
			repoFiles: lateReadRepo,
			finalizer: func(call int, ctx *types.AgentContext) (*agent.StageOutput, error) {
				if call == 1 {
					pendingLateRead(ctx)
				}
				return nil, refusedAnswerRequest()
			},
		}, 2, capacity},
		{"repair request refused", refusalRunOptions{
			finalizer: func(call int, _ *types.AgentContext) (*agent.StageOutput, error) {
				if call == 1 {
					return nil, &agent.AnswerContextByteBudgetError{ActualBytes: 777916, AllocatedBytes: 700000, RequestsSent: 1}
				}
				return nil, refusedAnswerRequest()
			},
		}, 2, transcript},
		{"refused after a dispatch that failed otherwise", refusalRunOptions{
			finalizer: func(call int, _ *types.AgentContext) (*agent.StageOutput, error) {
				if call == 1 {
					return nil, errors.New("provider closed connection after the answer prompt")
				}
				return nil, refusedAnswerRequest()
			},
		}, 2, transcript},
		{"no refusal at all", refusalRunOptions{
			finalizer: func(int, *types.AgentContext) (*agent.StageOutput, error) {
				return nil, errors.New("provider closed connection after the answer prompt")
			},
		}, 2, transcript},
	} {
		t.Run(tc.name, func(t *testing.T) {
			tc.opts.traceSideFile = true
			run := runRefusalScenario(t, tc.opts)
			if run.finalizerCalls != tc.dispatches || run.bus.Mutable.Result() != "" {
				t.Fatalf("fixture: %d answer dispatches (want %d), result %q", run.finalizerCalls, tc.dispatches, run.bus.Mutable.Result())
			}
			got := run.sideFileReason(t)
			if got != tc.want {
				t.Errorf("side-file reason = %q, want %s", got, tc.want)
			}
			if frontend := run.orchestrator.UnformedAnswerRootCauseReason(); frontend != got {
				t.Errorf("the frontend's reason %q disagrees with the default side file %q", frontend, got)
			}
		})
	}

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
		if got := run.orchestrator.UnformedAnswerRootCauseReason(); got != transcript {
			t.Errorf("without a standing refusal the unformed-answer reason is the transcript one, got %q", got)
		}
	})

	var none *Orchestrator
	if got := none.UnformedAnswerRootCauseReason(); got != transcript {
		t.Errorf("no run: reason = %q, want %s", got, transcript)
	}
}

// The frontend's reason describes the last Run only. An orchestrator that
// serves several questions must not carry one run's refusal into the next.
func TestUnformedAnswerReasonIsReadPerRun(t *testing.T) {
	run := runRefusalScenario(t, refusalRunOptions{
		traceSideFile: true,
		finalizer: func(call int, _ *types.AgentContext) (*agent.StageOutput, error) {
			if call == 1 {
				return nil, refusedAnswerRequest()
			}
			return &agent.StageOutput{MissingPiece: types.MissingNone, FinalAnswer: "Source 在 src.go:1 定义。"}, nil
		},
	})
	if got := run.orchestrator.UnformedAnswerRootCauseReason(); run.finalizerCalls != 1 || got != "final_answer_request_exceeds_capacity" {
		t.Fatalf("fixture: the first run must end on the standing refusal: calls=%d reason=%q", run.finalizerCalls, got)
	}
	repo := t.TempDir()
	if err := os.WriteFile(filepath.Join(repo, "src.go"), []byte("package src\n\nfunc Source() {}\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	bus, err := run.orchestrator.Run("解释 Source", repo, "main")
	if err != nil || bus == nil || !strings.Contains(bus.Mutable.Result(), "Source") {
		t.Fatalf("fixture: the second run must be answered: err=%v", err)
	}
	if got := run.orchestrator.UnformedAnswerRootCauseReason(); got != "final_answer_transcript_not_available" {
		t.Errorf("the earlier run's refusal leaked into the next run's reason: %q", got)
	}
}

// Negative pin: the refusal is terminal bookkeeping for the scheduler, the
// reader and the side file. Neither a side-file reason code nor the reader
// notice nor the refused size is handed to the next answer dispatch.
func TestNothingAboutARefusalIsHandedToTheNextAnswerDispatch(t *testing.T) {
	var secondDispatch string
	run := runRefusalScenario(t, refusalRunOptions{
		traceSideFile: true,
		repoFiles:     lateReadRepo,
		finalizer: func(call int, ctx *types.AgentContext) (*agent.StageOutput, error) {
			if call == 1 {
				pendingLateRead(ctx)
			} else {
				secondDispatch = modelFacingDump(t, ctx)
			}
			return nil, refusedAnswerRequest()
		},
	})
	if run.finalizerCalls != 2 || len(secondDispatch) < 200 {
		t.Fatalf("fixture must capture the second dispatch's context: calls=%d captured=%d bytes", run.finalizerCalls, len(secondDispatch))
	}
	if !strings.Contains(secondDispatch, "late.go") {
		t.Fatal("vacuous: the captured context must carry what the stage is handed, including the drained read")
	}
	banned := append(outputdump.AllRootCauseUnavailableReasonCodes(),
		refusalStepNoticeZH, refusalStepNoticeEN, "742912", string(types.FinalizerRefusalFirstRequest), "request_exceeds_capacity")
	for _, text := range banned {
		if strings.Contains(secondDispatch, text) {
			t.Errorf("the next answer dispatch was handed %q", text)
		}
	}
}
