package agent

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/tool"
	"github.com/hanchaoqun/codrax/internal/types"
)

// All thirteen call relations are read and grounded through the production
// tools. The numbered functions/nodes are fixture identities, not r1054 names.
func b1649ThirteenActualPairs(t *testing.T) (*types.BusContext, *types.AnswerDocumentV2, map[string]string) {
	t.Helper()
	repo := t.TempDir()
	var source, body strings.Builder
	source.WriteString("package sample\n")
	body.WriteString("sequenceDiagram\n    participant n0 as Caller\n")
	for i := 1; i <= 13; i++ {
		fmt.Fprintf(&source, "func Step%02d() {}\n", i)
		fmt.Fprintf(&body, "    participant n%d as Step%02d\n", i, i)
	}
	source.WriteString("func Caller() {\n")
	var items []json.RawMessage
	labels := make(map[string]string)
	for i := 1; i <= 13; i++ {
		fmt.Fprintf(&source, " Step%02d()\n", i)
		label := fmt.Sprintf("invoke step %02d", i)
		labels[fmt.Sprintf("n%d", i)] = label
		fmt.Fprintf(&body, "    n0->>n%d: %s\n", i, label)
		items = append(items, json.RawMessage(fmt.Sprintf(`{"scope":"line","evidence_kind":"mechanism","subject":"sample.Caller","predicate":"calls","object":"Step%02d","source":"calls.go","line_start":%d,"anchor_kind":"call","anchor_symbol":"Step%02d"}`, i, 15+i, i)))
	}
	source.WriteString("}\n")
	if err := os.WriteFile(filepath.Join(repo, "calls.go"), []byte(source.String()), 0o600); err != nil {
		t.Fatal(err)
	}
	bus := &types.BusContext{RepoRoot: repo, WorkDir: repo, Mutable: types.NewMutableState("B1649 thirteen distinct calls")}
	read, err := (&tool.ReadFile{}).Execute(bus, json.RawMessage(`{"path":"calls.go","offset":0,"limit":100}`))
	if err != nil || !read.Success {
		t.Fatalf("actual read failed: err=%v result=%+v", err, read)
	}
	bus.ToolResults = []types.ToolResult{read}
	bus.Mutable.SetTurnAArtifacts(types.TurnAArtifacts{ToolResults: bus.ToolResults})
	params, _ := json.Marshal(map[string]any{"items": items})
	emit, err := (&tool.EmitEvidence{}).Execute(bus, params)
	if err != nil || !emit.Success {
		t.Fatalf("actual call grounding failed: err=%v result=%+v", err, emit)
	}
	seen := make(map[string]bool)
	for _, ev := range bus.Mutable.EmittedEvidence() {
		if ev.Producer != tool.EmitEvidenceProducer || types.ClaimFormOf(ev) != types.ClaimCallEdge {
			continue
		}
		if !ev.IsCitable() || ev.GroundingStatus != types.GroundingGrounded || ev.Subject != "sample.Caller" ||
			ev.Source != "calls.go" || ev.LineStart < 16 || ev.LineStart > 28 || ev.LineEnd != ev.LineStart ||
			ev.Object != fmt.Sprintf("Step%02d", ev.LineStart-15) || !strings.Contains(ev.Snippet, ev.Object+"()") || ev.ID == "" || seen[ev.ID] {
			t.Fatalf("expected thirteen independent native call-site receipts: %+v", ev)
		}
		seen[ev.ID] = true
		bus.EvidenceItems = append(bus.EvidenceItems, ev)
	}
	if len(bus.EvidenceItems) != 13 {
		t.Fatalf("actual producer lost relations: %+v", bus.Mutable.EmittedEvidence())
	}
	bus.AnalysisIR = &types.AnalysisIR{RequestModel: types.RequestModel{Intent: types.IntentTrace, PredicateAxis: types.AxisCall}}
	doc := &types.AnswerDocumentV2{DocumentModel: "v2", Blocks: []types.AnswerBlock{
		{ID: "summary", Kind: types.BlockSummary, Text: "The caller invokes thirteen independent operations in source order."},
		{ID: "diagram", Kind: types.BlockDiagram, Diagram: &types.AnswerDiagramBlock{Kind: types.DiagramSequence, Language: "mermaid", Body: body.String()}},
	}}
	return bus, doc, labels
}

