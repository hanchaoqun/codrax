package orchestrator

import (
	"reflect"
	"testing"

	"github.com/hanchaoqun/codrax/internal/agent"
	"github.com/hanchaoqun/codrax/internal/skill"
	"github.com/hanchaoqun/codrax/internal/types"
)

// A sibling's successful tools are evidence even when that sibling has not
// reached ParseOutput before another branch closes the investigation.
func TestParallelExplorePreservesCompletedSiblingTools(t *testing.T) {
	for _, canceled := range []bool{false, true} {
		for _, winnerFirst := range []bool{false, true} {
			name := "completed"
			if canceled {
				name = "canceled"
			}
			if winnerFirst {
				name += "_winner_first"
			}
			t.Run(name, func(t *testing.T) {
				ready := make(chan struct{})
				trace := types.ToolResult{ToolName: "trace_query", Success: true, RawRef: "/repo/.codrax/blob/run/window.json", Observations: []types.ObservationRecord{{
					ID: "window-distribution", Producer: "trace_query", Origin: types.AnswerEvidenceOriginRuntimeArtifact,
					GroundingPolicy: types.ClaimGroundingHard, ClaimKey: "io_latency_distribution", Value: "10.9",
					SourceRef: types.ObservationSourceRef{Kind: types.ObservationSourceRuntimeArtifact, Path: "/capture.systrace", PayloadRef: "/repo/.codrax/blob/run/window.json"},
				}}}
				trace = types.AttachToolHandoffCarrier(trace)
				count := types.ToolResult{ToolName: "exec_command", Success: true, CommandMeasurement: &types.ToolCommandMeasurement{
					Kind: types.ToolCommandMeasurementKindCount, Value: 7, Origin: types.AnswerEvidenceOriginCommandMeasurement, ProofSource: "exec_command", Command: "count",
				}}
				failed := trace
				failed.Success = false
				failed.RawRef = "failed.json"
				ar, sr, sar := buildRegistries(map[types.AgentName]func(*types.AgentContext, *skill.Config) (*agent.StageOutput, error){
					types.AgentExplorer: func(ctx *types.AgentContext, _ *skill.Config) (*agent.StageOutput, error) {
						if ctx.ExploreDispatchKey == "winner" {
							<-ready
							ctx.Mutable.SetInvestigationComplete("winner closure")
							ctx.Mutable.SetTurnAArtifacts(types.TurnAArtifacts{
								AcceptedClosureReason: "winner closure",
								HandoffCarriers:       []types.ToolHandoffCarrier{{Version: types.ToolHandoffCarrierVersion, ToolName: "parent", ObservationRefs: []types.ToolObservationRef{{ID: "parent-observation"}}}},
							})
							return &agent.StageOutput{SignalUpdates: &types.ExecutionSignals{HasEnoughFacts: true}}, nil
						}
						tools := []types.ToolResult{trace, trace, count, failed, {ToolName: "emit_investigation_complete", Success: true, Summary: "unverified model conclusion"}, {ToolName: "exec_command", Success: true, Summary: "untyped stdout"}}
						if canceled {
							// ParseOutput has not written a handoff snapshot yet.
							for _, result := range tools {
								ctx.Mutable.AppendDispatchToolResult(result)
							}
						} else {
							ctx.Mutable.SetTurnAArtifacts(types.TurnAArtifacts{
								ToolResults: tools, InvestigationNotes: []string{"unfinished model note"},
								HandoffCarriers:       []types.ToolHandoffCarrier{{Version: types.ToolHandoffCarrierVersion, ToolName: "sibling", ObservationRefs: []types.ToolObservationRef{{ID: "unaccepted-sibling-carrier"}}}},
								AcceptedClosureReason: "not accepted", AcceptedAggregateFacts: []types.AnswerAggregateFact{{Label: "unverified", Value: "999"}},
							})
						}
						close(ready)
						if canceled {
							<-ctx.Context().Done()
							return nil, ctx.Context().Err()
						}
						return &agent.StageOutput{StageReport: "losing model report", SignalUpdates: &types.ExecutionSignals{HasEnoughFacts: false}}, nil
					},
				})
				o := New(types.PipelineSettings{MaxParallelism: 2}, ar, sr, sar)
				o.busCtx = &types.BusContext{Mutable: types.NewMutableState("parallel tool handoff")}
				windows := [][]*types.TaskNode{{{ID: "sibling"}}, {{ID: "winner"}}}
				if winnerFirst {
					windows[0], windows[1] = windows[1], windows[0]
				}
				out, err := o.dispatchExploreWindowsParallel(windows, nil, 2)
				if err != nil {
					t.Fatal(err)
				}
				if out == nil || !out.SignalUpdates.HasEnoughFacts || !o.busCtx.Signals.HasEnoughFacts {
					t.Fatalf("sibling changed winning signals: out=%+v bus=%+v", out, o.busCtx.Signals)
				}
				ta := o.busCtx.Mutable.TurnAArtifacts()
				if ta == nil || len(ta.ToolResults) != 2 {
					t.Fatalf("successful typed tools lost or duplicated: %+v", ta)
				}
				if !reflect.DeepEqual(ta.ToolResults, []types.ToolResult{trace, count}) {
					t.Fatalf("producer payload changed: %+v", ta.ToolResults)
				}
				refs := map[string]int{}
				for _, carrier := range ta.HandoffCarriers {
					for _, ref := range carrier.ObservationRefs {
						refs[ref.ID]++
					}
				}
				if !reflect.DeepEqual(refs, map[string]int{"parent-observation": 1, "window-distribution": 1}) {
					t.Fatalf("handoff observation mirror lost producer refs or accepted sibling explicit state: %+v", refs)
				}
				if ta.AcceptedClosureReason != "winner closure" || len(ta.AcceptedAggregateFacts) != 0 || len(ta.InvestigationNotes) != 0 || len(o.busCtx.StageReports) != 0 {
					t.Fatalf("unaccepted model state leaked: ta=%+v reports=%+v", ta, o.busCtx.StageReports)
				}
				ledger := types.CompileObservationLedger(types.ObservationLedgerInputFromBusContext(o.busCtx, types.ObservationExtractLedgerEvidenceLimit))
				seen := 0
				for _, row := range ledger.Records {
					if row.ID == "window-distribution" {
						seen++
						if row.Value != "10.9" {
							t.Fatalf("typed value changed: %+v", row)
						}
					}
				}
				if seen != 1 {
					t.Fatalf("finalizer ledger has %d copies of completed sibling observation", seen)
				}
			})
		}
	}
}
