package tool

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/types"
)

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