func b1649StagedDelta(t *testing.T, result *types.ToolResult, failures, additions int) types.AnswerDiagramRelationRepairDelta {
	t.Helper()
	if result == nil || result.Success || result.Repair == nil {
		t.Fatalf("expected actual structural rejection: %+v", result)
	}
	var delta types.AnswerDiagramRelationRepairDelta
	if err := json.Unmarshal([]byte(result.Repair.Metadata[types.ToolRepairMetaDiagramRelationRepairDeltaJSON]), &delta); err != nil {
		t.Fatalf("actual producer delta: %v %+v", err, result)
	}
	if len(delta.Failures) != failures || len(delta.AllowedAdditions) != additions || !delta.PreserveUnlistedEdges {
		t.Fatalf("current bounded roster mismatch: want %d/%d, got %+v", failures, additions, delta)
	}
	for _, failure := range delta.Failures {
		if failure.Issue != types.DiagramRelationFailureMissingGroundedCallAnchor || failure.BodyOccurrence != 1 {
			t.Fatalf("must exercise actual call-anchor failure, not another contract: %+v", failure)
		}
	}
	return delta
}

func b1649AttachCurrentChoices(t *testing.T, ctx *types.AgentContext, labels map[string]string) json.RawMessage {
	t.Helper()
	lease := ctx.Mutable.AnswerDiagramRelationRepairLease()
	if lease == nil {
		t.Fatal("actual installer did not publish the current lease")
	}
	schema := (&tool.EmitAnswerDocumentPatch{}).ParametersFor(ctx)
	var edits []map[string]any
	for _, candidate := range lease.AllowedAdditions {
		var owners []types.AnswerDiagramRelationRepairFailure
		for _, failure := range lease.Failures {
			if types.AnswerDiagramRelationRepairFailureCanAttachCandidate(failure, candidate) {
				owners = append(owners, failure)
			}
		}
		if len(owners) != 1 || !owners[0].AllowsAction("attach") || !strings.Contains(string(schema), candidate.AdditionRef) || !strings.Contains(string(schema), owners[0].FailureRef) {
			t.Fatalf("published choice must own one exact current pair and appear in schema: candidate=%+v owners=%+v", candidate, owners)
		}
		owner := owners[0]
		label, ok := labels[owner.ToNode]
		if !ok {
			t.Fatalf("unknown model node: %+v", owner)
		}
		edits = append(edits, map[string]any{"action": "attach", "failure_ref": owner.FailureRef, "addition_ref": candidate.AdditionRef,
			"edge": map[string]string{"from_node": owner.FromNode, "to_node": owner.ToNode, "visible_label": label}})
	}
	raw, err := json.Marshal(map[string]any{"unchanged_block_ids": []string{"summary"}, "diagram_edge_edits": edits})
	if err != nil {
		t.Fatal(err)
	}
	return raw
}

func b1649AssertCandidateCallSites(t *testing.T, delta types.AnswerDiagramRelationRepairDelta, evidence []types.EvidenceItem) {
	t.Helper()
	for _, candidate := range delta.AllowedAdditions {
		matched := 0
		for _, ev := range evidence {
			if candidate.EvidenceID == ev.ID && candidate.Source == fmt.Sprintf("%s:%d", ev.Source, ev.LineStart) &&
				candidate.RelationKind == types.DiagramRelCall && candidate.FromIdentity == ev.Subject && candidate.ToIdentity == ev.Object {
				matched++
			}
		}
		if matched != 1 {
			t.Fatalf("current choice must retain one exact original call-site row: %+v", candidate)
		}
	}
}

