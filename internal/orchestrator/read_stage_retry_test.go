package orchestrator

import (
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/agent"
	"github.com/hanchaoqun/codrax/internal/types"
)

func TestBuildExploreTransientRetryCheckpointHintIncludesTypedObservationOrigins(t *testing.T) {
	mut := types.NewMutableState("count generated tool files")
	mut.AppendDispatchToolResult(types.ToolResult{
		ToolName: "exec_command",
		Success:  true,
		Summary: "[exec_command: $ find internal/tool -name '*.go' | wc -l]\n" +
			"[exec_command: evidence_origin=command_measurement measurement=count]\n" +
			"140\n",
	})
	o := &Orchestrator{busCtx: &types.BusContext{Mutable: mut}}

	got := o.buildExploreTransientRetryCheckpointHint()
	for _, want := range []string{
		"Checkpoint summary",
		"remaining-objective context only",
		"typed observation origins=command_measurement:1",
		"successful tool results=1",
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("checkpoint hint missing %q:\n%s", want, got)
		}
	}
}

func TestBuildExploreTransientRetryCheckpointHintCarriesReadProofGuidance(t *testing.T) {
	mut := types.NewMutableState("weak read proof")
	mut.SetTurnAArtifacts(types.TurnAArtifacts{
		ReadFiles:          []string{"pkg/maybe.py"},
		AcceptedResultKind: "resolved",
		EvidenceItems: []types.EvidenceItem{{
			ID:              "ev-observed",
			Source:          "pkg/maybe.py",
			LineStart:       3,
			Subject:         "Maybe",
			GroundingStatus: types.GroundingGrounded,
		}},
		SourceLocalization: &types.SourceLocalizationReview{
			Status:      types.SourceLocalizationObserved,
			SourcePaths: []string{"pkg/maybe.py"},
		},
	})
	o := &Orchestrator{busCtx: &types.BusContext{Mutable: mut}}

	got := o.buildExploreTransientRetryCheckpointHint()
	for _, want := range []string{
		"proof authority=weak",
		"reason=proof_weak",
		"action=add_proof",
		"mode=advisory",
		"loop action=add_proof source=proof_authority reason=proof_weak",
		"loop shadow recommended=add_proof imperative=add_proof match=true reason=proof_weak",
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("checkpoint hint missing proof guidance %q:\n%s", want, got)
		}
	}
	if strings.Contains(got, "mode=hard") {
		t.Fatalf("weak read proof must not be rendered as a hard block:\n%s", got)
	}
}

func TestReadLoopAddProofActionSkipsCoveredProof(t *testing.T) {
	mut := types.NewMutableState("covered runtime proof")
	mut.SetTurnAArtifacts(types.TurnAArtifacts{
		AcceptedResultKind:               "resolved",
		RuntimeObservationOnlyCompletion: true,
		AcceptedAggregateFacts: []types.AnswerAggregateFact{{
			Kind:  types.AnswerAggregateScalar,
			Label: "answer",
			Value: "covered",
		}},
	})
	if got := readLoopAddProofActionSummaryFromMutable(mut); got != "" {
		t.Fatalf("covered proof should not request add_proof action, got %q", got)
	}
}

func TestBuildExploreFactRetryContinuationHintCarriesRuntimeFrontier(t *testing.T) {
	mut := types.NewMutableState("trace root cause")
	mut.AppendDispatchToolResult(types.ToolResult{
		ToolName: "grep",
		Success:  true,
		Summary: "[grep retrieval governor]\n" +
			"line_windows=record_trace.systrace:1102600-1102640(matches=4); record_trace.systrace:1139160-1139200(matches=3)\n" +
			"next_shape=single large runtime artifact matched too broadly; narrow with one exact timestamp/literal/thread id\n",
	})
	mut.SetTurnAArtifacts(types.TurnAArtifacts{
		ReadFiles: []string{"record_trace.systrace"},
		AcceptedAggregateFacts: []types.AnswerAggregateFact{{
			Kind:  types.AnswerAggregateScalar,
			Label: "frontier",
			Value: "[GT]ColdPool#5-36624",
		}},
	})
	o := &Orchestrator{busCtx: &types.BusContext{
		Mutable: mut,
		AnalysisIR: &types.AnalysisIR{RequestModel: types.RequestModel{
			Intent:     types.IntentRootCause,
			Scenario:   types.ScenarioRootCause,
			Complexity: types.ComplexityComplex,
		}},
	}}

	got := o.buildExploreFactRetryContinuationHint(&agent.StageOutput{
		ToolResults: []types.ToolResult{{
			ToolName: "grep",
			Success:  true,
			Summary:  "line_window_hint=first returned match is record_trace.systrace:1102623; next use read_file\n",
		}},
	})
	for _, want := range []string{
		exploreFactRetryCheckpointPrefix,
		"not a fresh investigation",
		"Runtime/log/trace continuation",
		"line_windows=record_trace.systrace:1102600-1102640",
		"line_window_hint=first returned match is record_trace.systrace:1102623",
		"frontier=[GT]ColdPool#5-36624",
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("fact retry checkpoint missing %q:\n%s", want, got)
		}
	}
}

func TestSoftAgentOutputRetryMessageRuntimeContinuationLocalized(t *testing.T) {
	bus := &types.BusContext{
		AnalysisIR: &types.AnalysisIR{RequestModel: types.RequestModel{
			Intent:     types.IntentRootCause,
			Scenario:   types.ScenarioRootCause,
			Complexity: types.ComplexityComplex,
		}},
	}
	got := softAgentOutputRetryMessage(bus, "zh", types.StageExplore, types.MissingFacts)
	if !strings.Contains(got, "继续上次调查") {
		t.Fatalf("runtime retry UX should indicate continuation, got %q", got)
	}
}
