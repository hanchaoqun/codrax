package tool

import (
	"encoding/json"
	"reflect"
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/toolparam"
	"github.com/hanchaoqun/codrax/internal/types"
)

func answerDocumentProjectedBlockSchema(t *testing.T, raw json.RawMessage) (map[string]any, map[string]any) {
	t.Helper()
	var root map[string]any
	if err := json.Unmarshal(raw, &root); err != nil {
		t.Fatalf("projected schema must parse: %v", err)
	}
	props := root["properties"].(map[string]any)
	blocks := props["blocks"].(map[string]any)
	blockItems := blocks["items"].(map[string]any)
	return blockItems, blockItems["properties"].(map[string]any)
}

// TestBuildAnswerDocumentParametersFor_NilViewReturnsCanonical pins
// the fallback contract: callers without a compiled view (tests,
// future no-context paths) MUST see the full canonical surface.
func TestBuildAnswerDocumentParametersFor_NilViewReturnsCanonical(t *testing.T) {
	got := BuildAnswerDocumentParametersFor(nil)
	canonical := (&EmitAnswerDocument{}).Parameters()
	if string(got) != string(canonical) {
		t.Errorf("nil view must yield canonical schema; diverged")
	}
}

func TestBuildAnswerDocumentParametersForProjectsExactRuntimeWorkReceiptChoices(t *testing.T) {
	view := &types.AnswerSemanticView{
		Family:         types.QFRootCauseTrace,
		RequiredBlocks: []types.BlockRequirement{{Kind: types.BlockSummary, Required: true}},
		RuntimeWorkRelationContract: &types.RuntimeWorkRelationContract{Rows: []types.RuntimeWorkRelationRow{{
			ObservationID: "trace_query:test#trace_semantic_span:1",
			AllowedConclusions: []types.RuntimeWorkRelationConclusion{
				types.RuntimeWorkRelationConclusionRelatedCausalityUnproven,
				types.RuntimeWorkRelationConclusionRelationUnproven,
			},
		}}},
	}
	_, props := answerDocumentProjectedBlockSchema(t, BuildAnswerDocumentParametersFor(view))
	node, ok := props["runtime_work_relation"].(map[string]any)
	if !ok {
		t.Fatal("active typed work-relation contract must expose its exact receipt field")
	}
	choices, ok := node["oneOf"].([]any)
	if !ok || len(choices) != 2 {
		t.Fatalf("runtime-work receipt choices=%T %+v", node["oneOf"], node["oneOf"])
	}
	choice := choices[0].(map[string]any)
	choiceProps := choice["properties"].(map[string]any)
	if got := choiceProps["observation_id"].(map[string]any)["const"]; got != "trace_query:test#trace_semantic_span:1" {
		t.Fatalf("observation const=%v", got)
	}
	if got := choiceProps["conclusion"].(map[string]any)["const"]; got != string(types.RuntimeWorkRelationConclusionRelatedCausalityUnproven) {
		t.Fatalf("conclusion const=%v", got)
	}
	second := choices[1].(map[string]any)["properties"].(map[string]any)
	if got := second["conclusion"].(map[string]any)["const"]; got != string(types.RuntimeWorkRelationConclusionRelationUnproven) {
		t.Fatalf("second model-selectable conclusion const=%v", got)
	}

	view.RuntimeWorkRelationContract = nil
	_, props = answerDocumentProjectedBlockSchema(t, BuildAnswerDocumentParametersFor(view))
	if _, exposed := props["runtime_work_relation"]; exposed {
		t.Fatal("unrelated dispatch must not expose runtime-work receipt mental load")
	}
}

func TestBuildAnswerDocumentParametersForProjectsConceptualTerminalChoicesAndEmptyEvidenceForm(t *testing.T) {
	view := &types.AnswerSemanticView{
		Family:         types.QFCallChain,
		RequiredBlocks: []types.BlockRequirement{{Kind: types.BlockSummary, Required: true}},
		ConceptualTerminalResolutionContract: &types.ConceptualTerminalResolutionContract{Rows: []types.ConceptualTerminalResolutionRow{{
			EvidenceID: "ev-terminal",
			AllowedConclusions: []types.ConceptualTerminalResolutionConclusion{
				types.ConceptualTerminalResolutionCurrentTerminalDiffers,
				types.ConceptualTerminalResolutionDestinationUnproven,
			},
		}}},
	}
	_, props := answerDocumentProjectedBlockSchema(t, BuildAnswerDocumentParametersFor(view))
	node, ok := props["conceptual_terminal_resolution"].(map[string]any)
	if !ok {
		t.Fatal("active conceptual-terminal contract must expose its model-owned receipt field")
	}
	choices := node["oneOf"].([]any)
	if len(choices) != 2 {
		t.Fatalf("conceptual-terminal choices=%+v", choices)
	}
	firstProps := choices[0].(map[string]any)["properties"].(map[string]any)
	if got := choices[0].(map[string]any)["description"].(string); !strings.Contains(got, "ev-terminal") {
		t.Fatalf("terminal choice did not publish its exact evidence mapping: %q", got)
	}
	if got := firstProps["evidence_id"].(map[string]any)["const"]; got != "ev-terminal" {
		t.Fatalf("terminal evidence const=%v", got)
	}
	if got := firstProps["conclusion"].(map[string]any)["const"]; got != string(types.ConceptualTerminalResolutionCurrentTerminalDiffers) {
		t.Fatalf("terminal conclusion const=%v", got)
	}

	view.ConceptualTerminalResolutionContract.Rows = nil
	_, props = answerDocumentProjectedBlockSchema(t, BuildAnswerDocumentParametersFor(view))
	node = props["conceptual_terminal_resolution"].(map[string]any)
	emptyChoice := node["oneOf"].([]any)[0].(map[string]any)
	emptyProps := emptyChoice["properties"].(map[string]any)
	if _, hasEvidence := emptyProps["evidence_id"]; hasEvidence {
		t.Fatal("no-row conceptual-terminal choice must not ask the model to invent an evidence id")
	}
	if got := emptyProps["conclusion"].(map[string]any)["const"]; got != string(types.ConceptualTerminalResolutionDestinationUnproven) {
		t.Fatalf("empty-evidence conclusion const=%v", got)
	}

	view.ConceptualTerminalResolutionContract = nil
	_, props = answerDocumentProjectedBlockSchema(t, BuildAnswerDocumentParametersFor(view))
	if _, exposed := props["conceptual_terminal_resolution"]; exposed {
		t.Fatal("non-conceptual dispatch must not expose terminal-resolution mental load")
	}
}

func TestBuildAnswerDocumentParametersFor_ProjectedBlockObjectIsClosedAndTeachingMatchesKindEnum(t *testing.T) {
	view := &types.AnswerSemanticView{
		Family: types.QFRoleLookup,
		RequiredBlocks: []types.BlockRequirement{
			{Kind: types.BlockSummary, Required: true},
		},
		OptionalBlocks: []types.BlockRequirement{
			{Kind: types.BlockSection},
			// A stale/soft roster entry cannot expose diagram without the
			// compiled DiagramPlan that owns its payload.
			{Kind: types.BlockDiagram},
			{Kind: types.BlockCaveat},
		},
	}
	blockItems, blockProps := answerDocumentProjectedBlockSchema(t, BuildAnswerDocumentParametersFor(view))
	if open, ok := blockItems["additionalProperties"].(bool); !ok || open {
		t.Fatalf("dispatch-local block schema must reject omitted/unknown payload fields: %+v", blockItems["additionalProperties"])
	}
	if _, exposed := blockProps["diagram"]; exposed {
		t.Fatal("diagram payload must be absent without a typed diagram plan")
	}
	kindNode := blockProps["kind"].(map[string]any)
	for _, raw := range kindNode["enum"].([]any) {
		if raw == string(types.BlockDiagram) {
			t.Fatalf("diagram kind must be absent without a typed diagram plan: %v", kindNode["enum"])
		}
	}
	teaching := projectedAnswerBlockKindTeaching(view)
	if strings.Contains(teaching, "`diagram`") ||
		!strings.Contains(teaching, "`summary`, `section`, `caveat`") {
		t.Fatalf("dispatch teaching drifted from projected kind enum: %s", teaching)
	}
}

func TestBuildAnswerDocumentParametersFor_DiagramPayloadRequiresNativeDiagramKind(t *testing.T) {
	view := &types.AnswerSemanticView{
		Family: types.QFArchitecture,
		RequiredBlocks: []types.BlockRequirement{
			{Kind: types.BlockSummary, Required: true},
			{Kind: types.BlockDiagram, Required: true},
		},
		OptionalBlocks: []types.BlockRequirement{{Kind: types.BlockSection}},
		DiagramPlan:    &types.DiagramFacetGraph{Kind: types.DiagramArchitecture, Required: true},
	}
	blockItems, blockProps := answerDocumentProjectedBlockSchema(t, BuildAnswerDocumentParametersFor(view))
	if _, exposed := blockProps["diagram"]; !exposed {
		t.Fatal("typed diagram plan must expose the native diagram payload")
	}
	found := false
	for _, entry := range schemaAllOfEntries(blockItems) {
		ifNode, _ := entry["if"].(map[string]any)
		required, _ := ifNode["required"].([]any)
		if !reflect.DeepEqual(required, []any{"diagram"}) {
			continue
		}
		thenNode := entry["then"].(map[string]any)
		thenProps := thenNode["properties"].(map[string]any)
		kindNode := thenProps["kind"].(map[string]any)
		if got := kindNode["const"]; got != string(types.BlockDiagram) {
			t.Fatalf("diagram payload discriminator=%v, want native diagram kind", got)
		}
		found = true
	}
	if !found {
		t.Fatalf("diagram payload ownership conditional missing: %+v", blockItems["allOf"])
	}
}

func TestBuildAnswerDocumentPatchParametersForReusesProjectedBlockSchema(t *testing.T) {
	view := &types.AnswerSemanticView{
		RelationAxis: types.AxisFlow,
		DiagramPlan:  &types.DiagramFacetGraph{Kind: types.DiagramFlow, Required: true},
		DiagramParticipantObligations: []types.DiagramParticipantHint{
			{Identity: "Analyzer", Role: types.DiagramParticipantIncidentRequired},
			{Identity: "Mutable", Role: types.DiagramParticipantIncidentRequired},
		},
	}
	fullItem, fullProps := answerDocumentProjectedBlockSchema(t, BuildAnswerDocumentParametersFor(view))

	var patchRoot map[string]any
	if err := json.Unmarshal(BuildAnswerDocumentPatchParametersFor(view), &patchRoot); err != nil {
		t.Fatalf("patch projected schema must parse: %v", err)
	}
	patchProps := patchRoot["properties"].(map[string]any)
	for _, field := range []string{"diagram_edge_edits", "diagram_boundary_replacements", "diagram_relation_scope_edits", "diagram_participant_edits"} {
		if _, ok := patchProps[field]; !ok {
			t.Fatalf("patch projected schema lost atomic diagram field %q", field)
		}
	}
	if got := patchProps["diagram_edge_edits"].(map[string]any)["maxItems"]; got != float64(maxModelAuthoredDiagramEdgeEdits) {
		t.Fatalf("diagram atomic edit schema/executor limit drift: schema=%v executor=%d", got, maxModelAuthoredDiagramEdgeEdits)
	}
	if got := patchProps["diagram_participant_edits"].(map[string]any)["maxItems"]; got != float64(maxModelAuthoredDiagramParticipantEdits) {
		t.Fatalf("diagram participant edit schema/executor limit drift: schema=%v executor=%d", got, maxModelAuthoredDiagramParticipantEdits)
	}
	if got := patchProps["diagram_relation_scope_edits"].(map[string]any)["maxItems"]; got != float64(maxModelAuthoredDiagramRelationScopeEdits) {
		t.Fatalf("diagram relation-scope edit schema/executor limit drift: schema=%v executor=%d", got, maxModelAuthoredDiagramRelationScopeEdits)
	}
	edgeEditItem := patchProps["diagram_edge_edits"].(map[string]any)["items"].(map[string]any)
	edgeEditProps := edgeEditItem["properties"].(map[string]any)
	if _, ok := edgeEditProps["failure_ref"]; !ok {
		t.Fatalf("diagram atomic edit schema lost the live failure selector: %v", edgeEditProps)
	}
	if _, ok := edgeEditProps["addition_ref"]; !ok {
		t.Fatalf("diagram atomic edit schema lost the live allowed-addition selector: %v", edgeEditProps)
	}
	for _, field := range []string{"from_node_visible_label", "to_node_visible_label"} {
		if _, ok := edgeEditProps[field]; !ok {
			t.Fatalf("diagram atomic edit schema lost model-owned endpoint presentation field %q: %v", field, edgeEditProps)
		}
	}
	edgePayload := edgeEditProps["edge"].(map[string]any)
	edgeRequired := edgePayload["required"].([]any)
	if len(edgeRequired) != 2 || edgeRequired[0] != "from_node" || edgeRequired[1] != "to_node" {
		t.Fatalf("addition_ref lane must require only model-owned visible endpoints at schema level: required=%v", edgeRequired)
	}
	if required := edgeEditItem["required"].([]any); len(required) != 1 || required[0] != "action" {
		t.Fatalf("failure_ref lane must not require coordinate retyping: required=%v", required)
	}
	fullBytes, _ := json.Marshal(fullItem)
	for _, field := range []string{"replace_blocks", "add_blocks"} {
		array := patchProps[field].(map[string]any)
		item := array["items"].(map[string]any)
		itemBytes, _ := json.Marshal(item)
		if string(itemBytes) != string(fullBytes) {
			t.Fatalf("patch %s block item drifted from projected full item", field)
		}
		props := item["properties"].(map[string]any)
		edge := props["edge_anchors"].(map[string]any)
		edgeProps := edge["items"].(map[string]any)["properties"].(map[string]any)
		for _, identity := range []string{"from_identity", "to_identity"} {
			if _, ok := edgeProps[identity]; !ok {
				t.Fatalf("patch %s lost edge identity selector %q: %v", field, identity, edgeProps)
			}
		}
		if _, ok := props["participant_boundaries"]; !ok {
			t.Fatalf("patch %s lost projected participant boundary field", field)
		}
	}
	if _, ok := fullProps["participant_boundaries"]; !ok {
		t.Fatal("test setup: full projected schema lacks participant boundary field")
	}
}

func TestBuildAnswerDocumentPatchParametersForProjectsClosedEnumFieldEdits(t *testing.T) {
	view := &types.AnswerSemanticView{
		TraceCausalClaimContract: &types.TraceCausalClaimContract{
			Allowed: []types.TraceCausalClaimCaliber{
				types.TraceCausalClaimNoConclusion,
				types.TraceCausalClaimBoundedWindow,
			},
			Ceiling: types.TraceCausalClaimBoundedWindow,
		},
		CurrentStatusDiagnostic: &types.CurrentStatusDiagnosticContract{Required: true},
		ErrorGranularityProfile: &types.ErrorGranularityProfile{IsGranularityQuestion: true},
	}
	var root map[string]any
	if err := json.Unmarshal(BuildAnswerDocumentPatchParametersFor(view), &root); err != nil {
		t.Fatalf("patch schema must parse: %v", err)
	}
	props := root["properties"].(map[string]any)
	edits := props["block_field_edits_v1"].(map[string]any)
	branches := edits["items"].(map[string]any)["oneOf"].([]any)
	seen := map[string][]any{}
	for _, raw := range branches {
		branchProps := raw.(map[string]any)["properties"].(map[string]any)
		field := branchProps["field"].(map[string]any)["const"].(string)
		seen[field] = branchProps["value"].(map[string]any)["enum"].([]any)
		if open, _ := raw.(map[string]any)["additionalProperties"].(bool); open {
			t.Fatalf("local field branch must stay closed: %+v", raw)
		}
	}
	if got := seen[string(types.AnswerBlockFieldTraceCausalClaimCaliber)]; !reflect.DeepEqual(got, []any{"no_causal_conclusion", "bounded_window_candidate"}) {
		t.Fatalf("trace local edit widened the dispatch ceiling: %v", got)
	}
	for _, field := range []types.AnswerBlockEditableFieldV1{
		types.AnswerBlockFieldScopeDisclosure,
		types.AnswerBlockFieldSurfaceRole,
	} {
		if len(seen[string(field)]) == 0 {
			t.Fatalf("generic safe enum field missing from v1 projection: %s", field)
		}
	}
}

