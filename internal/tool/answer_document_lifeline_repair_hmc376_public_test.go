package tool

import (
	"encoding/json"
	"fmt"
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/types"
)

// The capsule prints the request binding under the row it actually presents,
// states the lifeline rule once, and prints both thread numbers of a wake row.
func TestHMC376PublicCapsuleBindsPresentedCopy(t *testing.T) {
	ctx, rows := lifelineFixtureHMC376(t, lifelineBothRolesTraceHMC376, "wakeup_chain", "event_search")
	presented := lifelinePresentedWakesHMC376(rows)
	ledger := types.CompileObservationLedger(types.ObservationLedgerInputFromBusContext(ctx, types.ObservationExtractLedgerEvidenceLimit))
	text := RenderRuntimeDiagramRelationRecipes(ledger, &ctx.AnalysisIR.RequestModel)
	var bindings []string
	for _, line := range strings.Split(text, "\n") {
		if strings.Contains(line, "requested_participant_binding=") {
			bindings = append(bindings, line)
		}
	}
	// The requested thread is woken twice; both wakes are presented.
	if len(bindings) != 2 {
		t.Fatalf("want one binding under each presented wake of the requested thread, got %d: %s", len(bindings), text)
	}
	for _, row := range []RuntimeDiagramRelation{presented[0], presented[2]} {
		want, _ := json.Marshal(map[string]string{"participant": hmc242ParticipantQuote, "native_node": row.ToNode})
		if !strings.Contains(text, "requested_participant_binding="+string(want)) {
			t.Fatalf("binding does not name the presented node of the wake at %v: %s", row.eventOrder.timestamp, text)
		}
	}
	// The thread wakes a helper on a row that only the search recorded; no
	// lookup proved a binding for that row, so none is printed for it.
	if len(presented[1].requestedFrom)+len(presented[1].requestedTo) != 0 {
		t.Fatalf("a binding was minted for a row no lookup bound: %+v", presented[1])
	}
	if strings.Count(text, runtimeDiagramLifelineTeaching) != 1 || strings.Count(text, "One lifeline per recorded thread") != 1 {
		t.Fatalf("the lifeline rule must be stated exactly once: %s", text)
	}
	// The explanation of a binding is printed once; each later row keeps only
	// its exact binding.
	if strings.Count(text, "this exact selected native instance satisfies the requested identity") != 1 {
		t.Fatalf("the binding explanation must be stated exactly once: %s", text)
	}
	// A typed note keeps its exact owner rule; the lifeline rule must not
	// promise that any joined lifeline accepts it.
	if !strings.Contains(runtimeDiagramLifelineTeaching, "all of its arrows and free-text notes; a runtime_notes_v1 selection follows its own node_id rule") || strings.Contains(runtimeDiagramLifelineTeaching, "state notes") {
		t.Fatalf("the lifeline rule overstates typed notes: %s", runtimeDiagramLifelineTeaching)
	}
	for _, gone := range []string{"cannot merge different producer lanes", "Do not create a second generic lane", "不能混用端点"} {
		if strings.Contains(text, gone) {
			t.Fatalf("contradicting teaching %q is still printed: %s", gone, text)
		}
	}
	if !strings.Contains(text, "；发起线程号=42；被唤醒线程号=12；edge_anchor=") || !strings.Contains(text, "；发起线程号=11；被唤醒线程号=42；edge_anchor=") {
		t.Fatalf("wake rows do not state both thread numbers: %s", text)
	}
}

