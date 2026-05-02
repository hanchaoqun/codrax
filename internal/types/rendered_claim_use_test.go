package types

import "testing"

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

func TestRenderedClaimUse_FieldShape(t *testing.T) {
	// Pin the JSON tag shape so a future schema-renaming refactor
	// breaks tests instead of silently breaking emit-on-the-wire.
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
