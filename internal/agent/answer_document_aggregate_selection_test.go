package agent

import (
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/tool"
	"github.com/hanchaoqun/codrax/internal/types"
)

// Reproduce the actual analyzer classification, not a profile-free renderer
// unit test: execute a public query and carry it through TurnA into finalizer.
func TestProcessProfileBoundedNamedPublicFinalContext(t *testing.T) {
	path, _ := filepath.Abs("../../eval/fixtures/hmosperf_process_profile/events.systrace")
	for _, spelling := range []string{"ui-10 (tid=10)", "ui [10]", "ui 10", "tid=10"} {
		t.Run(spelling, func(t *testing.T) {
			start, end := 1., 1.02
			ctx := &types.AgentContext{RepoRoot: t.TempDir(), WorkDir: t.TempDir(), Language: "zh", AgentName: types.AgentFinalizer, Stage: types.StageFinalize, Mutable: types.NewMutableState("进程概览"), AnalysisIR: &types.AnalysisIR{RequestModel: types.RequestModel{
				Language: "zh", PerfTrace: &types.PerfBundle{},
				RuntimeTargets:              []types.RuntimeTarget{{Kind: types.RuntimeTargetKindThread, Thread: spelling, Source: "user_explicit", Confidence: .95}},
				RuntimeQuestionProfile:      &types.RuntimeQuestionProfile{Scope: types.RuntimeQuestionScopeBoundedFactSet, FactFamilies: []types.RuntimeQuestionFactFamily{types.RuntimeQuestionFactTargetSchedulerState, types.RuntimeQuestionFactCountOrDuration, types.RuntimeQuestionFactRecordedReason}},
				RuntimeArtifactScopeProfile: &types.RuntimeArtifactScopeProfile{RequestedScope: types.RuntimeArtifactScopeExplicitWindow, TimeStart: &start, TimeEnd: &end, SourceQuote: "1..1.02"},
			}}}
			params, _ := json.Marshal(map[string]any{"source": "path", "path": path, "view": "process_profile", "time_start": start, "time_end": end})
			r, err := (&tool.TraceQuery{}).Execute(types.ToolBusContext(ctx, types.AgentExplorer), params)
			if err != nil || !r.Success {
				t.Fatalf("query: %v %+v", err, r)
			}
			ctx.Mutable.AppendDispatchToolResult(r)
			ctx.Mutable.SetTurnAArtifacts(types.TurnAArtifacts{ToolResults: ctx.Mutable.DispatchToolResults()})
			prompt := (&answerDocumentEvaluator{}).BuildInitialInstruction(ctx, nil)
			for _, want := range []string{"### 已观测进程概览", "已观测成员=3", `"pid":12`, `"unknown_ms":20`, "Load", "submit_bio", "不画成唤醒因果链"} {
				if !strings.Contains(prompt, want) {
					t.Errorf("lost %q with real bounded profile", want)
				}
			}
			var record types.ObservationRecord
			for _, candidate := range r.Observations {
				if candidate.Predicate == tool.TraceProcessProfilePredicate {
					record = candidate
				}
			}
			for _, mutate := range []func(*types.ObservationRecord){
				func(r *types.ObservationRecord) { r.SourceRef.Path += ".other" },
				func(r *types.ObservationRecord) { r.SourceRef.QueryWindowStartTs = 0 },
				func(r *types.ObservationRecord) { r.SourceRef.QueryTargetPID = 11 },
				func(r *types.ObservationRecord) { r.SourceRef.QueryTargetScope = "process" },
				func(r *types.ObservationRecord) { r.Role = types.AnswerAggregateRolePrincipalAnswer },
			} {
				bad := record
				mutate(&bad)
				if got := answerDocScopeProjectedObservationRecords(ctx, []types.ObservationRecord{bad}); len(got) != 0 {
					t.Errorf("retained mismatched aggregate %+v", bad.SourceRef)
				}
			}
			peer := record
			peer.Subject, peer.Predicate, peer.ClaimKey, peer.RichNotes = "worker-11", "target_window_states", "target_window_states", nil
			if len(answerDocScopeProjectedObservationRecords(ctx, []types.ObservationRecord{peer})) != 0 {
				t.Fatal("query selector laundered unrelated peer facts")
			}
			own := peer
			own.Subject = "ui-10"
			if len(answerDocScopeProjectedObservationRecords(ctx, []types.ObservationRecord{own})) != 1 {
				t.Fatal("own states lost due to identity spelling")
			}
			start, end = 1.005, 1.015
			if strings.Contains((&answerDocumentEvaluator{}).BuildInitialInstruction(ctx, nil), "### 已观测进程概览") {
				t.Fatal("wider profile borrowed into narrower request")
			}
		})
	}
}