func TestAnswerDocumentPatchFieldEditTargetsUseExactKindAndExcludeSystemOrLeaseBlocks(t *testing.T) {
	view := &types.AnswerSemanticView{
		TraceCausalClaimContract: &types.TraceCausalClaimContract{
			Allowed: []types.TraceCausalClaimCaliber{types.TraceCausalClaimBoundedWindow},
			Ceiling: types.TraceCausalClaimBoundedWindow,
		},
		CurrentStatusDiagnostic: &types.CurrentStatusDiagnosticContract{Required: true},
		ErrorGranularityProfile: &types.ErrorGranularityProfile{IsGranularityQuestion: true},
	}
	prev := &types.AnswerDocumentV2{DocumentModel: "v2", Blocks: []types.AnswerBlock{
		{ID: "summary", Kind: types.BlockSummary},
		{ID: "decision", Kind: types.BlockDecision},
		{ID: "support", Kind: types.BlockSection},
		{ID: "system", Kind: types.BlockSummary, SystemGeneratedKind: types.AnswerSystemGeneratedRuntimeTrace},
	}}
	raw := projectAnswerDocumentPatchFieldEditTargets(
		BuildAnswerDocumentPatchParametersFor(view), prev, view, []string{"support"},
	)
	var root map[string]any
	if err := json.Unmarshal(raw, &root); err != nil {
		t.Fatalf("projected schema must parse: %v", err)
	}
	branches := root["properties"].(map[string]any)["block_field_edits_v1"].(map[string]any)["items"].(map[string]any)["oneOf"].([]any)
	idsByField := map[string][]any{}
	for _, rawBranch := range branches {
		props := rawBranch.(map[string]any)["properties"].(map[string]any)
		field := props["field"].(map[string]any)["const"].(string)
		idsByField[field] = props["block_id"].(map[string]any)["enum"].([]any)
	}
	if got := idsByField[string(types.AnswerBlockFieldTraceCausalClaimCaliber)]; !reflect.DeepEqual(got, []any{"summary"}) {
		t.Fatalf("trace field targets=%v want exact model summary only", got)
	}
	if got := idsByField[string(types.AnswerBlockFieldCurrentStatusVerdict)]; !reflect.DeepEqual(got, []any{"decision"}) {
		t.Fatalf("current-status field targets=%v want exact decision only", got)
	}
	if got := idsByField[string(types.AnswerBlockFieldSurfaceRole)]; !reflect.DeepEqual(got, []any{"summary", "decision"}) {
		t.Fatalf("surface-role targets=%v; excluded/system blocks leaked", got)
	}
}

func TestEmitAnswerDocumentPatchParametersFor_LocalLeasePublishesOnlyExecutableCapabilities(t *testing.T) {
	base := atomicPatchTestDocument()
	lease := types.NewAnswerDiagramRelationRepairLease(base,
		[]types.AnswerDiagramRelationRepairFailure{{
			BlockID: "diag", Issue: "call_edge_unproven",
			FromNode: "A", ToNode: "B", FromIdentity: "Analyzer", ToIdentity: "Explorer",
			RelationKind: types.DiagramRelPrecedence,
		}}, []types.AnswerDiagramRelationRepairCandidate{{
			BlockID: "diag", RelationKind: types.DiagramRelCall,
			FromIdentity: "Extractor", ToIdentity: "Finalizer", Source: "internal/orchestrator/topology.go:1",
		}})
	if lease == nil {
		t.Fatal("test setup: expected live local lease")
	}
	lease.OptionalOrphanCleanups = []types.AnswerDiagramOrphanCleanupCandidate{{
		BlockID: "diag", ParticipantID: "C", AllowedActions: []types.AnswerDiagramOrphanDispositionAction{
			types.AnswerDiagramOrphanDispositionRemove,
			types.AnswerDiagramOrphanDispositionRetain,
		},
	}}
	mut := types.NewMutableState("local diagram schema")
	mut.SetAnswerDocumentV2WithMutation(types.MutationReplaceAll, base)
	mut.SetAnswerDiagramRelationRepairLease(lease)
	raw := (&EmitAnswerDocumentPatch{}).ParametersFor(&types.AgentContext{Mutable: mut})
	var root map[string]any
	if err := json.Unmarshal(raw, &root); err != nil {
		t.Fatalf("projected patch schema must parse: %v", err)
	}
	props := root["properties"].(map[string]any)
	for _, field := range []string{"add_blocks"} {
		if _, ok := props[field]; ok {
			t.Fatalf("live local lease must hide roster-changing mutation %q", field)
		}
	}
	removeIDs := props["remove_block_ids"].(map[string]any)
	if got := removeIDs["items"].(map[string]any)["enum"].([]any); !reflect.DeepEqual(got, []any{"summary"}) {
		t.Fatalf("unrelated exact removal roster=%v, want only summary", got)
	}
	replaceBlocks, ok := props["replace_blocks"].(map[string]any)
	if !ok {
		t.Fatal("live local lease hid replacement of an unrelated existing block")
	}
	replaceItem := replaceBlocks["items"].(map[string]any)
	replaceProps := replaceItem["properties"].(map[string]any)
	if got := replaceProps["id"].(map[string]any)["enum"].([]any); !reflect.DeepEqual(got, []any{"summary"}) {
		t.Fatalf("whole-block replacement roster=%v, want only unrelated summary block", got)
	}
	edgeEdits := props["diagram_edge_edits"].(map[string]any)
	if edgeEdits["minItems"] != float64(1) || edgeEdits["maxItems"] != float64(2) || edgeEdits["uniqueItems"] != true {
		t.Fatalf("live refs must define the exact transaction cardinality: %+v", edgeEdits)
	}
	branches := edgeEdits["items"].(map[string]any)["oneOf"].([]any)
	if len(branches) != 2 {
		t.Fatalf("expected remove+add branches for an evidence-negative carrier, got %d: %+v", len(branches), branches)
	}
	wantFailureRef := lease.Failures[0].FailureRef
	wantAdditionRef := lease.AllowedAdditions[0].AdditionRef
	seen := make(map[string]bool)
	for _, rawBranch := range branches {
		branch := rawBranch.(map[string]any)
		if branch["additionalProperties"] != false {
			t.Fatalf("each exact branch must reject unadvertised legacy fields: %+v", branch)
		}
		branchProps := branch["properties"].(map[string]any)
		for _, forbidden := range []string{"block_id", "match", "occurrence", "body_occurrence"} {
			if _, ok := branchProps[forbidden]; ok {
				t.Fatalf("exact-ref branch leaked legacy selector %q: %+v", forbidden, branchProps)
			}
		}
		action := branchProps["action"].(map[string]any)["enum"].([]any)[0].(string)
		refField := "failure_ref"
		ref := wantFailureRef
		if action == "add" {
			refField, ref = "addition_ref", wantAdditionRef
		}
		if got := branchProps[refField].(map[string]any)["enum"].([]any); len(got) != 1 || got[0] != ref {
			t.Fatalf("branch %s did not pin the current opaque ref: %+v", action, branchProps)
		}
		if edge, ok := branchProps["edge"].(map[string]any); ok {
			edgeProps := edge["properties"].(map[string]any)
			if edge["additionalProperties"] != false || len(edgeProps) != 3 {
				t.Fatalf("replacement/addition edge must expose visible model fields only: %+v", edge)
			}
			for _, visible := range []string{"from_node", "to_node", "visible_label"} {
				if _, ok := edgeProps[visible]; !ok {
					t.Fatalf("edge branch lost model-owned field %q: %+v", visible, edgeProps)
				}
			}
			for _, visible := range []string{"from_node_visible_label", "to_node_visible_label"} {
				if _, ok := branchProps[visible]; !ok {
					t.Fatalf("edge branch lost model-owned endpoint presentation field %q: %+v", visible, branchProps)
				}
			}
		}
		seen[action] = true
	}
	if !seen["remove"] || seen["replace"] || !seen["add"] || seen["relabel"] {
		t.Fatalf("schema action roster drifted from live capabilities: %+v", seen)
	}

	boundaries := props["diagram_boundary_replacements"].(map[string]any)
	boundaryItem := boundaries["items"].(map[string]any)
	boundaryProps := boundaryItem["properties"].(map[string]any)
	if got := boundaryProps["block_id"].(map[string]any)["enum"].([]any); !reflect.DeepEqual(got, []any{"diag"}) {
		t.Fatalf("boundary repair must stay within the live diagram target: %v", got)
	}

	participantEdits := props["diagram_participant_edits"].(map[string]any)
	participantBranches := participantEdits["items"].(map[string]any)["oneOf"].([]any)
	if len(participantBranches) != 2 || participantEdits["maxItems"] != float64(1) {
		t.Fatalf("participant cleanup must expose one exact candidate with its two choices: %+v", participantEdits)
	}
	participantActions := make(map[string]bool)
	for _, rawBranch := range participantBranches {
		branch := rawBranch.(map[string]any)
		branchProps := branch["properties"].(map[string]any)
		action := branchProps["action"].(map[string]any)["enum"].([]any)[0].(string)
		participantActions[action] = true
		if got := branchProps["block_id"].(map[string]any)["enum"].([]any); !reflect.DeepEqual(got, []any{"diag"}) {
			t.Fatalf("participant block selector=%v", got)
		}
		if got := branchProps["participant_id"].(map[string]any)["enum"].([]any); !reflect.DeepEqual(got, []any{"C"}) {
			t.Fatalf("participant selector=%v", got)
		}
		_, hasLabel := branchProps["visible_label"]
		if hasLabel != (action == "retain_as_context") {
			t.Fatalf("visible label ownership drifted for action=%s: %+v", action, branchProps)
		}
	}
	if !participantActions["remove_if_isolated"] || !participantActions["retain_as_context"] {
		t.Fatalf("orphan disposition actions=%v", participantActions)
	}
	if desc := (&EmitAnswerDocumentPatch{}).DescriptionFor(&types.AgentContext{Mutable: mut}); !strings.Contains(desc, "current schema is the sole capability authority") ||
		!strings.Contains(desc, "only unrelated existing blocks") {
		t.Fatalf("live description must match the executable schema: %q", desc)
	}
}

func TestAnswerDocumentPatchFieldEditProjection_OffersExactExistingEndpointCarrierFacetOnly(t *testing.T) {
	view := &types.AnswerSemanticView{
		Family: types.QFCallChain,
		CallChainEndpointBoundary: &types.CallChainEndpointBoundary{
			Disposition:    types.CallChainEndpointNoDirectedPath,
			SourceEndpoint: "agent.buildAnalysisIR",
			RequestedSink:  "gate.Run",
			EvidenceCapsule: &types.CallChainEndpointEvidenceCapsule{
				Status: types.CallChainEndpointEvidenceSharedCalleeBoundary,
				SourcePath: []types.CallChainEvidenceEdge{{
					From: "agent.buildAnalysisIR", To: "gate.RunWith", EvidenceID: "source-edge",
				}},
				SinkPath: []types.CallChainEvidenceEdge{{
					From: "gate.Run", To: "gate.RunWith", EvidenceID: "sink-edge",
				}},
			},
		},
	}
	prev := &types.AnswerDocumentV2{DocumentModel: "v2", Blocks: []types.AnswerBlock{
		{
			ID: "source-path", Kind: types.BlockOrderedList,
			FacetIDs: []string{string(types.FacetPrincipalPathEdge)},
			Items:    []types.AnswerBlockItem{{ID: "source", EvidenceIDs: []string{"source-edge"}}},
			EdgeAnchors: []types.DiagramEdgeAnchor{{
				FromNode: "buildAnalysisIR", ToNode: "gate.RunWith", FromIdentity: "buildAnalysisIR", ToIdentity: "gate.RunWith", RelationKind: types.DiagramRelCall,
			}},
		},
		{
			ID: "sink-boundary", Kind: types.BlockOrderedList,
			FacetIDs: []string{string(types.FacetCurrentCodePath)},
			Items:    []types.AnswerBlockItem{{ID: "sink", EvidenceIDs: []string{"sink-edge"}}},
			EdgeAnchors: []types.DiagramEdgeAnchor{{
				FromNode: "gate.Run", ToNode: "gate.RunWith", FromIdentity: "Run", ToIdentity: "RunWith", RelationKind: types.DiagramRelCall,
			}},
		},
		{
			ID: "mixed-support", Kind: types.BlockOrderedList,
			Items: []types.AnswerBlockItem{
				{ID: "sink", EvidenceIDs: []string{"sink-edge"}},
				{ID: "other", EvidenceIDs: []string{"other-edge"}},
			},
			EdgeAnchors: []types.DiagramEdgeAnchor{{
				FromNode: "gate.Run", ToNode: "gate.RunWith", FromIdentity: "Run", ToIdentity: "RunWith", RelationKind: types.DiagramRelCall,
			}},
		},
	}}
	raw := projectAnswerDocumentPatchFieldEditTargets(
		BuildAnswerDocumentPatchParametersFor(view), prev, view, []string{"sink-boundary"},
	)
	var root map[string]any
	if err := json.Unmarshal(raw, &root); err != nil {
		t.Fatalf("projected patch schema must parse: %v", err)
	}
	branches := root["properties"].(map[string]any)["block_field_edits_v1"].(map[string]any)["items"].(map[string]any)["oneOf"].([]any)
	found := false
	for _, rawBranch := range branches {
		props := rawBranch.(map[string]any)["properties"].(map[string]any)
		if props["field"].(map[string]any)["const"] != string(types.AnswerBlockFieldAddFacetID) {
			continue
		}
		found = true
		if got := props["block_id"].(map[string]any)["enum"].([]any); !reflect.DeepEqual(got, []any{"sink-boundary"}) {
			t.Fatalf("facet addition targets=%v, want exact unmixed existing edge carrier", got)
		}
		if got := props["value"].(map[string]any)["enum"].([]any); !reflect.DeepEqual(got, []any{string(types.FacetPrincipalPathEdge)}) {
			t.Fatalf("facet addition value=%v, want closed exact ownership value", got)
		}
	}
	if !found {
		t.Fatal("exact metadata-only principal-path facet branch was not projected")
	}
	lease := types.NewAnswerDiagramRelationRepairLease(prev, []types.AnswerDiagramRelationRepairFailure{{
		BlockID: "sink-boundary", Issue: "call_edge_display_repair",
		FromNode: "gate.Run", ToNode: "gate.RunWith", FromIdentity: "Run", ToIdentity: "RunWith",
		RelationKind: types.DiagramRelCall,
	}}, nil)
	if lease == nil {
		t.Fatal("test setup did not create a same-generation relation lease")
	}
	params := &emitAnswerDocumentPatchParams{BlockFieldEditsV1: []types.AnswerBlockFieldEditV1{{
		BlockID: "sink-boundary", Field: types.AnswerBlockFieldAddFacetID, Value: string(types.FacetPrincipalPathEdge),
	}}}
	if violation := localDiagramLeaseWholeBlockMutationViolation(params, lease, prev, view); violation != nil {
		t.Fatalf("metadata-only facet ownership must remain executable on a live relation target: %+v", violation)
	}
	merged, err := types.ApplyAnswerDocumentV2Patch(prev, &types.AnswerDocumentV2Patch{BlockFieldEditsV1: params.BlockFieldEditsV1})
	if err != nil {
		t.Fatalf("metadata-only facet ownership patch failed: %v", err)
	}
	if violations := types.ValidateAnswerDiagramRelationRepairLease(lease, merged); len(violations) != 0 {
		t.Fatalf("unchanged relation topology must satisfy the live lease: %+v", violations)
	}
}