func b1649RejectHistoricalPatchWithoutMutation(t *testing.T, bus *types.BusContext, patch json.RawMessage, wantCode string) {
	t.Helper()
	snapshot := func() []byte {
		raw, err := json.Marshal([]any{bus.Mutable.AnswerDocumentV2(), bus.Mutable.PendingAnswerDocumentPatchBase(),
			bus.Mutable.LastRejectedAnswerDocumentV2(), bus.Mutable.AnswerDiagramRelationRepairLease()})
		if err != nil {
			t.Fatal(err)
		}
		return raw
	}
	before := snapshot()
	result, err := (&tool.EmitAnswerDocumentPatch{}).Execute(bus, patch)
	if err != nil || result.Success || result.Repair == nil ||
		result.Repair.Metadata[types.ToolRepairMetaAnswerDocumentPatchOutcome] != types.AnswerDocumentPatchOutcomeNotStaged {
		t.Fatalf("historical refs must fail without staging: err=%v result=%+v", err, result)
	}
	if wantCode != "" && result.Repair.Code != wantCode {
		t.Fatalf("wrong historical-ref diagnosis: want=%s result=%+v", wantCode, result)
	}
	if string(before) != string(snapshot()) {
		t.Fatal("rejected old refs changed accepted/rejected/staged documents or the active capability")
	}
	t.Logf("historical reference refused: code=%s, outcome=%s", result.Repair.Code, result.Repair.Metadata[types.ToolRepairMetaAnswerDocumentPatchOutcome])
}

