package tool

import (
	"encoding/json"
	"strings"
	"testing"

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

func TestEmitAnswerDocumentSchema_SourceDiagramEdgeOwnershipUsesTypedSingleSource(t *testing.T) {
	raw := string((&EmitAnswerDocument{}).Parameters())
	if !strings.Contains(raw, types.GroundedSourceDiagramEdgeOwnershipContract) {
		t.Fatalf("canonical schema missing source-diagram edge ownership contract: %s", raw)
	}
	if strings.Contains(raw, "Omit edge_anchors when no typed edge is needed; outside strict grounded call-chain contracts") {
		t.Fatalf("canonical schema leaked the pre-B217 narrower contract: %s", raw)
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
		ifNode := entry["if"].(map[string]any)
		ifProps := ifNode["properties"].(map[string]any)
		kindNode := ifProps["kind"].(map[string]any)
		kind := kindNode["const"].(string)
		thenNode := entry["then"].(map[string]any)
		req := thenNode["required"].([]any)
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

	inactive := &types.AnswerSemanticView{RequiredBlocks: active.RequiredBlocks}
	_, blockProps = answerDocumentProjectedBlockSchema(t, BuildAnswerDocumentParametersFor(inactive))
	if _, ok := blockProps["trace_causal_claim_caliber"]; ok {
		t.Fatalf("non-Trace/narrow answer schema must not expose a causal claim carrier: %+v", blockProps)
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
