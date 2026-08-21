package tool

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/types"
)

// emit_answer_document_patch_test.go — R16-c2 tool-level lock tests.
// The underlying ApplyPatch logic + 11 reject cases are pinned in
// internal/types/answer_document_v2_patch_test.go; this file pins
// the tool dispatch surface (peek prev emit, decode params,
// merge, write Mutable).

// helper: build a fresh BusContext with a prev emit.
func newPatchTestBusContext() *types.BusContext {
	bus := &types.BusContext{Mutable: &types.MutableState{}}
	bus.Mutable.SetAnswerDocumentV2WithMutation(types.MutationReplaceAll, &types.AnswerDocumentV2{
		DocumentModel: "v2",
		Citations:     []types.Citation{{File: "x.go", Line: 10}},
		Blocks: []types.AnswerBlock{
			{
				ID: "s1", Kind: types.BlockSummary,
				SurfaceRole: types.SurfacePrincipal,
				Text:        "summary text",
				FacetIDs:    []string{"current_code_path"},
				ClaimUses: []types.RenderedClaimUse{
					{ClaimForm: types.ClaimDefinitionFact, EvidenceID: "ev1"},
				},
			},
			{
				ID: "list1", Kind: types.BlockOrderedList,
				ClaimUses: []types.RenderedClaimUse{
					{ClaimForm: types.ClaimCallEdge},
				},
				Items: []types.AnswerBlockItem{
					{ID: "i1", Label: "A", CitationRef: 0},
				},
			},
		},
	})
	return bus
}

// TestEmitAnswerDocumentPatch_NoPrevRejects pins the "first
// dispatch must use emit_answer_document" guard.
func TestEmitAnswerDocumentPatch_NoPrevRejects(t *testing.T) {
	bus := &types.BusContext{Mutable: &types.MutableState{}}
	tool := &EmitAnswerDocumentPatch{}
	res, _ := tool.Execute(bus, json.RawMessage(`{"unchanged_block_ids":["x"]}`))
	if res.Success {
		t.Error("no prev emit must reject")
	}
	if !strings.Contains(res.Summary, "no previous emit") {
		t.Errorf("error must name the missing prev: %q", res.Summary)
	}
}

func TestEmitAnswerDocumentPatch_RejectsTopLevelRelationClaimsWithExactCarrierPath(t *testing.T) {
	bus := newPatchTestBusContext()
	tool := &EmitAnswerDocumentPatch{}
	res, err := tool.Execute(bus, json.RawMessage(`{
		"unchanged_block_ids":["s1","list1"],
		"relation_claims":[{"authority_id":"trace:test"}]
	}`))
	if err != nil {
		t.Fatalf("unexpected exec error: %v", err)
	}
	if res.Success {
		t.Fatalf("top-level patch relation_claims must not be silently quarantined: %+v", res)
	}
	for _, want := range []string{"replace_blocks[i].relation_claims", "add_blocks[i].relation_claims", "never at $.relation_claims"} {
		if !strings.Contains(res.Summary, want) {
			t.Fatalf("misplaced patch carrier error missing %q: %s", want, res.Summary)
		}
	}
}

func TestEmitAnswerDocumentPatch_EmptyRetryStatePrevRejects(t *testing.T) {
	mut := types.NewMutableState("retry")
	mut.SetRetryState(&types.RetryState{
		Attempt:      1,
		PrevEmitJSON: []byte(`{"blocks":[]}`),
	})
	bus := &types.BusContext{Mutable: mut}
	tool := &EmitAnswerDocumentPatch{}
	res, _ := tool.Execute(bus, json.RawMessage(`{"unchanged_block_ids":["x"]}`))
	if res.Success {
		t.Error("empty retry-state previous emit must reject")
	}
	if !strings.Contains(res.Summary, "no previous emit") {
		t.Errorf("error must name the missing usable prev: %q", res.Summary)
	}
}

// TestEmitAnswerDocumentPatch_EmptyPatchRejects pins the "every
// retry must declare some change" invariant at the tool layer.
func TestEmitAnswerDocumentPatch_EmptyPatchRejects(t *testing.T) {
	bus := newPatchTestBusContext()
	tool := &EmitAnswerDocumentPatch{}
	res, _ := tool.Execute(bus, json.RawMessage(`{}`))
	if res.Success {
		t.Error("empty patch must reject")
	}
}

func TestEmitAnswerDocumentPatch_EnforcesTypedRelationRepairLease(t *testing.T) {
	bus := newPatchTestBusContext()
	base := bus.Mutable.AnswerDocumentV2()
	bus.Mutable.SetAnswerDiagramRelationRepairLease(types.NewAnswerDiagramRelationRepairLease(base,
		[]types.AnswerDiagramRelationRepairFailure{{
			BlockID: "list1", Issue: "call_edge_unproven",
			FromNode: "Analyze", ToNode: "Explore",
		}}, nil))

	res, err := (&EmitAnswerDocumentPatch{}).Execute(bus, json.RawMessage(`{
		"unchanged_block_ids":["s1"],
		"replace_blocks":[{
			"id":"list1",
			"kind":"ordered_list",
			"items":[{"id":"i1","label":"A","citation_ref":0}],
			"edge_anchors":[{
				"from_node":"Extract","to_node":"Finalize","relation_kind":"call"
			}]
		}]
	}`))
	if err != nil {
		t.Fatalf("unexpected execution error: %v", err)
	}
	if res.Success || res.Repair == nil || res.Repair.Code != types.ToolRepairCodeAnswerDocRelationRepairScope {
		t.Fatalf("unlisted relation addition must be rejected by production patch path: %+v", res)
	}
	if !strings.Contains(res.Summary, "unlisted_relation_added") ||
		strings.TrimSpace(res.Repair.Metadata[types.ToolRepairMetaDiagramRelationRepairDeltaJSON]) == "" {
		t.Fatalf("scope rejection must carry compact typed retry metadata: %+v", res)
	}
	if got := bus.Mutable.AnswerDocumentV2(); got == nil || len(got.Blocks[1].EdgeAnchors) != 0 {
		t.Fatalf("rejected patch must not mutate accepted base: %+v", got)
	}
}

func TestEmitAnswerDocumentPatch_ReportsAbsentRelationLeaseWithoutInventingRefs(t *testing.T) {
	for _, tc := range []struct {
		name      string
		params    string
		wantField string
	}{
		{
			name: "historical failure ref",
			params: `{"unchanged_block_ids":["s1","list1"],"diagram_edge_edits":[` +
				`{"failure_ref":"rf1-000000000000000000000000","action":"remove"}]}`,
			wantField: "diagram_edge_edits[0].failure_ref",
		},
		{
			name: "historical addition ref",
			params: `{"unchanged_block_ids":["s1","list1"],"diagram_edge_edits":[` +
				`{"addition_ref":"ra1-000000000000000000000000","action":"add",` +
				`"edge":{"from_node":"A","to_node":"B","visible_label":"model label"}}]}`,
			wantField: "diagram_edge_edits[0].addition_ref",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			bus := newPatchTestBusContext()
			before := bus.Mutable.AnswerDocumentV2()
			if lease := bus.Mutable.AnswerDiagramRelationRepairLease(); lease != nil {
				t.Fatalf("fixture must begin without a live relation lease: %+v", lease)
			}
			res, err := (&EmitAnswerDocumentPatch{}).Execute(bus, json.RawMessage(tc.params))
			if err != nil {
				t.Fatalf("unexpected execution error: %v", err)
			}
			if res.Success || res.Repair == nil ||
				res.Repair.Code != types.ToolRepairCodeAnswerDocRelationRepairLeaseAbsent ||
				res.Repair.Metadata[types.ToolRepairMetaDiagramRelationRepairLeaseStatus] != "absent" {
				t.Fatalf("ref without a live lease must publish the exact absent state: %+v", res)
			}
			if !reflect.DeepEqual(res.Repair.Fields, []string{tc.wantField}) ||
				strings.TrimSpace(res.Repair.Metadata[types.ToolRepairMetaDiagramRelationRepairDeltaJSON]) != "" ||
				!strings.Contains(res.Summary, "not present in a live relation-repair lease") {
				t.Fatalf("absent state must identify the structured ref and mint no replacement roster: %+v", res)
			}
			if after := bus.Mutable.AnswerDocumentV2(); !reflect.DeepEqual(before, after) {
				t.Fatalf("rejected historical ref must not mutate the accepted draft: before=%+v after=%+v", before, after)
			}
		})
	}
}

func TestEmitAnswerDocumentPatch_RelationLeaseRejectsCrossKindDiagramReplacement(t *testing.T) {
	mut := types.NewMutableState("relation repair")
	base := &types.AnswerDocumentV2{DocumentModel: "v2", Blocks: []types.AnswerBlock{
		{ID: "summary", Kind: types.BlockSummary, Text: "summary"},
		{
			ID: "flow", Kind: types.BlockDiagram,
			Diagram:     &types.AnswerDiagramBlock{Kind: types.DiagramSequence, Language: "mermaid", Body: "sequenceDiagram\n A->>B: call"},
			EdgeAnchors: []types.DiagramEdgeAnchor{{FromNode: "A", ToNode: "B", RelationKind: types.DiagramRelCall}},
		},
	}}
	mut.SetAnswerDocumentV2WithMutation(types.MutationReplaceAll, base)
	mut.SetAnswerDiagramRelationRepairLease(types.NewAnswerDiagramRelationRepairLease(base,
		[]types.AnswerDiagramRelationRepairFailure{{
			BlockID: "flow", Issue: "call_edge_unproven", FromNode: "A", ToNode: "B",
		}}, nil))
	bus := &types.BusContext{Mutable: mut}

	res, err := (&EmitAnswerDocumentPatch{}).Execute(bus, json.RawMessage(`{
		"unchanged_block_ids":["summary"],
		"replace_blocks":[{"id":"flow","kind":"table","columns":["阶段"],"items":[{"cells":["analyze"]}]}]
	}`))
	if err != nil {
		t.Fatalf("unexpected execution error: %v", err)
	}
	if res.Success || res.Repair == nil || res.Repair.Code != types.ToolRepairCodeAnswerDocRelationRepairScope ||
		!strings.Contains(res.Summary, "whole_replace_not_authorized") {
		t.Fatalf("cross-kind relation repair must fail at the typed lease: %+v", res)
	}
	got := mut.AnswerDocumentV2()
	if got == nil || got.Blocks[1].Kind != types.BlockDiagram {
		t.Fatalf("rejected cross-kind patch must preserve the diagram base: %+v", got)
	}
}

func TestEmitAnswerDocumentPatch_AtomicEditFailureRepublishesCurrentRelationLease(t *testing.T) {
	mut := types.NewMutableState("relation repair")
	base := &types.AnswerDocumentV2{DocumentModel: "v2", Blocks: []types.AnswerBlock{
		{ID: "summary", Kind: types.BlockSummary, Text: "summary"},
		{
			ID: "flow", Kind: types.BlockDiagram,
			Diagram: &types.AnswerDiagramBlock{
				Kind: types.DiagramSequence, Language: "mermaid",
				Body: "sequenceDiagram\n A->>B: model relation",
			},
		},
	}}
	mut.SetAnswerDocumentV2WithMutation(types.MutationReplaceAll, base)
	lease := types.NewAnswerDiagramRelationRepairLease(base,
		[]types.AnswerDiagramRelationRepairFailure{{
			BlockID: "flow", Issue: "missing_call_anchor", FromNode: "A", ToNode: "B",
			RelationKind: types.DiagramRelCall,
		}}, []types.AnswerDiagramRelationRepairCandidate{{
			BlockID: "flow", FromIdentity: "Worker.Run", ToIdentity: "Sink.Store",
			RelationKind: types.DiagramRelCall, Source: "worker.go:10-12",
		}})
	if lease == nil || len(lease.Failures) != 1 {
		t.Fatalf("expected one live relation failure: %+v", lease)
	}
	mut.SetAnswerDiagramRelationRepairLease(lease)
	bus := &types.BusContext{Mutable: mut}
	staleRef := "rf1-000000000000000000000000"

	liveRef := lease.Failures[0].FailureRef
	liveAdditionRef := lease.AllowedAdditions[0].AdditionRef
	if liveAdditionRef == "" {
		t.Fatal("expected the live lease to publish an addition_ref")
	}
	tests := []struct {
		name        string
		params      string
		summaryWant string
	}{
		{
			name: "stale ref",
			params: `{"unchanged_block_ids":["summary"],"diagram_edge_edits":[` +
				`{"failure_ref":"` + staleRef + `","action":"remove"}]}`,
			summaryWant: "unknown or stale",
		},
		{
			name: "action outside live capability",
			params: `{"unchanged_block_ids":["summary"],"diagram_edge_edits":[` +
				`{"failure_ref":"` + liveRef + `","action":"relabel","visible_label":"model wording"}]}`,
			summaryWant: "does not allow action=relabel",
		},
		{
			name: "stale addition ref",
			params: `{"unchanged_block_ids":["summary"],"diagram_edge_edits":[` +
				`{"addition_ref":"ra1-000000000000000000000000","action":"add","edge":` +
				`{"from_node":"Worker","to_node":"Sink","relation_kind":"call","visible_label":"model label"}}]}`,
			summaryWant: "unknown or stale",
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			res, err := (&EmitAnswerDocumentPatch{}).Execute(bus, json.RawMessage(tc.params))
			if err != nil {
				t.Fatalf("unexpected execution error: %v", err)
			}
			if res.Success || res.Repair == nil || res.Repair.Code != types.ToolRepairCodeAnswerDocRelationRepairScope {
				t.Fatalf("invalid atomic edit must return the live typed repair capsule: %+v", res)
			}
			if !strings.Contains(res.Summary, tc.summaryWant) {
				t.Fatalf("exact executor failure %q must remain visible in the summary: %s", tc.summaryWant, res.Summary)
			}
			raw := res.Repair.Metadata[types.ToolRepairMetaDiagramRelationRepairDeltaJSON]
			if !strings.Contains(raw, `"failure_ref":"`+liveRef+`"`) ||
				strings.Contains(raw, staleRef) || !strings.Contains(raw, `"allowed_additions"`) ||
				!strings.Contains(raw, `"addition_ref":"`+liveAdditionRef+`"`) {
				t.Fatalf("repair must republish the complete current lease and exclude stale refs: %s", raw)
			}
		})
	}
	if got := mut.AnswerDocumentV2(); got == nil || got.Blocks[1].Diagram.Body != base.Blocks[1].Diagram.Body {
		t.Fatalf("rejected atomic edit must not mutate the accepted document: %+v", got)
	}
}

func TestEmitAnswerDocumentPatch_ParticipantOnlyAdditionRefExecutesAndConsumes(t *testing.T) {
	mut := types.NewMutableState("participant-only addition")
	base := &types.AnswerDocumentV2{DocumentModel: "v2", Blocks: []types.AnswerBlock{
		{ID: "summary", Kind: types.BlockSummary, Text: "summary"},
		{ID: "flow", Kind: types.BlockDiagram, Diagram: &types.AnswerDiagramBlock{
			Kind: types.DiagramFlow, Language: "mermaid", Body: "flowchart LR\n BusContext[BusContext]",
		}},
	}}
	mut.SetAnswerDocumentV2WithMutation(types.MutationReplaceAll, base)
	lease := types.NewAnswerDiagramRelationRepairLease(base, nil, []types.AnswerDiagramRelationRepairCandidate{{
		BlockID: "flow", RelationKind: types.DiagramRelArgumentFlow,
		FromIdentity: "o.busCtx", ToIdentity: "ctxbuilder.BuildAgentContext", Source: "internal/orchestrator/extract_work.go:15",
	}})
	if lease == nil || len(lease.Failures) != 0 || len(lease.AllowedAdditions) != 1 {
		t.Fatalf("expected one additions-only live lease: %+v", lease)
	}
	mut.SetAnswerDiagramRelationRepairLease(lease)
	bus := &types.BusContext{Mutable: mut}
	params := fmt.Sprintf(`{
		"unchanged_block_ids":["summary"],
		"diagram_edge_edits":[{"action":"add","addition_ref":%q,
			"edge":{"from_node":"BusContext","to_node":"BuildAgentContext","visible_label":"作为参数传递"}}]
	}`, lease.AllowedAdditions[0].AdditionRef)
	res, err := (&EmitAnswerDocumentPatch{}).Execute(bus, json.RawMessage(params))
	if err != nil || !res.Success {
		t.Fatalf("same-generation participant candidate must be executable: err=%v res=%+v", err, res)
	}
	if got := mut.AnswerDiagramRelationRepairLease(); got != nil {
		t.Fatalf("successful additions-only generation must be consumed before later independent checks: %+v", got)
	}
	doc := mut.AnswerDocumentV2()
	if doc == nil || len(doc.Blocks) != 2 || len(doc.Blocks[1].EdgeAnchors) != 1 {
		t.Fatalf("model-authored visible edge and typed hidden tuple were not persisted: %+v", doc)
	}
	anchor := doc.Blocks[1].EdgeAnchors[0]
	if anchor.FromNode != "BusContext" || anchor.ToNode != "BuildAgentContext" ||
		anchor.FromIdentity != "o.busCtx" || anchor.ToIdentity != "ctxbuilder.BuildAgentContext" ||
		anchor.RelationKind != types.DiagramRelArgumentFlow || anchor.VisibleLabel != "作为参数传递" {
		t.Fatalf("addition ref must stamp only the selected hidden tuple and preserve model wording: %+v", anchor)
	}
}