func TestAnswerDocumentPatchFieldEditProjection_OffersMemberSetOnlyForUniqueTypedRoster(t *testing.T) {
	view := &types.AnswerSemanticView{Presentation: types.AnswerPresentationContract{RequestedDimensions: []types.RequestedAnswerDimension{{
		Index: 1, Role: types.RequestedAnswerDimensionMemberSet, Required: true,
	}}}}
	prev := &types.AnswerDocumentV2{DocumentModel: "v2", Blocks: []types.AnswerBlock{
		{ID: "summary", Kind: types.BlockSummary, Text: "model summary"},
		{ID: "roster", Kind: types.BlockBulletList, FacetIDs: []string{string(types.FacetEnumerationItem)},
			ClaimUses: []types.RenderedClaimUse{{ClaimForm: types.ClaimCallEdge, EvidenceID: "ev-a"}}, Items: []types.AnswerBlockItem{
				{ID: "a", EvidenceIDs: []string{"ev-a"}}, {ID: "b", EvidenceIDs: []string{"ev-b"}},
			}},
		{ID: "relation", Kind: types.BlockOrderedList, FacetIDs: []string{string(types.FacetEnumerationItem), string(types.FacetPrincipalPathEdge)},
			Items: []types.AnswerBlockItem{{ID: "edge", EvidenceIDs: []string{"ev-edge"}}}, EdgeAnchors: []types.DiagramEdgeAnchor{{
				FromNode: "A", ToNode: "B", FromIdentity: "A", ToIdentity: "B", RelationKind: types.DiagramRelCall,
			}}},
	}}
	raw := projectAnswerDocumentPatchFieldEditTargets(BuildAnswerDocumentPatchParametersFor(view), prev, view, nil)
	var root map[string]any
	if err := json.Unmarshal(raw, &root); err != nil {
		t.Fatalf("projected patch schema must parse: %v", err)
	}
	branches := root["properties"].(map[string]any)["block_field_edits_v1"].(map[string]any)["items"].(map[string]any)["oneOf"].([]any)
	found := false
	for _, rawBranch := range branches {
		props := rawBranch.(map[string]any)["properties"].(map[string]any)
		if props["field"].(map[string]any)["const"] != string(types.AnswerBlockFieldAddFacetID) {
			continue
		}
		values := props["value"].(map[string]any)["enum"].([]any)
		if !reflect.DeepEqual(values, []any{string(types.FacetMemberSet)}) {
			continue
		}
		found = true
		if got := props["block_id"].(map[string]any)["enum"].([]any); !reflect.DeepEqual(got, []any{"roster"}) {
			t.Fatalf("member_set targets=%v, want the unique typed roster only", got)
		}
	}
	if !found {
		t.Fatal("unique typed roster did not receive an atomic member_set membership branch")
	}
	prev.Blocks[1].ClaimUses[0].EvidenceID = "ev-outside-roster"
	if got := answerDocumentMemberSetFacetAdditionCandidateBlockIDs(prev, view); len(got) != 0 {
		t.Fatalf("relation provenance outside the roster items must fail closed, got %v", got)
	}
	prev.Blocks[1].ClaimUses[0].EvidenceID = "ev-a"
	prev.Blocks = append(prev.Blocks, types.AnswerBlock{ID: "second-roster", Kind: types.BlockTable,
		FacetIDs: []string{string(types.FacetEnumerationItem)}, Items: []types.AnswerBlockItem{{ID: "c", EvidenceIDs: []string{"ev-c"}}}})
	if got := answerDocumentMemberSetFacetAdditionCandidateBlockIDs(prev, view); len(got) != 0 {
		t.Fatalf("ambiguous roster ownership must fail closed, got %v", got)
	}
}

func TestEmitAnswerDocumentPatchParametersFor_OptionalLeasePublishesExactTargetRemoval(t *testing.T) {
	base := atomicPatchTestDocument()
	lease := types.NewAnswerDiagramRelationRepairLeaseWithTargetRemoval(base,
		[]types.AnswerDiagramRelationRepairFailure{{
			BlockID: "diag", Issue: "call_edge_unproven",
			FromNode: "A", ToNode: "B", FromIdentity: "missing", ToIdentity: "carrier",
			RelationKind: types.DiagramRelPrecedence,
		}}, nil, true)
	if lease == nil {
		t.Fatal("test setup: optional target removal must make the otherwise ambiguous generation executable")
	}
	mut := types.NewMutableState("optional local diagram schema")
	mut.SetAnswerDocumentV2WithMutation(types.MutationReplaceAll, base)
	mut.SetAnswerDiagramRelationRepairLease(lease)
	raw := (&EmitAnswerDocumentPatch{}).ParametersFor(&types.AgentContext{Mutable: mut})
	var root map[string]any
	if err := json.Unmarshal(raw, &root); err != nil {
		t.Fatalf("projected patch schema must parse: %v", err)
	}
	props := root["properties"].(map[string]any)
	removeIDs, ok := props["remove_block_ids"].(map[string]any)
	if !ok {
		t.Fatalf("optional lease must expose its exact model-selected removal branch: %v", props)
	}
	items := removeIDs["items"].(map[string]any)
	if got := items["enum"].([]any); !reflect.DeepEqual(got, []any{"diag", "summary"}) {
		t.Fatalf("optional target-removal roster=%v, want exact target plus unrelated summary", got)
	}
	if _, ok := props["add_blocks"]; ok {
		t.Fatal("optional target removal must not reopen arbitrary roster additions")
	}
}

func TestEmitAnswerDocumentPatchParametersFor_BoundaryLeasePublishesLocalBranchesOnly(t *testing.T) {
	base := atomicPatchTestDocument()
	for i := range base.Blocks {
		if base.Blocks[i].ID == "diag" {
			base.Blocks[i].ParticipantBoundaries = []types.DiagramParticipantBoundary{
				{Participant: "Analyzer", Status: types.DiagramParticipantBoundaryUnproven},
				{Participant: "Keep", Status: types.DiagramParticipantBoundaryUnproven},
			}
		}
	}
	lease := types.WithAnswerDiagramParticipantBoundaryRepairFailures(base, nil,
		[]types.AnswerDiagramParticipantBoundaryRepairFailure{
			{BlockID: "diag", Participant: "Analyzer", Issue: "stale_boundary_for_connected_participant"},
			{BlockID: "diag", Participant: "Explorer", Issue: "missing_unproven_boundary"},
		})
	if lease == nil || len(lease.ParticipantBoundaryFailures) != 2 {
		t.Fatalf("test setup: expected two boundary capabilities: %+v", lease)
	}
	mut := types.NewMutableState("boundary schema")
	mut.SetAnswerDocumentV2WithMutation(types.MutationReplaceAll, base)
	mut.SetAnswerDiagramRelationRepairLease(lease)
	raw := (&EmitAnswerDocumentPatch{}).ParametersFor(&types.AgentContext{Mutable: mut})
	var root map[string]any
	if err := json.Unmarshal(raw, &root); err != nil {
		t.Fatalf("projected patch schema must parse: %v", err)
	}
	props := root["properties"].(map[string]any)
	for _, forbidden := range []string{"add_blocks", "diagram_boundary_replacements", "diagram_edge_edits"} {
		if _, exists := props[forbidden]; exists {
			t.Fatalf("boundary-only generation must hide broader capability %q: %v", forbidden, props)
		}
	}
	if got := props["remove_block_ids"].(map[string]any)["items"].(map[string]any)["enum"].([]any); !reflect.DeepEqual(got, []any{"summary"}) {
		t.Fatalf("boundary-only unrelated removal roster=%v", got)
	}
	replaceBlocks, ok := props["replace_blocks"].(map[string]any)
	if !ok {
		t.Fatal("boundary-only generation hid replacement of an unrelated existing block")
	}
	replaceItem := replaceBlocks["items"].(map[string]any)
	replaceProps := replaceItem["properties"].(map[string]any)
	if got := replaceProps["id"].(map[string]any)["enum"].([]any); !reflect.DeepEqual(got, []any{"summary"}) {
		t.Fatalf("boundary-only replacement roster=%v, want only unrelated summary block", got)
	}
	field := props["diagram_boundary_edits"].(map[string]any)
	branches := field["items"].(map[string]any)["oneOf"].([]any)
	if len(branches) != 2 || field["minItems"] != float64(1) || field["maxItems"] != float64(2) {
		t.Fatalf("expected two exact boundary branches: %+v", field)
	}
	want := make(map[string]string)
	for _, failure := range lease.ParticipantBoundaryFailures {
		want[failure.BoundaryRef] = string(failure.AllowedBoundaryActions[0])
	}
	for _, rawBranch := range branches {
		branch := rawBranch.(map[string]any)
		if branch["additionalProperties"] != false {
			t.Fatalf("boundary branch must reject unadvertised coordinates: %+v", branch)
		}
		branchProps := branch["properties"].(map[string]any)
		ref := branchProps["boundary_ref"].(map[string]any)["enum"].([]any)[0].(string)
		action := branchProps["action"].(map[string]any)["enum"].([]any)[0].(string)
		if want[ref] != action {
			t.Fatalf("schema branch is not lease-owned: ref=%q action=%q want=%v", ref, action, want)
		}
	}
}

func TestEmitAnswerDocumentPatchParametersFor_VisibilityLeasePublishesModelAuthoredDeclarationBranch(t *testing.T) {
	base := atomicPatchTestDocument()
	for i := range base.Blocks {
		if base.Blocks[i].ID == "diag" {
			base.Blocks[i].ParticipantBoundaries = []types.DiagramParticipantBoundary{{
				Participant: "BusContext", Status: types.DiagramParticipantBoundaryUnproven,
			}}
		}
	}
	lease := types.WithAnswerDiagramParticipantVisibilityRepairFailures(base, nil,
		[]types.AnswerDiagramParticipantVisibilityRepairFailure{{
			BlockID: "diag", Participant: "BusContext", Issue: "boundary_participant_not_visible",
		}})
	if lease == nil || len(lease.ParticipantVisibilityFailures) != 1 {
		t.Fatalf("test setup: expected one visibility capability: %+v", lease)
	}
	mut := types.NewMutableState("visibility schema")
	mut.SetAnswerDocumentV2WithMutation(types.MutationReplaceAll, base)
	mut.SetAnswerDiagramRelationRepairLease(lease)
	raw := (&EmitAnswerDocumentPatch{}).ParametersFor(&types.AgentContext{Mutable: mut})
	var root map[string]any
	if err := json.Unmarshal(raw, &root); err != nil {
		t.Fatalf("projected patch schema must parse: %v", err)
	}
	props := root["properties"].(map[string]any)
	for _, forbidden := range []string{"add_blocks", "diagram_edge_edits", "diagram_boundary_edits"} {
		if _, exists := props[forbidden]; exists {
			t.Fatalf("visibility-only generation must hide broader capability %q", forbidden)
		}
	}
	if got := props["remove_block_ids"].(map[string]any)["items"].(map[string]any)["enum"].([]any); !reflect.DeepEqual(got, []any{"summary"}) {
		t.Fatalf("visibility-only unrelated removal roster=%v", got)
	}
	field := props["diagram_participant_edits"].(map[string]any)
	branches := field["items"].(map[string]any)["oneOf"].([]any)
	if len(branches) != 1 || field["minItems"] != float64(1) || field["maxItems"] != float64(1) {
		t.Fatalf("expected one exact visibility branch: %+v", field)
	}
	branchProps := branches[0].(map[string]any)["properties"].(map[string]any)
	ref := branchProps["participant_ref"].(map[string]any)["enum"].([]any)[0].(string)
	if ref != lease.ParticipantVisibilityFailures[0].ParticipantRef ||
		branchProps["action"].(map[string]any)["enum"].([]any)[0] != "ensure_visible" {
		t.Fatalf("schema branch is not bound to the live visibility failure: %+v", branchProps)
	}
	if _, ok := branchProps["node_id"]; !ok {
		t.Fatal("model-authored node_id field missing")
	}
	if _, ok := branchProps["visible_label"]; !ok {
		t.Fatal("model-authored visible_label field missing")
	}
}

func TestEmitAnswerDocumentPatchParametersFor_LocalLeaseHidesWholeReplacementWhenOnlyTargetExists(t *testing.T) {
	base := atomicPatchTestDocument()
	base.Blocks = append([]types.AnswerBlock(nil), base.Blocks[1])
	lease := types.NewAnswerDiagramRelationRepairLease(base,
		[]types.AnswerDiagramRelationRepairFailure{{
			BlockID: "diag", Issue: "call_edge_unproven",
			FromNode: "A", ToNode: "B", FromIdentity: "Analyzer", ToIdentity: "Explorer",
			RelationKind: types.DiagramRelPrecedence,
		}}, nil)
	if lease == nil {
		t.Fatal("test setup: expected live local lease")
	}
	mut := types.NewMutableState("target-only schema")
	mut.SetAnswerDocumentV2WithMutation(types.MutationReplaceAll, base)
	mut.SetAnswerDiagramRelationRepairLease(lease)
	raw := (&EmitAnswerDocumentPatch{}).ParametersFor(&types.AgentContext{Mutable: mut})
	var root map[string]any
	if err := json.Unmarshal(raw, &root); err != nil {
		t.Fatalf("projected patch schema must parse: %v", err)
	}
	props := root["properties"].(map[string]any)
	if _, exists := props["replace_blocks"]; exists {
		t.Fatalf("target-only relation lease must not expose whole replacement of its diagram: %v", props)
	}
}

func TestEmitAnswerDocumentPatchParametersFor_LocalLeaseHidesSystemGeneratedUnrelatedBlock(t *testing.T) {
	base := atomicPatchTestDocument()
	base.Blocks[0].SystemGeneratedKind = types.AnswerSystemGeneratedEvidenceSupplement
	lease := types.NewAnswerDiagramRelationRepairLease(base,
		[]types.AnswerDiagramRelationRepairFailure{{
			BlockID: "diag", Issue: "call_edge_unproven",
			FromNode: "A", ToNode: "B", FromIdentity: "Analyzer", ToIdentity: "Explorer",
			RelationKind: types.DiagramRelPrecedence,
		}}, nil)
	if lease == nil {
		t.Fatal("test setup: expected live local lease")
	}
	mut := types.NewMutableState("system-unrelated schema")
	mut.SetAnswerDocumentV2WithMutation(types.MutationReplaceAll, base)
	mut.SetAnswerDiagramRelationRepairLease(lease)
	raw := (&EmitAnswerDocumentPatch{}).ParametersFor(&types.AgentContext{Mutable: mut})
	var root map[string]any
	if err := json.Unmarshal(raw, &root); err != nil {
		t.Fatalf("projected patch schema must parse: %v", err)
	}
	props := root["properties"].(map[string]any)
	if _, exists := props["replace_blocks"]; exists {
		t.Fatalf("system-generated unrelated block must not be exposed for model replacement: %v", props)
	}
}

