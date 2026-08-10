package tool

import (
	"reflect"
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/types"
)

func TestAnswerDocumentFieldQuarantineProfilesCoverTypedWireFields(t *testing.T) {
	tests := []struct {
		name    string
		wire    any
		allowed map[string]bool
	}{
		{name: "full document", wire: emitAnswerDocumentV2Params{}, allowed: answerDocumentFullEmitQuarantineProfile.TopLevelAllowed},
		{name: "patch document", wire: emitAnswerDocumentPatchParams{}, allowed: answerDocumentPatchQuarantineProfile.TopLevelAllowed},
		{name: "block", wire: emitAnswerBlockV2{}, allowed: answerDocumentBlockAllowedFields},
		{name: "item", wire: emitAnswerBlockItemV2{}, allowed: answerDocumentItemAllowedFields},
		{name: "citation", wire: emitAnswerCitationV2{}, allowed: answerDocumentCitationAllowedFields},
		{name: "snippet", wire: emitCodeSnippetV2{}, allowed: answerDocumentSnippetAllowedFields},
		{name: "diagram", wire: emitAnswerDiagramV2{}, allowed: answerDocumentDiagramAllowedFields},
		{name: "claim use", wire: types.RenderedClaimUse{}, allowed: answerDocumentClaimUseAllowedFields},
		{name: "edge anchor", wire: types.DiagramEdgeAnchor{}, allowed: answerDocumentEdgeAnchorAllowedFields},
		{name: "participant boundary", wire: types.DiagramParticipantBoundary{}, allowed: answerDocumentParticipantBoundaryAllowedFields},
		{name: "relation claim", wire: types.AnswerRelationClaim{}, allowed: answerDocumentRelationClaimAllowedFields},
		{name: "exact resolution", wire: types.AnswerExactResolution{}, allowed: answerDocumentExactResolutionAllowedFields},
		{name: "missing role", wire: types.AnswerMissingRequestedRole{}, allowed: answerDocumentMissingRoleAllowedFields},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			for _, field := range jsonFieldNames(reflect.TypeOf(tc.wire)) {
				if !tc.allowed[field] {
					t.Errorf("typed JSON field %q is absent from its quarantine allowlist", field)
				}
			}
		})
	}
}

func jsonFieldNames(typ reflect.Type) []string {
	if typ.Kind() == reflect.Pointer {
		typ = typ.Elem()
	}
	out := make([]string, 0, typ.NumField())
	for i := 0; i < typ.NumField(); i++ {
		tag := strings.Split(typ.Field(i).Tag.Get("json"), ",")[0]
		if tag == "" || tag == "-" {
			continue
		}
		out = append(out, tag)
	}
	return out
}
