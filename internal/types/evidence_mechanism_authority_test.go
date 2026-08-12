package types

import "testing"

func TestEvidenceMechanismAuthorityBoundaryUsesClaimFormNotPath(t *testing.T) {
	tests := []struct {
		name string
		item EvidenceItem
		want string
	}{
		{
			name: "prompt text in Go source",
			item: EvidenceItem{Source: "internal/skill/defaults.go", AnchorKind: AnchorTextReference},
			want: "source_shape_authority=visible_text_only executable_mechanism=unproven",
		},
		{
			name: "documentation text outside a skill path",
			item: EvidenceItem{Source: "docs/architecture.md", AnchorKind: AnchorTextReference},
			want: "source_shape_authority=visible_text_only executable_mechanism=unproven",
		},
		{
			name: "literal in ArkTS",
			item: EvidenceItem{Source: "entry/src/main/ets/App.ets", AnchorKind: AnchorStringLiteral},
			want: "source_shape_authority=literal_value_only executable_mechanism=unproven",
		},
		{
			name: "literal in Cangjie",
			item: EvidenceItem{Source: "src/main.cj", AnchorKind: AnchorStringLiteral},
			want: "source_shape_authority=literal_value_only executable_mechanism=unproven",
		},
		{
			name: "call retains its own exact contract",
			item: EvidenceItem{Source: "query.go", AnchorKind: AnchorCall},
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := EvidenceMechanismAuthorityBoundary(tc.item); got != tc.want {
				t.Fatalf("authority boundary = %q, want %q", got, tc.want)
			}
		})
	}
}