func TestEmitAnswerDocumentPatchParametersFor_LabelPairLeasePublishesRelabelOnly(t *testing.T) {
	base := atomicPatchTestDocument()
	lease := types.NewAnswerDiagramRelationRepairLease(base,
		[]types.AnswerDiagramRelationRepairFailure{{
			BlockID: "diag", Issue: diagramTypedRecipeMissingVisibleLabel,
			FromNode: "A", ToNode: "B", FromIdentity: "Analyzer", ToIdentity: "Explorer",
			RelationKind: types.DiagramRelPrecedence, BodyOccurrence: 1,
		}}, nil)
	if lease == nil || len(lease.Failures) != 1 ||
		lease.Failures[0].TargetCarrier != types.AnswerDiagramRelationRepairCarrierLabelPair ||
		!lease.Failures[0].AllowsAction("relabel") {
		t.Fatalf("test setup did not produce one label-pair capability: %+v", lease)
	}
	mut := types.NewMutableState("label pair schema")
	mut.SetAnswerDocumentV2WithMutation(types.MutationReplaceAll, base)
	mut.SetAnswerDiagramRelationRepairLease(lease)
	raw := (&EmitAnswerDocumentPatch{}).ParametersFor(&types.AgentContext{Mutable: mut})
	var root map[string]any
	if err := json.Unmarshal(raw, &root); err != nil {
		t.Fatalf("projected patch schema must parse: %v", err)
	}
	props := root["properties"].(map[string]any)
	edits := props["diagram_edge_edits"].(map[string]any)
	branches := edits["items"].(map[string]any)["oneOf"].([]any)
	if len(branches) != 1 || edits["minItems"] != float64(1) || edits["maxItems"] != float64(1) {
		t.Fatalf("presentation-only label-pair lease must expose one relabel branch: %+v", edits)
	}
	seenActions := map[string]bool{}
	for _, rawBranch := range branches {
		branch := rawBranch.(map[string]any)
		branchProps := branch["properties"].(map[string]any)
		actions := branchProps["action"].(map[string]any)["enum"].([]any)
		if len(actions) != 1 {
			t.Fatalf("label-pair branch action=%v", actions)
		}
		action := actions[0].(string)
		seenActions[action] = true
		if got := branchProps["failure_ref"].(map[string]any)["enum"].([]any); !reflect.DeepEqual(got, []any{lease.Failures[0].FailureRef}) {
			t.Fatalf("label-pair ref=%v", got)
		}
		required := map[string]bool{}
		for _, rawField := range branch["required"].([]any) {
			required[rawField.(string)] = true
		}
		if !required["failure_ref"] || !required["action"] || (action == "relabel") != required["visible_label"] {
			t.Fatalf("label-pair branch has wrong required fields: action=%s branch=%+v", action, branch)
		}
		for _, hidden := range []string{"block_id", "match", "occurrence", "body_occurrence", "edge"} {
			if _, exists := branchProps[hidden]; exists {
				t.Fatalf("label-pair branch leaked hidden/legacy field %q: %+v", hidden, branchProps)
			}
		}
	}
	if !seenActions["relabel"] || seenActions["remove"] {
		t.Fatalf("label-pair actions=%v", seenActions)
	}
}

func TestEmitAnswerDocumentPatchParametersFor_AnchoredComponentJoinPublishesReplaceRefOnly(t *testing.T) {
	base := atomicPatchTestDocument()
	lease := types.NewAnswerDiagramRelationRepairLease(base,
		[]types.AnswerDiagramRelationRepairFailure{{
			BlockID: "diag", Issue: diagramParticipantComponentJoinEndpointMappingIssue,
			FromNode: "A", ToNode: "B", FromIdentity: "Analyzer", ToIdentity: "Explorer",
			RelationKind: types.DiagramRelPrecedence, BodyOccurrence: 1,
		}}, nil)
	if lease == nil || len(lease.Failures) != 1 ||
		lease.Failures[0].TargetCarrier != types.AnswerDiagramRelationRepairCarrierPriorAnchor ||
		len(lease.Failures[0].AllowedActions) != 1 ||
		lease.Failures[0].AllowedActions[0] != types.AnswerDiagramRelationRepairActionReplace {
		t.Fatalf("test setup did not produce one replace-only anchored join capability: %+v", lease)
	}
	mut := types.NewMutableState("anchored component join schema")
	mut.SetAnswerDocumentV2WithMutation(types.MutationReplaceAll, base)
	mut.SetAnswerDiagramRelationRepairLease(lease)
	var root map[string]any
	if err := json.Unmarshal((&EmitAnswerDocumentPatch{}).ParametersFor(&types.AgentContext{Mutable: mut}), &root); err != nil {
		t.Fatalf("projected patch schema must parse: %v", err)
	}
	props := root["properties"].(map[string]any)
	edits := props["diagram_edge_edits"].(map[string]any)
	branches := edits["items"].(map[string]any)["oneOf"].([]any)
	if len(branches) != 1 || edits["minItems"] != float64(1) || edits["maxItems"] != float64(1) {
		t.Fatalf("anchored component join lease must expose exactly one branch: %+v", edits)
	}
	branch := branches[0].(map[string]any)
	branchProps := branch["properties"].(map[string]any)
	if got := branchProps["action"].(map[string]any)["enum"].([]any); !reflect.DeepEqual(got, []any{"replace"}) {
		t.Fatalf("anchored component join action=%v", got)
	}
	if got := branchProps["failure_ref"].(map[string]any)["enum"].([]any); !reflect.DeepEqual(got, []any{lease.Failures[0].FailureRef}) {
		t.Fatalf("anchored component join ref=%v", got)
	}
	if _, ok := branchProps["visible_label"]; ok {
		t.Fatalf("replace-only branch must carry reader wording inside edge, not a competing top-level label: %+v", branchProps)
	}
	edge := branchProps["edge"].(map[string]any)
	edgeProps := edge["properties"].(map[string]any)
	if edge["additionalProperties"] != false || len(edgeProps) != 3 {
		t.Fatalf("replacement edge must expose exactly the three model-authored presentation fields: %+v", edge)
	}
	for _, field := range []string{"from_node", "to_node", "visible_label"} {
		if _, ok := edgeProps[field]; !ok {
			t.Fatalf("replace-only branch lost model-owned field %q: %+v", field, edgeProps)
		}
	}
	assertEndpointLabelStateContract(t, branch, "A", "A", "B", "B", "C", "C")
}

func TestEmitAnswerDocumentPatchParametersFor_EndpointLabelStateUsesPendingPatchBase(t *testing.T) {
	accepted := atomicPatchTestDocument()
	staged := atomicPatchTestDocument()
	staged.Blocks[1].Diagram.Body = strings.Replace(
		staged.Blocks[1].Diagram.Body,
		"    participant C\n",
		"    participant C\n    participant BusinessHandler as \"业务处理器\"\n",
		1,
	)
	lease := types.NewAnswerDiagramRelationRepairLease(staged,
		[]types.AnswerDiagramRelationRepairFailure{{
			BlockID: "diag", Issue: diagramParticipantComponentJoinEndpointMappingIssue,
			FromNode: "A", ToNode: "B", FromIdentity: "Analyzer", ToIdentity: "Explorer",
			RelationKind: types.DiagramRelPrecedence, BodyOccurrence: 1,
		}}, nil)
	if lease == nil {
		t.Fatal("test setup did not produce a staged relation lease")
	}
	mut := types.NewMutableState("pending endpoint state")
	mut.SetAnswerDocumentV2WithMutation(types.MutationReplaceAll, accepted)
	mut.SetPendingAnswerDocumentPatchBase(staged)
	mut.SetAnswerDiagramRelationRepairLease(lease)
	var root map[string]any
	if err := json.Unmarshal((&EmitAnswerDocumentPatch{}).ParametersFor(&types.AgentContext{Mutable: mut}), &root); err != nil {
		t.Fatalf("projected patch schema must parse: %v", err)
	}
	branches := root["properties"].(map[string]any)["diagram_edge_edits"].(map[string]any)["items"].(map[string]any)["oneOf"].([]any)
	if len(branches) != 1 {
		t.Fatalf("staged lease must expose one exact branch: %+v", branches)
	}
	assertEndpointLabelStateContract(t, branches[0].(map[string]any),
		"A", "A", "B", "B", "BusinessHandler", "业务处理器", "C", "C")
}

func assertEndpointLabelStateContract(t *testing.T, branch map[string]any, idLabels ...string) {
	t.Helper()
	if len(idLabels)%2 != 0 {
		t.Fatalf("test setup id/label pairs are unbalanced: %v", idLabels)
	}
	allOf, ok := branch["allOf"].([]any)
	if !ok {
		t.Fatalf("edge branch did not publish a state-dependent endpoint-label contract: %+v", branch)
	}
	want := make(map[string]string, len(idLabels)/2)
	for i := 0; i < len(idLabels); i += 2 {
		want[idLabels[i]] = idLabels[i+1]
	}
	seen := map[string]map[string]bool{"from": {}, "to": {}}
	newEndpointRequired := map[string]bool{}
	for _, rawRule := range allOf {
		rule := rawRule.(map[string]any)
		ifNode := rule["if"].(map[string]any)
		edge := ifNode["properties"].(map[string]any)["edge"].(map[string]any)
		edgeProps := edge["properties"].(map[string]any)
		for _, side := range []string{"from", "to"} {
			nodeRule, exists := edgeProps[side+"_node"]
			if !exists {
				continue
			}
			ids := nodeRule.(map[string]any)["enum"].([]any)
			if elseNode, hasElse := rule["else"].(map[string]any); hasElse {
				required := elseNode["required"].([]any)
				newEndpointRequired[side] = reflect.DeepEqual(required, []any{side + "_node_visible_label"}) && len(ids) == len(want)
				continue
			}
			if len(ids) != 1 {
				continue
			}
			id := ids[0].(string)
			labelRule := rule["then"].(map[string]any)["properties"].(map[string]any)[side+"_node_visible_label"].(map[string]any)
			labels := labelRule["enum"].([]any)
			if len(labels) == 1 && labels[0] == want[id] {
				seen[side][id] = true
			}
		}
	}
	for side := range seen {
		if !newEndpointRequired[side] {
			t.Fatalf("%s-side new endpoint did not require a model-authored visible label: %+v", side, allOf)
		}
		for id := range want {
			if !seen[side][id] {
				t.Fatalf("%s-side existing endpoint %q did not permit only its exact current label: %+v", side, id, allOf)
			}
		}
	}
}

func TestEmitAnswerDocumentPatchParametersFor_ExistingBodyPublishesPairedAttachNotDuplicateAdd(t *testing.T) {
	base := &types.AnswerDocumentV2{DocumentModel: "v2", Blocks: []types.AnswerBlock{{
		ID: "flow", Kind: types.BlockDiagram,
		Diagram: &types.AnswerDiagramBlock{Kind: types.DiagramFlow, Language: "mermaid", Body: "flowchart LR\n A -->|model wording| B\n"},
	}}}
	lease := types.NewAnswerDiagramRelationRepairLease(base, []types.AnswerDiagramRelationRepairFailure{{
		BlockID: "flow", Issue: "missing_relation_anchor", FromNode: "A", ToNode: "B",
		FromIdentity: "pkg.A.run", ToIdentity: "pkg.B.accept", BodyOccurrence: 1,
	}}, []types.AnswerDiagramRelationRepairCandidate{{
		BlockID: "flow", RelationKind: types.DiagramRelArgumentFlow,
		FromIdentity: "pkg.A.run", ToIdentity: "pkg.B.accept", Source: "internal/source.go:10",
	}})
	if lease == nil || !lease.Failures[0].AllowsAction("attach") {
		t.Fatalf("test setup missing paired attach capability: %+v", lease)
	}
	mut := types.NewMutableState("paired attach schema")
	mut.SetAnswerDocumentV2WithMutation(types.MutationReplaceAll, base)
	mut.SetAnswerDiagramRelationRepairLease(lease)
	var root map[string]any
	if err := json.Unmarshal((&EmitAnswerDocumentPatch{}).ParametersFor(&types.AgentContext{Mutable: mut}), &root); err != nil {
		t.Fatalf("projected schema must parse: %v", err)
	}
	branches := root["properties"].(map[string]any)["diagram_edge_edits"].(map[string]any)["items"].(map[string]any)["oneOf"].([]any)
	if len(branches) != 2 {
		t.Fatalf("existing body must expose remove+attach only, got %+v", branches)
	}
	seen := map[string]bool{}
	for _, rawBranch := range branches {
		props := rawBranch.(map[string]any)["properties"].(map[string]any)
		action := props["action"].(map[string]any)["enum"].([]any)[0].(string)
		seen[action] = true
		if action != "attach" {
			continue
		}
		if _, ok := props["failure_ref"]; !ok {
			t.Fatalf("attach branch lost exact body selector: %+v", props)
		}
		if _, ok := props["addition_ref"]; !ok {
			t.Fatalf("attach branch lost exact typed candidate selector: %+v", props)
		}
		required := rawBranch.(map[string]any)["required"].([]any)
		if !reflect.DeepEqual(required, []any{"failure_ref", "addition_ref", "action", "edge"}) {
			t.Fatalf("attach branch required fields=%v", required)
		}
	}
	if !seen["remove"] || !seen["attach"] || seen["replace"] || seen["add"] {
		t.Fatalf("paired schema advertised a contradictory or duplicate-producing path: %v", seen)
	}
	description := (&EmitAnswerDocumentPatch{}).DescriptionFor(&types.AgentContext{Mutable: mut})
	if !strings.Contains(description, "exact action=attach schema branch") ||
		!strings.Contains(description, "never infer a pair from adjacent rows") {
		t.Fatalf("paired schema description did not bind attach to its exact branch: %s", description)
	}
}

