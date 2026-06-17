package types

import "testing"

func TestPlanRepairPackFromToolResult(t *testing.T) {
	pack := PlanRepairPack{
		ToolName:          "emit_change_plan",
		ReasonCode:        "old_text_mismatch",
		FailingFieldPaths: []string{"$.changes[0].edits[0].old_text"},
		FailingPaths:      []string{"widget.py"},
	}
	result := ToolResult{
		ToolName: "emit_change_plan",
		Success:  false,
		Repair: &ToolRepair{
			Code: PlanRepairToolCode,
			Metadata: map[string]string{
				PlanRepairMetadataKey: PlanRepairPackJSON(pack),
			},
		},
	}

	got, ok := PlanRepairPackFromToolResult(result)
	if !ok {
		t.Fatal("expected plan repair pack metadata to parse")
	}
	if got.ReasonCode != "old_text_mismatch" || got.FailingPaths[0] != "widget.py" {
		t.Fatalf("unexpected repair pack: %+v", got)
	}
}

func TestPlanRepairPackFromToolResultRejectsWrongCodeOrInvalidPayload(t *testing.T) {
	valid := PlanRepairPackJSON(PlanRepairPack{ReasonCode: "old_text_mismatch"})
	cases := []ToolResult{
		{Repair: &ToolRepair{Code: "other_repair", Metadata: map[string]string{PlanRepairMetadataKey: valid}}},
		{Repair: &ToolRepair{Code: PlanRepairToolCode, Metadata: map[string]string{PlanRepairMetadataKey: `{"reason_code":`}}},
		{Repair: &ToolRepair{Code: PlanRepairToolCode, Metadata: map[string]string{PlanRepairMetadataKey: `{"version":1}`}}},
		{Repair: nil},
	}

	for _, tc := range cases {
		if got, ok := PlanRepairPackFromToolResult(tc); ok {
			t.Fatalf("expected rejection, got %+v", got)
		}
	}
}
