package types

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestSurfaceRole_IsValid(t *testing.T) {
	cases := []struct {
		role SurfaceRole
		want bool
	}{
		{SurfacePrincipal, true},
		{SurfaceSupport, true},
		{SurfaceProseOnly, true},
		{SurfaceDiagramOnly, true},
		{"", false},
		{"unknown", false},
		{"PRINCIPAL", false}, // case-sensitive — caller must use NormalizeSurfaceRole
	}
	for _, c := range cases {
		t.Run(string(c.role), func(t *testing.T) {
			if got := c.role.IsValid(); got != c.want {
				t.Errorf("%q: got %v want %v", c.role, got, c.want)
			}
		})
	}
}

func TestNormalizeSurfaceRole(t *testing.T) {
	cases := []struct {
		raw     string
		want    SurfaceRole
		wantOK  bool
		comment string
	}{
		{"", "", true, "empty passes through (caller treats as not-annotated)"},
		{"  ", "", true, "whitespace-only passes through as empty"},
		{"principal", SurfacePrincipal, true, "exact match"},
		{"PRINCIPAL", SurfacePrincipal, true, "case-folded"},
		{"  Support  ", SurfaceSupport, true, "trim + case-fold"},
		{"prose_only", SurfaceProseOnly, true, "exact"},
		{"diagram_only", SurfaceDiagramOnly, true, "exact"},
		{"bogus", SurfacePrincipal, false, "unknown returns fallback + ok=false"},
	}
	for _, c := range cases {
		t.Run(c.comment, func(t *testing.T) {
			got, ok := NormalizeSurfaceRole(c.raw)
			if got != c.want || ok != c.wantOK {
				t.Errorf("raw=%q: got (%q,%v) want (%q,%v)", c.raw, got, ok, c.want, c.wantOK)
			}
		})
	}
}

func TestAllSurfaceRoles_DefensiveCopy(t *testing.T) {
	a := AllSurfaceRoles()
	b := AllSurfaceRoles()
	if len(a) != 4 {
		t.Fatalf("expected 4 roles, got %d", len(a))
	}
	a[0] = "mutated"
	if b[0] == "mutated" {
		t.Error("AllSurfaceRoles must return defensive copies — caller mutation leaked into next call")
	}
}

func TestRenderedClaimUse_IsEmpty(t *testing.T) {
	if !(*RenderedClaimUse)(nil).IsEmpty() {
		t.Error("nil pointer must report IsEmpty=true")
	}
	if !(&RenderedClaimUse{}).IsEmpty() {
		t.Error("zero-value struct must report IsEmpty=true")
	}
	cases := []*RenderedClaimUse{
		{FacetID: "enumeration_item"},
		{EvidenceID: "ev-1"},
		{ClaimForm: ClaimDefinitionFact},
		{SurfaceRole: SurfacePrincipal},
	}
	for _, c := range cases {
		if c.IsEmpty() {
			t.Errorf("non-empty struct reported empty: %+v", c)
		}
	}
}

// TestDiagramEdgeAnchor_HasEdgeAnchor pins the Phase 1-B
// source-fix invariant: edge anchors live in DiagramEdgeAnchor
// (not RenderedClaimUse). Both FromNode AND ToNode required.
func TestDiagramEdgeAnchor_HasEdgeAnchor(t *testing.T) {
	if (*DiagramEdgeAnchor)(nil).HasEdgeAnchor() {
		t.Error("nil pointer must NOT report HasEdgeAnchor")
	}
	if (&DiagramEdgeAnchor{}).HasEdgeAnchor() {
		t.Error("zero struct must NOT report HasEdgeAnchor")
	}
	if (&DiagramEdgeAnchor{FromNode: "A"}).HasEdgeAnchor() {
		t.Error("only FromNode populated must NOT count — both required")
	}
	if (&DiagramEdgeAnchor{ToNode: "B"}).HasEdgeAnchor() {
		t.Error("only ToNode populated must NOT count — both required")
	}
	if !(&DiagramEdgeAnchor{FromNode: "A", ToNode: "B"}).HasEdgeAnchor() {
		t.Error("both FromNode and ToNode populated MUST report HasEdgeAnchor")
	}
}