func TestEmitAnswerDocumentPatchMixedLeaseNarrowsDiagramAndKeepsNonDiagramBlockRepair(t *testing.T) {
	base := &types.AnswerDocumentV2{DocumentModel: "v2", Blocks: []types.AnswerBlock{
		{ID: "summary", Kind: types.BlockSummary, Text: "keep or merge"},
		{ID: "alias", Kind: types.BlockSummary, Text: "remove after merge"},
		{ID: "flow", Kind: types.BlockDiagram, Diagram: &types.AnswerDiagramBlock{
			Kind: types.DiagramFlow, Language: "mermaid", Body: "flowchart LR\n A --> B\n",
		}},
		{ID: "chain", Kind: types.BlockOrderedList, EdgeAnchors: []types.DiagramEdgeAnchor{{
			FromNode: "X", ToNode: "Y", FromIdentity: "pkg.X", ToIdentity: "pkg.Y", RelationKind: types.DiagramRelCall,
		}}},
	}}
	lease := types.NewAnswerDiagramRelationRepairLease(base, []types.AnswerDiagramRelationRepairFailure{
		{BlockID: "flow", Issue: "missing_call_anchor", FromNode: "A", ToNode: "B", BodyOccurrence: 1},
		{BlockID: "chain", Issue: "standalone_relation_endpoint_identity_missing", FromNode: "X", ToNode: "Y", RelationKind: types.DiagramRelCall},
	}, nil)
	if lease == nil {
		t.Fatal("mixed relation lease setup failed")
	}
	mut := types.NewMutableState("mixed exact relation schema")
	mut.SetAnswerDocumentV2WithMutation(types.MutationReplaceAll, base)
	mut.SetAnswerDiagramRelationRepairLease(lease)
	ctx := &types.AgentContext{Mutable: mut}
	var root map[string]any
	if err := json.Unmarshal((&EmitAnswerDocumentPatch{}).ParametersFor(ctx), &root); err != nil {
		t.Fatalf("mixed schema must parse: %v", err)
	}
	props := root["properties"].(map[string]any)
	branches := props["diagram_edge_edits"].(map[string]any)["items"].(map[string]any)["oneOf"].([]any)
	if len(branches) != 1 {
		t.Fatalf("mixed lease must expose only the exact diagram failure branch: %+v", branches)
	}
	branchProps := branches[0].(map[string]any)["properties"].(map[string]any)
	if _, ok := branchProps["failure_ref"]; !ok {
		t.Fatalf("mixed lease fell back to legacy coordinates: %+v", branchProps)
	}
	for _, forbidden := range []string{"block_id", "match", "occurrence", "body_occurrence"} {
		if _, ok := branchProps[forbidden]; ok {
			t.Fatalf("mixed exact branch leaked %q: %+v", forbidden, branchProps)
		}
	}
	replaceIDs := props["replace_blocks"].(map[string]any)["items"].(map[string]any)["properties"].(map[string]any)["id"].(map[string]any)["enum"].([]any)
	if !reflect.DeepEqual(replaceIDs, []any{"alias", "chain", "summary"}) {
		t.Fatalf("mixed lease unrelated replacement roster=%v", replaceIDs)
	}
	removeIDs := props["remove_block_ids"].(map[string]any)["items"].(map[string]any)["enum"].([]any)
	if !reflect.DeepEqual(removeIDs, []any{"alias", "chain", "summary"}) {
		t.Fatalf("mixed lease unrelated removal roster=%v", removeIDs)
	}
	description := (&EmitAnswerDocumentPatch{}).DescriptionFor(ctx)
	if !strings.Contains(description, "current schema is the sole capability authority") ||
		!strings.Contains(description, "only unrelated existing blocks") {
		t.Fatalf("mixed exact description drifted from schema: %s", description)
	}
}

func TestEmitAnswerDocumentPatchParametersFor_StandaloneRelationPublishesExactMetadataAttach(t *testing.T) {
	base := &types.AnswerDocumentV2{DocumentModel: "v2", Blocks: []types.AnswerBlock{
		{ID: "summary", Kind: types.BlockSummary, Text: "keep"},
		{ID: "chain", Kind: types.BlockOrderedList, Title: "model title", EdgeAnchors: []types.DiagramEdgeAnchor{{
			FromNode: "caller", ToNode: "callee", RelationKind: types.DiagramRelCall, VisibleLabel: "model wording",
		}}},
	}}
	lease := types.NewAnswerDiagramRelationRepairLease(base, []types.AnswerDiagramRelationRepairFailure{{
		BlockID: "chain", Issue: diagramStandaloneRelationIdentityMissing,
		FromNode: "caller", ToNode: "callee", RelationKind: types.DiagramRelCall,
		TargetCarrier: types.AnswerDiagramRelationRepairCarrierPriorAnchorMetadata,
	}}, []types.AnswerDiagramRelationRepairCandidate{{
		BlockID: "chain", RelationKind: types.DiagramRelCall,
		FromIdentity: "pkg.Caller.run", ToIdentity: "pkg.Callee.accept", Source: "src/call.go:10",
	}})
	if lease == nil || len(lease.Failures) != 1 || len(lease.AllowedAdditions) != 1 ||
		!lease.Failures[0].AllowsAction("attach") {
		t.Fatalf("standalone metadata attach lease setup failed: %+v", lease)
	}
	mut := types.NewMutableState("standalone metadata attach schema")
	mut.SetAnswerDocumentV2WithMutation(types.MutationReplaceAll, base)
	mut.SetAnswerDiagramRelationRepairLease(lease)
	ctx := &types.AgentContext{Mutable: mut}
	var root map[string]any
	if err := json.Unmarshal((&EmitAnswerDocumentPatch{}).ParametersFor(ctx), &root); err != nil {
		t.Fatalf("projected schema must parse: %v", err)
	}
	props := root["properties"].(map[string]any)
	branches := props["diagram_edge_edits"].(map[string]any)["items"].(map[string]any)["oneOf"].([]any)
	if len(branches) != 2 {
		t.Fatalf("standalone metadata row must expose exact remove and typed attach branches: %+v", branches)
	}
	seen := map[string]map[string]any{}
	for _, rawBranch := range branches {
		branch := rawBranch.(map[string]any)
		branchProps := branch["properties"].(map[string]any)
		action := branchProps["action"].(map[string]any)["enum"].([]any)[0].(string)
		seen[action] = branch
	}
	attach := seen["attach"]
	if attach == nil || !reflect.DeepEqual(attach["required"], []any{"failure_ref", "addition_ref", "action"}) {
		t.Fatalf("metadata attach branch must require only the two exact refs and model-selected action: %+v", attach)
	}
	if _, ok := attach["properties"].(map[string]any)["edge"]; ok {
		t.Fatalf("metadata attach must preserve the existing visible row instead of asking the model to replay it: %+v", attach)
	}
	replaceIDs := props["replace_blocks"].(map[string]any)["items"].(map[string]any)["properties"].(map[string]any)["id"].(map[string]any)["enum"].([]any)
	if !reflect.DeepEqual(replaceIDs, []any{"summary"}) {
		t.Fatalf("atomic standalone carrier must be excluded from whole replacement roster: %v", replaceIDs)
	}
	removeIDs := props["remove_block_ids"].(map[string]any)["items"].(map[string]any)["enum"].([]any)
	if !reflect.DeepEqual(removeIDs, []any{"summary"}) {
		t.Fatalf("optional diagram removal must not widen to a standalone relation carrier: %v", removeIDs)
	}
}

func TestLocalDiagramLeaseSchemaPublishesOnlyMissingTypedRequiredBlockAddition(t *testing.T) {
	base := &types.AnswerDocumentV2{DocumentModel: "v2", Blocks: []types.AnswerBlock{
		{ID: "flow", Kind: types.BlockDiagram, Diagram: &types.AnswerDiagramBlock{
			Kind: types.DiagramFlow, Language: "mermaid", Body: "flowchart LR\n A --> B\n",
		}},
		{ID: "chain", Kind: types.BlockOrderedList, Text: "model-authored path"},
	}}
	lease := types.NewAnswerDiagramRelationRepairLeaseWithTargetRemoval(base,
		[]types.AnswerDiagramRelationRepairFailure{{
			BlockID: "flow", Issue: "missing_call_anchor", FromNode: "A", ToNode: "B", BodyOccurrence: 1,
		}}, nil, true)
	if lease == nil {
		t.Fatal("missing-required mixed lease setup failed")
	}
	view := &types.AnswerSemanticView{RequiredBlocks: []types.BlockRequirement{
		{Kind: types.BlockSummary, MinCount: 1, MaxCount: 1, Required: true},
		{Kind: types.BlockOrderedList, MinCount: 1, MaxCount: 1, Required: true},
	}}
	raw := narrowAnswerDocumentPatchParametersForLocalDiagramLease(
		BuildAnswerDocumentPatchParametersFor(view), lease, base, view,
	)
	var root map[string]any
	if err := json.Unmarshal(raw, &root); err != nil {
		t.Fatalf("missing-required mixed schema must parse: %v", err)
	}
	props := root["properties"].(map[string]any)
	addBlocks, ok := props["add_blocks"].(map[string]any)
	if !ok {
		t.Fatal("typed summary deficit must remain executable alongside the diagram lease")
	}
	if addBlocks["minItems"] != float64(1) || addBlocks["maxItems"] != float64(1) {
		t.Fatalf("typed addition capacity=%+v, want exactly one missing carrier", addBlocks)
	}
	kinds := addBlocks["items"].(map[string]any)["properties"].(map[string]any)["kind"].(map[string]any)["enum"].([]any)
	if !reflect.DeepEqual(kinds, []any{"summary"}) {
		t.Fatalf("typed addition kinds=%v, want only missing summary", kinds)
	}
}

func TestEmitAnswerDocumentPatchParametersFor_NoLeaseHidesGenerationScopedCapabilities(t *testing.T) {
	mut := types.NewMutableState("retry without a relation lease")
	raw := (&EmitAnswerDocumentPatch{}).ParametersFor(&types.AgentContext{Mutable: mut})
	var root map[string]any
	if err := json.Unmarshal(raw, &root); err != nil {
		t.Fatalf("no-lease patch schema must parse: %v", err)
	}
	props := root["properties"].(map[string]any)
	for _, field := range []string{"replace_blocks", "add_blocks", "remove_block_ids"} {
		if _, ok := props[field]; !ok {
			t.Fatalf("no-lease compatibility schema lost executable whole mutation %q", field)
		}
	}
	if _, ok := props["diagram_participant_edits"]; ok {
		t.Fatal("no-lease schema must not advertise generation-scoped participant cleanup")
	}
	edgeItem := props["diagram_edge_edits"].(map[string]any)["items"].(map[string]any)
	edgeProps := edgeItem["properties"].(map[string]any)
	for _, field := range []string{"failure_ref", "addition_ref"} {
		if _, ok := edgeProps[field]; ok {
			t.Fatalf("no-lease schema leaked generation-scoped selector %q", field)
		}
	}
	if _, ok := edgeItem["anyOf"]; ok {
		t.Fatalf("no-lease schema retained an opaque-selector alternative: %+v", edgeItem)
	}
	if got := edgeItem["required"].([]any); !reflect.DeepEqual(got, []any{"block_id", "action"}) {
		t.Fatalf("no-lease edge edit must require executable legacy coordinates, got %v", got)
	}
	for _, field := range []string{"block_id", "match", "edge"} {
		if _, ok := edgeProps[field]; !ok {
			t.Fatalf("no-lease schema lost executable model-authored field %q", field)
		}
	}
	matchProps := edgeProps["match"].(map[string]any)["properties"].(map[string]any)
	for _, field := range []string{"from_identity", "to_identity", "relation_kind"} {
		if _, ok := matchProps[field]; !ok {
			t.Fatalf("legacy compatibility match lost %q", field)
		}
	}
	desc := (&EmitAnswerDocumentPatch{}).DescriptionFor(&types.AgentContext{Mutable: mut})
	if strings.Contains(desc, "failure_ref") || strings.Contains(desc, "addition_ref") ||
		!strings.Contains(desc, "identify an existing block") {
		t.Fatalf("no-lease description contradicted its executable schema: %q", desc)
	}
}

func TestEmitAnswerDocumentPatchParametersFor_AdditionOnlyLeaseDropsUnavailableCleanupSurface(t *testing.T) {
	base := atomicPatchTestDocument()
	lease := types.NewAnswerDiagramRelationRepairLease(base, nil, []types.AnswerDiagramRelationRepairCandidate{{
		BlockID: "diag", RelationKind: types.DiagramRelCall,
		FromIdentity: "Extractor", ToIdentity: "Finalizer", Source: "internal/orchestrator/topology.go:1",
	}})
	mut := types.NewMutableState("addition-only local diagram schema")
	mut.SetAnswerDocumentV2WithMutation(types.MutationReplaceAll, base)
	mut.SetAnswerDiagramRelationRepairLease(lease)
	var root map[string]any
	if err := json.Unmarshal((&EmitAnswerDocumentPatch{}).ParametersFor(&types.AgentContext{Mutable: mut}), &root); err != nil {
		t.Fatalf("projected patch schema must parse: %v", err)
	}
	props := root["properties"].(map[string]any)
	if _, ok := props["diagram_participant_edits"]; ok {
		t.Fatal("lease without an orphan-cleanup candidate must not advertise participant edits")
	}
	branches := props["diagram_edge_edits"].(map[string]any)["items"].(map[string]any)["oneOf"].([]any)
	if len(branches) != 1 {
		t.Fatalf("addition-only lease must expose exactly one executable branch: %+v", branches)
	}
}

func TestEmitAnswerDocumentPatchParametersFor_NonDiagramLeaseKeepsCompatibilitySurface(t *testing.T) {
	base := atomicPatchTestDocument()
	base.Blocks = append(base.Blocks, types.AnswerBlock{
		ID: "list", Kind: types.BlockOrderedList,
		EdgeAnchors: []types.DiagramEdgeAnchor{{
			FromNode: "X", ToNode: "Y", FromIdentity: "X.run", ToIdentity: "Y.run", RelationKind: types.DiagramRelCall,
		}},
	})
	lease := types.NewAnswerDiagramRelationRepairLease(base, []types.AnswerDiagramRelationRepairFailure{{
		BlockID: "list", Issue: "typed_anchor_without_visible_edge",
		FromNode: "X", ToNode: "Y", FromIdentity: "X.run", ToIdentity: "Y.run", RelationKind: types.DiagramRelCall,
	}}, nil)
	mut := types.NewMutableState("non-diagram relation schema")
	mut.SetAnswerDocumentV2WithMutation(types.MutationReplaceAll, base)
	mut.SetAnswerDiagramRelationRepairLease(lease)
	var root map[string]any
	if err := json.Unmarshal((&EmitAnswerDocumentPatch{}).ParametersFor(&types.AgentContext{Mutable: mut}), &root); err != nil {
		t.Fatalf("compatibility patch schema must parse: %v", err)
	}
	props := root["properties"].(map[string]any)
	for _, field := range []string{"replace_blocks", "add_blocks", "remove_block_ids"} {
		if _, ok := props[field]; !ok {
			t.Fatalf("non-diagram/mixed lease must retain compatibility field %q", field)
		}
	}
	description := (&EmitAnswerDocumentPatch{}).DescriptionFor(&types.AgentContext{Mutable: mut})
	for _, want := range []string{
		"executable compatibility operations shown in this tool's current parameter schema",
		"failure_ref only with an action listed in that row",
		"broad compatibility schema publishes no paired attach branch",
		"Whole-block edits remain available",
	} {
		if !strings.Contains(description, want) {
			t.Fatalf("non-diagram compatibility description missing %q: %s", want, description)
		}
	}
	if strings.Contains(description, "action=attach pair that binds both refs") {
		t.Fatalf("broad compatibility description advertised an unavailable attach operation: %s", description)
	}
}

func TestEmitAnswerDocumentSchema_CandidateRoleEnumMatchesTypes(t *testing.T) {
	raw := (&EmitAnswerDocument{}).Parameters()
	var root map[string]any
	if err := json.Unmarshal(raw, &root); err != nil {
		t.Fatalf("schema must parse: %v", err)
	}
	props := root["properties"].(map[string]any)
	blocks := props["blocks"].(map[string]any)
	blockItems := blocks["items"].(map[string]any)
	blockProps := blockItems["properties"].(map[string]any)
	itemsField := blockProps["items"].(map[string]any)
	itemNode := itemsField["items"].(map[string]any)
	itemProps := itemNode["properties"].(map[string]any)
	roleNode := itemProps["candidate_role"].(map[string]any)
	roleDescription, _ := roleNode["description"].(string)
	if !strings.Contains(roleDescription, "not a generic entity type") ||
		!strings.Contains(roleDescription, "threads, processes, CPUs, frames, or spans") {
		t.Fatalf("candidate_role schema must tell runtime rows to omit the source/inventory role field: %q", roleDescription)
	}
	enum := roleNode["enum"].([]any)
	want := types.AllAnswerCandidateRoles()
	if len(enum) != len(want) {
		t.Fatalf("candidate_role enum len=%d want=%d (%v)", len(enum), len(want), enum)
	}
	for i, role := range want {
		if enum[i] != string(role) {
			t.Fatalf("candidate_role enum[%d]=%v want %q (full=%v)", i, enum[i], role, enum)
		}
	}
}