// Explicit selections may name lanes that different lookups published for
// one thread; the selected lifeline still hosts all arrows of that thread.
func TestHMC376PublicSelectionsAcrossCopies(t *testing.T) {
	ctx, rows := lifelineFixtureHMC376(t, lifelineBothRolesTraceHMC376, "wakeup_chain", "event_search")
	presented := lifelinePresentedWakesHMC376(rows)
	doc := lifelineDocHMC376(rows, presented, lifelinePerThreadNodeHMC376)
	// T42 selects its woken-side lane while its second arrow records the
	// waking-side lane; T12 selects its waking-side lane while its first
	// arrow records the woken-side lane.
	if presented[2].ToNode == presented[1].FromNode || presented[2].FromNode == presented[1].ToNode {
		t.Fatalf("fixture must publish both threads under two lanes: %+v", presented)
	}
	doc.Blocks[1].Diagram.RuntimeParticipantsV1 = []types.RuntimeDiagramParticipantSelection{
		{NodeID: "T11", NativeParticipant: presented[0].FromNode},
		{NodeID: "T12", NativeParticipant: presented[2].FromNode},
		{NodeID: "T42", NativeParticipant: presented[2].ToNode},
	}
	out := lifelineEmitHMC376(t, ctx, doc)
	if !out.Success {
		t.Fatalf("selections across lookup copies of one thread were rejected: %s", out.Summary)
	}
	got := ctx.Mutable.AnswerDocumentV2()
	if got == nil || len(got.Blocks[1].EdgeAnchors) != 3 || len(got.Blocks[1].Diagram.RuntimeParticipantsV1) != 3 {
		t.Fatalf("acceptance lost arrows or selections: %+v", got)
	}
	// A selection that puts two different threads on one node stays rejected.
	ctx, rows = lifelineFixtureHMC376(t, lifelineBothRolesTraceHMC376, "wakeup_chain", "event_search")
	presented = lifelinePresentedWakesHMC376(rows)
	doc = lifelineDocHMC376(rows, presented, lifelinePerThreadNodeHMC376)
	doc.Blocks[1].Diagram.RuntimeParticipantsV1 = []types.RuntimeDiagramParticipantSelection{{NodeID: "T42", NativeParticipant: presented[0].FromNode}}
	if out := lifelineEmitHMC376(t, ctx, doc); out.Success || !strings.Contains(out.Summary, "selects conflicting native threads") {
		t.Fatalf("one node selected for another thread than its arrows was accepted: %s", out.Summary)
	}
}

// A published thread id used as the lifeline id hosts the arrows that other
// lookup copies recorded for that same thread, whichever lane was chosen.
func TestHMC376PublicNativeIdLifelineHostsOtherCopy(t *testing.T) {
	for _, side := range []string{"woken lane", "waking lane"} {
		t.Run(side, func(t *testing.T) {
			ctx, rows := lifelineFixtureHMC376(t, lifelineBothRolesTraceHMC376, "wakeup_chain", "event_search")
			presented := lifelinePresentedWakesHMC376(rows)
			mainNode, mainLabel := presented[2].ToNode, presented[2].ToLabel
			if side == "waking lane" {
				mainNode, mainLabel = presented[1].FromNode, presented[1].FromLabel
			}
			node := func(row RuntimeDiagramRelation, from bool) (string, string) {
				thread := row.threadEndpoints.to
				if from {
					thread = row.threadEndpoints.from
				}
				if thread.PID == 42 {
					return mainNode, mainLabel
				}
				return lifelinePublishedNodeHMC376(row, from)
			}
			out := lifelineEmitHMC376(t, ctx, lifelineDocHMC376(rows, presented, node))
			if !out.Success {
				t.Fatalf("a published thread id as the lifeline of its own thread was rejected: %s", out.Summary)
			}
			if got := ctx.Mutable.AnswerDocumentV2(); got == nil || len(got.Blocks[1].EdgeAnchors) != 3 {
				t.Fatalf("acceptance lost recorded wake arrows: %+v", got)
			}
		})
	}
	// The published id of one thread never hosts an arrow of another thread.
	ctx, rows := lifelineFixtureHMC376(t, lifelineBothRolesTraceHMC376, "wakeup_chain", "event_search")
	presented := lifelinePresentedWakesHMC376(rows)
	wrong := func(row RuntimeDiagramRelation, from bool) (string, string) {
		if !from && row.threadEndpoints.to.PID == 12 {
			return presented[0].FromNode, presented[0].FromLabel // loader-11's published id
		}
		return lifelinePublishedNodeHMC376(row, from)
	}
	out := lifelineEmitHMC376(t, ctx, lifelineDocHMC376(rows, presented, wrong))
	if out.Success {
		t.Fatal("the published id of one thread hosted an arrow of another thread")
	}
	hint := lifelineHintHMC376(t, out.Summary, types.DiagramRelationFailureRuntimeLifelineTwoThreads)
	if !strings.Contains(hint, "The lifeline declared with the published id of thread loader-11 is that thread") || !strings.Contains(hint, `Arrow "唤醒2" (target-42 → helper-12) ends at thread helper-12`) ||
		strings.Contains(hint, `"唤醒1"`) || strings.Contains(hint, `"唤醒3"`) || lifelineLongHexHMC376.MatchString(hint) {
		t.Fatalf("rejection must name only the odd arrow, without long ids: %s", hint)
	}
}