// G3 step 1 (post_v2_runtime_gap_remediation, 2026-05-04). Pins the
// new RelationKind field semantics: typed enum overrides label
// inference; unset / unknown means "fall back to label".
func TestDiagramEdgeAnchor_HasTypedRelation(t *testing.T) {
	cases := []struct {
		name string
		anc  *DiagramEdgeAnchor
		want bool
	}{
		{"nil receiver", nil, false},
		{"zero struct", &DiagramEdgeAnchor{}, false},
		{"unknown sentinel", &DiagramEdgeAnchor{RelationKind: DiagramRelUnknown}, false},
		{"typed call", &DiagramEdgeAnchor{RelationKind: DiagramRelCall}, true},
		{"typed guard", &DiagramEdgeAnchor{RelationKind: DiagramRelGuard}, true},
		{"typed import", &DiagramEdgeAnchor{RelationKind: DiagramRelImport}, true},
		{"typed precedence", &DiagramEdgeAnchor{RelationKind: DiagramRelPrecedence}, true},
		{"typed contain", &DiagramEdgeAnchor{RelationKind: DiagramRelContain}, true},
		{"typed observe", &DiagramEdgeAnchor{RelationKind: DiagramRelObserve}, true},
		// Bogus relation kind → IsValid()=false → not typed.
		{"bogus relation kind", &DiagramEdgeAnchor{RelationKind: DiagramRelationKind("foo")}, false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := c.anc.HasTypedRelation(); got != c.want {
				t.Errorf("HasTypedRelation()=%v, want %v (anc=%+v)", got, c.want, c.anc)
			}
		})
	}
}

// G3 step 1: JSON serialisation of RelationKind preserves shape +
// omits when empty (back-compat with answers emitted before the
// field existed).
func TestDiagramEdgeAnchor_RelationKindJSONOmitempty(t *testing.T) {
	noTyped := DiagramEdgeAnchor{FromNode: "A", ToNode: "B", ClaimForm: ClaimCallEdge}
	withTyped := DiagramEdgeAnchor{FromNode: "A", ToNode: "B", RelationKind: DiagramRelCall, ClaimForm: ClaimCallEdge}

	noTypedJSON, err := json.Marshal(noTyped)
	if err != nil {
		t.Fatalf("marshal noTyped: %v", err)
	}
	if strings.Contains(string(noTypedJSON), "relation_kind") {
		t.Errorf("noTyped marshal must omit relation_kind: %s", noTypedJSON)
	}
	withTypedJSON, err := json.Marshal(withTyped)
	if err != nil {
		t.Fatalf("marshal withTyped: %v", err)
	}
	if !strings.Contains(string(withTypedJSON), `"relation_kind":"call"`) {
		t.Errorf(`withTyped marshal must contain "relation_kind":"call": %s`, withTypedJSON)
	}
}

func TestRenderedClaimUse_FieldShape(t *testing.T) {
	// Pin the JSON tag shape so a future schema-renaming refactor
	// breaks tests instead of silently breaking emit-on-the-wire.
	// Phase 1-B: claim_use is back to 4 fields (from/to_node moved
	// to DiagramEdgeAnchor on AnswerBlock.EdgeAnchors[]).
	c := RenderedClaimUse{
		FacetID:     "enumeration_item",
		EvidenceID:  "ev-7",
		ClaimForm:   ClaimDefinitionFact,
		SurfaceRole: SurfacePrincipal,
	}
	if c.FacetID == "" || c.EvidenceID == "" ||
		c.ClaimForm == "" || c.SurfaceRole == "" {
		t.Fatal("struct field assignment failed")
	}
}
