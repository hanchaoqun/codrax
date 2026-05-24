package orchestrator

import (
	"context"
	"errors"
	"testing"

	"github.com/hanchaoqun/codrax/internal/agent"
	"github.com/hanchaoqun/codrax/internal/skill"
	"github.com/hanchaoqun/codrax/internal/types"
)

func TestDispatchStage_AppliesPartialOutputOnTransientError(t *testing.T) {
	partialEvidence := types.EvidenceItem{
		Kind:      types.EvidenceMechanism,
		Subject:   "buildAnalysisIR",
		Object:    "gate.RunWith",
		Source:    "internal/agent/analyzer.go",
		LineStart: 2278,
	}
	ar, sr, sar := buildRegistries(map[types.AgentName]func(*types.AgentContext, *skill.Config) (*agent.StageOutput, error){
		types.AgentExplorer: func(ctx *types.AgentContext, _ *skill.Config) (*agent.StageOutput, error) {
			ctx.Mutable.SetTurnAArtifacts(types.TurnAArtifacts{
				UserQuestion:          ctx.Objective,
				ReadFiles:             []string{"internal/agent/analyzer.go"},
				EvidenceItems:         []types.EvidenceItem{partialEvidence},
				ToolResults:           []types.ToolResult{{ToolName: "emit_evidence", Success: true, Summary: "accepted"}},
				FlowFindings:          []types.FlowFindingDigest{{ID: "flow-1", Sources: []string{"test"}}},
				TerminalEvidenceCount: 1,
			})
			return &agent.StageOutput{
				Error:         "LLM call failed: stream stalled",
				MissingPiece:  types.MissingFacts,
				StageReport:   "partial explorer report",
				EvidenceItems: []types.EvidenceItem{partialEvidence},
				ToolResults:   []types.ToolResult{{ToolName: "emit_evidence", Success: true, Summary: "accepted"}},
				FlowFindings:  []types.FlowFindingDigest{{ID: "flow-1", Sources: []string{"test"}}},
			}, errors.New("upstream LLM stream stalled")
		},
	})

	o := New(types.PipelineSettings{}, ar, sr, sar)
	o.busCtx = &types.BusContext{
		Ctx:     context.Background(),
		Mutable: types.NewMutableState("trace buildAnalysisIR to gate.Run"),
		TaskState: types.TaskState{
			Missing: types.MissingFacts,
		},
	}

	out, err := o.dispatchStage(types.StageExplore)
	if err == nil {
		t.Fatal("dispatchStage should preserve the original transient error")
	}
	if out == nil || len(out.EvidenceItems) != 1 {
		t.Fatalf("dispatchStage should return the partial StageOutput, got %+v", out)
	}
	if len(o.busCtx.EvidenceItems) != 1 || o.busCtx.EvidenceItems[0].Subject != "buildAnalysisIR" {
		t.Fatalf("partial evidence was not merged into BusContext: %+v", o.busCtx.EvidenceItems)
	}
	if len(o.busCtx.ToolResults) != 1 {
		t.Fatalf("partial tool results were not merged into BusContext: %+v", o.busCtx.ToolResults)
	}
	if len(o.busCtx.StageReports) != 1 || o.busCtx.StageReports[0].Findings != "partial explorer report" {
		t.Fatalf("partial stage report was not retained: %+v", o.busCtx.StageReports)
	}
	if ta := o.busCtx.Mutable.TurnAArtifacts(); ta == nil || len(ta.EvidenceItems) != 1 || len(ta.ReadFiles) != 1 {
		t.Fatalf("TurnAArtifacts side effects were not preserved: %+v", ta)
	}
	if o.busCtx.TaskState.LastError == "" {
		t.Fatal("partial failure should still update LastError for retry policy")
	}
}