// A search alone records the requested thread with and without its process
// number, so no lookup proves the request binding. One lifeline per thread is
// accepted, the thread stays unverified and the unproven boundary stays
// required: no binding is minted from the lifeline join.
func TestHMC376PublicSearchOnlyKeepsUnprovenBoundary(t *testing.T) {
	ctx, rows := lifelineFixtureHMC376(t, lifelineBothRolesTraceHMC376, "event_search")
	presented := lifelinePresentedWakesHMC376(rows)
	if len(presented) != 3 || len(lifelineLanesByThreadHMC376(rows)[42]) != 2 {
		t.Fatalf("fixture must publish the requested thread under two lanes: %+v", presented)
	}
	for _, row := range rows {
		if len(row.requestedFrom)+len(row.requestedTo) != 0 {
			t.Fatalf("a request binding was minted without a proving lookup: %+v", row)
		}
	}
	if unmapped := RuntimeDiagramUnmappedRequestedParticipants(rows, &ctx.AnalysisIR.RequestModel); len(unmapped) != 1 || unmapped[0] != hmc242ParticipantQuote {
		t.Fatalf("the requested thread must stay unverified: %v", unmapped)
	}
	doc := lifelineDocHMC376(rows, presented, lifelinePerThreadNodeHMC376)
	out := lifelineEmitHMC376(t, ctx, doc)
	if issues := lifelineIssuesHMC376(out.Summary); out.Success || len(issues) != 1 || issues[0] != string(DiagramParticipantCoverageMissingBoundary) {
		t.Fatalf("want only the missing unproven boundary, got success=%t %v: %s", out.Success, issues, out.Summary)
	}
	// The same lifelines with the honest boundary are accepted.
	ctx, rows = lifelineFixtureHMC376(t, lifelineBothRolesTraceHMC376, "event_search")
	doc = lifelineDocHMC376(rows, lifelinePresentedWakesHMC376(rows), lifelinePerThreadNodeHMC376)
	doc.Blocks[1].ParticipantBoundaries = []types.DiagramParticipantBoundary{{Participant: hmc242ParticipantQuote, Status: types.DiagramParticipantBoundaryUnproven}}
	doc.Blocks[1].Diagram.Body += "\n    participant requested as " + hmc242ParticipantQuote
	out = lifelineEmitHMC376(t, ctx, doc)
	if !out.Success {
		t.Fatalf("one lifeline per thread with the unproven boundary was rejected: %s", out.Summary)
	}
	got := ctx.Mutable.AnswerDocumentV2()
	if got == nil || len(got.Blocks[1].EdgeAnchors) != 3 || len(got.Blocks[1].ParticipantBoundaries) != 1 {
		t.Fatalf("acceptance lost arrows or the unproven boundary: %+v", got)
	}
}

