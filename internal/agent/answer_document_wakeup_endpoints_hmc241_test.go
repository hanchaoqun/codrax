package agent

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	ctxbuilder "github.com/hanchaoqun/codrax/internal/context"
	"github.com/hanchaoqun/codrax/internal/skill"
	"github.com/hanchaoqun/codrax/internal/tool"
	"github.com/hanchaoqun/codrax/internal/types"
)

// The public HMC240 input already published each endpoint's TGID in the typed
// event. Its display handoff must not drop that identity or fill a missing
// target TGID from an independently known waker/nearby emitter.
func TestHMC241WakeupEndpointPublicFinalizer(t *testing.T) {
	raw, err := os.ReadFile("../../eval/fixtures/hmosperf_event_participants/wakeups.systrace")
	if err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		name, trace, targetProcess         string
		loaderTGID, workerTGID, targetTGID int
	}{
		{"known", string(raw), "20", 10, 30, 20},
		{"unknown_target_not_waker_or_neighbor", strings.ReplaceAll(string(raw), "target-42 (20)", "target-42 (-----)"), "未记录", 10, 30, 0},
		{"renamed_independent_processes", strings.NewReplacer("loader", "reader", "worker", "decoder", "target", "consumer", "(10)", "(701)", "(30)", "(702)", "(20)", "(703)").Replace(string(raw)), "703", 701, 702, 703},
	} {
		t.Run(tc.name, func(t *testing.T) {
			dir := t.TempDir()
			path := filepath.Join(dir, "wakeups.systrace")
			if err := os.WriteFile(path, []byte(tc.trace), 0600); err != nil {
				t.Fatal(err)
			}
			start, end := 1.0, 1.020
			params, _ := json.Marshal(map[string]any{"source": "path", "path": path, "view": "wakeup_chain", "pid": 42, "time_start": start, "time_end": end, "trace_flavor": "harmony_hitrace"})
			result, err := (&tool.TraceQuery{}).Execute(&types.BusContext{RepoRoot: dir, WorkDir: dir}, params)
			if err != nil || !result.Success {
				t.Fatalf("public query: %v %s", err, result.Summary)
			}
			target := "target-42"
			if tc.name == "renamed_independent_processes" {
				target = "consumer-42"
			}
			ctx := tracePrincipalValueAuthorityTestContext(target, 42, result.Observations)
			ctx.Language, ctx.AgentName, ctx.Stage = "zh", types.AgentFinalizer, types.StageFinalize
			ctx.Mutable.SetTurnAArtifacts(types.TurnAArtifacts{ToolResults: []types.ToolResult{result}})
			ctx.AnalysisIR.RequestModel.RuntimeQuestionProfile = &types.RuntimeQuestionProfile{Scope: types.RuntimeQuestionScopeRelationAnalysis}
			ctx.AnalysisIR.RequestModel.RuntimeArtifactScopeProfile = &types.RuntimeArtifactScopeProfile{RequestedScope: types.RuntimeArtifactScopeExplicitWindow, TimeStart: &start, TimeEnd: &end}
			ctx.Mutable.SetRequestModel(ctx.AnalysisIR.RequestModel)
			ctx = ctxbuilder.BuildAgentContext(&types.BusContext{RepoRoot: dir, WorkDir: dir, Language: "zh", Mutable: ctx.Mutable, AnalysisIR: ctx.AnalysisIR}, types.AgentFinalizer, types.StageFinalize)
			before, _ := json.Marshal(result.Observations)
			projectionBefore, _ := json.Marshal(types.TraceCausalProjectionFromObservationRecords(result.Observations))
			instruction := (&answerDocumentEvaluator{}).BuildInitialInstruction(ctx, nil)
			prompt := ctxbuilder.BuildPromptContext(ctx, &skill.Config{Name: "wakeup-endpoint-test"})
			waitHandoff := ""
			for _, section := range prompt.UserSections {
				if section.Title == ctxbuilder.SectionTraceWaitEvidence {
					waitHandoff = section.Content
				}
			}
			ledger := types.ObservationLedger{Records: result.Observations}
			relations := tool.RuntimeDiagramRelations(ledger, &ctx.AnalysisIR.RequestModel)
			if len(relations) != 3 {
				t.Fatalf("three target events required, got %d", len(relations))
			}
			count := 0
			for _, record := range result.Observations {
				if record.Predicate != "wakeup_chain_edge" {
					continue
				}
				count++
				f, ok := types.DecodeRuntimeWakeupEvent(record)
				if !ok || f.Wakee.PID != 42 || f.Wakee.TGID != tc.targetTGID {
					t.Fatalf("target process is not independently observed: valid=%t event=%+v", ok, f)
				}
				wantWakerTGID := tc.loaderTGID
				if f.Waker.PID == 13 {
					wantWakerTGID = tc.workerTGID
				}
				if f.Waker.TGID != wantWakerTGID {
					t.Fatalf("waker identity changed: %+v", f)
				}
				want := "发起线程所属进程 TGID=" + hmc241Decimal(wantWakerTGID) + "；被唤醒线程所属进程 TGID=" + tc.targetProcess
				for face, text := range map[string]string{"public_tool": result.Summary, "edge_summary": record.Summary, "finalizer_instruction": instruction, "wait_handoff": waitHandoff} {
					if !strings.Contains(text, want) {
						t.Errorf("%s lost endpoint process facts %q", face, want)
					}
				}
				matched := false
				for _, relation := range relations {
					if relation.FromLabel == record.Subject && relation.ToLabel == record.Object && strings.Contains(relation.ScopeLabel, want) {
						matched = true
					}
				}
				if !matched {
					t.Errorf("native relation recipe lost endpoint process facts %q", want)
				}
			}
			if count != 3 {
				t.Fatalf("edge population changed: %d", count)
			}
			after, _ := json.Marshal(result.Observations)
			projectionAfter, _ := json.Marshal(types.TraceCausalProjectionFromObservationRecords(result.Observations))
			if string(before) != string(after) || string(projectionBefore) != string(projectionAfter) {
				t.Fatal("display changed source observations or causal projection")
			}
		})
	}
}

func hmc241Decimal(v int) string {
	b, _ := json.Marshal(v)
	return string(b)
}
