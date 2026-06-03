package repl

import (
	"strings"
	"testing"
)

func TestParseLocalOperationSkillResultWorkflowAliases(t *testing.T) {
	payload := `{
	  "success": true,
	  "summary": "manual extracted",
	  "next_action": {
	    "provider_name": "skill:ppt_builder",
	    "operation": "presentation_generation",
	    "kind": "presentation_generation",
	    "surface": "slides",
	    "risk": "medium",
	    "effects": "local_file_write",
	    "requires_approval": true,
	    "request": "build slides",
	    "arguments": {"source_payload_ref":"notes.md"}
	  },
	  "workflow_state": {
	    "id": "wf-1",
	    "phase": "manual_extracted",
	    "return_provider": "skill:manual_reader",
	    "state": {"source_payload_ref":"notes.md"}
	  },
	  "return_to_action": {
	    "provider": "skill:manual_reader",
	    "kind": "artifact_generation",
	    "surface": "local_file",
	    "prompt": "compose final report",
	    "args": {"deck_path":"out/deck.pptx"}
	  }
	}`
	parsed, ok := parseLocalOperationSkillJSONResult(payload)
	if !ok {
		t.Fatal("result was not parsed as JSON")
	}
	if len(parsed.NextActions) != 1 {
		t.Fatalf("NextActions len=%d want 1: %+v", len(parsed.NextActions), parsed.NextActions)
	}
	action := parsed.NextActions[0]
	if action.Provider != "skill:ppt_builder" || action.OperationKind != "presentation_generation" || action.TargetSurface != "slides" {
		t.Fatalf("action aliases not preserved: %+v", action)
	}
	if !action.RequiresConfirmation || len(action.SideEffects) != 1 || action.SideEffects[0] != "local_file_write" {
		t.Fatalf("action policy aliases not preserved: %+v", action)
	}
	if action.Input["source_payload_ref"] != "notes.md" {
		t.Fatalf("action input not preserved: %+v", action.Input)
	}
	if parsed.WorkflowState.WorkflowID != "wf-1" || parsed.WorkflowState.Step != "manual_extracted" || parsed.WorkflowState.ReturnTo != "skill:manual_reader" {
		t.Fatalf("workflow state aliases not preserved: %+v", parsed.WorkflowState)
	}
	if !strings.Contains(parsed.WorkflowState.Compact(), "data_keys=source_payload_ref") {
		t.Fatalf("workflow state compact missing data keys: %s", parsed.WorkflowState.Compact())
	}
	if parsed.ReturnAction.Provider != "skill:manual_reader" || parsed.ReturnAction.OperationKind != "artifact_generation" || parsed.ReturnAction.TargetSurface != "local_file" {
		t.Fatalf("return action aliases not preserved: %+v", parsed.ReturnAction)
	}
	if parsed.ReturnAction.Input["deck_path"] != "out/deck.pptx" {
		t.Fatalf("return action input not preserved: %+v", parsed.ReturnAction.Input)
	}
}
