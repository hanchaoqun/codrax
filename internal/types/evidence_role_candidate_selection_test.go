package types

import "testing"

func TestB1642CitationRoleCandidatesRequireExactUniqueLocationAndRole(t *testing.T) {
	a := EvidenceItem{ID: "a", Kind: EvidenceDirect, Source: "Config.properties", LineStart: 1, LineEnd: 1, Subject: "config.key", AnchorKind: AnchorPrecedence, GroundingStatus: GroundingGrounded}
	for _, change := range []struct {
		name string
		edit func(*EvidenceItem)
	}{
		{"different_file", func(b *EvidenceItem) { b.Source = "other.properties" }},
		{"case_sensitive_file", func(b *EvidenceItem) { b.Source = "config.properties" }},
		{"different_line", func(b *EvidenceItem) { b.LineStart = 2; b.LineEnd = 2 }},
		{"different_origin", func(b *EvidenceItem) { b.Origin = ClaimOriginCurrentRepo }},
		{"different_source_ref", func(b *EvidenceItem) { b.EvidenceRef = "other-receipt" }},
		{"different_producer", func(b *EvidenceItem) { b.Producer = "other-producer" }},
		{"different_form", func(b *EvidenceItem) { b.AnchorKind = AnchorStringLiteral }},
		{"same_line_different_role", func(b *EvidenceItem) { b.DiagramRole = EvidenceDiagramRoleConfig }},
		{"same_line_different_owner", func(b *EvidenceItem) { b.OwnerSymbol = "Other" }},
	} {
		t.Run(change.name, func(t *testing.T) {
			b := a
			b.ID = "b"
			change.edit(&b)
			for _, pool := range [][]EvidenceItem{{a, b}, {b, a}} {
				forms := []ClaimForm{ClaimPrecedenceRole, ClaimLiteralValueFact}
				if _, ok := SelectAnswerItemCitationRole(pool, nil, forms, "config.key", ""); ok {
					t.Fatal("ambiguous same-label candidates became one first-pool repair target")
				}
				for _, own := range []EvidenceItem{a, b} {
					selected, ok := SelectAnswerItemCitationRole(pool, []EvidenceItem{own}, forms, "config.key", "")
					if !ok || selected.ID != own.ID {
						t.Fatal("valid current citation was replaced by another candidate")
					}
				}
			}
		})
	}
	duplicate := a
	duplicate.ID = "same-role-copy"
	if _, ok := SelectAnswerItemCitationRole([]EvidenceItem{a, duplicate}, nil, []ClaimForm{ClaimPrecedenceRole}, "config.key", ""); !ok {
		t.Fatal("exact duplicate evidence removed unique repair")
	}
	unknown := a
	unknown.GroundingStatus = GroundingUngrounded
	if selected, ok := SelectAnswerItemCitationRole([]EvidenceItem{a}, []EvidenceItem{unknown}, []ClaimForm{ClaimPrecedenceRole}, "config.key", ""); !ok || selected.GroundingStatus == GroundingUngrounded {
		t.Fatal("ungrounded current source must not self-authorize")
	}
	if _, ok := SelectAnswerItemCitationRole([]EvidenceItem{a}, []EvidenceItem{a}, []ClaimForm{ClaimCallEdge}, "config.key", ""); ok {
		t.Fatal("citation cannot add an unselected claim form")
	}
	external := a
	external.Origin = ClaimOriginPerf
	duplicateExternal := external
	duplicateExternal.ID = "another-runtime-row"
	if _, ok := SelectAnswerItemCitationRole([]EvidenceItem{external, duplicateExternal}, nil, []ClaimForm{ClaimExternalObservation}, "config.key", ""); ok {
		t.Fatal("external same-location rows without query receipts claimed unique")
	}
}