func TestEmitAnswerDocumentSchema_ClaimAndDiagramEnumsMatchTypes(t *testing.T) {
	_, blockProps := answerDocumentProjectedBlockSchema(t, (&EmitAnswerDocument{}).Parameters())

	claimUses := blockProps["claim_uses"].(map[string]any)
	claimItem := claimUses["items"].(map[string]any)
	claimProps := claimItem["properties"].(map[string]any)
	claimEnum := claimProps["claim_form"].(map[string]any)["enum"].([]any)
	wantClaims := types.AllClaimForms()
	if len(claimEnum) != len(wantClaims) {
		t.Fatalf("claim_form enum len=%d want=%d (%v)", len(claimEnum), len(wantClaims), claimEnum)
	}
	for i, form := range wantClaims {
		if claimEnum[i] != string(form) {
			t.Fatalf("claim_form enum[%d]=%v want %q (full=%v)", i, claimEnum[i], form, claimEnum)
		}
	}

	edgeAnchors := blockProps["edge_anchors"].(map[string]any)
	edgeItem := edgeAnchors["items"].(map[string]any)
	edgeProps := edgeItem["properties"].(map[string]any)
	relationEnum := edgeProps["relation_kind"].(map[string]any)["enum"].([]any)
	wantRelations := types.AllDiagramRelationKinds()
	if len(relationEnum) != len(wantRelations) {
		t.Fatalf("relation_kind enum len=%d want=%d (%v)", len(relationEnum), len(wantRelations), relationEnum)
	}
	for i, relation := range wantRelations {
		if relationEnum[i] != string(relation) {
			t.Fatalf("relation_kind enum[%d]=%v want %q (full=%v)", i, relationEnum[i], relation, relationEnum)
		}
	}
	if _, leaked := edgeProps["claim_form"]; leaked {
		t.Fatalf("edge_anchors must expose relation_kind as the sole typed relation authority; claim_form leaked: %v", edgeProps)
	}
	for _, identityField := range []string{"from_identity", "to_identity"} {
		if _, ok := edgeProps[identityField]; !ok {
			t.Fatalf("edge_anchors must expose optional typed endpoint selector %q: %v", identityField, edgeProps)
		}
	}
	if _, ok := edgeProps["visible_label"]; !ok {
		t.Fatalf("edge_anchors must expose the model-authored standalone visibility field: %v", edgeProps)
	}
	required := edgeItem["required"].([]any)
	for _, want := range []string{"from_node", "to_node", "relation_kind"} {
		found := false
		for _, got := range required {
			if got == want {
				found = true
				break
			}
		}
		if !found {
			t.Fatalf("edge_anchors required=%v missing %q", required, want)
		}
	}
}

func TestBuildAnswerDocumentParametersFor_ProjectedClaimEnumIsSoleAvailabilityTeaching(t *testing.T) {
	view := &types.AnswerSemanticView{RequiredBlocks: []types.BlockRequirement{{
		Kind:                 types.BlockOrderedList,
		Required:             true,
		MinCount:             1,
		MaxCount:             1,
		AcceptableClaimForms: []types.ClaimForm{types.ClaimDefinitionFact},
	}}}
	_, blockProps := answerDocumentProjectedBlockSchema(t, BuildAnswerDocumentParametersFor(view))
	claimUses := blockProps["claim_uses"].(map[string]any)
	description, _ := claimUses["description"].(string)
	if !strings.Contains(description, "sibling projected claim_form.enum is the sole availability authority") {
		t.Fatalf("claim teaching must identify the projected enum as the only availability source: %q", description)
	}
	for _, absentForm := range []string{"text_reference_fact", "literal_value_fact", "callback_handoff"} {
		if strings.Contains(description, absentForm) {
			t.Fatalf("projected description must not advertise unavailable form %q: %q", absentForm, description)
		}
	}
	claimItem := claimUses["items"].(map[string]any)
	claimProps := claimItem["properties"].(map[string]any)
	enum := claimProps["claim_form"].(map[string]any)["enum"].([]any)
	if len(enum) != 1 || enum[0] != string(types.ClaimDefinitionFact) {
		t.Fatalf("projected claim enum=%v, want only definition_fact", enum)
	}
}

func TestEmitAnswerDocumentSchema_SourceDiagramEdgeOwnershipUsesTypedSingleSource(t *testing.T) {
	raw := string((&EmitAnswerDocument{}).Parameters())
	if !strings.Contains(raw, types.GroundedSourceDiagramEdgeOwnershipContract) {
		t.Fatalf("canonical schema missing source-diagram edge ownership contract: %s", raw)
	}
	if !strings.Contains(raw, types.GroundedSourceDiagramRelationEvidenceContract) {
		t.Fatalf("canonical schema missing strict relation-evidence contract: %s", raw)
	}
	if strings.Contains(raw, "Omit edge_anchors when no typed edge is needed; outside strict grounded call-chain contracts") {
		t.Fatalf("canonical schema leaked the pre-B217 narrower contract: %s", raw)
	}
}

func TestBuildAnswerDocumentParametersFor_ProjectsTypedParticipantBoundariesOnlyForRequiredFlowSlate(t *testing.T) {
	active := &types.AnswerSemanticView{
		RelationAxis: types.AxisFlow,
		DiagramPlan:  &types.DiagramFacetGraph{Kind: types.DiagramFlow, Required: true},
		DiagramParticipantObligations: []types.DiagramParticipantHint{
			{Identity: "Analyzer", Role: types.DiagramParticipantIncidentRequired},
			{Identity: "BusContext", Role: types.DiagramParticipantContextOnly},
		},
	}
	blockItems, props := answerDocumentProjectedBlockSchema(t, BuildAnswerDocumentParametersFor(active))
	field, ok := props["participant_boundaries"].(map[string]any)
	if !ok {
		t.Fatalf("active typed flow participant contract must expose participant_boundaries: %v", props)
	}
	if desc, _ := field["description"].(string); !strings.Contains(desc, "Analyzer") || strings.Contains(desc, "BusContext") || !strings.Contains(desc, "sibling of diagram") || !strings.Contains(desc, "NEVER put") {
		t.Fatalf("projected boundary description must name incident-only roster: %q", desc)
	}
	encoded, _ := json.Marshal(blockItems["allOf"])
	if strings.Contains(string(encoded), "participant_boundaries") {
		t.Fatalf("all-covered diagrams must not pay a redundant empty-array presence gate: %s", encoded)
	}

	for name, view := range map[string]*types.AnswerSemanticView{
		"no obligations": {RelationAxis: types.AxisFlow, DiagramPlan: &types.DiagramFacetGraph{Kind: types.DiagramFlow, Required: true}},
		"non flow":       {RelationAxis: types.AxisDefine, DiagramPlan: &types.DiagramFacetGraph{Kind: types.DiagramFlow, Required: true}, DiagramParticipantObligations: active.DiagramParticipantObligations},
		"optional":       {RelationAxis: types.AxisFlow, DiagramPlan: &types.DiagramFacetGraph{Kind: types.DiagramFlow, Required: false}, DiagramParticipantObligations: active.DiagramParticipantObligations},
	} {
		t.Run(name, func(t *testing.T) {
			_, projected := answerDocumentProjectedBlockSchema(t, BuildAnswerDocumentParametersFor(view))
			if _, leaked := projected["participant_boundaries"]; leaked {
				t.Fatalf("inactive contract leaked participant_boundaries: %v", projected)
			}
		})
	}
}

func TestBuildAnswerDocumentParametersFor_ProjectsRequestedRelationScopeOnlyForMultiParticipantFlow(t *testing.T) {
	active := &types.AnswerSemanticView{
		Family:       types.QFGeneric,
		RelationAxis: types.AxisFlow,
		DiagramPlan:  &types.DiagramFacetGraph{Kind: types.DiagramArchitecture, Required: true},
		DiagramParticipantObligations: []types.DiagramParticipantHint{
			{Identity: "Analyzer", Role: types.DiagramParticipantIncidentRequired},
			{Identity: "Mutable", Role: types.DiagramParticipantIncidentRequired},
		},
	}
	_, activeProps := answerDocumentProjectedBlockSchema(t, BuildAnswerDocumentParametersFor(active))
	field, ok := activeProps["requested_relation_scope"].(map[string]any)
	if !ok {
		t.Fatalf("required multi-participant source-flow schema must expose requested_relation_scope: %v", activeProps)
	}
	if desc, _ := field["description"].(string); !strings.Contains(desc, "requested_relation_spine_status=unproven") ||
		!strings.Contains(desc, "exactly one") {
		t.Fatalf("projected requested relation scope teaching is incomplete: %q", desc)
	}

	for name, view := range map[string]*types.AnswerSemanticView{
		"trace":           {Family: types.QFRootCauseTrace, RelationAxis: types.AxisFlow, DiagramPlan: active.DiagramPlan, DiagramParticipantObligations: active.DiagramParticipantObligations},
		"one participant": {Family: types.QFGeneric, RelationAxis: types.AxisFlow, DiagramPlan: active.DiagramPlan, DiagramParticipantObligations: active.DiagramParticipantObligations[:1]},
		"non flow":        {Family: types.QFGeneric, RelationAxis: types.AxisDefine, DiagramPlan: active.DiagramPlan, DiagramParticipantObligations: active.DiagramParticipantObligations},
	} {
		t.Run(name, func(t *testing.T) {
			_, props := answerDocumentProjectedBlockSchema(t, BuildAnswerDocumentParametersFor(view))
			if _, leaked := props["requested_relation_scope"]; leaked {
				t.Fatalf("inactive lane leaked requested_relation_scope: %v", props)
			}
		})
	}
}

func TestBuildAnswerDocumentParametersFor_ProjectsSourceInventoryIdentityOnlyWhenAvailable(t *testing.T) {
	assertFields := func(t *testing.T, view *types.AnswerSemanticView, want bool) {
		t.Helper()
		_, blockProps := answerDocumentProjectedBlockSchema(t, BuildAnswerDocumentParametersFor(view))
		_, familyPresent := blockProps["source_inventory_family"]
		items := blockProps["items"].(map[string]any)
		itemSchema := items["items"].(map[string]any)
		itemProps := itemSchema["properties"].(map[string]any)
		_, rowIDPresent := itemProps["source_inventory_row_id"]
		if familyPresent != want || rowIDPresent != want {
			t.Fatalf("source-inventory identity projection family=%v row_id=%v want=%v", familyPresent, rowIDPresent, want)
		}
	}

	assertFields(t, &types.AnswerSemanticView{}, false)
	assertFields(t, &types.AnswerSemanticView{SourceInventoryRowIdentityAvailable: true}, true)
}

func TestBuildAnswerDocumentParametersFor_ProjectsItemEvidenceIdentityOnlyForCurrentSource(t *testing.T) {
	assertField := func(t *testing.T, view *types.AnswerSemanticView, want bool) {
		t.Helper()
		_, blockProps := answerDocumentProjectedBlockSchema(t, BuildAnswerDocumentParametersFor(view))
		items := blockProps["items"].(map[string]any)
		itemSchema := items["items"].(map[string]any)
		itemProps := itemSchema["properties"].(map[string]any)
		_, present := itemProps["evidence_ids"]
		if present != want {
			t.Fatalf("item evidence identity projection present=%v want=%v props=%v", present, want, itemProps)
		}
	}

	assertField(t, &types.AnswerSemanticView{}, false)
	assertField(t, &types.AnswerSemanticView{ItemEvidenceIdentityAvailable: true}, true)
	// Source-inventory identity alone must not widen the independent generic
	// current-source evidence selector.
	assertField(t, &types.AnswerSemanticView{SourceInventoryRowIdentityAvailable: true}, false)
}

func TestEmitAnswerDocumentSchema_ErrorGranularityVerdictEnumMatchesTypes(t *testing.T) {
	raw := (&EmitAnswerDocument{}).Parameters()
	var root map[string]any
	if err := json.Unmarshal(raw, &root); err != nil {
		t.Fatalf("schema must parse: %v", err)
	}
	props := root["properties"].(map[string]any)
	blocks := props["blocks"].(map[string]any)
	blockItems := blocks["items"].(map[string]any)
	blockProps := blockItems["properties"].(map[string]any)
	node := blockProps["error_granularity_verdict"].(map[string]any)
	enum := node["enum"].([]any)
	want := types.AllErrorGranularityVerdicts()
	if len(enum) != len(want) {
		t.Fatalf("error_granularity_verdict enum len=%d want=%d (%v)", len(enum), len(want), enum)
	}
	for i, verdict := range want {
		if enum[i] != string(verdict) {
			t.Fatalf("error_granularity_verdict enum[%d]=%v want %q (full=%v)", i, enum[i], verdict, enum)
		}
	}
}

func TestEmitAnswerDocumentSchema_CurrentStatusVerdictEnumMatchesTypes(t *testing.T) {
	raw := (&EmitAnswerDocument{}).Parameters()
	var root map[string]any
	if err := json.Unmarshal(raw, &root); err != nil {
		t.Fatalf("schema must parse: %v", err)
	}
	props := root["properties"].(map[string]any)
	blocks := props["blocks"].(map[string]any)
	blockItems := blocks["items"].(map[string]any)
	blockProps := blockItems["properties"].(map[string]any)
	node := blockProps["current_status_verdict"].(map[string]any)
	enum := node["enum"].([]any)
	want := types.AllCurrentStatusVerdicts()
	if len(enum) != len(want) {
		t.Fatalf("current_status_verdict enum len=%d want=%d (%v)", len(enum), len(want), enum)
	}
	for i, verdict := range want {
		if enum[i] != string(verdict) {
			t.Fatalf("current_status_verdict enum[%d]=%v want %q (full=%v)", i, enum[i], verdict, enum)
		}
	}
}

func TestEmitAnswerDocumentSchema_TraceCausalClaimCaliberEnumMatchesTypes(t *testing.T) {
	_, blockProps := answerDocumentProjectedBlockSchema(t, (&EmitAnswerDocument{}).Parameters())
	node := blockProps["trace_causal_claim_caliber"].(map[string]any)
	enum := node["enum"].([]any)
	want := types.AllTraceCausalClaimCalibers()
	if len(enum) != len(want) {
		t.Fatalf("trace_causal_claim_caliber enum len=%d want=%d (%v)", len(enum), len(want), enum)
	}
	for i, caliber := range want {
		if enum[i] != string(caliber) {
			t.Fatalf("trace_causal_claim_caliber enum[%d]=%v want %q (full=%v)", i, enum[i], caliber, enum)
		}
	}
}

func TestEmitAnswerDocumentSchema_SectionItemsAreNativeStructuredCitationCarrier(t *testing.T) {
	_, blockProps := answerDocumentProjectedBlockSchema(t, (&EmitAnswerDocument{}).Parameters())
	kindDescription, _ := blockProps["kind"].(map[string]any)["description"].(string)
	itemsDescription, _ := blockProps["items"].(map[string]any)["description"].(string)
	for _, want := range []string{
		"section uses block.text for narrative",
		"may also use block.items[] for structured or cited rows",
	} {
		if !strings.Contains(kindDescription, want) {
			t.Fatalf("block kind JSON teaching missing %q: %s", want, kindDescription)
		}
	}
	if !strings.Contains(itemsDescription, "Block items for section / ordered_list / bullet_list / table") {
		t.Fatalf("items JSON teaching omitted section carrier: %s", itemsDescription)
	}
	if strings.Contains(kindDescription, "summary/section/scalar/decision/caveat use block.text") {
		t.Fatalf("block kind JSON teaching retained the ambiguous section=text-only grouping: %s", kindDescription)
	}
}

