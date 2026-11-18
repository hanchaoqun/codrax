package tool

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/types"
)

// The compact participant repair names only what exists: a published row, the
// addition published for its source, and its printed edge_anchor. Anything
// else falls back to the general participant guidance (empty result).
func TestDiagramParticipantRuntimeCompactRepairHMC376(t *testing.T) {
	const capture, clock, participant = "/captures/a.ftrace", "boot", "requested"
	waker := threadGroupSideHMC376{"rt_thread_w", types.RuntimeWakeupThread{Comm: "waker", PID: 12, TGID: 10}}
	wakee := threadGroupSideHMC376{"rt_thread_t", types.RuntimeWakeupThread{Comm: "wakee", PID: 42, TGID: 40}}
	rows := func(bound int) []RuntimeDiagramRelation {
		var out []RuntimeDiagramRelation
		for line := 1; line <= 10; line++ {
			row := threadGroupRowHMC376(capture, line, clock, waker, wakee)
			row.ScopeLabel = "recorded wake"
			if line == bound {
				row.requestedTo = []string{participant}
			}
			out = append(out, row)
		}
		return out
	}
	delta := func(row RuntimeDiagramRelation) string {
		raw, _ := json.Marshal(types.AnswerDiagramRelationRepairDelta{Version: types.AnswerDiagramRelationRepairDeltaVersion, PreserveUnlistedEdges: true,
			AllowedAdditions: []types.AnswerDiagramRelationRepairCandidate{{BlockID: "d", RelationKind: row.Kind, FromIdentity: "another-lookup-from", ToIdentity: "another-lookup-to", Source: row.SupportRefs[0]}}})
		return string(raw)
	}
	doc := func(anchors ...types.DiagramEdgeAnchor) *types.AnswerDocumentV2 {
		return &types.AnswerDocumentV2{DocumentModel: "v2", Blocks: []types.AnswerBlock{{ID: "d", Kind: types.BlockDiagram, EdgeAnchors: anchors,
			Diagram: &types.AnswerDiagramBlock{Kind: types.DiagramSequence, Body: "sequenceDiagram"}}}}
	}
	missing := []DiagramParticipantCoverageMismatch{{BlockID: "d", Participant: participant, Issue: DiagramParticipantCoverageIdentityMissing}}
	const patch, reemit = `in a patch use diagram_edge_edits action "add" with the addition_ref listed for this row's source`, "in a full re-emit copy this row's edge_anchor from the runtime diagram anchors"

	printed, late := rows(2), rows(10)
	for _, tc := range []struct {
		name         string
		rows         []RuntimeDiagramRelation
		delta        string
		want, forbid []string
	}{
		{"printed row with a published addition", printed, delta(printed[1]), []string{`Participant "requested" is the recorded endpoint "wakee-42"`, `Published row to draw: "waker-12" → "wakee-42"；recorded wake；发起线程号=12；被唤醒线程号=42`, "whose to_node is that endpoint's lifeline", patch + "; " + reemit}, nil},
		{"row beyond the printed rows with a published addition", late, delta(late[9]), []string{patch}, []string{reemit}},
		{"printed row without a published addition", printed, "", []string{reemit}, []string{patch}},
		{"an addition for another source is not this row's", printed, delta(printed[5]), []string{reemit}, []string{patch}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got := diagramParticipantRuntimeCompactRepair(doc(), missing, tc.rows, tc.delta)
			for _, want := range tc.want {
				if !strings.Contains(got, want) {
					t.Fatalf("repair lacks %q: %s", want, got)
				}
			}
			for _, forbid := range tc.forbid {
				if strings.Contains(got, forbid) {
					t.Fatalf("repair names an operation that does not exist (%q): %s", forbid, got)
				}
			}
		})
	}
	for name, got := range map[string]string{
		"no operation exists for the row":          diagramParticipantRuntimeCompactRepair(doc(), missing, late, ""),
		"the participant is bound on no row":       diagramParticipantRuntimeCompactRepair(doc(), missing, rows(0), ""),
		"another kind of participant mismatch":     diagramParticipantRuntimeCompactRepair(doc(), []DiagramParticipantCoverageMismatch{{BlockID: "d", Participant: participant, Issue: DiagramParticipantCoverageMissingBoundary}}, printed, ""),
		"one of two mismatches is of another kind": diagramParticipantRuntimeCompactRepair(doc(), append([]DiagramParticipantCoverageMismatch{{Participant: "other", Issue: DiagramParticipantCoverageDuplicate}}, missing...), printed, ""),
		"unreadable published additions":           diagramParticipantRuntimeCompactRepair(doc(), missing, printed, "{"),
		"no rows":                                  diagramParticipantRuntimeCompactRepair(doc(), missing, nil, ""),
	} {
		if got != "" {
			t.Fatalf("%s: the general guidance must be kept, got %s", name, got)
		}
	}
	// A row that is already drawn, by its own or another lookup's credentials
	// for the same physical row, is never offered as an addition.
	copyRow := printed[1]
	copyRow.FromIdentity, copyRow.ToIdentity, copyRow.FromNode, copyRow.ToNode, copyRow.requestedTo = "copy-from", "copy-to", "rt_thread_w2", "rt_thread_t2", nil
	withCopy := append(append([]RuntimeDiagramRelation(nil), printed...), copyRow)
	for name, anchor := range map[string]types.DiagramEdgeAnchor{
		"own credentials":              {FromNode: "A", ToNode: "B", FromIdentity: printed[1].FromIdentity, ToIdentity: printed[1].ToIdentity, RelationKind: types.DiagramRelWakeup},
		"another lookup's credentials": {FromNode: "A", ToNode: "B", FromIdentity: "copy-from", ToIdentity: "copy-to", RelationKind: types.DiagramRelWakeup},
	} {
		got := diagramParticipantRuntimeCompactRepair(doc(anchor), missing, withCopy, delta(printed[1]))
		if !strings.Contains(got, "is already drawn but is not accepted as drawn: repair that arrow instead of adding another one") || strings.Contains(got, patch) || strings.Contains(got, reemit) {
			t.Fatalf("%s: a drawn row was offered again: %s", name, got)
		}
	}
}
