package agent

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	ctxbuilder "github.com/hanchaoqun/codrax/internal/context"
	"github.com/hanchaoqun/codrax/internal/skill"
	"github.com/hanchaoqun/codrax/internal/tool"
	"github.com/hanchaoqun/codrax/internal/types"
)

// Run the customer fixture through the public tool and the actual finalizer
// instruction builder. Edge values describe the right-hand endpoint's wait;
// the left-hand endpoint has a separately measured state account.
func TestWakeupWaitOwnerPublicFinalizer(t *testing.T) {
	data, err := os.ReadFile("../../eval/cases/trace_query_wakeup_causal_io_chain.case")
	if err != nil {
		t.Fatal(err)
	}
	_, fixture, ok := strings.Cut(string(data), "HTRACE='")
	if !ok {
		t.Fatal("trace absent")
	}
	fixture, _, ok = strings.Cut(fixture, "\n'\n")
	if !ok {
		t.Fatal("trace not closed")
	}
	for _, state := range []string{"io_wait", "d_sleep", "s_sleep"} {
		for _, rename := range []bool{false, true} {
			t.Run(fmt.Sprintf("%s/renamed=%t", state, rename), func(t *testing.T) {
				trace := fixture
				if state != "io_wait" {
					trace = strings.ReplaceAll(trace, "iowait=1", "iowait=0")
				}
				if state == "s_sleep" {
					trace = strings.ReplaceAll(trace, "prev_state=D", "prev_state=S")
				}
				names := []string{"threadpool", "network", "cookie", "app"}
				if rename {
					for i, name := range names {
						names[i] = []string{"worker", "transport", "broker", "client"}[i]
						trace = strings.ReplaceAll(trace, name, names[i])
					}
				}
				dir := t.TempDir()
				path := filepath.Join(dir, "chain.ftrace")
				if err := os.WriteFile(path, []byte(trace), 0600); err != nil {
					t.Fatal(err)
				}
				start, end := 2.0, 2.020
				params, _ := json.Marshal(map[string]any{"source": "path", "path": path, "view": "root_cause_rank", "pid": 100, "time_start": start, "time_end": end, "trace_flavor": "harmony_hitrace"})
				result, err := (&tool.TraceQuery{}).Execute(&types.BusContext{RepoRoot: dir, WorkDir: dir}, params)
				if err != nil || !result.Success {
					t.Fatalf("query: %v %s", err, result.Summary)
				}
				ctx := tracePrincipalValueAuthorityTestContext(names[3]+"-100", 100, result.Observations)
				ctx.Language, ctx.AgentName, ctx.Stage = "zh", types.AgentFinalizer, types.StageFinalize
				ctx.Mutable.SetTurnAArtifacts(types.TurnAArtifacts{ToolResults: []types.ToolResult{result}})
				ctx.AnalysisIR.RequestModel.RuntimeQuestionProfile = &types.RuntimeQuestionProfile{Scope: types.RuntimeQuestionScopeCausalDiagnosis}
				ctx.AnalysisIR.RequestModel.RuntimeArtifactScopeProfile = &types.RuntimeArtifactScopeProfile{RequestedScope: types.RuntimeArtifactScopeExplicitWindow, TimeStart: &start, TimeEnd: &end}
				ctx.Mutable.SetRequestModel(ctx.AnalysisIR.RequestModel)
				ctx = ctxbuilder.BuildAgentContext(&types.BusContext{RepoRoot: dir, WorkDir: dir, Language: "zh", Mutable: ctx.Mutable, AnalysisIR: ctx.AnalysisIR}, types.AgentFinalizer, types.StageFinalize)
				before, _ := json.Marshal(result.Observations)
				instruction := (&answerDocumentEvaluator{}).BuildInitialInstruction(ctx, nil)
				prompt := ctxbuilder.BuildPromptContext(ctx, &skill.Config{Name: "trace-wait-test"})
				waitHandoff := ""
				for _, section := range prompt.UserSections {
					if section.Title == ctxbuilder.SectionTraceWaitEvidence {
						waitHandoff = section.Content
					}
				}
				if waitHandoff == "" {
					t.Fatal("public context builder lost the wakeup evidence section")
				}
				for _, want := range []struct {
					from, to, value string
					ts              float64
				}{{names[0] + "-400", names[1] + "-300", "14.000", 2.016}, {names[1] + "-300", names[2] + "-200", "17.000", 2.018}, {names[2] + "-200", names[3] + "-100", "20.000", 2.020}} {
					ownedValue := "wakee=" + want.to + " pre_wakeup_wait=" + want.value + "ms"
					found := false
					for _, r := range result.Observations {
						if r.Predicate != "wakeup_chain_edge" || r.Subject != want.from || r.Object != want.to {
							continue
						}
						found = true
						if r.Value != want.value || r.Unit != "ms" || r.Span.StartTs != want.ts || r.Span.EndTs != want.ts {
							t.Fatalf("edge measurement changed: %+v", r)
						}
						if !strings.Contains(r.Summary, ownedValue) {
							t.Errorf("typed observation lacks wakee ownership: %s", r.Summary)
						}
					}
					if !found {
						t.Fatalf("public chain lost %s -> %s", want.from, want.to)
					}
					for face, text := range map[string]string{"tool": result.Summary, "initial_instruction": instruction, "model_context": waitHandoff} {
						if !strings.Contains(text, ownedValue) {
							t.Errorf("%s lacks exact owned value %q", face, ownedValue)
						}
					}
				}
				for _, want := range []string{"belongs to the wakee", "not the waker", "not post-wakeup scheduling delay"} {
					for face, text := range map[string]string{"tool": result.Summary, "model_context": waitHandoff} {
						if !strings.Contains(text, want) {
							t.Errorf("%s lacks ownership teaching %q", face, want)
						}
					}
				}
				projection := types.TraceCausalProjectionFromObservationRecords(result.Observations)
				if projection.WindowStartTs != start || projection.WindowEndTs != end {
					t.Fatalf("query window changed: %v..%v", projection.WindowStartTs, projection.WindowEndTs)
				}
				ownFound, pathFound := false, false
				pathSuffix := names[0] + "-400 -> " + names[1] + "-300 -> " + names[2] + "-200 -> " + names[3] + "-100"
				for _, r := range result.Observations {
					if r.Predicate == "wakeup_chain" && strings.Contains(r.Object, pathSuffix) {
						pathFound = true
					}
					if r.Predicate == "wakeup_causal_impact" && r.Subject == names[0]+"-400" && r.Object == state && r.Value == "11.000" {
						ownFound = true
					}
				}
				if !ownFound || !pathFound {
					t.Fatalf("own %s 11ms account or full chain lost: own=%t path=%t", state, ownFound, pathFound)
				}
				after, _ := json.Marshal(result.Observations)
				if string(before) != string(after) {
					t.Fatal("instruction rendering mutated the observations")
				}
			})
		}
	}
}
