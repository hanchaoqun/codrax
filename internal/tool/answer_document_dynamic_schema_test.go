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