// TestBuildAnswerDocumentParametersFor_EnumerationDropsDiagramAndAbsence
// — an enumeration family with no diagram and no missing requested
// roles must drop edge_anchors, diagram, exact_resolution, and
// missing_requested_roles entirely.
func TestBuildAnswerDocumentParametersFor_EnumerationDropsDiagramAndAbsence(t *testing.T) {
	view := &types.AnswerSemanticView{
		Family: types.QFEnumeration,
		RequiredBlocks: []types.BlockRequirement{
			{Kind: types.BlockSummary, Required: true},
			{Kind: types.BlockOrderedList, Required: true,
				AcceptableClaimForms: []types.ClaimForm{types.ClaimDefinitionFact}},
		},
	}
	got := BuildAnswerDocumentParametersFor(view)
	s := string(got)
	for _, want := range []string{`"summary"`, `"ordered_list"`, `"definition_fact"`} {
		if !strings.Contains(s, want) {
			t.Errorf("projected schema must keep %q; got:\n%s", want, s)
		}
	}
	for _, banned := range []string{`"missing_requested_roles"`, `"exact_resolution"`, `"edge_anchors"`} {
		if strings.Contains(s, banned) {
			t.Errorf("enumeration view must drop %q; got:\n%s", banned, s)
		}
	}
	// diagram block payload must also disappear — view has no plan.
	if strings.Contains(s, `"diagram":{`) || strings.Contains(s, `"diagram": {`) {
		t.Errorf("enumeration view must drop diagram payload; got:\n%s", s)
	}
}

// TestBuildAnswerDocumentParametersFor_ConfigPrecedenceKeepsMissingRoles
// — a config-precedence family with non-empty MissingRequestedRoles
// must keep the missing_requested_roles surface.
func TestBuildAnswerDocumentParametersFor_ConfigPrecedenceKeepsMissingRoles(t *testing.T) {
	view := &types.AnswerSemanticView{
		Family: types.QFConfigPrecedence,
		MissingRequestedRoles: []types.AnswerMissingRequestedRole{
			{Role: types.EvidenceDiagramRoleOverride, Label: "CLI"},
		},
		ExactResolution: &types.ExactResolutionContract{},
	}
	got := BuildAnswerDocumentParametersFor(view)
	s := string(got)
	if !strings.Contains(s, `"missing_requested_roles"`) {
		t.Errorf("config-precedence view must keep missing_requested_roles; got:\n%s", s)
	}
	if !strings.Contains(s, `"exact_resolution"`) {
		t.Errorf("non-nil ExactResolution must keep exact_resolution; got:\n%s", s)
	}
	// no diagram → still drops edge_anchors and diagram payload.
	if strings.Contains(s, `"edge_anchors"`) {
		t.Errorf("config-precedence view (no diagram) must drop edge_anchors; got:\n%s", s)
	}
}

func TestBuildAnswerDocumentParametersFor_SuppressedExactResolutionStaysInternal(t *testing.T) {
	view := &types.AnswerSemanticView{
		Family:                               types.QFConfigPrecedence,
		ExactResolution:                      &types.ExactResolutionContract{Targets: []string{"max_visits"}},
		SuppressExactResolutionAnswerSurface: true,
	}
	s := string(BuildAnswerDocumentParametersFor(view))
	if strings.Contains(s, `"exact_resolution"`) {
		t.Fatalf("internally retained exact contract must not leak into the model answer schema when its answer surface is suppressed:\n%s", s)
	}
}

// TestBuildAnswerDocumentParametersFor_ArchitectureKeepsDiagramAndPinsKind
// — an architecture family with a DiagramFacetGraph must keep
// diagram + edge_anchors and pin the diagram.kind enum to the
// declared family.
func TestBuildAnswerDocumentParametersFor_ArchitectureKeepsDiagramAndPinsKind(t *testing.T) {
	view := &types.AnswerSemanticView{
		Family: types.QFArchitecture,
		RequiredBlocks: []types.BlockRequirement{
			{Kind: types.BlockSummary, Required: true},
			{Kind: types.BlockDiagram, Required: true},
		},
		DiagramPlan: &types.DiagramFacetGraph{
			Required: true,
			Kind:     types.DiagramArchitecture,
		},
	}
	got := BuildAnswerDocumentParametersFor(view)
	var root map[string]any
	if err := json.Unmarshal(got, &root); err != nil {
		t.Fatalf("projected schema must parse: %v", err)
	}
	props := root["properties"].(map[string]any)
	blocks := props["blocks"].(map[string]any)
	bItems := blocks["items"].(map[string]any)
	bProps := bItems["properties"].(map[string]any)
	if _, ok := bProps["edge_anchors"]; !ok {
		t.Errorf("architecture view must keep edge_anchors")
	}
	diagram, ok := bProps["diagram"].(map[string]any)
	if !ok {
		t.Fatalf("architecture view must keep diagram payload")
	}
	dProps := diagram["properties"].(map[string]any)
	kind := dProps["kind"].(map[string]any)
	enum := kind["enum"].([]any)
	if len(enum) != 1 || enum[0] != "architecture" {
		t.Errorf("diagram.kind enum must pin to architecture; got %v", enum)
	}
}

func TestBuildAnswerDocumentParametersFor_ExplicitDiagramKindOverridesFamilyDefault(t *testing.T) {
	view := types.BuildAnswerSemanticView(&types.AnalysisIR{
		RequestModel: types.RequestModel{
			Intent:   types.IntentExplain,
			Scenario: types.ScenarioArchitectureExplain,
		},
	}, &types.AnswerSurfacePlan{
		Diagram: &types.DiagramContract{
			Required:       true,
			RequiredKind:   types.DiagramSequence,
			PreferredKinds: []types.DiagramKind{types.DiagramSequence},
		},
	})
	if view == nil || view.DiagramPlan == nil {
		t.Fatalf("semantic view must keep required diagram plan: %+v", view)
	}
	if view.DiagramPlan.Kind != types.DiagramSequence {
		t.Fatalf("semantic view diagram kind=%q, want %q", view.DiagramPlan.Kind, types.DiagramSequence)
	}
	got := BuildAnswerDocumentParametersFor(view)
	var root map[string]any
	if err := json.Unmarshal(got, &root); err != nil {
		t.Fatalf("projected schema must parse: %v", err)
	}
	props := root["properties"].(map[string]any)
	blocks := props["blocks"].(map[string]any)
	bItems := blocks["items"].(map[string]any)
	bProps := bItems["properties"].(map[string]any)
	diagram := bProps["diagram"].(map[string]any)
	dProps := diagram["properties"].(map[string]any)
	kind := dProps["kind"].(map[string]any)
	enum := kind["enum"].([]any)
	if len(enum) != 1 || enum[0] != "sequence" {
		t.Errorf("diagram.kind enum must preserve explicit sequence request; got %v", enum)
	}
}

// TestBuildAnswerDocumentParametersFor_PerKindPayloadConditionals
// pins the if/then conditionals that teach the LLM each kind's
// required payload field. Pre-fix only kind=diagram had a hard
// reject for missing payload (and the customer reported diagram
// payload as a frequent retry source); other kinds silently
// shipped broken-empty renders. Now every allowed kind carries an
// allOf entry mapping kind=X to required=[id, kind, <payload>].
func TestBuildAnswerDocumentParametersFor_PerKindPayloadConditionals(t *testing.T) {
	view := &types.AnswerSemanticView{
		Family: types.QFArchitecture,
		RequiredBlocks: []types.BlockRequirement{
			{Kind: types.BlockSummary, Required: true},
			{Kind: types.BlockSection, Required: true},
			{Kind: types.BlockOrderedList, Required: true},
			{Kind: types.BlockDiagram, Required: true},
		},
		DiagramPlan: &types.DiagramFacetGraph{Required: true, Kind: types.DiagramArchitecture},
	}
	got := BuildAnswerDocumentParametersFor(view)
	var root map[string]any
	if err := json.Unmarshal(got, &root); err != nil {
		t.Fatalf("schema must parse: %v", err)
	}
	props := root["properties"].(map[string]any)
	blocks := props["blocks"].(map[string]any)
	bItems := blocks["items"].(map[string]any)
	allOf, ok := bItems["allOf"].([]any)
	if !ok {
		t.Fatalf("blocks.items must carry allOf conditionals; got %+v", bItems)
	}
	wantPayloads := map[string]string{
		"summary":      "text",
		"section":      "text",
		"ordered_list": "items",
		"diagram":      "diagram",
	}
	gotPayloads := make(map[string]string, len(allOf))
	for _, c := range allOf {
		entry := c.(map[string]any)
		ifNode, _ := entry["if"].(map[string]any)
		ifProps, _ := ifNode["properties"].(map[string]any)
		kindNode, _ := ifProps["kind"].(map[string]any)
		kind, _ := kindNode["const"].(string)
		thenNode, _ := entry["then"].(map[string]any)
		req, _ := thenNode["required"].([]any)
		if kind == "" || len(req) == 0 {
			continue
		}
		// Find the payload field — it is the third entry after "id"
		// and "kind" in the canonical order.
		for _, r := range req {
			s := r.(string)
			if s != "id" && s != "kind" {
				gotPayloads[kind] = s
			}
		}
	}
	if len(gotPayloads) != len(wantPayloads) {
		t.Errorf("expected one conditional per allowed kind; got %v", gotPayloads)
	}
	for k, want := range wantPayloads {
		if got := gotPayloads[k]; got != want {
			t.Errorf("kind=%q want payload=%q got %q", k, want, got)
		}
	}
}

func TestBuildAnswerDocumentParametersFor_TableDoesNotForceItemsPayload(t *testing.T) {
	view := &types.AnswerSemanticView{
		Family: types.QFComparison,
		RequiredBlocks: []types.BlockRequirement{
			{Kind: types.BlockSummary, Required: true},
			{Kind: types.BlockTable, Required: true},
		},
	}
	got := BuildAnswerDocumentParametersFor(view)
	var root map[string]any
	if err := json.Unmarshal(got, &root); err != nil {
		t.Fatalf("schema must parse: %v", err)
	}
	props := root["properties"].(map[string]any)
	blocks := props["blocks"].(map[string]any)
	bItems := blocks["items"].(map[string]any)
	if schemaBlockKindRequiresField(bItems, "table", "items") {
		t.Fatalf("table blocks must not force items[]; markdown block.text and columns/cells are valid carriers: %+v", bItems["allOf"])
	}
	blockProps := bItems["properties"].(map[string]any)
	if _, ok := blockProps["columns"]; !ok {
		t.Fatalf("projected table schema should expose optional columns[]")
	}
	items := blockProps["items"].(map[string]any)
	itemProps := items["items"].(map[string]any)["properties"].(map[string]any)
	if _, ok := itemProps["cells"]; !ok {
		t.Fatalf("projected table schema should expose optional items[].cells[]")
	}
}

func TestBuildAnswerDocumentParametersFor_SourceInventoryPrincipalTableRequiresItems(t *testing.T) {
	view := &types.AnswerSemanticView{
		Family:                              types.QFEnumeration,
		SourceInventoryRowIdentityAvailable: true,
		RequiredBlocks: []types.BlockRequirement{
			{Kind: types.BlockSummary, Required: true},
			{Kind: types.BlockTable, Required: true},
		},
	}
	got := BuildAnswerDocumentParametersFor(view)
	var root map[string]any
	if err := json.Unmarshal(got, &root); err != nil {
		t.Fatalf("schema must parse: %v", err)
	}
	props := root["properties"].(map[string]any)
	blocks := props["blocks"].(map[string]any)
	bItems := blocks["items"].(map[string]any)

	found := false
	for _, raw := range schemaAllOfEntries(bItems) {
		ifNode, _ := raw["if"].(map[string]any)
		ifProps, _ := ifNode["properties"].(map[string]any)
		kindNode, _ := ifProps["kind"].(map[string]any)
		roleNode, _ := ifProps["surface_role"].(map[string]any)
		if kindNode["const"] != "table" || roleNode["const"] != "principal" {
			continue
		}
		thenNode, _ := raw["then"].(map[string]any)
		required, _ := thenNode["required"].([]any)
		for _, field := range required {
			if field == "items" {
				found = true
			}
		}
	}
	if !found {
		t.Fatalf("typed source-inventory principal table must require row-local items[] sidecars: %+v", bItems["allOf"])
	}
}

func TestBuildAnswerDocumentParametersFor_RequiredBlockCardinalityAndTypedDecision(t *testing.T) {
	view := &types.AnswerSemanticView{
		Family: types.QFRootCauseTrace,
		RequiredBlocks: []types.BlockRequirement{
			{Kind: types.BlockSummary, MinCount: 1, MaxCount: 1, Required: true},
			{Kind: types.BlockDecision, MinCount: 1, MaxCount: 1, Required: true},
		},
		CurrentStatusDiagnostic: &types.CurrentStatusDiagnosticContract{
			Required:        true,
			AllowedVerdicts: []types.CurrentStatusVerdict{types.CurrentStatusStillPresent, types.CurrentStatusFixed},
		},
	}
	got := BuildAnswerDocumentParametersFor(view)
	var root map[string]any
	if err := json.Unmarshal(got, &root); err != nil {
		t.Fatalf("projected schema must parse: %v", err)
	}
	props := root["properties"].(map[string]any)
	blocks := props["blocks"].(map[string]any)
	if !schemaArrayHasKindCardinality(blocks, "summary", 1, 1) {
		t.Fatalf("blocks[] schema must require exactly one summary; got %+v", blocks["allOf"])
	}
	if !schemaArrayHasKindCardinality(blocks, "decision", 1, 1) {
		t.Fatalf("blocks[] schema must require exactly one decision; got %+v", blocks["allOf"])
	}
	blockItems := blocks["items"].(map[string]any)
	blockProps := blockItems["properties"].(map[string]any)
	if _, ok := blockProps["error_granularity_verdict"]; ok {
		t.Fatalf("inactive error_granularity_verdict should be projected out")
	}
	current := blockProps["current_status_verdict"].(map[string]any)
	enum := current["enum"].([]any)
	if len(enum) != 2 || enum[0] != "still_present" || enum[1] != "fixed" {
		t.Fatalf("current_status_verdict enum should be narrowed to allowed verdicts, got %v", enum)
	}
	if !schemaBlockKindRequiresField(blockItems, "decision", "current_status_verdict") {
		t.Fatalf("decision block conditional must require current_status_verdict; got %+v", blockItems["allOf"])
	}
}