func lifelineInstallRejectionLeaseHMC376(t *testing.T, ctx *types.BusContext, out types.ToolResult) *types.AnswerDiagramRelationRepairLease {
	t.Helper()
	base := ctx.Mutable.PendingAnswerDocumentPatchBase()
	if base == nil {
		base = ctx.Mutable.LastRejectedAnswerDocumentV2()
	}
	if base == nil || out.Success || out.Repair == nil {
		t.Fatalf("no public rejection to repair: %+v", out)
	}
	delta := lifelineRelationDeltaHMC376(t, out)
	lease := types.NewAnswerDiagramRelationRepairLease(base, delta.Failures, delta.AllowedAdditions)
	var participants struct {
		Mismatches []struct {
			BlockID     string `json:"block_id"`
			Participant string `json:"participant"`
			Issue       string `json:"issue"`
		} `json:"mismatches"`
	}
	if raw := out.Repair.Metadata[types.ToolRepairMetaDiagramParticipantRepairDeltaJSON]; raw != "" {
		if err := json.Unmarshal([]byte(raw), &participants); err != nil {
			t.Fatal(err)
		}
	}
	var visibility []types.AnswerDiagramParticipantVisibilityRepairFailure
	for _, m := range participants.Mismatches {
		visibility = append(visibility, types.AnswerDiagramParticipantVisibilityRepairFailure{BlockID: m.BlockID, Participant: m.Participant, Issue: m.Issue})
	}
	lease = types.WithAnswerDiagramParticipantVisibilityRepairFailures(base, lease, visibility)
	if lease == nil {
		t.Fatalf("rejection published no executable repair: %s", out.Summary)
	}
	ctx.Mutable.SetAnswerDiagramRelationRepairLease(lease)
	return lease
}

// The requested thread is drawn only as a waker, on a row no lookup bound.
// The rejection names the one published row to draw and its operation in a
// short message; that one addition is accepted.
func TestHMC376PublicMissingWakeArrowOneAddition(t *testing.T) {
	ctx, rows := lifelineFixtureHMC376(t, lifelineBothRolesTraceHMC376, "wakeup_chain", "event_search")
	presented := lifelinePresentedWakesHMC376(rows)
	doc := lifelineDocHMC376(rows, presented[1:2], lifelinePerThreadNodeHMC376)
	out := lifelineEmitHMC376(t, ctx, doc)
	if issues := lifelineIssuesHMC376(out.Summary); out.Success || len(issues) != 1 || issues[0] != string(DiagramParticipantCoverageIdentityMissing) {
		t.Fatalf("want only the missing participant arrow, got success=%t %v: %s", out.Success, issues, out.Summary)
	}
	hint := lifelineHintHMC376(t, out.Summary, string(DiagramParticipantCoverageIdentityMissing))
	row := presented[0]
	for _, want := range []string{
		`Participant "` + hmc242ParticipantQuote + `" is the recorded endpoint "target-42"`,
		"Published row to draw: " + runtimeDiagramRecipeRowText(row),
		"Add it as one arrow whose to_node is that endpoint's lifeline",
		`diagram_edge_edits action "add" with the addition_ref listed for this row's source`,
	} {
		if !strings.Contains(hint, want) {
			t.Errorf("repair text lacks %q: %s", want, hint)
		}
	}
	if strings.Contains(hint, "For every typed incident_required participant") || lifelineLongHexHMC376.MatchString(hint) {
		t.Errorf("repair text repeats the general participant guidance or long ids: %s", hint)
	}
	if limit := 900 + len(row.SupportRefs[0]); len(hint) > limit {
		t.Errorf("repair text is %d bytes, want at most %d", len(hint), limit)
	}
	// The retry metadata names the mismatch and advertises no participant
	// label operation, because none is published for it.
	if raw := out.Repair.Metadata[types.ToolRepairMetaDiagramParticipantRepairDeltaJSON]; !strings.Contains(raw, `"issue":"required_participant_identity_not_visible"`) || strings.Contains(raw, "identity_action") {
		t.Errorf("retry metadata lost the mismatch or advertises an unpublished participant operation: %s", raw)
	}
	lease := lifelineInstallRejectionLeaseHMC376(t, ctx, out)
	// The addition is selected the way the message says: by the row's source.
	var addition *types.AnswerDiagramRelationRepairCandidate
	for i, candidate := range lease.AllowedAdditions {
		if candidate.RelationKind == row.Kind && candidate.Source == strings.Join(row.SupportRefs, "; ") {
			if addition != nil {
				t.Fatalf("two published additions share the named source: %+v", lease.AllowedAdditions)
			}
			addition = &lease.AllowedAdditions[i]
		}
	}
	if addition == nil || addition.ToNativeParticipant == "" || len(addition.FromNodeIDs) != 1 {
		t.Fatalf("the named row has no published addition: %+v", lease.AllowedAdditions)
	}
	// The woken end goes on the thread's existing lifeline T42, although that
	// lifeline's first arrow was recorded by another lookup on another lane.
	placement := sequencePlacementPublicRef(t, ctx, "T42->>T12: 唤醒1")
	raw, _ := json.Marshal(map[string]any{"unchanged_block_ids": []string{"summary", "table"}, "diagram_edge_edits": []any{map[string]any{
		"addition_ref": addition.AdditionRef, "action": "add", "placement_ref": placement,
		"from_node_visible_label": "loader-11", "to_native_participant": addition.ToNativeParticipant,
		"edge": map[string]any{"from_node": addition.FromNodeIDs[0], "to_node": "T42", "visible_label": "唤醒0"}}}})
	patched, err := (&EmitAnswerDocumentPatch{}).Execute(ctx, raw)
	if err != nil || !patched.Success {
		t.Fatalf("the one named addition did not repair the diagram: %v %s", err, patched.Summary)
	}
	got := ctx.Mutable.AnswerDocumentV2()
	if got == nil || len(got.Blocks[1].EdgeAnchors) != 2 || !strings.Contains(got.Blocks[1].Diagram.Body, "->>T42: 唤醒0") || !strings.Contains(got.Blocks[1].Diagram.Body, "T42->>T12: 唤醒1") ||
		strings.Index(got.Blocks[1].Diagram.Body, "->>T42: 唤醒0") > strings.Index(got.Blocks[1].Diagram.Body, "T42->>T12: 唤醒1") {
		t.Fatalf("repair lost or misplaced a recorded wake arrow: %+v", got)
	}
}