func TestB1649ThirteenActualPairsRepairAcrossEightThenFive(t *testing.T) {
	bus, model, labels := b1649ThirteenActualPairs(t)
	ctx := &types.AgentContext{Mutable: bus.Mutable, AnalysisIR: bus.AnalysisIR, EvidenceItems: bus.EvidenceItems}
	modelJSON, _ := json.Marshal(model)
	full, err := (&tool.EmitAnswerDocument{}).Execute(bus, modelJSON)
	if err != nil {
		t.Fatal(err)
	}
	b1649StagedDelta(t, &full, 13, 8)
	if bus.Mutable.AnswerDocumentV2() != nil || bus.Mutable.PendingAnswerDocumentPatchBase() != nil || bus.Mutable.LastRejectedAnswerDocumentV2() == nil {
		t.Fatal("failed full emit must remain the rejected draft, not an accepted or manually staged answer")
	}
	if !installAnswerDocDiagramRelationRepairLease(ctx, ctx.Mutable, &full, true) {
		t.Fatal("actual finalizer installer rejected the producer's 13/8 delta")
	}
	first := b1649StagedDelta(t, &full, 13, 8)
	b1649AssertCandidateCallSites(t, first, bus.EvidenceItems)
	t.Log("actual full reject -> installed lease: 13 failures, 8 source-backed choices")
	firstPatch := b1649AttachCurrentChoices(t, ctx, labels)
	partial, err := (&tool.EmitAnswerDocumentPatch{}).Execute(bus, firstPatch)
	if err != nil {
		t.Fatal(err)
	}
	b1649StagedDelta(t, &partial, 5, 5)
	if partial.Repair.Metadata[types.ToolRepairMetaAnswerDocumentPatchOutcome] != types.AnswerDocumentPatchOutcomeStagedForRetry {
		t.Fatalf("eight chosen attaches must be staged for the five remaining ordinary failures: %+v", partial)
	}
	staged := bus.Mutable.PendingAnswerDocumentPatchBase()
	if staged == nil || len(staged.Blocks[1].EdgeAnchors) != 8 || staged.Blocks[1].Diagram.Body != model.Blocks[1].Diagram.Body ||
		!reflect.DeepEqual(staged.Blocks[0], model.Blocks[0]) || bus.Mutable.AnswerDocumentV2() != nil || bus.Mutable.AnswerDiagramRelationRepairLease() != nil {
		t.Fatalf("partial progress must preserve the unpublished model body and discharge the old lease: %+v", staged)
	}
	b1649RejectHistoricalPatchWithoutMutation(t, bus, firstPatch, types.ToolRepairCodeAnswerDocRelationRepairLeaseAbsent)
	if !installAnswerDocDiagramRelationRepairLease(ctx, ctx.Mutable, &partial, true) {
		t.Fatal("actual finalizer installer must bind five repairs to the staged generation")
	}
	second := b1649StagedDelta(t, &partial, 5, 5)
	b1649AssertCandidateCallSites(t, second, bus.EvidenceItems)
	lease := bus.Mutable.AnswerDiagramRelationRepairLease()
	if len(lease.Blocks) != 1 || !reflect.DeepEqual(lease.Blocks[0].BaseAnchors, staged.Blocks[1].EdgeAnchors) {
		t.Fatalf("new lease must use eight-anchor staged base, not the still-present original rejected draft: %+v", lease)
	}
	t.Log("actual patch reject -> staged generation -> installed lease: 5 failures, 5 source-backed choices; first 8 anchors are the new base")
	b1649RejectHistoricalPatchWithoutMutation(t, bus, firstPatch, types.ToolRepairCodeAnswerDocRelationRepairScope)
	oldRefs := make(map[string]bool)
	selectedSources := make(map[string]bool)
	for _, f := range first.Failures {
		oldRefs[f.FailureRef] = true
	}
	for _, a := range first.AllowedAdditions {
		oldRefs[a.AdditionRef] = true
		selectedSources[a.EvidenceID] = true
	}
	for _, f := range second.Failures {
		if oldRefs[f.FailureRef] {
			t.Fatalf("staged generation reused an old failure ref: %+v", f)
		}
	}
	for _, a := range second.AllowedAdditions {
		if oldRefs[a.AdditionRef] || selectedSources[a.EvidenceID] {
			t.Fatalf("staged generation reused an old addition ref: %+v", a)
		}
		selectedSources[a.EvidenceID] = true
	}
	if len(selectedSources) != len(bus.EvidenceItems) {
		t.Fatalf("bounded generations lost/duplicated source choices: %d of %d", len(selectedSources), len(bus.EvidenceItems))
	}
	secondPatch := b1649AttachCurrentChoices(t, ctx, labels)
	finished, err := (&tool.EmitAnswerDocumentPatch{}).Execute(bus, secondPatch)
	if err != nil || !finished.Success {
		t.Fatalf("the final five exact choices must publish: err=%v result=%+v", err, finished)
	}
	got := bus.Mutable.AnswerDocumentV2()
	if got == nil || len(got.Blocks) < 2 || len(got.Blocks[1].EdgeAnchors) != 13 ||
		got.Blocks[1].Diagram.Body != model.Blocks[1].Diagram.Body || !reflect.DeepEqual(got.Blocks[0], model.Blocks[0]) ||
		!reflect.DeepEqual(got.Blocks[1].EdgeAnchors[:8], staged.Blocks[1].EdgeAnchors) {
		t.Fatalf("bounded repairs lost the thirteen model-authored relations or prior eight anchors: %+v", got)
	}
	if issues := tool.DiagramCallEdgeEvidenceMismatches(got, &types.AnswerSemanticView{Family: types.QFCallChain}, bus.EvidenceItems); len(issues) != 0 {
		t.Fatalf("final ordinary call gate still reports failures: %+v", issues)
	}
	if bus.Mutable.PendingAnswerDocumentPatchBase() != nil || bus.Mutable.AnswerDiagramRelationRepairLease() != nil {
		t.Fatal("successful persistence must consume the retry generation")
	}
	b1649RejectHistoricalPatchWithoutMutation(t, bus, secondPatch, types.ToolRepairCodeAnswerDocRelationRepairLeaseAbsent)
	t.Log("actual final patch persisted: 13 anchors, original 13-message body preserved byte-for-byte, retry generation consumed")
	after, _ := json.Marshal(model)
	if string(after) != string(modelJSON) {
		t.Fatal("production altered the caller's original model document")
	}
}
