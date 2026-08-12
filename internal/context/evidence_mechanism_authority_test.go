package context

import (
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/types"
)

func TestEvidencePromptLineCarriesSourceShapeMechanismAuthority(t *testing.T) {
	for _, tc := range []struct {
		kind types.AnchorKind
		want string
	}{
		{types.AnchorDefinition, "source_shape_authority=definition_site_only executable_body=unproven"},
		{types.AnchorTextReference, "source_shape_authority=visible_text_only executable_mechanism=unproven"},
		{types.AnchorStringLiteral, "source_shape_authority=literal_value_only executable_mechanism=unproven"},
	} {
		item := types.EvidenceItem{
			Kind: types.EvidenceDirect, AnchorKind: tc.kind,
			AnchorSymbol: "B/E pairing teaching", Source: "policy.ext", LineStart: 9,
			GroundingStatus: types.GroundingGrounded,
		}
		for _, opts := range []evidenceRenderOptions{{}, {AuthoritativeSurface: true}} {
			got := evidencePromptLine(item, opts)
			if !strings.Contains(got, tc.want) {
				t.Fatalf("prompt evidence line lost %q for kind=%s opts=%+v: %s", tc.want, tc.kind, opts, got)
			}
		}
	}
}

func TestEvidencePromptLineDoesNotDowngradeExecutableCall(t *testing.T) {
	item := types.EvidenceItem{
		Kind: types.EvidenceRelationship, AnchorKind: types.AnchorCall,
		Subject: "parse", Object: "pair", Source: "query.go", LineStart: 10,
		GroundingStatus: types.GroundingGrounded,
	}
	got := evidencePromptLine(item, evidenceRenderOptions{AuthoritativeSurface: true})
	if strings.Contains(got, "executable_mechanism=unproven") {
		t.Fatalf("an exact call anchor must retain its own call-edge contract: %s", got)
	}
}