// An unproven boundary row left on the requested participant contradicts the
// arrow the repair asks for. The one rejection names both changes, and one
// patch carrying them both is accepted.
func TestHMC376PublicMissingWakeArrowNamesTheStaleBoundary(t *testing.T) {
	ctx, rows := lifelineFixtureHMC376(t, lifelineBothRolesTraceHMC376, "wakeup_chain", "event_search")
	presented := lifelinePresentedWakesHMC376(rows)
	doc := lifelineDocHMC376(rows, presented[1:2], lifelinePerThreadNodeHMC376)
	doc.Blocks[1].ParticipantBoundaries = []types.DiagramParticipantBoundary{{Participant: hmc242ParticipantQuote, Status: types.DiagramParticipantBoundaryUnproven}}
	out := lifelineEmitHMC376(t, ctx, doc)
	if issues := lifelineIssuesHMC376(out.Summary); out.Success || len(issues) != 1 || issues[0] != string(DiagramParticipantCoverageIdentityMissing) {
		t.Fatalf("want only the missing participant arrow, got success=%t %v: %s", out.Success, issues, out.Summary)
	}
	hint := lifelineHintHMC376(t, out.Summary, string(DiagramParticipantCoverageIdentityMissing))
	blockID := doc.Blocks[1].ID
	for _, want := range []string{"The same repair must also drop this participant's unproven boundary row", fmt.Sprintf("diagram_boundary_replacements for block %q", blockID), "in a full re-emit omit that row"} {
		if !strings.Contains(hint, want) {
			t.Fatalf("repair text lacks %q: %s", want, hint)
		}
	}
	lease := lifelineInstallRejectionLeaseHMC376(t, ctx, out)
	if schema := string((&EmitAnswerDocumentPatch{}).parametersForContext(nil, ctx.Mutable, ctx)); !strings.Contains(schema, `"diagram_boundary_replacements"`) {
		t.Fatalf("the named boundary operation is not published: %s", schema)
	}
	var addition *types.AnswerDiagramRelationRepairCandidate
	for i, candidate := range lease.AllowedAdditions {
		if candidate.RelationKind == presented[0].Kind && candidate.Source == strings.Join(presented[0].SupportRefs, "; ") {
			addition = &lease.AllowedAdditions[i]
		}
	}
	if addition == nil {
		t.Fatalf("the named row has no published addition: %+v", lease.AllowedAdditions)
	}
	raw, _ := json.Marshal(map[string]any{"unchanged_block_ids": []string{"summary", "table"},
		"diagram_boundary_replacements": []any{map[string]any{"block_id": blockID, "participant_boundaries": []any{}}},
		"diagram_edge_edits": []any{map[string]any{"addition_ref": addition.AdditionRef, "action": "add", "placement_ref": sequencePlacementPublicRef(t, ctx, "T42->>T12: 唤醒1"),
			"from_node_visible_label": "loader-11", "to_native_participant": addition.ToNativeParticipant,
			"edge": map[string]any{"from_node": addition.FromNodeIDs[0], "to_node": "T42", "visible_label": "唤醒0"}}}})
	patched, err := (&EmitAnswerDocumentPatch{}).Execute(ctx, raw)
	if err != nil || !patched.Success {
		t.Fatalf("the two named changes did not repair the diagram in one patch: %v %s", err, patched.Summary)
	}
	if got := ctx.Mutable.AnswerDocumentV2(); got == nil || len(got.Blocks[1].ParticipantBoundaries) != 0 || len(got.Blocks[1].EdgeAnchors) != 2 {
		t.Fatalf("repair kept the stale boundary or lost a recorded wake arrow: %+v", got)
	}
}

