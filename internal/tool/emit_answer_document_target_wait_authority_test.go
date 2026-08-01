package tool

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/types"
)

func targetWaitAuthorityIntegrationBus() *types.BusContext {
	count := 3
	observation := types.ObservationRecord{
		ID:              "trace_query:window#target_window_wait_occurrences",
		Origin:          types.AnswerEvidenceOriginRuntimeArtifact,
		Producer:        "trace_query",
		Role:            types.AnswerAggregateRoleSupportingCoverage,
		GroundingPolicy: types.ClaimGroundingHard,
		Subject:         "main-59566",
		Predicate:       "target_window_wait_occurrences",
		Object:          "complete",
		Value:           "3",
		ResultCount:     &count,
		RichNotes: []string{
			types.TraceNoteKeyTargetWaitOccurrencePrompt + "=status=complete,emitted=3,total=3",
			types.TraceNoteKeyTargetWaitOccurrencePromptSum + "=0.635",
			types.TraceNoteKeyTargetWaitOccurrence + "=#1 state=io_wait 34579.451701..34579.451839 duration=0.138ms iowait=1 caller=sync_buffer_read_wi lines=1-2 reason_line=3",
			types.TraceNoteKeyTargetWaitOccurrence + "=#2 state=io_wait 34579.452934..34579.453081 duration=0.147ms iowait=1 caller=sync_buffer_read_wi lines=4-5 reason_line=6",
			types.TraceNoteKeyTargetWaitOccurrence + "=#3 state=io_wait 34579.471372..34579.471722 duration=0.350ms iowait=1 caller=sync_buffer_read_wi lines=7-8 reason_line=9",
		},
	}
	mu := types.NewMutableState("q")
	mu.SetTurnAArtifacts(types.TurnAArtifacts{ToolResults: []types.ToolResult{{
		ToolName:     "trace_query",
		Success:      true,
		Observations: []types.ObservationRecord{observation},
	}}})
	return &types.BusContext{
		Mutable: mu,
		AnalysisIR: &types.AnalysisIR{RequestModel: types.RequestModel{
			Scenario: types.ScenarioRootCause,
			RuntimeTargets: []types.RuntimeTarget{{
				Kind:   types.RuntimeTargetKindThread,
				PID:    59566,
				Thread: "main-59566",
				Source: "user_explicit",
			}},
		}},
	}
}

func targetWaitAuthorityEmit(third string) json.RawMessage {
	return json.RawMessage(`{
		"blocks": [
			{
				"id": "summary",
				"kind": "summary",
				"text": "3 段 io_wait，总量 0.635ms"
			},
			{
				"id": "rows",
				"kind": "ordered_list",
				"surface_role": "principal",
				"items": [
					{"id": "one", "text": "34579.451701..34579.451839，0.138ms"},
					{"id": "two", "text": "34579.452934..34579.453081，0.147ms"},
					{"id": "three", "text": "` + third + `，0.350ms"}
				]
			}
		]
	}`)
}

func TestEmitAnswerDocumentTreatsModelTargetWaitRosterDriftAsAdvisory(t *testing.T) {
	bus := targetWaitAuthorityIntegrationBus()
	emit := &EmitAnswerDocument{}
	res, err := emit.Execute(bus, targetWaitAuthorityEmit("34579.471723..34579.471876"))
	if err != nil {
		t.Fatalf("tool errors are carried in ToolResult, got Go error: %v", err)
	}
	if !res.Success || res.Repair != nil {
		t.Fatalf("free-prose target wait drift must not trigger a hard repair: %+v", res)
	}
	if got := bus.Mutable.AnswerDocumentV2(); got == nil {
		t.Fatal("advisory-only prose did not persist")
	} else {
		var surfaces []string
		for _, block := range got.Blocks {
			surfaces = append(surfaces, types.AnswerBlockVisibleSurface(block))
		}
		if !strings.Contains(strings.Join(surfaces, "\n"), "34579.471723..34579.471876") {
			t.Fatalf("advisory-only prose must persist unchanged: %+v", got)
		}
	}

	bus = targetWaitAuthorityIntegrationBus()
	res, err = emit.Execute(bus, targetWaitAuthorityEmit("34579.471372..34579.471722"))
	if err != nil || !res.Success {
		t.Fatalf("exact target wait roster should persist: result=%+v err=%v", res, err)
	}
}
