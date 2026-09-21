package agent

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	ctxbuilder "github.com/hanchaoqun/codrax/internal/context"
	"github.com/hanchaoqun/codrax/internal/tool"
	"github.com/hanchaoqun/codrax/internal/types"
)

// The ordinary timeline preview only lists twelve scheduler intervals. Its
// complete target wait inventory must still reach both exploration and the
// finite final-answer input without requiring a second window_stats query.
func TestWindowlessTimelineDeliversCompleteWaitsToFinalizer(t *testing.T) {
	fixture, err := filepath.Abs("../../eval/fixtures/real_traces/donghu_tieba_frame.systrace")
	if err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(fixture)
	if err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	queryBus := &types.BusContext{RepoRoot: dir, WorkDir: dir,
		AttachedHitrace: string(data), AttachedHitraceSource: "harmony_hitrace",
		RuntimeArtifactPreflight: types.RuntimeArtifactPreflightProfile{Artifacts: []types.RuntimeArtifactPreflightArtifact{{Kind: "trace", Source: fixture, Carrier: "attachment"}}},
	}
	result, err := (&tool.TraceQuery{}).Execute(queryBus, json.RawMessage(`{"source":"attached_trace","view":"thread_timeline","pid":59566,"trace_flavor":"harmony_hitrace"}`))
	if err != nil || !result.Success {
		t.Fatalf("timeline failed: %v; %s", err, result.Summary)
	}
	for _, want := range []string{
		"state_totals intervals=243", "target_d_io_wait_occurrence_roster status=complete",
		"d_state=0 io_wait=3 sleep_iowait=0", "wall_clock_sum=0.635ms",
		"ordinal=3 state=io_wait window=34579.471372..34579.471722 duration=0.350ms",
	} {
		if !strings.Contains(result.Summary, want) {
			t.Errorf("first tool face lost %q", want)
		}
	}
	if strings.Count(result.Summary, types.TraceSchedulerWaitPartitionTeaching) != 1 {
		t.Error("first tool face lost or repeated shared D/IO meaning")
	}
	for _, lang := range []string{"zh", "en"} {
		t.Run(lang, func(t *testing.T) {
			ctx := tracePrincipalValueAuthorityTestContext("com.baidu.tieba-59566", 59566, result.Observations)
			ctx.Mutable.SetTurnAArtifacts(types.TurnAArtifacts{ToolResults: []types.ToolResult{result}})
			rm := &ctx.AnalysisIR.RequestModel
			rm.Intent, rm.Language = types.IntentExplain, lang
			rm.RuntimeQuestionProfile = &types.RuntimeQuestionProfile{Scope: types.RuntimeQuestionScopeBoundedFactSet,
				FactFamilies: []types.RuntimeQuestionFactFamily{types.RuntimeQuestionFactTargetSchedulerState, types.RuntimeQuestionFactTargetWaitOccurrences}}
			rm.RuntimeArtifactScopeProfile = &types.RuntimeArtifactScopeProfile{RequestedScope: types.RuntimeArtifactScopeFullArtifact}
			ctx.Mutable.SetRequestModel(*rm)
			ctx.Mutable.SetAnswerDocumentV2WithMutation(types.MutationReplaceAll, &types.AnswerDocumentV2{DocumentModel: "v2", Blocks: []types.AnswerBlock{{ID: "model", Kind: types.BlockSummary, Text: "Model-owned answer remains unchanged."}}})
			ctx = ctxbuilder.BuildAgentContext(&types.BusContext{RepoRoot: dir, WorkDir: dir, Language: lang, Mutable: ctx.Mutable, AnalysisIR: ctx.AnalysisIR}, types.AgentFinalizer, types.StageFinalize)
			ledger := answerDocObservationLedger(ctx)
			waits := types.BuildTraceTargetWaitSummaryAuthorities(ledger, &ctx.AnalysisIR.RequestModel)
			if len(waits) != 1 {
				t.Fatalf("complete wait authority unavailable: %+v", waits)
			}
			wait := waits[0]
			if wait.Count != 3 || wait.DStateOccurrences != 0 || wait.IOWaitOccurrences != 3 || wait.SleepIOWaitOccurrences != 0 || wait.WallClockMS < .6349 || wait.WallClockMS > .6351 {
				t.Fatalf("wait identity/count/ruler changed: %+v", wait)
			}
			before, _ := json.Marshal([]any{result, ledger, ctx.AnalysisIR.RequestModel, ctx.Mutable.AnswerDocumentV2()})
			instruction := (&answerDocumentEvaluator{}).BuildInitialInstruction(ctx, nil)
			for _, want := range []string{"d_state_occurrences=0; io_wait_occurrences=3; sleep_iowait_occurrences=0;", "wall_clock_sum=0.635ms", "principal_occurrence=`#3 state=io_wait", types.TraceSchedulerWaitPartitionTeaching} {
				if !strings.Contains(instruction, want) {
					t.Errorf("actual finite final input lost %q", want)
				}
			}
			after, _ := json.Marshal([]any{result, answerDocObservationLedger(ctx), ctx.AnalysisIR.RequestModel, ctx.Mutable.AnswerDocumentV2()})
			if string(before) != string(after) {
				t.Fatal("delivery changed query facts, user scope, or model answer")
			}
		})
	}
}
