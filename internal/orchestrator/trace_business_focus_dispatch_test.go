package orchestrator

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/hanchaoqun/codrax/internal/agent"
	"github.com/hanchaoqun/codrax/internal/skill"
	"github.com/hanchaoqun/codrax/internal/types"
)

func publishDispatchBusinessFocus(t *testing.T, m *types.MutableState, path string, alternate bool) types.TraceBusinessSpanRef {
	t.Helper()
	candidate := types.TraceBusinessSpanCandidate{Path: path, TID: 241, Thread: "document-main", Name: "OpenDocument", Kind: "sync", StartLine: 2, EndLine: 40, StartTs: 1.001, EndTs: 1.051}
	if alternate {
		candidate.EndTs += .001
		candidate.EndLine++
	}
	result := types.ToolResult{ToolName: "trace_query", Success: true, TraceQuerySourceRead: types.TraceQueryPhysicalSourceReadCandidate(path), TraceBusinessSpanCandidates: []types.TraceBusinessSpanCandidate{candidate}, Observations: []types.ObservationRecord{{
		ID: "window", Producer: "trace_query", Origin: types.AnswerEvidenceOriginRuntimeArtifact, GroundingPolicy: types.ClaimGroundingHard,
		SourceRef: types.ObservationSourceRef{Kind: types.ObservationSourceRuntimeArtifact, ArtifactKind: "trace", Path: path},
	}}}
	m.StampTraceQuerySourceRead(m.PrepareTraceQuerySourceRead(path), &result)
	m.StampTraceBusinessSpanRefs(&result)
	m.AppendDispatchToolResult(result)
	// These fake workers bypass explorer.ParseOutput, so mirror the native
	// result into the TurnA handoff that a completed real worker publishes.
	m.SetTurnAArtifacts(types.TurnAArtifacts{ToolResults: []types.ToolResult{result}})
	if len(result.TraceBusinessSpanRefs) != 1 {
		t.Fatalf("missing native receipt: %+v", result)
	}
	return result.TraceBusinessSpanRefs[0]
}

func businessFocusDispatchPath(t *testing.T) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "capture.trace")
	if err := os.WriteFile(path, []byte("trace bytes\n"), 0600); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestBusinessFocusSerialDispatchSuccessBoundary(t *testing.T) {
	for _, scenario := range []string{"success", "runtime_error", "output_error", "nil_output", "parent_canceled"} {
		t.Run(scenario, func(t *testing.T) {
			path := businessFocusDispatchPath(t)
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			var selected string
			ar, sr, sar := buildRegistries(map[types.AgentName]func(*types.AgentContext, *skill.Config) (*agent.StageOutput, error){
				types.AgentExplorer: func(ctx *types.AgentContext, _ *skill.Config) (*agent.StageOutput, error) {
					ref := publishDispatchBusinessFocus(t, ctx.Mutable, path, false)
					selected = ref.Token()
					if !ctx.Mutable.AcceptInvestigationCompleteWithBusinessSpanRef("accepted before worker end", &ref) {
						t.Fatal("accept failed")
					}
					if status, _ := ctx.Mutable.AcceptedTraceBusinessFocus(); status == types.TraceBusinessFocusSelected {
						t.Fatal("pending authorized inside worker")
					}
					out := &agent.StageOutput{MissingPiece: types.MissingNone}
					switch scenario {
					case "nil_output":
						return nil, nil
					case "runtime_error":
						return out, errors.New("post-completion stream failed")
					case "output_error":
						out.Error = "failed output"
					case "parent_canceled":
						cancel()
					}
					return out, nil
				},
			})
			o := New(types.PipelineSettings{}, ar, sr, sar)
			o.busCtx = &types.BusContext{Mutable: types.NewMutableState("serial focus"), Ctx: ctx}
			_, _ = o.dispatchStage(types.StageExplore)
			status, ref := o.busCtx.Mutable.AcceptedTraceBusinessFocus()
			if scenario == "success" {
				if status != types.TraceBusinessFocusSelected || ref.Token() != selected {
					t.Fatalf("success focus = %s %q", status, ref.Token())
				}
			} else if status != types.TraceBusinessFocusInvalid || ref.Token() != "" {
				t.Fatalf("failed worker focus = %s %q", status, ref.Token())
			}
			if !o.busCtx.Mutable.IsInvestigationComplete() {
				t.Fatal("new focus gate changed legacy closure retention")
			}
		})
	}
}