func TestEmitAnswerDocumentPatch_LiveFailureRefQuarantinesLegacySelectorMirrors(t *testing.T) {
	mut := types.NewMutableState("relation repair ref-first")
	base := &types.AnswerDocumentV2{DocumentModel: "v2", Blocks: []types.AnswerBlock{
		{ID: "summary", Kind: types.BlockSummary, Text: "summary"},
		{
			ID: "flow", Kind: types.BlockDiagram,
			Diagram: &types.AnswerDiagramBlock{
				Kind: types.DiagramSequence, Language: "mermaid",
				Body: "sequenceDiagram\n A->>B: model relation",
			},
		},
	}}
	lease := types.NewAnswerDiagramRelationRepairLease(base,
		[]types.AnswerDiagramRelationRepairFailure{{
			BlockID: "flow", Issue: "missing_call_anchor", FromNode: "A", ToNode: "B",
		}}, nil)
	if lease == nil || len(lease.Failures) != 1 {
		t.Fatalf("expected one live relation failure: %+v", lease)
	}
	mut.SetAnswerDocumentV2WithMutation(types.MutationReplaceAll, base)
	mut.SetAnswerDiagramRelationRepairLease(lease)
	bus := &types.BusContext{Mutable: mut}

	params := fmt.Sprintf(`{
		"unchanged_block_ids":["summary"],
		"diagram_edge_edits":[{
			"failure_ref":%q,"action":"remove","block_id":"flow",
			"occurrence":77,"body_occurrence":99,
			"match":{"from_node":"legacy-X","to_node":"legacy-Y","relation_kind":"call"}
		}]
	}`, lease.Failures[0].FailureRef)
	res, err := (&EmitAnswerDocumentPatch{}).Execute(bus, json.RawMessage(params))
	if err != nil || !res.Success {
		t.Fatalf("live ref must outrank selector mirrors without changing action: err=%v res=%+v", err, res)
	}
	got := mut.AnswerDocumentV2()
	if got == nil || len(got.Blocks) != 2 || got.Blocks[1].Diagram == nil ||
		strings.Contains(got.Blocks[1].Diagram.Body, "A->>B") {
		t.Fatalf("ref-first remove did not target the lease-owned carrier: %+v", got)
	}
}

func TestEmitAnswerDocumentPatch_RelationLeaseAllowsOnlyStructuredCandidateAddition(t *testing.T) {
	bus := newPatchTestBusContext()
	base := bus.Mutable.AnswerDocumentV2()
	bus.Mutable.SetAnswerDiagramRelationRepairLease(types.NewAnswerDiagramRelationRepairLease(base,
		[]types.AnswerDiagramRelationRepairFailure{{
			BlockID: "list1", Issue: "call_edge_unproven", FromNode: "A", ToNode: "B",
		}},
		[]types.AnswerDiagramRelationRepairCandidate{{
			BlockID: "list1", RelationKind: types.DiagramRelPrecedence,
			FromIdentity: "analyzer", ToIdentity: "explorer", Source: "internal/types/enums.go:120-121",
		}}))

	res, err := (&EmitAnswerDocumentPatch{}).Execute(bus, json.RawMessage(`{
		"unchanged_block_ids":["s1"],
		"replace_blocks":[{
			"id":"list1","kind":"ordered_list",
			"items":[{"id":"i1","label":"A","citation_ref":0}],
			"edge_anchors":[{
				"from_node":"AnalyzeStage","to_node":"ExploreStage",
				"from_identity":"analyzer","to_identity":"explorer",
				"relation_kind":"precedence","visible_label":"model wording"
			}]
		}]
	}`))
	if err != nil || !res.Success {
		t.Fatalf("listed structured candidate must pass the production lease before ordinary authority checks: result=%+v err=%v", res, err)
	}
	doc := bus.Mutable.AnswerDocumentV2()
	if doc == nil || len(doc.Blocks[1].EdgeAnchors) != 1 || doc.Blocks[1].EdgeAnchors[0].VisibleLabel != "model wording" {
		t.Fatalf("candidate permission must preserve the model-authored carrier: %+v", doc)
	}
}

func TestAnswerDiagramRelationRepairLease_ConsumesAfterMatchingPatchGeneration(t *testing.T) {
	mut := types.NewMutableState("relation repair generation")
	base := &types.AnswerDocumentV2{DocumentModel: "v2", Blocks: []types.AnswerBlock{{
		ID: "flow", Kind: types.BlockDiagram,
		Diagram:     &types.AnswerDiagramBlock{Kind: types.DiagramSequence, Language: "mermaid", Body: "sequenceDiagram\n A->>B: call"},
		EdgeAnchors: []types.DiagramEdgeAnchor{{FromNode: "A", ToNode: "B", RelationKind: types.DiagramRelCall}},
	}}}
	lease := types.NewAnswerDiagramRelationRepairLease(base, []types.AnswerDiagramRelationRepairFailure{{
		BlockID: "flow", Issue: "call_edge_unproven", FromNode: "A", ToNode: "B",
	}}, nil)
	mut.SetAnswerDiagramRelationRepairLease(lease)

	matching := &types.AnswerDocumentV2{DocumentModel: "v2", Blocks: []types.AnswerBlock{{
		ID: "flow", Kind: types.BlockDiagram,
		Diagram: &types.AnswerDiagramBlock{Kind: types.DiagramSequence, Language: "mermaid", Body: "sequenceDiagram\n participant A\n participant B"},
	}}}
	gotLease, violations := validateAndConsumeAnswerDiagramRelationRepairLease(mut, matching)
	if gotLease == nil || len(violations) != 0 {
		t.Fatalf("matching local repair must consume its generation lease: lease=%+v violations=%+v", gotLease, violations)
	}
	if got := mut.AnswerDiagramRelationRepairLease(); got != nil {
		t.Fatalf("satisfied relation generation must not constrain a later independent contract: %+v", got)
	}

	mut.SetAnswerDiagramRelationRepairLease(lease)
	escaped := &types.AnswerDocumentV2{DocumentModel: "v2", Blocks: []types.AnswerBlock{{ID: "flow", Kind: types.BlockTable}}}
	_, violations = validateAndConsumeAnswerDiagramRelationRepairLease(mut, escaped)
	if len(violations) == 0 {
		t.Fatal("carrier escape must remain rejected")
	}
	if got := mut.AnswerDiagramRelationRepairLease(); got == nil {
		t.Fatal("failed local repair must retain its lease for the next retry")
	}
}

func TestEmitAnswerDocumentPatch_SplitsFusedProseAndDiagramWithoutDroppingEither(t *testing.T) {
	bus := newPatchTestBusContext()
	raw := json.RawMessage(`{
		"unchanged_block_ids": ["list1"],
		"replace_blocks": [{
			"id": "s1",
			"kind": "summary",
			"text": "updated model conclusion",
			"diagram": {
				"kind": "flow",
				"language": "mermaid",
				"body": "flowchart TD\n  Request --> Result"
			}
		}]
	}`)
	res, err := (&EmitAnswerDocumentPatch{}).Execute(bus, raw)
	if err != nil {
		t.Fatalf("unexpected exec error: %v", err)
	}
	if !res.Success {
		t.Fatalf("typed fused patch recovery must succeed: %s", res.Summary)
	}
	doc := bus.Mutable.AnswerDocumentV2()
	if doc == nil || len(doc.Blocks) != 3 {
		t.Fatalf("expected replaced prose, unchanged list, and diagram half: %+v", doc)
	}
	if doc.Blocks[0].ID != "s1" || doc.Blocks[0].Kind != types.BlockSummary || doc.Blocks[0].Text != "updated model conclusion" || doc.Blocks[0].Diagram != nil {
		t.Fatalf("replaced model prose changed or retained diagram: %+v", doc.Blocks[0])
	}
	if doc.Blocks[2].Kind != types.BlockDiagram || doc.Blocks[2].Diagram == nil || doc.Blocks[2].Diagram.Body != "flowchart TD\n  Request --> Result" {
		t.Fatalf("split diagram half missing from merged patch: %+v", doc.Blocks)
	}
}

func TestEmitAnswerDocumentPatch_EmptyOptionalDiagramObjectOnProseBlockIsAbsent(t *testing.T) {
	bus := newPatchTestBusContext()
	raw := json.RawMessage(`{
		"unchanged_block_ids": ["list1"],
		"replace_blocks": [{
			"id": "s1",
			"kind": "summary",
			"text": "updated model conclusion",
			"diagram": {}
		}]
	}`)
	res, err := (&EmitAnswerDocumentPatch{}).Execute(bus, raw)
	if err != nil {
		t.Fatalf("unexpected exec error: %v", err)
	}
	if !res.Success {
		t.Fatalf("empty optional object must not invalidate a prose patch: %s", res.Summary)
	}
	doc := bus.Mutable.AnswerDocumentV2()
	if doc == nil || len(doc.Blocks) != 2 || doc.Blocks[0].ID != "s1" ||
		doc.Blocks[0].Kind != types.BlockSummary || doc.Blocks[0].Text != "updated model conclusion" ||
		doc.Blocks[0].Diagram != nil {
		t.Fatalf("patched prose block changed during empty-object canonicalization: %+v", doc)
	}
}

func TestEmitAnswerDocumentPatch_DropsUniqueNestedItemIDsFromUnchangedBlockList(t *testing.T) {
	bus := newPatchTestBusContext()
	tool := &EmitAnswerDocumentPatch{}
	res, err := tool.Execute(bus, json.RawMessage(`{
		"unchanged_block_ids":["s1","i1"],
		"replace_blocks":[{
			"id":"list1",
			"kind":"ordered_list",
			"text":"updated list",
			"claim_uses":[{"claim_form":"call_edge"}],
			"items":[{"id":"i1","label":"A","citation_ref":0}]
		}]
	}`))
	if err != nil {
		t.Fatalf("unexpected exec error: %v", err)
	}
	if !res.Success {
		t.Fatalf("a uniquely owned nested item id must not invalidate block-level unchanged ids: %s", res.Summary)
	}
	doc := bus.Mutable.AnswerDocumentV2()
	if doc == nil || len(doc.Blocks) != 2 || doc.Blocks[1].Text != "updated list" {
		t.Fatalf("replacement did not apply after nested item-id tolerance: %+v", doc)
	}
}

func TestEmitAnswerDocumentPatch_StillRejectsUnknownAndAmbiguousUnchangedIDs(t *testing.T) {
	for _, tc := range []struct {
		name string
		id   string
		prep func(*types.AnswerDocumentV2)
	}{
		{name: "unknown", id: "not-a-block-or-item"},
		{name: "ambiguous nested item", id: "shared", prep: func(doc *types.AnswerDocumentV2) {
			doc.Blocks[0].Items = []types.AnswerBlockItem{{ID: "shared"}}
			doc.Blocks[1].Items = append(doc.Blocks[1].Items, types.AnswerBlockItem{ID: "shared"})
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			bus := newPatchTestBusContext()
			if tc.prep != nil {
				doc := bus.Mutable.AnswerDocumentV2()
				tc.prep(doc)
				bus.Mutable.SetAnswerDocumentV2WithMutation(types.MutationReplaceAll, doc)
			}
			tool := &EmitAnswerDocumentPatch{}
			params, _ := json.Marshal(map[string]any{"unchanged_block_ids": []string{tc.id}})
			res, err := tool.Execute(bus, params)
			if err != nil {
				t.Fatalf("unexpected exec error: %v", err)
			}
			if res.Success || !strings.Contains(res.Summary, "not present in previous emit") {
				t.Fatalf("%q must remain a strict unknown block id: %+v", tc.id, res)
			}
		})
	}
}

func TestEmitAnswerDocumentPatch_AddBlockMissingIDStillRejects(t *testing.T) {
	bus := newPatchTestBusContext()
	tool := &EmitAnswerDocumentPatch{}
	res, err := tool.Execute(bus, json.RawMessage(`{"add_blocks":[{"kind":"summary","text":"extra"}]}`))
	if err != nil {
		t.Fatalf("unexpected exec error: %v", err)
	}
	if res.Success {
		t.Fatalf("patch add block without id must remain strict; got %+v", res)
	}
	if !strings.Contains(res.Summary, "id is required") {
		t.Fatalf("patch rejection should name id requirement, got %q", res.Summary)
	}
}

func TestEmitAnswerDocumentPatch_PreEmitSoftHintsStayAdvisory(t *testing.T) {
	bus := newPatchTestBusContext()
	bus.AnalysisIR = &types.AnalysisIR{
		RequestModel: types.RequestModel{
			RawRequest: "compare A and B",
			Intent:     types.IntentExplain,
			Buckets: []types.QuestionBucket{
				{Label: "A", Anchors: []string{"A"}, Index: 1},
				{Label: "B", Anchors: []string{"B"}, Index: 2},
			},
		},
	}
	tool := &EmitAnswerDocumentPatch{}
	res, err := tool.Execute(bus, json.RawMessage(`{"unchanged_block_ids":["s1","list1"]}`))
	if err != nil {
		t.Fatalf("unexpected exec error: %v", err)
	}
	if !res.Success {
		t.Fatalf("patch path should split pre-emit soft hints into advisory instead of hard retrying: %s", res.Summary)
	}
}

func TestEmitAnswerDocumentPatch_QuarantinesUnknownSchemaMetadata(t *testing.T) {
	bus := newPatchTestBusContext()
	tool := &EmitAnswerDocumentPatch{}
	res, err := tool.Execute(bus, json.RawMessage(`{
		"unchanged_block_ids": ["s1", "list1"],
		"add_blocks": [
			{"id": "scope_note", "kind": "caveat", "text": "Scope note.", "metadata": {"dropped": true}}
		],
		"claim_uses": [{"claim_form": "definition"}],
		"metadata": {"model": "local"}
	}`))
	if err != nil {
		t.Fatalf("unexpected exec error: %v", err)
	}
	if !res.Success {
		t.Fatalf("unknown patch metadata should be quarantined instead of forcing retry: %+v", res)
	}
	doc := bus.Mutable.AnswerDocumentV2()
	if doc == nil || len(doc.Blocks) != 3 || doc.Blocks[2].ID != "scope_note" {
		t.Fatalf("merged document missing quarantined add block: %+v", doc)
	}
}

func TestEmitAnswerDocumentPatch_RejectsStructurallyContaminatedFieldName(t *testing.T) {
	bus := newPatchTestBusContext()
	before := bus.Mutable.AnswerDocumentV2()
	tool := &EmitAnswerDocumentPatch{}
	res, err := tool.Execute(bus, json.RawMessage(`{
		"unchanged_block_ids": ["s1", "list1"],
		"add_blocks": [{
			"id":"c1", "kind":"caveat", "text":"recoverable",
			"\"}, {\"claim_uses": [{"claim_form":"external_observation"}]
		}]
	}`))
	if err != nil {
		t.Fatalf("unexpected exec error: %v", err)
	}
	if res.Success || res.Repair == nil || res.Repair.Code != types.ToolRepairCodeAnswerDocStructuralCarrierCorruption {
		t.Fatalf("structurally contaminated patch should request an exact retry: %+v", res)
	}
	after := bus.Mutable.AnswerDocumentV2()
	if after == nil || before == nil || len(after.Blocks) != len(before.Blocks) {
		t.Fatalf("rejected patch must leave previous accepted document intact: before=%+v after=%+v", before, after)
	}
}

