package agent

import (
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/types"
)

func TestRenderTypedToolHandoffCarriersRendersTypedFieldsOnly(t *testing.T) {
	out := renderTypedToolHandoffCarriers("### Typed handoff", []types.ToolHandoffCarrier{{
		Version:    types.ToolHandoffCarrierVersion,
		ToolName:   "emit_change_plan",
		ReasonCode: "invalid_enum",
		Repair: &types.ToolRepair{
			Code: "write_plan_repair_pack",
			Hint: "run repo_map then rewrite the whole answer",
		},
		SupportedJSON: &types.ToolJSONSurfaceDescriptor{
			ToolName:          "emit_change_plan",
			ReasonCode:        "invalid_enum",
			FailingFieldPaths: []string{"$.changes[0].edits[0].kind"},
			AcceptedEnums: map[string][]string{
				"$.changes[].edits[].kind": {"replace"},
			},
		},
		AcceptedEvidence: []types.AcceptedEvidenceRef{{
			ID:             "ev-1",
			Source:         "internal/app/main.py",
			LineStart:      12,
			OwnerSymbol:    "main",
			AnchorSymbol:   "run",
			SourcePathRole: types.SourcePathRoleProduction,
		}},
	}})
	for _, want := range []string{
		"tool=`emit_change_plan`",
		"reason=`invalid_enum`",
		"json_fields=`$.changes[0].edits[0].kind`",
		"enum_fields=`$.changes[].edits[].kind`",
		"evidence=`ev-1` @ `internal/app/main.py:12`",
		"owner=`main`",
	} {
		if !strings.Contains(out, want) {
			t.Fatalf("rendered handoff missing %q:\n%s", want, out)
		}
	}
	if strings.Contains(out, "rewrite the whole answer") || strings.Contains(out, "run repo_map") {
		t.Fatalf("renderer leaked repair hint prose:\n%s", out)
	}
}

func TestRenderAnswerDocToolHandoffCarriersConsumesTurnA(t *testing.T) {
	mut := types.NewMutableState("q")
	mut.SetTurnAArtifacts(types.TurnAArtifacts{
		HandoffCarriers: []types.ToolHandoffCarrier{{
			Version:    types.ToolHandoffCarrierVersion,
			ToolName:   "emit_evidence",
			ReasonCode: "accepted_evidence_handoff",
			AcceptedEvidence: []types.AcceptedEvidenceRef{{
				ID:          "ev-accepted",
				Source:      "src/owner.ts",
				LineStart:   5,
				OwnerSymbol: "Owner",
			}},
		}},
	})
	ctx := &types.AgentContext{Mutable: mut}
	out := renderAnswerDocToolHandoffCarriers(ctx)
	if !strings.Contains(out, "## Typed Repair And Evidence Handoff") ||
		!strings.Contains(out, "evidence=`ev-accepted` @ `src/owner.ts:5`") {
		t.Fatalf("finalizer handoff not rendered:\n%s", out)
	}
}
