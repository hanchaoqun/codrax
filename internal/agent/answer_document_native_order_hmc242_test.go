package agent

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/tool"
	"github.com/hanchaoqun/codrax/internal/types"
)

// Select both the event and its insertion position from the public live schema.
func nativeOrderLocalPatchHMC242(t *testing.T, ctx *types.AgentContext, before string) json.RawMessage {
	t.Helper()
	lease := ctx.Mutable.AnswerDiagramRelationRepairLease()
	if lease == nil {
		t.Fatal("missing current native-order lease")
	}
	var f types.AnswerDiagramRelationRepairFailure
	var c types.AnswerDiagramRelationRepairCandidate
	for _, failure := range lease.Failures {
		if !answerDocNativeOrderHasLocalCandidate(failure, lease.AllowedAdditions) {
			continue
		}
		f = failure
		for _, candidate := range lease.AllowedAdditions {
			if candidate.BlockID == f.BlockID && candidate.RelationKind == f.RelationKind && candidate.FromIdentity == f.FromIdentity && candidate.ToIdentity == f.ToIdentity {
				c = candidate
			}
		}
	}
	if c.AdditionRef == "" {
		t.Fatalf("no exact same-event local capability: %+v", lease)
	}
	var schema any
	if err := json.Unmarshal((&tool.EmitAnswerDocumentPatch{}).ParametersFor(ctx), &schema); err != nil {
		t.Fatal(err)
	}
	placement := ""
	var walk func(any)
	walk = func(v any) {
		switch v := v.(type) {
		case map[string]any:
			if v["before"] == before {
				placement, _ = v["placement_ref"].(string)
			}
			for _, child := range v {
				walk(child)
			}
		case []any:
			for _, child := range v {
				walk(child)
			}
		case string:
			if i := strings.Index(v, "sequence_placement_choices="); i >= 0 {
				var rows []any
				if json.Unmarshal([]byte(v[i+len("sequence_placement_choices="):]), &rows) == nil {
					walk(rows)
				}
			}
		}
	}
	walk(schema)
	if placement == "" {
		t.Fatalf("public schema did not publish position before %q", before)
	}
	raw, _ := json.Marshal(map[string]any{"unchanged_block_ids": []string{"summary"}, "diagram_edge_edits": []any{
		map[string]any{"action": "remove", "failure_ref": f.FailureRef},
		map[string]any{"action": "add", "addition_ref": c.AdditionRef, "placement_ref": placement,
			"edge": map[string]string{"from_node": f.FromNode, "to_node": f.ToNode, "visible_label": "observed event"}},
	}})
	return raw
}

func TestNativeOrderTeachingRequiresCurrentCapabilityHMC242(t *testing.T) {
	ctx, rows := sequenceOrderHMC230Fixture(t)
	raw, _ := json.Marshal(sequenceOrderHMC230Doc(rows, true))
	res, err := (&tool.EmitAnswerDocument{}).Execute(types.ToolBusContext(ctx, types.AgentFinalizer), raw)
	if err != nil || res.Success || res.Repair == nil {
		t.Fatalf("real order rejection missing: %v %+v", err, res)
	}
	if !installAnswerDocDiagramRelationRepairLease(ctx, ctx.Mutable, &res, false) {
		t.Fatal("real order failure did not issue current executable refs")
	}
	delta, _, ok := parseAnswerDocDiagramRelationRepairDelta(&res)
	if !ok || len(delta.Failures) == 0 {
		t.Fatal("real order delta missing")
	}
	for _, change := range []string{"valid", "no_ref", "no_carrier", "no_action", "no_occurrence", "other_event", "other_block", "no_addition_ref"} {
		t.Run(change, func(t *testing.T) {
			f := delta.Failures[0]
			cs := append([]types.AnswerDiagramRelationRepairCandidate(nil), delta.AllowedAdditions...)
			switch change {
			case "no_ref":
				f.FailureRef = ""
			case "no_carrier":
				f.TargetCarrier = types.AnswerDiagramRelationRepairCarrierUnknown
			case "no_action":
				f.AllowedActions = nil
			case "no_occurrence":
				f.BodyOccurrence = 0
			default:
				for i := range cs {
					switch change {
					case "other_event":
						cs[i].FromIdentity += "other"
					case "other_block":
						cs[i].BlockID += "other"
					case "no_addition_ref":
						cs[i].AdditionRef = ""
					}
				}
			}
			if answerDocNativeOrderHasLocalCandidate(f, cs) != (change == "valid") {
				t.Fatal("teaching diverged from the precise current capability")
			}
		})
	}
}
