package tool

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/types"
)

// Existing endpoint/lease tests intentionally append their new messages. Their
// callers must now declare that choice, rather than relying on production's
// former implicit EOF insertion. The public placement suite chooses interior
// positions through ParametersFor and never uses these migration helpers.
func sequenceEndPlacementForTest(prev *types.AnswerDocumentV2, blockID string) string {
	for _, block := range prev.Blocks {
		if block.ID == blockID {
			for _, position := range sequenceInsertionPositions(block) {
				if position.Before == "" {
					return position.Ref
				}
			}
		}
	}
	return ""
}

func sequenceEndPlacementJSONForTest(t *testing.T, bus *types.BusContext, raw json.RawMessage) json.RawMessage {
	t.Helper()
	var payload map[string]json.RawMessage
	if err := json.Unmarshal(raw, &payload); err != nil {
		t.Fatal(err)
	}
	var edits []emitAnswerDiagramEdgeEdit
	if err := json.Unmarshal(payload["diagram_edge_edits"], &edits); err != nil {
		t.Fatal(err)
	}
	var wireEdits []map[string]json.RawMessage
	if err := json.Unmarshal(payload["diagram_edge_edits"], &wireEdits); err != nil {
		t.Fatal(err)
	}
	prev := bus.Mutable.PendingAnswerDocumentPatchBase()
	if prev == nil {
		prev = bus.Mutable.AnswerDocumentV2()
	}
	if prev == nil {
		prev = bus.Mutable.LastRejectedAnswerDocumentV2()
	}
	if prev == nil {
		t.Fatal("fixture has no patch base")
	}
	for i := range edits {
		resolved, err := resolveAtomicDiagramFailureRef(edits[i], bus.Mutable.AnswerDiagramRelationRepairLease())
		if err != nil {
			continue
		} // Keep the original invalid-ref negative test.
		for _, block := range prev.Blocks {
			if block.ID == resolved.BlockID && atomicSequenceEditCreatesStatement(block, resolved, bus.Mutable.AnswerDiagramRelationRepairLease()) && strings.TrimSpace(edits[i].PlacementRef) == "" {
				wireEdits[i]["placement_ref"], _ = json.Marshal(sequenceEndPlacementForTest(prev, block.ID))
			}
		}
	}
	encoded, err := json.Marshal(wireEdits)
	if err != nil {
		t.Fatal(err)
	}
	payload["diagram_edge_edits"] = encoded
	encoded, err = json.Marshal(payload)
	if err != nil {
		t.Fatal(err)
	}
	return encoded
}
