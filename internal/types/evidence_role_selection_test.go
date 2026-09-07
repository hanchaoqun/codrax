package types

import (
	"reflect"
	"testing"
)

func TestB1606SelectedCitationRoleFormsKeepAllDeclaredFormsAndLegacyFallback(t *testing.T) {
	available := AllClaimForms()
	before := append([]ClaimForm(nil), available...)
	for _, form := range AllClaimForms() {
		selected := []RenderedClaimUse{{ClaimForm: form, FacetID: "chosen", EvidenceID: "model-owned"}}
		selectedBefore := append([]RenderedClaimUse(nil), selected...)
		got := SelectedCitationRoleClaimForms(selected, available)
		want := ClaimFormsSupportingCitationRoleAlignment([]ClaimForm{form})
		if !reflect.DeepEqual(got, want) {
			t.Fatalf("selected %s widened to optional alternatives: got=%v want=%v", form, got, want)
		}
		if !reflect.DeepEqual(selected, selectedBefore) || !reflect.DeepEqual(available, before) {
			t.Fatal("selection changed model claims or available view forms")
		}
	}
	if got, want := SelectedCitationRoleClaimForms(nil, available), ClaimFormsSupportingCitationRoleAlignment(available); !reflect.DeepEqual(got, want) {
		t.Fatalf("legacy absent claim_uses lost prior constraints: got=%v want=%v", got, want)
	}
	if got := SelectedCitationRoleClaimForms([]RenderedClaimUse{{}}, available); len(got) != 0 {
		t.Fatalf("empty explicit annotation changed existing emit semantics: %v", got)
	}
	selected := []RenderedClaimUse{{ClaimForm: ClaimDefinitionFact}, {ClaimForm: ClaimCallEdge}, {ClaimForm: ClaimCallEdge}, {ClaimForm: ClaimImportEdge}}
	if got, want := SelectedCitationRoleClaimForms(selected, nil), []ClaimForm{ClaimCallEdge, ClaimImportEdge}; !reflect.DeepEqual(got, want) {
		t.Fatalf("mixed explicit relations lost or duplicated: got=%v want=%v", got, want)
	}
}