func TestEmitAnswerDocumentPatch_VerifiesNonEmptyAppendedCitationQuote(t *testing.T) {
	repo := t.TempDir()
	if err := os.WriteFile(filepath.Join(repo, "source.go"), []byte("package sample\nfunc exact() {}\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	bus := newPatchTestBusContext()
	bus.RepoRoot = repo
	tool := &EmitAnswerDocumentPatch{}
	res, err := tool.Execute(bus, json.RawMessage(`{
		"unchanged_block_ids": ["list1"],
		"replace_blocks": [{
			"id":"s1", "kind":"summary", "text":"fixed lead",
			"items":[{"id":"lead-cite", "citation_ref":1}]
		}],
		"append_citations": [{"file":"source.go", "line":2, "quote":"func wrong() {}"}]
	}`))
	if err != nil {
		t.Fatalf("unexpected exec error: %v", err)
	}
	if !res.Success {
		t.Fatalf("patch with repairable non-empty quote should succeed: %+v", res)
	}
	doc := bus.Mutable.AnswerDocumentV2()
	if doc == nil || len(doc.Citations) != 2 {
		t.Fatalf("unexpected merged citation pool: %+v", doc)
	}
	if got := doc.Citations[1].Quote; got != "func exact() {}" {
		t.Fatalf("patch citation retained stale model quote: %q", got)
	}
}

func TestEmitAnswerDocumentPatch_DropsAppendedCitationWhoseExactSourceRowIsBlank(t *testing.T) {
	repo := t.TempDir()
	if err := os.WriteFile(filepath.Join(repo, "source.py"), []byte("import os\n\nimport _fastlex\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	bus := newPatchTestBusContext()
	bus.RepoRoot = repo
	res, err := (&EmitAnswerDocumentPatch{}).Execute(bus, json.RawMessage(`{
		"unchanged_block_ids": ["list1"],
		"replace_blocks": [{
			"id":"s1", "kind":"summary", "text":"model-authored import explanation",
			"items":[{"id":"lead-cite", "citation_ref":1}]
		}],
		"append_citations": [{"file":"source.py", "line":2, "quote":"import _fastlex"}]
	}`))
	if err != nil {
		t.Fatalf("unexpected exec error: %v", err)
	}
	if !res.Success {
		t.Fatalf("blank-row citation is a mechanically removable carrier, not a reason to lose the model answer: %+v", res)
	}
	doc := bus.Mutable.AnswerDocumentV2()
	if doc == nil || len(doc.Citations) != 1 {
		t.Fatalf("forged blank-row citation survived patch persist: %+v", doc)
	}
	if len(doc.Blocks) == 0 || len(doc.Blocks[0].Items) != 1 || doc.Blocks[0].Items[0].CitationRef != -1 {
		t.Fatalf("item reference to blank source row was not detached: %+v", doc)
	}
}

func TestEmitAnswerDocumentPatch_RepairsSingleUnknownItemTextAlias(t *testing.T) {
	bus := newPatchTestBusContext()
	tool := &EmitAnswerDocumentPatch{}
	res, err := tool.Execute(bus, json.RawMessage(`{
		"unchanged_block_ids": ["s1"],
		"replace_blocks": [
			{"id": "list1", "kind": "ordered_list", "items": [
				{"id": "i1", "label": "333707a", "tool": "标记 deterministic command measurements 的稳定化与计量路径", "citation_ref": -1}
			]}
		]
	}`))
	if err != nil {
		t.Fatalf("unexpected exec error: %v", err)
	}
	if !res.Success {
		t.Fatalf("single unknown item text alias should be repaired on patch path: %+v", res)
	}
	doc := bus.Mutable.AnswerDocumentV2()
	if doc == nil || len(doc.Blocks) != 2 || len(doc.Blocks[1].Items) != 1 {
		t.Fatalf("merged document missing repaired list block: %+v", doc)
	}
	if got := doc.Blocks[1].Items[0].Text; !strings.Contains(got, "deterministic command") {
		t.Fatalf("unknown item text alias was not preserved as visible text: %q", got)
	}
}

// TestEmitAnswerDocumentPatch_PureUnchangedAppliesAndPreserves is
// the **R16 load-bearing tool-level test**. LLM declares "all
// blocks unchanged"; tool clones them verbatim; result has every
// typed annotation field preserved.
func TestEmitAnswerDocumentPatch_PureUnchangedAppliesAndPreserves(t *testing.T) {
	bus := newPatchTestBusContext()
	tool := &EmitAnswerDocumentPatch{}
	params := json.RawMessage(`{"unchanged_block_ids":["s1","list1"]}`)
	res, err := tool.Execute(bus, params)
	if err != nil {
		t.Fatalf("Execute err: %v", err)
	}
	if !res.Success {
		t.Fatalf("apply must succeed; got %+v", res)
	}
	doc := bus.Mutable.AnswerDocumentV2()
	if doc == nil {
		t.Fatal("merged doc not written to Mutable")
	}
	if len(doc.Blocks) != 2 {
		t.Errorf("block count = %d, want 2", len(doc.Blocks))
	}
	// Block-level claim_use preservation
	if len(doc.Blocks[0].ClaimUses) == 0 {
		t.Error("s1 ClaimUses dropped by patch (load-bearing R16 invariant)")
	}
	if doc.Blocks[0].FacetIDs == nil || doc.Blocks[0].FacetIDs[0] != "current_code_path" {
		t.Errorf("s1 FacetIDs lost; got %+v", doc.Blocks[0].FacetIDs)
	}
	// list1 block-level claim_uses preservation
	if len(doc.Blocks[1].ClaimUses) != 1 || doc.Blocks[1].ClaimUses[0].ClaimForm != types.ClaimCallEdge {
		t.Errorf("list1 ClaimUses lost on unchanged block; got %+v", doc.Blocks[1].ClaimUses)
	}
}

// TestEmitAnswerDocumentPatch_ReplaceBlock pins the typical retry
// flow: replace one block, leave others untouched.
func TestEmitAnswerDocumentPatch_ReplaceBlock(t *testing.T) {
	bus := newPatchTestBusContext()
	tool := &EmitAnswerDocumentPatch{}
	params := json.RawMessage(`{
		"unchanged_block_ids": ["list1"],
		"replace_blocks": [{
			"id": "s1",
			"kind": "summary",
			"surface_role": "principal",
			"text": "fixed summary",
			"facet_ids": ["current_code_path"],
			"claim_uses": [{"claim_form": "definition_fact", "evidence_id": "ev1"}]
		}]
	}`)
	res, err := tool.Execute(bus, params)
	if err != nil {
		t.Fatalf("Execute err: %v", err)
	}
	if !res.Success {
		t.Fatalf("must succeed; got %+v", res)
	}
	doc := bus.Mutable.AnswerDocumentV2()
	if doc.Blocks[0].Text != "fixed summary" {
		t.Errorf("s1 not replaced; got Text=%q", doc.Blocks[0].Text)
	}
	if doc.Blocks[1].ID != "list1" {
		t.Errorf("list1 not preserved at original position")
	}
}

func TestEmitAnswerDocumentPatch_InheritsMissingKindForExactReplacementID(t *testing.T) {
	bus := newPatchTestBusContext()
	tool := &EmitAnswerDocumentPatch{}
	params := json.RawMessage(`{
		"replace_blocks":[{
			"id":"s1",
			"text":"replacement summary"
		}]
	}`)
	res, _ := tool.Execute(bus, params)
	if !res.Success {
		t.Fatalf("exact replacement should inherit the previous block kind: %s", res.Summary)
	}
	doc := bus.Mutable.AnswerDocumentV2()
	if doc == nil || len(doc.Blocks) == 0 || doc.Blocks[0].Kind != types.BlockSummary || doc.Blocks[0].Text != "replacement summary" {
		t.Fatalf("replacement kind/content mismatch: %+v", doc)
	}
}

func TestEmitAnswerDocumentPatch_InheritsOmittedCarrierMetadataForStableReplacement(t *testing.T) {
	bus := &types.BusContext{Mutable: &types.MutableState{}}
	bus.Mutable.SetAnswerDocumentV2WithMutation(types.MutationReplaceAll, &types.AnswerDocumentV2{
		DocumentModel: "v2",
		Blocks: []types.AnswerBlock{{
			ID: "path", Kind: types.BlockOrderedList,
			SurfaceRole: types.SurfacePrincipal,
			FacetIDs:    []string{"current_code_path"},
			Items:       []types.AnswerBlockItem{{ID: "hop-1", Text: "old"}},
		}},
	})
	params := json.RawMessage(`{
		"replace_blocks":[{
			"id":"path",
			"kind":"ordered_list",
			"items":[{"id":"hop-1","text":"repaired"}]
		}]
	}`)
	res, _ := (&EmitAnswerDocumentPatch{}).Execute(bus, params)
	if !res.Success {
		t.Fatalf("stable replacement should inherit omitted carrier metadata: %s", res.Summary)
	}
	doc := bus.Mutable.AnswerDocumentV2()
	if doc == nil || len(doc.Blocks) != 1 {
		t.Fatalf("missing patched document: %+v", doc)
	}
	got := doc.Blocks[0]
	if got.SurfaceRole != types.SurfacePrincipal || len(got.FacetIDs) != 1 ||
		got.FacetIDs[0] != "current_code_path" {
		t.Fatalf("omitted stable carrier metadata was lost: %+v", got)
	}
	if len(got.Items) != 1 || got.Items[0].Text != "repaired" {
		t.Fatalf("replacement content must remain model-authored: %+v", got.Items)
	}
}

func TestNormalizeSparsePatchRelationMetadataEdits_PreservesVisiblePrincipalCarrier(t *testing.T) {
	prev := &types.AnswerDocumentV2{Blocks: []types.AnswerBlock{
		{
			ID: "path", Kind: types.BlockOrderedList, Title: "完整调用链",
			SurfaceRole: types.SurfacePrincipal,
			FacetIDs:    []string{string(types.FacetCurrentCodePath), string(types.FacetPrincipalPathEdge)},
			ClaimUses:   []types.RenderedClaimUse{{ClaimForm: types.ClaimCallEdge, FacetID: string(types.FacetPrincipalPathEdge)}},
			Items: []types.AnswerBlockItem{{
				ID: "hop-1", Label: "FastTokenizer.tokenize", Text: "model-authored visible hop", CitationRef: 3,
			}},
		},
	}}
	patch := &types.AnswerDocumentV2Patch{ReplaceBlocks: []types.AnswerBlock{{
		ID: "path", Kind: types.BlockOrderedList,
		EdgeAnchors: []types.DiagramEdgeAnchor{{
			FromNode: "FastTokenizer.tokenize", ToNode: "_fastlex.tokenize_bytes",
			FromIdentity: "FastTokenizer.tokenize", ToIdentity: "_fastlex.tokenize_bytes",
			RelationKind: types.DiagramRelCall, VisibleLabel: "调用原生分词入口",
		}},
	}}}
	raw := json.RawMessage(`{"replace_blocks":[{"id":"path","edge_anchors":[{"from_node":"FastTokenizer.tokenize","to_node":"_fastlex.tokenize_bytes","from_identity":"FastTokenizer.tokenize","to_identity":"_fastlex.tokenize_bytes","relation_kind":"call","visible_label":"调用原生分词入口"}]}]}`)
	changed, fields := normalizeSparsePatchRelationMetadataEdits(prev, raw, patch)
	if !changed || len(fields) != 1 {
		t.Fatalf("sparse typed relation edit was not absorbed: changed=%t fields=%v", changed, fields)
	}
	got := patch.ReplaceBlocks[0]
	if got.Kind != types.BlockOrderedList || got.Title != "完整调用链" || got.SurfaceRole != types.SurfacePrincipal ||
		len(got.FacetIDs) != 2 || len(got.ClaimUses) != 1 || len(got.Items) != 1 ||
		got.Items[0].Text != "model-authored visible hop" || got.Items[0].CitationRef != 3 {
		t.Fatalf("previous model-authored carrier content was not preserved: %+v", got)
	}
	if len(got.EdgeAnchors) != 1 || got.EdgeAnchors[0].VisibleLabel != "调用原生分词入口" {
		t.Fatalf("model-authored typed relation delta was not retained: %+v", got.EdgeAnchors)
	}
	doc := &types.AnswerDocumentV2{Blocks: []types.AnswerBlock{got}}
	if removed := normalizeOrphanDiagramEdgeAnchors(doc, &types.AnswerSemanticView{Family: types.QFCallChain}); removed != 0 {
		t.Fatalf("standalone carrier was reclassified as orphan after sparse repair: removed=%d doc=%+v", removed, doc)
	}
}

func TestNormalizeSparsePatchRelationMetadataEdits_DoesNotMergeVisibleContentOrDeletion(t *testing.T) {
	prev := &types.AnswerDocumentV2{Blocks: []types.AnswerBlock{{
		ID: "path", Kind: types.BlockOrderedList, SurfaceRole: types.SurfacePrincipal,
		ClaimUses: []types.RenderedClaimUse{{ClaimForm: types.ClaimCallEdge}},
		Items:     []types.AnswerBlockItem{{ID: "hop-1", Text: "old"}},
	}}}
	for _, tc := range []struct {
		name string
		raw  json.RawMessage
		blk  types.AnswerBlock
	}{
		{
			name: "visible content edit keeps full replacement semantics",
			raw:  json.RawMessage(`{"replace_blocks":[{"id":"path","text":"new","edge_anchors":[{"from_node":"A","to_node":"B","relation_kind":"call"}]}]}`),
			blk: types.AnswerBlock{ID: "path", Kind: types.BlockOrderedList, Text: "new", EdgeAnchors: []types.DiagramEdgeAnchor{{
				FromNode: "A", ToNode: "B", RelationKind: types.DiagramRelCall,
			}}},
		},
		{
			name: "explicit relation deletion keeps full replacement semantics",
			raw:  json.RawMessage(`{"replace_blocks":[{"id":"path","edge_anchors":[]}]}`),
			blk:  types.AnswerBlock{ID: "path", Kind: types.BlockOrderedList},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			patch := &types.AnswerDocumentV2Patch{ReplaceBlocks: []types.AnswerBlock{tc.blk}}
			changed, fields := normalizeSparsePatchRelationMetadataEdits(prev, tc.raw, patch)
			if changed || len(fields) != 0 {
				t.Fatalf("non-annotation-only replacement was merged: changed=%t fields=%v block=%+v", changed, fields, patch.ReplaceBlocks[0])
			}
		})
	}
}

func TestEmitAnswerDocumentPatchProductionWiresSparseRelationMetadataEdit(t *testing.T) {
	bus := newPatchTestBusContext()
	prev := bus.Mutable.AnswerDocumentV2()
	prev.Blocks[1].SurfaceRole = types.SurfacePrincipal
	prev.Blocks[1].FacetIDs = []string{string(types.FacetPrincipalPathEdge)}
	bus.Mutable.SetAnswerDocumentV2WithMutation(types.MutationReplaceAll, prev)
	params := json.RawMessage(`{
		"unchanged_block_ids":["s1"],
		"replace_blocks":[{
			"id":"list1",
			"edge_anchors":[{
				"from_node":"入口",
				"to_node":"工作线程",
				"visible_label":"调用",
				"from_identity":"A",
				"to_identity":"B",
				"relation_kind":"call"
			}]
		}]
	}`)
	res, err := (&EmitAnswerDocumentPatch{}).Execute(bus, params)
	if err != nil || !res.Success {
		t.Fatalf("production sparse relation metadata patch failed: result=%+v err=%v", res, err)
	}
	doc := bus.Mutable.AnswerDocumentV2()
	if doc == nil || len(doc.Blocks) != 2 {
		t.Fatalf("production patch lost document: %+v", doc)
	}
	got := doc.Blocks[1]
	if got.Kind != types.BlockOrderedList || got.SurfaceRole != types.SurfacePrincipal ||
		len(got.Items) != 1 || got.Items[0].Label != "A" || len(got.ClaimUses) != 1 ||
		len(got.EdgeAnchors) != 1 || got.EdgeAnchors[0].VisibleLabel != "调用" {
		t.Fatalf("production path did not preserve content plus relation delta: %+v", got)
	}
}

func TestEmitAnswerDocumentPatch_RejectsPrincipalPathFacetAfterRelationMetadataStripped(t *testing.T) {
	mut := types.NewMutableState("trace the call chain")
	mut.SetAnswerDocumentV2WithMutation(types.MutationReplaceAll, &types.AnswerDocumentV2{
		DocumentModel: "v2",
		Blocks: []types.AnswerBlock{
			{ID: "summary", Kind: types.BlockSummary, Text: "model-authored chain summary"},
			{
				ID: "path", Kind: types.BlockOrderedList,
				SurfaceRole: types.SurfacePrincipal,
				FacetIDs:    []string{string(types.FacetCurrentCodePath), string(types.FacetPrincipalPathEdge)},
				ClaimUses:   []types.RenderedClaimUse{{ClaimForm: types.ClaimCallEdge, FacetID: string(types.FacetPrincipalPathEdge)}},
				EdgeAnchors: []types.DiagramEdgeAnchor{{
					FromNode: "entry", ToNode: "worker",
					FromIdentity: "Entry.run", ToIdentity: "Worker.handle",
					RelationKind: types.DiagramRelCall,
				}},
				Items: []types.AnswerBlockItem{{ID: "hop-1", Text: "old model-authored hop"}},
			},
		},
	})
	bus := &types.BusContext{
		Mutable: mut,
		AnalysisIR: &types.AnalysisIR{RequestModel: types.RequestModel{
			Intent:        types.IntentTrace,
			PredicateAxis: types.AxisCall,
			AnalyzerHints: types.AnalyzerHints{Kind: string(types.ReqCallChain)},
		}},
	}
	res, _ := (&EmitAnswerDocumentPatch{}).Execute(bus, json.RawMessage(`{
		"unchanged_block_ids":["summary"],
		"replace_blocks":[{
			"id":"path",
			"kind":"ordered_list",
			"surface_role":"principal",
			"facet_ids":["current_code_path","principal_path_edge"],
			"items":[{"id":"hop-1","text":"repaired visible hop but relation metadata omitted"}]
		}]
	}`))
	if res.Success || res.Repair == nil {
		t.Fatalf("patch must not ship a principal path after stripping its typed relation owner: %+v", res)
	}
	if res.Repair.Code != "answer_doc_pre_emit_contract" ||
		!strings.Contains(res.Summary, `blocks[id="path"].claim_uses`) ||
		!strings.Contains(res.Summary, `blocks[id="path"].edge_anchors`) ||
		res.Repair.Metadata[types.ToolRepairMetaDiagramRelationFailureIssues] != diagramStandalonePrincipalPathMissingOwner {
		t.Fatalf("patch rejection lost its precise same-block repair contract: summary=%s repair=%+v", res.Summary, res.Repair)
	}
}

func TestInheritMissingPatchReplacementCarrierMetadataHonorsExplicitClear(t *testing.T) {
	prev := &types.AnswerDocumentV2{Blocks: []types.AnswerBlock{{
		ID: "path", Kind: types.BlockOrderedList,
		SurfaceRole: types.SurfacePrincipal,
		FacetIDs:    []string{"principal_path_edge"},
		Items:       []types.AnswerBlockItem{{ID: "hop-1"}},
	}}}
	blocks := []emitAnswerBlockV2{{
		ID: "path", Kind: string(types.BlockOrderedList),
		Items: []emitAnswerBlockItemV2{{ID: "hop-1"}},
	}}
	raw := json.RawMessage(`{"replace_blocks":[{"id":"path","kind":"ordered_list","facet_ids":[],"surface_role":"","items":[{"id":"hop-1"}]}]}`)
	changed, fields := inheritMissingPatchReplacementCarrierMetadata(prev, raw, blocks)
	if changed || len(fields) != 0 {
		t.Fatalf("explicit carrier values must remain model-owned: changed=%t fields=%v", changed, fields)
	}
	if blocks[0].FacetIDs != nil || blocks[0].SurfaceRole != "" {
		t.Fatalf("explicit clear/value was overwritten: %+v", blocks[0])
	}
}

func TestInheritMissingPatchReplacementCarrierMetadataRequiresStableItemOverlap(t *testing.T) {
	prev := &types.AnswerDocumentV2{Blocks: []types.AnswerBlock{{
		ID: "path", Kind: types.BlockOrderedList,
		SurfaceRole: types.SurfacePrincipal,
		FacetIDs:    []string{"principal_path_edge"},
		Items:       []types.AnswerBlockItem{{ID: "hop-1"}},
	}}}
	blocks := []emitAnswerBlockV2{{
		ID: "path", Kind: string(types.BlockOrderedList),
		Items: []emitAnswerBlockItemV2{{ID: "new-hop"}},
	}}
	raw := json.RawMessage(`{"replace_blocks":[{"id":"path","kind":"ordered_list","items":[{"id":"new-hop"}]}]}`)
	changed, fields := inheritMissingPatchReplacementCarrierMetadata(prev, raw, blocks)
	if changed || len(fields) != 0 || blocks[0].SurfaceRole != "" || blocks[0].FacetIDs != nil {
		t.Fatalf("wholesale content replacement must not inherit carriers: changed=%t fields=%v block=%+v", changed, fields, blocks[0])
	}
}

func TestEmitAnswerDocumentPatch_MissingKindDoesNotAuthorizeUnknownReplacementOrAdd(t *testing.T) {
	for _, params := range []json.RawMessage{
		json.RawMessage(`{"replace_blocks":[{"id":"unknown","text":"new"}]}`),
		json.RawMessage(`{"add_blocks":[{"id":"new","text":"new"}]}`),
	} {
		bus := newPatchTestBusContext()
		res, _ := (&EmitAnswerDocumentPatch{}).Execute(bus, params)
		if res.Success || !strings.Contains(res.Summary, `kind="" is not a valid block kind`) {
			t.Fatalf("missing kind outside exact replacement must remain rejected: success=%t summary=%s", res.Success, res.Summary)
		}
	}
}

func TestEmitAnswerDocumentPatch_PreservesTableTrailingProse(t *testing.T) {
	bus := &types.BusContext{Mutable: &types.MutableState{}}
	bus.Mutable.SetAnswerDocumentV2WithMutation(types.MutationReplaceAll, &types.AnswerDocumentV2{
		DocumentModel: "v2",
		Blocks: []types.AnswerBlock{{
			ID:       "metrics",
			Kind:     types.BlockTable,
			Text:     "| metric | value |\n|---|---|\n| running | 3.5ms |\n\nNext step: inspect rival-30 on same CPU for CPU pressure.",
			FacetIDs: []string{"observed_artifact_fact"},
			ClaimUses: []types.RenderedClaimUse{{
				ClaimForm: types.ClaimExternalObservation,
				FacetID:   "observed_artifact_fact",
			}},
		}},
	})
	tool := &EmitAnswerDocumentPatch{}
	res, err := tool.Execute(bus, json.RawMessage(`{
		"replace_blocks": [{
			"id": "metrics",
			"kind": "table",
			"columns": ["metric", "value"],
			"items": [{"id": "r1", "cells": ["running", "3.5ms"]}],
			"facet_ids": ["observed_artifact_fact"],
			"claim_uses": [{"claim_form": "external_observation", "facet_id": "observed_artifact_fact"}]
		}]
	}`))
	if err != nil {
		t.Fatalf("Execute err: %v", err)
	}
	if !res.Success {
		t.Fatalf("table-tail preservation patch must succeed: %+v", res)
	}
	doc := bus.Mutable.AnswerDocumentV2()
	if doc == nil || len(doc.Blocks) != 2 {
		t.Fatalf("merged doc=%+v, want replacement table plus preserved tail block", doc)
	}
	if doc.Blocks[0].ID != "metrics" || len(doc.Blocks[0].Items) != 1 || strings.TrimSpace(doc.Blocks[0].Text) != "" {
		t.Fatalf("replacement table not preserved structurally: %+v", doc.Blocks[0])
	}
	if doc.Blocks[1].Kind != types.BlockSection || !strings.Contains(doc.Blocks[1].Text, "Next step: inspect rival-30") {
		t.Fatalf("table trailing prose was not preserved: %+v", doc.Blocks[1])
	}
	if len(doc.Blocks[1].FacetIDs) == 0 || len(doc.Blocks[1].ClaimUses) == 0 {
		t.Fatalf("preserved tail block lost annotations: %+v", doc.Blocks[1])
	}
}

// TestEmitAnswerDocumentPatch_AddNewBlock pins add path through
// tool layer (kind validation + id uniqueness).
func TestEmitAnswerDocumentPatch_AddNewBlock(t *testing.T) {
	bus := newPatchTestBusContext()
	tool := &EmitAnswerDocumentPatch{}
	params := json.RawMessage(`{
		"add_blocks": [{
			"id": "diag1",
			"kind": "diagram",
			"diagram": {"kind": "flow", "language": "mermaid", "body": "flowchart TD\nA-->B"}
		}]
	}`)
	res, err := tool.Execute(bus, params)
	if err != nil {
		t.Fatalf("Execute err: %v", err)
	}
	if !res.Success {
		t.Fatalf("add must succeed; got %+v", res)
	}
	doc := bus.Mutable.AnswerDocumentV2()
	if len(doc.Blocks) != 3 {
		t.Errorf("block count after add = %d, want 3", len(doc.Blocks))
	}
	if doc.Blocks[2].ID != "diag1" {
		t.Error("added block at wrong position")
	}
}

func TestEmitAnswerDocumentPatch_NormalizesReplaceNewBlockToAdd(t *testing.T) {
	bus := newPatchTestBusContext()
	tool := &EmitAnswerDocumentPatch{}
	params := json.RawMessage(`{
		"replace_blocks": [{
			"id": "diag1",
			"kind": "diagram",
			"facet_ids": ["diagram_spine"],
			"diagram": {"kind": "flow", "language": "mermaid", "body": "flowchart TD\nA-->B"}
		}]
	}`)
	res, err := tool.Execute(bus, params)
	if err != nil {
		t.Fatalf("Execute err: %v", err)
	}
	if !res.Success {
		t.Fatalf("new block misfiled under replace_blocks should be normalized to add_blocks; got %+v", res)
	}
	doc := bus.Mutable.AnswerDocumentV2()
	if len(doc.Blocks) != 3 || doc.Blocks[2].ID != "diag1" {
		t.Fatalf("normalized block should append at tail, got %+v", doc.Blocks)
	}
}

func TestEmitAnswerDocumentPatch_RequiredTableCannotDriftToRecoveredAddSection(t *testing.T) {
	bus := newPatchTestBusContext()
	bus.AnalysisIR = &types.AnalysisIR{
		RequestModel: types.RequestModel{
			Intent:   types.IntentEnumerate,
			Scenario: types.ScenarioGeneric,
			Predicates: types.SemanticPredicates{
				HasPerMemberTable: true,
			},
		},
	}
	prev := bus.Mutable.AnswerDocumentV2()
	prev.Blocks[1] = types.AnswerBlock{
		ID:          "list1",
		Kind:        types.BlockTable,
		Columns:     []string{"member", "attribute"},
		SurfaceRole: types.SurfacePrincipal,
		FacetIDs:    []string{string(types.FacetEnumerationItem)},
		ClaimUses: []types.RenderedClaimUse{
			{ClaimForm: types.ClaimDefinitionFact, EvidenceID: "ev1"},
		},
		Items: []types.AnswerBlockItem{{
			ID: "i1", Label: "A", Cells: []string{"A", "value"}, CitationRef: 0,
		}},
	}
	bus.Mutable.SetAnswerDocumentV2WithMutation(types.MutationReplaceAll, prev)

	tool := &EmitAnswerDocumentPatch{}
	res, err := tool.Execute(bus, json.RawMessage(`{
		"remove_block_ids": ["list1"],
		"replace_blocks": [{
			"id": "new-section",
			"kind": "section",
			"text": "The same members restated as prose.",
			"facet_ids": ["enumeration_item"],
			"claim_uses": [{"claim_form": "definition_fact", "evidence_id": "ev1"}]
		}]
	}`))
	if err != nil {
		t.Fatalf("unexpected exec error: %v", err)
	}
	if res.Success {
		t.Fatalf("patch must not delete a typed sole-kind table and replace it with a recovered add section: %+v", res)
	}
	if !strings.Contains(res.Summary, "table") {
		t.Fatalf("rejection should name the missing typed table carrier: %q", res.Summary)
	}
	got := bus.Mutable.AnswerDocumentV2()
	if got == nil || len(got.Blocks) != 2 || got.Blocks[1].Kind != types.BlockTable {
		t.Fatalf("rejected patch must leave previous table document intact: %+v", got)
	}
}

func TestEmitAnswerDocumentPatch_NormalizesAddExistingBlockToReplace(t *testing.T) {
	bus := newPatchTestBusContext()
	tool := &EmitAnswerDocumentPatch{}
	params := json.RawMessage(`{
		"add_blocks": [{
			"id": "s1",
			"kind": "summary",
			"surface_role": "principal",
			"text": "fixed summary",
			"facet_ids": ["current_code_path"],
			"claim_uses": [{"claim_form": "definition_fact", "evidence_id": "ev1"}]
		}]
	}`)
	res, err := tool.Execute(bus, params)
	if err != nil {
		t.Fatalf("Execute err: %v", err)
	}
	if !res.Success {
		t.Fatalf("existing block misfiled under add_blocks should be normalized to replace_blocks; got %+v", res)
	}
	doc := bus.Mutable.AnswerDocumentV2()
	if len(doc.Blocks) != 2 || doc.Blocks[0].ID != "s1" || doc.Blocks[0].Text != "fixed summary" {
		t.Fatalf("normalized existing block should replace in place, got %+v", doc.Blocks)
	}
}

// B1097 production shape: a retry put an existing principal relation list in
// add_blocks and omitted surface_role. The add->replace recovery ran after the
// omission-inheritance pass, silently demoting the list to supporting and
// allowing its retained call_edge claim to escape the empty-anchor hard gate.
func TestEmitAnswerDocumentPatch_NormalizedAddExistingPreservesPrincipalRelationGate(t *testing.T) {
	bus := newPatchTestBusContext()
	prev := bus.Mutable.AnswerDocumentV2()
	prev.Blocks[1].SurfaceRole = types.SurfacePrincipal
	prev.Blocks[1].FacetIDs = []string{string(types.FacetCurrentCodePath)}
	bus.Mutable.SetAnswerDocumentV2WithMutation(types.MutationReplaceAll, prev)
	bus.AnalysisIR = &types.AnalysisIR{RequestModel: types.RequestModel{
		Intent:        types.IntentTrace,
		PredicateAxis: types.AxisCall,
		AnalyzerHints: types.AnalyzerHints{Kind: string(types.ReqCallChain)},
	}}

	res, err := (&EmitAnswerDocumentPatch{}).Execute(bus, json.RawMessage(`{
		"unchanged_block_ids":["s1"],
		"add_blocks":[{
			"id":"list1",
			"kind":"ordered_list",
			"claim_uses":[{"claim_form":"call_edge","facet_id":"current_code_path"}],
			"facet_ids":["current_code_path"],
			"items":[{"id":"i1","label":"A","text":"descriptive row","citation_ref":0}]
		}]
	}`))
	if err != nil {
		t.Fatalf("unexpected exec error: %v", err)
	}
	if res.Success || res.Repair == nil {
		t.Fatalf("normalized existing principal relation block must remain subject to the empty-anchor hard gate: %+v", res)
	}
	if !strings.Contains(res.Summary, `blocks[id="list1"].edge_anchors`) ||
		res.Repair.Metadata[types.ToolRepairMetaDiagramRelationFailureIssues] != diagramStandaloneRelationClaimHasNoAnchor {
		t.Fatalf("normalized retry lost the precise relation-owner repair: summary=%s repair=%+v", res.Summary, res.Repair)
	}
	got := bus.Mutable.AnswerDocumentV2()
	if got == nil || got.Blocks[1].SurfaceRole != types.SurfacePrincipal || len(got.Blocks[1].EdgeAnchors) != 0 {
		t.Fatalf("rejected normalized patch must leave the previous principal block intact: %+v", got)
	}
}

func TestEmitAnswerDocumentPatch_RejectedPatchStagesCandidateWithoutAdvancingAcceptedOrFullRejectedBase(t *testing.T) {
	if got := (&EmitAnswerDocumentPatch{}).Description(); !strings.Contains(got, "NONE of that patch's edits become the accepted answer") ||
		!strings.Contains(got, "retry-local staging base") || !strings.Contains(got, "never user-visible") {
		t.Fatalf("patch schema teaching must distinguish accepted state from retry staging: %s", got)
	}
	mut := types.NewMutableState("transactional rejected patch")
	base := &types.AnswerDocumentV2{
		DocumentModel: "v2",
		Citations:     []types.Citation{{File: "x.go", Line: 10}},
		Blocks: []types.AnswerBlock{
			{
				ID: "s1", Kind: types.BlockSummary,
				SurfaceRole: types.SurfacePrincipal,
				Text:        "stable rejected summary",
				FacetIDs:    []string{string(types.FacetCurrentCodePath)},
			},
			{
				ID: "path", Kind: types.BlockOrderedList,
				SurfaceRole: types.SurfacePrincipal,
				FacetIDs:    []string{string(types.FacetCurrentCodePath)},
				ClaimUses:   []types.RenderedClaimUse{{ClaimForm: types.ClaimCallEdge}},
				Items:       []types.AnswerBlockItem{{ID: "hop", Label: "stable hop", CitationRef: 0}},
			},
		},
	}
	mut.SetLastRejectedAnswerDocumentV2(base)
	bus := &types.BusContext{
		Mutable: mut,
		AnalysisIR: &types.AnalysisIR{RequestModel: types.RequestModel{
			Intent:        types.IntentTrace,
			PredicateAxis: types.AxisCall,
			AnalyzerHints: types.AnalyzerHints{Kind: string(types.ReqCallChain)},
		}},
	}

	res, err := (&EmitAnswerDocumentPatch{}).Execute(bus, json.RawMessage(`{
		"unchanged_block_ids":["s1"],
		"replace_blocks":[{
			"id":"path",
			"kind":"ordered_list",
			"surface_role":"principal",
			"facet_ids":["current_code_path"],
			"claim_uses":[{"claim_form":"call_edge"}],
			"items":[{"id":"hop","label":"changed hidden hop","citation_ref":0}]
		}]
	}`))
	if err != nil {
		t.Fatalf("unexpected execution error: %v", err)
	}
	if res.Success || !strings.Contains(res.Summary, `blocks[id="path"].edge_anchors`) {
		t.Fatalf("invalid relation patch must reach the ordinary hard gate: %+v", res)
	}
	if got := mut.AnswerDocumentV2(); got != nil {
		t.Fatalf("rejected-draft patch must not create an accepted document: %+v", got)
	}
	got := mut.LastRejectedAnswerDocumentV2()
	if got == nil || len(got.Blocks) != 2 || got.Blocks[1].Items[0].Label != "stable hop" {
		t.Fatalf("rejected patch changed the initial rejected-full base: %+v", got)
	}
	if got.Blocks[0].Text != "stable rejected summary" {
		t.Fatalf("rejected patch changed an unrelated base block: %+v", got.Blocks[0])
	}
	staged := mut.PendingAnswerDocumentPatchBase()
	if staged == nil || len(staged.Blocks) != 2 || staged.Blocks[1].Items[0].Label != "changed hidden hop" {
		t.Fatalf("merged rejected patch must remain as the exact retry-local staging base: %+v", staged)
	}
}

func TestEmitAnswerDocumentPatch_PrefersRetryLocalStagingBase(t *testing.T) {
	bus := newPatchTestBusContext()
	bus.Mutable.SetPendingAnswerDocumentPatchBase(&types.AnswerDocumentV2{
		DocumentModel: "v2",
		Blocks: []types.AnswerBlock{{
			ID: "staged", Kind: types.BlockSummary, SurfaceRole: types.SurfacePrincipal,
			Text: "model-authored staged summary",
		}},
	})

	res, err := (&EmitAnswerDocumentPatch{}).Execute(bus, json.RawMessage(`{
		"replace_blocks":[{
			"id":"staged","kind":"summary","surface_role":"principal",
			"text":"refined staged summary"
		}]
	}`))
	if err != nil || !res.Success {
		t.Fatalf("patch must refine the live staging base: res=%+v err=%v", res, err)
	}
	got := bus.Mutable.AnswerDocumentV2()
	if got == nil || len(got.Blocks) != 1 || got.Blocks[0].ID != "staged" || got.Blocks[0].Text != "refined staged summary" {
		t.Fatalf("accepted patch did not commit the staged generation: %+v", got)
	}
	if bus.Mutable.PendingAnswerDocumentPatchBase() != nil {
		t.Fatal("accepted patch must clear retry-local staging state")
	}
}

func TestEmitAnswerDocumentPatch_NormalizesRemoveThenAddSameExistingBlockToReplace(t *testing.T) {
	bus := newPatchTestBusContext()
	tool := &EmitAnswerDocumentPatch{}
	params := json.RawMessage(`{
		"remove_block_ids": ["list1"],
		"add_blocks": [{
			"id": "list1",
			"kind": "ordered_list",
			"claim_uses": [{"claim_form": "call_edge"}],
			"items": [{"id": "i1", "label": "A", "text": "rewritten", "citation_ref": 0}]
		}]
	}`)
	res, err := tool.Execute(bus, params)
	if err != nil {
		t.Fatalf("Execute err: %v", err)
	}
	if !res.Success {
		t.Fatalf("remove+add same existing block should normalize to replace_blocks; got %+v", res)
	}
	doc := bus.Mutable.AnswerDocumentV2()
	if len(doc.Blocks) != 2 || doc.Blocks[1].ID != "list1" {
		t.Fatalf("normalized same-id remove+add should replace in place, got %+v", doc.Blocks)
	}
	if got := doc.Blocks[1].Items[0].Text; got != "rewritten" {
		t.Fatalf("replacement payload not applied: %q", got)
	}
}

func TestEmitAnswerDocumentPatch_RemoveAlreadyAbsentBlockIsIdempotent(t *testing.T) {
	bus := newPatchTestBusContext()
	tool := &EmitAnswerDocumentPatch{}
	res, err := tool.Execute(bus, json.RawMessage(`{"remove_block_ids":["diagram-already-removed"]}`))
	if err != nil {
		t.Fatalf("Execute err: %v", err)
	}
	if !res.Success {
		t.Fatalf("already-satisfied removal should not consume another retry: %+v", res)
	}
	doc := bus.Mutable.AnswerDocumentV2()
	if doc == nil || len(doc.Blocks) != 2 {
		t.Fatalf("idempotent removal must preserve the current document: %+v", doc)
	}
}

func TestEmitAnswerDocumentPatch_NormalizesWhitespaceAndIdenticalDuplicateOps(t *testing.T) {
	bus := newPatchTestBusContext()
	tool := &EmitAnswerDocumentPatch{}
	params := json.RawMessage(`{
		"remove_block_ids": [" list1 ", "list1"],
		"replace_blocks": [
			{
				"id": " s1 ",
				"kind": "summary",
				"surface_role": "principal",
				"text": "fixed summary",
				"facet_ids": ["current_code_path"],
				"claim_uses": [{"claim_form": "definition_fact", "evidence_id": "ev1"}]
			},
			{
				"id": "s1",
				"kind": "summary",
				"surface_role": "principal",
				"text": "fixed summary",
				"facet_ids": ["current_code_path"],
				"claim_uses": [{"claim_form": "definition_fact", "evidence_id": "ev1"}]
			}
		],
		"add_blocks": [
			{"id": " extra ", "kind": "section", "text": "extra detail"},
			{"id": "extra", "kind": "section", "text": "extra detail"}
		]
	}`)
	res, err := tool.Execute(bus, params)
	if err != nil {
		t.Fatalf("Execute err: %v", err)
	}
	if !res.Success {
		t.Fatalf("whitespace/identical duplicate patch ops should be normalized transactionally; got %+v", res)
	}
	doc := bus.Mutable.AnswerDocumentV2()
	if doc == nil || len(doc.Blocks) != 2 {
		t.Fatalf("normalized patch should replace s1, remove list1, add extra; got %+v", doc)
	}
	if doc.Blocks[0].ID != "s1" || doc.Blocks[0].Text != "fixed summary" {
		t.Fatalf("replacement block was not normalized/applied: %+v", doc.Blocks[0])
	}
	if doc.Blocks[1].ID != "extra" || doc.Blocks[1].Text != "extra detail" {
		t.Fatalf("add block was not normalized/applied: %+v", doc.Blocks[1])
	}
}

func TestEmitAnswerDocumentPatch_ConflictingDuplicateBlocksStillReject(t *testing.T) {
	bus := newPatchTestBusContext()
	tool := &EmitAnswerDocumentPatch{}
	params := json.RawMessage(`{
		"replace_blocks": [
			{"id": "s1", "kind": "summary", "text": "first"},
			{"id": " s1 ", "kind": "summary", "text": "second"}
		]
	}`)
	res, err := tool.Execute(bus, params)
	if err != nil {
		t.Fatalf("Execute err: %v", err)
	}
	if res.Success {
		t.Fatal("same-id blocks with different payloads must still reject; runtime must not choose between model-authored variants")
	}
	if !strings.Contains(res.Summary, `replace_blocks["s1"] duplicated`) {
		t.Fatalf("reject should preserve the precise duplicate-id diagnostic, got %q", res.Summary)
	}
}

// TestEmitAnswerDocumentPatch_RecoverFromRetryState confirms the
// fallback path: when AnswerDocumentV2 was cleared (e.g. by
// ResetForFallback), the tool falls back to RetryState.PrevEmitJSON.
func TestEmitAnswerDocumentPatch_RecoverFromRetryState(t *testing.T) {
	bus := &types.BusContext{Mutable: &types.MutableState{}}
	prevDoc := &types.AnswerDocumentV2{
		DocumentModel: "v2",
		Blocks: []types.AnswerBlock{
			{ID: "s1", Kind: types.BlockSummary, Text: "from retry state"},
		},
	}
	prevJSON, _ := json.Marshal(prevDoc)
	bus.Mutable.SetRetryState(&types.RetryState{
		Attempt:      1,
		PrevEmitJSON: prevJSON,
	})
	// AnswerDocumentV2 NOT set on Mutable — must fall back.
	tool := &EmitAnswerDocumentPatch{}
	params := json.RawMessage(`{"unchanged_block_ids":["s1"]}`)
	res, _ := tool.Execute(bus, params)
	if !res.Success {
		t.Fatalf("must recover from RetryState.PrevEmitJSON; got %+v", res)
	}
	doc := bus.Mutable.AnswerDocumentV2()
	if doc == nil || len(doc.Blocks) != 1 || doc.Blocks[0].ID != "s1" {
		t.Errorf("recovery + apply broken; got %+v", doc)
	}
}

func TestEmitAnswerDocumentPatch_UsesRejectedDraftAsBase(t *testing.T) {
	bus := &types.BusContext{Mutable: &types.MutableState{}}
	bus.Mutable.SetLastRejectedAnswerDocumentV2(&types.AnswerDocumentV2{
		DocumentModel: "v2",
		Citations:     []types.Citation{{File: "x.go", Line: 10}},
		Blocks: []types.AnswerBlock{
			{ID: "s1", Kind: types.BlockSummary, Text: "previous rejected summary"},
			{
				ID:    "list1",
				Kind:  types.BlockOrderedList,
				Items: []types.AnswerBlockItem{{ID: "i1", Label: "A", CitationRef: 0}},
			},
		},
	})
	tool := &EmitAnswerDocumentPatch{}
	params := json.RawMessage(`{
		"unchanged_block_ids": ["s1", "list1"],
		"add_blocks": [{"id":"scope_caveat","kind":"caveat","text":"Scope note"}]
	}`)
	res, _ := tool.Execute(bus, params)
	if !res.Success {
		t.Fatalf("patch should apply against structurally valid rejected draft; got %+v", res)
	}
	doc := bus.Mutable.AnswerDocumentV2()
	if doc == nil || len(doc.Blocks) != 3 {
		t.Fatalf("merged document not persisted from rejected-draft patch base: %+v", doc)
	}
	if doc.Blocks[2].Kind != types.BlockCaveat || doc.Blocks[0].Text != "previous rejected summary" {
		t.Fatalf("patch did not preserve base blocks and append caveat: %+v", doc.Blocks)
	}
}

// TestEmitAnswerDocumentPatch_RejectsInvalidKind covers tool-layer
// kind validation (mirrors the V2 emit gate).
func TestEmitAnswerDocumentPatch_RejectsInvalidKind(t *testing.T) {
	bus := newPatchTestBusContext()
	tool := &EmitAnswerDocumentPatch{}
	params := json.RawMessage(`{"add_blocks":[{"id":"new1","kind":"bogus_kind"}]}`)
	res, _ := tool.Execute(bus, params)
	if res.Success {
		t.Error("invalid kind must reject at tool layer")
	}
}

// TestEmitAnswerDocumentPatch_DiagramRequiresPayload covers the
// diagram-kind sanity check at tool layer.
func TestEmitAnswerDocumentPatch_DiagramRequiresPayload(t *testing.T) {
	bus := newPatchTestBusContext()
	tool := &EmitAnswerDocumentPatch{}
	// kind=diagram with no diagram payload
	params := json.RawMessage(`{"add_blocks":[{"id":"d1","kind":"diagram"}]}`)
	res, _ := tool.Execute(bus, params)
	if res.Success {
		t.Error("kind=diagram without payload must reject")
	}
}

// TestEmitAnswerDocumentPatch_ToolMetadata pins Name + Description
// presence + Schema validity.
func TestEmitAnswerDocumentPatch_ToolMetadata(t *testing.T) {
	tool := &EmitAnswerDocumentPatch{}
	if tool.Name() != "emit_answer_document_patch" {
		t.Errorf("Name = %q", tool.Name())
	}
	desc := tool.Description()
	for _, want := range []string{
		"DELTA",
		"unchanged_block_ids",
		"replace_blocks",
		"add_blocks",
		"remove_block_ids",
		"diagram_edge_edits",
		"diagram_boundary_replacements",
		"diagram_participant_edits",
	} {
		if !strings.Contains(desc, want) {
			t.Errorf("Description missing %q", want)
		}
	}
	var schemaObj map[string]interface{}
	if err := json.Unmarshal(tool.Parameters(), &schemaObj); err != nil {
		t.Errorf("Schema not valid JSON: %v", err)
	}
}

// TestEmitAnswerDocumentPatch_StringWrappedAddBlocks confirms the
// flat-mode tolerance fix-up: when the LLM stringifies the
// `add_blocks` array (same MiniMax streaming bug that hits the full
// emit's `blocks` field), the patch tool now silently re-parses
// instead of forcing an LLM retry.
func TestEmitAnswerDocumentPatch_StringWrappedAddBlocks(t *testing.T) {
	bus := &types.BusContext{Mutable: types.NewMutableState("")}
	// Seed a previous emit so the patch has something to apply against.
	prev := &types.AnswerDocumentV2{
		Blocks: []types.AnswerBlock{
			{ID: "s1", Kind: types.BlockSummary, Text: "lead"},
		},
	}
	bus.Mutable.SetAnswerDocumentV2WithMutation(types.MutationReplaceAll, prev)

	// Stringified add_blocks — exact MiniMax bug shape.
	params := json.RawMessage(`{"add_blocks": "[{\"id\":\"sec_extra\",\"kind\":\"section\",\"title\":\"More\",\"text\":\"detail\",\"surface_role\":\"principal\",\"claim_uses\":[{\"claim_form\":\"definition_fact\"}]}]","unchanged_block_ids":["s1"]}`)
	tool := &EmitAnswerDocumentPatch{}
	res, _ := tool.Execute(bus, params)
	if !res.Success {
		t.Fatalf("patch tool must auto-repair stringified add_blocks; got Success=false: %s", res.Summary)
	}
	doc := bus.Mutable.AnswerDocumentV2()
	if doc == nil {
		t.Fatal("repaired patch did not write merged doc")
	}
	if len(doc.Blocks) != 2 {
		t.Errorf("expected 2 blocks after patch (s1 unchanged + sec_extra added); got %d", len(doc.Blocks))
	}
}

// TestEmitAnswerDocumentPatch_StringWrappedReplaceBlocks confirms
// the same fix-up for replace_blocks (the more common patch field).
func TestEmitAnswerDocumentPatch_StringWrappedReplaceBlocks(t *testing.T) {
	bus := &types.BusContext{Mutable: types.NewMutableState("")}
	prev := &types.AnswerDocumentV2{
		Blocks: []types.AnswerBlock{
			{ID: "s1", Kind: types.BlockSummary, Text: "old summary"},
		},
	}
	bus.Mutable.SetAnswerDocumentV2WithMutation(types.MutationReplaceAll, prev)

	params := json.RawMessage(`{"replace_blocks": "[{\"id\":\"s1\",\"kind\":\"summary\",\"text\":\"new summary\"}]"}`)
	tool := &EmitAnswerDocumentPatch{}
	res, _ := tool.Execute(bus, params)
	if !res.Success {
		t.Fatalf("patch tool must auto-repair stringified replace_blocks; got Success=false: %s", res.Summary)
	}
	doc := bus.Mutable.AnswerDocumentV2()
	if doc == nil || len(doc.Blocks) == 0 || doc.Blocks[0].Text != "new summary" {
		t.Errorf("repaired replace_blocks did not apply; got %+v", doc)
	}
}

func TestEmitAnswerDocumentPatch_DuplicateReplaceBlockArraysStaySeparate(t *testing.T) {
	bus := &types.BusContext{Mutable: types.NewMutableState("")}
	bus.Mutable.SetAnswerDocumentV2WithMutation(types.MutationReplaceAll, &types.AnswerDocumentV2{
		Blocks: []types.AnswerBlock{
			{ID: "summary", Kind: types.BlockSummary, Text: "lead"},
			{ID: "hop-list", Kind: types.BlockOrderedList, Items: []types.AnswerBlockItem{{ID: "old", Label: "old", Text: "old"}}},
			{ID: "sequence-diagram", Kind: types.BlockDiagram, Diagram: &types.AnswerDiagramBlock{
				Kind: types.DiagramSequence, Language: "mermaid", Body: "sequenceDiagram\n  A->>B: old",
			}},
		},
	})

	// Production witness r160: one model tool call emitted replace_blocks
	// twice. Both values are valid schema arrays and must be concatenated;
	// last-key-wins would lose the diagram repair, while element-wise object
	// fusion would manufacture a second split diagram.
	params := json.RawMessage(`{
		"unchanged_block_ids":["summary"],
		"replace_blocks":[{
			"id":"sequence-diagram",
			"kind":"diagram",
			"diagram":{"kind":"sequence","language":"mermaid","body":"sequenceDiagram\n  A->>B: fixed"}
		}],
		"replace_blocks":[{
			"id":"hop-list",
			"kind":"ordered_list",
			"items":[{"id":"new","label":"new","text":"fixed"}]
		}]
	}`)
	res, err := (&EmitAnswerDocumentPatch{}).Execute(bus, params)
	if err != nil {
		t.Fatalf("Execute err: %v", err)
	}
	if !res.Success {
		t.Fatalf("duplicate array batches should repair losslessly: %s", res.Summary)
	}
	doc := bus.Mutable.AnswerDocumentV2()
	if doc == nil || len(doc.Blocks) != 3 {
		t.Fatalf("patch must remain count-neutral: %+v", doc)
	}
	diagrams := 0
	for _, block := range doc.Blocks {
		switch block.ID {
		case "hop-list":
			if len(block.Items) != 1 || block.Items[0].ID != "new" {
				t.Fatalf("second replace_blocks batch was lost: %+v", block)
			}
		case "sequence-diagram":
			diagrams++
			if block.Diagram == nil || !strings.Contains(block.Diagram.Body, "fixed") {
				t.Fatalf("first replace_blocks batch was lost: %+v", block)
			}
		}
	}
	if diagrams != 1 {
		t.Fatalf("duplicate array repair manufactured %d diagram blocks: %+v", diagrams, doc.Blocks)
	}
}

func TestEmitAnswerDocumentPatch_StringWrappedAddBlocksMissingOuterBlockClose(t *testing.T) {
	bus := &types.BusContext{Mutable: types.NewMutableState("")}
	prev := &types.AnswerDocumentV2{
		Blocks: []types.AnswerBlock{
			{ID: "s1", Kind: types.BlockSummary, Text: "lead"},
		},
	}
	bus.Mutable.SetAnswerDocumentV2WithMutation(types.MutationReplaceAll, prev)

	params := json.RawMessage(`{"unchanged_block_ids":["s1"],"add_blocks":"[{\"id\":\"d1\",\"kind\":\"diagram\",\"diagram\":{\"kind\":\"flow\",\"language\":\"mermaid\",\"body\":\"flowchart TD\\n    A --> B\"}, {\"id\":\"c1\",\"kind\":\"caveat\",\"text\":\"keep this caveat\"}]"}`)
	tool := &EmitAnswerDocumentPatch{}
	res, _ := tool.Execute(bus, params)
	if !res.Success {
		t.Fatalf("patch tool must auto-repair stringified add_blocks missing outer block close; got: %s", res.Summary)
	}
	doc := bus.Mutable.AnswerDocumentV2()
	if doc == nil || len(doc.Blocks) != 3 {
		t.Fatalf("repaired patch did not preserve every block: %+v", doc)
	}
	if doc.Blocks[1].Kind != types.BlockDiagram || doc.Blocks[1].Diagram == nil {
		t.Fatalf("diagram block not preserved: %+v", doc.Blocks[1])
	}
	if doc.Blocks[2].Kind != types.BlockCaveat || !strings.Contains(doc.Blocks[2].Text, "keep this caveat") {
		t.Fatalf("caveat block not preserved: %+v", doc.Blocks[2])
	}
}

func TestEmitAnswerDocumentPatch_PrunesScalarItemFragmentsWhenTextPreservesDisplay(t *testing.T) {
	bus := &types.BusContext{Mutable: types.NewMutableState("")}
	bus.Mutable.SetAnswerDocumentV2WithMutation(types.MutationReplaceAll, &types.AnswerDocumentV2{
		Citations: []types.Citation{{File: "a.go", Line: 1}},
		Blocks: []types.AnswerBlock{
			{ID: "members", Kind: types.BlockTable, Text: "| 文件 | 角色 |\n|---|---|\n| a.go:1 | A |"},
		},
	})
	params := json.RawMessage(`{
		"replace_blocks": [{
			"id": "members",
			"kind": "table",
			"columns": ["文件", "角色"],
			"text": "| 文件 | 角色 |\n|---|---|\n| a.go:1 | A |\n| a.go:1 | B |",
			"items": [
				{"id":"r1", "label":"A", "citation_ref":0},
				"citation_ref:8",
				"{\"id\":\"r2\",\"label\":\"B\",\"citation_ref\":0}"
			]
		}]
	}`)
	tool := &EmitAnswerDocumentPatch{}
	res, _ := tool.Execute(bus, params)
	if !res.Success {
		t.Fatalf("patch should tolerate item scalar fragments when block.text preserves display: %s", res.Summary)
	}
	doc := bus.Mutable.AnswerDocumentV2()
	if doc == nil || len(doc.Blocks) != 1 {
		t.Fatalf("missing patched doc: %+v", doc)
	}
	if got := len(doc.Blocks[0].Items); got != 2 {
		t.Fatalf("expected two typed item objects after pruning scalar fragment; got %d (%+v)", got, doc.Blocks[0].Items)
	}
	if doc.Blocks[0].Items[1].ID != "r2" {
		t.Fatalf("stringified item object should be preserved as typed item, got %+v", doc.Blocks[0].Items[1])
	}
}

func TestEmitAnswerDocumentPatch_StringWrappedReplaceCitationsWithExtraCloser(t *testing.T) {
	bus := &types.BusContext{Mutable: types.NewMutableState("")}
	bus.Mutable.SetAnswerDocumentV2WithMutation(types.MutationReplaceAll, &types.AnswerDocumentV2{
		Blocks: []types.AnswerBlock{
			{ID: "s1", Kind: types.BlockSummary, Text: "lead"},
		},
		Citations: []types.Citation{{File: "old.go", Line: 1}},
	})
	params := json.RawMessage(`{
		"unchanged_block_ids": ["s1"],
		"replace_citations": "[{\"file\":\"internal/agent/sub_explorer.go\",\"line\":31}]]"
	}`)
	tool := &EmitAnswerDocumentPatch{}
	res, _ := tool.Execute(bus, params)
	if !res.Success {
		t.Fatalf("patch tool must auto-repair overclosed stringified replace_citations; got Success=false: %s", res.Summary)
	}
	doc := bus.Mutable.AnswerDocumentV2()
	if doc == nil || len(doc.Citations) != 1 || doc.Citations[0].File != "internal/agent/sub_explorer.go" || doc.Citations[0].Line != 31 {
		t.Fatalf("repaired replace_citations did not apply; got %+v", doc)
	}
}

func TestEmitAnswerDocumentPatch_NormalizesReplaceCitationsForPreservedBlocks(t *testing.T) {
	bus := newPatchTestBusContext()
	params := json.RawMessage(`{
		"unchanged_block_ids": ["list1"],
		"replace_blocks": [{
			"id": "s1",
			"kind": "summary",
			"text": "fixed lead",
			"items": [{"id":"lead_cite", "citation_ref": 1}]
		}],
		"replace_citations": [
			{"file":"x.go", "line":10},
			{"file":"z.go", "line":99}
		]
	}`)
	tool := &EmitAnswerDocumentPatch{}
	res, _ := tool.Execute(bus, params)
	if !res.Success {
		t.Fatalf("replace_citations with preserved citation-bearing blocks should normalize, got: %s", res.Summary)
	}
	doc := bus.Mutable.AnswerDocumentV2()
	if doc == nil || len(doc.Citations) != 2 {
		t.Fatalf("normalized citation pool = %+v, want inherited old + appended new", doc)
	}
	if doc.Citations[0].File != "x.go" || doc.Citations[1].File != "z.go" {
		t.Fatalf("unexpected citation pool after normalization: %+v", doc.Citations)
	}
	if got := doc.Blocks[0].Items[0].CitationRef; got != 1 {
		t.Fatalf("replacement block citation_ref should be remapped to appended citation index 1, got %d", got)
	}
	if got := doc.Blocks[1].Items[0].CitationRef; got != 0 {
		t.Fatalf("preserved block citation_ref should still point at inherited pool index 0, got %d", got)
	}
}

func TestEmitAnswerDocumentPatch_MergesAppendCitationsIntoReplaceCitations(t *testing.T) {
	bus := &types.BusContext{Mutable: types.NewMutableState("")}
	bus.Mutable.SetAnswerDocumentV2WithMutation(types.MutationReplaceAll, &types.AnswerDocumentV2{
		Blocks:    []types.AnswerBlock{{ID: "s1", Kind: types.BlockSummary, Text: "lead"}},
		Citations: []types.Citation{{File: "old.go", Line: 1}},
	})
	params := json.RawMessage(`{
		"unchanged_block_ids": ["s1"],
		"replace_citations": [{"file":"a.go", "line":1}],
		"append_citations": [{"file":"b.go", "line":2}]
	}`)
	tool := &EmitAnswerDocumentPatch{}
	res, _ := tool.Execute(bus, params)
	if !res.Success {
		t.Fatalf("replace_citations + append_citations should merge instead of hard-retrying, got: %s", res.Summary)
	}
	doc := bus.Mutable.AnswerDocumentV2()
	if doc == nil || len(doc.Citations) != 2 {
		t.Fatalf("merged citation pool = %+v, want 2 citations", doc)
	}
	if doc.Citations[0].File != "a.go" || doc.Citations[1].File != "b.go" {
		t.Fatalf("unexpected merged citation pool: %+v", doc.Citations)
	}
}

func TestNormalizeAnswerDocumentPatchCitationOps_DeduplicatesInheritedAppendPoolAndRemaps(t *testing.T) {
	prev := &types.AnswerDocumentV2{Citations: []types.Citation{{File: "a.go", Line: 10, Quote: "richer quote"}}}
	patch := &types.AnswerDocumentV2Patch{
		AppendCitations: []types.Citation{
			{File: "a.go", Line: 10, Quote: "short quote"},
			{File: "b.go", Line: 20},
			{File: "b.go", Line: 20, Quote: "duplicate coordinate"},
		},
		ReplaceBlocks: []types.AnswerBlock{{
			ID: "table", Kind: types.BlockTable,
			Items: []types.AnswerBlockItem{
				{ID: "old", Label: "A", CitationRef: 1},
				{ID: "new", Label: "B", CitationRef: 2},
				{ID: "dup", Label: "B2", CitationRef: 3},
			},
		}},
	}
	changed, fields := normalizeAnswerDocumentPatchCitationOps(prev, patch)
	if !changed || len(fields) == 0 {
		t.Fatalf("duplicate append citations were not normalized: changed=%v fields=%v", changed, fields)
	}
	if len(patch.AppendCitations) != 1 || patch.AppendCitations[0].File != "b.go" {
		t.Fatalf("append pool=%+v, want one new coordinate", patch.AppendCitations)
	}
	refs := []int{
		patch.ReplaceBlocks[0].Items[0].CitationRef,
		patch.ReplaceBlocks[0].Items[1].CitationRef,
		patch.ReplaceBlocks[0].Items[2].CitationRef,
	}
	if !reflect.DeepEqual(refs, []int{0, 1, 1}) {
		t.Fatalf("citation refs=%v, want inherited/new/new remap", refs)
	}
}

func TestNormalizeAnswerDocumentPatchCitationRefs_MixedCarrierExactRowIDOutranksLabel(t *testing.T) {
	ctx, _, classID := sourceInventoryDuplicateCartRowIDTestContext(t)
	prev := &types.AnswerDocumentV2{Citations: []types.Citation{
		{File: "cart/Cart.cj", Line: 30},
		{File: "cart/Cart.cj", Line: 14},
	}}
	patch := &types.AnswerDocumentV2Patch{ReplaceBlocks: []types.AnswerBlock{{
		ID: "mixed", Kind: types.BlockTable, SurfaceRole: types.SurfacePrincipal,
		FacetIDs: []string{string(types.FacetEnumerationItem)},
		Items: []types.AnswerBlockItem{{
			ID: "cart", Label: "Cart", SourceInventoryRowID: classID, CitationRef: 0,
		}},
	}}}
	changed, fields := normalizeAnswerDocumentPatchCitationRefs(prev, patch, ctx)
	if !changed || len(fields) == 0 {
		t.Fatalf("exact row id did not drive patch citation binding: changed=%v fields=%v", changed, fields)
	}
	if got := patch.ReplaceBlocks[0].Items[0].CitationRef; got != 1 {
		t.Fatalf("mixed carrier exact row id bound citation_ref=%d, want class row index 1", got)
	}
}

func TestEmitAnswerDocumentPatch_MergesAppendCitationsBeforePreservedPoolNormalization(t *testing.T) {
	bus := newPatchTestBusContext()
	params := json.RawMessage(`{
		"unchanged_block_ids": ["list1"],
		"replace_blocks": [{
			"id": "s1",
			"kind": "summary",
			"text": "fixed lead",
			"items": [{"id":"lead_cite", "citation_ref": 2}]
		}],
		"replace_citations": [
			{"file":"x.go", "line":10},
			{"file":"z.go", "line":99}
		],
		"append_citations": [
			{"file":"w.go", "line":5}
		]
	}`)
	tool := &EmitAnswerDocumentPatch{}
	res, _ := tool.Execute(bus, params)
	if !res.Success {
		t.Fatalf("merged citation ops should still normalize preserved citation-bearing blocks, got: %s", res.Summary)
	}
	doc := bus.Mutable.AnswerDocumentV2()
	if doc == nil || len(doc.Citations) != 3 {
		t.Fatalf("normalized merged citation pool = %+v, want inherited old + two appended citations", doc)
	}
	if doc.Citations[0].File != "x.go" || doc.Citations[1].File != "z.go" || doc.Citations[2].File != "w.go" {
		t.Fatalf("unexpected citation pool after merged normalization: %+v", doc.Citations)
	}
	if got := doc.Blocks[0].Items[0].CitationRef; got != 2 {
		t.Fatalf("replacement block citation_ref should be remapped through merged pool to index 2, got %d", got)
	}
	if got := doc.Blocks[1].Items[0].CitationRef; got != 0 {
		t.Fatalf("preserved block citation_ref should still point at inherited pool index 0, got %d", got)
	}
}

func TestPreservePatchReplacementStableItemCitationRefsRepairsRowOrdinalDrift(t *testing.T) {
	prev := &types.AnswerDocumentV2{
		Citations: []types.Citation{
			{File: "a.go", Line: 10},
			{File: "a.go", Line: 20},
			{File: "a.go", Line: 30},
		},
		Blocks: []types.AnswerBlock{{
			ID:   "chain",
			Kind: types.BlockOrderedList,
			Items: []types.AnswerBlockItem{
				{ID: "h1", Label: "A -> B", Text: "first edge", CitationRef: 0},
				{ID: "h2", Label: "A -> C", Text: "second edge", CitationRef: 1},
				{ID: "h3", Label: "C -> D", Text: "third edge", CitationRef: 2},
			},
		}},
	}
	patch := &types.AnswerDocumentV2Patch{
		AppendCitations: []types.Citation{
			{File: "a.go", Line: 1},
			{File: "a.go", Line: 40},
		},
		ReplaceBlocks: []types.AnswerBlock{{
			ID:   "chain",
			Kind: types.BlockOrderedList,
			Items: []types.AnswerBlockItem{
				{ID: "source", Label: "A", Text: "definition", CitationRef: 3},
				{ID: "h1", Label: "A -> B", Text: "first edge", CitationRef: 1},
				{ID: "h2", Label: "A -> C", Text: "second edge", CitationRef: 2},
				{ID: "sink", Label: "D", Text: "definition", CitationRef: 4},
				{ID: "h3", Label: "C -> D", Text: "third edge", CitationRef: 4},
			},
		}},
	}
	changed, fields := preservePatchReplacementStableItemCitationRefs(prev, patch)
	if !changed || len(fields) != 3 {
		t.Fatalf("row-ordinal citation drift should repair all stable rows, changed=%v fields=%v", changed, fields)
	}
	got := patch.ReplaceBlocks[0].Items
	if got[1].CitationRef != 0 || got[2].CitationRef != 1 || got[4].CitationRef != 2 {
		t.Fatalf("stable item citation refs were not restored: %+v", got)
	}
	if got[0].CitationRef != 3 || got[3].CitationRef != 4 {
		t.Fatalf("new item citation refs must stay model-owned: %+v", got)
	}
}

func TestPreservePatchReplacementStableItemCitationRefsDoesNotOverrideIntentionalCitationEdit(t *testing.T) {
	prev := &types.AnswerDocumentV2{
		Citations: []types.Citation{{File: "a.go", Line: 10}, {File: "a.go", Line: 20}},
		Blocks: []types.AnswerBlock{{
			ID:    "list",
			Kind:  types.BlockOrderedList,
			Items: []types.AnswerBlockItem{{ID: "i1", Label: "A", Text: "same", CitationRef: 0}},
		}},
	}
	patch := &types.AnswerDocumentV2Patch{ReplaceBlocks: []types.AnswerBlock{{
		ID:    "list",
		Kind:  types.BlockOrderedList,
		Items: []types.AnswerBlockItem{{ID: "i1", Label: "A", Text: "same", CitationRef: 1}},
	}}}
	changed, fields := preservePatchReplacementStableItemCitationRefs(prev, patch)
	if changed || len(fields) != 0 || patch.ReplaceBlocks[0].Items[0].CitationRef != 1 {
		t.Fatalf("same-position explicit citation edit must remain model-owned: changed=%v fields=%v patch=%+v", changed, fields, patch)
	}
}

func TestEmitAnswerDocumentPatchProductionPreservesStableItemCitationAcrossInsertedRow(t *testing.T) {
	bus := newPatchTestBusContext()
	params := json.RawMessage(`{
		"unchanged_block_ids": ["s1"],
		"replace_blocks": [{
			"id": "list1",
			"kind": "ordered_list",
			"claim_uses": [{"claim_form":"call_edge"}],
			"items": [
				{"id":"new-definition","label":"New","text":"new endpoint","citation_ref":1},
				{"id":"i1","label":"A","citation_ref":1}
			]
		}],
		"append_citations": [{"file":"new.go","line":20}]
	}`)
	res, err := (&EmitAnswerDocumentPatch{}).Execute(bus, params)
	if err != nil || !res.Success {
		t.Fatalf("production patch route rejected inserted-row repair: result=%+v err=%v", res, err)
	}
	doc := bus.Mutable.AnswerDocumentV2()
	if doc == nil || len(doc.Blocks) != 2 || len(doc.Blocks[1].Items) != 2 {
		t.Fatalf("production patch route lost merged block/items: %+v", doc)
	}
	if got := doc.Blocks[1].Items[1].CitationRef; got != 0 {
		t.Fatalf("stable item citation_ref drift survived production patch route: got=%d doc=%+v", got, doc)
	}
	if got := doc.Blocks[1].Items[0].CitationRef; got != 1 {
		t.Fatalf("new item citation_ref should stay on appended citation: got=%d doc=%+v", got, doc)
	}
}

func TestEmitAnswerDocumentPatch_BindsExactEvidenceIDsBeforePoolRangeGate(t *testing.T) {
	bus := &types.BusContext{
		Mutable: types.NewMutableState("页面分配时序图"),
		AnalysisIR: &types.AnalysisIR{
			RequestModel: types.RequestModel{Intent: types.IntentTrace},
		},
	}
	bus.Mutable.SetTurnAArtifacts(types.TurnAArtifacts{EvidenceItems: []types.EvidenceItem{
		{
			ID:              "slowpath-definition",
			Kind:            types.EvidenceDirect,
			Source:          "mm/page_alloc.c",
			LineStart:       4687,
			AnchorKind:      types.AnchorDefinition,
			Subject:         "__alloc_pages_slowpath",
			AnchorSymbol:    "__alloc_pages_slowpath",
			Summary:         "慢速路径主入口。",
			GroundingStatus: types.GroundingGrounded,
		},
		{
			ID:              "slowpath-retry",
			Kind:            types.EvidenceDirect,
			Source:          "mm/page_alloc.c",
			LineStart:       4782,
			AnchorKind:      types.AnchorCall,
			Subject:         "__alloc_pages_slowpath",
			Object:          "get_page_from_freelist",
			Summary:         "__alloc_pages_slowpath calls get_page_from_freelist.",
			GroundingStatus: types.GroundingGrounded,
		},
		{
			ID:              "fastpath-call",
			Kind:            types.EvidenceDirect,
			Source:          "mm/page_alloc.c",
			LineStart:       5226,
			AnchorKind:      types.AnchorCall,
			Object:          "get_page_from_freelist",
			Summary:         "首次分配尝试调用快速路径。",
			SurfaceTerms:    []string{"快速路径"},
			GroundingStatus: types.GroundingGrounded,
		},
	}})
	bus.Mutable.SetAnswerDocumentV2WithMutation(types.MutationReplaceAll, &types.AnswerDocumentV2{
		DocumentModel: "v2",
		Citations: []types.Citation{
			{File: "mm/page_alloc.c", Line: 4687},
			{File: "mm/page_alloc.c", Line: 4782},
			{File: "mm/page_alloc.c", Line: 5226},
		},
		Blocks: []types.AnswerBlock{
			{
				ID:          "s1",
				Kind:        types.BlockSummary,
				SurfaceRole: types.SurfacePrincipal,
				Text:        "页面分配先走快速路径，失败后进入慢速路径。",
				FacetIDs:    []string{"current_code_path"},
				ClaimUses:   []types.RenderedClaimUse{{ClaimForm: types.ClaimCallEdge, FacetID: "current_code_path"}},
			},
			{
				ID:          "hops",
				Kind:        types.BlockOrderedList,
				SurfaceRole: types.SurfacePrincipal,
				FacetIDs:    []string{"current_code_path", "principal_path_edge"},
				ClaimUses:   []types.RenderedClaimUse{{ClaimForm: types.ClaimCallEdge, FacetID: "principal_path_edge"}},
				EdgeAnchors: []types.DiagramEdgeAnchor{{
					FromNode: "slowpath", ToNode: "freelist",
					FromIdentity: "__alloc_pages_slowpath", ToIdentity: "get_page_from_freelist",
					RelationKind: types.DiagramRelCall, VisibleLabel: "慢速路径调用快速分配尝试",
				}},
				Items: []types.AnswerBlockItem{
					{ID: "fast", Label: "get_page_from_freelist (快速路径)", Text: "快速路径核心函数。", CitationRef: 2},
					{ID: "slow", Label: "__alloc_pages_slowpath (慢速路径)", Text: "慢速路径主入口。", CitationRef: 0},
				},
			},
			{
				ID:   "caveat1",
				Kind: types.BlockCaveat,
				Text: "仅基于当前已收集的 mm/page_alloc.c 证据说明快慢路径。",
			},
		},
	})
	params := json.RawMessage(`{
		"unchanged_block_ids": ["s1", "caveat1"],
		"replace_blocks": [{
			"id": "hops",
			"kind": "ordered_list",
			"surface_role": "principal",
			"facet_ids": ["current_code_path", "principal_path_edge"],
			"claim_uses": [{"claim_form": "call_edge", "facet_id": "principal_path_edge"}],
			"edge_anchors": [{"from_node":"slowpath", "to_node":"freelist", "visible_label":"慢速路径调用快速分配尝试", "from_identity":"__alloc_pages_slowpath", "to_identity":"get_page_from_freelist", "relation_kind":"call"}],
			"items": [
				{"id":"fast", "label":"get_page_from_freelist (快速路径)", "text":"快速路径核心函数。", "evidence_ids":["fastpath-call"]},
				{"id":"slow", "label":"__alloc_pages_slowpath (慢速路径)", "text":"慢速路径主入口。", "evidence_ids":["slowpath-definition"]},
				{"id":"retry", "label":"get_page_from_freelist (慢速路径重试)", "text":"慢速路径重新尝试 get_page_from_freelist。", "evidence_ids":["slowpath-retry"]}
			]
		}]
	}`)
	tool := &EmitAnswerDocumentPatch{}
	res, _ := tool.Execute(bus, params)
	if !res.Success {
		t.Fatalf("patch path should bind exact evidence IDs before citation-pool range validation, got: %s", res.Summary)
	}
	doc := bus.Mutable.AnswerDocumentV2()
	if doc == nil {
		t.Fatal("patch path produced nil doc")
	}
	var hops *types.AnswerBlock
	for i := range doc.Blocks {
		if doc.Blocks[i].ID == "hops" {
			hops = &doc.Blocks[i]
			break
		}
	}
	if hops == nil || len(hops.Items) != 3 {
		t.Fatalf("missing normalized hops block: %+v", doc)
	}
	if got := hops.Items[0].CitationRef; got != 2 {
		t.Fatalf("fast-path evidence identity should bind existing citation index 2, got %d", got)
	}
	if got := hops.Items[2].CitationRef; got != 1 {
		t.Fatalf("retry evidence identity should bind the slowpath call site, got %d", got)
	}
}

func TestEmitAnswerDocumentPatch_RebindsExplicitSourceLocationsAgainstInheritedCitationPool(t *testing.T) {
	bus := &types.BusContext{Mutable: types.NewMutableState("enumerate source inventory classes")}
	bus.Mutable.SetTurnAArtifacts(types.TurnAArtifacts{EvidenceItems: []types.EvidenceItem{
		{
			Kind:            types.EvidenceDirect,
			Source:          "eval/fixtures/testdata/cangjie_minimal/main.cj",
			LineStart:       11,
			AnchorKind:      types.AnchorDefinition,
			Subject:         "App",
			AnchorSymbol:    "App",
			GroundingStatus: types.GroundingGrounded,
		},
		{
			Kind:            types.EvidenceDirect,
			Source:          "internal/thirdparty/tree-sitter-cangjie/corpus/sources/02_class_init_methods.cj",
			LineStart:       6,
			AnchorKind:      types.AnchorDefinition,
			Subject:         "Greeter",
			AnchorSymbol:    "Greeter",
			GroundingStatus: types.GroundingGrounded,
		},
	}})
	bus.Mutable.SetAnswerDocumentV2WithMutation(types.MutationReplaceAll, &types.AnswerDocumentV2{
		DocumentModel: "v2",
		Citations: []types.Citation{
			{File: "old/a.go", Line: 1},
			{File: "old/b.go", Line: 2},
			{File: "old/c.go", Line: 3},
			{File: "old/d.go", Line: 4},
			{File: "old/e.go", Line: 5},
			{File: "eval/fixtures/ts-monorepo-ws/packages/cli/src/main.ts", Line: 4},
			{File: "cmd/root.go", Line: 583},
		},
		Blocks: []types.AnswerBlock{
			{ID: "s1", Kind: types.BlockSummary, Text: "lead"},
			{ID: "l_class", Kind: types.BlockOrderedList, Items: []types.AnswerBlockItem{
				{ID: "c1", Label: "App", Text: "stale", CitationRef: 5},
				{ID: "c2", Label: "Greeter", Text: "stale", CitationRef: 6},
			}},
		},
	})
	params := json.RawMessage(`{
		"unchanged_block_ids": ["s1"],
		"replace_blocks": [{
			"id": "l_class",
			"kind": "ordered_list",
			"items": [
				{"id":"c1", "label":"App", "text":"文件路径 eval/fixtures/testdata/cangjie_minimal/main.cj:11，包路径 demo.app。", "citation_ref":5},
				{"id":"c2", "label":"Greeter", "text":"文件路径 internal/thirdparty/tree-sitter-cangjie/corpus/sources/02_class_init_methods.cj:6，包路径 demo.greeter。", "citation_ref":6}
			]
		}]
	}`)
	tool := &EmitAnswerDocumentPatch{}
	res, _ := tool.Execute(bus, params)
	if !res.Success {
		t.Fatalf("explicit source-location evidence should rebind inherited stale citation refs, got: %s", res.Summary)
	}
	doc := bus.Mutable.AnswerDocumentV2()
	if doc == nil || len(doc.Citations) != 9 {
		t.Fatalf("expected two typed citations appended to stale inherited pool, got %+v", doc)
	}
	var classBlock *types.AnswerBlock
	for i := range doc.Blocks {
		if doc.Blocks[i].ID == "l_class" {
			classBlock = &doc.Blocks[i]
			break
		}
	}
	if classBlock == nil || len(classBlock.Items) != 2 {
		t.Fatalf("missing replaced class block: %+v", doc)
	}
	if got := doc.Citations[classBlock.Items[0].CitationRef]; got.File != "eval/fixtures/testdata/cangjie_minimal/main.cj" || got.Line != 11 {
		t.Fatalf("App citation not rebound to typed explicit location: ref=%d cit=%+v", classBlock.Items[0].CitationRef, got)
	}
	if got := doc.Citations[classBlock.Items[1].CitationRef]; got.File != "internal/thirdparty/tree-sitter-cangjie/corpus/sources/02_class_init_methods.cj" || got.Line != 6 {
		t.Fatalf("Greeter citation not rebound to typed explicit location: ref=%d cit=%+v", classBlock.Items[1].CitationRef, got)
	}
}

func TestEmitAnswerDocumentPatch_RebindsPatchAppendCitationWhenInheritedPoolIsLonger(t *testing.T) {
	bus := &types.BusContext{Mutable: types.NewMutableState("enumerate source inventory classes")}
	bus.Mutable.SetTurnAArtifacts(types.TurnAArtifacts{EvidenceItems: []types.EvidenceItem{{
		Kind:            types.EvidenceDirect,
		Source:          "eval/fixtures/testdata/cangjie_minimal/cart/Cart.cj",
		LineStart:       14,
		AnchorKind:      types.AnchorDefinition,
		Subject:         "Cart",
		AnchorSymbol:    "Cart",
		GroundingStatus: types.GroundingGrounded,
	}}})
	prevCitations := make([]types.Citation, 0, 12)
	for i := 1; i <= 12; i++ {
		prevCitations = append(prevCitations, types.Citation{File: "stale.go", Line: i})
	}
	bus.Mutable.SetAnswerDocumentV2WithMutation(types.MutationReplaceAll, &types.AnswerDocumentV2{
		DocumentModel: "v2",
		Citations:     prevCitations,
		Blocks: []types.AnswerBlock{{
			ID:   "classes",
			Kind: types.BlockOrderedList,
			Items: []types.AnswerBlockItem{{
				ID:          "class-cart",
				Label:       "Cart",
				Text:        "public class 声明，包路径 demo.cart",
				CitationRef: 11,
			}},
		}},
	})
	params := json.RawMessage(`{
		"replace_blocks": [{
			"id": "classes",
			"kind": "ordered_list",
			"items": [{
				"id":"class-cart",
				"label":"Cart",
				"text":"public class 声明，包路径 demo.cart",
				"citation_ref": 12
			}]
		}],
		"append_citations": [{"file":"eval/fixtures/testdata/cangjie_minimal/cart/Cart.cj", "line":14}]
	}`)
	tool := &EmitAnswerDocumentPatch{}
	res, _ := tool.Execute(bus, params)
	if !res.Success {
		t.Fatalf("patch append citation should rebind item refs against the inherited pool, got: %s", res.Summary)
	}
	doc := bus.Mutable.AnswerDocumentV2()
	if doc == nil || len(doc.Citations) != 13 {
		t.Fatalf("expected appended Cart citation after stale inherited pool, got %+v", doc)
	}
	if got := doc.Blocks[0].Items[0].CitationRef; got != 12 {
		t.Fatalf("class-cart should point to appended citation index 12, got %d", got)
	}
	if got := doc.Citations[12]; got.File != "eval/fixtures/testdata/cangjie_minimal/cart/Cart.cj" || got.Line != 14 {
		t.Fatalf("unexpected appended citation: %+v", got)
	}
}

func TestEmitAnswerDocumentPatch_AppendCitationStaysAlignedAcrossPathCanonicalization(t *testing.T) {
	bus := &types.BusContext{
		Mutable: types.NewMutableState("enumerate source inventory classes"),
		AnalysisIR: &types.AnalysisIR{RequestModel: types.RequestModel{
			Intent: types.IntentEnumerate,
		}},
	}
	bus.Mutable.SetTurnAArtifacts(types.TurnAArtifacts{EvidenceItems: []types.EvidenceItem{{
		ID:              "native-add-definition",
		Kind:            types.EvidenceDirect,
		Source:          "eval/fixtures/testdata/cangjie_minimal/bridge/Bridge.cj",
		LineStart:       6,
		Scope:           types.ScopeLine,
		AnchorKind:      types.AnchorDefinition,
		Subject:         "native_add",
		AnchorSymbol:    "native_add",
		Snippet:         "foreign func native_add(a: Int64, b: Int64): Int64",
		GroundingStatus: types.GroundingGrounded,
	}}})
	prevCitations := []types.Citation{
		{File: "eval/fixtures/testdata/cangjie_minimal/bridge/Bridge.cj", Line: 4, Quote: "package demo.bridge"},
		{File: "old/a.go", Line: 1},
		{File: "old/b.go", Line: 2},
		{File: "old/c.go", Line: 3},
		{File: "old/d.go", Line: 4},
		{File: "old/e.go", Line: 5},
		{File: "old/f.go", Line: 6},
		{File: "old/g.go", Line: 7},
	}
	bus.Mutable.SetAnswerDocumentV2WithMutation(types.MutationReplaceAll, &types.AnswerDocumentV2{
		DocumentModel: "v2",
		Citations:     prevCitations,
		Blocks: []types.AnswerBlock{
			{ID: "s1", Kind: types.BlockSummary, Text: "lead"},
			{ID: "foreign-section", Kind: types.BlockOrderedList, Items: []types.AnswerBlockItem{{
				ID:          "ff1",
				Label:       "native_add",
				Text:        "stale package citation",
				CitationRef: 0,
			}}},
		},
	})
	params := json.RawMessage(`{
		"unchanged_block_ids": ["s1"],
		"replace_blocks": [{
			"id": "foreign-section",
			"kind": "ordered_list",
			"claim_uses": [{"claim_form": "definition_fact"}],
			"facet_ids": ["enumeration_item"],
			"items": [{
				"id":"ff1",
				"label":"native_add",
				"text":"FFI 外部函数声明，签名 foreign func native_add(a: Int64, b: Int64): Int64，属于包 demo.bridge。",
				"evidence_ids": ["native-add-definition"]
			}]
		}],
		"append_citations": [{"file":"eval/fixtures/testdata/cangjie_minimal/bridge/bridge.cj", "line":6, "quote":"foreign func native_add(a: Int64, b: Int64): Int64"}]
	}`)
	tool := &EmitAnswerDocumentPatch{}
	res, _ := tool.Execute(bus, params)
	if !res.Success {
		t.Fatalf("patch append citation should remain aligned after typed path canonicalization, got: %s", res.Summary)
	}
	doc := bus.Mutable.AnswerDocumentV2()
	if doc == nil || len(doc.Citations) != 9 {
		t.Fatalf("expected appended native_add citation after inherited pool, got %+v", doc)
	}
	var section *types.AnswerBlock
	for i := range doc.Blocks {
		if doc.Blocks[i].ID == "foreign-section" {
			section = &doc.Blocks[i]
			break
		}
	}
	if section == nil || len(section.Items) != 1 {
		t.Fatalf("missing foreign-section block: %+v", doc)
	}
	if got := section.Items[0].CitationRef; got != 8 {
		t.Fatalf("native_add should keep the appended definition citation index 8, got %d cit=%+v", got, doc.Citations[got])
	}
	if got := doc.Citations[section.Items[0].CitationRef]; got.Line != 6 || !strings.EqualFold(got.File, "eval/fixtures/testdata/cangjie_minimal/bridge/bridge.cj") {
		t.Fatalf("native_add citation should point at line 6 append candidate, got %+v", got)
	}
}

func TestEmitAnswerDocumentPatch_RebindsCandidateRoleSourceInventoryRow(t *testing.T) {
	bus := &types.BusContext{
		Mutable: types.NewMutableState("enumerate source inventory functions"),
		AnalysisIR: &types.AnalysisIR{RequestModel: types.RequestModel{
			Intent: types.IntentEnumerate,
			SourceInventoryProfile: &types.SourceInventoryProfile{
				IsSourceInventory: true,
				TargetRoles:       []types.AnswerCandidateRole{types.AnswerCandidateRoleFunction},
			},
		}},
	}
	bus.Mutable.SetSourceInventoryObservation(types.SourceInventoryObservation{
		Complete: true,
		Scopes:   []string{"."},
		Sets: []types.SourceInventoryObservationSet{{
			Role:     types.AnswerCandidateRoleFunction,
			Complete: true,
			Count:    1,
			Total:    1,
			Members: []types.SourceInventoryObservationMember{{
				Name:          "Serve",
				Role:          types.AnswerCandidateRoleFunction,
				File:          "src/serve.cj",
				Line:          12,
				Language:      "cangjie",
				CoverageState: types.SourceInventoryCoverageObserved,
			}},
		}},
	})
	bus.Mutable.SetAnswerDocumentV2WithMutation(types.MutationReplaceAll, &types.AnswerDocumentV2{
		DocumentModel: "v2",
		Citations: []types.Citation{
			{File: "old/stale.go", Line: 1},
		},
		Blocks: []types.AnswerBlock{
			{ID: "s1", Kind: types.BlockSummary, Text: "lead"},
			{ID: "functions", Kind: types.BlockOrderedList, Items: []types.AnswerBlockItem{{
				ID:            "serve",
				Label:         "服务入口",
				Text:          "Serve 是入口函数。",
				CandidateRole: types.AnswerCandidateRoleFunction,
				CitationRef:   0,
			}}},
		},
	})
	params := json.RawMessage(`{
		"unchanged_block_ids": ["s1"],
		"replace_blocks": [{
			"id": "functions",
			"kind": "ordered_list",
			"items": [{
				"id": "serve",
				"label": "服务入口",
				"text": "Serve 是入口函数。",
				"candidate_role": "function",
				"citation_ref": 0
			}]
		}]
	}`)

	tool := &EmitAnswerDocumentPatch{}
	res, _ := tool.Execute(bus, params)
	if !res.Success {
		t.Fatalf("patch candidate_role source-inventory citation should normalize, got: %s", res.Summary)
	}
	doc := bus.Mutable.AnswerDocumentV2()
	if doc == nil || len(doc.Citations) == 0 {
		t.Fatalf("expected source-inventory citation persisted, got %+v", doc)
	}
	var functions *types.AnswerBlock
	for i := range doc.Blocks {
		if doc.Blocks[i].ID == "functions" {
			functions = &doc.Blocks[i]
			break
		}
	}
	if functions == nil || len(functions.Items) != 1 {
		t.Fatalf("missing patched functions block: %+v", doc)
	}
	ref := functions.Items[0].CitationRef
	if ref < 0 || ref >= len(doc.Citations) {
		t.Fatalf("candidate_role item citation_ref out of range after rebind: item=%+v citations=%+v", functions.Items[0], doc.Citations)
	}
	if got := doc.Citations[ref]; got.File != "src/serve.cj" || got.Line != 12 {
		t.Fatalf("candidate_role citation = %+v, want src/serve.cj:12", got)
	}
}

func TestEmitAnswerDocumentPatch_StringWrappedNestedDiagramAndExactResolution(t *testing.T) {
	bus := &types.BusContext{Mutable: types.NewMutableState("")}
	bus.Mutable.SetAnswerDocumentV2WithMutation(types.MutationReplaceAll, &types.AnswerDocumentV2{
		Blocks: []types.AnswerBlock{
			{ID: "s1", Kind: types.BlockSummary, Text: "lead"},
		},
	})
	params := json.RawMessage(`{
		"remove_block_ids": ["s1"],
		"add_blocks": [{
			"id": "d1",
			"kind": "diagram",
			"diagram": "{\"kind\":\"sequence\",\"language\":\"mermaid\",\"body\":\"sequenceDiagram\\nA->>B: hi\"}",
			"facet_ids": "[\"current_code_path\"]"
		}],
		"replace_exact_resolution": "{\"status\":\"exact_match\",\"anchor\":\"SubExplorer.Name\"}"
	}`)
	tool := &EmitAnswerDocumentPatch{}
	res, _ := tool.Execute(bus, params)
	if !res.Success {
		t.Fatalf("patch tool must auto-repair stringified object/nested fields; got Success=false: %s", res.Summary)
	}
	doc := bus.Mutable.AnswerDocumentV2()
	if doc == nil || doc.ExactResolution == nil || doc.ExactResolution.Anchor != "SubExplorer.Name" {
		t.Fatalf("replace_exact_resolution not repaired: %+v", doc)
	}
	if len(doc.Blocks) != 1 || doc.Blocks[0].Diagram == nil || len(doc.Blocks[0].FacetIDs) != 1 {
		t.Fatalf("nested diagram/facet_ids not repaired: %+v", doc)
	}
}

func TestEmitAnswerDocumentPatch_DiagramEdgeAnchorsPromotedToBlock(t *testing.T) {
	bus := &types.BusContext{Mutable: types.NewMutableState("")}
	bus.Mutable.SetAnswerDocumentV2WithMutation(types.MutationReplaceAll, &types.AnswerDocumentV2{
		Blocks: []types.AnswerBlock{
			{ID: "s1", Kind: types.BlockSummary, Text: "lead"},
		},
	})
	params := json.RawMessage(`{
		"remove_block_ids": ["s1"],
		"add_blocks": [{
			"id": "d1",
			"kind": "diagram",
			"diagram": {
				"kind": "sequence",
				"language": "mermaid",
				"body": "sequenceDiagram\nA->>B: hi",
				"edge_anchors": [{"from_node":"A","to_node":"B","relation_kind":"call"}]
			}
		}]
	}`)
	tool := &EmitAnswerDocumentPatch{}
	res, _ := tool.Execute(bus, params)
	if !res.Success {
		t.Fatalf("patch tool must promote diagram.edge_anchors to block edge_anchors; got: %s", res.Summary)
	}
	doc := bus.Mutable.AnswerDocumentV2()
	if doc == nil || len(doc.Blocks) != 1 || len(doc.Blocks[0].EdgeAnchors) != 1 {
		t.Fatalf("promoted edge anchors not persisted: %+v", doc)
	}
}

func TestEmitAnswerDocumentPatch_DiagramParticipantBoundariesPromotedToBlock(t *testing.T) {
	bus := &types.BusContext{Mutable: types.NewMutableState("")}
	bus.Mutable.SetAnswerDocumentV2WithMutation(types.MutationReplaceAll, &types.AnswerDocumentV2{
		Blocks: []types.AnswerBlock{{ID: "s1", Kind: types.BlockSummary, Text: "lead"}},
	})
	params := json.RawMessage(`{
		"unchanged_block_ids": ["s1"],
		"add_blocks": [{
			"id": "d1",
			"kind": "diagram",
			"diagram": {
				"kind": "architecture",
				"language": "mermaid",
				"body": "flowchart TD\nAnalyzer",
				"participant_boundaries": [{"participant":"Analyzer","status":"unproven"}]
			}
		}]
	}`)
	res, _ := (&EmitAnswerDocumentPatch{}).Execute(bus, params)
	if !res.Success {
		t.Fatalf("patch tool must promote diagram.participant_boundaries to block level: %s", res.Summary)
	}
	doc := bus.Mutable.AnswerDocumentV2()
	if doc == nil || len(doc.Blocks) != 2 || len(doc.Blocks[1].ParticipantBoundaries) != 1 {
		t.Fatalf("promoted participant boundary not persisted: %+v", doc)
	}
}

func TestEmitAnswerDocumentPatch_DiagramSingletonEdgeAnchorPromotedAndRepaired(t *testing.T) {
	bus := &types.BusContext{Mutable: types.NewMutableState("")}
	bus.Mutable.SetAnswerDocumentV2WithMutation(types.MutationReplaceAll, &types.AnswerDocumentV2{
		Blocks: []types.AnswerBlock{
			{ID: "s1", Kind: types.BlockSummary, Text: "lead"},
		},
	})
	params := json.RawMessage(`{
		"remove_block_ids": ["s1"],
		"add_blocks": [{
			"id": "d1",
			"kind": "diagram",
			"diagram": {
				"kind": "sequence",
				"language": "mermaid",
				"body": "sequenceDiagram\nA->>B: hi",
				"edge_anchors": {"fromNode":"A","toNode":"B","relationKind":"Call"}
			}
		}]
	}`)
	tool := &EmitAnswerDocumentPatch{}
	res, _ := tool.Execute(bus, params)
	if !res.Success {
		t.Fatalf("patch tool must promote and repair singleton diagram.edge_anchors; got: %s", res.Summary)
	}
	doc := bus.Mutable.AnswerDocumentV2()
	if doc == nil || len(doc.Blocks) != 1 || len(doc.Blocks[0].EdgeAnchors) != 1 {
		t.Fatalf("promoted singleton edge anchor not persisted: %+v", doc)
	}
	if got := doc.Blocks[0].EdgeAnchors[0].RelationKind; got != types.DiagramRelCall {
		t.Fatalf("relation_kind = %q, want call", got)
	}
}

func TestEmitAnswerDocumentPatch_RepairsAnnotationCamelCaseShape(t *testing.T) {
	bus := &types.BusContext{Mutable: types.NewMutableState("")}
	bus.Mutable.SetAnswerDocumentV2WithMutation(types.MutationReplaceAll, &types.AnswerDocumentV2{
		Blocks: []types.AnswerBlock{
			{ID: "s1", Kind: types.BlockSummary, Text: "lead"},
		},
	})
	params := json.RawMessage(`{
		"unchanged_block_ids": ["s1"],
		"add_blocks": [{
			"id": "d1",
			"kind": "diagram",
			"diagram": {"kind": "sequence", "language": "mermaid", "body": "sequenceDiagram\nA->>B: hi"},
			"claim_uses": [{"claimForm": "returnFact", "facetId": "current_code_path"}],
			"edge_anchors": [{"fromNode": "A", "toNode": "B", "relationKind": "Call", "claimForm": "callEdge"}]
		}]
	}`)
	tool := &EmitAnswerDocumentPatch{}
	res, _ := tool.Execute(bus, params)
	if !res.Success {
		t.Fatalf("patch tool must auto-repair camelCase annotation shape; got: %s", res.Summary)
	}
	doc := bus.Mutable.AnswerDocumentV2()
	if doc == nil || len(doc.Blocks) != 2 {
		t.Fatalf("repaired patch did not persist added block: %+v", doc)
	}
	added := doc.Blocks[1]
	if got := added.ClaimUses; len(got) != 1 ||
		got[0].ClaimForm != types.ClaimReturnFact ||
		got[0].FacetID != "current_code_path" {
		t.Fatalf("claim_uses not normalized: %+v", got)
	}
	if got := added.EdgeAnchors; len(got) != 1 ||
		got[0].RelationKind != types.DiagramRelCall ||
		got[0].ClaimForm != types.ClaimCallEdge {
		t.Fatalf("edge_anchors not normalized: %+v", got)
	}
}

func TestEmitAnswerDocumentPatch_RepairsSingletonArrayShapes(t *testing.T) {
	bus := &types.BusContext{Mutable: types.NewMutableState("")}
	bus.Mutable.SetAnswerDocumentV2WithMutation(types.MutationReplaceAll, &types.AnswerDocumentV2{
		Blocks: []types.AnswerBlock{
			{ID: "s1", Kind: types.BlockSummary, Text: "lead"},
		},
	})
	params := json.RawMessage(`{
		"unchanged_block_ids": ["s1"],
		"add_blocks": [{
			"id": "d1",
			"kind": "diagram",
			"diagram": {"kind": "sequence", "language": "mermaid", "body": "sequenceDiagram\nA->>B: hi"},
			"items": {"id": "i1", "label": "A", "text": "entry"},
			"columns": "Component",
			"facet_ids": "current_code_path",
			"claim_uses": {"claimForm": "definitionFact", "facetId": "current_code_path"},
			"edge_anchors": {"fromNode": "A", "toNode": "B", "relationKind": "Call"}
		}]
	}`)
	tool := &EmitAnswerDocumentPatch{}
	res, _ := tool.Execute(bus, params)
	if !res.Success {
		t.Fatalf("patch tool must repair singleton array shapes; got: %s", res.Summary)
	}
	doc := bus.Mutable.AnswerDocumentV2()
	if doc == nil || len(doc.Blocks) != 2 {
		t.Fatalf("repaired patch did not persist added block: %+v", doc)
	}
	added := doc.Blocks[1]
	if len(added.Items) != 1 || len(added.Columns) != 1 ||
		len(added.FacetIDs) != 1 || len(added.ClaimUses) != 1 ||
		len(added.EdgeAnchors) != 1 {
		t.Fatalf("singleton fields not repaired into arrays: %+v", added)
	}
	if added.ClaimUses[0].ClaimForm != types.ClaimDefinitionFact ||
		added.EdgeAnchors[0].RelationKind != types.DiagramRelCall {
		t.Fatalf("singleton annotation aliases not normalized: %+v / %+v", added.ClaimUses, added.EdgeAnchors)
	}
}

func TestEmitAnswerDocumentPatch_RejectsHeaderOnlyTable(t *testing.T) {
	bus := &types.BusContext{Mutable: types.NewMutableState("")}
	bus.Mutable.SetAnswerDocumentV2WithMutation(types.MutationReplaceAll, &types.AnswerDocumentV2{
		Blocks: []types.AnswerBlock{{ID: "s1", Kind: types.BlockSummary, Text: "lead"}},
	})
	params := json.RawMessage(`{
		"unchanged_block_ids": ["s1"],
		"add_blocks": [{
			"id": "stages",
			"kind": "table",
			"columns": ["Stage", "输入", "输出", "主要状态载体"]
		}]
	}`)
	res, err := (&EmitAnswerDocumentPatch{}).Execute(bus, params)
	if err != nil {
		t.Fatalf("unexpected exec error: %v", err)
	}
	if res.Success {
		t.Fatalf("patch must not add a header-only table: %+v", bus.Mutable.AnswerDocumentV2())
	}
	for _, want := range []string{"add_blocks[0]", "kind=table has no visible rows", "items[] row"} {
		if !strings.Contains(res.Summary, want) {
			t.Fatalf("patch repair summary missing %q: %s", want, res.Summary)
		}
	}
}

func TestEmitAnswerDocumentPatch_RepairsTopLevelSingletonArrayShapes(t *testing.T) {
	bus := &types.BusContext{Mutable: types.NewMutableState("")}
	bus.Mutable.SetAnswerDocumentV2WithMutation(types.MutationReplaceAll, &types.AnswerDocumentV2{
		Blocks: []types.AnswerBlock{
			{ID: "s1", Kind: types.BlockSummary, Text: "lead"},
		},
	})
	params := json.RawMessage(`{
		"unchanged_block_ids": "s1",
		"add_blocks": {"id": "extra", "kind": "summary", "text": "extra", "items": {"id": "c0", "citation_ref": 0}},
		"append_citations": {"file": "a.go", "line": 1},
		"replace_caveats": "bounded scope"
	}`)
	tool := &EmitAnswerDocumentPatch{}
	res, _ := tool.Execute(bus, params)
	if !res.Success {
		t.Fatalf("patch tool must repair top-level singleton array shapes; got: %s", res.Summary)
	}
	doc := bus.Mutable.AnswerDocumentV2()
	if doc == nil || len(doc.Blocks) != 2 || len(doc.Citations) != 1 || len(doc.Caveats) != 1 {
		t.Fatalf("top-level singleton fields not repaired into arrays: %+v", doc)
	}
}

func TestEmitAnswerDocumentPatch_RepairsSingleFacetIDsInsideClaimUse(t *testing.T) {
	bus := &types.BusContext{Mutable: types.NewMutableState("")}
	bus.Mutable.SetAnswerDocumentV2WithMutation(types.MutationReplaceAll, &types.AnswerDocumentV2{
		Blocks: []types.AnswerBlock{
			{ID: "s1", Kind: types.BlockSummary, Text: "lead"},
		},
	})
	params := json.RawMessage(`{
		"unchanged_block_ids": ["s1"],
		"add_blocks": [{
			"id": "detail",
			"kind": "section",
			"text": "detail",
			"claim_uses": [{"claim_form": "definition_fact", "facet_ids": ["current_code_path"]}]
		}]
	}`)
	tool := &EmitAnswerDocumentPatch{}
	res, _ := tool.Execute(bus, params)
	if !res.Success {
		t.Fatalf("patch tool must repair single claim_uses[].facet_ids value; got: %s", res.Summary)
	}
	doc := bus.Mutable.AnswerDocumentV2()
	if doc == nil || len(doc.Blocks) != 2 || len(doc.Blocks[1].ClaimUses) != 1 {
		t.Fatalf("repaired patch did not persist claim use: %+v", doc)
	}
	if got := doc.Blocks[1].ClaimUses[0].FacetID; got != "current_code_path" {
		t.Fatalf("facet_id = %q, want current_code_path", got)
	}
}

func TestEmitAnswerDocumentPatch_HoistsItemLevelClaimUses(t *testing.T) {
	bus := &types.BusContext{Mutable: types.NewMutableState("")}
	bus.Mutable.SetAnswerDocumentV2WithMutation(types.MutationReplaceAll, &types.AnswerDocumentV2{
		Blocks: []types.AnswerBlock{
			{ID: "s1", Kind: types.BlockSummary, Text: "lead"},
		},
	})
	params := json.RawMessage(`{
		"unchanged_block_ids": ["s1"],
		"add_blocks": [{
			"id": "list",
			"kind": "ordered_list",
			"items": [{
				"id": "i1",
				"label": "ContractCheck",
				"text": "checks the final answer contract",
				"claim_use": {"claimForm": "definitionFact", "facetId": "current_code_path"}
			}]
		}]
	}`)
	tool := &EmitAnswerDocumentPatch{}
	res, _ := tool.Execute(bus, params)
	if !res.Success {
		t.Fatalf("patch tool must hoist item-level claim_use; got: %s", res.Summary)
	}
	doc := bus.Mutable.AnswerDocumentV2()
	if doc == nil || len(doc.Blocks) != 2 {
		t.Fatalf("repaired patch did not persist added block: %+v", doc)
	}
	got := doc.Blocks[1].ClaimUses
	if len(got) != 1 || got[0].ClaimForm != types.ClaimDefinitionFact || got[0].FacetID != "current_code_path" {
		t.Fatalf("item-level claim_use was not hoisted and normalized: %+v", got)
	}
}

func TestEmitAnswerDocumentPatch_StringCitationAndSnippetLineNumbers(t *testing.T) {
	bus := &types.BusContext{Mutable: types.NewMutableState("")}
	bus.Mutable.SetAnswerDocumentV2WithMutation(types.MutationReplaceAll, &types.AnswerDocumentV2{
		Blocks: []types.AnswerBlock{
			{ID: "s1", Kind: types.BlockSummary, Text: "lead"},
		},
	})
	params := json.RawMessage(`{
		"unchanged_block_ids": ["s1"],
		"append_citations": [{"file":"internal/agent/sub_explorer.go","line":"31","line_end":"33"}],
		"replace_snippets": [{"file":"internal/agent/sub_explorer.go","start_line":"31","end_line":"33","code":"func (s *SubExplorer) Name() string { return \"explorer\" }"}]
	}`)
	tool := &EmitAnswerDocumentPatch{}
	res, _ := tool.Execute(bus, params)
	if !res.Success {
		t.Fatalf("patch tool must accept string line numbers; got Success=false: %s", res.Summary)
	}
	doc := bus.Mutable.AnswerDocumentV2()
	if doc == nil || len(doc.Citations) != 1 || doc.Citations[0].Line != 31 || doc.Citations[0].LineEnd != 33 {
		t.Fatalf("append_citations line numbers not parsed: %+v", doc)
	}
	if len(doc.Snippets) != 1 || doc.Snippets[0].StartLine != 31 || doc.Snippets[0].EndLine != 33 {
		t.Fatalf("replace_snippets line numbers not parsed: %+v", doc.Snippets)
	}
}