func TestBusinessFocusParallelDispatchSuccessBoundary(t *testing.T) {
	for _, scenario := range []string{"winner", "failed_after_accept", "output_error", "nil_output", "parent_canceled", "successful_conflict", "successful_same_instance", "successful_clear"} {
		t.Run(scenario, func(t *testing.T) {
			path := businessFocusDispatchPath(t)
			parentCtx, cancel := context.WithCancel(context.Background())
			defer cancel()
			ready := make(chan struct{})
			var winningToken string
			ar, sr, sar := buildRegistries(map[types.AgentName]func(*types.AgentContext, *skill.Config) (*agent.StageOutput, error){
				types.AgentExplorer: func(ctx *types.AgentContext, _ *skill.Config) (*agent.StageOutput, error) {
					if ctx.ExploreDispatchKey == "sibling" {
						ref := publishDispatchBusinessFocus(t, ctx.Mutable, path, scenario != "successful_same_instance")
						if scenario == "successful_conflict" || scenario == "successful_same_instance" || scenario == "successful_clear" {
							if scenario == "successful_clear" {
								ctx.Mutable.AcceptInvestigationCompleteWithBusinessSpanRef("sibling accepted without selection", nil)
							} else {
								ctx.Mutable.AcceptInvestigationCompleteWithBusinessSpanRef("sibling accepted", &ref)
							}
							close(ready)
							return &agent.StageOutput{MissingPiece: types.MissingNone}, nil
						}
						close(ready)
						<-ctx.Context().Done()
						return nil, ctx.Context().Err()
					}
					<-ready
					ref := publishDispatchBusinessFocus(t, ctx.Mutable, path, false)
					winningToken = ref.Token()
					ctx.Mutable.AcceptInvestigationCompleteWithBusinessSpanRef("winner accepted", &ref)
					out := &agent.StageOutput{MissingPiece: types.MissingNone}
					switch scenario {
					case "nil_output":
						return nil, nil
					case "failed_after_accept":
						return out, errors.New("post-completion failure")
					case "output_error":
						out.Error = "output failure"
					case "parent_canceled":
						cancel()
					}
					return out, nil
				},
			})
			o := New(types.PipelineSettings{MaxParallelism: 2}, ar, sr, sar)
			o.busCtx = &types.BusContext{Mutable: types.NewMutableState("parallel focus"), Ctx: parentCtx}
			if scenario == "successful_conflict" || scenario == "successful_same_instance" || scenario == "successful_clear" {
				// The existing mixed-origin mechanism lane waits for both workers,
				// making the successful-selection comparison deterministic.
				o.busCtx.AnalysisIR = &types.AnalysisIR{RequestModel: types.RequestModel{
					Intent: types.IntentExplain, Scenario: types.ScenarioArchitectureExplain,
					Predicates:    types.SemanticPredicates{IsHistoryLookup: true},
					AnalyzerHints: types.AnalyzerHints{Kind: string(types.ReqMechanism)},
				}}
				if o.parallelExploreAllowsEarlyConvergence() {
					t.Fatal("fixture must await both successful workers")
				}
			}
			_, _ = o.dispatchExploreWindowsParallel([][]*types.TaskNode{{{ID: "winner"}}, {{ID: "sibling"}}}, nil, 2)
			status, ref := o.busCtx.Mutable.AcceptedTraceBusinessFocus()
			switch scenario {
			case "winner":
				if status != types.TraceBusinessFocusSelected || ref.Token() != winningToken {
					t.Fatalf("winner focus = %s %q", status, ref.Token())
				}
			case "successful_same_instance":
				if status != types.TraceBusinessFocusSelected || ref.Token() != winningToken {
					t.Fatalf("equivalent instance focus = %s %q", status, ref.Token())
				}
			case "successful_conflict", "successful_clear":
				if status != types.TraceBusinessFocusConflict || ref.Token() != "" {
					t.Fatalf("different successful choices = %s %q", status, ref.Token())
				}
			default:
				if status != types.TraceBusinessFocusInvalid || ref.Token() != "" {
					t.Fatalf("failed group focus = %s %q", status, ref.Token())
				}
			}
			if handoff := o.busCtx.Mutable.TurnAArtifacts(); handoff == nil || len(handoff.ToolResults) == 0 {
				t.Fatal("focus isolation lost published producer tools")
			}
		})
	}
}
