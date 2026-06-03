package operation

import (
	"strings"
	"testing"
)

func TestMaterialsFromCommandStepPayload(t *testing.T) {
	materials := MaterialsFromCommandStep(CommandStepResult{
		Status:        StatusExecuted,
		OutputPreview: strings.Repeat("line\n", 50),
		PayloadRef:    "/tmp/codrax-operation/out.txt",
	})
	if len(materials) != 1 {
		t.Fatalf("materials=%+v", materials)
	}
	got := materials[0]
	if got.Source != MaterialSourceCommand || got.Kind != MaterialKindPayloadRef || got.Role != MaterialRoleSavedPayload {
		t.Fatalf("unexpected material identity: %+v", got)
	}
	if got.Ref != "/tmp/codrax-operation/out.txt" || got.CompletePreview {
		t.Fatalf("unexpected material ref/preview: %+v", got)
	}
	rendered := RenderMaterialsForPrompt(materials, 4)
	for _, want := range []string{
		"operation_materials",
		"source=command",
		"kind=payload_ref",
		"role=saved_payload",
		"ref=\"/tmp/codrax-operation/out.txt\"",
		"complete_preview=false",
		"not current-source evidence",
	} {
		if !strings.Contains(rendered, want) {
			t.Fatalf("rendered materials missing %q:\n%s", want, rendered)
		}
	}
}

func TestMaterialsFromProviderResult(t *testing.T) {
	materials := MaterialsFromProviderResult(
		"skill:manual_reader",
		"run",
		"manual extracted",
		"/tmp/manual.txt",
		[]string{"out/deck.pptx"},
		0,
		[]WorkflowNextAction{{
			Provider:      "skill:ppt_builder",
			OperationKind: "presentation_generation",
			TargetSurface: "slides",
			Request:       "build deck",
		}},
		WorkflowNextAction{
			Provider:      "skill:manual_reader",
			OperationKind: "artifact_generation",
			TargetSurface: "local_file",
			Request:       "compose report",
		},
		WorkflowState{WorkflowID: "wf-1", Step: "manual_extracted"},
	)
	if len(materials) != 5 {
		t.Fatalf("materials len=%d: %+v", len(materials), materials)
	}
	inline := RenderMaterialsInline(materials, 8)
	for _, want := range []string{
		"kind=payload_ref",
		"ref=/tmp/manual.txt",
		"kind=artifact_ref",
		"ref=out/deck.pptx",
		"kind=next_action",
		"skill:ppt_builder",
		"kind=return_action",
		"kind=workflow_state",
	} {
		if !strings.Contains(inline, want) {
			t.Fatalf("inline materials missing %q:\n%s", want, inline)
		}
	}
}