func TestBuildAnswerDocumentParametersForProjectsTraceCausalClaimCaliberOnlyWhenActive(t *testing.T) {
	active := &types.AnswerSemanticView{
		RequiredBlocks: []types.BlockRequirement{{Kind: types.BlockSummary, Required: true, MinCount: 1, MaxCount: 1}},
		TraceCausalClaimContract: &types.TraceCausalClaimContract{
			Allowed: []types.TraceCausalClaimCaliber{
				types.TraceCausalClaimNoConclusion,
				types.TraceCausalClaimBoundedWindow,
			},
			Ceiling: types.TraceCausalClaimBoundedWindow,
		},
	}
	blockItems, blockProps := answerDocumentProjectedBlockSchema(t, BuildAnswerDocumentParametersFor(active))
	node, ok := blockProps["trace_causal_claim_caliber"].(map[string]any)
	if !ok {
		t.Fatalf("active Trace causal contract must expose the caliber field: %+v", blockProps)
	}
	enum, _ := node["enum"].([]any)
	if len(enum) != 2 || enum[0] != string(types.TraceCausalClaimNoConclusion) ||
		enum[1] != string(types.TraceCausalClaimBoundedWindow) {
		t.Fatalf("Trace causal caliber enum did not preserve the typed ceiling: %+v", enum)
	}
	description, _ := node["description"].(string)
	for _, want := range []string{
		"Allowed for this dispatch: no_causal_conclusion, bounded_window_candidate.",
		"kind: \"summary\"",
		"surface_role: \"principal\"",
		"invalid on every other block kind, including `section`",
		"Use no_causal_conclusion only when the principal summary makes no cause or candidate attribution.",
		"Use bounded_window_candidate when the summary names or ranks selected-window candidates",
		"Evidence-status values such as unproven are not enum values for this field.",
		"raw enum literals are control metadata only",
		"selected literal stays only in the JSON field",
		"user-facing prose states its meaning in the answer language",
		"You choose the conclusion and caliber",
	} {
		if !strings.Contains(description, want) {
			t.Fatalf("Trace causal caliber dynamic JSON teaching missing %q: %s", want, description)
		}
	}
	if strings.Contains(description, "Use typed_chain_cause") || strings.Contains(description, "Use typed_frame_cause") {
		t.Fatalf("dynamic JSON teaching must not advertise values removed from this dispatch enum: %s", description)
	}
	if !schemaBlockKindRequiresField(blockItems, "summary", "trace_causal_claim_caliber") {
		t.Fatalf("active principal summary schema must require trace_causal_claim_caliber: %+v", blockItems["allOf"])
	}
	if !schemaPrincipalBlockKindRequiresField(blockItems, "summary", "trace_causal_claim_caliber") {
		t.Fatalf("Trace causal caliber must be required only by the principal-summary conditional: %+v", blockItems["allOf"])
	}
	if !schemaPrincipalBlockKindForbidsFieldElsewhere(blockItems, "summary", "trace_causal_claim_caliber") {
		t.Fatalf("Trace causal caliber must be schema-forbidden on every non-principal-summary block: %+v", blockItems["allOf"])
	}

	inactive := &types.AnswerSemanticView{RequiredBlocks: active.RequiredBlocks}
	_, blockProps = answerDocumentProjectedBlockSchema(t, BuildAnswerDocumentParametersFor(inactive))
	if _, ok := blockProps["trace_causal_claim_caliber"]; ok {
		t.Fatalf("non-Trace/narrow answer schema must not expose a causal claim carrier: %+v", blockProps)
	}
}

func TestBuildAnswerDocumentParametersFor_TraceCaliberExecutableSchemaRejectsNonPrincipalOwner(t *testing.T) {
	view := &types.AnswerSemanticView{
		RequiredBlocks: []types.BlockRequirement{
			{Kind: types.BlockSummary, Required: true, MinCount: 1, MaxCount: 1},
			{Kind: types.BlockSection, Required: true, MinCount: 1},
		},
		TraceCausalClaimContract: &types.TraceCausalClaimContract{
			Allowed: []types.TraceCausalClaimCaliber{types.TraceCausalClaimBoundedWindow},
			Ceiling: types.TraceCausalClaimBoundedWindow,
		},
	}
	schema := BuildAnswerDocumentParametersFor(view)
	valid := json.RawMessage(`{"blocks":[{"id":"lead","kind":"summary","surface_role":"principal","text":"bounded model conclusion","trace_causal_claim_caliber":"bounded_window_candidate"},{"id":"detail","kind":"section","surface_role":"principal","text":"model detail"}]}`)
	if err := toolparam.Validate(valid, schema); err != nil {
		t.Fatalf("principal-summary-only caliber shape must satisfy the executable schema: %v", err)
	}
	invalid := json.RawMessage(`{"blocks":[{"id":"lead","kind":"summary","surface_role":"principal","text":"bounded model conclusion","trace_causal_claim_caliber":"bounded_window_candidate"},{"id":"detail","kind":"section","surface_role":"principal","text":"model detail","trace_causal_claim_caliber":"bounded_window_candidate"}]}`)
	if err := toolparam.Validate(invalid, schema); err == nil {
		t.Fatal("projected schema admitted trace_causal_claim_caliber on a section that the runtime must reject")
	}
}

func TestBuildAnswerDocumentParametersFor_SourceOptionalRuntimeViewDropsCurrentStatusVerdict(t *testing.T) {
	view := &types.AnswerSemanticView{
		Family: types.QFRootCauseTrace,
		RequiredBlocks: []types.BlockRequirement{
			{Kind: types.BlockSummary, MinCount: 1, MaxCount: 1, Required: true},
			{Kind: types.BlockOrderedList, MinCount: 1, Required: true},
		},
		OptionalBlocks: []types.BlockRequirement{
			{Kind: types.BlockDecision, Required: false},
		},
	}
	got := BuildAnswerDocumentParametersFor(view)
	var root map[string]any
	if err := json.Unmarshal(got, &root); err != nil {
		t.Fatalf("projected schema must parse: %v", err)
	}
	props := root["properties"].(map[string]any)
	blocks := props["blocks"].(map[string]any)
	blockItems := blocks["items"].(map[string]any)
	blockProps := blockItems["properties"].(map[string]any)
	if _, ok := blockProps["current_status_verdict"]; ok {
		t.Fatalf("source-optional runtime view must not expose current_status_verdict property: %+v", blockProps["current_status_verdict"])
	}
	if schemaBlockKindRequiresField(blockItems, "decision", "current_status_verdict") {
		t.Fatalf("source-optional runtime view must not require current_status_verdict; allOf=%+v", blockItems["allOf"])
	}
}

func TestBuildAnswerDocumentParametersFor_AlternativeKindsWidenKindEnumAndCardinality(t *testing.T) {
	view := &types.AnswerSemanticView{
		Family: types.QFEnumeration,
		RequiredBlocks: []types.BlockRequirement{
			{Kind: types.BlockSummary, MinCount: 1, Required: true},
			{
				Kind:             types.BlockOrderedList,
				AlternativeKinds: []types.AnswerBlockKind{types.BlockTable, types.BlockBulletList},
				MinCount:         1,
				Required:         true,
			},
		},
	}
	got := BuildAnswerDocumentParametersFor(view)
	var root map[string]any
	if err := json.Unmarshal(got, &root); err != nil {
		t.Fatalf("projected schema must parse: %v", err)
	}
	props := root["properties"].(map[string]any)
	blocks := props["blocks"].(map[string]any)
	bItems := blocks["items"].(map[string]any)
	bProps := bItems["properties"].(map[string]any)
	kind := bProps["kind"].(map[string]any)
	enum := kind["enum"].([]any)
	for _, want := range []string{"ordered_list", "table", "bullet_list"} {
		found := false
		for _, got := range enum {
			if got == want {
				found = true
				break
			}
		}
		if !found {
			t.Fatalf("alternative carrier %q missing from block.kind enum %v", want, enum)
		}
	}
	if !schemaArrayHasAnyKindCardinality(blocks, []string{"ordered_list", "table", "bullet_list"}, 1, 0) {
		t.Fatalf("blocks[] schema must accept one of the alternative carriers; got %+v", blocks["allOf"])
	}
}

// TestBuildAnswerDocumentParametersFor_BlockKindEnumRestricted
// confirms block.kind is narrowed to the kinds the view declares.
func TestBuildAnswerDocumentParametersFor_BlockKindEnumRestricted(t *testing.T) {
	view := &types.AnswerSemanticView{
		Family: types.QFRoleLookup,
		RequiredBlocks: []types.BlockRequirement{
			{Kind: types.BlockSummary, Required: true},
			{Kind: types.BlockScalar, Required: true},
		},
		OptionalBlocks: []types.BlockRequirement{
			{Kind: types.BlockSection},
			{Kind: types.BlockCaveat},
		},
	}
	got := BuildAnswerDocumentParametersFor(view)
	var root map[string]any
	if err := json.Unmarshal(got, &root); err != nil {
		t.Fatalf("projected schema must parse: %v", err)
	}
	props := root["properties"].(map[string]any)
	blocks := props["blocks"].(map[string]any)
	bItems := blocks["items"].(map[string]any)
	bProps := bItems["properties"].(map[string]any)
	kind := bProps["kind"].(map[string]any)
	enum := kind["enum"].([]any)
	if len(enum) != 4 {
		t.Errorf("expected 4 kinds (summary/section/scalar/caveat); got %v", enum)
	}
	for _, banned := range []string{"ordered_list", "bullet_list", "diagram", "table", "decision"} {
		for _, e := range enum {
			if e == banned {
				t.Errorf("kind %q must be projected away; full enum: %v", banned, enum)
			}
		}
	}
}

func TestBuildAnswerDocumentParametersFor_PresentationAllowedBlocksWidenKindEnum(t *testing.T) {
	view := &types.AnswerSemanticView{
		Family: types.QFRoleLookup,
		RequiredBlocks: []types.BlockRequirement{
			{Kind: types.BlockSummary, Required: true},
			{Kind: types.BlockScalar, Required: true},
		},
		OptionalBlocks: []types.BlockRequirement{
			{Kind: types.BlockSection},
			{Kind: types.BlockCaveat},
		},
		Presentation: types.AnswerPresentationContract{
			AllowedBlocks: []types.AnswerBlockKind{types.BlockTable, types.BlockDecision, types.BlockDiagram},
		},
	}
	got := BuildAnswerDocumentParametersFor(view)
	var root map[string]any
	if err := json.Unmarshal(got, &root); err != nil {
		t.Fatalf("projected schema must parse: %v", err)
	}
	props := root["properties"].(map[string]any)
	blocks := props["blocks"].(map[string]any)
	bItems := blocks["items"].(map[string]any)
	bProps := bItems["properties"].(map[string]any)
	kind := bProps["kind"].(map[string]any)
	enum := kind["enum"].([]any)
	for _, want := range []string{"summary", "section", "scalar", "decision", "table", "caveat"} {
		found := false
		for _, got := range enum {
			if got == want {
				found = true
				break
			}
		}
		if !found {
			t.Fatalf("presentation block kind %q missing from projected enum %v", want, enum)
		}
	}
	for _, got := range enum {
		if got == "diagram" {
			t.Fatalf("diagram must not be schema-allowed without a DiagramPlan, got enum %v", enum)
		}
	}
	if schemaBlockKindRequiresField(bItems, "decision", "text") == false {
		t.Fatalf("presentation decision block should still receive payload conditional; got %+v", bItems["allOf"])
	}
	if schemaBlockKindRequiresField(bItems, "table", "items") {
		t.Fatalf("presentation table block must preserve markdown-table/text carriers; got %+v", bItems["allOf"])
	}
	if claimUses, ok := bProps["claim_uses"].(map[string]any); ok {
		if claimItems, ok := claimUses["items"].(map[string]any); ok {
			claimProps, _ := claimItems["properties"].(map[string]any)
			if claimForm, ok := claimProps["claim_form"].(map[string]any); ok {
				if enum, ok := claimForm["enum"].([]any); ok && len(enum) != len(types.AllClaimForms()) {
					t.Fatalf("presentation-only carriers must not inherit scalar-only claim_form narrowing; got %v", enum)
				}
			}
		}
	}
}

func schemaArrayHasKindCardinality(blocks map[string]any, kind string, min, max int) bool {
	allOf, _ := blocks["allOf"].([]any)
	for _, raw := range allOf {
		entry, _ := raw.(map[string]any)
		contains, _ := entry["contains"].(map[string]any)
		props, _ := contains["properties"].(map[string]any)
		kindNode, _ := props["kind"].(map[string]any)
		if kindNode["const"] != kind {
			continue
		}
		return intFromSchemaNumber(entry["minContains"]) == min &&
			intFromSchemaNumber(entry["maxContains"]) == max
	}
	return false
}

func schemaArrayHasAnyKindCardinality(blocks map[string]any, kinds []string, min, max int) bool {
	want := map[string]bool{}
	for _, kind := range kinds {
		want[kind] = true
	}
	allOf, _ := blocks["allOf"].([]any)
	for _, raw := range allOf {
		entry, _ := raw.(map[string]any)
		contains, _ := entry["contains"].(map[string]any)
		props, _ := contains["properties"].(map[string]any)
		kindNode, _ := props["kind"].(map[string]any)
		enumRaw, _ := kindNode["enum"].([]any)
		if len(enumRaw) != len(want) {
			continue
		}
		matches := true
		for _, rawKind := range enumRaw {
			kind, _ := rawKind.(string)
			if !want[kind] {
				matches = false
				break
			}
		}
		if !matches {
			continue
		}
		return intFromSchemaNumber(entry["minContains"]) == min &&
			intFromSchemaNumber(entry["maxContains"]) == max
	}
	return false
}

func schemaBlockKindRequiresField(blockItems map[string]any, kind, field string) bool {
	allOf, _ := blockItems["allOf"].([]any)
	for _, raw := range allOf {
		entry, _ := raw.(map[string]any)
		ifNode, _ := entry["if"].(map[string]any)
		ifProps, _ := ifNode["properties"].(map[string]any)
		kindNode, _ := ifProps["kind"].(map[string]any)
		if kindNode["const"] != kind {
			continue
		}
		thenNode, _ := entry["then"].(map[string]any)
		required, _ := thenNode["required"].([]any)
		for _, req := range required {
			if req == field {
				return true
			}
		}
	}
	return false
}

func schemaPrincipalBlockKindRequiresField(blockItems map[string]any, kind, field string) bool {
	allOf, _ := blockItems["allOf"].([]any)
	for _, raw := range allOf {
		entry, _ := raw.(map[string]any)
		ifNode, _ := entry["if"].(map[string]any)
		ifRequired, _ := ifNode["required"].([]any)
		if len(ifRequired) != 2 || ifRequired[0] != "kind" || ifRequired[1] != "surface_role" {
			continue
		}
		ifProps, _ := ifNode["properties"].(map[string]any)
		kindNode, _ := ifProps["kind"].(map[string]any)
		roleNode, _ := ifProps["surface_role"].(map[string]any)
		if kindNode["const"] != kind || roleNode["const"] != string(types.SurfacePrincipal) {
			continue
		}
		thenNode, _ := entry["then"].(map[string]any)
		required, _ := thenNode["required"].([]any)
		for _, req := range required {
			if req == field {
				return true
			}
		}
	}
	return false
}

func schemaPrincipalBlockKindForbidsFieldElsewhere(blockItems map[string]any, kind, field string) bool {
	allOf, _ := blockItems["allOf"].([]any)
	for _, raw := range allOf {
		entry, _ := raw.(map[string]any)
		ifNode, _ := entry["if"].(map[string]any)
		ifProps, _ := ifNode["properties"].(map[string]any)
		kindNode, _ := ifProps["kind"].(map[string]any)
		roleNode, _ := ifProps["surface_role"].(map[string]any)
		if kindNode["const"] != kind || roleNode["const"] != string(types.SurfacePrincipal) {
			continue
		}
		elseNode, _ := entry["else"].(map[string]any)
		properties, _ := elseNode["properties"].(map[string]any)
		if allowed, exists := properties[field]; exists && allowed == false {
			return true
		}
	}
	return false
}

func intFromSchemaNumber(v any) int {
	switch n := v.(type) {
	case float64:
		return int(n)
	case int:
		return n
	default:
		return 0
	}
}