// A participant reference that was never published is refused with what the
// draft does publish, so the next call does not have to guess again.
func TestHMC376PublicUnpublishedParticipantRefNamesOperations(t *testing.T) {
	ctx, rows := lifelineFixtureHMC376(t, lifelineBothRolesTraceHMC376, "wakeup_chain", "event_search")
	presented := lifelinePresentedWakesHMC376(rows)
	out := lifelineEmitHMC376(t, ctx, lifelineDocHMC376(rows, presented[1:2], lifelinePerThreadNodeHMC376))
	lease := lifelineInstallRejectionLeaseHMC376(t, ctx, out)
	if len(lease.ParticipantVisibilityFailures) != 0 || len(lease.AllowedAdditions) == 0 {
		t.Fatalf("fixture must publish additions and no participant reference: %+v", lease)
	}
	before, _ := json.Marshal(ctx.Mutable.LastRejectedAnswerDocumentV2())
	for _, guess := range []string{"tid 42", "T42", "target-42", presented[0].ToNode} {
		raw, _ := json.Marshal(map[string]any{"unchanged_block_ids": []string{"summary", "table"}, "diagram_participant_edits": []any{map[string]any{
			"participant_ref": guess, "action": "ensure_visible", "node_id": "T42", "visible_label": "target 线程"}}})
		refused, err := (&EmitAnswerDocumentPatch{}).Execute(ctx, raw)
		if err != nil || refused.Success {
			t.Fatalf("an unpublished participant reference was accepted: %v %+v", err, refused)
		}
		for _, want := range []string{"diagram_participant_edits[0]", "is stale, unknown, or does not allow action", "No participant_ref is published for this draft",
			"cannot be composed from a thread, node or participant name", `diagram_edge_edits action "add" with an addition_ref (`} {
			if !strings.Contains(refused.Summary, want) {
				t.Fatalf("refusal of %q lacks %q: %s", guess, want, refused.Summary)
			}
		}
	}
	if after, _ := json.Marshal(ctx.Mutable.LastRejectedAnswerDocumentV2()); string(before) != string(after) {
		t.Fatal("a refused participant reference changed the draft")
	}
	// With a published reference the refusal lists it instead.
	published := atomicDiagramPublishedParticipantOperations(&types.AnswerDiagramRelationRepairLease{ParticipantVisibilityFailures: []types.AnswerDiagramParticipantVisibilityRepairFailure{{
		ParticipantRef: "pv1-example", Participant: "Worker", AllowedParticipantActions: []types.AnswerDiagramParticipantVisibilityRepairAction{types.AnswerDiagramParticipantVisibilityRepairEnsureVisible}}}})
	if published != `Published participant_ref values for this draft: "pv1-example" for participant "Worker" (ensure_visible). Copy one exactly.` {
		t.Fatalf("published references are not listed: %s", published)
	}
	if none := atomicDiagramPublishedParticipantOperations(nil); !strings.Contains(none, "no other local diagram operation is") {
		t.Fatalf("an empty draft must not advertise an operation: %s", none)
	}
}
